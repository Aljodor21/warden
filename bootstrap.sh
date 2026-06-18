#!/usr/bin/env bash
#
# bootstrap.sh — instalador de warden desde cero.
#   Detecta la distro (Debian/Ubuntu o Arch), prepara la base e instala Docker.
#   La instalación de apps/stacks se agrega en capas siguientes.
#
#   Uso:
#       sudo ./bootstrap.sh
#   Simulación (no toca nada, solo muestra):
#       WARDEN_DRY_RUN=1 sudo ./bootstrap.sh
#
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
WARDEN_ROOT="$HERE"; export WARDEN_ROOT
# shellcheck source=/dev/null
source "$HERE/lib/core.sh"
# shellcheck source=/dev/null
source "$HERE/lib/distro.sh"
# shellcheck source=/dev/null
source "$HERE/lib/ui.sh"
# shellcheck source=/dev/null
source "$HERE/lib/catalog.sh"
for m in "$HERE"/modules/*.sh; do
  # shellcheck source=/dev/null
  source "$m"
done

need_root
[ "$DISTRO" = unknown ] && die "Distro no soportada (por ahora: Debian/Ubuntu y Arch)."

# Config del sitio (si existe). Plantilla en examples/site.conf.example
[ -f "$HERE/site/site.conf" ] && { source "$HERE/site/site.conf"; ok "Cargado site/site.conf"; }

ui_banner
log "Distro: $DISTRO    Hostname objetivo: ${WARDEN_HOSTNAME:-(sin cambio)}"

# Preflight: internet
if ! curl -fsS --max-time 5 https://github.com >/dev/null 2>&1; then
  warn "No detecto internet; las instalaciones pueden fallar."
fi

if ! ui_confirm "¿Instalar la base del sistema + Docker ahora?"; then
  die "Cancelado por el usuario."
fi

# Mejor experiencia de menús más adelante
ui_ensure_gum

# Cimientos
warden_base_install
warden_docker_install

echo
# --- Selección de componentes a instalar (a la carta, desde el catálogo) ---
mapfile -t CAT_LINES < <(catalog_each)
if [ "${#CAT_LINES[@]}" -eq 0 ]; then
  warn "El catálogo está vacío (site/catalog o examples/catalog). Nada de apps por ahora."
else
  OPTS=()
  for line in "${CAT_LINES[@]}"; do
    IFS='|' read -r tag nm _ _ <<<"$line"
    OPTS+=("$tag — $nm")
  done
  log "Elegí qué componentes instalar:"
  CHOSEN="$(ui_choose_multi 'Componentes a instalar' "${OPTS[@]}")"
  if [ -n "$CHOSEN" ]; then
    while IFS= read -r row; do
      [ -n "$row" ] || continue
      warden_stack_install "${row%% — *}" || warn "Falló ${row%% — *}, sigo con el resto"
    done <<<"$CHOSEN"
  else
    log "No elegiste componentes (queda solo base + Docker)."
  fi
fi

echo
if ui_confirm "¿Instalar Cockpit (panel del sistema, web en :9090)?"; then
  warden_cockpit_install
fi

echo
ok "Base lista. Estado:  warden status"
