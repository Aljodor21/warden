#!/usr/bin/env bash
# modules/register.sh — fija el disco de backup (fstab por UUID) y activa timers.

# Copia y enciende los timers de systemd (backup horario + verify nocturno).
warden_timer_install() {
  local src="$WARDEN_ROOT/backup/systemd" u
  for u in warden-backup.service warden-backup.timer warden-verify.service warden-verify.timer; do
    run "install -m 644 '$src/$u' '/etc/systemd/system/$u'"
  done
  run "systemctl daemon-reload"
  run "systemctl enable --now warden-backup.timer warden-verify.timer"
  ok "Timers activos: backup cada hora + verify nocturno"
}

# Si no hay disco de backup listo, lo detecta entre los discos disponibles,
# te deja elegir uno y lo prepara (formatea+monta+marca) sin pasos manuales.
_register_autoprep() {
  local mp="$1"
  local sysd; sysd="$(system_disk)"
  local opts=() name size model
  while read -r name size model; do
    [ "${name##*/}" = "$sysd" ] && continue
    opts+=("${name} (${size} ${model:-?})")
  done < <(lsblk -dpno NAME,SIZE,MODEL | awk '$1!=""')

  [ "${#opts[@]}" -gt 0 ] || die "No veo ningún disco disponible además del del sistema. Conectá uno y reintentá."

  log "No hay disco de backup preparado todavía. Discos disponibles:"
  local chosen; chosen="$(ui_menu "¿Cuál disco querés usar para el backup?" "${opts[@]}")"
  [ -n "$chosen" ] || die "Cancelado."
  local dev="${chosen%% *}"

  warden_init_disk "$dev"
}

# Fija el disco de backup en fstab (montaje por UUID) y activa la automatización.
warden_register() {
  local mp="${WARDEN_BACKUP_MOUNT:-/mnt/warden-backup}" marker=".warden-backup.id" src uuid
  src="$(findmnt -no SOURCE --target "$mp" 2>/dev/null)"
  if ! { [ -n "$src" ] && [ -f "$mp/$marker" ]; }; then
    _register_autoprep "$mp"
    src="$(findmnt -no SOURCE --target "$mp" 2>/dev/null)"
  fi
  { [ -n "$src" ] && [ -f "$mp/$marker" ]; } || die "El disco no quedó listo en $mp."
  uuid="$(blkid -s UUID -o value "$src")"
  [ -n "$uuid" ] || die "No pude leer el UUID de $src"

  if grep -q "$uuid" /etc/fstab 2>/dev/null; then
    ok "El disco ya está en /etc/fstab"
  else
    log "Agregando $mp a /etc/fstab (UUID=$uuid, con nofail)"
    backup_file /etc/fstab
    run "printf 'UUID=%s %s ext4 defaults,nofail 0 2\n' '$uuid' '$mp' >> /etc/fstab"
  fi

  run "mkdir -p /etc/warden"
  run "printf 'BACKUP_UUID=%s\n' '$uuid' > /etc/warden/config"

  warden_timer_install
  ok "Disco fijado y backup automático activo."
}
