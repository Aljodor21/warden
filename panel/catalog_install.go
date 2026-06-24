package main

import (
	"net/http"
	"strings"
	"sync"
)

// installProcs: un bgProcess por tag, para instalar varios componentes del
// catálogo sin que sus logs se pisen entre sí (mismo patrón que restoreAppProc).
var (
	installMu    sync.Mutex
	installProcs = map[string]*bgProcess{}
)

func installProc(tag string) *bgProcess {
	installMu.Lock()
	defer installMu.Unlock()
	p, ok := installProcs[tag]
	if !ok {
		p = &bgProcess{}
		installProcs[tag] = p
	}
	return p
}

// handleCatalogInstallStart: instala UN componente del catálogo genérico
// (ej. filebrowser) con un click — corre 'warden install-component <tag>',
// que es 'warden_stack_install' (modules/stacks.sh): crea sus carpetas de
// datos, genera secretos si necesita, y levanta su docker-compose. Nunca
// pregunta nada de forma interactiva, seguro de correr así.
func (s *server) handleCatalogInstallStart(w http.ResponseWriter, r *http.Request) {
	tag := strings.TrimSpace(r.PathValue("tag"))
	if tag == "" {
		http.Error(w, "Falta el tag.", http.StatusBadRequest)
		return
	}
	proc := installProc(tag)
	if proc.start() {
		go func() {
			ctx, cancel := bgCtx3min()
			defer cancel()
			runInBackground(ctx, proc, "sudo", s.wardenBin, "install-component", tag)
		}()
	}
	s.renderCatalogInstallLog(w, tag)
}

func (s *server) handleCatalogInstallPoll(w http.ResponseWriter, r *http.Request) {
	s.renderCatalogInstallLog(w, strings.TrimSpace(r.URL.Query().Get("tag")))
}

func (s *server) renderCatalogInstallLog(w http.ResponseWriter, tag string) {
	logText, running, done := installProc(tag).snapshot()
	render(w, "catalog_install_log.html", map[string]any{
		"Tag": tag, "Log": logText, "Running": running, "Done": done,
	})
}
