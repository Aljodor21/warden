package main

import (
	"net/http"
	"strings"
)

// handleRunnerRegisterStart: registra un self-hosted runner para un repo,
// EN VIVO desde el panel — sin que haga falta abrir una consola. Es seguro
// correrlo así porque 'warden runner <url> <token>' nunca pregunta nada
// cuando recibe ambos argumentos (el prompt interactivo solo se usa si
// faltan) y usa --unattended contra GitHub.
func (s *server) handleRunnerRegisterStart(w http.ResponseWriter, r *http.Request) {
	install := strings.TrimSpace(r.FormValue("install"))
	token := strings.TrimSpace(r.FormValue("token"))
	if install == "" || token == "" {
		render(w, "runner_register_log.html", map[string]any{"Err": "Faltan la URL del repo o el token."})
		return
	}
	if s.runnerReg.start() {
		go func() {
			ctx, cancel := bgCtx3min()
			defer cancel()
			runInBackground(ctx, &s.runnerReg, "sudo", s.wardenBin, "runner", install, token)
		}()
	}
	s.renderRunnerRegisterLog(w)
}

func (s *server) handleRunnerRegisterPoll(w http.ResponseWriter, r *http.Request) {
	s.renderRunnerRegisterLog(w)
}

func (s *server) renderRunnerRegisterLog(w http.ResponseWriter) {
	logText, running, done := s.runnerReg.snapshot()
	render(w, "runner_register_log.html", map[string]any{"Log": logText, "Running": running, "Done": done})
}
