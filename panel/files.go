package main

import (
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// FileEntry: una fila del explorador (carpeta o archivo).
type FileEntry struct {
	Name    string
	Path    string // ruta relativa a dataRoot, para construir links
	IsDir   bool
	Size    string
	ModTime string
}

// safeFilesPath resuelve una ruta RELATIVA pedida por el usuario contra
// dataRoot, y rechaza cualquier intento de salir de ahí (".." o symlinks
// que escapen) — el explorador de archivos solo puede ver lo que warden
// ya gestiona (NAS, Immich, Docmost...), nunca el resto del sistema.
func safeFilesPath(dataRoot, rel string) (string, error) {
	clean := filepath.Clean("/" + rel) // fuerza a que sea absoluto-relativo, neutraliza "../.."
	full := filepath.Join(dataRoot, clean)
	resolved, err := filepath.EvalSymlinks(full)
	if err != nil {
		// El archivo/carpeta puede no existir todavía al solo querer
		// LISTAR un padre — no es un error de seguridad, solo de I/O.
		resolved = full
	}
	rootResolved, err := filepath.EvalSymlinks(dataRoot)
	if err != nil {
		rootResolved = dataRoot
	}
	if resolved != rootResolved && !strings.HasPrefix(resolved, rootResolved+string(filepath.Separator)) {
		return "", fmt.Errorf("ruta fuera de %s", dataRoot)
	}
	return full, nil
}

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

func (s *server) handleFilesBrowse(w http.ResponseWriter, r *http.Request) {
	rel := r.URL.Query().Get("path")
	full, err := safeFilesPath(s.dataRoot, rel)
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	info, err := os.Stat(full)
	if err != nil || !info.IsDir() {
		http.Error(w, "No encuentro esa carpeta.", http.StatusNotFound)
		return
	}
	dirents, err := os.ReadDir(full)
	if err != nil {
		http.Error(w, "No pude leer la carpeta: "+err.Error(), http.StatusInternalServerError)
		return
	}
	var entries []FileEntry
	for _, de := range dirents {
		fi, err := de.Info()
		if err != nil {
			continue
		}
		entries = append(entries, FileEntry{
			Name:    de.Name(),
			Path:    path.Join(filepath.Clean("/"+rel), de.Name()),
			IsDir:   de.IsDir(),
			Size:    humanFileSize(fi.Size()),
			ModTime: fi.ModTime().Format("02 Jan 2006 15:04"),
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir // carpetas primero
		}
		return entries[i].Name < entries[j].Name
	})

	cur := filepath.Clean("/" + rel)
	var parent string
	if cur != "/" {
		parent = path.Dir(cur)
	}
	render(w, "files.html", map[string]any{
		"Page": "files", "AdminUnlocked": s.isAdmin(r),
		"Cur": cur, "Parent": parent, "Entries": entries,
	})
}

func (s *server) handleFilesDownload(w http.ResponseWriter, r *http.Request) {
	rel := r.URL.Query().Get("path")
	full, err := safeFilesPath(s.dataRoot, rel)
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	info, err := os.Stat(full)
	if err != nil || info.IsDir() {
		http.Error(w, "No encuentro ese archivo.", http.StatusNotFound)
		return
	}
	http.ServeFile(w, r, full)
}
