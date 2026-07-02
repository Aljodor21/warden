# CI/CD

Que un `git push` a un repo de app construya y actualice su contenedor en tu server y lo publique en Cloudflare — sin entrar por SSH ni exponer puertos.

## Cómo funciona

```
git push → GitHub → runner en tu server → docker compose build/up → warden publish
```

El truco: estás tras CGNAT, GitHub no puede conectarse a tu red. Un **self-hosted runner** instalado en tu server **sale hacia GitHub** a buscar trabajo y lo ejecuta localmente — todo ocurre adentro de tu red.

## Dos CI/CD diferentes

| | Para qué | Runners |
|---|---|---|
| CI de warden (`.github/workflows/ci.yml`) | Revisa el código de warden (shellcheck, `bash -n`) | Runners de GitHub |
| CD de tus apps (esta guía) | Build y deploy de cada app tuya | Runner en **tu server** |

## Piezas

- `warden runner <url> <token>` — instala el agente en tu server
- `examples/deploy.yml` — workflow que va en **cada repo de app**
- `examples/warden-runner.sudoers` — permite que el runner llame a `warden publish`
- `warden publish` — regenera el ingress de Cloudflare desde el catálogo

## Configurar un runner (por app)

**1. Registrá el runner en tu server:**

```bash
sudo warden runner https://github.com/USUARIO/MI-APP <TOKEN>
```

El token: repo → Settings → Actions → Runners → **New self-hosted runner**.

**2. Instalá el sudoers:**

```bash
sudo cp examples/warden-runner.sudoers /etc/sudoers.d/warden-runner
# Editá USUARIO dentro del archivo, luego:
sudo visudo -cf /etc/sudoers.d/warden-runner
```

**3. Copiá el workflow al repo de la app:**

```bash
cp examples/deploy.yml .github/workflows/deploy.yml
```

El `deploy.yml` incluido hace: checkout → docker build → docker compose up → `warden publish`.

**4. Asegurate que el catálogo tenga el componente:**

```ini
# site/catalog/miapp.component
COMP_CF_HOST="miapp.tudominio.com"
COMP_CF_PORT="8080"
```

**5. `git push` → el runner lo toma, construye y publica.**

También podés registrar runners desde el panel: **Catálogo** → editar la app → campo de token → **Registrar runner**.

## Ver runners activos

**Panel** → Sistema → sección Runners: lista cada runner con su estado (online/offline).

**CLI:**
```bash
sudo warden runner status
```

## Preguntas frecuentes

**¿Y si tengo varias apps?** Un runner por repo, todos en el mismo server. Cada uno escucha solo los eventos de su repo.

**¿Qué pasa si el runner está offline?** GitHub encola los jobs hasta 30 días. Cuando el runner vuelve, ejecuta el trabajo pendiente.

**¿Hace falta IP pública?** No. El runner sale hacia GitHub, no al revés. Funciona perfecto tras CGNAT.
