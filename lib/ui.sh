#!/usr/bin/env bash
# lib/ui.sh — interfaz de usuario. Usa gum (bonito); si no está, cae a texto plano.

# Instala gum según la distro (una vez). Si no se puede, seguimos en modo plano.
ui_ensure_gum() {
  has gum && return 0
  case "${DISTRO:-unknown}" in
    debian)
      run "mkdir -p /etc/apt/keyrings"
      run "curl -fsSL https://repo.charm.sh/apt/gpg.key | gpg --dearmor -o /etc/apt/keyrings/charm.gpg"
      run "echo 'deb [signed-by=/etc/apt/keyrings/charm.gpg] https://repo.charm.sh/apt/ * *' > /etc/apt/sources.list.d/charm.list"
      run "apt-get update -qq && apt-get install -y gum" || true
      ;;
    arch)
      run "pacman -S --needed --noconfirm gum" || true
      ;;
  esac
  has gum || warn "No se pudo instalar gum; sigo en modo texto plano."
}

ui_banner() {
  if has gum; then
    gum style --border rounded --padding "1 3" --border-foreground 212 \
      "warden" "tu server, a tu manera"
  else
    echo "===== warden ====="
  fi
}

# ui_confirm "pregunta"  -> retorna 0 (sí) / 1 (no)
ui_confirm() {
  [ "${WARDEN_CI:-0}" = 1 ] && return 0
  if has gum; then gum confirm "$1"; else
    read -rp "$1 [s/N] " r; [ "$r" = s ] || [ "$r" = S ]
  fi
}

# ui_input "etiqueta" "valor_por_defecto" -> imprime lo escrito
ui_input() {
  [ "${WARDEN_CI:-0}" = 1 ] && { echo "${2:-}"; return 0; }
  if has gum; then gum input --placeholder "$1" --value "${2:-}"; else
    read -rp "$1 [${2:-}]: " r; echo "${r:-${2:-}}"
  fi
}

# ui_choose_multi "Título" opcion1 opcion2 ...  -> imprime las elegidas (una por línea)
ui_choose_multi() {
  local title="$1"; shift
  if has gum; then
    printf '%s\n' "$@" | gum choose --no-limit --header "$title"
  else
    echo "$title (escribí los números separados por espacio):" >&2
    local i=1; for o in "$@"; do echo "  $i) $o" >&2; i=$((i+1)); done
    read -rp "> " nums
    local sel=(); for n in $nums; do sel+=("${!n}"); done
    printf '%s\n' "${sel[@]}"
  fi
}

# ui_menu "Título" op1 op2 ...  -> imprime UNA opción elegida
ui_menu() {
  local title="$1"; shift
  # En CI devuelve la primera opción sin interacción.
  [ "${WARDEN_CI:-0}" = 1 ] && { echo "$1"; return 0; }
  if has gum; then
    printf '%s\n' "$@" | gum choose --header "$title"
  else
    local opts=("$@") i=1 n
    echo "$title" >&2
    for o in "${opts[@]}"; do echo "  $i) $o" >&2; i=$((i+1)); done
    read -rp "> " n
    echo "${opts[$((n-1))]:-}"
  fi
}

# ui_timezone "default" -> elegís de la lista REAL de zonas horarias del
# sistema (timedatectl list-timezones), no escribís/copiás el nombre a
# mano. Con gum podés escribir para filtrar (ej: "Bogota") en vez de
# scrollear cientos de opciones; el valor detectado queda primero en la
# lista para confirmarlo con un solo Enter.
ui_timezone() {
  local def="${1:-}"
  local list=()
  if has timedatectl; then
    mapfile -t list < <(timedatectl list-timezones)
  elif [ -d /usr/share/zoneinfo ]; then
    mapfile -t list < <(find /usr/share/zoneinfo -type f -printf '%P\n' 2>/dev/null | sort)
  fi
  if [ "${#list[@]}" -eq 0 ] || ! has gum; then
    ui_input "Zona horaria (ej: America/Bogota)" "$def"
    return
  fi
  [ -n "$def" ] && list=("$def" "${list[@]}")
  ui_menu "Zona horaria — escribí para filtrar (ej: Bogota)" "${list[@]}"
}
