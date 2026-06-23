#!/usr/bin/env bash
# modules/backup.sh — backup genérico guiado por el catálogo.
#   Para CADA componente del catálogo: dumpea su BD (si tiene) y junta sus
#   rutas de archivos. Luego respalda todo con restic (en Docker), montando
#   cada ruta tal cual para guardar la ruta REAL (restaurar en sitio).

warden_backup() {
  local mount repo dumps passfile dry
  mount="${WARDEN_BACKUP_MOUNT:-/mnt/warden-backup}"
  repo="$mount/restic-repo"
  dumps="$mount/_dumps"
  passfile="${RESTIC_PASS_FILE:-/root/.warden-restic-password}"
  dry="${WARDEN_DRY_RUN:-0}"

  [ -d "$repo" ]     || die "No hay repositorio en $repo (¿montaste el disco de backup?)"
  [ -f "$passfile" ] || die "Falta la contraseña restic ($passfile)"
  mkdir -p "$dumps"

  # --- Recorrer el catálogo: juntar rutas (+exclusiones) y dumpear BD ---
  local file_paths=() exclude_args=() tag p pass
  while IFS='|' read -r tag _ _ _; do
    [ -n "$tag" ] || continue
    catalog_load "$tag" || continue
    for p in "${COMP_PATHS[@]:-}"; do
      [ -n "$p" ] && [ -e "$p" ] && file_paths+=("$p")
    done
    for p in "${COMP_EXCLUDES[@]:-}"; do
      [ -n "$p" ] && exclude_args+=(--exclude "$p")
    done
    if [ "${COMP_DB_TYPE:-}" = postgres ] && [ -n "${COMP_DB_CONTAINER:-}" ] \
       && docker ps --format '{{.Names}}' | grep -qx "$COMP_DB_CONTAINER"; then
      log "Dump BD ${COMP_DB_NAME} (${COMP_DB_CONTAINER})"
      pass="$(docker exec "$COMP_DB_CONTAINER" printenv POSTGRES_PASSWORD 2>/dev/null || true)"
      if [ "$dry" = 1 ]; then
        echo "   [dry-run] pg_dump $COMP_DB_NAME -> $dumps/$COMP_DB_NAME.sql"
      else
        docker exec -e PGPASSWORD="$pass" "$COMP_DB_CONTAINER" \
          pg_dump -U "$COMP_DB_USER" -d "$COMP_DB_NAME" > "$dumps/$COMP_DB_NAME.sql"
      fi
    fi
  done < <(catalog_each)

  # --- restic en Docker (cada ruta real montada -> snapshot con ruta real) ---
  local base=(docker run --rm -e RESTIC_PASSWORD_FILE=/pass
              -v "$passfile:/pass:ro" -v "$repo:/repo")
  local vargs=()
  for p in "${file_paths[@]:-}"; do [ -n "$p" ] && vargs+=(-v "$p:$p:ro"); done

  if ! "${base[@]}" restic/restic -r /repo cat config >/dev/null 2>&1; then
    log "Inicializando repositorio restic"
    [ "$dry" = 1 ] && echo "   [dry-run] restic init" || "${base[@]}" restic/restic -r /repo init
  fi

  if [ "${#file_paths[@]}" -gt 0 ]; then
    log "Backup de archivos (${#file_paths[@]} rutas${exclude_args:+, ${#exclude_args[@]} exclusiones})"
    if [ "$dry" = 1 ]; then
      printf '   [dry-run] restic backup --tag files'; printf ' %q' "${exclude_args[@]}" "${file_paths[@]}"; echo
    else
      "${base[@]}" "${vargs[@]}" restic/restic -r /repo backup --tag files "${exclude_args[@]}" "${file_paths[@]}"
    fi
  fi

  log "Backup de dumps de BD"
  if [ "$dry" = 1 ]; then
    echo "   [dry-run] restic backup --tag db /dumps"
  else
    "${base[@]}" -v "$dumps:/dumps:ro" restic/restic -r /repo backup --tag db /dumps
  fi

  rm -rf "$dumps"
  ok "Backup completo en $repo"
}

# warden_verify — comprueba la integridad del repositorio (restic check).
warden_verify() {
  local mount repo passfile
  mount="${WARDEN_BACKUP_MOUNT:-/mnt/warden-backup}"
  repo="$mount/restic-repo"
  passfile="${RESTIC_PASS_FILE:-/root/.warden-restic-password}"
  [ -d "$repo" ]     || die "No hay repositorio en $repo"
  [ -f "$passfile" ] || die "Falta la contraseña restic ($passfile)"
  log "Verificando integridad (restic check)"
  docker run --rm -e RESTIC_PASSWORD_FILE=/pass \
    -v "$passfile:/pass:ro" -v "$repo:/repo" restic/restic -r /repo check
  ok "Repositorio íntegro"
}
