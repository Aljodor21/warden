#!/usr/bin/env bash
# modules/watch.sh — monitorea contenedores del catálogo y alerta por ntfy
# si alguno cae o entra en restart loop.
#
# El estado entre corridas vive en /run/warden-watch/ (volátil: se limpia al
# reiniciar, que es exactamente lo que queremos — un reinicio del server no
# dispara alertas de "app caída").

WATCH_STATE_DIR="/run/warden-watch"

warden_watch() {
  mkdir -p "$WATCH_STATE_DIR"

  local tag container status prev_status state_file
  while IFS='|' read -r tag _ _ _; do
    [ -n "$tag" ] || continue
    catalog_load "$tag" || continue

    # Saltar apps de CI/CD — las maneja GitHub Actions, no warden.
    is_deployed_install "${COMP_INSTALL:-}" && continue

    container="${COMP_CONTAINER:-$tag}"
    [ -n "$container" ] || continue

    status="$(docker inspect --format '{{.State.Status}}' "$container" 2>/dev/null)"
    [ -n "$status" ] || continue  # no está instalado — ignorar

    state_file="$WATCH_STATE_DIR/$tag"
    prev_status="$(cat "$state_file" 2>/dev/null || echo "ok")"

    case "$status" in
      running)
        if [ "$prev_status" = "alerted" ]; then
          warden_notify "$COMP_NAME se recuperó" \
            "El contenedor '$container' volvió a estar activo." \
            "default" "white_check_mark"
        fi
        echo "ok" > "$state_file"
        ;;
      restarting|exited|dead)
        if [ "$prev_status" != "alerted" ]; then
          warden_notify "$COMP_NAME caído" \
            "El contenedor '$container' está en estado '$status'. Revisá con: docker logs $container" \
            "high" "rotating_light"
          echo "alerted" > "$state_file"
        fi
        ;;
    esac
  done < <(catalog_each)
}

# Instala el timer de systemd para monitoreo continuo (cada 5 min).
warden_watch_install() {
  local src="$WARDEN_ROOT/backup/systemd" u
  for u in warden-watch.service warden-watch.timer; do
    run "install -m 644 '$src/$u' '/etc/systemd/system/$u'"
  done
  run "systemctl daemon-reload"
  run "systemctl enable --now warden-watch.timer"
  ok "Monitor de contenedores activo (cada 5 min)"
}
