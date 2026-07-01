# warden

> Base opinionada y reutilizable para montar y mantener tu propio servidor casero
> (homelab): instalación desde cero, backup/restauración, dashboard y CI/CD,
> en **Debian/Ubuntu** o **Arch**. Todo tiene DOS caminos equivalentes: la
> **consola** (`warden ...`) o el **panel web** (`warden-panel`) —
> elegís el que te quede más cómodo en cada momento, ninguno es "el oficial".

`warden` convierte un Linux recién instalado en un homelab sólido y mantenible
con un comando. Los **datos** se respaldan con `restic` a un disco; la
**configuración** vive como código y es reproducible. Si toca recuperarte o
migrar a otro SO, el camino ya está trazado.

---

## Inicio rápido

```bash
git clone https://github.com/Aljodor21/warden.git
cd warden
mkdir -p site/catalog
cp examples/site.conf.example site/site.conf   # editá tus datos
sudo ./bootstrap.sh                             # elegí preset o a la carta
```

Al terminar, el comando `warden` queda en el PATH y el panel web corre en
`http://<tu-server>` (puerto 80).

---

## Flujo de uso

`bootstrap.sh` es el único paso que **siempre es consola** — instala el sistema
desde cero. Todo lo demás tiene camino equivalente por consola o por el panel:

```
Linux limpio
    └─▶ sudo ./bootstrap.sh          ← siempre desde consola (una sola vez)
             │
             ├─▶ Consola (warden …)  ← cualquier subcomando en cualquier momento
             │
             └─▶ Panel web :80       ← mismas acciones, con interfaz
                     ├─ Dashboard (salud del sistema)
                     ├─ Catálogo (apps + CI/CD)
                     ├─ Tienda (instalar apps en un click)
                     ├─ Archivos (FileBrowser)
                     ├─ Backups (backup/restore/timers)
                     └─ Sistema (VPN, Cloudflare, zona horaria, reset)
```

### Primer uso — secuencia típica

1. **Instalar desde cero** → `sudo ./bootstrap.sh` (elige preset o a la carta).
2. **Abrir el panel** → `http://<IP>` — candado de admin arriba a la derecha.
3. **Zona horaria** → Sistema → Conectividad → selector → Aplicar.
4. **VPN (Tailscale)** → Sistema → Conectividad → Conectar (o `sudo warden vpn`).
5. **Túnel Cloudflare** → Sistema → Conectividad → Configurar (o `sudo warden cloudflare-init`).
6. **Disco de backup** → Backups → Preparar disco → Guardar la clave de cifrado que aparece.
7. **Primer backup** → Backups → Hacer backup ahora (o `sudo warden backup`).
8. **Activar timer** → Backups → Activar (backup automático cada hora desde ahí).
9. **Agregar apps** → Tienda (un click) o Catálogo → Nueva app.

---

## Características

### Instalación

