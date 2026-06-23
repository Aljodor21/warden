#!/usr/bin/env bash
# modules/panel.sh — instala warden-panel: un panel web mínimo (Go, binario
# único, cero dependencias) para gestionar site/catalog sin consola.
#   - Compila en el server (instala el toolchain de Go solo para esto).
#   - Corre como servicio systemd, escucha en :7890.
#   - Protegido con clave (hash sha256), accesible solo desde la LAN
#     (regla ya agregada a modules/firewall.sh).

PANEL_BIN="/usr/local/bin/warden-panel"
PANEL_PASSFILE="/etc/warden/panel.passwd"
PANEL_SERVICE="/etc/systemd/system/warden-panel.service"

_panel_ensure_go() {
  has go && return 0
  log "Instalando el compilador de Go (solo se usa para compilar el panel)"
  case "${DISTRO:-unknown}" in
    debian) run "apt-get install -y golang-go" ;;
    arch)   run "pacman -S --needed --noconfirm go" ;;
    *) die "Distro no soportada para instalar Go" ;;
  esac
}

warden_panel_install() {
  _panel_ensure_go

  log "Compilando warden-panel"
  run "bash -c 'cd \"$WARDEN_ROOT/panel\" && go build -o \"$PANEL_BIN\" .'"

  run "mkdir -p /etc/warden"
  if [ -f "$PANEL_PASSFILE" ]; then
    ok "Ya hay una clave configurada para el panel (no la piso)"
  else
    local pass pass2
    pass="$(ui_input 'Elegí una clave para el panel web' '')"
    [ -n "$pass" ] || die "La clave no puede estar vacía"
    if [ "${WARDEN_DRY_RUN:-0}" = 1 ]; then
      echo "   [dry-run] guardaría el hash de la clave en $PANEL_PASSFILE"
    else
      printf '%s' "$pass" | sha256sum | awk '{print $1}' > "$PANEL_PASSFILE"
      chmod 600 "$PANEL_PASSFILE"
    fi
    ok "Clave del panel guardada (hasheada, no en texto plano)"
  fi

  log "Dejando warden-panel como servicio (systemd)"
  {
    echo "[Unit]"
    echo "Description=warden-panel (panel web del catálogo)"
    echo "After=network.target"
    echo
    echo "[Service]"
    echo "ExecStart=$PANEL_BIN -addr 0.0.0.0:7890 -catalog $WARDEN_ROOT/site/catalog -warden /usr/local/bin/warden -passfile $PANEL_PASSFILE"
    echo "Restart=on-failure"
    echo "User=root"
    echo
    echo "[Install]"
    echo "WantedBy=multi-user.target"
  } > /tmp/warden-panel.service
  run "install -m 644 /tmp/warden-panel.service '$PANEL_SERVICE'"
  run "rm -f /tmp/warden-panel.service"
  run "systemctl daemon-reload"
  run "systemctl enable --now warden-panel"

  ok "Panel listo. Desde tu LAN/Tailscale: http://$(hostname).local:7890 (la regla de ufw ya lo restringe a tu LAN)"
}
