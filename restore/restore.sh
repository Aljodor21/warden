#!/usr/bin/env bash
#
# restore.sh — restaura un backup de warden desde un disco, SOLO.
#
#   1) Detecta el disco de backup (cualquiera con el marcador, no el del SO).
#   2) Mira QUÉ hay realmente en el backup (rutas de archivos + nombres de
#      BD), cruzándolo con el catálogo — no una lista fija de apps.
#   3) Para cada app que tiene datos en el backup pero no está instalada
#      todavía, la instala SOLA con su receta del catálogo (sin preguntar
#      "¿querés instalar X?" — si hay un backup de X, se restaura X).
#      Las apps de CI/CD (su propio repo, sin receta de compose local) no
#      se pueden instalar solas — se avisa y se sigue con el resto.
#   4) Si hay datos de algo que no tiene receta en NINGÚN catálogo (ni
#      genérico ni site/), se avisa y se deja quieto — no se inventa nada.
#   5) Restaura los archivos DIRECTO en su ubicación real, y carga los
#      dumps de BD en su contenedor — deteniendo y volviendo a prender las
#      apps afectadas solo.
#   6) Al terminar, registra el disco como el destino permanente de los
#      backups automáticos (mismo disco, no uno nuevo — solo falta dejarlo
#      anotado para que el timer siga usándolo).
#
#   Modo automático (para el panel, sin preguntar nada):
#     WARDEN_RESTORE_AUTO=1 ./restore.sh
#   Modo interactivo (consola): ./restore.sh
#   Solo una app puntual (desde el panel tras registrar un runner, o desde
#   el propio deploy.yml de esa app después de levantar su contenedor):
#     WARDEN_RESTORE_AUTO=1 ./restore.sh --tag <tag>
#
#   restic corre en Docker (cero instalación).
#
set -euo pipefail

ONLY_TAG=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    --tag) ONLY_TAG="${2:-}"; shift 2 ;;
    *) shift ;;
  esac
done

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
# shellcheck source=/dev/null
source "$WARDEN_ROOT/modules/stacks.sh"
# shellcheck source=/dev/null
source "$WARDEN_ROOT/modules/register.sh"

need_root
has docker || die "Falta docker"
has jq || ensure_pkg jq jq

MARKER=".warden-backup.id"
MOUNT="${WARDEN_BACKUP_MOUNT:-/mnt/warden-backup}"
RESTIC_PASS_FILE="${RESTIC_PASS_FILE:-/root/.warden-restic-password}"
STAGE="${WARDEN_RESTORE_DIR:-/var/tmp/warden-restore}"   # solo para dumps de BD
AUTO_FLAG_FILE="/run/warden-restore-auto"
AUTO=0
[ "${WARDEN_RESTORE_AUTO:-0}" = 1 ] && AUTO=1
[ -f "$AUTO_FLAG_FILE" ] && AUTO=1
# El flag de variable de entorno puede perderse al cruzar un 'sudo'
# intermedio según la política de sudoers (visto en vivo: una versión de
# sudo que ignora '-E' y bloquea preservar el entorno completo) — el
# archivo en /run es un respaldo que no depende de eso en absoluto, el
# panel lo crea/borra él mismo alrededor del comando.

# --- restic en Docker ---
restic_files() {  # restaura archivos DIRECTO en su ruta real del host
  docker run --rm -e RESTIC_PASSWORD_FILE=/pass \
    -v "$RESTIC_PASS_FILE":/pass:ro -v "$REPO":/repo -v /:/restore \
    restic/restic -r /repo "$@"
}
restic_dumps() {  # dumps a una carpeta de staging (no son rutas reales del host)
  docker run --rm -e RESTIC_PASSWORD_FILE=/pass \
    -v "$RESTIC_PASS_FILE":/pass:ro -v "$REPO":/repo -v "$STAGE":/restore \
    restic/restic -r /repo "$@"
}
restic_ro() {  # solo lectura, sin target de escritura
  docker run --rm -e RESTIC_PASSWORD_FILE=/pass \
    -v "$RESTIC_PASS_FILE":/pass:ro -v "$REPO":/repo \
    restic/restic -r /repo "$@"
}

TO_ENSURE_RUNNING=()
_stop_container() {
  local c="$1"
  [ -n "$c" ] || return 0
  printf '%s\n' "${TO_ENSURE_RUNNING[@]}" | grep -qx "$c" || TO_ENSURE_RUNNING+=("$c")
  docker ps --format '{{.Names}}' | grep -qx "$c" || return 0
  log "Deteniendo $c"
  run "docker stop '$c'"
}
_ensure_running() {
  local c
  for c in "${TO_ENSURE_RUNNING[@]}"; do
    [ -n "$c" ] || continue
    log "Asegurando que $c quede prendido"
    run "docker start '$c'" || warn "No pude prender $c, revisalo a mano (docker logs $c)"
  done
}
trap _ensure_running EXIT

