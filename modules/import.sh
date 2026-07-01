#!/usr/bin/env bash
# modules/import.sh — importa una app externa (docker-compose) al formato de
# warden, generando dos archivos en tu site/ (privado, no en el repo):
#   site/catalog/<tag>.component
#   site/stacks/<tag>/docker-compose.yml
#
# v1: pensado para apps de UN SOLO servicio (el caso común). Si el compose
# trae varios servicios (ej. app + base de datos), importa el principal y
# avisa que el resto hay que armarlo a mano — no inventa una integración.

# ¿el puerto ya está ocupado? Cruza 3 fuentes: lo declarado en el catálogo,
# lo que escucha el SO, y los mapeos de contenedores docker.
_import_port_in_use() {
  local port="$1"
  grep -rhoE 'COMP_CF_PORT="[0-9]+"' "$WARDEN_ROOT/catalog" "$WARDEN_ROOT/site/catalog" 2>/dev/null \
    | grep -q "\"$port\"" && return 0
  # -tuln: TCP y UDP escuchando (antes solo TCP — no cazaba conflictos UDP como
  # el 68/udp del cliente DHCP, que hacía fallar apps tipo AdGuard).
  ss -tulnH 2>/dev/null | grep -qE "[:.]${port}[[:space:]]" && return 0
  docker ps --format '{{.Ports}}' 2>/dev/null | grep -qE "(^|[^0-9:])$port->" && return 0
  return 1
}

# primer puerto libre desde $1 hacia arriba
_import_free_port() {
  local p="${1:-8000}"
  while _import_port_in_use "$p"; do p=$((p + 1)); done
  echo "$p"
}

# Rango "web" de un puerto de contenedor: menor = más probable que sea la UI
# web. Se usa para elegir a qué puerto apunta el link del dashboard.
_import_web_rank() {
  case "$1" in
    80) echo 0 ;; 3000) echo 1 ;; 8080) echo 2 ;; 8000) echo 3 ;;
    8096) echo 4 ;; 5000) echo 5 ;; 9000) echo 6 ;; 443) echo 7 ;; 8443) echo 8 ;;
    *) echo 100 ;;
  esac
}

# sanitiza un texto para usarlo como tag: minúsculas y solo [a-z0-9-]
_import_slug() {
  echo "$1" | tr '[:upper:]' '[:lower:]' | sed -E 's/[^a-z0-9]+/-/g; s/^-+//; s/-+$//'
}

# convierte un tag (kebab) en una etiqueta de env: UPPER con guiones a '_'
_import_envname() {
  echo "$1" | tr '[:lower:]-' '[:upper:]_'
}

# Title Case simple para el nombre legible (uptime-kuma -> Uptime Kuma)
_import_pretty() {
  echo "$1" | sed -E 's/[-_]+/ /g' | awk '{for(i=1;i<=NF;i++)$i=toupper(substr($i,1,1)) substr($i,2)}1'
}

