# NAS

La sección NAS gestiona los usuarios de **Samba** (compartición de archivos en red local) sin tocar la terminal.

## Acciones

| Acción | Descripción |
|---|---|
| Agregar usuario | Crea el usuario del sistema y lo registra en Samba |
| Cambiar clave | Actualiza la contraseña de Samba del usuario |
| Eliminar usuario | Borra el usuario de Samba (no elimina el usuario del sistema) |

Cada acción recarga la configuración de Samba automáticamente al guardar.

## Conectar desde otro equipo

=== "Windows"
    Abrí el Explorador de archivos → barra de dirección → `\\<IP-del-server>`

=== "macOS"
    Finder → Ir → Conectarse al servidor → `smb://<IP-del-server>`

=== "Linux"
    ```bash
    # Montar temporalmente
    sudo mount -t cifs //<IP>/warden /mnt/warden -o user=<usuario>
    ```

!!! tip "IP fija"
    Para que la dirección no cambie, asigná una IP estática al server desde tu router (reserva DHCP por MAC).

## Acceso por VPN

Si tenés Tailscale configurado, podés acceder al NAS desde cualquier lugar usando la IP Tailscale del server (`100.x.x.x`).

```bash
# CLI — ver usuarios actuales
sudo pdbedit -L
```
