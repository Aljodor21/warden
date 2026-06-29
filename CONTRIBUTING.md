# Contribuir a warden

## CI automático

Cada push a `main`/`dev` o PR dispara tres jobs en GitHub Actions
(`.github/workflows/ci.yml`):

| Job | Qué hace |
|---|---|
| `shell` | `bash -n` + `shellcheck --severity=error` en todos los `.sh` |
| `go` | `gofmt` + `go vet` + `go build` + `go test -race` del panel (`panel/`) |
| `dry-run` | Corre `bootstrap.sh` completo en Ubuntu 24.04 sin instalar nada de verdad |

Si alguno falla, el PR no se puede mergear hasta que se corrija.

## Modo no-interactivo (`WARDEN_CI=1`)

El bootstrap usa menús y prompts interactivos (`gum`, `read`). Para que corran
sin input humano en CI, existe la variable `WARDEN_CI=1`:

| Función | Comportamiento normal | Con `WARDEN_CI=1` |
|---|---|---|
| `ui_confirm` | Pregunta sí/no | Siempre sí |
| `ui_input` | Pide texto | Devuelve el valor por defecto |
| `ui_menu` | Muestra un menú | Elige la primera opción |

Combinada con `WARDEN_DRY_RUN=1`, ningún comando real se ejecuta.

## Modo dry-run (`WARDEN_DRY_RUN=1`)

Todos los comandos destructivos pasan por `run "..."` en `lib/core.sh`.
Con `WARDEN_DRY_RUN=1`, `run` imprime `[dry-run] <comando>` en vez de ejecutarlo.

Regla: **todo comando que modifica el sistema va dentro de `run`**. Si un módulo
llama algo directamente (sin `run`), hay que guardarlo con `has <tool> || return 0`
cuando el tool no esté instalado en dry-run.

## Preset no-interactivo (`WARDEN_PRESET`)

```bash
WARDEN_PRESET=basico ./bootstrap.sh    # salta el menú de preset
WARDEN_PRESET=completo ./bootstrap.sh
```

## Probar localmente antes de pushear

```bash
# Sintaxis y shellcheck
bash -n bootstrap.sh lib/*.sh modules/*.sh
shellcheck --severity=error bootstrap.sh lib/*.sh modules/*.sh

# Panel Go (mismo orden que el CI)
cd panel
gofmt -l .          # vacío = todo formateado; si lista algo: gofmt -w .
go vet ./...
go build ./...
go test -race ./...
cd ..

# Dry-run completo (necesita sudo)
WARDEN_DRY_RUN=1 WARDEN_CI=1 WARDEN_PRESET=basico sudo ./bootstrap.sh
```

## Estructura del repo

```
bootstrap.sh     Instalador principal (punto de entrada)
bin/warden       CLI unificado (menú + subcomandos)
lib/             Núcleo compartido (distro, ui, catálogo, presets)
modules/         Un archivo por función (backup, cloudflare, panel…)
panel/           warden-panel — servidor Go del dashboard web
stacks/          docker-compose de cada app curada
catalog/         Recetas genéricas de apps
examples/        Plantillas para que el usuario arme su site/
restore/         Script de disaster recovery
docs/            Documentación extra
site/            Config privada del usuario (en .gitignore, nunca al repo)
```

## Agregar un módulo nuevo

1. Crear `modules/<nombre>.sh` con una función `warden_<nombre>()`.
2. Sourcear en `bin/warden` (sección de `source`).
3. Agregar el subcomando en el `case` al final de `bin/warden`.
4. Si tiene prompts interactivos, asegurarse de que pasen por `ui_confirm` /
   `ui_input` / `ui_menu` (no `read` directo) para que `WARDEN_CI=1` funcione.
5. Si ejecuta comandos del sistema, usar `run "..."` para que `WARDEN_DRY_RUN=1`
   los salte correctamente.
