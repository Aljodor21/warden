package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeEnv(t *testing.T) {
	got := normalizeEnv("A=1\r\nB=2\r\n\r\n  \n")
	if got != "A=1\nB=2\n" {
		t.Errorf("normalizeEnv = %q", got)
	}
	if normalizeEnv("  \n\r\n") != "" {
		t.Error("un texto en blanco debería quedar vacío")
	}
}

func TestValidTag(t *testing.T) {
	for _, bad := range []string{"", "../x", "a/b", "a.b", "a b", "a\x00"} {
		if validTag(bad) {
			t.Errorf("validTag(%q) debería ser falso", bad)
		}
	}
	if !validTag("camino-del-guerrero") {
		t.Error("un tag normal debería ser válido")
	}
}

func TestSaveEnvPermissionsAndContent(t *testing.T) {
	s := &server{root: t.TempDir()}
	if err := s.saveEnv("mi-app", "SECRET=abc\r\nDEBUG=False"); err != nil {
		t.Fatalf("saveEnv: %v", err)
	}
	path := s.envPath("mi-app")
	b, err := os.ReadFile(path)
	if err != nil || string(b) != "SECRET=abc\nDEBUG=False\n" {
		t.Fatalf("contenido = %q (%v)", b, err)
	}
	fi, _ := os.Stat(path)
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("el .env debe ser 0600, es %v", fi.Mode().Perm())
	}
	di, _ := os.Stat(s.envDir())
	if di.Mode().Perm() != 0o700 {
		t.Errorf("la carpeta debe ser 0700, es %v", di.Mode().Perm())
	}
	// No deben quedar temporales.
	left, _ := filepath.Glob(filepath.Join(s.envDir(), ".env-*.tmp"))
	if len(left) != 0 {
		t.Errorf("quedaron temporales: %v", left)
	}
	if err := s.saveEnv("../afuera", "X=1"); err == nil {
		t.Error("un tag con .. no debería poder guardarse")
	}
	if err := s.saveEnv("otra", strings.Repeat("A", maxEnvBytes+10)); err == nil {
		t.Error("un archivo demasiado grande debería rechazarse")
	}
}

func TestFetchEnvExample(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/Aljodor21/camino/HEAD/.env.example" {
			w.Write([]byte("SECRET_KEY=cambiar\n"))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	old := rawBase
	rawBase = srv.URL
	defer func() { rawBase = old }()

	got, name := fetchEnvExample("https://github.com/Aljodor21/camino")
	if got != "SECRET_KEY=cambiar\n" || name != ".env.example" {
		t.Errorf("fetchEnvExample = %q, %q", got, name)
	}
	if got, _ := fetchEnvExample("https://github.com/Aljodor21/sin-ejemplo"); got != "" {
		t.Error("sin ejemplo debería devolver vacío")
	}
	if got, _ := fetchEnvExample("no es una url"); got != "" {
		t.Error("una url inválida debería devolver vacío")
	}
}

func TestEnvPageLockedAndUnlocked(t *testing.T) {
	render1 := func(data map[string]any) string {
		var buf bytes.Buffer
		if err := tmpl.ExecuteTemplate(&buf, "env_edit.html", data); err != nil {
			t.Fatalf("render falló: %v", err)
		}
		return buf.String()
	}
	locked := render1(map[string]any{"Page": "catalog", "AdminUnlocked": false, "Name": "Mi App", "Tag": "mi-app",
		"Path": "/x/mi-app.env", "Content": "SECRET=abc"})
	if strings.Contains(locked, "SECRET=abc") || strings.Contains(locked, "<textarea") {
		t.Error("sin modo admin no debe mostrarse el contenido ni el formulario")
	}
	open := render1(map[string]any{"Page": "catalog", "AdminUnlocked": true, "Name": "Mi App", "Tag": "mi-app",
		"Path": "/x/mi-app.env", "Content": "SECRET=abc", "Source": ".env.example"})
	if !strings.Contains(open, "SECRET=abc") || !strings.Contains(open, `action="/app/mi-app/env"`) {
		t.Error("con modo admin debe mostrarse el formulario con el contenido")
	}
}

// Recorrido completo: guardar desde el formulario y volver a abrirlo; sin modo admin se rechaza.
func TestEnvHandlersRoundTrip(t *testing.T) {
	root := t.TempDir()
	s := &server{root: root, siteCatalogDir: root + "/site/catalog", adminSess: newAdminSessions()}
	if err := os.MkdirAll(s.siteCatalogDir, 0o755); err != nil {
		t.Fatal(err)
	}
	c := &Component{Tag: "mi-app", Name: "Mi App", Kind: "none", Install: "https://github.com/u/mi-app", Container: "mi-app"}
	if err := writeComponentFile(s.siteCatalogDir+"/mi-app.component", c); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /app/{tag}/env", s.handleEnvForm)
	mux.HandleFunc("POST /app/{tag}/env", s.handleEnvSave)
	mux.HandleFunc("GET /app/{tag}/steps", s.handleAppSteps)

	// Sin clave configurada el panel no tiene candado (modo de pruebas): guarda y relee.
	post := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/app/mi-app/env", strings.NewReader("content=SECRET%3Dxyz"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	mux.ServeHTTP(post, req)
	if post.Code != http.StatusSeeOther {
		t.Fatalf("guardar devolvió %d: %s", post.Code, post.Body.String())
	}
	get := httptest.NewRecorder()
	mux.ServeHTTP(get, httptest.NewRequest("GET", "/app/mi-app/env", nil))
	if !strings.Contains(get.Body.String(), "SECRET=xyz") {
		t.Error("el formulario debería mostrar lo que se guardó")
	}
	steps := httptest.NewRecorder()
	mux.ServeHTTP(steps, httptest.NewRequest("GET", "/app/mi-app/steps", nil))
	if !strings.Contains(steps.Body.String(), `hx-post="/runner/register"`) {
		t.Error("los pasos deberían incluir el formulario del runner")
	}

	// Con candado y sin sesión de admin: no guarda y no muestra nada.
	s.passwordHash = sha256Hex("clave")
	denied := httptest.NewRecorder()
	req2 := httptest.NewRequest("POST", "/app/mi-app/env", strings.NewReader("content=OTRO%3D1"))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	mux.ServeHTTP(denied, req2)
	if denied.Code != http.StatusForbidden {
		t.Errorf("sin admin debería ser 403, fue %d", denied.Code)
	}
	view := httptest.NewRecorder()
	mux.ServeHTTP(view, httptest.NewRequest("GET", "/app/mi-app/env", nil))
	if strings.Contains(view.Body.String(), "SECRET=xyz") {
		t.Error("sin admin no debe verse el contenido")
	}
	// Una app que no es de CI/CD o no existe da 404.
	missing := httptest.NewRecorder()
	mux.ServeHTTP(missing, httptest.NewRequest("GET", "/app/no-existe/env", nil))
	if missing.Code != http.StatusNotFound {
		t.Errorf("app inexistente debería ser 404, fue %d", missing.Code)
	}
}
