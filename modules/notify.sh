#!/usr/bin/env bash
# modules/notify.sh — notificaciones push vía ntfy.
#
# Configuración: guardar la URL del servidor ntfy en /etc/warden/ntfy-url
# con 'warden notify-url <url>'. Sin eso, todas las funciones son no-op.

NTFY_URL_FILE="/etc/warden/ntfy-url"
NTFY_TOPIC="${NTFY_TOPIC:-warden}"

# warden_notify <título> <mensaje> [prioridad] [tags]
# prioridad: low | default | high | urgent
# tags: nombres emoji separados por coma (ej: "rotating_light,warning")
warden_notify() {
  local title="$1" msg="$2" priority="${3:-default}" tags="${4:-}"
  local url
  url="$(cat "$NTFY_URL_FILE" 2>/dev/null | tr -d '[:space:]')"
  [ -n "$url" ] || return 0
  local -a headers=(-H "Title: $title" -H "Priority: $priority")
  [ -n "$tags" ] && headers+=(-H "Tags: $tags")
  curl -sf "${headers[@]}" -d "$msg" "$url/$NTFY_TOPIC" >/dev/null 2>&1 || true
}

# warden_notify_url_set <url> — guarda la URL del servidor ntfy y envía
# una notificación de prueba para confirmar que llega.
warden_notify_url_set() {
  local url="${1:-}"
  [ -n "$url" ] || die "Falta la URL. Uso: warden notify-url <http://ip:puerto>"
  mkdir -p /etc/warden
  printf '%s\n' "$url" > "$NTFY_URL_FILE"
  ok "URL de ntfy guardada: $url"
  warden_notify "warden conectado" "Las notificaciones push están activas en este servidor." "default" "bell"
}
