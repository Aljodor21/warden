package main

import "net/http"

// handleRestoreStart corre 'warden restore' en modo automático desde el
// panel — Al fue explícito: el flujo real para él SIEMPRE es el navegador,
// nunca la consola. Es seguro porque restore.sh detecta WARDEN_RESTORE_AUTO=1
// y nunca cae a un prompt interactivo en ese modo (si falta el disco o la
// contraseña, falla con un mensaje claro en vez de colgarse esperando input).
//
// La variable se pasa con 'sudo env VAR=1 ...' en vez de cmd.Env, porque
// sudo no propaga variables de entorno arbitrarias salvo que estén en
// env_keep de sudoers — 'env' delante del comando lo evita sin tocar esa
// configuración.
func (s *server) handleRestoreStart(w http.ResponseWriter, r *http.Request) {
	if s.restoreProc.start() {
		go func() {
			ctx, cancel := bgCtx10min() // puede tardar: instala apps que falten antes de restaurar
			defer cancel()
			runInBackground(ctx, &s.restoreProc, "sudo", "env", "WARDEN_RESTORE_AUTO=1", s.wardenBin, "restore")
		}()
	}
	s.renderRestoreLog(w)
}

func (s *server) handleRestorePoll(w http.ResponseWriter, r *http.Request) {
	s.renderRestoreLog(w)
}

func (s *server) renderRestoreLog(w http.ResponseWriter) {
	logText, running, done := s.restoreProc.snapshot()
	render(w, "restore_log.html", map[string]any{"Log": logText, "Running": running, "Done": done})
}
