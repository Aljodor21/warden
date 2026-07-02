# Contribuir

warden es un proyecto personal y opinionado, pero las contribuciones son bienvenidas si van en la misma dirección: simple, local, sin magia.

## Antes de abrir un PR

- Describí en un issue qué querés agregar y por qué — para no trabajar en paralelo sobre lo mismo.
- Las adiciones que requieran instalar un runtime nuevo en el host (Node, Python, Ruby…) no van a ser aceptadas.
- warden no tiene telemetría, no tiene cuentas, no tiene servicios externos pagos — no agreges eso.

## Clonar y correr el panel localmente

```bash
git clone https://github.com/Aljodor21/warden.git
cd warden/panel

# Compilar y correr el panel (requiere Go 1.21+)
go build -o warden-panel . && ./warden-panel
# Panel en http://localhost:8080
```

El panel no necesita el sistema real para compilar. Algunas secciones van a mostrar errores al cargar datos del sistema — es esperado si no tenés Docker, Tailscale, etc.

## Estructura del código

```
bin/warden             Script principal CLI (Bash)
lib/                   Funciones compartidas entre módulos
modules/               Un archivo por función: backup, cloudflare, vpn, docker…
panel/
  main.go              Rutas HTTP, servidor, middleware
  *.go                 Un archivo por sección del panel
  templates/
    *.html             Templates Go — HTML + HTMX + Alpine.js
    style.html         Todo el CSS (variables, dark mode, glass)
    nav.html           Barra de navegación compartida
stacks/                Recetas Docker Compose curadas
examples/              Configs de ejemplo: site.conf, components, deploy.yml
docs/                  Esta documentación
```

## Estilo de código

**Bash:**
- POSIX sh donde se pueda; Bash donde sea necesario.
- Seguí el estilo de `lib/ui.sh` para los helpers de UI.
- `shellcheck` no debe dar errores (`make check` o el CI los ve).

**Go:**
- Stdlib solamente — sin frameworks externos.
- Seguí el patrón de los handlers existentes: `func (s *server) handleX(w, r)`.
- Sin comentarios que expliquen qué hace el código — solo el por qué si es no obvio.

**Templates HTML:**
- Sin emojis decorativos en la UI.
- HTMX para interactividad; Alpine.js para estado reactivo en el cliente.
- El CSS vive en `templates/style.html` — usá las variables existentes.

## CI

El CI de este repo corre `shellcheck` sobre todos los scripts Bash y `bash -n` para validar sintaxis. No es necesario correrlo localmente antes de abrir el PR — el CI lo hace al pushear.

## Reporte de bugs

Abrí un issue con:
- Qué versión de Debian/Ubuntu/Arch usás
- El comando exacto que falló
- El output completo del error
