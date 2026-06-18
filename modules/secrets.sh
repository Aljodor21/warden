#!/usr/bin/env bash
# modules/secrets.sh — cifrado de secretos con age (escrow para recuperación).
#   init    -> genera la llave de cifrado (guardala FUERA del server)
#   save    -> cifra los secretos hacia site/secrets/*.tar.age
#   restore -> los descifra a su ruta original (necesita la llave)

AGE_KEY="${AGE_KEY:-/etc/warden/age.key}"
SECRETS_DIR="${SECRETS_DIR:-${WARDEN_ROOT:-/etc/warden}/site/secrets}"

# Rutas a respaldar cifradas.
_secret_paths() {
  printf '%s\n' \
    "${RESTIC_PASS_FILE:-/root/.warden-restic-password}" \
    "/etc/cloudflared"
}

warden_secrets_init() {
  if [ -f "$AGE_KEY" ]; then ok "Ya existe la llave de cifrado ($AGE_KEY)"; return 0; fi
  [ "${WARDEN_DRY_RUN:-0}" = 1 ] && { echo "[dry-run] generaría la llave en $AGE_KEY"; return 0; }
  has age || ensure_pkg age age-keygen
  mkdir -p "$(dirname "$AGE_KEY")"
  age-keygen -o "$AGE_KEY" 2>/dev/null
  chmod 600 "$AGE_KEY"
  echo
  warn "COPIÁ '$AGE_KEY' a un lugar SEGURO fuera del server (gestor de claves, USB)."
  warn "Sin esa llave NO se pueden descifrar los secretos en una reinstalación."
}

warden_secrets_save() {
  if [ "${WARDEN_DRY_RUN:-0}" = 1 ]; then
    echo "[dry-run] cifraría:"; _secret_paths | sed 's/^/   - /'; return 0
  fi
  has age || ensure_pkg age age-keygen
  [ -f "$AGE_KEY" ] || die "Falta la llave. Corré 'warden secrets init' primero."
  local pub; pub="$(age-keygen -y "$AGE_KEY")"
  mkdir -p "$SECRETS_DIR"
  local p name
  while read -r p; do
    [ -e "$p" ] || continue
    name="$(echo "${p#/}" | tr '/' '_')"
    log "Cifrando $p"
    tar czf - -C / "${p#/}" 2>/dev/null | age -r "$pub" > "$SECRETS_DIR/$name.tar.age"
  done < <(_secret_paths)
  ok "Secretos cifrados en $SECRETS_DIR (van en site/, fuera del repo público)"
}

warden_secrets_restore() {
  if [ "${WARDEN_DRY_RUN:-0}" = 1 ]; then
    echo "[dry-run] descifraría $SECRETS_DIR/*.tar.age a su ruta original"; return 0
  fi
  has age || ensure_pkg age age-keygen
  [ -f "$AGE_KEY" ] || die "Falta la llave $AGE_KEY (traela de tu copia segura)."
  local f
  for f in "$SECRETS_DIR"/*.tar.age; do
    [ -e "$f" ] || continue
    log "Restaurando $(basename "$f")"
    age -d -i "$AGE_KEY" "$f" | tar xzf - -C /
  done
  ok "Secretos restaurados a sus rutas originales"
}
