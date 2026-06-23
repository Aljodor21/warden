// warden-panel — panel web mínimo para gestionar warden sin consola.
// Stack: Go + html/template + HTMX + Alpine + CSS nativo (todo embebido en el
// binario; cero Node, cero build, cero dependencia de internet en runtime).
//
// Seguridad:
//   - HTTP Basic Auth contra un hash SHA-256 en disco (no texto plano).
//   - Pensado para escuchar SOLO en LAN/Tailscale (lo impone el firewall).
//   - Timeouts explícitos (Go no los pone por defecto).
//   - html/template escapa todo por defecto.
package main

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

//go:embed templates/*.html
var templatesFS embed.FS

//go:embed static/*
var staticFS embed.FS

// 'dict' deja armar el mapa de datos para un sub-template en una sola línea
// desde dentro de otro template (ej. pasarle el estado de admin a "nav").
var tmpl = template.Must(template.New("").Funcs(template.FuncMap{
	"dict": func(pairs ...any) (map[string]any, error) {
		if len(pairs)%2 != 0 {
			return nil, fmt.Errorf("dict: número impar de argumentos")
		}
		m := make(map[string]any, len(pairs)/2)
		for i := 0; i < len(pairs); i += 2 {
			key, ok := pairs[i].(string)
			if !ok {
				return nil, fmt.Errorf("dict: la clave debe ser string")
			}
			m[key] = pairs[i+1]
		}
		return m, nil
	},
}).ParseFS(templatesFS, "templates/*.html"))

type server struct {
	repoCatalogDir string // <root>/catalog — recetas genéricas (solo lectura)
	siteCatalogDir string // <root>/site/catalog — tuyo, gana en empates, ÚNICO destino de escritura
	wardenBin      string
	passwordHash   string // sha256 hex, vacío = sin auth (solo pruebas locales)
	adminSess      *adminSessions

	// Para calcular la tasa de red entre refrescos del dashboard.
	mu          sync.Mutex
	lastRx      int64
	lastTx      int64
	lastNetTime time.Time
}

// catalogDirs: orden de prioridad igual a lib/catalog.sh (repo primero, site
// después — si un tag se repite, site gana).
func (s *server) catalogDirs() []string {
	return []string{s.repoCatalogDir, s.siteCatalogDir}
}

func main() {
	addr := flag.String("addr", "0.0.0.0:7890", "dirección donde escuchar")
	root := flag.String("root", "/home/alejo/proyectos/warden", "raíz del repo de warden (WARDEN_ROOT)")
	wardenBin := flag.String("warden", "/usr/local/bin/warden", "ruta del binario warden")
	passFile := flag.String("passfile", "/etc/warden/panel.passwd", "archivo con el hash sha256 de la clave")
	flag.Parse()

	s := &server{
		repoCatalogDir: *root + "/catalog",
		siteCatalogDir: *root + "/site/catalog",
		wardenBin:      *wardenBin,
		adminSess:      newAdminSessions(),
	}
	if b, err := os.ReadFile(*passFile); err == nil {
		s.passwordHash = strings.TrimSpace(string(b))
	} else {
		log.Printf("AVISO: no pude leer %s — el panel queda SIN contraseña. Solo para pruebas locales.", *passFile)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handleDashboard)
	mux.HandleFunc("GET /partials/health", s.handleHealthPartial)
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /catalog", s.handleList)
	mux.HandleFunc("GET /edit/{tag}", s.handleEditForm)
	mux.HandleFunc("POST /edit/{tag}", s.handleEditSave)
	mux.HandleFunc("GET /new", s.handleEditForm)
	mux.HandleFunc("POST /new", s.handleEditSave)
	mux.HandleFunc("POST /publish", s.handlePublish)
	withUsers := func() map[string]any { return map[string]any{"Users": s.nasUsers()} }
	noExtra := func() map[string]any { return map[string]any{} }
	mux.HandleFunc("GET /nas", s.handleNAS)
	mux.HandleFunc("POST /nas/add", s.requireAdmin("nas_fragment.html", withUsers, s.handleNASAdd))
	mux.HandleFunc("POST /nas/del", s.requireAdmin("nas_fragment.html", withUsers, s.handleNASDel))
	mux.HandleFunc("POST /nas/reveal", s.requireAdmin("err_inline.html", noExtra, s.handleNASReveal))
	mux.HandleFunc("POST /admin/unlock", s.handleAdminUnlock)
	mux.HandleFunc("POST /admin/lock", s.handleAdminLock)
	mux.HandleFunc("GET /admin/status", s.handleAdminStatus)
	mux.Handle("GET /static/", http.FileServer(http.FS(staticFS)))

	srv := &http.Server{
		Addr:         *addr,
		Handler:      s.withAuth(mux),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second, // 'publish' puede tardar un poco
		IdleTimeout:  60 * time.Second,
	}
	log.Printf("warden-panel escuchando en %s (catálogo: %s + %s)", *addr, s.repoCatalogDir, s.siteCatalogDir)
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

// netRates calcula bytes/seg comparando con la muestra anterior.
func (s *server) netRates(h Health) (down, up float64) {
	var rx, tx int64
	for _, n := range h.Nets {
		rx += n.Rx
		tx += n.Tx
	}
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.lastNetTime.IsZero() {
		dt := now.Sub(s.lastNetTime).Seconds()
		if dt > 0 {
			down = float64(rx-s.lastRx) / dt
			up = float64(tx-s.lastTx) / dt
			if down < 0 {
				down = 0
			}
			if up < 0 {
				up = 0
			}
		}
	}
	s.lastRx, s.lastTx, s.lastNetTime = rx, tx, now
	return
}

func (s *server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	render(w, "dashboard.html", map[string]any{"Page": "dashboard", "AdminUnlocked": s.isAdmin(r)})
}

func (s *server) handleHealthPartial(w http.ResponseWriter, r *http.Request) {
	h := gatherHealth()
	down, up := s.netRates(h)
	render(w, "health_fragment.html", s.buildHealthView(h, down, up))
}

func (s *server) handleList(w http.ResponseWriter, r *http.Request) {
	comps, err := listComponentsMerged(s.catalogDirs())
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
	render(w, "list.html", map[string]any{"Rows": rows, "Page": "catalog", "AdminUnlocked": s.isAdmin(r)})
}

func (s *server) handleEditForm(w http.ResponseWriter, r *http.Request) {
	tag := r.PathValue("tag")
	c := &Component{}
	if tag != "" {
		comps, err := listComponentsMerged(s.catalogDirs())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		found := false
		for _, cc := range comps {
			if cc.Tag == tag {
				c, found = cc, true
				break
			}
		}
		if !found {
			http.Error(w, "no encontré ese componente: "+tag, http.StatusNotFound)
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
	// SIEMPRE en site/catalog — nunca en catalog/ (eso es del repo genérico,
	// se actualiza con git pull, no se debe pisar con cambios locales).
	if err := writeComponentFile(s.siteCatalogDir+"/"+tag+".component", c); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/catalog", http.StatusSeeOther)
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
