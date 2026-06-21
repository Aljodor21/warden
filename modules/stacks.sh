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

  export WARDEN_DATA="${WARDEN_DATA:-/srv/warden}"
  export TZ="${WARDEN_TIMEZONE:-UTC}"

  # Crear vacías las carpetas de datos declaradas (instalación limpia = listo desde el día 1).
  local p
  for p in "${COMP_PATHS[@]:-}"; do
    [ -n "$p" ] && run "mkdir -p '$p'"
  done

  # Generar (una sola vez) los secretos que el stack necesite.
  local envfile="" secret
  if [ "${#COMP_SECRETS[@]}" -gt 0 ]; then
    envfile="/etc/warden/secrets/$tag.env"
    if [ "${WARDEN_DRY_RUN:-0}" = 1 ]; then
      echo "   [dry-run] generaría secretos en $envfile: ${COMP_SECRETS[*]}"
    else
      run "mkdir -p /etc/warden/secrets"
      [ -f "$envfile" ] || : > "$envfile"
      for secret in "${COMP_SECRETS[@]}"; do
        grep -q "^$secret=" "$envfile" 2>/dev/null || \
          echo "$secret=$(openssl rand -hex 16)" >> "$envfile"
      done
      chmod 600 "$envfile"
    fi
  fi

  log "Instalando $COMP_NAME ($tag)…"
  if [ -n "$envfile" ]; then
    run "_compose --env-file '$envfile' -f '$compose' up -d" || { warn "$COMP_NAME falló al levantar"; return 1; }
  else
    run "_compose -f '$compose' up -d" || { warn "$COMP_NAME falló al levantar"; return 1; }
  fi
  ok "$COMP_NAME arriba"

  # Mensaje de post-instalación, si el stack define uno (ej. cómo conectarte).
  local postinstall="$(dirname "$compose")/post-install.sh"
  if [ -f "$postinstall" ] && [ "${WARDEN_DRY_RUN:-0}" != 1 ]; then
    # shellcheck source=/dev/null
    bash "$postinstall"
  fi
}
