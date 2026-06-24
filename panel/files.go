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

// filesProxy: la sección "Archivos" del panel no es un explorador propio —
// es un reverse proxy hacia el contenedor FileBrowser (catalog/filebrowser),
// publicado solo en loopback (127.0.0.1:8095) y nunca expuesto directo a la
// LAN. Al entra siempre por /files en el panel (mismo puerto de siempre);
// por detrás, esto reenvía al contenedor real. FileBrowser corre con
// --baseurl /files (ver stacks/filebrowser/docker-compose.yml) para que sus
// propios links/assets coincidan con este prefijo.
func newFilesProxy() http.Handler {
	target, err := url.Parse("http://127.0.0.1:8095")
	if err != nil {
		panic(err) // URL fija y válida — solo fallaría por un typo en el código
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		http.Error(w, "FileBrowser no está disponible — ¿está instalado y corriendo? (Catálogo → FileBrowser)", http.StatusBadGateway)
	}
	return proxy
}
