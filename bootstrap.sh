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
# shellcheck source=/dev/null
source "$HERE/lib/presets.sh"
for m in "$HERE"/modules/*.sh; do
  # shellcheck source=/dev/null
  source "$m"
done

need_root
[ "$DISTRO" = unknown ] && die "Distro no soportada (por ahora: Debian/Ubuntu y Arch)."

# Asegurar la carpeta de config del sitio (un clon nuevo no la trae, está en .gitignore).
mkdir -p "$HERE/site/catalog"
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
# --- Modo de instalación: preset (combo) o a la carta ---
MODE="$(ui_menu '¿Qué tipo de server querés montar?' \
  'minimal — dashboard + backup (lo básico)' \
  'media — minimal + Immich (fotos/medios)' \
  'dev — minimal + apps de desarrollo' \
  'a la carta — elegir manual')"

case "$MODE" in
  minimal*) warden_preset_install minimal ;;
  media*)   warden_preset_install media ;;
  dev*)     warden_preset_install dev ;;
  *)
    # A la carta: apps del catálogo + módulos, uno por uno.
    mapfile -t CAT_LINES < <(catalog_each)
    if [ "${#CAT_LINES[@]}" -gt 0 ]; then
      OPTS=()
      for line in "${CAT_LINES[@]}"; do
        IFS='|' read -r tag nm _ _ <<<"$line"; OPTS+=("$tag — $nm")
      done
      CHOSEN="$(ui_choose_multi 'Apps a instalar' "${OPTS[@]}")"
      while IFS= read -r row; do
        [ -n "$row" ] || continue
        warden_stack_install "${row%% — *}" || warn "Falló ${row%% — *}, sigo"
      done <<<"$CHOSEN"
    fi
    for q in \
      "Cockpit (panel del sistema, :9090):warden_cockpit_install" \
      "Homepage (tablero principal):warden_homepage_install" \
      "Backrest (UI de backups, :9898):warden_backrest_install" \
      "ntfy (alertas, :8080):warden_ntfy_install" \
      "shell (zsh + oh-my-zsh + p10k):warden_dotfiles_install" \
      "MOTD (saludo al iniciar sesión):warden_motd_install" \
      "firewall equilibrado (ufw):warden_firewall_install"; do
      echo
      if ui_confirm "¿Instalar ${q%%:*}?"; then "${q#*:}" || warn "Falló ${q%%:*}, sigo"; fi
    done
    ;;
esac

# Regenerar Homepage al final, ya con las apps corriendo (para que no quede vacío).
if docker ps --format '{{.Names}}' 2>/dev/null | grep -qx homepage; then
  warden_homepage_config && log "Homepage actualizado con lo que quedó corriendo"
fi

echo
log "Dejando 'warden' disponible en el PATH"
run "ln -sf '$WARDEN_ROOT/bin/warden' /usr/local/bin/warden"

echo
ok "Listo. Escribí:  warden"
