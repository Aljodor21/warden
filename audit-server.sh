#!/usr/bin/env bash
# Auditoría root del servidor — solo lectura, no cambia nada.
set +e
line(){ echo; echo "########## $1 ##########"; }

line "1) PLACA BASE / BIOS"
dmidecode -t baseboard 2>/dev/null | grep -E "Manufacturer|Product Name|Version" | head
dmidecode -t bios 2>/dev/null | grep -E "Vendor|Version|Release Date" | head

line "2) MEMORIA RAM (modulos y slots)"
dmidecode -t memory 2>/dev/null | grep -E "Size|Type:|Speed|Locator|Manufacturer|Part Number" | grep -vE "No Module|Unknown|Error" | head -40

line "3) SALUD DISCO SSD (sda)"
smartctl -H -i -A /dev/sda 2>/dev/null | grep -iE "Model|Serial|Capacity|SMART overall|Power_On_Hours|Wear|Reallocated|Pending|Temperature|Percent" | head -30

line "4) SALUD DISCO USB (sdb)"
smartctl -H -i -A -d sat /dev/sdb 2>/dev/null | grep -iE "Model|Serial|Capacity|SMART overall|Power_On_Hours|Reallocated|Pending|Temperature" | head -20
echo "(si arriba sale vacio, el USB no expone SMART por el puente USB; es normal)"

line "5) TEMPERATURAS"
sensors 2>/dev/null | grep -iE "Core|Package|temp" | head -10 || echo "(lm-sensors no instalado)"

line "6) FIREWALL (ufw)"
ufw status verbose 2>/dev/null || echo "(ufw no responde)"

line "7) PUERTOS -> PROCESO (quien escucha cada puerto)"
ss -tlnp 2>/dev/null | awk 'NR==1 || /LISTEN/' | sed -E 's/users:\(\("([^"]+)".*pid=([0-9]+).*/\1 pid=\2/' | sort -u

line "8) USUARIOS SAMBA"
pdbedit -L 2>/dev/null || echo "(sin usuarios samba / solo guest)"

line "9) USO DE DISCO EN /home"
du -sh /home/* 2>/dev/null | sort -rh | head

line "10) USB conectados (lsusb) y discos fisicos"
lsusb 2>/dev/null | grep -viE "hub|root" | head
echo "---"
smartctl --scan 2>/dev/null

line "11) AUTOSTART / unidades habilitadas no-estandar"
systemctl list-unit-files --state=enabled 2>/dev/null | grep -iE "cloudflared|tailscale|docker|casaos|rclone|smb|nmb|ssh"

echo
echo "########## FIN AUDITORIA ##########"
