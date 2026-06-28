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
    pass="$(ui_input 'Elegí una clave para el panel web' "${WARDEN_CI:+warden-ci-test}")"
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
    echo "ExecStart=$PANEL_BIN -addr 0.0.0.0:80 -root $WARDEN_ROOT -warden /usr/local/bin/warden -passfile $PANEL_PASSFILE"
    echo "Restart=on-failure"
    echo "User=root"
    echo
    echo "[Install]"
    echo "WantedBy=multi-user.target"
  } > /tmp/warden-panel.service
  run "install -m 644 /tmp/warden-panel.service '$PANEL_SERVICE'"
  run "rm -f /tmp/warden-panel.service"
  run "systemctl daemon-reload"
  run "systemctl enable warden-panel"
  # 'restart' (no 'start') — si ya estaba corriendo, esto FUERZA a que cargue
  # el binario recién compilado. 'enable --now'/'start' es no-op si ya corría.
  run "systemctl restart warden-panel"

  # No depender de que se reaplique TODO modules/firewall.sh — si ufw está
  # activo, asegurar la regla de este puerto puntual, ahora mismo.
  if has ufw && ufw status 2>/dev/null | grep -q "Status: active"; then
    if [ -n "${WARDEN_LAN:-}" ]; then
      run "ufw allow from '$WARDEN_LAN' to any port 80 proto tcp comment 'warden panel'"
    else
      warn "ufw está activo pero no hay WARDEN_LAN definido — el panel puede no ser alcanzable. Agregá la regla a mano: ufw allow <TU_LAN> to any port 80 proto tcp"
    fi
  fi

  ok "Panel listo. Desde tu LAN/Tailscale: http://$(hostname)"
}
