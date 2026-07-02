# Panel web

El panel web (`warden-panel`) es un servidor HTTP escrito en **Go** (solo stdlib, sin frameworks) con **HTMX** + **Alpine.js** vendorizados — sin CDN, sin Node, sin paso de build.

Un binario de pocos MB de RAM corre como servicio systemd en el puerto 80. Todo lo que hace el panel lo puede hacer también la CLI `warden` por consola — son dos caminos al mismo resultado.

## Seguridad

Un **candado de Admin** en la barra de navegación desbloquea las acciones que modifican el sistema (instalar, borrar, configurar). La sesión de admin dura hasta que cerrás el navegador — sin Expires en la cookie.

Las acciones de solo lectura (ver logs, ver estado) no requieren admin.

## Secciones

| Página | Descripción |
|---|---|
| [Dashboard](dashboard.md) | Salud del sistema en vivo: CPU, RAM, discos, red, apps |
| [Catálogo](catalogo.md) | Apps gestionadas por warden — editar, logs, instalar, eliminar |
| [Tienda](tienda.md) | Grilla de +100 apps self-hosted para instalar en un click |
| [Backups](backups.md) | Backup con restic, snapshots, restauración, timers |
| [Sistema](sistema.md) | VPN, Cloudflare, ntfy, runners, secretos, zona de peligro |
| [NAS](nas.md) | Usuarios Samba sin tocar la terminal |
| [Apariencia](apariencia.md) | Temas de color, fondos de pantalla, ajustes de vidrio |
