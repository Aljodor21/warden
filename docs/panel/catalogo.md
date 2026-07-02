# Catálogo

![Catálogo](../screenshots/screenshot-catalog.png)

El catálogo muestra todas las apps gestionadas por warden. Se puebla automáticamente al instalar con el preset — no hace falta editar ningún archivo.

Las apps aparecen en dos grupos:

- **Instaladas por warden**: Docker Compose gestionado por warden.
- **Desplegadas vía CI/CD**: el build/deploy lo hace GitHub Actions en tu server.

Cada card muestra el estado real (corriendo/caída), el link si tiene subdominio, y botones de acción.

## Acciones por app

### Ver logs en vivo
El botón **Ver logs** abre un visor de las últimas 100 líneas del contenedor con auto-scroll y refresco cada 2 segundos. El scroll solo baja solo si estás al final — si subiste a leer, no te interrumpe.

### Editar app
Formulario con todos los campos del componente:

- Nombre y tag
- Tipo de backup (files / postgres / none)
- Rutas de datos para respaldar
- Subdominio Cloudflare y puerto
- Selector de puerto: muestra los puertos que expone el contenedor para elegir con un click
- Editor del `docker-compose.yml` integrado — sin terminal

Al guardar con subdominio, publica el túnel automáticamente en background y redirige de inmediato.

### Eliminar app
Baja el contenedor, borra imágenes y volúmenes, regenera el túnel y borra el registro DNS de Cloudflare (si configuraste el API Token).

## Instalar una app que no está activa

Si una app aparece en el catálogo pero no está corriendo (contenedor caído o no instalado), el botón **Instalar** la levanta con log en vivo.

## Agregar una app nueva

El botón **+ Nueva app** abre un formulario de alta. Los campos mínimos son nombre y tag; los demás son opcionales. La app queda registrada en el catálogo y disponible para backup, tunnel y CI/CD desde ese momento.

Para instalar apps de terceros de forma más rápida, usá la [Tienda](tienda.md) — tiene más de 100 apps listas con un click.
