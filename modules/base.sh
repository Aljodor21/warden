#!/usr/bin/env bash
# modules/base.sh — preparación base del sistema (paquetes, hora, hostname).

warden_base_install() {
  log "Base: actualizando índices de paquetes"
  pkg_update || warn "No se pudieron actualizar los índices"

  log "Base: utilidades esenciales"
  ensure_pkg curl
  ensure_pkg git
  ensure_pkg jq
  ensure_pkg ca-certificates ca-certificates

  if [ -n "${WARDEN_TIMEZONE:-}" ]; then
    log "Zona horaria → $WARDEN_TIMEZONE"
    run "timedatectl set-timezone '$WARDEN_TIMEZONE'"
  fi
  if [ -n "${WARDEN_HOSTNAME:-}" ]; then
    log "Hostname → $WARDEN_HOSTNAME"
    run "hostnamectl set-hostname '$WARDEN_HOSTNAME'"
  fi

  ok "Base lista"
}
