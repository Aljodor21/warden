package main

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
)

// humanFileSize: usado por el selector de "corrida de backup" (backups.go)
// para mostrar el tamaño de cada snapshot de forma legible.
func humanFileSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

// newFilesProxy: reverse proxy hacia el contenedor FileBrowser
// (catalog/filebrowser), publicado solo en loopback (127.0.0.1:8095) y
// nunca expuesto directo a la LAN. FileBrowser corre con --baseurl
// /files/app y --noauth (ver stacks/filebrowser/docker-compose.yml): sin
// login propio, porque llegar hasta acá YA exigió el admin lock del
// panel — un segundo candado encima sería redundante.
func newFilesProxy() http.Handler {
	target, err := url.Parse("http://127.0.0.1:8095")
	if err != nil {
		panic(err) // URL fija y válida — solo fallaría por un typo en el código
	}
	return httputil.NewSingleHostReverseProxy(target)
}

// handleFiles: "Archivos" en el panel — la página con el menú de warden y
// un iframe apuntando a /files/app/ (FileBrowser real). El iframe es
// deliberado: FileBrowser es una SPA que reemplazaría toda la pantalla si
// se navegara directo a él, y volver al dashboard desde varias carpetas
// adentro significaba apretar "atrás" muchas veces — con el menú siempre
// visible arriba, volver es un solo click sin importar dónde estés
// navegando adentro.
func (s *server) handleFiles(w http.ResponseWriter, r *http.Request) {
	if !s.isAdmin(r) {
		render(w, "files_locked.html", map[string]any{"Page": "files", "AdminUnlocked": false})
		return
	}
	if !runningContainers()["filebrowser"] {
		render(w, "files_install.html", map[string]any{"Page": "files", "AdminUnlocked": true})
		return
	}
	render(w, "files.html", map[string]any{"Page": "files", "AdminUnlocked": true})
}

// handleFilesApp: el iframe de Archivos apunta acá — el proxy real hacia
// FileBrowser. Mismo candado que handleFiles (entrar directo a esta ruta
// sin pasar por /files no debe esquivar el admin lock).
func (s *server) handleFilesApp(w http.ResponseWriter, r *http.Request) {
	if !s.isAdmin(r) {
		http.Error(w, "Desbloqueá el modo admin en el panel.", http.StatusForbidden)
		return
	}
	s.filesProxy.ServeHTTP(w, r)
}
