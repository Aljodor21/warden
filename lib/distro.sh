#!/usr/bin/env bash
# lib/distro.sh — capa de abstracción de distribución.
# Permite que el mismo código corra en Debian/Ubuntu (apt) y Arch (pacman).

detect_distro() {
  local id="" like=""
  if [ -r /etc/os-release ]; then
    # shellcheck source=/dev/null
    . /etc/os-release
    id="${ID:-}"; like="${ID_LIKE:-}"
  fi
  case "$id $like" in
    *debian*|*ubuntu*) echo debian ;;
    *arch*)            echo arch ;;
    *) echo unknown ;;
  esac
}
DISTRO="$(detect_distro)"

# Mapeo de nombre lógico -> nombre de paquete por distro.
pkg_name() {
  local logical="$1"
  case "$DISTRO:$logical" in
    *:restic)       echo restic ;;
    *:dialog)       echo dialog ;;
    *:jq)           echo jq ;;
    debian:age)     echo age ;;
    arch:age)       echo age ;;
    debian:smartctl) echo smartmontools ;;
    arch:smartctl)   echo smartmontools ;;
    *) echo "$logical" ;;
  esac
}

is_installed() { command -v "$1" >/dev/null 2>&1; }

pkg_update() {
  case "$DISTRO" in
    debian) sudo apt-get update -qq ;;
    arch)   sudo pacman -Sy --noconfirm ;;
    *) echo "Distro no soportada para actualizar índices." >&2; return 1 ;;
  esac
}

# ensure_pkg <nombre_logico> [binario_a_verificar]
ensure_pkg() {
  local logical="$1" bin="${2:-$1}" pkg
  is_installed "$bin" && return 0
  pkg="$(pkg_name "$logical")"
  echo "→ Instalando '$pkg' (distro: $DISTRO)…"
  case "$DISTRO" in
    debian) sudo apt-get install -y "$pkg" ;;
    arch)   sudo pacman -S --needed --noconfirm "$pkg" ;;
    *) echo "Distro desconocida: instalá '$pkg' manualmente." >&2; return 1 ;;
  esac
}
