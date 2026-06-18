# CI/CD de despliegue

Que un `git push` a un repo de app construya y actualice su contenedor en tu
server y lo publique en Cloudflare — sin entrar por SSH. Funciona tras CGNAT
porque el runner **sale** hacia GitHub a buscar trabajo.

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
