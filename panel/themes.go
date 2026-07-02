package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func (s *server) handleThemes(w http.ResponseWriter, r *http.Request) {
	render(w, "themes.html", map[string]any{"Page": "themes"})
}

// handleServerImages lista archivos de imagen en un directorio del servidor.
func (s *server) handleServerImages(w http.ResponseWriter, r *http.Request) {
	dir := filepath.Clean(r.URL.Query().Get("dir"))
	if dir == "" || dir == "." {
		dir = "/srv/warden/wallpapers"
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}

	exts := map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".webp": true, ".gif": true}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && exts[strings.ToLower(filepath.Ext(e.Name()))] {
			files = append(files, e.Name())
		}
	}
	if files == nil {
		files = []string{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"dir": dir, "files": files})
}

// handleServerImageFile sirve un archivo de imagen individual del servidor.
// dir + name se validan para evitar path traversal.
func (s *server) handleServerImageFile(w http.ResponseWriter, r *http.Request) {
	dir := filepath.Clean(r.URL.Query().Get("dir"))
	name := r.URL.Query().Get("name")

	// name no puede contener separadores — solo el nombre base del archivo
	if dir == "" || name == "" || name != filepath.Base(name) {
		http.NotFound(w, r)
		return
	}

	http.ServeFile(w, r, filepath.Join(dir, name))
}
