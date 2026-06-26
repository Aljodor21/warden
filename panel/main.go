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
	root           string // WARDEN_ROOT
	repoCatalogDir string // <root>/catalog — recetas genéricas (solo lectura)
	siteCatalogDir string // <root>/site/catalog — tuyo, gana en empates, ÚNICO destino de escritura
	wardenBin      string
	passwordHash   string // sha256 hex, vacío = sin auth (solo pruebas locales)
	adminSess      *adminSessions
	filesProxy     http.Handler // hacia el contenedor FileBrowser, ver files.go

	// Para calcular tasas entre refrescos del dashboard.
	mu           sync.Mutex
	lastRx       int64
	lastTx       int64
	lastNetTime  time.Time
	prevCores    []CoreStat
	prevCoreAt   time.Time
	netHistory   [40]NetSample
	netHistIdx   int
	netHistFull  bool

	// Estado del backup en segundo plano (puede tardar minutos).
	backupProc bgProcess

	// Estado de 'cloudflare-init' en segundo plano (espera tu login).
	cfInit bgProcess

	// Estado de 'warden runner' en segundo plano (descarga + registro).
	runnerReg bgProcess

	// Estado de 'warden restore' en segundo plano.
	restoreProc bgProcess

	// Estado de preparación de disco (parted + mkfs) en segundo plano.
	diskPrep bgProcess

	// Estado de 'warden reset' en segundo plano (mata todo, incluido este panel).
	resetProc bgProcess

	// Caché de restic snapshots: evita arrancar un contenedor Docker en cada
	// carga de página o acción de backups. TTL 60s; se invalida tras cada backup.
	snapMu       sync.Mutex
	snapCached   []Snapshot
	snapCacheErr string
	snapCacheSize string
	snapCacheAt  time.Time
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
		root:           *root,
		repoCatalogDir: *root + "/catalog",
		siteCatalogDir: *root + "/site/catalog",
		wardenBin:      *wardenBin,
		adminSess:      newAdminSessions(),
		filesProxy:     newFilesProxy(),
	}
	if b, err := os.ReadFile(*passFile); err == nil {
		s.passwordHash = strings.TrimSpace(string(b))
	} else {
		log.Printf("AVISO: no pude leer %s — el panel queda SIN contraseña. Solo para pruebas locales.", *passFile)
	}

	noExtra := func() map[string]any { return map[string]any{} }

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handleDashboard)
	mux.HandleFunc("GET /partials/health", s.handleHealthPartial)
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /catalog", s.handleList)
	mux.HandleFunc("GET /edit/{tag}", s.handleEditForm)
	mux.HandleFunc("POST /edit/{tag}", s.handleEditSave)
	mux.HandleFunc("POST /delete/{tag}", s.requireAdmin("err_inline.html", noExtra, s.handleDeleteApp))
	mux.HandleFunc("POST /delete/{tag}/with-token", s.requireAdmin("err_inline.html", noExtra, s.handleDeleteAppWithToken))
	mux.HandleFunc("GET /new", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/new/deploy", http.StatusSeeOther)
	})
	mux.HandleFunc("GET /new/install", s.handleEditForm)
	mux.HandleFunc("POST /new/install", s.handleEditSave)
	mux.HandleFunc("GET /new/deploy", s.handleNewDeployForm)
	mux.HandleFunc("POST /new/deploy", s.handleNewDeploySave)
	mux.HandleFunc("POST /new/deploy/check-runner", s.handleCheckRunner)
	mux.HandleFunc("POST /runner/register", s.requireAdmin("runner_register_log.html", noExtra, s.handleRunnerRegisterStart))
	mux.HandleFunc("GET /runner/register-log", s.handleRunnerRegisterPoll)
	mux.HandleFunc("POST /catalog/install/{tag}", s.requireAdmin("catalog_install_log.html", noExtra, s.handleCatalogInstallStart))
	mux.HandleFunc("GET /catalog/install-log", s.handleCatalogInstallPoll)
	mux.HandleFunc("POST /backups/restore", s.requireAdmin("restore_log.html", noExtra, s.handleRestoreStart))
	mux.HandleFunc("POST /backups/restore-from", s.requireAdmin("restore_log.html", noExtra, s.handleRestoreFromSnapshot))
	mux.HandleFunc("GET /backups/restore-log", s.handleRestorePoll)
	mux.HandleFunc("POST /publish", s.handlePublish)
	withUsers := func() map[string]any { return map[string]any{"Users": s.nasUsers()} }
	mux.HandleFunc("GET /nas", s.handleNAS)
	mux.HandleFunc("POST /nas/add", s.requireAdmin("nas_fragment.html", withUsers, s.handleNASAdd))
	mux.HandleFunc("POST /nas/del", s.requireAdmin("nas_fragment.html", withUsers, s.handleNASDel))
	mux.HandleFunc("POST /nas/reveal", s.requireAdmin("err_inline.html", noExtra, s.handleNASReveal))
	mux.HandleFunc("POST /admin/unlock", s.handleAdminUnlock)
	mux.HandleFunc("POST /admin/lock", s.handleAdminLock)
	mux.HandleFunc("GET /admin/status", s.handleAdminStatus)
	withSys := func() map[string]any { return map[string]any{"Sys": s.gatherSystemView()} }
	mux.HandleFunc("GET /system", s.handleSystem)
	mux.HandleFunc("POST /system/vpn", s.requireAdmin("system_fragment.html", withSys, s.handleVPNInstall))
	mux.HandleFunc("POST /system/vpn-subnet", s.requireAdmin("system_fragment.html", withSys, s.handleVPNSubnet))
	mux.HandleFunc("POST /system/secrets-init", s.requireAdmin("system_fragment.html", withSys, s.handleSecretsInit))
	mux.HandleFunc("POST /system/secrets-save", s.requireAdmin("system_fragment.html", withSys, s.handleSecretsSave))
	mux.HandleFunc("POST /system/cloudflare-init", s.requireAdmin("cloudflare_log.html", noExtra, s.handleCloudflareInitStart))
	mux.HandleFunc("GET /system/cloudflare-log", s.handleCloudflareInitPoll)
	mux.HandleFunc("POST /system/cloudflare-token", s.requireAdmin("system_fragment.html", withSys, s.handleSaveCloudflareToken))
	mux.HandleFunc("POST /system/reset", s.requireAdmin("system_fragment.html", withSys, s.handleReset))
	mux.HandleFunc("GET /system/reset-log", s.handleResetLog)
	mux.HandleFunc("POST /system/timezone", s.requireAdmin("system_fragment.html", withSys, s.handleSetTimezone))
	mux.HandleFunc("GET /system/mem", s.handleSystemMem)
	mux.HandleFunc("GET /backups/content", s.handleBackupsContent)
	withBackups := func() map[string]any { return map[string]any{"B": s.gatherBackupsView()} }
	mux.HandleFunc("GET /backups", s.handleBackupsPage)
	mux.HandleFunc("GET /about", s.handleAbout)
	mux.HandleFunc("/files/app/", s.handleFilesApp)
	mux.HandleFunc("/files/", s.handleFiles)
	mux.HandleFunc("/files", s.handleFiles)
	mux.HandleFunc("POST /backups/now", s.requireAdmin("backups_fragment.html", withBackups, s.handleBackupNow))
	mux.HandleFunc("GET /backups/now-log", s.handleBackupNowLog)
	mux.HandleFunc("POST /backups/register-timer", s.requireAdmin("backups_fragment.html", withBackups, s.handleRegisterTimer))
	mux.HandleFunc("POST /backups/set-passfile", s.requireAdmin("backups_fragment.html", withBackups, s.handleSetPassfile))
	mux.HandleFunc("POST /backups/disk/mount", s.requireAdmin("backups_fragment.html", withBackups, s.handleDiskMount))
	mux.HandleFunc("POST /backups/disk/unmount", s.requireAdmin("backups_fragment.html", withBackups, s.handleDiskUnmount))
	mux.HandleFunc("POST /backups/disk/prepare", s.requireAdmin("backups_fragment.html", withBackups, s.handleDiskPrepare))
	mux.HandleFunc("GET /backups/disk/prepare-log", s.handleDiskPrepareLog)
	mux.Handle("GET /static/", http.FileServer(http.FS(staticFS)))

	srv := &http.Server{
		Addr:         *addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second, // 'publish' puede tardar un poco
		IdleTimeout:  60 * time.Second,
	}
	log.Printf("warden-panel escuchando en %s (catálogo: %s + %s)", *addr, s.repoCatalogDir, s.siteCatalogDir)
	log.Fatal(srv.ListenAndServe())
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
	var installed, deployed []row
	for _, c := range comps {
		isRunning := c.Container != "" && running[c.Container]
		if !isRunning {
			// El catálogo (la receta) sobrevive a un 'warden reset' a propósito
			// (para no tener que redescribir cada app al reinstalar) — pero acá
			// solo mostramos lo que está REALMENTE instalado ahora, no recetas
			// de algo que ya no corre.
			continue
		}
		rw := row{c, isRunning}
		if c.IsDeployed() {
			deployed = append(deployed, rw)
		} else {
			installed = append(installed, rw)
		}
	}
	render(w, "list.html", map[string]any{
		"Installed": installed, "Deployed": deployed,
		"Page": "catalog", "AdminUnlocked": s.isAdmin(r),
	})
}

