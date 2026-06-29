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
  ss -tlnH 2>/dev/null | grep -qE "[:.]${port}[[:space:]]" && return 0
  docker ps --format '{{.Ports}}' 2>/dev/null | grep -qE "(^|[^0-9:])$port->" && return 0
  return 1
}

# primer puerto libre desde $1 hacia arriba
_import_free_port() {
  local p="${1:-8000}"
  while _import_port_in_use "$p"; do p=$((p + 1)); done
  echo "$p"
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
  local image cport hport
  image="$(echo "$json" | jq -r --arg s "$svc" '.services[$s].image // empty')"
  [ -n "$image" ] || { rm -rf "$workdir"; die "El servicio '$svc' no define 'image' (¿usa build?). No soportado en v1."; }
  cport="$(echo "$json" | jq -r --arg s "$svc" '.services[$s].ports[0].target // empty')"
  hport="$(echo "$json" | jq -r --arg s "$svc" '.services[$s].ports[0].published // empty')"

  # 6. Volúmenes de datos (sus targets), saltando montajes de sistema
  local -a targets=()
  local t
  while IFS= read -r t; do
    [ -n "$t" ] || continue
    case "$t" in
      /etc/*|/var/run/*|/run/*|/proc/*|/sys/*|/usr/*|/dev/*|*docker.sock) continue ;;
    esac
    targets+=("$t")
  done < <(echo "$json" | jq -r --arg s "$svc" '.services[$s].volumes[]?.target // empty')

  # 7. Puerto libre (si la app expone uno)
  local port=""
  [ -n "$hport" ] && port="$(_import_free_port "$hport")"

  # 8. KIND
  local kind="none"
  [ "${#targets[@]}" -gt 0 ] && kind="files"

  # 9. ¿necesita HTTPS? (heurística: imágenes conocidas que exigen contexto seguro)
  local needs_https=0
  case "$image" in
    *vaultwarden*|*bitwarden*|*keycloak*|*authelia*) needs_https=1 ;;
  esac

  # --- Generar las partes dinámicas del compose -------------------------------
  local envname; envname="$(_import_envname "$tag")"
  local ports_block="" vols_block="" paths_str="" bn hostpath
  [ -n "$cport" ] && ports_block=$'    ports:\n      - "${'"$envname"'_PORT:-'"$port"'}:'"$cport"$'"\n'
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
  [ -n "$port" ] && echo "   Puerto:    $port$([ -n "$hport" ] && [ "$port" != "$hport" ] && echo " (el original $hport estaba ocupado)")"
  [ "${#targets[@]}" -gt 0 ] && echo "   Datos:     ${targets[*]}"
  echo "   Archivos:  site/catalog/$tag.component · site/stacks/$tag/docker-compose.yml"
  [ "$nsvc" -gt 1 ] && warn "El compose tenía $nsvc servicios — importé solo '$svc'. Si necesita base de datos u otro, armalo a mano."
  [ "$needs_https" = 1 ] && warn "Esta app suele EXIGIR HTTPS (contexto seguro): exponela por el túnel de Cloudflare con subdominio, no http plano."
  echo
  echo "Siguiente paso:  warden install-component $tag"
}
