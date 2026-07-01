package main

import (
	"net/http"
	"os"
	"os/exec"
	"strings"
)

// La "tienda" muestra una grilla de apps (plantillas de Portainer, ver
// store.go) con su ícono. Al instalar, genera el compose de esa app y se lo
// pasa al importador (modules/import.sh), que lo adapta al formato de warden y
// lo instala. También deja pegar un compose/URL propio para lo que no esté.

func (s *server) handleTienda(w http.ResponseWriter, r *http.Request) {
	apps, err := s.storeApps()
	data := map[string]any{"Page": "tienda", "AdminUnlocked": s.isAdmin(r), "Apps": apps}
	if err != nil {
		data["FetchErr"] = "No pude bajar la lista de apps (¿hay internet?): " + err.Error()
	}
	render(w, "tienda.html", data)
}

// handleTiendaInstall: instalar una app de la grilla (por su tag).
func (s *server) handleTiendaInstall(w http.ResponseWriter, r *http.Request) {
	tag := strings.TrimSpace(r.FormValue("tag"))
	if tag == "" {
		render(w, "tienda_log.html", map[string]any{"Err": "Falta el tag de la app."})
		return
	}
	t := s.storeTemplateByTag(tag)
	if t == nil {
		render(w, "tienda_log.html", map[string]any{"Err": "No encontré esa app en la tienda."})
		return
	}
	s.doImportInstall(w, composeFromTemplate(*t, tag), "", tag)
}

// handleTiendaImport: pegar/enlazar un compose propio.
func (s *server) handleTiendaImport(w http.ResponseWriter, r *http.Request) {
	tag := strings.TrimSpace(r.FormValue("tag"))
	url := strings.TrimSpace(r.FormValue("url"))
	compose := strings.TrimSpace(r.FormValue("compose"))
	if tag == "" {
		render(w, "tienda_log.html", map[string]any{"Err": "Poné un nombre/tag para la app."})
		return
	}
	if url == "" && compose == "" {
		render(w, "tienda_log.html", map[string]any{"Err": "Pegá el compose o una URL."})
		return
	}
	s.doImportInstall(w, compose, url, tag)
}

// doImportInstall corre 'warden import' y, si sale bien, 'install-component' en
// segundo plano (vuelca el log en vivo). source: si hay composeText lo escribe
// a un temp; si no, usa la URL.
func (s *server) doImportInstall(w http.ResponseWriter, composeText, url, tag string) {
	source := url
	var tmpPath string
	if composeText != "" {
		f, err := os.CreateTemp("", "warden-import-*.yml")
		if err != nil {
			render(w, "tienda_log.html", map[string]any{"Err": "No pude crear el archivo temporal: " + err.Error()})
			return
		}
		_, _ = f.WriteString(composeText)
		_ = f.Close()
		source, tmpPath = f.Name(), f.Name()
	}
	if source == "" {
		render(w, "tienda_log.html", map[string]any{"Err": "Falta el compose o la URL."})
		return
	}

	if s.tiendaProc.start() {
		// Argumentos directos a exec (sin shell) — sin inyección aunque tag/url
		// vengan del formulario.
		go func() {
			defer s.tiendaProc.finish()
			if tmpPath != "" {
				defer os.Remove(tmpPath)
			}
			ctx, cancel := bgCtx3min()
			defer cancel()
			imp := exec.CommandContext(ctx, "sudo", s.wardenBin, "import", source, tag)
			imp.Stdout, imp.Stderr = &s.tiendaProc, &s.tiendaProc
			if err := imp.Run(); err != nil {
				return // el error del importador ya quedó en el log
			}
			ins := exec.CommandContext(ctx, "sudo", s.wardenBin, "install-component", tag)
			ins.Stdout, ins.Stderr = &s.tiendaProc, &s.tiendaProc
			_ = ins.Run()
		}()
	}
	logText, running, done := s.tiendaProc.snapshot()
	render(w, "tienda_log.html", map[string]any{"Log": logText, "Running": running, "Done": done})
}

func (s *server) handleTiendaImportLog(w http.ResponseWriter, r *http.Request) {
	logText, running, done := s.tiendaProc.snapshot()
	render(w, "tienda_log.html", map[string]any{"Log": logText, "Running": running, "Done": done})
}
