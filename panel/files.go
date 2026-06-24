package main

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// filesRoot: el explorador de archivos del panel gestiona SOLO la carpeta
// del NAS — Al fue explícito en no querer ver Immich/Docmost mezclados ahí,
// solo lo que comparte por archivos sueltos (fotos/documentos no son lo
// mismo que el storage interno de una app).
func (s *server) filesRoot() string {
	return filepath.Join(s.dataRoot, "nas")
}

// FileEntry: una fila del explorador (carpeta o archivo).
type FileEntry struct {
	Name    string
	Path    string // ruta relativa a dataRoot, para construir links
	IsDir   bool
	IsImage bool // para mostrar una miniatura real, estilo galería
	Size    string
	ModTime string
}

func looksLikeImage(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp", ".heic", ".avif":
		return true
	}
	return false
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
	full, err := safeFilesPath(s.filesRoot(), rel)
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
			IsImage: !de.IsDir() && looksLikeImage(de.Name()),
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
		"Err": r.URL.Query().Get("err"),
	})
}

func (s *server) handleFilesDownload(w http.ResponseWriter, r *http.Request) {
	rel := r.URL.Query().Get("path")
	full, err := safeFilesPath(s.filesRoot(), rel)
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

// filesRedirect vuelve a la carpeta dada, opcionalmente con un mensaje de
// error — todo por redirect normal (sin JS, sin HTMX) para que esto siga
// siendo lo más simple y liviano posible.
func filesRedirect(w http.ResponseWriter, r *http.Request, rel, errMsg string) {
	q := url.Values{"path": {rel}}
	if errMsg != "" {
		q.Set("err", errMsg)
	}
	http.Redirect(w, r, "/files?"+q.Encode(), http.StatusSeeOther)
}

// safeFileName: el nombre de un archivo subido o de una carpeta/rename
// nuevo nunca debe poder inyectar una ruta (ej. "../../etc/passwd" como
// "nombre" en vez de como "path") — nos quedamos solo con el componente
// final, sin separadores.
func safeFileName(name string) string {
	name = strings.TrimSpace(name)
	name = filepath.Base(filepath.Clean("/" + name))
	if name == "" || name == "." || name == "/" {
		return ""
	}
	return name
}

// requireAdminWrite: las operaciones de escritura del explorador (subir,
// crear carpeta, borrar, renombrar) modifican archivos reales del NAS — se
// exigen detrás del mismo lock de admin que el resto de acciones
// destructivas del panel, pero sin forzar el patrón de "re-renderizar la
// página completa" (esto son POSTs de formulario simple, no fragments).
func (s *server) requireAdminWrite(w http.ResponseWriter, r *http.Request) bool {
	if !s.isAdmin(r) {
		http.Error(w, "Desbloqueá el modo admin para hacer esto.", http.StatusForbidden)
		return false
	}
	return true
}

func (s *server) handleFilesUpload(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdminWrite(w, r) {
		return
	}
	rel := r.URL.Query().Get("path")
	dir, err := safeFilesPath(s.filesRoot(), rel)
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		http.Error(w, "Carpeta destino inválida.", http.StatusBadRequest)
		return
	}
	// 2 GiB por archivo: generoso para fotos/documentos del NAS, sin
	// dejar la subida totalmente sin límite.
	if err := r.ParseMultipartForm(2 << 30); err != nil {
		filesRedirect(w, r, rel, "Subida inválida: "+err.Error())
		return
	}
	files := r.MultipartForm.File["file"]
	if len(files) == 0 {
		filesRedirect(w, r, rel, "No se eligió ningún archivo.")
		return
	}
	for _, fh := range files {
		name := safeFileName(fh.Filename)
		if name == "" {
			continue
		}
		src, err := fh.Open()
		if err != nil {
			filesRedirect(w, r, rel, "No pude leer el archivo subido: "+err.Error())
			return
		}
		dst, err := os.OpenFile(filepath.Join(dir, name), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			src.Close()
			filesRedirect(w, r, rel, "No pude guardar '"+name+"': "+err.Error())
			return
		}
		_, copyErr := io.Copy(dst, src)
		src.Close()
		dst.Close()
		if copyErr != nil {
			filesRedirect(w, r, rel, "Falló al guardar '"+name+"': "+copyErr.Error())
			return
		}
	}
	filesRedirect(w, r, rel, "")
}

func (s *server) handleFilesMkdir(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdminWrite(w, r) {
		return
	}
	rel := r.FormValue("path")
	name := safeFileName(r.FormValue("name"))
	if name == "" {
		filesRedirect(w, r, rel, "Poné un nombre para la carpeta.")
		return
	}
	dir, err := safeFilesPath(s.filesRoot(), rel)
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	if err := os.Mkdir(filepath.Join(dir, name), 0755); err != nil {
		filesRedirect(w, r, rel, "No pude crear la carpeta: "+err.Error())
		return
	}
	filesRedirect(w, r, rel, "")
}

func (s *server) handleFilesDelete(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdminWrite(w, r) {
		return
	}
	rel := r.FormValue("path")
	full, err := safeFilesPath(s.filesRoot(), rel)
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	// No se permite borrar la raíz del NAS entera por esta vía.
	if filepath.Clean(full) == filepath.Clean(s.filesRoot()) {
		http.Error(w, "No se puede borrar la raíz del NAS.", http.StatusForbidden)
		return
	}
	parent := path.Dir(filepath.Clean("/" + rel))
	if err := os.RemoveAll(full); err != nil {
		filesRedirect(w, r, parent, "No pude borrar: "+err.Error())
		return
	}
	filesRedirect(w, r, parent, "")
}

func (s *server) handleFilesRename(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdminWrite(w, r) {
		return
	}
	rel := r.FormValue("path")
	newName := safeFileName(r.FormValue("name"))
	parent := path.Dir(filepath.Clean("/" + rel))
	if newName == "" {
		filesRedirect(w, r, parent, "Poné un nombre nuevo.")
		return
	}
	full, err := safeFilesPath(s.filesRoot(), rel)
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	if filepath.Clean(full) == filepath.Clean(s.filesRoot()) {
		http.Error(w, "No se puede renombrar la raíz del NAS.", http.StatusForbidden)
		return
	}
	newFull := filepath.Join(filepath.Dir(full), newName)
	if err := os.Rename(full, newFull); err != nil {
		filesRedirect(w, r, parent, "No pude renombrar: "+err.Error())
		return
	}
	filesRedirect(w, r, parent, "")
}
