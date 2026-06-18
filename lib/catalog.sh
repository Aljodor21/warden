#!/usr/bin/env bash
# lib/catalog.sh — carga el catálogo declarativo de componentes.
#
# Cada componente vive en catalog/<algo>.component y define variables COMP_*.
# El catálogo es la ÚNICA fuente de verdad: lo usan el instalador (bootstrap),
# el backup/restore (uburoom-backup) y el CI/CD (cloudflare).
#
# NOTA: ningún .component guarda contraseñas. Las credenciales de BD se leen
# del propio contenedor en tiempo de ejecución (docker inspect / printenv).

_CAT_LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# El catálogo real vive en site/catalog (tu config, ignorada por git).
# Si no existe (clon nuevo), cae a las plantillas de examples/catalog.
if [ -z "${CATALOG_DIR:-}" ]; then
  if [ -d "$_CAT_LIB_DIR/../site/catalog" ]; then
    CATALOG_DIR="$_CAT_LIB_DIR/../site/catalog"
  else
    CATALOG_DIR="$_CAT_LIB_DIR/../examples/catalog"
  fi
fi

# catalog_each: emite una línea por componente => tag|nombre|tipo|origen_install
catalog_each() {
  local f
  for f in "$CATALOG_DIR"/*.component; do
    [ -e "$f" ] || continue
    (
      COMP_NAME=""; COMP_TAG=""; COMP_KIND=""; COMP_INSTALL=""
      # shellcheck source=/dev/null
      source "$f"
      printf '%s|%s|%s|%s\n' "$COMP_TAG" "$COMP_NAME" "$COMP_KIND" "${COMP_INSTALL:-—}"
    )
  done
}

# catalog_load <tag>: carga (source) un componente por su tag en el shell actual.
catalog_load() {
  local want="$1" f
  for f in "$CATALOG_DIR"/*.component; do
    [ -e "$f" ] || continue
    local t; t="$(. "$f"; echo "$COMP_TAG")"
    if [ "$t" = "$want" ]; then
      # shellcheck source=/dev/null
      source "$f"; return 0
    fi
  done
  return 1
}
