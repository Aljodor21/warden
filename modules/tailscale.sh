#!/usr/bin/env bash
# modules/tailscale.sh — VPN Tailscale (acceso remoto seguro, incluso tras CGNAT).

TAILSCALE_SUBNET_FILE="/etc/warden/tailscale-subnet"

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

# Detecta la subred local del servidor ignorando loopback, docker y tailscale.
_tailscale_detect_subnet() {
  ip route show scope link 2>/dev/null \
    | grep -Ev 'lo |docker|br-|veth|tailscale|169\.' \
    | awk '{print $1}' | head -1
}

# Habilita IP forwarding + exit node + subnet router.
# Uso: warden_tailscale_subnet [CIDR]   (si no se pasa CIDR, lo detecta y pregunta)
warden_tailscale_subnet() {
  local subnet="${1:-}"

  if ! tailscale ip -4 >/dev/null 2>&1; then
    die "Tailscale no está conectado. Corré 'warden vpn' primero."
  fi

  if [ -z "$subnet" ]; then
    local detected
    detected="$(_tailscale_detect_subnet)"
    subnet="$(ui_input "Subred local a exponer (CIDR)" "${detected:-192.168.1.0/24}")"
  fi

  [ -n "$subnet" ] || die "La subred no puede estar vacía."

  log "Habilitando IP forwarding"
  run "mkdir -p /etc/sysctl.d"
  printf 'net.ipv4.ip_forward = 1\nnet.ipv6.conf.all.forwarding = 1\n' \
    > /etc/sysctl.d/99-tailscale.conf
  run "sysctl -p /etc/sysctl.d/99-tailscale.conf"

  log "Activando exit node y subnet router ($subnet)"
  run "tailscale up --advertise-exit-node --advertise-routes=$subnet"

  run "mkdir -p /etc/warden"
  echo "$subnet" > "$TAILSCALE_SUBNET_FILE"

  ok "Exit node y subnet router activados para $subnet"
  warn "Aprobá las rutas en https://login.tailscale.com/admin/machines para que funcionen"
}
