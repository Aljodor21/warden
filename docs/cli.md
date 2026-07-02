# CLI — comando `warden`

```bash
warden          # abre el menú interactivo principal
warden <cmd>    # ejecutar un subcomando directamente
```

## Referencia completa

| Subcomando | Qué hace | Equivalente en panel |
|---|---|---|
| `status` | Discos, disco de backup, catálogo | Dashboard / Backups |
| `doctor` | Chequeo de salud: firewall, docker, discos, backup | — |
| `install` | Lanza el instalador (`bootstrap.sh`) | — |
| `panel` | Instala/recompila/activa `warden-panel` | — |
| `backup` | Respalda todo el catálogo (restic) | Backups → Hacer backup ahora |
| `verify` | `restic check` (integridad del repositorio) | — |
| `register` | Fija el disco en `/etc/fstab` y activa los timers | Backups → Activar |
| `restore` | Restaura desde un disco de backup | Backups → Restaurar |
| `import <fuente> [tag]` | Importa un compose externo al formato warden | Tienda → Pegar compose |
| `install-component <tag>` | Instala un componente del catálogo | Tienda / Catálogo → Instalar |
| `publish` | Regenera el ingress de Cloudflare desde el catálogo | Automático al guardar app |
| `runner <url> <token>` | Registra un self-hosted runner de GitHub Actions | Catálogo → Registrar runner |
| `vpn` | Instala y conecta Tailscale | Sistema → VPN |
| `vpn exit-node on\|off` | Activa/desactiva este server como salida de internet | — |
| `vpn subnet on [CIDR]\|off` | Anuncia una subred por Tailscale | — |
| `cloudflare-init` | Crea el túnel de Cloudflare (primera vez) | Sistema → Cloudflare |
| `cloudflare-reset` | Borra el túnel y la sesión para reconfigurar | Sistema → Cloudflare |
| `secrets init` | Genera la llave age de cifrado | Sistema → Secretos |
| `secrets save` | Cifra y guarda credenciales del túnel | Sistema → Secretos |
| `secrets restore` | Restaura credenciales cifradas | Sistema → Secretos |
| `ntfy` | Instala y configura el servidor de alertas push | Sistema → ntfy |
| `motd` | Instala el saludo al iniciar sesión SSH | — |
| `reset` | Borra TODO lo que warden instaló/configuró | Sistema → Zona de peligro |

## Variables de entorno

| Variable | Efecto |
|---|---|
| `WARDEN_DRY_RUN=1` | Modo simulación — muestra qué haría sin ejecutar nada |

## Menú interactivo

`warden` sin argumentos abre un TUI con `gum` donde podés navegar todas las opciones con flechas y Enter. Útil para explorar sin recordar los subcomandos.

## Estructura del binario

```
bin/warden          Script principal — menú + despacho de subcomandos
lib/                Núcleo: distro, UI (gum), catálogo, presets, helpers
modules/            Una pieza por función: docker, backup, cloudflare, vpn…
```
