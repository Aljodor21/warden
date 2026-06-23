#!/usr/bin/env bash
# modules/reset.sh — instalación limpia: borra TODO lo que warden instaló
# (contenedores, datos, config) para reinstalar desde cero sin saturar.
#
#   warden reset            — borra apps/datos/config (conserva la llave age,
#                              el túnel de Cloudflare, Tailscale y el firewall)
#   warden reset --full     — además: llave age, túnel de Cloudflare (LO BORRA
#                              de tu cuenta, no solo local), Tailscale
#                              (desconecta este server) y firewall (vuelve a
#                              ufw sin reglas/desactivado). No queda NADA de
#                              lo que warden configuró.
#
# NO toca nunca: el disco de backup externo, ni los paquetes del sistema
# (Docker, avahi, zsh, cloudflared, tailscale — los binarios se quedan, solo
# se borra SU CONFIGURACIÓN/ESTADO en modo --full), ni tu site/.

_reset_down() {  # <archivo compose> [override] [envfile]
  local compose="$1" override="${2:-}" envfile="${3:-}"
  [ -f "$compose" ] || return 0
  local args=(-f "$compose")
  [ -n "$override" ] && [ -f "$override" ] && args+=(-f "$override")
  [ -n "$envfile" ] && [ -f "$envfile" ] && args+=(--env-file "$envfile")
  _compose "${args[@]}" down -v --remove-orphans || warn "No bajó completo: $compose (revisalo a mano con docker ps -a)"
}

warden_reset() {
  local full="${1:-}"
  echo "Esto va a BORRAR:"
  echo "  - Todos los contenedores/datos de apps instaladas por warden (catálogo + dashboard)"
  echo "  - ${WARDEN_DATA:-/srv/warden} (datos de Immich/NAS/etc.)"
  echo "  - /etc/warden (config, usuarios del NAS, secretos generados)"
  echo "  - Imágenes de Docker que queden sin usar (docker image prune)"
  if [ "$full" = "--full" ]; then
    echo "  - /etc/warden/age.key (¡tu llave de cifrado! sin ella no se descifran backups viejos)"
    echo "  - El túnel de Cloudflare (se BORRA de tu cuenta, no solo local) y su config"
    echo "  - La conexión de Tailscale (este server se desconecta de tu tailnet)"
    echo "  - El firewall (ufw vuelve a desactivado, sin reglas)"
  fi
  echo "No toca: el disco de backup externo, ni los paquetes del sistema."
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

  log "Borrando config de warden"
  if [ "$full" = "--full" ]; then
    run "rm -rf /etc/warden"
  else
    if [ -f /etc/warden/age.key ]; then
      run "find /etc/warden -mindepth 1 ! -path /etc/warden/age.key -delete"
      warn "Se conservó /etc/warden/age.key. Para borrar todo: warden reset --full"
    else
      run "rm -rf /etc/warden"
    fi
  fi

  if [ "$full" = "--full" ]; then
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
  fi

  ok "Reset completo. Corré 'sudo ./bootstrap.sh' para reinstalar desde cero."
}
