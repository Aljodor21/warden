#!/usr/bin/env bash
#
# restore.sh — restaura un backup de warden desde un disco, SOLO.
#   1) Detecta el disco de backup (cualquiera con el marcador, no el del SO).
#   2) Lista los snapshots y te deja elegir qué restaurar (immich/nas/db).
#   3) Restaura los archivos DIRECTO en su ubicación real (no a mano), y
#      carga los dumps de BD en su contenedor — deteniendo y volviendo a
#      prender las apps afectadas solo, sin pasos manuales.
#   restic corre en Docker (cero instalación).
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
STAGE="${WARDEN_RESTORE_DIR:-/var/tmp/warden-restore}"   # solo para dumps de BD

# Restaura ARCHIVOS directo en su ruta real (las rutas guardadas ya son
# absolutas y reales, por eso el target es la raíz del host).
restic_files() {
  docker run --rm -e RESTIC_PASSWORD_FILE=/pass \
    -v "$RESTIC_PASS_FILE":/pass:ro -v "$REPO":/repo -v /:/restore \
    restic/restic -r /repo "$@"
}

# Restaura dumps de BD a una carpeta temporal (van a /dumps en el snapshot,
# no a una ruta real del host) para luego cargarlos con psql.
restic_dumps() {
  docker run --rm -e RESTIC_PASSWORD_FILE=/pass \
    -v "$RESTIC_PASS_FILE":/pass:ro -v "$REPO":/repo -v "$STAGE":/restore \
    restic/restic -r /repo "$@"
}

# Solo lectura (listar snapshots) — no necesita ningún target de escritura.
restic_ro() {
  docker run --rm -e RESTIC_PASSWORD_FILE=/pass \
    -v "$RESTIC_PASS_FILE":/pass:ro -v "$REPO":/repo \
    restic/restic -r /repo "$@"
}

TO_ENSURE_RUNNING=()
_stop_container() {  # detiene si está corriendo; SIEMPRE queda anotado para garantizar que vuelva a prender
  local c="$1"
  [ -n "$c" ] || return 0
  printf '%s\n' "${TO_ENSURE_RUNNING[@]:-}" | grep -qx "$c" || TO_ENSURE_RUNNING+=("$c")
  docker ps --format '{{.Names}}' | grep -qx "$c" || return 0
  log "Deteniendo $c"
  run "docker stop '$c'"
}
# Garantiza que cada app afectada quede prendida al final, la hayamos
# parado nosotros o ya estuviera caída de antes (ej. se crasheó sola por
# los archivos que faltaban). 'docker start' en uno ya prendido no hace nada.
_ensure_running() {
  local c
  for c in "${TO_ENSURE_RUNNING[@]:-}"; do
    [ -n "$c" ] || continue
    log "Asegurando que $c quede prendido"
    run "docker start '$c'" || warn "No pude prender $c, revisalo a mano (docker logs $c)"
  done
}
trap _ensure_running EXIT

# Carga los dumps restaurados ($STAGE/dumps/*.sql) en su contenedor,
# deteniendo antes la app que los usa (no la BD) y prendiéndola al final.
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
      warn "No sé a qué contenedor va $db.sql; queda en $STAGE/dumps"; continue
    fi
    catalog_load "$found"
    if ! docker ps --format '{{.Names}}' | grep -qx "${COMP_DB_CONTAINER:-}"; then
      warn "$COMP_DB_CONTAINER no está corriendo; instalá $COMP_NAME primero. Salto $db."; continue
    fi
    [ -n "${COMP_CONTAINER:-}" ] && _stop_container "$COMP_CONTAINER"
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

# --- 3. Mostrar snapshots ---
log "Snapshots en el disco:"
restic_ro snapshots

# --- 4. Elegir, confirmar UNA vez, y restaurar todo solo ---
choices="$(ui_choose_multi 'Qué restaurar' immich nas db)"
[ -n "$choices" ] || { log "No elegiste nada."; exit 0; }

affected=()
while IFS= read -r tag; do
  case "$tag" in
    immich|nas)
      catalog_load "$tag" 2>/dev/null && [ -n "${COMP_CONTAINER:-}" ] && affected+=("$COMP_CONTAINER")
      ;;
  esac
done <<<"$choices"

echo "Vas a restaurar: $(echo "$choices" | tr '\n' ' ')"
[ "${#affected[@]}" -gt 0 ] && echo "Esto detiene brevemente (y vuelve a prender al final): ${affected[*]}"
ui_confirm "¿Restaurar ahora?" || { log "Cancelado."; exit 0; }

while IFS= read -r tag; do
  [ -n "$tag" ] || continue
  case "$tag" in
    immich|nas)
      # Las rutas guardadas en el backup son absolutas y reales: restauramos
      # directo a su lugar, filtrando dentro del snapshot combinado por ruta
      # (warden backup guarda todos los archivos juntos bajo el tag 'files').
      catalog_load "$tag" || die "No conozco el componente '$tag' en el catálogo"
      [ -n "${COMP_CONTAINER:-}" ] && _stop_container "$COMP_CONTAINER"
      log "Restaurando '$tag' en su ubicación real…"
      for p in "${COMP_PATHS[@]:-}"; do
        [ -n "$p" ] || continue
        run "restic_files restore latest --tag files --include '$p' --target /restore"
      done
      ok "'$tag' restaurado"
      ;;
    db)
      log "Restaurando dumps de BD…"
      run "mkdir -p '$STAGE'"
      run "restic_dumps restore latest --tag db --target /restore"
      warden_restore_dbs
      ;;
  esac
done <<<"$choices"

ok "Restauración terminada."
