#!/usr/bin/env bash
# modules/runner.sh — self-hosted runner de GitHub Actions (CI/CD tras CGNAT).
#   Pide la URL del repo y el token de registro (no se guardan en el repo) y
#   deja el agente corriendo como servicio.

RUNNER_DIR="${RUNNER_DIR:-/opt/warden/actions-runner}"
RUNNER_USER="${RUNNER_USER:-${SUDO_USER:-$USER}}"

warden_runner() {
  local url token name arch ver tarball
  url="${1:-$(ui_input 'URL del repo (https://github.com/usuario/repo)' '')}"
  token="${2:-$(ui_input 'Token de registro (GitHub > repo > Settings > Actions > Runners)' '')}"
  [ -n "$url" ] && [ -n "$token" ] || die "Necesito la URL del repo y el token de registro."
  name="${WARDEN_HOSTNAME:-$(hostname)}-runner"

  case "$(uname -m)" in
    x86_64)        arch=x64 ;;
    aarch64|arm64) arch=arm64 ;;
    *) die "Arquitectura $(uname -m) no soportada por el runner" ;;
  esac

  if [ "${WARDEN_DRY_RUN:-0}" = 1 ]; then
    echo "[dry-run] descargaría y registraría el runner '$name' en $url"; return 0
  fi

  has curl || ensure_pkg curl
  mkdir -p "$RUNNER_DIR"; chown "$RUNNER_USER" "$RUNNER_DIR"

  if [ ! -f "$RUNNER_DIR/config.sh" ]; then
    ver="$(curl -fsSL https://api.github.com/repos/actions/runner/releases/latest \
           | grep -oE '"tag_name": *"v[^"]+"' | head -1 | grep -oE '[0-9.]+')"
    [ -n "$ver" ] || die "No pude averiguar la última versión del runner"
    tarball="actions-runner-linux-$arch-$ver.tar.gz"
    log "Descargando runner $ver ($arch)"
    curl -fsSL -o "/tmp/$tarball" \
      "https://github.com/actions/runner/releases/download/v$ver/$tarball"
    sudo -u "$RUNNER_USER" tar xzf "/tmp/$tarball" -C "$RUNNER_DIR"
    rm -f "/tmp/$tarball"
    [ -f "$RUNNER_DIR/bin/installdependencies.sh" ] && bash "$RUNNER_DIR/bin/installdependencies.sh" || true
  fi

  log "Registrando el runner en $url"
  sudo -u "$RUNNER_USER" bash -c \
    "cd '$RUNNER_DIR' && ./config.sh --url '$url' --token '$token' --name '$name' --labels warden --unattended --replace"

  log "Instalando el runner como servicio"
  ( cd "$RUNNER_DIR" && ./svc.sh install "$RUNNER_USER" && ./svc.sh start )
  ok "Runner '$name' registrado y corriendo (etiqueta: warden)."
}
