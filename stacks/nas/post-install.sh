#!/usr/bin/env bash
# Mensaje de conexión tras instalar el NAS genérico.
host="$(hostname).local"
ip="$(hostname -I 2>/dev/null | awk '{print $1}')"
pass_file="/etc/warden/secrets/nas.env"
pass="$(grep '^NAS_PASSWORD=' "$pass_file" 2>/dev/null | cut -d= -f2)"
echo
echo "   Cómo conectarte al NAS:"
echo "     Windows/Linux:  smb://${host}/warden"
echo "     macOS (Finder > Ir > Conectarse al servidor): smb://${host}/warden"
echo "     (si tu red no resuelve .local, usá la IP: smb://${ip:-<ip-del-server>}/warden)"
echo "     Usuario: warden"
echo "     Contraseña: ${pass:-<ver $pass_file>}"
