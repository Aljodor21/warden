package main

import (
	"bytes"
	"net/http"
	"os"
	"strings"
	"sync"
)

// bgProcess es un proceso en segundo plano cuya salida se puede mostrar EN
// VIVO con polling (HTMX hx-trigger="every 2s") — el patrón se reutiliza
// para cualquier comando lento del panel (cloudflare-init, registrar un
// runner...) sin bloquear el WriteTimeout del servidor HTTP.
type bgProcess struct {
	mu      sync.Mutex
	running bool
	log     bytes.Buffer
	done    bool
}

func (b *bgProcess) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.log.Write(p)
}

func (b *bgProcess) snapshot() (string, bool, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.log.String(), b.running, b.done
}

// start marca el proceso como corriendo (no-op si ya estaba corriendo —
// devuelve false en ese caso, para no relanzarlo dos veces).
func (b *bgProcess) start() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.running {
		return false
	}
	b.running = true
	b.done = false
	b.log.Reset()
	return true
}

func (b *bgProcess) finish() {
	b.mu.Lock()
	b.running = false
	b.done = true
	b.mu.Unlock()
}

func (s *server) handleCloudflareInitStart(w http.ResponseWriter, r *http.Request) {
	if s.cfInit.start() {
		go func() {
			ctx, cancel := bgCtx10min()
			defer cancel()
			runInBackground(ctx, &s.cfInit, "sudo", s.wardenBin, "cloudflare-init")
		}()
	}
	s.renderCloudflareLog(w)
}

func (s *server) handleCloudflareInitPoll(w http.ResponseWriter, r *http.Request) {
	s.renderCloudflareLog(w)
}

func (s *server) renderCloudflareLog(w http.ResponseWriter) {
	logText, running, done := s.cfInit.snapshot()
	render(w, "cloudflare_log.html", map[string]any{"Log": logText, "Running": running, "Done": done})
}

// cloudflareTunnelID lee el ID del túnel configurado, si hay uno (solo
// para mostrarlo en Sistema — no se usa para decidir nada).
func cloudflareTunnelID() string {
	b, err := os.ReadFile("/etc/cloudflared/config.yml")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "tunnel:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "tunnel:"))
		}
	}
	return ""
}
