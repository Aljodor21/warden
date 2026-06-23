#!/usr/bin/env bash
# lib/presets.sh — combos predefinidos de instalación.
# Cada item es "mod:<modulo>" (una función warden_<modulo>_install) o "app:<tag>".

preset_items() {
  case "$1" in
    # básico: dashboard (panel propio) + NAS — un server liviano con almacenamiento compartido.
    basico)
      echo "mod:cockpit mod:panel app:nas mod:dotfiles mod:motd mod:firewall" ;;
    # completo: básico + apps (Backrest, ntfy, Immich, Docmost, Excalidraw).
    completo)
      echo "mod:cockpit mod:panel app:nas mod:backrest mod:ntfy app:immich app:docmost app:excalidraw mod:dotfiles mod:motd mod:firewall" ;;
    *) return 1 ;;
  esac
}

warden_preset_install() {
  local name="$1" item
  log "Instalando preset: $name"
  for item in $(preset_items "$name"); do
    case "$item" in
      mod:*) "warden_${item#mod:}_install" || warn "Falló ${item#mod:}, sigo" ;;
      app:*) warden_stack_install "${item#app:}" || warn "Falló app ${item#app:}, sigo" ;;
    esac
  done
}
