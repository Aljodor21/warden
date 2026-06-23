#!/usr/bin/env bash
# modules/reset.sh — instalación limpia: borra TODO lo que warden instaló
# (contenedores, datos, config) para reinstalar desde cero sin saturar.
#
#   warden reset            — borra apps/datos/config (conserva la llave age)
#   warden reset --full     — además borra la llave age (¡rompe backups cifrados!)
#
# NO toca: el disco de backup externo, paquetes del sistema (Docker, avahi,
# zsh...), ni tu site/. Esto es "limpiar la capa de warden", no reinstalar el SO.

_reset_down() {  # <archivo compose> [override]
  local compose="$1" override="${2:-}"
  [ -f "$compose" ] || return 0
  if [ -n "$override" ] && [ -f "$override" ]; then
    _compose -f "$compose" -f "$override" down -v --remove-orphans 2>/dev/null
  else
    _compose -f "$compose" down -v --remove-orphans 2>/dev/null
  fi
}

warden_reset() {
  local full="${1:-}"
  echo "Esto va a BORRAR:"
  echo "  - Todos los contenedores/datos de apps instaladas por warden (catálogo + dashboard)"
  echo "  - ${WARDEN_DATA:-/srv/warden} (datos de Immich/NAS/etc.)"
  echo "  - /etc/warden (config, usuarios del NAS, secretos generados)"
  echo "  - Imágenes de Docker que queden sin usar (docker image prune)"
  [ "$full" = "--full" ] && echo "  - /etc/warden/age.key (¡tu llave de cifrado! sin ella no se descifran backups viejos)"
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
        _reset_down "$WARDEN_ROOT/$COMP_INSTALL" "/etc/warden/$tag/docker-compose.override.yml" ;;
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

  ok "Reset completo. Corré 'sudo ./bootstrap.sh' para reinstalar desde cero."
}
