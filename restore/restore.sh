#!/usr/bin/env bash
#
# restore.sh — restaura un backup de warden desde un disco.
#   1) Detecta el disco de backup (cualquiera con el marcador, no el del SO).
#   2) Lista los snapshots por tag (immich, nas, db…).
#   3) Restaura lo que elijas: archivos a una carpeta de staging, y las BD
#      cargándolas en su contenedor (orden seguro).
#   restic corre en Docker (cero instalación).
#
#   Nota: los snapshots actuales guardan rutas con prefijo /data y /dumps
#   (por cómo se montó en el backup), así que los archivos se restauran bajo
#   $STAGE y de ahí se mueven a su ubicación final.
#
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
WARDEN_ROOT="$(cd "$HERE/.." && pwd)"; export WARDEN_ROOT
# shellcheck source=/dev/null
source "$WARDEN_ROOT/lib/core.sh"
# shellcheck source=/dev/null
source "$WARDEN_ROOT/lib/distro.sh"
# shellcheck source=/dev/null
source "$WARDEN_ROOT/lib/ui.sh"
# shellcheck source=/dev/null
source "$WARDEN_ROOT/lib/catalog.sh"

need_root
has docker || die "Falta docker"

MARKER=".warden-backup.id"
MOUNT="${WARDEN_BACKUP_MOUNT:-/mnt/warden-backup}"
RESTIC_PASS_FILE="${RESTIC_PASS_FILE:-/root/.warden-restic-password}"
STAGE="${WARDEN_RESTORE_DIR:-/var/tmp/warden-restore}"

# restic dentro de Docker (repo del disco + staging de salida).
restic() {
  docker run --rm \
    -e RESTIC_PASSWORD_FILE=/pass \
    -v "$RESTIC_PASS_FILE":/pass:ro \
    -v "$REPO":/repo \
    -v "$STAGE":/restore \
    restic/restic -r /repo "$@"
}

# Carga los dumps restaurados ($STAGE/dumps/*.sql) en su contenedor.
warden_restore_dbs() {
  local sql db found t pass
  for sql in "$STAGE"/dumps/*.sql; do
    [ -e "$sql" ] || continue
    db="$(basename "$sql" .sql)"
    found=""
    while IFS='|' read -r t _ _ _; do
      ( catalog_load "$t" >/dev/null 2>&1 && [ "${COMP_DB_NAME:-}" = "$db" ] ) && { found="$t"; break; }
    done < <(catalog_each)
    if [ -z "$found" ]; then
      warn "No sé a qué contenedor va $db.sql; lo dejo en $STAGE/dumps"; continue
    fi
    catalog_load "$found"
    if ! docker ps --format '{{.Names}}' | grep -qx "${COMP_DB_CONTAINER:-}"; then
      warn "$COMP_DB_CONTAINER no está corriendo; instalá $COMP_NAME primero. Salto $db."; continue
    fi
    ui_confirm "Cargar dump en '$db' ($COMP_DB_CONTAINER)? Detené antes la app que lo usa." || continue
    pass="$(docker exec "$COMP_DB_CONTAINER" printenv POSTGRES_PASSWORD 2>/dev/null || true)"
    run "docker exec -i -e PGPASSWORD='$pass' '$COMP_DB_CONTAINER' psql -U '$COMP_DB_USER' -d '$db' < '$sql'"
    ok "BD '$db' cargada"
  done
}

# --- 1. Encontrar el disco de backup ---
sysdisk="/dev/$(system_disk)"
bmount=""
while read -r mp; do
  [ -n "$mp" ] && [ -f "$mp/$MARKER" ] && { bmount="$mp"; break; }
done < <(lsblk -rno MOUNTPOINT)

if [ -z "$bmount" ]; then
  log "No hay disco de backup montado. Discos disponibles (el del sistema es $sysdisk):"
  lsblk -pno NAME,SIZE,FSTYPE,LABEL,MOUNTPOINT | grep -v "^$sysdisk"
  dev="$(ui_input 'Partición del disco de backup (ej: /dev/sdb1)' '')"
  [ -b "$dev" ] || die "No es una partición válida: $dev"
  run "mkdir -p '$MOUNT'"; run "mount '$dev' '$MOUNT'"
  [ -f "$MOUNT/$MARKER" ] || die "Ese disco no tiene un backup de warden."
  bmount="$MOUNT"
fi
REPO="$bmount/restic-repo"
[ -d "$REPO" ] || die "No encuentro el repositorio restic en $REPO"
ok "Backup encontrado en $bmount"

# --- 2. Contraseña del repositorio ---
if [ ! -f "$RESTIC_PASS_FILE" ]; then
  warn "No está $RESTIC_PASS_FILE."
  p="$(ui_input 'Contraseña del repositorio restic' '')"
  RESTIC_PASS_FILE="$(mktemp)"; printf '%s' "$p" > "$RESTIC_PASS_FILE"
fi

run "mkdir -p '$STAGE'"

# --- 3. Mostrar snapshots ---
log "Snapshots en el disco:"
restic snapshots

# --- 4. Elegir y restaurar ---
choices="$(ui_choose_multi 'Qué restaurar' immich nas db)"
[ -n "$choices" ] || { log "No elegiste nada."; exit 0; }

while IFS= read -r tag; do
  [ -n "$tag" ] || continue
  case "$tag" in
    immich|nas)
      # 'warden backup' guarda TODAS las rutas de archivos juntas bajo el tag
      # 'files' (un solo snapshot) — así que para restaurar un componente
      # puntual filtramos por su ruta real dentro de ese snapshot, no por tag.
      catalog_load "$tag" || die "No conozco el componente '$tag' en el catálogo"
      log "Restaurando '$tag' bajo $STAGE …"
      for p in "${COMP_PATHS[@]:-}"; do
        [ -n "$p" ] || continue
        run "restic restore latest --tag files --include '$p' --target /restore"
      done
      ok "'$tag' restaurado en $STAGE (movelo a su ubicación final)"
      ;;
    db)
      log "Restaurando dumps de BD …"
      run "restic restore latest --tag db --target /restore"
      warden_restore_dbs
      ;;
  esac
done <<<"$choices"

ok "Restauración terminada. Revisá $STAGE."
