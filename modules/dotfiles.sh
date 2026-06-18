#!/usr/bin/env bash
# modules/dotfiles.sh — shell bonito: zsh + oh-my-zsh + plugins + powerlevel10k.

_omz_clone() {  # <usuario> <repo> <destino>
  [ -d "$3" ] && return 0
  run "sudo -u '$1' git clone --depth=1 '$2' '$3'"
}

warden_dotfiles_install() {
  local u="${SUDO_USER:-$USER}" home
  home="$(getent passwd "$u" | cut -d: -f6)"
  [ -n "$home" ] || { warn "No encuentro el home de $u"; return 1; }

  ensure_pkg zsh zsh

  local omz="$home/.oh-my-zsh" custom
  custom="$omz/custom"

  if [ ! -d "$omz" ]; then
    log "Instalando oh-my-zsh para $u"
    _omz_clone "$u" https://github.com/ohmyzsh/ohmyzsh.git "$omz"
  else
    ok "oh-my-zsh ya está"
  fi

  _omz_clone "$u" https://github.com/zsh-users/zsh-autosuggestions     "$custom/plugins/zsh-autosuggestions"
  _omz_clone "$u" https://github.com/zsh-users/zsh-syntax-highlighting "$custom/plugins/zsh-syntax-highlighting"
  _omz_clone "$u" https://github.com/romkatv/powerlevel10k             "$custom/themes/powerlevel10k"

  local zrc="$home/.zshrc"
  if [ "${WARDEN_DRY_RUN:-0}" != 1 ]; then
    backup_file "$zrc"
    cat > "$zrc" <<'EOF'
export ZSH="$HOME/.oh-my-zsh"
ZSH_THEME="powerlevel10k/powerlevel10k"
plugins=(git docker docker-compose sudo z colored-man-pages history-substring-search zsh-autosuggestions zsh-syntax-highlighting)
source "$ZSH/oh-my-zsh.sh"
[[ -f "$HOME/.p10k.zsh" ]] && source "$HOME/.p10k.zsh"
EOF
    run "chown '$u' '$zrc'"
  else
    echo "[dry-run] escribiría $zrc (powerlevel10k + plugins esenciales)"
  fi

  run "chsh -s \"\$(command -v zsh)\" '$u'" || warn "No pude cambiar el shell por defecto a zsh"
  ok "Shell listo para $u (reiniciá sesión; 'p10k configure' para ajustar el tema)"
}