# is_deployed_install <COMP_INSTALL> — true si apunta a un repo (CI/CD), no
# a un docker-compose.yml local (mismo criterio que stacks.sh/panel).
is_deployed_install() {
  case "$1" in http*://*|git@*) return 0 ;; *) return 1 ;; esac
}

# --- 1. Encontrar el disco de backup ---
sysdisk="/dev/$(system_disk)"
bmount=""
while read -r mp; do
  [ -n "$mp" ] && [ -f "$mp/$MARKER" ] && { bmount="$mp"; break; }
done < <(lsblk -rno MOUNTPOINT)

if [ -z "$bmount" ]; then
  if [ "$AUTO" = 1 ] || [ ! -t 0 ]; then
    die "No hay disco de backup montado. Conectalo (se monta solo si tiene el marcador) y reintentá."
  fi
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
  [ "$AUTO" = 1 ] && die "Falta la contraseña restic en $RESTIC_PASS_FILE."
  warn "No está $RESTIC_PASS_FILE."
  p="$(ui_input 'Contraseña del repositorio restic' '')"
  RESTIC_PASS_FILE="$(mktemp)"; printf '%s' "$p" > "$RESTIC_PASS_FILE"
fi

log "Snapshots en el disco:"
restic_ro snapshots

# --- 3. Qué hay REALMENTE en el backup (no una lista fija) ---
mapfile -t filePaths < <(restic_ro snapshots --tag files --json --latest 1 2>/dev/null \
  | jq -r '.[0].paths[]? // empty' 2>/dev/null || true)
mapfile -t dbFiles < <(restic_ro ls latest --tag db 2>/dev/null | grep '\.sql$' || true)
dbNames=(); for f in "${dbFiles[@]}"; do [ -n "$f" ] && dbNames+=("$(basename "$f" .sql)"); done

if [ "${#filePaths[@]}" -eq 0 ] && [ "${#dbNames[@]}" -eq 0 ]; then
  ok "No hay nada para restaurar en este disco."
  exit 0
fi

# Mapeo: ruta real -> tag, y nombre de BD -> tag, recorriendo TODO el
# catálogo (genérico + site) — así reconoce cualquier app, no solo
# immich/nas hardcodeados como antes.
declare -A PATH_TO_TAG DBNAME_TO_TAG
while IFS='|' read -r t _ _ _; do
  [ -n "$t" ] || continue
  catalog_load "$t" 2>/dev/null || continue
  for p in "${COMP_PATHS[@]}"; do [ -n "$p" ] && PATH_TO_TAG["$p"]="$t"; done
  [ -n "${COMP_DB_NAME:-}" ] && DBNAME_TO_TAG["${COMP_DB_NAME}"]="$t"
done < <(catalog_each)

neededTags=(); unknownPaths=()
for p in "${filePaths[@]}"; do
  t="${PATH_TO_TAG[$p]:-}"
  if [ -n "$t" ]; then neededTags+=("$t"); else unknownPaths+=("$p"); fi
done

neededDBTags=(); unknownDBs=()
for db in "${dbNames[@]}"; do
  t="${DBNAME_TO_TAG[$db]:-}"
  if [ -n "$t" ]; then neededDBTags+=("$t"); else unknownDBs+=("$db"); fi
done

for p in "${unknownPaths[@]}"; do
  warn "Hay datos respaldados en '$p' pero ningún componente del catálogo los reclama (¿falta tu site/catalog?) — no se restauran."
done
for db in "${unknownDBs[@]}"; do
  warn "Hay un dump de BD '$db' pero ningún componente lo reclama — no se restaura."
done

# Unión de tags a procesar (archivos + BD), sin duplicados.
declare -A seen
allTags=()
for t in "${neededTags[@]}" "${neededDBTags[@]}"; do
  [ -n "${seen[$t]:-}" ] && continue
  seen[$t]=1
  allTags+=("$t")
done

if [ "${#allTags[@]}" -eq 0 ]; then
  ok "Nada que restaurar (todo lo del backup es de rutas/BD sin receta conocida)."
  exit 0
fi

if [ -n "$ONLY_TAG" ]; then
  if printf '%s\n' "${allTags[@]}" | grep -qx "$ONLY_TAG"; then
    allTags=("$ONLY_TAG")
  else
    ok "No hay datos pendientes en el backup para '$ONLY_TAG'."
    exit 0
  fi
fi

