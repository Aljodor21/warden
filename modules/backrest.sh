#!/usr/bin/env bash
# modules/backrest.sh — Backrest: UI web de backups (restic) sobre el disco.

BACKREST_HOME="${BACKREST_HOME:-/etc/warden/backrest}"

warden_backrest_install() {
  run "mkdir -p '$BACKREST_HOME/config' '$BACKREST_HOME/data' '$BACKREST_HOME/cache'"
  export BACKREST_HOME
  log "Levantando Backrest"
  run "_compose -f '$WARDEN_ROOT/stacks/backrest/docker-compose.yml' up -d"
  ok "Backrest → http://$(warden_host):${BACKREST_PORT:-9898}"
}
