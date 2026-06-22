# warden — visión

> **Omarchy para servidores.** Un instalador opinionado, bonito y batteries-included
> que convierte un Linux recién instalado (Debian/Arch) en un homelab sólido,
> seguro y mantenible — con un comando. Hecho a la medida de Al, pero **útil para
> cualquiera** como base de su server inicial.

## Filosofía
- **Opinionado**: buenas decisiones por defecto, no mil preguntas. Avanzados sobreescriben en `site.conf`.
- **Bonito**: estética coherente (TUI, MOTD, dashboards, docs). El server también puede ser hermoso.
- **Liviano y curado**: el hardware es modesto (8 GB). Pocas piezas, bien elegidas. Nada de amontonar.
- **Reutilizable**: core genérico + `site.conf`. El server de Al = un `examples/`.
- **Confiable**: idempotente, `--dry-run`, auto-probado en CI, documentado.

## La experiencia (lo que vive el usuario)
1. Instala Debian/Arch → `curl … | bash` o clona y corre `warden`.
2. **Banner + asistente TUI** (bonito) detecta distro/hardware y pregunta lo esencial.
3. Elige **preset** (`básico` / `completo`) o apps a la carta. Backup opcional.
4. warden instala, endurece y configura. Idempotente.
5. Al hacer SSH: **MOTD dashboard** hermoso (disco, backups, servicios, alertas).
6. Día a día: `warden` (TUI) + Cockpit + Backrest. Alertas a tu celular (ntfy).

## Stack curado (liviano por diseño)
| Necesidad | Pieza | Por qué |
|---|---|---|
| Sistema/admin | **Cockpit** | discos, servicios, logs, red |
| Backup | **restic + Backrest** | motor + UI/agenda/B2 |
| Apps | catálogo + compose (¿CasaOS? a curar) | recetas reutilizables |
| Alertas | **ntfy** | push al celular: backup/disco/servicio |
| Uptime | **Uptime Kuma** | ¿está arriba mi servicio? |
| Logs contenedores | **Dozzle** | ver logs bonito |
| Métricas | **Beszel** (liviano) | CPU/RAM/historial |
| Seguridad | fail2ban + **lynis** (`warden audit`) | anti-fuerza-bruta + auditoría |
| Shell | oh-my-zsh + p10k + fastfetch | bonito y productivo |

> ⚠️ Anti-sprawl: NO metemos 6 dashboards. Se curan a un set coherente.

## Las "opiniones" a definir (dan el carácter)
1. **TUI**: `gum` (moderno, hermoso, 1 binario) vs `whiptail` (universal, plano).
2. **Distro bendecida**: Debian/Ubuntu primario, Arch soportado (¿o al revés?).
3. **Dashboards**: curar — ¿CasaOS se queda o lo reemplaza Dockge/Cockpit?
4. **Exposición**: Cloudflare (público) + Tailscale (privado), elegible por app.
5. **Alertas**: ntfy como hub único.
6. **Monitoreo**: liviano (Beszel + Dozzle) vs pesado (Netdata).

## Para que sirva a otros
- README con capturas/gifs · licencia (MIT) · guía "agrega tu app" · runbook DR.
- **Manual bonito** (mkdocs-material) como Omarchy.
- **CI propio**: shellcheck + instalación `--dry-run` en contenedor → confianza.
