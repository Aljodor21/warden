package main

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// El único paso de 'warden cloudflare-init' que de verdad bloquea es el
// login (cloudflared imprime una URL y espera, de forma asíncrona, a que
// completes el login en TU navegador — no lee nada de la terminal para
// eso). El resto del script (nombre del túnel, conflictos) usa prompts que,
// sin TTY (stdin en /dev/null, el default de exec.Cmd), caen solos a una
// opción segura: usan el hostname como nombre, y NUNCA pisan un túnel
// existente sin confirmación explícita. Por eso esto SÍ se puede correr
// desde el panel: el proceso corre en segundo plano y el panel muestra su
// salida en vivo (con polling) para que copies la URL apenas aparezca.
type cfInitState struct {
	mu      sync.Mutex
	running bool
	log     bytes.Buffer
	done    bool
}

func (c *cfInitState) write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.log.Write(p)
}

func (c *cfInitState) snapshot() (string, bool, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.log.String(), c.running, c.done
}

func (s *server) handleCloudflareInitStart(w http.ResponseWriter, r *http.Request) {
	s.cfInit.mu.Lock()
	if s.cfInit.running {
		s.cfInit.mu.Unlock()
		s.renderCloudflareLog(w)
		return
	}
	s.cfInit.running = true
	s.cfInit.done = false
	s.cfInit.log.Reset()
	s.cfInit.mu.Unlock()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute) // tope si nadie completa el login
		defer cancel()
		cmd := exec.CommandContext(ctx, "sudo", s.wardenBin, "cloudflare-init")
		cmd.Stdin = nil // explícito: sin TTY, los prompts caen a su default seguro
		cmd.Stdout = &cfWriter{s: &s.cfInit}
		cmd.Stderr = &cfWriter{s: &s.cfInit}
		_ = cmd.Run()
		s.cfInit.mu.Lock()
		s.cfInit.running = false
		s.cfInit.done = true
		s.cfInit.mu.Unlock()
	}()

	s.renderCloudflareLog(w)
}

type cfWriter struct{ s *cfInitState }

func (w *cfWriter) Write(p []byte) (int, error) { return w.s.write(p) }

func (s *server) handleCloudflareInitPoll(w http.ResponseWriter, r *http.Request) {
	s.renderCloudflareLog(w)
}

func (s *server) renderCloudflareLog(w http.ResponseWriter) {
	logText, running, done := s.cfInit.snapshot()
	render(w, "cloudflare_log.html", map[string]any{"Log": logText, "Running": running, "Done": done})
}

// cloudflareTunnelName lee el nombre/ID del túnel configurado, si hay uno
// (solo para mostrarlo en Sistema — no se usa para decidir nada).
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
