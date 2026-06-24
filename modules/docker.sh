#!/usr/bin/env bash
# modules/docker.sh — instala Docker y limita el tamaño de sus logs.

warden_docker_install() {
  if has docker; then
    ok "Docker ya está instalado"
  else
    log "Instalando Docker"
    case "$DISTRO" in
      debian) run "curl -fsSL https://get.docker.com | sh" ;;
      arch)   ensure_pkg docker docker; ensure_pkg docker-compose docker-compose ;;
      *) die "No sé instalar Docker en esta distro" ;;
    esac
  fi

  # Límite de logs: evita que los logs de contenedores llenen el disco.
  local dj=/etc/docker/daemon.json
  if [ ! -f "$dj" ]; then
    log "Configurando límite de logs de Docker (10m x 3)"
    run "mkdir -p /etc/docker"
    run "printf '%s\n' '{ \"log-driver\": \"json-file\", \"log-opts\": { \"max-size\": \"10m\", \"max-file\": \"3\" } }' > '$dj'"
  else
    warn "Ya existe $dj — no lo toco (revisalo a mano si querés el límite de logs)"
  fi

  run "systemctl enable --now docker"

  # 'enable --now' devuelve apenas el servicio arrancó, pero containerd
  # puede tardar unos segundos más en terminar de inicializar su
  # almacenamiento interno — un módulo posterior (ej. NAS) que haga
  # 'docker compose up' de inmediato puede pegarle a esa ventana y fallar
  # con un error de mkdir en /var/lib/containerd. Esperamos activamente a
  # que el daemon responda de verdad antes de seguir.
  if [ "${WARDEN_DRY_RUN:-0}" != 1 ]; then
    log "Esperando a que el daemon de Docker esté listo de verdad"
    local i=0
    until docker info >/dev/null 2>&1; do
      i=$((i + 1))
      [ "$i" -ge 30 ] && { warn "Docker tardó más de 30s en responder — seguimos igual."; break; }
      sleep 1
    done
  fi

  # Permitir docker sin sudo al usuario que invocó el script.
  local u="${SUDO_USER:-}"
  if [ -n "$u" ] && [ "$u" != root ]; then
    run "usermod -aG docker '$u'"
    warn "Agregado '$u' al grupo docker. Cerrá sesión y volvé a entrar para que aplique."
  fi

  ok "Docker listo"
}
