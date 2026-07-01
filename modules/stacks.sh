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

# Ajusta el dueño de las carpetas de datos ANTES de levantar el contenedor.
# Muchas imágenes corren como usuario no-root (n8n → UID 1000 'node', Gitea → 1000,
# etc.). Si warden crea las carpetas como root, el contenedor no puede escribir y
# entra en restart loop inmediato. Hacemos el chown previo para que el primer
# arranque ya funcione, sin intervención manual.
#
# Requiere que la imagen ya esté en caché local (llamar después de 'compose pull').
_stack_preflight_perms() {
  local compose_file="$1"; shift
  local -a paths=("$@")
  [ "${#paths[@]}" -eq 0 ] && return 0

  # Imagen del compose — nuestra plantilla generada por import.sh siempre la tiene
  # en una línea propia 'image: ...', así que un grep simple alcanza.
  local img
  img=$(grep -m1 '^\s*image:' "$compose_file" | awk '{print $2}' | tr -d '"'"'")
  [ -n "$img" ] || return 0

  # Preguntar a la imagen qué UID efectivo usa (hereda el USER del Dockerfile).
  # Al overridear el entrypoint con sh, evitamos que tini / scripts de init
  # interfieran; la imagen corre como el usuario que declare su Dockerfile.
  local uid
  uid=$(docker run --rm --entrypoint sh "$img" -c "id -u" 2>/dev/null | tr -d '[:space:]')
  case "$uid" in
    ''|0|*[!0-9]*) return 0 ;;  # root o no detectado → no hay nada que ajustar
  esac

  log "La imagen corre como UID $uid — ajustando dueño de datos antes de arrancar…"
  local p
  for p in "${paths[@]}"; do
    [ -n "$p" ] && chown -R "$uid" "$p" 2>/dev/null || true
  done
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

  # Bajar imágenes primero (separado del 'up') para poder inspeccionar el usuario
  # del contenedor y pre-ajustar permisos antes del primer arranque.
  if [ "${WARDEN_DRY_RUN:-0}" != 1 ]; then
    if [ -n "$envfile" ]; then
      _compose --env-file "$envfile" -f "$compose" pull 2>&1 || true
    else
      _compose -f "$compose" pull 2>&1 || true
    fi
    _stack_preflight_perms "$compose" "${COMP_PATHS[@]:-}"
  fi

  if [ -n "$envfile" ]; then
    run "_compose --env-file '$envfile' -f '$compose' $extra up -d" || { warn "$COMP_NAME falló al levantar"; return 1; }
  else
    run "_compose -f '$compose' $extra up -d" || { warn "$COMP_NAME falló al levantar"; return 1; }
  fi
  ok "$COMP_NAME arriba"

  # Mensaje de post-instalación, si el stack define uno (ej. cómo conectarte).
  local postinstall="$(dirname "$compose")/post-install.sh"
  if [ -f "$postinstall" ] && [ "${WARDEN_DRY_RUN:-0}" != 1 ]; then
    # shellcheck source=/dev/null
    bash "$postinstall"
  fi
}
