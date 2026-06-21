#!/usr/bin/env bash
# modules/homepage.sh — Homepage: la cara única del server (Opción A).
# Genera su configuración desde el catálogo y levanta el stack.

HOMEPAGE_CONFIG="${HOMEPAGE_CONFIG:-/etc/warden/homepage}"

# Genera settings/widgets/services.yaml desde el catálogo y los servicios activos.
warden_homepage_config() {
  local ip svc up; ip="$(hostname -I 2>/dev/null | awk '{print $1}')"
  run "mkdir -p '$HOMEPAGE_CONFIG'"
  [ "${WARDEN_DRY_RUN:-0}" = 1 ] && { echo "[dry-run] generaría la config de Homepage en $HOMEPAGE_CONFIG"; return 0; }

  cat > "$HOMEPAGE_CONFIG/settings.yaml" <<EOF
title: ${WARDEN_HOSTNAME:-warden}
theme: light
color: slate
headerStyle: boxed
cardBlur: md
target: _blank
layout:
  Dashboard:
    style: row
    columns: 4
  Almacenamiento:
    style: row
    columns: 3
  Apps:
    style: row
    columns: 4
EOF
  cat > "$HOMEPAGE_CONFIG/widgets.yaml" <<EOF
- greeting:
    text_size: xl
    text: ${WARDEN_HOSTNAME:-warden}
- datetime:
    text_size: l
    format:
      timeStyle: short
      hour12: false
- resources:
    label: sistema
    cpu: true
    memory: true
    disk: /
    uptime: true
- search:
    provider: duckduckgo
    target: _blank
EOF
  # Estilo "iCloud + CasaOS": claro, suave, cards redondeadas con sombra,
  # grupos como secciones bien separadas.
  cat > "$HOMEPAGE_CONFIG/custom.css" <<'EOF'
:root {
  --color-50: 248 250 252;
}
body, #page_container {
  background: linear-gradient(160deg, #eef2f7 0%, #e4e9f0 45%, #dfe5ed 100%) !important;
}
#information-widgets, .services-group, .service-card, #bookmarks {
  border-radius: 18px !important;
}
.service-card, #information-widgets > div {
  box-shadow: 0 1px 3px rgba(15,23,42,.06), 0 8px 20px rgba(15,23,42,.06) !important;
  background: rgba(255,255,255,.72) !important;
  backdrop-filter: blur(10px);
}
h2, h3 { font-weight: 600 !important; letter-spacing: -.01em; }
EOF
  cat > "$HOMEPAGE_CONFIG/docker.yaml" <<'EOF'
warden:
  socket: /var/run/docker.sock
EOF
  # Quitar los bookmarks de ejemplo (Github/Reddit/YouTube) que Homepage trae por defecto.
  printf -- '- Enlaces:\n    - warden:\n        - abbr: WD\n          href: https://github.com/Aljodor21/warden\n' \
    > "$HOMEPAGE_CONFIG/bookmarks.yaml"

  svc="$HOMEPAGE_CONFIG/services.yaml"
  up="$(docker ps --format '{{.Names}}' 2>/dev/null)"
  : > "$svc"

  # --- Grupo Dashboard: los paneles de warden que estén activos ---
  local dash=""
  systemctl is-active --quiet cockpit.socket 2>/dev/null && dash="${dash}    - Cockpit:
        href: https://${ip:-localhost}:9090
        description: panel del sistema
        icon: cockpit.png
"
  grep -qx backrest <<<"$up" && dash="${dash}    - Backrest:
        href: http://${ip:-localhost}:9898
        description: backups
        icon: restic.png
        server: warden
        container: backrest
"
  grep -qx ntfy <<<"$up" && dash="${dash}    - ntfy:
        href: http://${ip:-localhost}:8080
        description: alertas
        icon: ntfy.png
        server: warden
        container: ntfy
"
  [ -n "$dash" ] && { echo "- Dashboard:" >> "$svc"; printf '%s' "$dash" >> "$svc"; }

  # --- Apps y Almacenamiento: componentes del catálogo corriendo (o públicos por dominio).
  # Las que son solo "files" (NAS, carpetas compartidas) van a Almacenamiento,
  # el resto (con BD o sin datos) van a Apps — como CasaOS separa Files de Apps.
  local apps="" storage="" tag entry kind
  while IFS='|' read -r tag _ kind _; do
    [ -n "$tag" ] || continue
    entry="$(
      catalog_load "$tag" || exit 0
      href=""
      if [ -n "${COMP_CF_HOST:-}" ]; then
        href="https://$COMP_CF_HOST"
      elif [ -n "${COMP_CF_PORT:-}" ]; then
        cont="${COMP_CONTAINER:-$tag}"
        grep -qx "$cont" <<<"$up" || exit 0
        href="http://${ip:-localhost}:$COMP_CF_PORT"
      elif [ "${COMP_KIND:-}" = "files" ]; then
        cont="${COMP_CONTAINER:-$tag}"
        grep -qx "$cont" <<<"$up" || exit 0
        href="#"
      else
        exit 0
      fi
      printf '    - %s:\n        href: %s\n        description: %s\n        icon: %s.png\n' "$COMP_NAME" "$href" "$COMP_TAG" "${COMP_ICON:-$COMP_TAG}"
      [ -n "${cont:-}" ] && printf '        server: warden\n        container: %s\n' "$cont"
    )"
    if [ -n "$entry" ]; then
      if [ "$kind" = "files" ]; then storage="${storage}${entry}"$'\n'
      else apps="${apps}${entry}"$'\n'
      fi
    fi
  done < <(catalog_each)
  [ -n "$storage" ] && { echo "- Almacenamiento:" >> "$svc"; printf '%s' "$storage" >> "$svc"; }
  [ -n "$apps" ] && { echo "- Apps:" >> "$svc"; printf '%s' "$apps" >> "$svc"; }

  # Fallback: si todo quedó vacío, al menos Cockpit.
  [ -s "$svc" ] || printf -- '- Sistema:\n    - Cockpit:\n        href: https://%s:9090\n' "${ip:-localhost}" > "$svc"
}

warden_homepage_install() {
  log "Generando configuración de Homepage desde el catálogo"
  warden_homepage_config
  export HOMEPAGE_CONFIG
  log "Levantando Homepage"
  run "_compose -f '$WARDEN_ROOT/stacks/homepage/docker-compose.yml' up -d"
  local ip; ip="$(hostname -I 2>/dev/null | awk '{print $1}')"
  ok "Homepage → http://${ip:-<ip>}:${HOMEPAGE_PORT:-7575}"
}
