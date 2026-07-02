# Instalación

## Requisitos

- Debian 12+ / Ubuntu 22.04+ / Arch Linux (instalación base)
- Acceso root o sudo
- Git instalado (`apt install git` / `pacman -S git`)
- Conexión a internet

## Pasos

### 1. Clonar el repo

```bash
git clone https://github.com/Aljodor21/warden.git
cd warden
sudo ./bootstrap.sh
```

Eso es todo. El instalador hace el resto.

### 2. Responder las preguntas iniciales

La primera vez que corre, el instalador te pide unos datos básicos sobre tu server:

- **Nombre del server** — se detecta automáticamente desde el hostname, podés cambiarlo
- **Zona horaria** — lista interactiva, solo elegís la tuya
- **Subred de tu LAN** — para las reglas de firewall (ej: `192.168.1.0/24`)

Estos datos se guardan en `site/site.conf` y no se vuelven a pedir.

### 3. Elegir un preset

=== "Preset básico"
    Docker, warden-panel, Cockpit, NAS (Samba), shell (zsh + p10k), MOTD, firewall (ufw).

    Ideal para empezar rápido con un server liviano.

=== "Preset completo"
    Todo lo del básico más: Backrest, ntfy (alertas push), Immich (fotos), Docmost (wiki), Excalidraw.

=== "A la carta"
    Elegís manualmente cada módulo y app desde un menú interactivo.

El preset instala todo y registra las apps en el catálogo automáticamente — no hay que tocar ningún archivo.

### 4. Abrir el panel

Al terminar, el panel queda disponible en:

```
http://<IP-del-server>
```

El candado **Admin** (arriba a la derecha) desbloquea las acciones que modifican el sistema.

---

## Instalar más apps después

Una vez instalado warden, podés agregar más apps en cualquier momento desde la **Tienda** en el panel — sin terminal, sin editar archivos.

---

## Actualizar warden

```bash
cd ~/warden
git pull
sudo warden panel   # recompila e instala el panel
```

---

## Probar en una VM

Si querés probar antes de instalar en hardware real, ver [Prueba en VM](https://github.com/Aljodor21/warden/blob/main/docs/PRUEBA-VM.md).
