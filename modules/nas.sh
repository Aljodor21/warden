#!/usr/bin/env bash
# modules/nas.sh — gestión de usuarios del NAS genérico (Samba).
#
#   warden nas adduser <nombre> [clave]   — crea un usuario (clave aleatoria si no se da)
#   warden nas passwd  <nombre> [clave]   — cambia la clave de un usuario
#   warden nas deluser <nombre>           — elimina un usuario (no el 'warden' por defecto)
#   warden nas users                      — lista los usuarios (sin mostrar claves)
#
# Los usuarios viven en /etc/warden/nas/users.txt (nombre:clave, 600).
# Cada cambio regenera el override de compose y reinicia el contenedor 'nas'.

NAS_DIR="/etc/warden/nas"
NAS_USERS_FILE="$NAS_DIR/users.txt"
NAS_OVERRIDE="$NAS_DIR/docker-compose.override.yml"

_nas_seed() {
  mkdir -p "$NAS_DIR"
  [ -f "$NAS_USERS_FILE" ] && return 0
  local pass
  pass="$(grep '^NAS_PASSWORD=' /etc/warden/secrets/nas.env 2>/dev/null | cut -d= -f2)"
  [ -n "$pass" ] || pass="$(openssl rand -hex 16)"
  echo "warden:$pass" > "$NAS_USERS_FILE"
  chmod 600 "$NAS_USERS_FILE"
}

# Regenera el override (usuarios + share compartido) y aplica con el instalador genérico.
# Sintaxis verificada contra el README de dperson/samba:
#   -u "nombre;clave"          (uno por usuario)
#   -s "nombre;/ruta;...;usuarios_separados_por_COMA"
_nas_apply() {
  local name pass users_args=() valid=()
  while IFS=: read -r name pass; do
    [ -n "$name" ] || continue
    users_args+=("$name;$pass")
    valid+=("$name")
  done < "$NAS_USERS_FILE"

  local valid_csv; valid_csv="$(IFS=,; echo "${valid[*]}")"

  # OJO: no redefinir USER/SHARE acá (ni vaciarlos). El script de la imagen
  # trata una variable definida-pero-vacía como "hay que procesarla" y
  # revienta con "$2: unbound variable". Dejamos que el 'warden' del entorno
  # (definido en el compose base) siga viviendo, y el 'command' solo agrega
  # usuarios — llamarlo dos veces para 'warden' es inofensivo.
  {
    echo "services:"
    echo "  nas:"
    echo "    command:"
    local u
    for u in "${users_args[@]}"; do
      echo "      - \"-u\""
      echo "      - \"$u\""
    done
    echo "      - \"-s\""
    echo "      - \"warden;/share;yes;no;no;${valid_csv}\""
  } > "$NAS_OVERRIDE"
  chmod 600 "$NAS_OVERRIDE"

  warden_stack_install nas
}

# Reaplica el override actual (útil tras un fallo, sin re-agregar usuarios).
warden_nas_reload() { _nas_seed; _nas_apply; ok "NAS reaplicado"; }

warden_nas_adduser() {
  local name="$1" pass="${2:-$(openssl rand -hex 12)}"
  [ -n "$name" ] || { warn "Uso: warden nas adduser <nombre> [clave]"; return 1; }
  _nas_seed
  grep -q "^$name:" "$NAS_USERS_FILE" && { warn "'$name' ya existe (usá: warden nas passwd $name)"; return 1; }
  echo "$name:$pass" >> "$NAS_USERS_FILE"
  _nas_apply
  ok "Usuario '$name' creado. Clave: $pass"
}

warden_nas_passwd() {
  local name="$1" pass="${2:-$(openssl rand -hex 12)}"
  [ -n "$name" ] || { warn "Uso: warden nas passwd <nombre> [clave]"; return 1; }
  _nas_seed
  grep -q "^$name:" "$NAS_USERS_FILE" || { warn "No existe '$name' (usá: warden nas adduser)"; return 1; }
  sed -i "s/^$name:.*/$name:$pass/" "$NAS_USERS_FILE"
  _nas_apply
  ok "Clave de '$name' actualizada: $pass"
}

warden_nas_deluser() {
  local name="$1"
  [ -n "$name" ] || { warn "Uso: warden nas deluser <nombre>"; return 1; }
  [ "$name" = warden ] && { warn "No se puede borrar el usuario por defecto 'warden'"; return 1; }
  _nas_seed
  sed -i "/^$name:/d" "$NAS_USERS_FILE"
  _nas_apply
  ok "Usuario '$name' eliminado"
}

warden_nas_users() {
  _nas_seed
  cut -d: -f1 "$NAS_USERS_FILE"
}
