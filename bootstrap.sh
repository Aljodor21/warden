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
log "Componentes en el catálogo (instalación de stacks: próxima capa):"
catalog_each | while IFS='|' read -r tag nm kind _; do
  printf '   - %-16s %s\n' "$tag" "$nm"
done

echo
ok "Base lista. Comprobá el estado con:  warden status"
