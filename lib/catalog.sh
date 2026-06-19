#!/usr/bin/env bash
# lib/catalog.sh — carga el catálogo declarativo de componentes.
#
# Recetas genéricas del repo (catalog/) + overrides/extras de tu sitio
# (site/catalog/). Si un tag existe en ambos, gana el de site/.
# El catálogo es la fuente de verdad: lo usan el instalador (bootstrap),
# el backup/restore (warden) y el CI/CD (cloudflare).
#
# NOTA: ningún .component guarda contraseñas. Las credenciales de BD se leen
# del propio contenedor en tiempo de ejecución (docker inspect / printenv).

_CAT_LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
_ROOT="$(cd "$_CAT_LIB_DIR/.." && pwd)"

# Para catalog_each: repo primero, site después (site sobreescribe).
CATALOG_DIRS=()
[ -d "$_ROOT/catalog" ]      && CATALOG_DIRS+=("$_ROOT/catalog")
[ -d "$_ROOT/site/catalog" ] && CATALOG_DIRS+=("$_ROOT/site/catalog")

# catalog_each: una línea por componente => tag|nombre|tipo|origen_install
catalog_each() {
  [ "${#CATALOG_DIRS[@]}" -gt 0 ] || return 0
  declare -A seen; local order=() d f tag
  for d in "${CATALOG_DIRS[@]}"; do
    for f in "$d"/*.component; do
      [ -e "$f" ] || continue
      tag="$(. "$f"; echo "$COMP_TAG")"
      [ -n "${seen[$tag]+x}" ] || order+=("$tag")
      seen[$tag]="$f"          # último gana (site sobreescribe al repo)
    done
  done
  for tag in "${order[@]}"; do
    ( COMP_NAME=""; COMP_TAG=""; COMP_KIND=""; COMP_INSTALL=""
      source "${seen[$tag]}"
      printf '%s|%s|%s|%s\n' "$COMP_TAG" "$COMP_NAME" "$COMP_KIND" "${COMP_INSTALL:-—}" )
  done
}

# Limpia todas las variables COMP_* antes de cargar un componente, para que
# un componente que no define algo (ej. COMP_SECRETS) no herede el valor del
# componente cargado anteriormente en el mismo shell.
_catalog_reset() {
  COMP_NAME=""; COMP_TAG=""; COMP_KIND=""; COMP_INSTALL=""
  COMP_CONTAINER=""; COMP_ICON=""; COMP_NOTE=""
  COMP_DB_TYPE=""; COMP_DB_CONTAINER=""; COMP_DB_NAME=""; COMP_DB_USER=""
  COMP_CF_HOST=""; COMP_CF_PORT=""
  COMP_PATHS=(); COMP_EXCLUDES=(); COMP_SECRETS=()
}

# catalog_load <tag>: carga (source) un componente. site/ tiene prioridad.
catalog_load() {
  local want="$1" d f t dirs=()
  [ -d "$_ROOT/site/catalog" ] && dirs+=("$_ROOT/site/catalog")
  [ -d "$_ROOT/catalog" ]      && dirs+=("$_ROOT/catalog")
  [ "${#dirs[@]}" -gt 0 ] || return 1
  for d in "${dirs[@]}"; do
    for f in "$d"/*.component; do
      [ -e "$f" ] || continue
      t="$(. "$f"; echo "$COMP_TAG")"
      [ "$t" = "$want" ] && { _catalog_reset; source "$f"; return 0; }
    done
  done
  return 1
}
