#!/usr/bin/env bash
# modules/homepage.sh — Homepage: la cara única del server (Opción A).
# Genera su configuración desde el catálogo y levanta el stack.

HOMEPAGE_CONFIG="${HOMEPAGE_CONFIG:-/etc/warden/homepage}"

# Genera settings/widgets/services.yaml desde el catálogo y los servicios activos.
warden_homepage_config() {
  local ip svc up; ip="$(hostname -I 2>/dev/null | awk '{print $1}')"
  run "mkdir -p '$HOMEPAGE_CONFIG'"
  [ "${WARDEN_DRY_RUN:-0}" = 1 ] && { echo "[dry-run] generaría la config de Homepage en $HOMEPAGE_CONFIG"; return 0; }

  printf 'title: %s\n' "${WARDEN_HOSTNAME:-warden}" > "$HOMEPAGE_CONFIG/settings.yaml"
  cat > "$HOMEPAGE_CONFIG/widgets.yaml" <<'EOF'
- resources:
    cpu: true
    memory: true
    disk: /
EOF

  svc="$HOMEPAGE_CONFIG/services.yaml"
  up="$(docker ps --format '{{.Names}}' 2>/dev/null)"
  : > "$svc"

  # --- Grupo Dashboard: los paneles de warden que estén activos ---
  local dash=""
  systemctl is-active --quiet cockpit.socket 2>/dev/null && dash="${dash}    - Cockpit:
        href: https://${ip:-localhost}:9090
        description: panel del sistema
"
  grep -qx backrest <<<"$up" && dash="${dash}    - Backrest:
        href: http://${ip:-localhost}:9898
        description: backups
"
  grep -qx ntfy <<<"$up" && dash="${dash}    - ntfy:
        href: http://${ip:-localhost}:8080
        description: alertas
"
  [ -n "$dash" ] && { echo "- Dashboard:" >> "$svc"; printf '%s' "$dash" >> "$svc"; }

  # --- Grupo Apps: componentes del catálogo corriendo (o públicos por dominio) ---
  local apps="" tag entry
  while IFS='|' read -r tag _ _ _; do
    [ -n "$tag" ] || continue
    entry="$(
      catalog_load "$tag" || exit 0
      href=""
      if [ -n "${COMP_CF_HOST:-}" ]; then
        href="https://$COMP_CF_HOST"
      elif [ -n "${COMP_CF_PORT:-}" ]; then
        grep -qx "${COMP_CONTAINER:-$tag}" <<<"$up" || exit 0
        href="http://${ip:-localhost}:$COMP_CF_PORT"
      else
        exit 0
      fi
      printf '    - %s:\n        href: %s\n        description: %s\n' "$COMP_NAME" "$href" "$COMP_TAG"
    )"
    [ -n "$entry" ] && apps="${apps}${entry}"
  done < <(catalog_each)
  [ -n "$apps" ] && { echo "- Apps:" >> "$svc"; printf '%s' "$apps" >> "$svc"; }

  # Fallback: si todo quedó vacío, al menos Cockpit.
  [ -s "$svc" ] || printf -- '- Sistema:\n    - Cockpit:\n        href: https://%s:9090\n' "${ip:-localhost}" > "$svc"
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
