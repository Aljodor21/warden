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
// nunca expuesto directo a la LAN. FileBrowser corre con --baseurl /files
// (ver stacks/filebrowser/docker-compose.yml) para que sus propios
// links/assets coincidan con este prefijo.
func newFilesProxy() http.Handler {
	target, err := url.Parse("http://127.0.0.1:8095")
	if err != nil {
		panic(err) // URL fija y válida — solo fallaría por un typo en el código
	}
	return httputil.NewSingleHostReverseProxy(target)
}

// handleFiles: "Archivos" en el panel. Si FileBrowser no está instalado o
// no está corriendo, se ofrece instalarlo CON UN CLICK, ahí mismo — sin
// mandar a otra sección a buscarlo (eso fue justo la confusión real: el
// catálogo general no es donde la gente espera resolver esto).
func (s *server) handleFiles(w http.ResponseWriter, r *http.Request) {
	if !runningContainers()["filebrowser"] {
		render(w, "files_install.html", map[string]any{
			"Page": "files", "AdminUnlocked": s.isAdmin(r),
		})
		return
	}
	s.filesProxy.ServeHTTP(w, r)
}
