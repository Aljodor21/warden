# Roadmap — fases de warden

## Fase 0 — Análisis y diseño ✅
Inventario, decisiones (stack, dashboard), estructura del repo y separación
**core genérico / config del sitio** (`site/`).

## Fase 1 — Backup de datos ✅
`restic` (en Docker) respaldando datos + dumps de BD a un disco externo, con
tags por componente y verificación (`restic check`). Cero huella en el server.

## Fase 2 — Instalador desde cero ✅
`bootstrap.sh` multi-distro (apt/pacman) + UI con `gum`. Presets (`básico` /
`completo`) o instalación a la carta. Probado en hardware real (OptiPlex 7040).

## Fase 3 — Restauración (disaster recovery) ✅
Restauración inteligente: lee los `paths` del snapshot, los cruza con el
catálogo, e instala sola cualquier app con datos en el backup. Dumps de BD
incluidos. Validado en migración real: 74 GiB + DBs restaurados, todas las
apps revividas.

## Fase 4 — Permanente en el server ✅
Montaje por UUID (`/etc/fstab`), timer de backup horario + verify nocturno,
Backrest, Cockpit, firewall endurecido. warden queda "residente".

## Fase 5 — Panel web completo ✅
`warden-panel`: cinco páginas (Dashboard, Catálogo, NAS, Backups, Sistema).
Go stdlib + HTMX + Alpine.js, sin Node/build. Todas las acciones de `warden`
disponibles desde el navegador — mismo código por debajo. DR test completo
pasado en producción real.

## Fase 6 — Tienda de apps ✅
Grilla de apps listas para instalar (plantillas de Portainer, +100 apps).
Instalar en un click desde el panel — detecta si hay receta curada en warden
y la usa directamente; si no, importa y adapta el compose automáticamente.
`warden import` disponible también por consola. Apps ya instaladas marcadas
visualmente.

## Fase 7 — Pulido y calidad ✅
- [x] CI propio: `shellcheck` + `bash -n` + build Go en cada push/PR
- [x] Alertas push (ntfy): instalación con un click, watch de contenedores,
  notificaciones de backup/verify, log en vivo
- [x] VPN connect migrado a bgProcess: link de auth aparece en el panel en tiempo real
- [x] Logs en vivo por contenedor desde el catálogo (últimas 100 líneas, auto-scroll)
- [x] Guardar app en catálogo redirige de inmediato (publish en background)
- [x] Anti doble-click: botones y formularios se deshabilitan durante requests HTMX

## Fase 8 — Apariencia y experiencia ✅
- [x] 7 temas de color: Dark, Nord, Catppuccin Mocha, Dracula, Gruvbox, Tokyo Night, Claro
- [x] Fondos de pantalla desde dharmx/walls (GitHub API, paginados, caché 24h)
- [x] Explorador de imágenes del servidor (ruta configurable)
- [x] Subida de imagen local con redimensionado automático a 1920px
- [x] Detección de luminancia: contraste automático en imágenes claras
- [x] 4 sliders persistentes: opacidad vidrio · desenfoque vidrio · brillo fondo · desenfoque fondo
- [x] Navbar con efecto glass sutil
- [x] Contenedores difuminados con backdrop-filter real
- [x] Página "Acerca de" con grupos de tecnologías, descripciones y créditos externos

## Pendiente (opcional / baja prioridad)
- [ ] Tests unitarios para módulos bash críticos (backup, restore, reset)
- [ ] Documentación en MkDocs con capturas del panel (si se publica para otros)
