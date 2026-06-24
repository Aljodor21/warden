#!/usr/bin/env bash
# modules/reset.sh — instalación limpia: borra TODO lo que warden instaló o
# configuró, dejando el sistema como antes de instalar warden.
#
#   warden reset
#
# Borra: contenedores/datos/config de warden, la llave age, el túnel de
# Cloudflare (de tu cuenta, no solo local), la conexión de Tailscale, las
# apps desplegadas vía CI/CD (su contenedor, usando el repo que el runner
# ya tenía clonado) + el runner mismo, el firewall (ufw vuelve a
# desactivado), Y los paquetes que warden instaló (Docker, Cockpit, avahi,
# zsh, age, cloudflared, tailscale, el compilador de Go) — el sistema
# queda como antes de correr bootstrap.sh por primera vez, no solo sin la
# configuración de warden.
#
# NO toca nunca: el disco de backup externo, tu site/, ni el repo de
# GitHub de tus apps de CI/CD (solo lo que vive en este server).

_reset_down() {  # <archivo compose> [override] [envfile] [contenedor]
  local compose="$1" override="${2:-}" envfile="${3:-}" container="${4:-}"
  [ -f "$compose" ] || return 0
  # Si una corrida anterior ya purgó Docker (estado intermedio real, no
  # hipotético — visto en vivo), _compose moriría con 'die' y abortaría
  # TODO el reset antes de llegar a la purga de paquetes/limpieza de
  # directorios. No hay nada que bajar sin Docker de todas formas.
  if ! has docker; then
    warn "Docker ya no está instalado — salto bajar $compose (no hay nada corriendo sin Docker)"
    return 0
  fi
  local args=(-f "$compose")
  [ -n "$override" ] && [ -f "$override" ] && args+=(-f "$override")
  [ -n "$envfile" ] && [ -f "$envfile" ] && args+=(--env-file "$envfile")
  if ! _compose "${args[@]}" down -v --remove-orphans; then
    # Pasa si falta el .env (ej. un reset previo ya borró /etc/warden):
    # docker compose ni siquiera puede interpolar el archivo, y no baja
    # nada. Si conocemos el contenedor por el catálogo, lo forzamos directo
    # en vez de dejarlo corriendo suelto.
    if [ -n "$container" ] && docker ps -a --format '{{.Names}}' | grep -qx "$container"; then
      warn "$compose no bajó por compose (probable .env faltante) — forzando 'docker rm -f $container'"
      run "docker rm -f '$container'" || warn "No pude borrar $container, revisalo a mano"
    else
      warn "No bajó completo: $compose (revisalo a mano con docker ps -a)"
    fi
  fi
}

