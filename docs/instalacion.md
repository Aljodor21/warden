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
```

### 2. Configurar tu sitio

```bash
mkdir -p site/catalog
cp examples/site.conf.example site/site.conf
```

Editá `site/site.conf` con tus datos:

```bash
# Nombre de tu server (solo letras/números/guiones)
SITE_NAME=miserver

# Usuario principal del sistema
SITE_USER=tu_usuario

# Zona horaria (ej: America/Bogota, America/Argentina/Buenos_Aires)
SITE_TZ=America/Bogota
```

### 3. Correr el instalador

```bash
sudo ./bootstrap.sh
```

El instalador detecta tu distro y te pregunta qué instalar:

=== "Preset básico"
    Instala: Docker, warden-panel, Cockpit, NAS (Samba), shell (zsh + p10k), MOTD, firewall (ufw).
    
    Ideal para empezar rápido.

=== "Preset completo"
    Todo lo del básico más: Backrest, ntfy (alertas push), Immich (fotos), Docmost (wiki), Excalidraw.

=== "A la carta"
    Elegís manualmente cada módulo y app.

### 4. Abrir el panel

Una vez terminado el bootstrap, el panel queda en:

```
http://<IP-del-server>
```

El candado **Admin** (arriba a la derecha) desbloquea las acciones que modifican el sistema. La clave es la que configuraste en `site.conf` o la que te pidió el instalador.

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

---

## Agregar tus propias apps

Por cada app tuya, creá un archivo en `site/catalog/`:

```bash
cp examples/catalog/app.component.example site/catalog/miapp.component
```

Editá los campos necesarios (`COMP_NAME`, `COMP_CONTAINER`, `COMP_PATHS`, etc.) y la app aparece en el Catálogo. Ver [Catálogo](panel/catalogo.md) para más detalle.
