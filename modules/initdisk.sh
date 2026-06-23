#!/usr/bin/env bash
# modules/initdisk.sh — prepara un disco NUEVO (vacío) como destino de backup:
# lo formatea, lo monta, le pone el marcador y arma lo que warden_backup necesita
# (carpeta del repo restic + contraseña). Después de esto: 'warden register'.
#
#   warden init-disk /dev/vdb

_initdisk_part() {  # <dispositivo> -> imprime la ruta de su 1ra partición
  local dev="$1" part
  part="${dev}p1"; [ -e "$part" ] || part="${dev}1"
  echo "$part"
}

warden_init_disk() {
  local dev="${1:-}"
  local mount="${WARDEN_BACKUP_MOUNT:-/mnt/warden-backup}"
  local marker=".warden-backup.id"
  local passfile="${RESTIC_PASS_FILE:-/root/.warden-restic-password}"

  [ -n "$dev" ] || die "Uso: warden init-disk /dev/vdb"
  [ -b "$dev" ] || die "$dev no es un disco válido (mirá 'lsblk')"

  local sysd; sysd="$(lsblk -no PKNAME "$(findmnt -no SOURCE /)" 2>/dev/null | head -n1)"
  [ "${dev##*/}" != "$sysd" ] || die "$dev es el disco del sistema — no lo toco."

  echo "Esto va a BORRAR todo en $dev, formatearlo ext4 y montarlo en $mount."
  if [ "${WARDEN_DRY_RUN:-0}" != 1 ]; then
    read -rp "Escribí BORRAR para continuar: " ok
    [ "$ok" = "BORRAR" ] || { echo "Cancelado."; return 1; }
  fi

  log "Particionando y formateando $dev"
  run "parted -s '$dev' mklabel gpt mkpart primary ext4 0% 100%"
  local part; part="$(_initdisk_part "$dev")"
  run "mkfs.ext4 -F -q '$part'"

  run "mkdir -p '$mount'"
  run "mount '$part' '$mount'"

  log "Marcando el disco como destino de backup warden"
  run "bash -c 'uuidgen 2>/dev/null || cat /proc/sys/kernel/random/uuid' > '$mount/$marker'"
  run "mkdir -p '$mount/restic-repo'"

  if [ -f "$passfile" ]; then
    ok "Ya existe una clave restic en $passfile, la reuso"
  else
    log "Generando clave restic en $passfile"
    run "bash -c \"openssl rand -base64 32 > '$passfile'\""
    run "chmod 600 '$passfile'"
    warn "Guardá esa clave fuera de este server (warden secrets save) — sin ella no se puede restaurar."
  fi

  ok "Disco listo en $mount. Seguí con: warden register"
}
