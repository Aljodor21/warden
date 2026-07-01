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

## Fase 7 — Pulido y calidad (en curso)
- [x] CI propio: `shellcheck` + `bash -n` + build Go en cada push/PR
- [ ] Cobertura de tests para los módulos críticos (backup, restore, reset)
- [ ] Documentación en MkDocs con capturas/gifs del panel
- [ ] Soporte de alertas push (ntfy) en eventos de backup y caída de servicio
- [ ] Exportar/importar `site/` para facilitar migraciones entre servidores
