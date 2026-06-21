#!/usr/bin/env bash
# modules/cockpit.sh — Cockpit: panel de administración del sistema (web, :9090).

warden_cockpit_install() {
  ensure_pkg cockpit cockpit-bridge
  run "systemctl enable --now cockpit.socket"
  ok "Cockpit listo → https://$(warden_host):9090"
}
