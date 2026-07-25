#!/usr/bin/env bash
# modules/disk.sh — gestión interactiva de discos no-sistema:
#   montar, desmontar, preparar (init + reformatear unificados), silenciar.

# ¿el disco es de platos (HDD) y no SSD/NVMe?
_disk_is_rotational() {
  [ "$(lsblk -dno ROTA "$1" 2>/dev/null | tr -d ' ')" = "1" ]
}

# Ruta estable del disco: /dev/sdX cambia de orden entre arranques, by-id no.
# Sin esto, hdparm.conf podría terminar aplicándole la config a otro disco.
_disk_by_id() {
  local dev="$1" link
  link="$(find /dev/disk/by-id -lname "*/${dev##*/}" ! -name '*-part*' 2>/dev/null | head -1)"
  echo "${link:-$dev}"
}

# Aplica y persiste el ahorro de energía de un HDD de backup.
# Los discos de 2.5" aparcan los cabezales cada pocos minutos: hacen "click"
# y queman ciclos de carga (SMART 193 Load_Cycle_Count, ~600k de vida útil).
# En un disco que solo recibe backups eso es puro desgaste y ruido de noche.
_disk_quiet_apply() {
  local dev="$1"
  local conf="/etc/hdparm.conf" byid; byid="$(_disk_by_id "$dev")"

  # apm=254 -> sin parking agresivo · spindown=24 -> duerme tras 2 min sin uso
  run "hdparm -B 254 -S 24 '$dev'"

  if grep -q "^$byid {" "$conf" 2>/dev/null; then
    ok "Ya estaba configurado en $conf, no lo duplico."
  else
    log "Persistiendo la configuración en $conf (la aplica udev al arrancar)"
    run "bash -c \"printf '\n# warden: HDD de backup — sin parking de cabezales, duerme a los 2 min\n%s {\n    apm = 254\n    spindown_time = 24\n}\n' '$byid' >> '$conf'\""
  fi
}

# Silenciado automático: se llama solo al montar/preparar un disco de backup.
# No instala nada ni interrumpe el flujo — si falta hdparm, avisa y sigue.
_disk_quiet_auto() {
  local dev="$1"
  _disk_is_rotational "$dev" || return 0
  if ! is_installed hdparm; then
    warn "Es un disco mecánico y hdparm no está instalado — va a hacer ruido."
    warn "Silencialo con: warden disk quiet"
    return 0
  fi
  log "Disco mecánico: lo configuro para que no aparque cabezales y duerma solo"
  _disk_quiet_apply "$dev"
}

# warden disk quiet [dev] — silenciar un HDD a mano (o volver a aplicarlo).
warden_disk_quiet() {
  local dev="${1:-}"
  [ -n "$dev" ] || dev="$(_disk_pick 'Elegí el disco a silenciar')" || return 1

  local sysd; sysd="$(system_disk)"
  [ "${dev##*/}" != "$sysd" ] || { warn "$dev es el disco del sistema — no lo toco."; return 1; }

  if ! _disk_is_rotational "$dev"; then
    ok "$dev es SSD/NVMe: no aparca cabezales ni hace ruido. Nada que hacer."
    return 0
  fi

  ensure_pkg hdparm hdparm
  _disk_quiet_apply "$dev"
  ok "Listo. Comprobalo con: hdparm -C $dev  (debe decir 'standby' cuando nadie lo use)"
}

# Lista los discos no-sistema con su rol. Imprime "nombre|tamaño|rol|detalle".
_disk_list_non_system() {
  local name size type sysd cls role detail
  sysd="$(system_disk)"
  while read -r name size type _rest; do
    [ "$type" = "disk" ] || continue
    [ "${name##*/}" = "$sysd" ] && continue
    cls="$(classify_disk "$name")"
    role="${cls%%|*}"; detail="${cls#*|}"
    echo "${name}|${size}|${role}|${detail}"
  done < <(lsblk -dpno NAME,SIZE,TYPE,TRAN,MODEL)
}

# Picker interactivo de discos no-sistema. Imprime el /dev/sdX elegido.
# $1 = mensaje de cabecera
_disk_pick() {
  local header="${1:-Elegí un disco}"
  local entries=() devs=()
  local name size role detail label
  while IFS='|' read -r name size role detail; do
    label="${name##*/}  ${size}  [${role}]  ${detail}"
    entries+=("$label")
    devs+=("$name")
  done < <(_disk_list_non_system)

  [ "${#devs[@]}" -gt 0 ] || { warn "No hay discos no-sistema detectados."; return 1; }

  local chosen
  chosen="$(ui_menu "$header" "${entries[@]}")" || return 1
  [ -n "$chosen" ] || return 1

  # recuperar el /dev correspondiente a la línea elegida
  local i=0
  for entry in "${entries[@]}"; do
    [ "$entry" = "$chosen" ] && { echo "${devs[$i]}"; return 0; }
    i=$((i+1))
  done
  return 1
}

# warden disk mount — monta un disco en /mnt/warden-backup
warden_disk_mount() {
  local mount="${WARDEN_BACKUP_MOUNT:-/mnt/warden-backup}"

  if [ -d "$mount" ] && mountpoint -q "$mount"; then
    warn "Ya hay un disco montado en $mount. Desmontalo primero (warden disk unmount)."
    return 1
  fi

  local dev
  dev="$(_disk_pick 'Elegí el disco a montar')" || return 1

  # detectar partición
  local part
  part="${dev}p1"; [ -b "$part" ] || part="${dev}1"
  [ -b "$part" ] || { warn "No encontré partición en $dev. ¿Está particionado? Usá 'Preparar disco' primero."; return 1; }

  run "mkdir -p '$mount'"
  run "mount '$part' '$mount'"

  if [ -f "$mount/$MARKER" ]; then
    ok "Disco de backup warden montado en $mount."
  else
    warn "Disco montado en $mount pero no tiene el marcador warden (no fue preparado con warden)."
  fi

  _disk_quiet_auto "$dev"
}

# warden disk unmount — desmonta el disco de backup actual
warden_disk_unmount() {
  local mount="${WARDEN_BACKUP_MOUNT:-/mnt/warden-backup}"

  if ! mountpoint -q "$mount" 2>/dev/null; then
    warn "No hay ningún disco montado en $mount."
    return 1
  fi

  run "umount '$mount'"
  ok "Disco desmontado de $mount."
}

# warden disk prepare — unifica init-disk + reformatear en uno solo
warden_disk_prepare() {
  local mount="${WARDEN_BACKUP_MOUNT:-/mnt/warden-backup}"
  local passfile="${RESTIC_PASS_FILE:-/root/.warden-restic-password}"

  local dev
  dev="$(_disk_pick 'Elegí el disco a preparar como backup')" || return 1

  local sysd; sysd="$(system_disk)"
  [ "${dev##*/}" != "$sysd" ] || { die "$dev es el disco del sistema — no lo toco."; return 1; }

  # ¿tiene datos?
  if disk_has_fs "$dev"; then
    echo
    warn "El disco $dev YA TIENE DATOS (no es un disco vacío)."
    echo "Formatearlo va a BORRAR TODO lo que haya en él de forma IRREVERSIBLE."
    echo
    read -rp "Escribí BORRAR para continuar: " ok
    [ "$ok" = "BORRAR" ] || { echo "Cancelado."; return 1; }
  else
    ui_confirm "El disco $dev está vacío. ¿Lo inicializamos como disco de backup?" || return 1
  fi

  # desmontar si estuviera montado
  if mountpoint -q "$mount" 2>/dev/null; then
    log "Desmontando $mount antes de formatear"
    run "umount '$mount'"
  fi

  log "Particionando y formateando $dev"
  run "parted -s '$dev' mklabel gpt mkpart primary ext4 0% 100%"
  local part
  part="${dev}p1"; [ -e "$part" ] || part="${dev}1"
  run "mkfs.ext4 -F -q '$part'"

  run "mkdir -p '$mount'"
  run "mount '$part' '$mount'"

  log "Marcando el disco como destino de backup warden"
  run "bash -c 'uuidgen 2>/dev/null || cat /proc/sys/kernel/random/uuid' > '$mount/$MARKER'"
  run "mkdir -p '$mount/restic-repo'"

  if [ -f "$passfile" ]; then
    ok "Ya existe clave restic en $passfile, la reuso."
  else
    log "Generando clave restic en $passfile"
    run "bash -c \"openssl rand -base64 32 > '$passfile'\""
    run "chmod 600 '$passfile'"
    warn "Guardá esta clave fuera del server (warden secrets save) — sin ella no podés restaurar."
  fi

  _disk_quiet_auto "$dev"

  ok "Disco preparado y montado en $mount."
  echo
  log "Siguiente paso: 'warden register' para activar el backup automático."
}

# warden disk explore — muestra snapshots y uso del disco de backup montado
warden_disk_explore() {
  local mount="${WARDEN_BACKUP_MOUNT:-/mnt/warden-backup}"
  local passfile="${RESTIC_PASS_FILE:-/root/.warden-restic-password}"

  if ! mountpoint -q "$mount" 2>/dev/null; then
    warn "No hay disco de backup montado en $mount. Montalo primero."
    return 1
  fi

  echo "${C_B}Uso del disco:${C_RESET}"
  df -h "$mount" | tail -1 | awk '{printf "  Usado: %s / %s (%s libre)\n", $3, $2, $4}'
  echo

  local repo="$mount/restic-repo"
  if [ ! -d "$repo" ]; then
    warn "No hay repositorio restic en $mount. El disco fue preparado pero nunca se hizo backup."
    return 0
  fi

  echo "${C_B}Snapshots disponibles:${C_RESET}"
  docker run --rm \
    -v "$repo:/repo:ro" \
    -v "$passfile:/passfile:ro" \
    restic/restic -r /repo --password-file /passfile snapshots --compact 2>/dev/null \
    || warn "No se pudieron listar los snapshots (¿primer uso?)."
}

# Menú principal de gestión de discos
warden_disk_menu() {
  local opt
  opt="$(ui_menu 'Gestionar disco' \
    'Listar discos' \
    'Montar disco' \
    'Desmontar disco de backup' \
    'Preparar disco (nuevo o reformatear)' \
    'Explorar backup' \
    'Silenciar disco (HDD)' \
    'Volver')"
  case "$opt" in
    'Listar discos')
      echo
      printf "  %-8s %-8s %-8s %s\n" "DISCO" "TAM" "ROL" "DETALLE"
      while IFS='|' read -r name size role detail; do
        printf "  %-8s %-8s %-8s %s\n" "${name##*/}" "$size" "$role" "$detail"
      done < <(_disk_list_non_system)
      # también el de sistema
      local sysd syssize
      sysd="$(system_disk)"
      syssize="$(lsblk -dnpo SIZE "/dev/$sysd" 2>/dev/null || echo '?')"
      printf "  %-8s %-8s %-8s %s\n" "$sysd" "$syssize" "SYSTEM" "disco del sistema (/)"
      echo
      ;;
    'Montar disco')                        need_root; warden_disk_mount ;;
    'Desmontar disco de backup')           need_root; warden_disk_unmount ;;
    'Preparar disco (nuevo o reformatear)') need_root; warden_disk_prepare ;;
    'Explorar backup')                     need_root; warden_disk_explore ;;
    'Silenciar disco (HDD)')               need_root; warden_disk_quiet ;;
    *) : ;;
  esac
}
