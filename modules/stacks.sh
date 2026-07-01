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

# Detecta si un contenedor crashea por permisos en sus datos y lo corrige solo.
# Muchas imágenes corren como un usuario no-root (ej. n8n → UID 1000 'node') pero
# warden crea las carpetas del host como root → el contenedor no puede escribir.
_stack_fix_perms() {
  local container="$1"; shift
  local -a paths=("$@")

  sleep 3
  local cstatus
  cstatus=$(docker inspect "$container" --format '{{.State.Status}}' 2>/dev/null) || return 0
  [ "$cstatus" = "restarting" ] || return 0

  docker logs "$container" 2>&1 | grep -qi "EACCES\|permission denied" || return 0

  warn "Contenedor reiniciando por permisos — corrigiendo automáticamente…"

  local img uid
  img=$(docker inspect "$container" --format '{{.Config.Image}}' 2>/dev/null)
  [ -n "$img" ] || return 1

  # Preguntar al propio contenedor qué UID usa (hereda el USER del Dockerfile).
  uid=$(docker run --rm --entrypoint sh "$img" -c "id -u" 2>/dev/null | tr -d '[:space:]')
  # Ignorar si es root (0) o no es número — en ese caso el problema es otro.
  case "$uid" in
    ''|0) warn "UID del contenedor es root o no se detectó — revisá a mano: docker logs $container"; return 1 ;;
    *[!0-9]*) warn "UID no numérico ($uid) — revisá a mano: docker logs $container"; return 1 ;;
  esac

  log "Ajustando dueño de datos a UID $uid…"
  local p
  for p in "${paths[@]:-}"; do
    [ -n "$p" ] && chown -R "$uid" "$p" 2>/dev/null || true
  done

  docker restart "$container" >/dev/null 2>&1
  sleep 5

  cstatus=$(docker inspect "$container" --format '{{.State.Status}}' 2>/dev/null)
  if [ "$cstatus" = "running" ]; then
    ok "Permisos corregidos (UID $uid) — contenedor arriba"
  else
    warn "Corregí permisos pero $container sigue con problemas. Revisá: docker logs $container"
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

  # Override propio del componente (ej. usuarios del NAS) — si existe, se respeta
  # SIEMPRE, incluso al reinstalar, para no perder configuración hecha en caliente.
  local override="/etc/warden/$tag/docker-compose.override.yml" extra=""
  [ -f "$override" ] && extra="-f '$override'"

  log "Instalando $COMP_NAME ($tag)…"
  if [ -n "$envfile" ]; then
    run "_compose --env-file '$envfile' -f '$compose' $extra up -d --pull always" || { warn "$COMP_NAME falló al levantar"; return 1; }
  else
    run "_compose -f '$compose' $extra up -d --pull always" || { warn "$COMP_NAME falló al levantar"; return 1; }
  fi

  # Si el contenedor arrancó pero entra en restart loop por permisos (típico en
  # apps que corren como usuario no-root, ej. n8n como 'node'/UID 1000), corregir
  # el dueño de las carpetas de datos y reiniciar sin intervención manual.
  if [ "${WARDEN_DRY_RUN:-0}" != 1 ] && [ -n "${COMP_CONTAINER:-}" ]; then
    _stack_fix_perms "$COMP_CONTAINER" "${COMP_PATHS[@]:-}"
  fi

  ok "$COMP_NAME arriba"

  # Mensaje de post-instalación, si el stack define uno (ej. cómo conectarte).
  local postinstall="$(dirname "$compose")/post-install.sh"
  if [ -f "$postinstall" ] && [ "${WARDEN_DRY_RUN:-0}" != 1 ]; then
    # shellcheck source=/dev/null
    bash "$postinstall"
  fi
}
