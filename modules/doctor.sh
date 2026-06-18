#!/usr/bin/env bash
# modules/doctor.sh — chequeo de salud de warden (solo lectura).

warden_doctor() {
  local mount repo passfile c
  mount="${WARDEN_BACKUP_MOUNT:-/mnt/warden-backup}"
  repo="$mount/restic-repo"
  passfile="${RESTIC_PASS_FILE:-/root/.warden-restic-password}"

  echo "== warden doctor =="

  _ck() {  # <texto> <comando...>
    local txt="$1"; shift
    if "$@" >/dev/null 2>&1; then printf ' \033[32m✓\033[0m %s\n' "$txt"
    else printf ' \033[31m✗\033[0m %s\n' "$txt"; fi
  }

  _ck "Distro soportada ($DISTRO)"    test "$DISTRO" != unknown
  _ck "Docker operativo"              docker info
  _ck "Disco de backup montado"       mountpoint -q "$mount"
  _ck "Repositorio restic presente"   test -d "$repo"
  _ck "Contraseña restic (¡escrow!)"  test -f "$passfile"
  _ck "Timer de backup activo"        systemctl is-active --quiet warden-backup.timer
  _ck "Timer de verify activo"        systemctl is-active --quiet warden-verify.timer
  _ck "Firewall ufw activo"           bash -c "ufw status 2>/dev/null | grep -q 'Status: active'"

  echo "Contenedores del dashboard:"
  for c in homepage backrest ntfy; do
    if docker ps --format '{{.Names}}' 2>/dev/null | grep -qx "$c"; then
      printf '   \033[32m✓\033[0m %s\n' "$c"
    else
      printf '   \033[2m–\033[0m %s (no instalado)\n' "$c"
    fi
  done

  if [ -f "$passfile" ] && [ -d "$repo" ]; then
    echo "Último snapshot:"
    docker run --rm -e RESTIC_PASSWORD_FILE=/pass -v "$passfile:/pass:ro" -v "$repo:/repo" \
      restic/restic -r /repo snapshots --latest 1 2>/dev/null | tail -n 3 || echo "   (no disponible)"
  fi
}