# --- 4. Por cada app con datos en el backup: instalarla si falta, o avisar
#        si es de CI/CD (no se puede instalar sola) ---
toRestore=(); affected=(); pendingCICD=()
for t in "${allTags[@]}"; do
  catalog_load "$t" || { warn "'$t' no está en el catálogo, lo salto."; continue; }
  name="${COMP_NAME:-$t}"
  container="${COMP_CONTAINER:-}"

  running=0
  [ -n "$container" ] && docker ps --format '{{.Names}}' | grep -qx "$container" && running=1

  if [ "$running" -eq 0 ]; then
    if [ -n "${COMP_INSTALL:-}" ] && is_deployed_install "$COMP_INSTALL"; then
      warn "'$name' tiene datos en el backup pero vive en su propio repo (CI/CD) — no se puede instalar sola. Registrá su runner y hacé un deploy, después volvé a restaurar. Salto por ahora."
      pendingCICD+=("$t")
      continue
    fi
    log "'$name' no está instalada — instalándola con su receta del catálogo…"
    warden_stack_install "$t" || { warn "No pude instalar '$name', salto su restauración."; continue; }
    catalog_load "$t"  # warden_stack_install puede haber tocado variables globales
    container="${COMP_CONTAINER:-}"
    if [ -n "$container" ] && ! docker ps --format '{{.Names}}' | grep -qx "$container"; then
      warn "'$name' no quedó corriendo después de instalarla, salto su restauración."
      continue
    fi
  fi

  toRestore+=("$t")
  [ -n "$container" ] && affected+=("$container")
done

# Línea parseable: el panel la lee para mostrar, por cada app de CI/CD
# pendiente, el flujo de 'pegá el token / hacé push / restaurar datos'.
[ "${#pendingCICD[@]}" -gt 0 ] && echo "PENDING_CICD:$(IFS=,; echo "${pendingCICD[*]}")"

if [ "${#toRestore[@]}" -eq 0 ]; then
  ok "No quedó nada instalable para restaurar."
  exit 0
fi

echo "Se va a restaurar: ${toRestore[*]}"
[ "${#affected[@]}" -gt 0 ] && echo "Esto detiene brevemente (y vuelve a prender al final): ${affected[*]}"
if [ "$AUTO" != 1 ]; then
  ui_confirm "¿Restaurar ahora?" || { log "Cancelado."; exit 0; }
fi

# Limpiar el staging de dumps de una corrida ANTERIOR — sin esto, un dump
# viejo que quedó en disco se reutiliza en vez de bajar el del backup
# actual (bug real: generar un backup nuevo no servía de nada porque
# restore.sh seguía usando el .sql de la corrida pasada, detectando que
# "ya existe" sin chequear si es el más reciente).
run "rm -rf '$STAGE/dumps'"

# --- 5. Restaurar de verdad ---
for t in "${toRestore[@]}"; do
  catalog_load "$t" || continue
  [ -n "${COMP_CONTAINER:-}" ] && _stop_container "$COMP_CONTAINER"

  if [ "${#COMP_PATHS[@]}" -gt 0 ]; then
    log "Restaurando archivos de '${COMP_NAME:-$t}' en su ubicación real…"
    for p in "${COMP_PATHS[@]}"; do
      [ -n "$p" ] || continue
      run "restic_files restore latest --tag files --include '$p' --target /restore"
    done
  fi

  if [ -n "${COMP_DB_NAME:-}" ] && printf '%s\n' "${dbNames[@]}" | grep -qx "$COMP_DB_NAME"; then
    log "Restaurando dump de BD '${COMP_DB_NAME}'…"
    run "mkdir -p '$STAGE'"
    [ -f "$STAGE/dumps/${COMP_DB_NAME}.sql" ] || run "restic_dumps restore latest --tag db --include /dumps/${COMP_DB_NAME}.sql --target /restore"
    if docker ps --format '{{.Names}}' | grep -qx "${COMP_DB_CONTAINER:-}"; then
      pass="$(docker exec "$COMP_DB_CONTAINER" printenv POSTGRES_PASSWORD 2>/dev/null || true)"
      run "docker exec -i -e PGPASSWORD='$pass' '$COMP_DB_CONTAINER' psql -U '$COMP_DB_USER' -d '$COMP_DB_NAME' < '$STAGE/dumps/${COMP_DB_NAME}.sql'"
      ok "BD '${COMP_DB_NAME}' cargada"
    else
      warn "${COMP_DB_CONTAINER:-(sin contenedor)} no está corriendo; dump queda en $STAGE/dumps sin cargar."
    fi
  fi
  ok "'${COMP_NAME:-$t}' restaurada"
done

# --- 6. Dejar este disco como el destino permanente de los backups
#        automáticos (mismo disco, no uno nuevo — solo falta registrarlo) ---
if [ "$bmount" = "$MOUNT" ]; then
  warden_register || warn "No pude registrar el disco automáticamente — corré 'warden register' a mano."
else
  warn "El disco está montado en $bmount (no en $MOUNT) — para activar el backup automático, corré 'warden register' a mano."
fi

ok "Restauración terminada."
