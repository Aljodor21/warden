#!/usr/bin/env bash
# modules/runner.sh — self-hosted runner de GitHub Actions (CI/CD tras CGNAT).
#   Pide la URL del repo y el token de registro (no se guardan en el repo) y
#   deja el agente corriendo como servicio.

# Cada repo tiene su propio runner, en su propia carpeta: /opt/warden/actions-runner-<usuario>-<repo>.
# (Un mismo runner no se puede registrar en dos repos.) RUNNER_DIR fuerza una carpeta concreta.
RUNNER_BASE="${RUNNER_BASE:-/opt/warden}"

# Usuario con el que corre el runner. El instalador de GitHub se niega a correr como root, y cuando
# warden lo llama el panel (que corre como root) SUDO_USER llega vacío o 'root': en ese caso se usa
# el dueño del repo de warden (el usuario del servidor). RUNNER_USER lo fuerza.
_runner_user() {
  local u="${RUNNER_USER:-${SUDO_USER:-}}"
  if [ -z "$u" ] || [ "$u" = root ]; then
    u="$(stat -c %U "${WARDEN_ROOT:-.}" 2>/dev/null || true)"
  fi
  [ -n "$u" ] && [ "$u" != root ] \
    || die "No sé con qué usuario correr el runner (GitHub no permite root). Define RUNNER_USER=usuario."
  printf '%s' "$u"
}

warden_runner() {
  local url token name arch ver tarball slug dir user
  url="${1:-$(ui_input 'URL del repo (https://github.com/usuario/repo)' '')}"
  token="${2:-$(ui_input 'Token de registro (GitHub > repo > Settings > Actions > Runners)' '')}"
  [ -n "$url" ] && [ -n "$token" ] || die "Necesito la URL del repo y el token de registro."
  name="${WARDEN_HOSTNAME:-$(hostname)}-runner"
  slug="$(printf '%s' "${url%.git}" | sed -E 's#^(https://github\.com/|git@github\.com:)##; s#[^A-Za-z0-9._-]+#-#g')"
  dir="${RUNNER_DIR:-$RUNNER_BASE/actions-runner-$slug}"

  case "$(uname -m)" in
    x86_64)        arch=x64 ;;
    aarch64|arm64) arch=arm64 ;;
    *) die "Arquitectura $(uname -m) no soportada por el runner" ;;
  esac

  if [ "${WARDEN_DRY_RUN:-0}" = 1 ]; then
    echo "[dry-run] descargaría y registraría el runner '$name' en $url"; return 0
  fi

  user="$(_runner_user)" || exit 1
  has curl || ensure_pkg curl
  mkdir -p "$dir"

  if [ ! -f "$dir/config.sh" ]; then
    ver="$(curl -fsSL https://api.github.com/repos/actions/runner/releases/latest \
           | grep -oE '"tag_name": *"v[^"]+"' | head -1 | grep -oE '[0-9.]+')"
    [ -n "$ver" ] || die "No pude averiguar la última versión del runner"
    tarball="actions-runner-linux-$arch-$ver.tar.gz"
    log "Descargando runner $ver ($arch)"
    curl -fsSL -o "/tmp/$tarball" \
      "https://github.com/actions/runner/releases/download/v$ver/$tarball"
    sudo -u "$user" tar xzf "/tmp/$tarball" -C "$dir" --no-same-owner
    rm -f "/tmp/$tarball"
    [ -f "$dir/bin/installdependencies.sh" ] && bash "$dir/bin/installdependencies.sh" || true
  fi

  # Todo el contenido debe ser del usuario del runner (una carpeta a medias de un intento
  # anterior puede traer otro dueño).
  chown -R "$user" "$dir"

  log "Registrando el runner en $url"
  sudo -u "$user" bash -c \
    "cd '$dir' && ./config.sh --url '$url' --token '$token' --name '$name' --labels warden --unattended --replace" \
    || die "No se pudo registrar el runner (revisa el token: dura una hora y sirve para un solo repo)."
  [ -f "$dir/.runner" ] || die "El registro no dejó el archivo .runner: el runner NO quedó registrado."

  log "Instalando el runner como servicio"
  ( cd "$dir" && ./svc.sh install "$user" && ./svc.sh start ) \
    || die "No pude instalar o arrancar el runner como servicio."
  ok "Runner '$name' registrado y corriendo (etiqueta: warden)."
}
