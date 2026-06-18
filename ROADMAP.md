# Roadmap — fases de warden

## Fase 0 — Análisis y diseño ✅
Inventario, decisiones (stack, dashboard), estructura del repo y separación
**core genérico / config del sitio** (`site/`).

## Fase 1 — Backup de datos ✅
`restic` (en Docker) respaldando datos + dumps de BD a un disco externo, con
tags por componente y verificación (`restic check`). Cero huella en el server.

## Fase 2 — Instalador desde cero (en curso)
`bootstrap.sh` multi-distro (apt/pacman) + UI con `gum`. Instala base + Docker +
los componentes o el preset elegido. Se prueba en un SO limpio (VM o PC).

## Fase 3 — Restauración (disaster recovery)
Sobre la instalación nueva: conectar el disco, listar snapshots por tag y
restaurar datos + BD. Validar que las apps reviven.

## Fase 4 — Permanente en el server
Montaje por UUID (fstab), timer de backup + verify nocturno, Backrest, Cockpit,
Homepage y firewall endurecido. Recién aquí warden se vuelve "residente".

## Fase 5 — Pulido y reutilizable
Manual (mkdocs), CI propio (shellcheck + dry-run), presets, temas, alertas
(ntfy) y CI/CD de despliegue (build + compose up + Cloudflare).
