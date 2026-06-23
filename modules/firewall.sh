#!/usr/bin/env bash
# modules/firewall.sh — firewall equilibrado con ufw.
#
# OJO: Docker NO respeta ufw para los puertos que publica. Las BD se cierran
# fijando el bind a 127.0.0.1 en el compose, no con reglas de ufw.

warden_firewall_install() {
  ensure_pkg ufw ufw
  local lan="${WARDEN_LAN:-}"

  log "Aplicando política equilibrada de ufw"
  run "ufw --force reset"
  run "ufw default deny incoming"
  run "ufw default allow outgoing"

  # VPN de confianza: acceso total por Tailscale (si está).
  if ip link show tailscale0 >/dev/null 2>&1; then
    run "ufw allow in on tailscale0"
  fi

  if [ -n "$lan" ]; then
    run "ufw allow from '$lan' to any port 22 proto tcp comment 'SSH LAN'"
    # Paneles de warden accesibles solo desde la LAN.
    local p
    for p in 9090 7575 9898 8080 7890; do
      run "ufw allow from '$lan' to any port $p proto tcp comment 'warden panel'"
    done
    if command -v smbd >/dev/null 2>&1; then
      run "ufw allow from '$lan' to any port 139,445 proto tcp comment 'Samba LAN'"
    fi
  else
    warn "WARDEN_LAN no definido en site.conf — permito SSH desde cualquier lado para no dejarte afuera."
    run "ufw allow 22/tcp"
  fi

  run "ufw --force enable"
  ok "Firewall activo (equilibrado). Las BD se cierran en el compose, no en ufw."
}
