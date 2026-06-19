#!/usr/bin/env bash
# modules/tailscale.sh — VPN Tailscale (acceso remoto seguro, incluso tras CGNAT).

warden_tailscale_install() {
  if has tailscale; then
    ok "Tailscale ya instalado"
  else
    log "Instalando Tailscale"
    run "curl -fsSL https://tailscale.com/install.sh | sh"
  fi
  run "systemctl enable --now tailscaled"

  if tailscale ip -4 >/dev/null 2>&1; then
    ok "Tailscale ya está conectado ($(tailscale ip -4 2>/dev/null | head -1))"
  else
    warn "Iniciá sesión en Tailscale: se abrirá una URL para autorizar el equipo."
    run "tailscale up"
  fi
}
