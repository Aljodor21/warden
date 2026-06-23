#!/usr/bin/env bash
# modules/reset.sh — instalación limpia: borra TODO lo que warden instaló o
# configuró, dejando el sistema como antes de instalar warden.
#
#   warden reset
#
# Borra: contenedores/datos/config de warden, la llave age, el túnel de
# Cloudflare (de tu cuenta, no solo local), la conexión de Tailscale, y
# resetea el firewall (ufw) a desactivado/sin reglas. NO queda nada de lo
# que warden configuró.
#
# NO toca nunca: el disco de backup externo, ni los paquetes del sistema
# (Docker, avahi, zsh, cloudflared, tailscale — los binarios se quedan
# instalados, solo se borra SU CONFIGURACIÓN/ESTADO), ni tu site/.

_reset_down() {  # <archivo compose> [override] [envfile]
  local compose="$1" override="${2:-}" envfile="${3:-}"
  [ -f "$compose" ] || return 0
  local args=(-f "$compose")
  [ -n "$override" ] && [ -f "$override" ] && args+=(-f "$override")
  [ -n "$envfile" ] && [ -f "$envfile" ] && args+=(--env-file "$envfile")
  _compose "${args[@]}" down -v --remove-orphans || warn "No bajó completo: $compose (revisalo a mano con docker ps -a)"
}

warden_reset() {
  echo "Esto va a BORRAR TODO lo que warden instaló o configuró:"
  echo "  - Todos los contenedores/datos de apps instaladas por warden (catálogo + dashboard)"
  echo "  - ${WARDEN_DATA:-/srv/warden} (datos de Immich/NAS/etc.)"
  echo "  - /etc/warden (config, usuarios del NAS, secretos, la llave age)"
  echo "  - Imágenes de Docker que queden sin usar (docker image prune)"
  echo "  - El túnel de Cloudflare (se BORRA de tu cuenta, no solo local) y su config"
  echo "  - La conexión de Tailscale (este server se desconecta de tu tailnet)"
  echo "  - El firewall (ufw vuelve a desactivado, sin reglas)"
  echo "No toca: el disco de backup externo, ni los paquetes del sistema (quedan instalados)."
  echo

  if [ "${WARDEN_DRY_RUN:-0}" != 1 ]; then
    read -rp "Escribí exactamente BORRAR para continuar: " ok
    [ "$ok" = "BORRAR" ] || { echo "Cancelado."; return 1; }
  fi

  log "Bajando stacks del dashboard"
  _reset_down "$WARDEN_ROOT/stacks/homepage/docker-compose.yml"
  _reset_down "$WARDEN_ROOT/stacks/backrest/docker-compose.yml"
  _reset_down "$WARDEN_ROOT/stacks/ntfy/docker-compose.yml"

  log "Bajando apps del catálogo"
  local tag
  while IFS='|' read -r tag _ _ _; do
    [ -n "$tag" ] || continue
    catalog_load "$tag" || continue
    case "${COMP_INSTALL:-}" in
      */docker-compose.yml)
        _reset_down "$WARDEN_ROOT/$COMP_INSTALL" "/etc/warden/$tag/docker-compose.override.yml" "/etc/warden/secrets/$tag.env" ;;
    esac
  done < <(catalog_each)

  log "Limpiando imágenes de Docker sin usar (libera espacio en disco)"
  run "docker image prune -af"

  log "Desactivando timers"
  run "systemctl disable --now warden-backup.timer warden-verify.timer 2>/dev/null || true"
  run "rm -f /etc/systemd/system/warden-backup.* /etc/systemd/system/warden-verify.*"
  run "systemctl daemon-reload"

  log "Borrando datos generados (${WARDEN_DATA:-/srv/warden})"
  run "rm -rf '${WARDEN_DATA:-/srv/warden}'"

  log "Borrando config de warden (incluida la llave age)"
  run "rm -rf /etc/warden"

  if has cloudflared && [ -f /etc/cloudflared/config.yml ]; then
    log "Borrando el túnel de Cloudflare (de tu cuenta, no solo local)"
    local tid; tid="$(awk '/^tunnel:/{print $2; exit}' /etc/cloudflared/config.yml 2>/dev/null)"
    run "systemctl disable --now cloudflared 2>/dev/null || true"
    [ -n "$tid" ] && run "cloudflared tunnel delete -f '$tid' 2>/dev/null || true"
    run "rm -rf /etc/cloudflared"
  fi

  if has tailscale; then
    log "Desconectando Tailscale de este server"
    run "tailscale logout 2>/dev/null || true"
    run "systemctl disable --now tailscaled 2>/dev/null || true"
  fi

  if has ufw; then
    log "Reseteando firewall (ufw queda desactivado, sin reglas)"
    run "ufw --force reset >/dev/null 2>&1 || true"
  fi

  if [ -f /etc/systemd/system/warden-panel.service ]; then
    log "Borrando el panel web"
    run "systemctl disable --now warden-panel 2>/dev/null || true"
    run "rm -f /etc/systemd/system/warden-panel.service /usr/local/bin/warden-panel"
    run "systemctl daemon-reload"
  fi

  ok "Reset completo — el sistema queda como antes de instalar warden. Corré 'sudo ./bootstrap.sh' para reinstalar desde cero."
}
