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
}
