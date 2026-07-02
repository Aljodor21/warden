# warden

> Base opinionada y reutilizable para montar y mantener tu propio servidor casero (homelab): instalación desde cero, backup/restauración, panel web y CI/CD — en **Debian/Ubuntu** o **Arch**, con un solo comando.

![Dashboard de warden](screenshots/screenshot-dashboard.png)

warden convierte un Linux recién instalado en un homelab sólido y mantenible. Los **datos** se respaldan con `restic` a un disco; la **configuración** vive como código y es reproducible. Si toca recuperarte o migrar a otro SO, el camino ya está trazado.

Todo tiene **dos caminos equivalentes**: la consola (`warden ...`) o el panel web — elegís el que te quede más cómodo en cada momento, ninguno es "el oficial".

---

## Inicio rápido

```bash
git clone https://github.com/Aljodor21/warden.git
cd warden
mkdir -p site/catalog
cp examples/site.conf.example site/site.conf   # editá tus datos
sudo ./bootstrap.sh                             # elegí preset o a la carta
```

Al terminar, `warden` queda en el PATH y el panel web corre en `http://<tu-server>` (puerto 80).

---

## Qué incluye

| Sección | Qué hace |
|---|---|
| [Dashboard](panel/dashboard.md) | Salud del sistema en vivo: CPU, RAM, discos, red, apps |
| [Catálogo](panel/catalogo.md) | Gestión de apps instaladas — editar, ver logs, eliminar |
| [Tienda](panel/tienda.md) | +100 apps listas para instalar en un click |
| [Backups](panel/backups.md) | Backup cifrado y versionado con restic, restauración inteligente |
| [Sistema](panel/sistema.md) | VPN, Cloudflare Tunnel, alertas push, runners CI/CD |
| [NAS](panel/nas.md) | Usuarios Samba sin terminal |
| [Apariencia](panel/apariencia.md) | Temas, fondos de pantalla, efecto vidrio ajustable |
| [CLI](cli.md) | Todos los subcomandos de `warden` |
| [CI/CD](cicd.md) | Deploy automático desde GitHub Actions a tu server |

---

## Por qué warden

- **Sin IP pública**: funciona tras CGNAT con Cloudflare Tunnel + Tailscale.
- **Sin runtime extra**: panel en Go (binario único), restic en Docker, cero Node.
- **Reproducible**: si el server muere, `git clone + bootstrap.sh` lo reconstruye.
- **Moderno pero liviano**: hardware modesto (8 GB RAM) funciona perfecto.
- **Código abierto**: MIT, sin telemetría, sin cuentas, todo local.
