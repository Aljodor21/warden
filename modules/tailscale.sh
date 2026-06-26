#!/usr/bin/env bash
# modules/tailscale.sh — VPN Tailscale (acceso remoto seguro, incluso tras CGNAT).

TAILSCALE_SUBNET_FILE="/etc/warden/tailscale-subnet"
TAILSCALE_EXITNODE_FILE="/etc/warden/tailscale-exitnode"

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

# Asegura que IP forwarding esté habilitado (requisito para exit node y subnet).
_tailscale_enable_forwarding() {
  run "mkdir -p /etc/sysctl.d"
  printf 'net.ipv4.ip_forward = 1\nnet.ipv6.conf.all.forwarding = 1\n' \
    > /etc/sysctl.d/99-tailscale.conf
  run "sysctl -p /etc/sysctl.d/99-tailscale.conf"
}

# Aplica el estado actual de ambos toggles con un solo tailscale up.
_tailscale_apply() {
  local args=""
  [ -f "$TAILSCALE_EXITNODE_FILE" ] && args="$args --advertise-exit-node"
  [ -f "$TAILSCALE_SUBNET_FILE" ]   && args="$args --advertise-routes=$(cat "$TAILSCALE_SUBNET_FILE")"
  run "tailscale up$args"
}

# Activa o desactiva el exit node.
# Uso: warden_tailscale_exitnode on|off
warden_tailscale_exitnode() {
  local action="${1:-}"
  if ! tailscale ip -4 >/dev/null 2>&1; then
    die "Tailscale no está conectado. Corré 'warden vpn' primero."
  fi
  [ -n "$action" ] || action="$(ui_menu "Exit Node" "on" "off")"
  run "mkdir -p /etc/warden"
  case "$action" in
    on)
      _tailscale_enable_forwarding
      touch "$TAILSCALE_EXITNODE_FILE"
      _tailscale_apply
      ok "Exit node activado"
      warn "Aprobalo en https://login.tailscale.com/admin/machines si es la primera vez"
      ;;
    off)
      rm -f "$TAILSCALE_EXITNODE_FILE"
      _tailscale_apply
      ok "Exit node desactivado"
      ;;
    *) die "Uso: warden vpn exit-node on|off" ;;
  esac
}

# Activa, cambia o desactiva el subnet router.
# Uso: warden_tailscale_subnet on [CIDR] | off
warden_tailscale_subnet() {
  local action="${1:-}"
  if ! tailscale ip -4 >/dev/null 2>&1; then
    die "Tailscale no está conectado. Corré 'warden vpn' primero."
  fi
  [ -n "$action" ] || action="$(ui_menu "Subnet Router" "on" "off")"
  run "mkdir -p /etc/warden"
  case "$action" in
    on)
      local subnet="${2:-}"
      if [ -z "$subnet" ]; then
        local detected
        detected="$(_tailscale_detect_subnet)"
        subnet="$(ui_input "Subred local a exponer (CIDR)" "${detected:-192.168.1.0/24}")"
      fi
      [ -n "$subnet" ] || die "La subred no puede estar vacía."
      _tailscale_enable_forwarding
      echo "$subnet" > "$TAILSCALE_SUBNET_FILE"
      _tailscale_apply
      ok "Subnet router activado para $subnet"
      warn "Aprobá las rutas en https://login.tailscale.com/admin/machines si es la primera vez"
      ;;
    off)
      rm -f "$TAILSCALE_SUBNET_FILE"
      _tailscale_apply
      ok "Subnet router desactivado"
      ;;
    *) die "Uso: warden vpn subnet on [CIDR] | off" ;;
  esac
}
