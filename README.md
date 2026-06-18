# warden

> Base opinionada y reutilizable para montar y mantener tu propio servidor casero
> (homelab): instalación desde cero, backup/restauración, dashboard y CI/CD —
> todo desde la consola, en **Debian/Ubuntu** o **Arch**.

`warden` convierte un Linux recién instalado en un homelab sólido y mantenible
con un comando. Los **datos** se respaldan con `restic` a un disco; la
**configuración** vive como código y es reproducible. Si toca recuperarte o
migrar a otro SO, el camino está trazado.

---

## Características

**Instalación**
- Instalador `bootstrap.sh` que **detecta la distro** (apt/pacman) y se adapta.
- **Presets** (`minimal` / `media` / `dev`) o instalación **a la carta**.
- Menús de terminal con [`gum`](https://github.com/charmbracelet/gum).

**Backup y restauración**
- Backup **cifrado, versionado y deduplicado** con `restic`.
- **Manual** (`warden backup`) y **automático** (timer cada hora + verificación nocturna).
- Disco de backup **interno o externo**, autodetectado (nunca el del sistema).
- **Dumps de bases de datos** generados desde el catálogo.
- **Restauración** (disaster recovery) desde el disco, por componente.

**Dashboard (un solo frente)**
- [Homepage](https://gethomepage.dev) como cara única, **generada desde el catálogo**.
- [Cockpit](https://cockpit-project.org) (sistema), [Backrest](https://github.com/garethgeorge/backrest)
  (UI de backups) y [ntfy](https://ntfy.sh) (alertas push).

**CI/CD**
- Self-hosted runner de GitHub Actions (funciona tras CGNAT).
- `warden publish`: regenera el ingress de **Cloudflare Tunnel** desde el catálogo.
- Plantilla `deploy.yml` lista para tus repos de apps.

**Seguridad y robustez**
- Firewall equilibrado (`ufw`), secretos cifrados con [`age`](https://github.com/FiloSottile/age) (escrow).
- **Idempotente** (re-ejecutable sin romper) y modo **`--dry-run`** (`WARDEN_DRY_RUN=1`).

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

## El comando `warden`

```bash
warden                 # menú principal (interactivo)
```

| Subcomando | Qué hace |
|---|---|
| `status` | Discos, disco de backup, catálogo |
| `doctor` | Chequeo de salud (firewall, docker, discos, backup) |
| `install` | Lanza el instalador (`bootstrap.sh`) |
| `backup` | Respalda todo el catálogo (restic) |
| `verify` | `restic check` (integridad del repositorio) |
| `register` | Fija el disco en `/etc/fstab` y activa los timers |
| `restore` | Restaura desde un disco de backup |
| `publish` | Regenera el ingress de Cloudflare desde el catálogo |
| `runner <url> <token>` | Registra un self-hosted runner de GitHub Actions |
| `secrets <init\|save\|restore>` | Cifra/restaura secretos con `age` |
| `motd` | Instala el saludo al iniciar sesión |

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
