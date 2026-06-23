#!/usr/bin/env bash
# modules/cloudflare.sh — publica apps por Cloudflare Tunnel desde el catálogo.
#
# Regenera la sección 'ingress' de la config de cloudflared a partir del
# catálogo: cada componente con COMP_CF_HOST + COMP_CF_PORT se vuelve una regla
# (hostname -> http://localhost:puerto), con el catch-all 404 siempre al final.
# Valida ANTES de reemplazar y respalda el archivo previo.

CF_CONFIG="${CF_CONFIG:-/etc/cloudflared/config.yml}"

warden_publish() {
  has cloudflared || die "Falta cloudflared"
  [ -f "$CF_CONFIG" ] || die "No encuentro $CF_CONFIG"

  local tunnel cred tid tmp tag
  tunnel="$(grep -E '^tunnel:' "$CF_CONFIG" | head -1)"
  cred="$(grep -E '^credentials-file:' "$CF_CONFIG" | head -1)"
  tid="$(awk '/^tunnel:/{print $2; exit}' "$CF_CONFIG")"
  [ -n "$tid" ] || die "No pude leer 'tunnel:' de $CF_CONFIG"

  # Generar el nuevo config en un temporal.
  tmp="$(mktemp)"
  {
    echo "$tunnel"
    [ -n "$cred" ] && echo "$cred"
    echo "ingress:"
    while IFS='|' read -r tag _ _ _; do
      ( catalog_load "$tag" || exit 0
        { [ -n "${COMP_CF_HOST:-}" ] && [ -n "${COMP_CF_PORT:-}" ]; } || exit 0
        printf '  - hostname: %s\n    service: http://localhost:%s\n' \
          "$COMP_CF_HOST" "$COMP_CF_PORT" )
    done < <(catalog_each)
    echo "  - service: http_status:404"
  } > "$tmp"

  if [ "${WARDEN_DRY_RUN:-0}" = 1 ]; then
    echo "[dry-run] el nuevo $CF_CONFIG sería:"; sed 's/^/    /' "$tmp"
    rm -f "$tmp"; return 0
  fi

  # Validar ANTES de reemplazar (si no valida, no se toca nada).
  if ! cloudflared --config "$tmp" tunnel ingress validate >/dev/null 2>&1; then
    rm -f "$tmp"; die "El config generado no validó; no cambio nada."
  fi

  backup_file "$CF_CONFIG"
  install -m 644 "$tmp" "$CF_CONFIG"; rm -f "$tmp"

  # Crear las rutas DNS de cada hostname (idempotente).
  while IFS='|' read -r tag _ _ _; do
    ( catalog_load "$tag" || exit 0
      [ -n "${COMP_CF_HOST:-}" ] || exit 0
      log "Ruta DNS: $COMP_CF_HOST"
      cloudflared tunnel route dns "$tid" "$COMP_CF_HOST" >/dev/null 2>&1 || true )
  done < <(catalog_each)

  run "systemctl restart cloudflared"
  ok "Publicado desde el catálogo y cloudflared recargado."

  # Cada deploy puede agregar una app nueva al catálogo (site/catalog) — que
  # aparezca SOLA en el Homepage, sin tener que regenerarlo a mano.
  if command -v warden_homepage_config >/dev/null 2>&1 && docker ps --format '{{.Names}}' | grep -qx homepage; then
    warden_homepage_config
    run "docker restart homepage >/dev/null 2>&1 || true"
    ok "Homepage actualizado con la app recién publicada."
  fi

  # Las credenciales del túnel acaban de tocarse — actualizamos solos el
  # respaldo cifrado, si ya existe la llave (si no, es un paso manual de
  # una sola vez: 'warden secrets init'). No rompe nada si falta.
  if has age && [ -f "${AGE_KEY:-/etc/warden/age.key}" ] && command -v warden_secrets_save >/dev/null 2>&1; then
    warden_secrets_save
  else
    warn "No actualicé el respaldo cifrado de Cloudflared (falta 'warden secrets init'). Es manual, una sola vez."
  fi
}
