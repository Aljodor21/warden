# CI/CD de despliegue

Que un `git push` a un repo de app construya y actualice su contenedor en tu
server y lo publique en Cloudflare — sin entrar por SSH. Funciona tras CGNAT
porque el runner **sale** hacia GitHub a buscar trabajo.

## Conceptos

- **CI** (Integración Continua): cada `git push` dispara revisiones/pruebas automáticas.
- **CD** (Despliegue Continuo): si pasan, se despliega solo.

En este proyecto hay **dos CI/CD distintos**:

1. **CI de warden** (`.github/workflows/ci.yml`, este repo): revisa el *código de
   warden* (shellcheck + `bash -n`) en los runners de GitHub. No despliega nada.
2. **CD de tus apps** (esta guía): al hacer push a un repo de app, un runner **en tu
   server** construye y actualiza su contenedor, y publica en Cloudflare.

**¿Por qué un runner en tu server?** Estás tras **CGNAT**: GitHub no puede entrar a
tu red. El self-hosted runner **sale** hacia GitHub, toma el trabajo y lo ejecuta
localmente — el despliegue ocurre dentro de tu red sin abrir puertos.

**Conexión con el catálogo:** `warden publish` no inventa nada — lee el
`COMP_CF_HOST` y `COMP_CF_PORT` de cada app en el catálogo y regenera el ingress
de Cloudflare. Definís la app una vez y el despliegue la publica solo.

## Piezas
- `warden runner <url> <token>` — instala el agente (runner) en tu server.
- `warden publish` — regenera el ingress de Cloudflare desde el catálogo.
- `examples/deploy.yml` — workflow que va EN CADA repo de app.
- `examples/warden-runner.sudoers` — deja que el runner llame a `warden publish`.

## Pasos (por cada app)
1. En tu server, registrá el runner:
   `sudo warden runner https://github.com/USUARIO/APP <TOKEN>`
   (el token: repo → Settings → Actions → Runners → New self-hosted runner)
2. Instalá el sudoers (editando USUARIO):
   `sudo cp examples/warden-runner.sudoers /etc/sudoers.d/warden-runner`
   y validalo: `sudo visudo -cf /etc/sudoers.d/warden-runner`
3. Copiá `examples/deploy.yml` a `.github/workflows/deploy.yml` EN EL REPO DE LA APP.
4. Asegurate que el componente de esa app (en `site/catalog/`) tenga
   `COMP_CF_HOST` y `COMP_CF_PORT`.
5. `git push` → se despliega solo.

## Flujo
```
git push → GitHub → runner en tu server → docker compose build/up → warden publish (Cloudflare)
```

## Variables de entorno (.env) de una app

El `.env` de una app tiene secretos y no va a GitHub. En el panel, **Catálogo → app de CI/CD → Variables (.env)**:

- Si todavía no hay un `.env` guardado, el panel trae el ejemplo del repo de la app (`.env.docker.example`, `.env.example`, `.env.sample` o `example.env`, si el repo es público) para que cambies los valores por los reales. No se guarda nada hasta que le des **Guardar**.
- Se guarda en `site/env/<tag>.env` (carpeta privada, fuera de git), con permiso `0600` y a nombre del dueño del repo de warden — el usuario del runner — para que el workflow de la app lo pueda leer: `cp ~/warden/site/env/<tag>.env .env`.
- Hace falta el modo admin para verlo y editarlo.

## Registrar el runner después

La pantalla de próximos pasos (con el formulario para pegar el token del runner) sale al guardar una app nueva, pero si no lo registraste en ese momento puedes volver: **Catálogo → app de CI/CD → Runner y pasos**. La pantalla revisa si el repo ya tiene un runner: si lo tiene, lo dice y no pide nada; si no, muestra el formulario para pegar el token.

## Desplegar warden mismo

Cada push a `main` del repo `warden` actualiza el servidor: `git pull` en `~/warden` (eso ya actualiza el comando `warden` y los módulos, porque `/usr/local/bin/warden` apunta al repo), compila el panel, lo instala y lo reinicia. Si no responde, vuelve al panel anterior. Lo hace `.github/workflows/deploy.yml`.

Puesta en marcha (una vez):

```bash
# 1. Runner para el repo warden (token: repo warden → Settings → Actions → Runners → New self-hosted runner)
sudo warden runner https://github.com/USUARIO/warden <TOKEN>

# 2. Permiso para instalar el panel y reiniciarlo (edita USUARIO dentro del archivo)
sudo cp examples/warden-deploy.sudoers /etc/sudoers.d/warden-deploy
sudo visudo -cf /etc/sudoers.d/warden-deploy
```

Cada repo tiene su propio runner en su propia carpeta (`/opt/warden/actions-runner-USUARIO-REPO`), así que el de warden convive con los de tus apps. El workflow asume que el repo está en `/home/alejo/warden`; si lo tienes en otra ruta, cambia `WARDEN_DIR` en `deploy.yml` y el sudoers.

Seguridad: el runner puede instalar como root el panel que compila. En un repo público, en Settings → Actions → General deja **"Require approval for all outside collaborators"**, para que nadie ajeno corra workflows en tu servidor.
