package main

import (
	"net/http"
	"os"
	"strings"
	"sync"
)

// restoreAutoFlagFile: respaldo independiente de sudo/env_keep. Visto en
// vivo: una versión de sudo que ignora '-E' y bloquea preservar el entorno
// completo ("preserving the entire environment is not supported") al
// cruzar el segundo 'sudo' interno que bin/warden hace para 'restore' —
// la variable de entorno se perdía ahí. Este archivo lo lee restore.sh
// como alternativa: el panel (ya root) lo escribe/borra directo, sin
// depender de ninguna política de sudoers.
const restoreAutoFlagFile = "/run/warden-restore-auto"

// restoreAutoFlagRefs: contador de referencias — si dos restauraciones
// corren en paralelo (el general + un "Ya desplegué" de una app puntual),
// la primera que termine no debe borrar el flag que la otra todavía usa.
var (
	restoreAutoFlagMu   sync.Mutex
	restoreAutoFlagRefs int
)

func acquireRestoreAutoFlag() {
	restoreAutoFlagMu.Lock()
	defer restoreAutoFlagMu.Unlock()
	restoreAutoFlagRefs++
	_ = os.WriteFile(restoreAutoFlagFile, nil, 0644)
}

func releaseRestoreAutoFlag() {
	restoreAutoFlagMu.Lock()
	defer restoreAutoFlagMu.Unlock()
	restoreAutoFlagRefs--
	if restoreAutoFlagRefs <= 0 {
		restoreAutoFlagRefs = 0
		os.Remove(restoreAutoFlagFile)
	}
}

// handleRestoreStart corre 'warden restore' en modo automático desde el
// panel — Al fue explícito: el flujo real para él SIEMPRE es el navegador,
// nunca la consola. Es seguro porque restore.sh detecta el modo auto y
// nunca cae a un prompt interactivo en ese modo (si falta el disco o la
// contraseña, falla con un mensaje claro en vez de colgarse esperando input).
func (s *server) handleRestoreStart(w http.ResponseWriter, r *http.Request) {
	if s.restoreProc.start() {
		acquireRestoreAutoFlag()
		go func() {
			defer releaseRestoreAutoFlag()
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

// handleRestoreFromSnapshot: "Restaurar desde esta corrida" — igual que
// 'Restaurar ahora', pero fijando QUÉ snapshot de archivos/BD usar, en
// vez de dejar que restore.sh asuma ciegamente que el más reciente tiene
// contenido real (no siempre es así: el backup automático puede correr
// justo después de reinstalar una app, capturándola vacía).
func (s *server) handleRestoreFromSnapshot(w http.ResponseWriter, r *http.Request) {
	filesID := strings.TrimSpace(r.FormValue("files_id"))
	dbID := strings.TrimSpace(r.FormValue("db_id"))
	if filesID == "" {
		http.Error(w, "Falta el snapshot de archivos.", http.StatusBadRequest)
		return
	}
	if s.restoreProc.start() {
		acquireRestoreAutoFlag()
		go func() {
			defer releaseRestoreAutoFlag()
			ctx, cancel := bgCtx10min()
			defer cancel()
			args := []string{"restore", "--files-snapshot", filesID}
			if dbID != "" {
				args = append(args, "--db-snapshot", dbID)
			}
			cmdArgs := append([]string{"env", "WARDEN_RESTORE_AUTO=1", s.wardenBin}, args...)
			runInBackground(ctx, &s.restoreProc, "sudo", cmdArgs...)
		}()
	}
	s.renderRestoreLog(w)
}

func (s *server) renderRestoreLog(w http.ResponseWriter) {
	logText, running, done := s.restoreProc.snapshot()
	render(w, "restore_log.html", map[string]any{"Log": logText, "Running": running, "Done": done})
}
