#!/usr/bin/env bash
# lib/core.sh — núcleo: logging, ejecución segura/dry-run y salvaguardas.

WARDEN_DRY_RUN="${WARDEN_DRY_RUN:-0}"

log()  { printf '\033[1;34m::\033[0m %s\n' "$*"; }
ok()   { printf '\033[1;32m ✓\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m !\033[0m %s\n' "$*" >&2; }
die()  { printf '\033[1;31m ✗\033[0m %s\n' "$*" >&2; exit 1; }

# run "comando" — ejecuta de verdad, o solo lo imprime si WARDEN_DRY_RUN=1
run() {
  if [ "$WARDEN_DRY_RUN" = 1 ]; then
    printf '   [dry-run] %s\n' "$*"
  else
    eval "$@"
  fi
}

need_root() { [ "$(id -u)" -eq 0 ] || die "Esto necesita root. Corré con sudo."; }

has() { command -v "$1" >/dev/null 2>&1; }

# Copia de seguridad antes de editar cualquier archivo del sistema.
backup_file() {
  local f="$1"
  [ -f "$f" ] || return 0
  run "cp -a '$f' '$f.warden.bak.$(date +%Y%m%d-%H%M%S)'"
}
