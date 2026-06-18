#!/usr/bin/env bash
# modules/motd.sh — saludo (MOTD) al iniciar sesión, con estado del server.

warden_motd_install() {
  local dest=/etc/profile.d/99-warden-motd.sh
  [ "${WARDEN_DRY_RUN:-0}" = 1 ] && { echo "[dry-run] instalaría el MOTD en $dest"; return 0; }
  cat > "$dest" <<'EOF'
#!/usr/bin/env bash
# MOTD de warden (saludo al iniciar sesión). Desactivar: export WARDEN_NO_MOTD=1
[ -n "${WARDEN_NO_MOTD:-}" ] && return 0 2>/dev/null
_c() { printf '\033[%sm' "$1"; }
RST=$(_c 0); B=$(_c '1;34'); G=$(_c '1;32'); D=$(_c 2)
host=$(hostname)
disk=$(df -h / 2>/dev/null | awk 'NR==2{print $5" de "$2}')
mem=$(free -h 2>/dev/null | awk '/^Mem:/{print $3"/"$2}')
load=$(cut -d' ' -f1-3 /proc/loadavg 2>/dev/null)
docks=$(docker ps -q 2>/dev/null | wc -l)
if systemctl is-active --quiet warden-backup.timer 2>/dev/null; then
  bk="${G}activo${RST}"
else
  bk="${D}inactivo${RST}"
fi
echo
echo "  ${B}warden${RST} · ${host}"
echo "  ${D}──────────────────────────────${RST}"
printf "  Disco /: %s    RAM: %s\n" "$disk" "$mem"
printf "  Carga: %s\n" "$load"
printf "  Docker: %s contenedores    Backup auto: %s\n" "$docks" "$bk"
echo
EOF
  chmod +x "$dest"
  ok "MOTD instalado (lo verás al iniciar sesión)"
}