warden_reset() {
  echo "Esto va a BORRAR TODO lo que warden instaló o configuró:"
  echo "  - Todos los contenedores/datos de apps instaladas por warden (catálogo + dashboard)"
  echo "  - ${WARDEN_DATA:-/srv/warden} (datos de Immich/NAS/etc.)"
  echo "  - /etc/warden (config, usuarios del NAS, secretos, la llave age)"
  echo "  - Imágenes de Docker que queden sin usar (docker image prune)"
  echo "  - El túnel de Cloudflare (se BORRA de tu cuenta, no solo local) y su config"
  echo "  - La conexión de Tailscale (este server se desconecta de tu tailnet)"
  echo "  - Las apps desplegadas vía CI/CD (sus contenedores en este server) y el runner registrado"
  echo "  - El firewall (ufw vuelve a desactivado, sin reglas)"
  echo "  - Los paquetes que warden instaló (Docker, Cockpit, avahi, zsh, age, cloudflared, tailscale, Go)"
  echo "No toca: el disco de backup externo, tu site/, ni el repo de GitHub de tus apps."
  echo

  if [ "${WARDEN_DRY_RUN:-0}" != 1 ]; then
    read -rp "Escribí exactamente BORRAR para continuar: " ok
    [ "$ok" = "BORRAR" ] || { echo "Cancelado."; return 1; }
  fi

  log "Bajando stacks del dashboard"
  _reset_down "$WARDEN_ROOT/stacks/homepage/docker-compose.yml"
  _reset_down "$WARDEN_ROOT/stacks/backrest/docker-compose.yml"
  _reset_down "$WARDEN_ROOT/stacks/ntfy/docker-compose.yml"

  log "Bajando apps del catálogo (instaladas por warden y desplegadas vía CI/CD)"
  local tag runner_dir="${RUNNER_DIR:-/opt/warden/actions-runner}"
  while IFS='|' read -r tag _ _ _; do
    [ -n "$tag" ] || continue
    catalog_load "$tag" || continue
    case "${COMP_INSTALL:-}" in
      */docker-compose.yml)
        _reset_down "$WARDEN_ROOT/$COMP_INSTALL" "/etc/warden/$tag/docker-compose.override.yml" "/etc/warden/secrets/$tag.env" "${COMP_CONTAINER:-}" ;;
      http*://*|git@*)
        # App de CI/CD: vive en su propio repo, clonado por el runner en
        # _work/<repo>/<repo>/ (estructura estándar de actions-runner).
        local reponame; reponame="$(basename "${COMP_INSTALL%.git}")"
        _reset_down "$runner_dir/_work/$reponame/$reponame/docker-compose.yml" ;;
    esac
  done < <(catalog_each)

  if [ -d "$runner_dir" ] && [ -f "$runner_dir/svc.sh" ]; then
    log "Desinstalando el runner de GitHub Actions (queda offline en GitHub, podés borrarlo ahí si querés del todo)"
    ( cd "$runner_dir" && ./svc.sh stop 2>/dev/null; ./svc.sh uninstall 2>/dev/null ) || true
    run "rm -rf '$runner_dir'"
  fi

  if has docker; then
    log "Limpiando imágenes de Docker sin usar (libera espacio en disco)"
    run "docker image prune -af"
  fi

  log "Desactivando timers"
  run "systemctl disable --now warden-backup.timer warden-verify.timer 2>/dev/null || true"
  run "rm -f /etc/systemd/system/warden-backup.* /etc/systemd/system/warden-verify.*"
  run "systemctl daemon-reload"

  log "Borrando datos generados (${WARDEN_DATA:-/srv/warden})"
  run "rm -rf '${WARDEN_DATA:-/srv/warden}'"

  log "Borrando config de warden (incluida la llave age)"
  run "rm -rf /etc/warden"

  if has cloudflared && [ -f /etc/cloudflared/config.yml ]; then
    log "Borrando el túnel de Cloudflare (de tu cuenta, no solo local)"
    local tid; tid="$(awk '/^tunnel:/{print $2; exit}' /etc/cloudflared/config.yml 2>/dev/null)"
    run "systemctl disable --now cloudflared 2>/dev/null || true"
    [ -n "$tid" ] && run "cloudflared tunnel delete -f '$tid' 2>/dev/null || true"
    run "rm -rf /etc/cloudflared"
  fi

  if has tailscale; then
    log "Desconectando Tailscale de este server"
    run "tailscale logout 2>/dev/null || true"
    run "systemctl disable --now tailscaled 2>/dev/null || true"
  fi

  if has ufw; then
    log "Reseteando firewall (ufw queda desactivado, sin reglas)"
    run "ufw --force reset >/dev/null 2>&1 || true"
  fi

  log "Desinstalando los paquetes que warden instaló (Docker, Cockpit, cloudflared, tailscale, etc.) — el sistema queda como antes de instalar warden, no solo sin su configuración"
  case "${DISTRO:-unknown}" in
    debian)
      # Uno por uno, no todos en una sola invocación: un script post-removal
      # roto de un paquete (visto en vivo con 'cockpit' — su limpieza interna
      # de cockpit-bridge se traga el resto de la lista de argumentos y
      # aborta TODO el purge a mitad de camino) no debe frenar a los demás.
      local pkg
      for pkg in docker-ce docker-ce-cli docker-ce-rootless-extras containerd.io docker-buildx-plugin \
                 docker-compose-plugin cockpit avahi-daemon ufw zsh age cloudflared tailscale golang-go; do
        dpkg -l "$pkg" 2>/dev/null | grep -q '^ii' || continue
        run "apt-get purge -y '$pkg'" || warn "No pude purgar '$pkg', revisalo a mano (dpkg -l | grep $pkg)"
      done
      run "apt-get autoremove --purge -y"
      run "rm -rf /var/lib/docker /var/lib/containerd"
      run "rm -f /etc/apt/sources.list.d/tailscale.list"
      ;;
    arch)
      local pkg
      for pkg in docker docker-compose cockpit avahi ufw zsh age cloudflared tailscale go; do
        pacman -Qi "$pkg" >/dev/null 2>&1 || continue
        run "pacman -Rns --noconfirm '$pkg'" || warn "No pude quitar '$pkg', revisalo a mano (pacman -Qi $pkg)"
      done
      run "rm -rf /var/lib/docker"
      ;;
    *) warn "Distro no reconocida — los paquetes quedan instalados, desinstalalos a mano si querés" ;;
  esac

  if [ -f /etc/systemd/system/warden-panel.service ]; then
    log "Borrando el panel web"
    run "systemctl disable --now warden-panel 2>/dev/null || true"
    run "rm -f /etc/systemd/system/warden-panel.service /usr/local/bin/warden-panel"
    run "systemctl daemon-reload"
  fi

  ok "Reset completo — el sistema queda como antes de instalar warden. Corré 'sudo ./bootstrap.sh' para reinstalar desde cero."
}
