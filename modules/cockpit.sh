#!/usr/bin/env bash
# modules/cockpit.sh — Cockpit: panel de administración del sistema (web, :9090).

warden_cockpit_install() {
  ensure_pkg cockpit cockpit-bridge
  run "systemctl enable --now cockpit.socket"
  local ip; ip="$(hostname -I 2>/dev/null | awk '{print $1}')"
  ok "Cockpit listo → https://${ip:-<ip-del-server>}:9090"
}
