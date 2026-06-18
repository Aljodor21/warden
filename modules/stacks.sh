#!/usr/bin/env bash
# modules/stacks.sh — instala el stack (docker compose) de un componente del catálogo.

# Envoltura: usa docker compose v2 (plugin) o el binario v1.
_compose() {
  if docker compose version >/dev/null 2>&1; then
    docker compose "$@"
  elif has docker-compose; then
    docker-compose "$@"
  else
    die "No encuentro 'docker compose' ni 'docker-compose'"
  fi
}

# warden_stack_install <tag>
warden_stack_install() {
  local tag="$1"
  catalog_load "$tag" || { warn "Componente '$tag' no está en el catálogo"; return 1; }

  local target="${COMP_INSTALL:-}"
  if [ -z "$target" ] || [ "$target" = "—" ]; then
    warn "$COMP_NAME no define COMP_INSTALL; lo salto"; return 0
  fi

  # Proyecto con su propio repo git -> lo maneja el CI/CD, no el instalador.
  case "$target" in
    http*://*|git@*)
      warn "$COMP_NAME se despliega desde su repo git ($target) — eso es del CI/CD (otro paso)."
      return 0 ;;
  esac

  # Si no, es una ruta a un docker-compose dentro del repo.
  local compose="${WARDEN_ROOT:-.}/$target"
  [ -f "$compose" ] || { warn "No encuentro el compose de $COMP_NAME: $compose"; return 1; }

  log "Instalando $COMP_NAME ($tag)…"
  run "_compose -f '$compose' up -d"
  ok "$COMP_NAME arriba"
}
