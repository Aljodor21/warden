#!/usr/bin/env bash
# modules/cloudflare_init.sh — crea un túnel de Cloudflare DESDE CERO.
#   warden_publish (modules/cloudflare.sh) asume que el túnel YA existe;
#   esto es lo que falta antes: instalar cloudflared, loguearte, crear el
#   túnel, y dejar la config inicial lista para que 'warden publish' la
#   vaya llenando con las apps del catálogo.

CF_CONFIG="${CF_CONFIG:-/etc/cloudflared/config.yml}"

_cloudflared_install() {
  has cloudflared && return 0
  log "Instalando cloudflared"
  case "${DISTRO:-unknown}" in
    debian)
      run "curl -fsSL https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-amd64.deb -o /tmp/cloudflared.deb"
      run "dpkg -i /tmp/cloudflared.deb"
      run "rm -f /tmp/cloudflared.deb"
      ;;
    arch)
      run "pacman -S --needed --noconfirm cloudflared"
      ;;
    *) die "Distro no soportada para instalar cloudflared" ;;
  esac
}

warden_cloudflare_init() {
  _cloudflared_install
  run "mkdir -p /etc/cloudflared"

  if [ ! -f /etc/cloudflared/cert.pem ] && [ ! -f "$HOME/.cloudflared/cert.pem" ]; then
    warn "Te va a pedir abrir una URL para autorizar este server con tu cuenta de Cloudflare."
    warn "Elegí el dominio bajo el que va a vivir este túnel cuando te lo pregunte."
    cloudflared tunnel login
  else
    ok "Ya hay una sesión de Cloudflare autorizada en este server"
  fi

  local name
  name="$(ui_input 'Nombre para este túnel (identifica este server en Cloudflare)' "$(hostname)")"

  if cloudflared tunnel list 2>/dev/null | awk '{print $2}' | grep -qx "$name"; then
    ok "Ya existe un túnel llamado '$name', lo reuso"
  else
    log "Creando túnel '$name'"
    cloudflared tunnel create "$name" || die "No pude crear el túnel"
  fi

  local tid credfile
  tid="$(cloudflared tunnel list 2>/dev/null | awk -v n="$name" '$2==n{print $1; exit}')"
  [ -n "$tid" ] || die "Creé el túnel pero no pude leer su ID"
  credfile="$(find "$HOME/.cloudflared" /etc/cloudflared -maxdepth 1 -name "${tid}.json" 2>/dev/null | head -1)"
  [ -n "$credfile" ] || die "No encuentro el archivo de credenciales del túnel ($tid.json)"

  if [ "$credfile" != "/etc/cloudflared/${tid}.json" ]; then
    run "cp '$credfile' '/etc/cloudflared/${tid}.json'"
  fi

  if [ -f "$CF_CONFIG" ]; then
    ok "Ya existe $CF_CONFIG, no lo piso (corré 'warden publish' para agregar apps)"
  else
    log "Dejando la config inicial en $CF_CONFIG"
    {
      echo "tunnel: $tid"
      echo "credentials-file: /etc/cloudflared/${tid}.json"
      echo "ingress:"
      echo "  - service: http_status:404"
    } > "$CF_CONFIG"
  fi

  run "cloudflared service install || true"
  run "systemctl enable --now cloudflared"
  ok "Túnel '$name' ($tid) listo. Ahora: agregá COMP_CF_HOST a tu app en el catálogo y corré 'warden publish'."
}
