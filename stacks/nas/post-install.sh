#!/usr/bin/env bash
# Tras instalar el NAS: si es la primera vez (no hay override todavía),
# sembramos el usuario por defecto y aplicamos el override (única fuente de
# verdad de usuarios/share). Después, mostramos cómo conectarse.
set -u
ROOT="${WARDEN_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)}"

if [ ! -f /etc/warden/nas/docker-compose.override.yml ]; then
  # shellcheck source=/dev/null
  source "$ROOT/lib/core.sh"
  # shellcheck source=/dev/null
  source "$ROOT/lib/distro.sh"
  # shellcheck source=/dev/null
  source "$ROOT/lib/catalog.sh"
  # shellcheck source=/dev/null
  source "$ROOT/modules/stacks.sh"
  # shellcheck source=/dev/null
  source "$ROOT/modules/nas.sh"
  _nas_seed
  _nas_apply
  exit 0   # la propia _nas_apply ya reinstaló y volvió a correr este script
fi

host="$(hostname).local"
ip="$(hostname -I 2>/dev/null | awk '{print $1}')"
echo
echo "   Cómo conectarte al NAS:"
echo "     Windows/Linux:  smb://${host}/warden"
echo "     macOS (Finder > Ir > Conectarse al servidor): smb://${host}/warden"
echo "     (si tu red no resuelve .local, usá la IP: smb://${ip:-<ip-del-server>}/warden)"
echo "     Usuarios:"
cut -d: -f1 /etc/warden/nas/users.txt 2>/dev/null | sed 's/^/       - /'
echo "     Ver las claves: sudo warden nas users -v"