- `bootstrap.sh` **detecta la distro** (apt/pacman) y se adapta.
- **Presets** (`básico` / `completo`) o instalación **a la carta**.
- Menús de terminal con [`gum`](https://github.com/charmbracelet/gum).

| Preset | Instala |
|---|---|
| `básico` | Cockpit + warden-panel + NAS + shell (zsh/p10k) + MOTD + firewall |
| `completo` | `básico` + Backrest + ntfy + Immich (fotos) + Docmost (wiki) + Excalidraw |
| `a la carta` | elegís manualmente apps y módulos, uno por uno |

### Backup y restauración

- Backup **cifrado, versionado y deduplicado** con `restic` (corre en Docker,
  no hace falta instalarlo aparte).
- **Manual** (`warden backup` / botón en el panel) y **automático** (timer
  systemd cada hora + verificación nocturna con `restic check`).
- Disco de backup **interno o externo**, autodetectado por un marcador —
  nunca confunde el disco del sistema con el de backup.
- **Dumps de bases de datos** (PostgreSQL) desde el catálogo, junto con los
  archivos, en el mismo snapshot.
- **Restauración inteligente**: mira los `paths` del snapshot y los `.sql`,
  los cruza con el catálogo, e **instala sola** cualquier app que tenga datos
  en el backup pero no esté instalada. Las apps de CI/CD quedan avisadas (su
  deploy depende de GitHub Actions, no del restore). Datos sin receta se
  reportan, nunca se inventa una instalación. Al terminar, fija el disco en
  `/etc/fstab` por UUID y activa el timer automáticamente.

### Panel web

`warden-panel` es un servidor propio en **Go** (stdlib, sin frameworks) +
**HTMX** + **Alpine.js** vendorizados — sin CDN, sin paso de build, sin Node.
Pesa unos pocos MB de RAM. Un candado de admin por sesión del navegador protege
todo lo que cambia el sistema.

#### Dashboard

- Salud en vivo: CPU por núcleo, RAM, discos (donut SVG con porcentaje), red
  con **histograma de velocidad** de los últimos ~2 minutos (sparkline de bajada
  y subida).
- Procesos top CPU.
- Apps agrupadas: *instaladas por warden* vs *desplegadas vía CI/CD*, con
  estado real (corriendo/caída) y link directo si tienen subdominio.
- Herramientas del sistema (Cockpit, Backrest, ntfy) con sus URLs.
- Se refresca solo cada 3 segundos — no hace falta recargar la página.

#### Tienda

- Grilla de apps listas para instalar, tomadas de las plantillas de Portainer
  (más de 100 apps: Vaultwarden, Immich, Gitea, n8n, Nextcloud, etc.).
- **Instalar en un click** — detecta si la app ya tiene receta curada en warden
  y la usa directamente; si no, importa el compose y lo adapta al formato warden.
- Apps ya instaladas marcadas visualmente — no se pueden instalar dos veces.
- También acepta un **compose propio** (pegado o por URL) para lo que no esté
  en la grilla. Log de instalación en vivo, fijo en pantalla.

#### Catálogo

- Lista de apps con estado real (corriendo/caída), links directos y acciones.
- Alta de apps con formulario: nombre, tipo de backup, rutas de datos,
  subdominio Cloudflare (solo si el túnel está configurado), puerto.
- **Selector de puerto**: muestra los puertos que expone el contenedor y
  permite elegir con un click cuál usar para el link del dashboard.
- **Editor de docker-compose.yml** integrado: editá el compose de la app
  directamente desde el panel, sin terminal.
- Valida en vivo que el puerto no choque con otro.
- Al guardar con subdominio, **publica el túnel automáticamente**.
- **Registra el runner** pegando el token de GitHub (sin terminal), con log
  en vivo del proceso.
- **Elimina una app**: baja el contenedor, borra imágenes y volúmenes, regenera
  el túnel, borra el registro DNS de Cloudflare si guardaste el API Token.
- Instala apps del catálogo directamente desde la lista con log de progreso.

#### NAS

- Alta, cambio de clave y baja de usuarios de Samba, sin terminal.
- Recarga la config de Samba automáticamente al guardar.

#### Backups

- Discos detectados y su rol (SYSTEM / BACKUP / OTHER / EMPTY).
- Preparar disco vacío: formatea, inicializa el repositorio restic y **muestra
  la clave de cifrado** — guardala fuera del server, es lo único que no se puede
  recuperar.
- Montar/desmontar el disco desde el panel.
- Lista de snapshots con fecha (en tu zona horaria), antigüedad semáforo y
  tamaño.
- **Backup ahora** con log en vivo — el proceso sigue aunque cierres la página.
- **Restaurar desde una corrida específica** — elige el snapshot, ve el log
  en vivo, el proceso restaura archivos y BD de todas las apps de esa corrida.
- Timer automático: activar, ver próxima y última ejecución (en tu zona horaria).
- Ingresar la clave de cifrado si el repo fue creado en otro server.

#### Sistema

- **Zona horaria** — selector con zonas comunes (América Latina, Europa, otros),
  aplica en vivo con `timedatectl` y refleja la hora correcta en todos los
  timestamps del panel inmediatamente.
- **Tailscale (VPN)** — instalar y conectar con log en vivo de la URL de
  autorización; muestra IP Tailscale cuando está conectada.
- **Túnel Cloudflare** — configurar con streaming de la URL de login;
  muestra el dominio y las apps publicadas cuando está activo.
- **API Token de Cloudflare** — guardar para que "Eliminar app" borre el
  registro DNS automáticamente (opcional).
- **Llave `age`** — generar la llave de cifrado de secretos.
- **Respaldo de secretos** — exportar/actualizar las credenciales del túnel
  cifradas con `age`, guardadas en `site/secrets/`.
- **Runners de GitHub Actions** — lista de runners activos con su estado.
- **Consumo de RAM** — desglose: cuánto usa el panel propio y cada contenedor.
- **Zona de peligro** — botón "Eliminar sistema": borra todo lo que warden
  instaló/configuró (contenedores, datos, `/etc/warden`, túnel de Cloudflare
  *en tu cuenta*, Tailscale, firewall, paquetes). Pide escribir `BORRAR` en
  un campo de texto para habilitar el botón. Log en vivo mientras corre —
  el panel se apaga al final, eso es normal.

### El comando `warden`

```bash
warden   # menú principal interactivo
```

| Subcomando | Qué hace | Equivalente en panel |
|---|---|---|
| `status` | Discos, disco de backup, catálogo | Dashboard / Backups |
| `doctor` | Chequeo de salud (firewall, docker, discos, backup) | — |
| `install` | Lanza el instalador (`bootstrap.sh`) | — |
| `panel` | Instala/activa `warden-panel` | — |
| `backup` | Respalda todo el catálogo (restic) | Backups → Hacer backup ahora |
| `verify` | `restic check` (integridad del repositorio) | — |
| `register` | Fija el disco en `/etc/fstab` y activa los timers | Backups → Activar |
| `restore` | Restaura desde un disco de backup | Backups → Restaurar |
| `import <fuente> [tag]` | Importa un compose externo al formato warden | Tienda → Pegar compose |
| `install-component <tag>` | Instala un componente del catálogo | Tienda / Catálogo → Instalar |
| `publish` | Regenera el ingress de Cloudflare desde el catálogo | Automático al guardar app |
| `runner <url> <token>` | Registra un self-hosted runner de GitHub Actions | Catálogo → Registrar runner |
| `vpn` | Instala/conecta Tailscale | Sistema → Conectividad |
| `vpn exit-node on\|off` | Activa/desactiva este server como salida de internet | — |
| `vpn subnet on [CIDR]\|off` | Anuncia una subred por Tailscale | — |
| `cloudflare-init` | Crea el túnel de Cloudflare (primera vez) | Sistema → Conectividad |
| `cloudflare-reset` | Borra el túnel y la sesión para reconfigurar de cero | Sistema → Conectividad |
| `secrets <init\|save\|restore>` | Cifra/restaura secretos con `age` | Sistema → Credenciales |
| `reset` | Borra TODO lo que warden instaló/configuró | Sistema → Zona de peligro |
| `motd` | Instala el saludo al iniciar sesión | — |

### CI/CD

- Self-hosted runner de GitHub Actions, uno por repo (funciona tras CGNAT,
  sin IP pública). Se registra por consola o pegando el token en el panel.
- `warden publish`: regenera el `ingress` de Cloudflare Tunnel desde el
  catálogo y recarga `cloudflared` — sin tocar el dashboard de Cloudflare.
- Plantilla `deploy.yml` y estructura mínima de `Dockerfile`/`docker-compose.yml`
  listos en `examples/` para copiar en tu repo.

### Seguridad y robustez

- Firewall equilibrado (`ufw`), secretos cifrados con
  [`age`](https://github.com/FiloSottile/age) (escrow en `site/secrets/`).
- **Idempotente** (re-ejecutable sin romper nada) y modo **`--dry-run`**
  (`WARDEN_DRY_RUN=1`).
- `warden reset` pide escribir `BORRAR` literal para confirmar, tanto por
  consola como desde el panel.
- CI con `shellcheck` + `bash -n` en cada push y PR — nada llega a `main`
  sin pasar los checks.

### Extras

- `warden doctor` — chequeo de salud del sistema.
- MOTD con estado del server al hacer SSH.
- Shell: zsh + oh-my-zsh + powerlevel10k.

---

## Tecnologías

| Pieza | Con qué está hecha | Por qué |
|---|---|---|
| Instalador y `warden` (CLI) | Bash + [`gum`](https://github.com/charmbracelet/gum) | Cero runtime que instalar; corre en cualquier Linux con bash |
| `warden-panel` (dashboard) | Go (solo `stdlib`) | Un solo binario estático, pocos MB de RAM, sin runtime que mantener |
| Interactividad del panel | [HTMX](https://htmx.org) + [Alpine.js](https://alpinejs.dev), vendorizados | Cero CDN, cero paso de build/Node |
| Backup/restore | [`restic`](https://restic.net) corriendo en Docker | Cifrado, deduplicado, versionado; no hace falta instalarlo en el host |
| Apps y runner | Docker / `docker compose` | Aislamiento y reproducibilidad por app |
| Acceso remoto sin IP pública | [Cloudflare Tunnel](https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/) + [Tailscale](https://tailscale.com) | Funciona tras CGNAT, sin abrir puertos del router |
| CI/CD | Self-hosted runner de [GitHub Actions](https://docs.github.com/actions/hosting-your-own-runners) | El build/deploy corre en TU server |
| Secretos | [`age`](https://github.com/FiloSottile/age) | Cifrado simple y moderno, sin GPG |
| Firewall | `ufw` | Capa simple sobre `iptables`/`nftables` |

---

## Cómo funciona

El **catálogo** es la fuente de verdad. Cada componente se define **una sola
vez** (qué instalar, qué datos respaldar, cómo dumpear su BD, su hostname/puerto
en Cloudflare) y lo consumen el instalador, el backup/restore y el CI/CD.

- **Núcleo genérico** (este repo) ⇄ **config de tu sitio** (`site/`, ignorada
  por git). Tu servidor es *un caso*, no el código.
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
panel/           warden-panel — servidor Go del dashboard web
stacks/          docker-compose de cada app curada
catalog/         Recetas genéricas (cada quien suma las suyas en site/)
examples/        Plantillas: site.conf, componentes, deploy.yml, sudoers
restore/         Restauración (disaster recovery)
docs/            Documentación (CI/CD, prueba en VM…)
site/            TU configuración privada (ignorada por git)
```

## Reutilizable

1. `mkdir -p site/catalog && cp examples/site.conf.example site/site.conf` y editalo.
2. Por cada app tuya: `cp examples/catalog/app.component.example site/catalog/<app>.component`.
3. `sudo ./bootstrap.sh`.

Tu `site/` nunca se sube al repo; el programa se actualiza con `git pull`.

---

## Documentación

- [ROADMAP.md](ROADMAP.md) — fases del proyecto.
- [docs/CICD.md](docs/CICD.md) — despliegue continuo de tus apps.
- [docs/PRUEBA-VM.md](docs/PRUEBA-VM.md) — probar warden en una máquina virtual.
- [CONTRIBUTING.md](CONTRIBUTING.md) — cómo contribuir: CI, dry-run, módulos nuevos.

## Licencia

MIT.
