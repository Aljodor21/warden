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

func (s *server) renderRestoreLog(w http.ResponseWriter) {
	logText, running, done := s.restoreProc.snapshot()
	data := map[string]any{"Log": logText, "Running": running, "Done": done}
	if done {
		data["Pending"] = s.pendingCICDFrom(logText)
	}
	render(w, "restore_log.html", data)
}

// PendingCICD: una app con datos en el backup que restore.sh no pudo
// instalar sola (vive en su propio repo) — el panel le da un camino
// accionable en vez de dejarla como un simple aviso de texto.
type PendingCICD struct {
	Tag, Name, Install string
	Owner, Repo        string
	RunnerFound        bool
	RunnerService      string
}

// pendingCICDFrom busca la línea 'PENDING_CICD:tag1,tag2' que restore.sh
// imprime al final, y resuelve cada tag contra el catálogo real.
func (s *server) pendingCICDFrom(logText string) []PendingCICD {
	var tags []string
	for _, line := range strings.Split(logText, "\n") {
		if t, ok := strings.CutPrefix(strings.TrimSpace(line), "PENDING_CICD:"); ok {
			tags = strings.Split(t, ",")
			break
		}
	}
	var out []PendingCICD
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		c, err := parseComponentFile(s.siteCatalogDir + "/" + tag + ".component")
		if err != nil {
			c, err = parseComponentFile(s.repoCatalogDir + "/" + tag + ".component")
		}
		if err != nil {
			continue
		}
		p := PendingCICD{Tag: tag, Name: c.Name, Install: c.Install}
		if owner, repo, ok := parseGitHubRepo(c.Install); ok {
			p.Owner, p.Repo = owner, repo
			p.RunnerService, p.RunnerFound = findRunnerService(owner, repo)
		}
		out = append(out, p)
	}
	return out
}

// restoreAppProcs: un bgProcess por tag, para poder restaurar varias apps
// pendientes en paralelo sin que se pisen los logs entre sí.
var (
	restoreAppMu    sync.Mutex
	restoreAppProcs = map[string]*bgProcess{}
)

func restoreAppProc(tag string) *bgProcess {
	restoreAppMu.Lock()
	defer restoreAppMu.Unlock()
	p, ok := restoreAppProcs[tag]
	if !ok {
		p = &bgProcess{}
		restoreAppProcs[tag] = p
	}
	return p
}

// handleRestoreAppStart: "Ya desplegué, restaurar datos ahora" — corre
// 'warden restore --tag <tag>' (no toca nada más del sistema), pensado
// para usarse después de pegar el token del runner y hacer push/re-run.
func (s *server) handleRestoreAppStart(w http.ResponseWriter, r *http.Request) {
	tag := strings.TrimSpace(r.FormValue("tag"))
	if tag == "" {
		http.Error(w, "Falta el tag.", http.StatusBadRequest)
		return
	}
	proc := restoreAppProc(tag)
	if proc.start() {
		acquireRestoreAutoFlag()
		go func() {
			defer releaseRestoreAutoFlag()
			ctx, cancel := bgCtx3min()
			defer cancel()
			runInBackground(ctx, proc, "sudo", "env", "WARDEN_RESTORE_AUTO=1", s.wardenBin, "restore", "--tag", tag)
		}()
	}
	s.renderRestoreAppLog(w, tag)
}

func (s *server) handleRestoreAppPoll(w http.ResponseWriter, r *http.Request) {
	s.renderRestoreAppLog(w, strings.TrimSpace(r.URL.Query().Get("tag")))
}

func (s *server) renderRestoreAppLog(w http.ResponseWriter, tag string) {
	logText, running, done := restoreAppProc(tag).snapshot()
	render(w, "restore_app_log.html", map[string]any{
		"Tag": tag, "Log": logText, "Running": running, "Done": done,
	})
}
