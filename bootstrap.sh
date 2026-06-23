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

if [ -f "$HERE/site/site.conf" ]; then
  source "$HERE/site/site.conf"
  ok "Cargado site/site.conf"
else
  # Primera vez: lo preguntamos solo (con valores detectados), nada de
  # editar un archivo a mano. Plantilla completa en examples/site.conf.example.
  log "Primera vez en este server — unos datos básicos (después no se vuelve a preguntar):"
  def_host="$(hostname)"
  def_tz="$(timedatectl show --property=Timezone --value 2>/dev/null || cat /etc/timezone 2>/dev/null || echo America/Bogota)"
  def_lan="$(ip -4 addr show scope global 2>/dev/null | awk '/inet /{print $2; exit}')"
  [ -n "$def_lan" ] || def_lan="192.168.0.0/24"

  WARDEN_HOSTNAME="$(ui_input 'Nombre del server (se accede como nombre.local)' "$def_host")"
  WARDEN_TIMEZONE="$(ui_input 'Zona horaria' "$def_tz")"
  WARDEN_LAN="$(ui_input 'Subred de tu LAN (para el firewall)' "$def_lan")"

  {
    echo "WARDEN_HOSTNAME=\"$WARDEN_HOSTNAME\""
    echo "WARDEN_TIMEZONE=\"$WARDEN_TIMEZONE\""
    echo "WARDEN_LAN=\"$WARDEN_LAN\""
    echo 'WARDEN_PRESET=""'
    echo 'WARDEN_BACKUP_UUID=""'
    echo 'WARDEN_DATA="/srv/warden"'
  } > "$HERE/site/site.conf"
  ok "Guardado en site/site.conf — no hace falta tocarlo de nuevo."
fi

ui_banner
log "Distro: $DISTRO    Hostname objetivo: ${WARDEN_HOSTNAME:-(sin cambio)}"

# Preflight: internet
if ! curl -fsS --max-time 5 https://github.com >/dev/null 2>&1; then
  warn "No detecto internet; las instalaciones pueden fallar."
fi

echo "Esto va a instalar:"
echo "  - Utilidades base (curl, git, jq, ca-certificates)"
echo "  - Docker (motor de contenedores)"
echo "  - avahi (acceso por nombre, ej. http://$(hostname).local, en vez de la IP)"
echo "  - Zona horaria y hostname según site/site.conf"
echo
if ! ui_confirm "¿Instalar todo esto ahora?"; then
  die "Cancelado por el usuario."
fi

# Mejor experiencia de menús más adelante
ui_ensure_gum

# Cimientos
warden_base_install
warden_docker_install

echo
echo "Tailscale (VPN) te deja entrar a este server desde afuera de tu red,"
echo "de forma segura, sin abrir puertos. Si decís que sí, en un momento te"
echo "va a pedir abrir una URL para asociar este server a tu cuenta."
if ui_confirm "¿Instalar Tailscale?"; then
  warden_tailscale_install
fi

echo
echo "Cloudflare Tunnel deja exponer tus apps a internet con un dominio propio,"
echo "sin abrir puertos — y es lo que necesita el CI/CD para publicar solo cada"
echo "app nueva que desplegués. Si decís que sí, te va a pedir loguearte con tu"
echo "cuenta de Cloudflare (abriendo una URL) y elegir el dominio."
if ui_confirm "¿Configurar Cloudflare Tunnel?"; then
  warden_cloudflare_init
fi

echo
echo "El panel web te deja ver y editar el catálogo (qué apps, sus rutas, su"
echo "subdominio) desde el navegador, en vez de editar archivos por consola."
echo "Queda protegido con clave, solo accesible desde tu LAN/Tailscale."
if ui_confirm "¿Instalar el panel web?"; then
  warden_panel_install
fi

echo
# --- Modo de instalación: preset (combo) o a la carta ---
MODE="$(ui_menu '¿Qué tipo de server querés montar?' \
  'básico — dashboard (Cockpit + panel propio) + NAS' \
  'completo — básico + Backrest + ntfy + Immich + Docmost + Excalidraw' \
  'a la carta — elegir manual, uno por uno')"

case "$MODE" in
  básico*|basico*) warden_preset_install basico ;;
  completo*)        warden_preset_install completo ;;
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
      "Panel propio de warden (dashboard, :7890):warden_panel_install" \
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

echo
log "Dejando 'warden' disponible en el PATH"
run "ln -sf '$WARDEN_ROOT/bin/warden' /usr/local/bin/warden"

echo
ok "Listo. Escribí:  warden"
