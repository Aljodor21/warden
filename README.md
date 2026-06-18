# warden

Una base **opinionada y reutilizable** para montar tu propio servidor casero
(homelab) y mantenerlo: instalación desde cero, backup y restauración, y un
centro de control — todo desde la consola, en Debian/Ubuntu o Arch.

La idea: que levantar un server, respaldarlo y, si toca, **recuperarlo o migrarlo
a otro SO**, sea reproducible y sin dramas. Los **datos** van a `restic` (a un
disco), la **configuración** vive como código.

## Principios

- **Opinionado**: buenas decisiones por defecto, no mil preguntas.
- **Liviano y curado**: pocas piezas, bien elegidas.
- **Genérico**: el programa es genérico; tu config específica vive en `site/`
  (ignorada por git). Tu server = un caso, no el código.
- **Seguro**: idempotente, `--dry-run`, y nunca toca lo que no debe.

## Cómo se usa (resumen)

```
sudo ./bootstrap.sh     # instala base + lo que elijas (menú)
warden                  # menú/estado: discos, servicios, backups, firewall
```

El **catálogo** define cada app una sola vez (qué instalar, qué respaldar, cómo
dumpear su BD, su hostname/puerto público). Lo consumen el instalador, el
backup/restore y el despliegue. **Sin contraseñas en el repo**: las credenciales
de BD se leen del contenedor en tiempo de ejecución.

## Estructura

```
bootstrap.sh         Instalador (detecta distro, menú con gum)
bin/warden           Comando: menú + estado + utilidades
lib/                 Núcleo: distro (apt/pacman), UI, catálogo, helpers
examples/            Plantillas: site.conf y componentes del catálogo
stacks/              docker-compose de cada app
restore/             Restauración (disaster recovery)
secrets/             Cifrados con age/sops (NO texto plano)
site/                TU config (privada, ignorada por git)
```

## Empezar en un server nuevo

1. `cp examples/site.conf.example site/site.conf` y editalo.
2. `cp examples/catalog/app.component.example site/catalog/<app>.component` por cada app.
3. `sudo ./bootstrap.sh`.

## Fases del proyecto

Ver [ROADMAP.md](ROADMAP.md). En corto: 0) diseño · 1) backup · 2) instalador
desde cero · 3) restauración · 4) permanente en el server · 5) pulido y manual.