warden_import() {
  local source="$1" tag="${2:-}"
  [ -n "$source" ] || die "Uso: warden import <url-o-archivo> [tag]"
  has docker || die "Falta docker (se usa para validar/normalizar el compose)"
  has jq || ensure_pkg jq jq

  # 1. Traer el compose a una carpeta temporal
  local workdir tmp
  workdir="$(mktemp -d)"; tmp="$workdir/docker-compose.yml"
  if [[ "$source" =~ ^https?:// ]]; then
    log "Bajando el compose de $source"
    curl -fsSL "$source" -o "$tmp" || { rm -rf "$workdir"; die "No pude bajar el compose desde esa URL"; }
  elif [ -f "$source" ]; then
    cp "$source" "$tmp"
  else
    rm -rf "$workdir"; die "No existe el archivo ni parece una URL: $source"
  fi

  # 2. Normalizar con 'docker compose config' -> JSON (valida + canoniza
  #    puertos/volúmenes a su forma larga, así el parseo con jq es confiable).
  local json
  json="$(cd "$workdir" && docker compose -f docker-compose.yml config --format json 2>/dev/null)" \
    || { rm -rf "$workdir"; die "El compose no es válido o necesita variables que no tiene. Ajustalo o probá otro."; }

  # 3. Servicio principal: el primero que publica puertos; si ninguno, el primero.
  local svc nsvc
  svc="$(echo "$json" | jq -r '([.services|to_entries[]|select(.value.ports!=null)][0].key) // (.services|keys|.[0]) // empty')"
  [ -n "$svc" ] || { rm -rf "$workdir"; die "No encontré ningún servicio en el compose"; }
  nsvc="$(echo "$json" | jq -r '.services|length')"

  # 4. Tag (del argumento, o derivado del servicio)
  [ -n "$tag" ] || tag="$svc"
  tag="$(_import_slug "$tag")"
  [ -n "$tag" ] || { rm -rf "$workdir"; die "No pude derivar un tag válido"; }
  if [ -f "$WARDEN_ROOT/site/catalog/$tag.component" ] || [ -f "$WARDEN_ROOT/catalog/$tag.component" ]; then
    rm -rf "$workdir"; die "Ya existe una app con el tag '$tag'. Probá: warden import <fuente> <otro-tag>"
  fi

  # 5. Imagen + puerto del servicio principal
  local image
  image="$(echo "$json" | jq -r --arg s "$svc" '.services[$s].image // empty')"
  [ -n "$image" ] || { rm -rf "$workdir"; die "El servicio '$svc' no define 'image' (¿usa build?). No soportado en v1."; }

  # 6. Volúmenes de datos (sus targets), saltando montajes de sistema
  local -a targets=()
  local t
  while IFS= read -r t; do
    [ -n "$t" ] || continue
    case "$t" in
      # Solo archivos de sistema puntuales (no todo /etc/* — hay apps que
      # guardan sus datos en /etc/<app>, ej. clamav en /etc/clamav).
      /etc/timezone|/etc/localtime|/etc/hosts|/etc/resolv.conf) continue ;;
      /var/run/*|/run/*|/proc/*|/sys/*|/dev/*|*/docker.sock) continue ;;
    esac
    targets+=("$t")
  done < <(echo "$json" | jq -r --arg s "$svc" '.services[$s].volumes[]?.target // empty')

  # 7. Puertos: publicar TODOS los que declara la app, cada uno con un puerto de
  #    host libre. Los pares tcp/udp del MISMO puerto de contenedor comparten
  #    host (ej. 54->53/tcp y 54->53/udp). Elegir el "web" para el link.
  local ports_block="" port="" all_ports=""
  local -a cp_order=()
  local -A CP_PROTOS CP_PUB
  local pub cport proto
  while IFS='|' read -r pub cport proto; do
    [ -n "$cport" ] || continue
    if [ -z "${CP_PROTOS[$cport]:-}" ]; then cp_order+=("$cport"); CP_PUB[$cport]="${pub:-$cport}"; fi
    CP_PROTOS[$cport]="${CP_PROTOS[$cport]:-} $proto"
  done < <(echo "$json" | jq -r --arg s "$svc" '.services[$s].ports[]? | "\(.published // "")|\(.target)|\(.protocol // "tcp")"')

  local assigned=" " link_rank=999 free rank pr
  for cport in "${cp_order[@]}"; do
    free="${CP_PUB[$cport]}"
    while _import_port_in_use "$free" || [[ "$assigned" == *" $free "* ]]; do free=$((free + 1)); done
    assigned="$assigned$free "
    for pr in ${CP_PROTOS[$cport]}; do
      if [ "$pr" = udp ]; then ports_block+="      - \"$free:$cport/udp\"\n"; else ports_block+="      - \"$free:$cport\"\n"; fi
    done
    all_ports="$all_ports $free→$cport"
    rank="$(_import_web_rank "$cport")"
    if [ "$rank" -lt "$link_rank" ]; then link_rank="$rank"; port="$free"; fi
  done
  [ -n "$ports_block" ] && ports_block=$'    ports:\n'"$ports_block"

  # 8. KIND
  local kind="none"
  [ "${#targets[@]}" -gt 0 ] && kind="files"

  # 9. ¿necesita HTTPS? (heurística: imágenes conocidas que exigen contexto seguro)
  local needs_https=0
  case "$image" in
    *vaultwarden*|*bitwarden*|*keycloak*|*authelia*) needs_https=1 ;;
  esac

  # --- Generar las partes dinámicas del compose (ports_block ya se armó arriba) -
  local vols_block="" paths_str="" bn hostpath t
  if [ "${#targets[@]}" -gt 0 ]; then
    vols_block=$'    volumes:\n'
    for t in "${targets[@]}"; do
      if [ "${#targets[@]}" -eq 1 ]; then
        hostpath='${WARDEN_DATA:-/srv/warden}/'"$tag"
        paths_str='"${WARDEN_DATA:-/srv/warden}/'"$tag"'"'
      else
        bn="$(_import_slug "$(basename "$t")")"; [ -n "$bn" ] || bn="data"
        hostpath='${WARDEN_DATA:-/srv/warden}/'"$tag"'/'"$bn"
        paths_str+=' "${WARDEN_DATA:-/srv/warden}/'"$tag"'/'"$bn"'"'
      fi
      vols_block+='      - "'"$hostpath"':'"$t"$'"\n'
    done
  fi

  # --- Escribir el stack ------------------------------------------------------
  local stackdir="$WARDEN_ROOT/site/stacks/$tag"
  mkdir -p "$stackdir"
  {
    echo "# $tag — importado por warden ($(date +%Y-%m-%d)) desde:"
    echo "#   $source"
    echo "# Revisá/ajustá si hace falta (env, volúmenes, etc.)."
    echo "name: $tag"
    echo "services:"
    echo "  $tag:"
    echo "    image: $image"
    echo "    container_name: $tag"
    echo "    restart: unless-stopped"
    printf '%b' "$ports_block"
    echo "    environment:"
    echo "      TZ: \"\${TZ:-UTC}\""
    printf '%b' "$vols_block"
  } > "$stackdir/docker-compose.yml"

  # --- Escribir el component --------------------------------------------------
  local name; name="$(_import_pretty "$tag")"
  mkdir -p "$WARDEN_ROOT/site/catalog"
  {
    echo "# $name — importada con 'warden import' desde $source"
    echo "COMP_NAME=\"$name\""
    echo "COMP_TAG=\"$tag\""
    echo "COMP_KIND=\"$kind\""
    echo "COMP_PATHS=( $paths_str )"
    echo "COMP_EXCLUDES=()"
    echo "COMP_DB_TYPE=\"\""
    echo "COMP_INSTALL=\"site/stacks/$tag/docker-compose.yml\""
    echo "COMP_CONTAINER=\"$tag\""
    echo "COMP_SECRETS=()"
    echo "COMP_ICON=\"$tag\""
    echo "COMP_CF_HOST=\"\""
    echo "COMP_CF_PORT=\"${port}\""
    echo "COMP_NOTE=\"Importada con 'warden import'. Revisá el .component y el stack antes de instalar.\""
  } > "$WARDEN_ROOT/site/catalog/$tag.component"

  rm -rf "$workdir"

  # --- Resumen ----------------------------------------------------------------
  ok "Importada '$name' (tag: $tag)"
  echo "   Imagen:    $image"
  echo "   Tipo:      $kind$([ "$kind" = none ] && echo ' (sin datos que respaldar)')"
  [ -n "$all_ports" ] && echo "   Puertos:  $all_ports  (host→contenedor)"
  [ -n "$port" ] && echo "   El link del dashboard apunta a: $port  (podés cambiarlo en Catálogo → editar)"
  [ "${#targets[@]}" -gt 0 ] && echo "   Datos:     ${targets[*]}"
  echo "   Archivos:  site/catalog/$tag.component · site/stacks/$tag/docker-compose.yml"
  [ "$nsvc" -gt 1 ] && warn "El compose tenía $nsvc servicios — importé solo '$svc'. Si necesita base de datos u otro, armalo a mano."
  [ "$needs_https" = 1 ] && warn "Esta app suele EXIGIR HTTPS (contexto seguro): exponela por el túnel de Cloudflare con subdominio, no http plano."
  echo
  echo "Siguiente paso:  warden install-component $tag"
}
