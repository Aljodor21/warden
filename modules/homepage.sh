#!/usr/bin/env bash
# modules/homepage.sh — Homepage: la cara única del server (Opción A).
# Genera su configuración desde el catálogo y levanta el stack.

HOMEPAGE_CONFIG="${HOMEPAGE_CONFIG:-/etc/warden/homepage}"

# Genera settings/widgets/services.yaml a partir del catálogo.
warden_homepage_config() {
  local ip; ip="$(hostname -I 2>/dev/null | awk '{print $1}')"
  run "mkdir -p '$HOMEPAGE_CONFIG'"
  [ "${WARDEN_DRY_RUN:-0}" = 1 ] && { echo "[dry-run] generaría la config de Homepage en $HOMEPAGE_CONFIG"; return 0; }

  printf 'title: %s\n' "${WARDEN_HOSTNAME:-warden}" > "$HOMEPAGE_CONFIG/settings.yaml"

  cat > "$HOMEPAGE_CONFIG/widgets.yaml" <<'EOF'
- resources:
    cpu: true
    memory: true
    disk: /
EOF

  # services.yaml: una tarjeta por componente que tenga puerto o dominio.
  {
    echo "- Apps:"
    while IFS='|' read -r tag _ _ _; do
      [ -n "$tag" ] || continue
      (
        catalog_load "$tag" || exit 0
        href=""
        if [ -n "${COMP_CF_HOST:-}" ]; then
          href="https://$COMP_CF_HOST"
        elif [ -n "${COMP_CF_PORT:-}" ]; then
          href="http://${ip:-localhost}:$COMP_CF_PORT"
        else
          exit 0
        fi
        printf '    - %s:\n        href: %s\n        description: %s\n' \
          "$COMP_NAME" "$href" "$COMP_TAG"
      )
    done < <(catalog_each)
  } > "$HOMEPAGE_CONFIG/services.yaml"
}

warden_homepage_install() {
  log "Generando configuración de Homepage desde el catálogo"
  warden_homepage_config
  export HOMEPAGE_CONFIG
  log "Levantando Homepage"
  run "_compose -f '$WARDEN_ROOT/stacks/homepage/docker-compose.yml' up -d"
  local ip; ip="$(hostname -I 2>/dev/null | awk '{print $1}')"
  ok "Homepage → http://${ip:-<ip>}:${HOMEPAGE_PORT:-7575}"
}