// El formulario de "nueva app" original mezclaba dos casos de uso muy
// distintos (instalar una app genérica de warden vs conectar un repo
// propio para CI/CD) en un solo formulario de 16 campos — confuso, sin
// contexto de dónde sacar cada dato. Por pedido de Al, "+ Nueva app" va
// SIEMPRE directo al formulario de CI/CD (es el único caso real que usa):
// /new/install queda vivo en el código para editar a mano si hiciera
// falta, pero no se ofrece en la navegación normal.

func (s *server) handleNewDeployForm(w http.ResponseWriter, r *http.Request) {
	render(w, "new_deploy.html", map[string]any{
		"Page": "catalog", "AdminUnlocked": s.isAdmin(r), "CloudflareSet": cloudflareConfigured(),
		"UsedPorts": s.usedPorts(),
	})
}

func (s *server) handleNewDeploySave(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	tag := strings.TrimSpace(r.FormValue("tag"))
	install := strings.TrimSpace(r.FormValue("install"))
	if tag == "" || strings.ContainsAny(tag, "/. \t") {
		http.Error(w, "el tag es obligatorio y no puede tener espacios, puntos ni barras", http.StatusBadRequest)
		return
	}
	container := strings.TrimSpace(r.FormValue("container"))
	if container == "" {
		container = tag
	}
	port := strings.TrimSpace(r.FormValue("cfport"))
	if owner, used := s.portInUse(port, tag); used {
		http.Error(w, "El puerto "+port+" ya lo usa '"+owner+"' — elegí otro.", http.StatusConflict)
		return
	}
	cfhost := strings.TrimSpace(r.FormValue("cfhost"))
	if cfhost != "" && !cloudflareConfigured() {
		// Defensa en el servidor además del HTML: aunque alguien fuerce el campo,
		// no se guarda un subdominio sin túnel configurado (quedaría inútil).
		cfhost = ""
	}
	c := &Component{
		Tag:       tag,
		Name:      r.FormValue("name"),
		Kind:      "none", // el código vive en su repo — fuera del backup/restore automático a propósito
		Install:   install,
		Container: container,
		CFHost:    cfhost,
		CFPort:    port,
		Note:      "Desplegada vía CI/CD — agregada desde el panel. No entra en backup/restore automático.",
	}
	if err := writeComponentFile(s.siteCatalogDir+"/"+tag+".component", c); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Si tiene subdominio, publicar YA — sin esto, el túnel queda con el
	// ingress viejo hasta que alguien se acuerde de tocar el botón
	// "Publicar" en Catálogo (bug real visto: guardar/editar sin esto deja
	// el subdominio configurado pero sin responder, porque cloudflared
	// nunca se entera del cambio).
	var publishErr error
	if c.CFHost != "" {
		ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
		defer cancel()
		_, publishErr = s.runWarden(ctx, "publish")
	}

	render(w, "new_deploy_done.html", map[string]any{
		"Page": "catalog", "AdminUnlocked": s.isAdmin(r), "Name": c.Name, "Install": c.Install,
		"Tag": c.Tag, "Container": c.Container, "Port": c.CFPort,
		"Published": c.CFHost != "", "PublishErr": publishErr,
	})
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

	// Mismo motivo que en handleNewDeploySave: si tiene subdominio, hay que
	// publicar YA — bug real visto en vivo (Al cambió puerto/contenedor de
	// una app y el subdominio quedó sin responder hasta correr 'publish' a
	// mano). Si falla, no escondo el error en un redirect silencioso.
	if c.CFHost != "" {
		ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
		defer cancel()
		if out, err := s.runWarden(ctx, "publish"); err != nil {
			http.Error(w, "Guardado, pero falló al publicar el túnel: "+out, http.StatusInternalServerError)
			return
		}
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

// handleDeleteApp decide QUÉ hacer al pedir borrar una app. Solo se puede
// borrar lo que VOS agregaste en site/catalog — las apps genéricas de
// warden (Immich, NAS...) viven en el repo compartido, borrarlas
// localmente no haría nada (reaparecerían en el próximo 'git pull').
//
// Si la app tiene subdominio y NO hay token de Cloudflare guardado, no
// asume nada: se DETIENE acá y pide el token (con el link para generarlo),
// dejando la opción de seguir sin él si no se quiere hacer ahora — recién
// con eso resuelto sigue a finishDeleteApp.
func (s *server) handleDeleteApp(w http.ResponseWriter, r *http.Request) {
	tag := r.PathValue("tag")
	path := s.siteCatalogDir + "/" + tag + ".component"
	c, err := parseComponentFile(path)
	if err != nil {
		http.Error(w, "Esa app no está en tu site/catalog — no se puede borrar desde aquí (es una receta genérica de warden, compartida).", http.StatusForbidden)
		return
	}

	skipToken := r.FormValue("skip_token") == "1"
	if c.CFHost != "" && !cloudflareTokenExists() && !skipToken {
		render(w, "delete_need_token.html", map[string]any{
			"Page": "catalog", "AdminUnlocked": s.isAdmin(r),
			"Tag": tag, "Name": c.Name, "Host": c.CFHost,
		})
		return
	}

	s.finishDeleteApp(w, r, tag, path, c)
}

// handleDeleteAppWithToken: viene de la pantalla de "pedí el token" —
// guarda el token (queda disponible para futuras veces, no es de un solo
// uso) y CONTINÚA el borrado ya con todo resuelto.
func (s *server) handleDeleteAppWithToken(w http.ResponseWriter, r *http.Request) {
	tag := r.PathValue("tag")
	token := strings.TrimSpace(r.FormValue("token"))
	if token == "" {
		http.Error(w, "El token no puede estar vacío.", http.StatusBadRequest)
		return
	}
	if err := saveCloudflareToken(token); err != nil {
		http.Error(w, "No pude guardar el token: "+err.Error(), http.StatusInternalServerError)
		return
	}
	path := s.siteCatalogDir + "/" + tag + ".component"
	c, err := parseComponentFile(path)
	if err != nil {
		http.Error(w, "Esa app ya no está en el catálogo.", http.StatusNotFound)
		return
	}
	s.finishDeleteApp(w, r, tag, path, c)
}

// finishDeleteApp hace el trabajo real: borra el archivo, regenera el
// túnel si hacía falta, y borra el registro DNS si hay token disponible.
func (s *server) finishDeleteApp(w http.ResponseWriter, r *http.Request, tag, path string, c *Component) {
	if err := os.Remove(path); err != nil {
		http.Error(w, "No pude borrar el archivo: "+err.Error(), http.StatusInternalServerError)
		return
	}

	var publishOut string
	var publishErr error
	var dnsMsg string
	var dnsErr error
	if c.CFHost != "" {
		ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
		defer cancel()
		publishOut, publishErr = s.runWarden(ctx, "publish")
		if cloudflareTokenExists() {
			dnsMsg, dnsErr = deleteDNSRecord(c.CFHost)
		}
	}

	render(w, "delete_done.html", map[string]any{
		"Page": "catalog", "AdminUnlocked": s.isAdmin(r),
		"Name": c.Name, "HadHost": c.CFHost != "", "Host": c.CFHost,
		"PublishOutput": publishOut, "PublishErr": publishErr,
		"DNSAttempted": cloudflareTokenExists(), "DNSMsg": dnsMsg, "DNSErr": dnsErr,
	})
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
