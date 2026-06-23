# warden

> Base opinionada y reutilizable para montar y mantener tu propio servidor casero
> (homelab): instalación desde cero, backup/restauración, dashboard y CI/CD,
> en **Debian/Ubuntu** o **Arch**. Todo tiene DOS caminos equivalentes: la
> **consola** (`warden ...`) o el **panel web** (`warden-panel`, sin
> consola) — elegís el que te quede más cómodo en cada momento, ninguno es
> "el oficial", hacen exactamente lo mismo por debajo.

`warden` convierte un Linux recién instalado en un homelab sólido y mantenible
con un comando. Los **datos** se respaldan con `restic` a un disco; la
**configuración** vive como código y es reproducible. Si toca recuperarte o
migrar a otro SO, el camino está trazado.

---

## Características

**Instalación**
- Instalador `bootstrap.sh` que **detecta la distro** (apt/pacman) y se adapta.
- **Presets** (`básico` / `completo`) o instalación **a la carta**.
- Menús de terminal con [`gum`](https://github.com/charmbracelet/gum).

**Backup y restauración**
- Backup **cifrado, versionado y deduplicado** con `restic` (corre en Docker, no hace falta instalarlo aparte).
- **Manual** (`warden backup` o el botón "Hacer backup ahora" del panel) y **automático**
  (timer systemd cada hora + verificación nocturna con `restic check`).
- Disco de backup **interno o externo**, autodetectado por un archivo marcador (nunca el del sistema).
- **Dumps de bases de datos** (PostgreSQL) generados desde el catálogo, junto con los archivos.
- **Restauración inteligente**: no es una lista fija — mira los `paths` reales dentro del snapshot
  y los nombres de los `.sql`, los cruza con TODO el catálogo, e **instala sola** (con su receta)
  cualquier app que tenga datos en el backup pero no esté instalada — sin preguntar, si hay un
  backup de algo es porque hay que restaurarlo. Las apps de CI/CD (su propio repo) no se pueden
  instalar solas, quedan avisadas; datos sin receta en ningún catálogo se reportan, nunca se inventa
  una instalación. Al terminar, deja el mismo disco anotado para que el backup automático lo siga
  usando. Funciona igual desde consola (`warden restore`) o desde el botón "Restaurar ahora" en
  Backups del panel (streaming del log en vivo, sin que te cuelgues esperando nada).

**Dashboard (un solo frente, cero dependencias de Node/build)**

`warden-panel` es un servidor propio en **Go** (stdlib, sin frameworks) + **HTMX** y
**Alpine.js** vendorizados (sin CDN, sin paso de build) — pesa unos pocos MB de RAM. Cinco páginas:

- **Dashboard**: salud en vivo (CPU/RAM/discos/red), apps agrupadas en *instaladas por warden* vs
  *desplegadas vía CI/CD*, con su estado real (corriendo/caída) y link directo si tienen subdominio.
- **Catálogo**: alta de apps con un formulario corto enfocado 100% en CI/CD (tu repo, puerto,
  subdominio) — valida en vivo que el puerto no choque con otro, que el subdominio solo se ofrezca
  si hay un túnel de Cloudflare configurado, y detecta si ya existe un runner para ese repo. Al
  guardar con subdominio, publica el túnel automáticamente (sin que haya que acordarse del botón
  "Publicar" después). Desde ahí también se **registra el runner pegando el token** (sin consola) y
  se **elimina una app** (borra su entrada, regenera el túnel, y si guardaste un Token de Cloudflare,
  borra también el registro DNS — si no lo guardaste, te lo pide ahí mismo, con la opción de seguir
  sin tocarlo si no querés hacerlo en ese momento).
- **NAS**: usuarios de Samba, alta/baja/cambio de clave, sin tocar una terminal.
- **Sistema**: VPN (Tailscale, con login por navegador), túnel de Cloudflare (configuración inicial
  con streaming de la URL de login, todo opcional — solo hace falta si vas a exponer algo a
  internet), API Token de Cloudflare (opcional, solo para borrar registros DNS), y la lista
  real de runners de GitHub Actions activos (warden no guarda ningún token de GitHub permanente —
  el de registro se usa una vez y vence en minutos).
- **Backups**: discos detectados y su rol, snapshots con fecha legible y semáforo de antigüedad,
  timer automático con su próxima ejecución, botón de backup manual, y el botón de Restaurar
  descrito arriba.

Un candado de admin **por sesión del navegador** (no por acción) protege todo lo que cambia el
sistema — se pide una vez, se cierra al cerrar el navegador.

Además, sin pisarse con lo de arriba: [Cockpit](https://cockpit-project.org) (sistema a fondo),
[Backrest](https://github.com/garethgeorge/backrest) (UI de backups) y [ntfy](https://ntfy.sh)
(alertas push) — herramientas pro ya hechas, enlazadas, no reinventadas.

**CI/CD**
- Self-hosted runner de GitHub Actions, uno por repo (funciona tras CGNAT, sin IP pública).
  Se registra por consola (`warden runner <url> <token>`) o pegando el token en el panel.
- `warden publish`: regenera el `ingress` de **Cloudflare Tunnel** desde el catálogo (qué subdominio
  va a qué puerto local) y recarga `cloudflared` — sin tocar el dashboard de Cloudflare a mano.
- Plantilla `deploy.yml` y la estructura mínima de `Dockerfile`/`docker-compose.yml` (con la
  variable de entorno `PORT` ya incluida) listas para copiar en tu repo.

**Seguridad y robustez**
- Firewall equilibrado (`ufw`), secretos cifrados con [`age`](https://github.com/FiloSottile/age) (escrow).
- **Idempotente** (re-ejecutable sin romper) y modo **`--dry-run`** (`WARDEN_DRY_RUN=1`).
- `warden reset`: borra TODO lo que warden instaló/configuró (contenedores, datos, `/etc/warden`,
  el túnel de Cloudflare *en tu cuenta* no solo local, la conexión de Tailscale, y deja `ufw`
  desactivado) — deja el sistema operativo limpio, como antes de instalar warden. No toca el disco
  de backup externo ni los paquetes del sistema. Pide escribir `BORRAR` literal para confirmar.

**Extras**
- `warden doctor` (chequeo de salud), MOTD con estado del server, shell
  (zsh + oh-my-zsh + powerlevel10k).

---

## Inicio rápido

```bash
git clone https://github.com/Aljodor21/warden.git
cd warden
mkdir -p site/catalog
cp examples/site.conf.example site/site.conf      # editá tus datos
sudo ./bootstrap.sh                               # elegí preset o a la carta
```

Al terminar, `warden` queda disponible en el `PATH`.

## Presets

Al instalar, `bootstrap.sh` te pregunta el **modo**:

| Preset | Instala |
|---|---|
| `básico` | Cockpit + warden-panel + NAS + shell (zsh/p10k) + MOTD + firewall |
| `completo` | `básico` + Backrest + ntfy + Immich (fotos) + Docmost (wiki) + Excalidraw |
| `a la carta` | elegís manualmente apps y módulos, uno por uno |

Se definen en [`lib/presets.sh`](lib/presets.sh) — fáciles de editar o de sumar el tuyo.

## El comando `warden`

```bash
warden                 # menú principal (interactivo)
```

| Subcomando | Qué hace |
|---|---|
| `status` | Discos, disco de backup, catálogo |
| `doctor` | Chequeo de salud (firewall, docker, discos, backup) |
| `install` | Lanza el instalador (`bootstrap.sh`) |
| `panel` | Instala/activa `warden-panel` (el dashboard web) |
| `backup` | Respalda todo el catálogo (restic) |
| `verify` | `restic check` (integridad del repositorio) |
| `register` | Fija el disco en `/etc/fstab` y activa los timers |
| `restore` | Restaura desde un disco de backup (instala sola lo que falte) |
| `publish` | Regenera el ingress de Cloudflare desde el catálogo |
| `runner <url> <token>` | Registra un self-hosted runner de GitHub Actions |
| `secrets <init\|save\|restore>` | Cifra/restaura secretos con `age` |
| `reset` | Borra TODO lo que warden instaló/configuró (pide confirmar `BORRAR`) |
| `motd` | Instala el saludo al iniciar sesión |

Cada uno de estos (salvo `reset`, que es deliberadamente solo de consola — es
destructivo y necesita la confirmación explícita escrita) tiene su
equivalente como botón/página en `warden-panel`.

## Tecnologías

| Pieza | Con qué está hecha | Por qué |
|---|---|---|
| Instalador y `warden` (CLI) | Bash + [`gum`](https://github.com/charmbracelet/gum) para menús | Cero runtime que instalar; corre en cualquier Linux con bash |
| `warden-panel` (dashboard) | Go (solo `stdlib`, sin frameworks) | Un solo binario estático, unos pocos MB de RAM, sin runtime que mantener |
| Interactividad del panel | [HTMX](https://htmx.org) + [Alpine.js](https://alpinejs.dev), vendorizados | Cero CDN, cero paso de build/Node — los `.js` viven en el repo |
| Backup/restore | [`restic`](https://restic.net) (corriendo en Docker) | Cifrado, deduplicado, versionado; no hace falta instalarlo en el host |
| Apps y runner de CI/CD | Docker / `docker compose` | Aislamiento y reproducibilidad por app |
| Acceso remoto sin IP pública | [Cloudflare Tunnel](https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/) + [Tailscale](https://tailscale.com) | Funciona tras CGNAT, sin abrir puertos del router |
| CI/CD | Self-hosted runner de [GitHub Actions](https://docs.github.com/actions/hosting-your-own-runners) | El build/deploy corre en TU server, no en la nube de GitHub |
| Secretos | [`age`](https://github.com/FiloSottile/age) | Cifrado simple y moderno, sin GPG |
| Firewall | `ufw` | Capa simple sobre `iptables`/`nftables` |

## Cómo funciona

El **catálogo** es la fuente de verdad. Cada componente se define **una sola
vez** (qué instalar, qué datos respaldar, cómo dumpear su BD, su hostname/puerto
en Cloudflare) y lo consumen el instalador, el backup/restore y el CI/CD.

- **Núcleo genérico** (este repo) ⇄ **config de tu sitio** (`site/`, ignorada por git).
  Tu servidor es *un caso*, no el código.
- **Sin contraseñas en el repo**: las credenciales de BD se leen del contenedor
  en tiempo de ejecución; los accesos (GitHub, Cloudflare) se piden cuando se
  necesitan y quedan locales.
- **Multi-distro**: una capa de abstracción mapea apt ⇄ pacman.

## Estructura

```
bootstrap.sh     Instalador (detecta distro, presets, menú)
bin/warden       Comando unificado (menú + subcomandos)
lib/             Núcleo: distro, UI (gum), catálogo, presets, helpers
modules/         Una pieza por función (docker, cockpit, backup, cloudflare…)
stacks/          docker-compose de cada app curada
catalog/         Recetas genéricas (cada quien suma las suyas en site/)
examples/        Plantillas: site.conf, componentes, deploy.yml, sudoers
restore/         Restauración (disaster recovery)
docs/            Documentación (CI/CD, prueba en VM…)
site/            TU configuración privada (ignorada por git)
```

## Reutilizable

Para usar warden en tu propio server:

1. `mkdir -p site/catalog && cp examples/site.conf.example site/site.conf` y editalo (nombre, LAN, zona horaria).
2. Por cada app tuya: `cp examples/catalog/app.component.example site/catalog/<app>.component`.
3. `sudo ./bootstrap.sh`.

Tu `site/` nunca se sube al repo; el programa se actualiza con `git pull`.

## Documentación

- [ROADMAP.md](ROADMAP.md) — fases del proyecto.
- [docs/CICD.md](docs/CICD.md) — despliegue continuo de tus apps.
- [docs/PRUEBA-VM.md](docs/PRUEBA-VM.md) — probar warden en una máquina virtual.

## Licencia

MIT.
