// warden-panel — panel web mínimo para gestionar site/catalog/*.component
// sin consola. Cero dependencias externas (solo librería estándar de Go),
// un solo binario, pensado para correr en máquinas con poca RAM.
//
// Seguridad:
//   - HTTP Basic Auth contra un hash SHA-256 guardado en disco (no texto plano).
//   - Pensado para escuchar SOLO en LAN/Tailscale (lo impone el firewall, no esta app).
//   - Timeouts explícitos en el servidor (Go no los pone por defecto).
//   - html/template escapa todo por defecto — sin riesgo de XSS por datos del catálogo.
package main

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"flag"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

//go:embed templates/*.html
var templatesFS embed.FS

var tmpl = template.Must(template.ParseFS(templatesFS, "templates/*.html"))

type server struct {
	catalogDir   string
	wardenBin    string
	passwordHash string // sha256 hex, vacío = sin auth (solo para pruebas locales)
}

func main() {
	addr := flag.String("addr", "127.0.0.1:7890", "dirección donde escuchar")
	catalogDir := flag.String("catalog", "/home/alejo/proyectos/warden/site/catalog", "carpeta de site/catalog")
	wardenBin := flag.String("warden", "/usr/local/bin/warden", "ruta del binario warden")
	passFile := flag.String("passfile", "/etc/warden/panel.passwd", "archivo con el hash sha256 de la clave")
	flag.Parse()

	s := &server{catalogDir: *catalogDir, wardenBin: *wardenBin}
	if b, err := os.ReadFile(*passFile); err == nil {
		s.passwordHash = strings.TrimSpace(string(b))
	} else {
		log.Printf("AVISO: no pude leer %s — el panel queda SIN contraseña. Solo para pruebas locales.", *passFile)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.handleList)
	mux.HandleFunc("GET /edit/{tag}", s.handleEditForm)
	mux.HandleFunc("POST /edit/{tag}", s.handleEditSave)
	mux.HandleFunc("GET /new", s.handleEditForm)
	mux.HandleFunc("POST /new", s.handleEditSave)
	mux.HandleFunc("POST /publish", s.handlePublish)

	srv := &http.Server{
		Addr:         *addr,
		Handler:      s.withAuth(mux),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second, // 'publish' puede tardar un poco
		IdleTimeout:  60 * time.Second,
	}
	log.Printf("warden-panel escuchando en %s (catálogo: %s)", *addr, *catalogDir)
	log.Fatal(srv.ListenAndServe())
}

func (s *server) withAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.passwordHash == "" {
			next.ServeHTTP(w, r)
			return
		}
		_, pass, ok := r.BasicAuth()
		if !ok || !checkPassword(pass, s.passwordHash) {
			w.Header().Set("WWW-Authenticate", `Basic realm="warden-panel"`)
			http.Error(w, "no autorizado", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func checkPassword(given, wantHashHex string) bool {
	sum := sha256.Sum256([]byte(given))
	gotHex := hex.EncodeToString(sum[:])
	return subtle.ConstantTimeCompare([]byte(gotHex), []byte(wantHashHex)) == 1
}

func (s *server) handleList(w http.ResponseWriter, r *http.Request) {
	comps, err := listComponents(s.catalogDir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	running := runningContainers()
	type row struct {
		*Component
		Running bool
	}
	var rows []row
	for _, c := range comps {
		rows = append(rows, row{c, c.Container != "" && running[c.Container]})
	}
	render(w, "list.html", map[string]any{"Rows": rows})
}

func (s *server) handleEditForm(w http.ResponseWriter, r *http.Request) {
	tag := r.PathValue("tag")
	c := &Component{}
	if tag != "" {
		var err error
		c, err = parseComponentFile(s.catalogDir + "/" + tag + ".component")
		if err != nil {
			http.Error(w, "no encontré ese componente: "+err.Error(), http.StatusNotFound)
			return
		}
		c.Tag = tag
	}
	render(w, "edit.html", map[string]any{"C": c, "IsNew": tag == ""})
}

func (s *server) handleEditSave(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	tag := r.PathValue("tag")
	if tag == "" {
		tag = strings.TrimSpace(r.FormValue("tag"))
	}
	if tag == "" || strings.ContainsAny(tag, "/. \t") {
		http.Error(w, "el tag es obligatorio y no puede tener espacios, puntos ni barras", http.StatusBadRequest)
		return
	}

	c := &Component{
		Tag:         tag,
		Comment:     r.FormValue("comment"),
		Name:        r.FormValue("name"),
		Kind:        r.FormValue("kind"),
		Paths:       splitLines(r.FormValue("paths")),
		Excludes:    splitLines(r.FormValue("excludes")),
		DBType:      r.FormValue("dbtype"),
		DBContainer: r.FormValue("dbcontainer"),
		DBName:      r.FormValue("dbname"),
		DBUser:      r.FormValue("dbuser"),
		Install:     r.FormValue("install"),
		Container:   r.FormValue("container"),
		Secrets:     splitLines(r.FormValue("secrets")),
		Icon:        r.FormValue("icon"),
		CFHost:      r.FormValue("cfhost"),
		CFPort:      r.FormValue("cfport"),
		Note:        r.FormValue("note"),
	}
	if err := writeComponentFile(s.catalogDir+"/"+tag+".component", c); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *server) handlePublish(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sudo", s.wardenBin, "publish")
	out, err := cmd.CombinedOutput()
	status := "OK"
	if err != nil {
		status = "ERROR: " + err.Error()
	}
	render(w, "publish.html", map[string]any{"Output": string(out), "Status": status})
}

func splitLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func runningContainers() map[string]bool {
	out, err := exec.Command("docker", "ps", "--format", "{{.Names}}").Output()
	m := map[string]bool{}
	if err != nil {
		return m
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			m[line] = true
		}
	}
	return m
}

func render(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
