package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// Variables de entorno (.env) de las apps de CI/CD.
//
// El .env tiene secretos, así que NO vive en el repo de la app: warden lo guarda en
// <root>/site/env/<tag>.env (site/ es privado y está fuera de git), con permiso 0600 y a nombre
// del dueño del repo de warden — el mismo usuario con el que corre el runner, para que el
// workflow de la app lo pueda leer. Si todavía no existe, el panel ofrece partir del ejemplo
// que traiga el repo de la app (.env.docker.example, .env.example, ...).

const maxEnvBytes = 64 << 10

// rawBase se puede cambiar en las pruebas.
var rawBase = "https://raw.githubusercontent.com"

var envExampleNames = []string{".env.docker.example", ".env.example", ".env.sample", "example.env"}

func (s *server) envDir() string { return s.root + "/site/env" }

func (s *server) envPath(tag string) string { return s.envDir() + "/" + tag + ".env" }

// validTag evita que un tag con / o .. apunte fuera de site/env.
func validTag(tag string) bool {
	return tag != "" && !strings.ContainsAny(tag, "/\\. \t\x00")
}

// normalizeEnv deja el texto como un .env limpio: saltos \n y un salto final.
func normalizeEnv(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = strings.TrimRight(text, "\n \t")
	if text == "" {
		return ""
	}
	return text + "\n"
}

// saveEnv escribe el .env de forma atómica y con permisos de archivo privado.
func (s *server) saveEnv(tag, content string) error {
	if !validTag(tag) {
		return errors.New("tag inválido")
	}
	content = normalizeEnv(content)
	if len(content) > maxEnvBytes {
		return fmt.Errorf("el archivo es demasiado grande (máximo %d KB)", maxEnvBytes>>10)
	}
	if strings.ContainsRune(content, 0) {
		return errors.New("el contenido tiene caracteres no válidos")
	}
	dir := s.envDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	uid, gid := s.rootOwner()
	_ = os.Chown(dir, uid, gid)
	tmp, err := os.CreateTemp(dir, ".env-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	_ = os.Chown(tmp.Name(), uid, gid)
	return os.Rename(tmp.Name(), s.envPath(tag))
}

// rootOwner: dueño del repo de warden (el usuario del runner). Si no se puede saber, el actual.
func (s *server) rootOwner() (int, int) {
	if fi, err := os.Stat(s.root); err == nil {
		if st, ok := fi.Sys().(*syscall.Stat_t); ok {
			return int(st.Uid), int(st.Gid)
		}
	}
	return os.Getuid(), os.Getgid()
}

// fetchEnvExample busca el ejemplo de variables en el repo de GitHub de la app.
// Devuelve el contenido y el nombre del archivo; vacío si no hay (o el repo es privado).
func fetchEnvExample(install string) (string, string) {
	owner, repo, ok := parseGitHubRepo(install)
	if !ok {
		return "", ""
	}
	client := &http.Client{Timeout: 8 * time.Second}
	for _, name := range envExampleNames {
		u := fmt.Sprintf("%s/%s/%s/HEAD/%s", rawBase, url.PathEscape(owner), url.PathEscape(repo), name)
		resp, err := client.Get(u)
		if err != nil {
			continue
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, maxEnvBytes+1))
		resp.Body.Close()
		if err != nil || resp.StatusCode != http.StatusOK || len(body) > maxEnvBytes || bytes.IndexByte(body, 0) >= 0 {
			continue
		}
		return string(body), name
	}
	return "", ""
}

// deployedComponent carga una app de CI/CD de site/catalog.
func (s *server) deployedComponent(tag string) (*Component, error) {
	if !validTag(tag) {
		return nil, errors.New("tag inválido")
	}
	c, err := parseComponentFile(filepath.Join(s.siteCatalogDir, tag+".component"))
	if err != nil {
		return nil, errors.New("esa app no está en tu catálogo")
	}
	if !c.IsDeployed() {
		return nil, errors.New("esa app no es de CI/CD")
	}
	return c, nil
}

// withRunnerStatus agrega a los datos de la pantalla de pasos si el repo ya tiene un runner
// registrado en este servidor (entonces no hay nada que registrar).
func withRunnerStatus(data map[string]any, install string) map[string]any {
	if owner, repo, ok := parseGitHubRepo(install); ok {
		if service, found := findRunnerService(owner, repo); found {
			data["RunnerFound"], data["RunnerService"] = true, service
		}
	}
	return data
}

// handleAppSteps: vuelve a mostrar los próximos pasos (Dockerfile, workflow y registro del
// runner) de una app de CI/CD ya guardada.
func (s *server) handleAppSteps(w http.ResponseWriter, r *http.Request) {
	c, err := s.deployedComponent(r.PathValue("tag"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	render(w, "new_deploy_done.html", withRunnerStatus(map[string]any{
		"Page": "catalog", "AdminUnlocked": s.isAdmin(r), "Name": c.Name, "Install": c.Install,
		"Tag": c.Tag, "Container": c.Container, "Port": c.CFPort, "Published": false,
	}, c.Install))
}

// handleEnvForm: muestra el .env guardado, o el ejemplo del repo si todavía no existe.
// Sin modo admin no se muestra nada (son secretos).
func (s *server) handleEnvForm(w http.ResponseWriter, r *http.Request) {
	c, err := s.deployedComponent(r.PathValue("tag"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	data := map[string]any{
		"Page": "catalog", "AdminUnlocked": s.isAdmin(r), "Name": c.Name, "Tag": c.Tag,
		"Path": s.envPath(c.Tag), "Saved": r.URL.Query().Get("saved") == "1",
	}
	if s.isAdmin(r) {
		if b, err := os.ReadFile(s.envPath(c.Tag)); err == nil {
			data["Exists"], data["Content"], data["Source"] = true, string(b), "guardado"
		} else if ex, name := fetchEnvExample(c.Install); ex != "" {
			data["Content"], data["Source"] = ex, name
		}
	}
	render(w, "env_edit.html", data)
}

// handleEnvSave guarda el .env que se editó en el formulario.
func (s *server) handleEnvSave(w http.ResponseWriter, r *http.Request) {
	if !s.isAdmin(r) {
		http.Error(w, "Desbloqueá el modo admin (botón arriba) para guardar variables.", http.StatusForbidden)
		return
	}
	c, err := s.deployedComponent(r.PathValue("tag"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.saveEnv(c.Tag, r.FormValue("content")); err != nil {
		http.Error(w, "No pude guardar: "+err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/app/"+c.Tag+"/env?saved=1", http.StatusSeeOther)
}
