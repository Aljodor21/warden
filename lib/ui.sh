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
  if has gum; then gum confirm "$1"; else
    read -rp "$1 [s/N] " r; [ "$r" = s ] || [ "$r" = S ]
  fi
}

# ui_input "etiqueta" "valor_por_defecto" -> imprime lo escrito
ui_input() {
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
