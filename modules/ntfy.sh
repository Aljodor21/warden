#!/usr/bin/env bash
# modules/ntfy.sh — ntfy: servidor de notificaciones push (alertas).

NTFY_HOME="${NTFY_HOME:-/etc/warden/ntfy}"

warden_ntfy_install() {
  run "mkdir -p '$NTFY_HOME/etc' '$NTFY_HOME/cache'"
  local ip; ip="$(hostname -I 2>/dev/null | awk '{print $1}')"
  if [ "${WARDEN_DRY_RUN:-0}" != 1 ] && [ ! -f "$NTFY_HOME/etc/server.yml" ]; then
    cat > "$NTFY_HOME/etc/server.yml" <<EOF
base-url: "${NTFY_BASE_URL:-http://${ip:-localhost}:${NTFY_PORT:-8080}}"
cache-file: "/var/cache/ntfy/cache.db"
EOF
  fi
  export NTFY_HOME
  log "Levantando ntfy"
  run "_compose -f '$WARDEN_ROOT/stacks/ntfy/docker-compose.yml' up -d"
  local url="http://${ip:-localhost}:${NTFY_PORT:-8080}"
  if [ "${WARDEN_DRY_RUN:-0}" != 1 ]; then
    mkdir -p /etc/warden
    printf '%s\n' "$url" > /etc/warden/ntfy-url
  fi
  ok "ntfy → $url"
  ok "URL guardada en /etc/warden/ntfy-url — backup y watch mandan alertas al topic 'warden'."
  ok "Instalá la app ntfy en tu celular y suscribite al topic 'warden' en $url"
}
