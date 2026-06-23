package main

import (
	"context"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

// runWarden ejecuta 'warden <args...>' (el proceso del panel ya corre como
// root via systemd, así que no hace falta 'sudo' — pero lo agregamos igual:
// si algún día el panel corre como usuario normal, sigue funcionando).
func (s *server) runWarden(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "sudo", append([]string{s.wardenBin}, args...)...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func (s *server) handleNAS(w http.ResponseWriter, r *http.Request) {
	render(w, "nas.html", map[string]any{"Page": "nas", "Users": s.nasUsers(), "AdminUnlocked": s.isAdmin(r)})
}

func (s *server) nasUsers() []string {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := s.runWarden(ctx, "nas", "users")
	if err != nil {
		return nil
	}
	var users []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			users = append(users, line)
		}
	}
	return users
}

func (s *server) handleNASAdd(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" || strings.ContainsAny(name, " \t/:") {
		s.nasError(w, "Nombre de usuario inválido.")
		return
	}
	args := []string{"nas", "adduser", name}
	if pass := strings.TrimSpace(r.FormValue("pass")); pass != "" {
		args = append(args, pass)
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	out, err := s.runWarden(ctx, args...)
	if err != nil {
		s.nasError(w, "No se pudo crear el usuario: "+out)
		return
	}
	render(w, "nas_fragment.html", map[string]any{"Users": s.nasUsers(), "Msg": strings.TrimSpace(out)})
}

func (s *server) handleNASDel(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.FormValue("name"))
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	out, err := s.runWarden(ctx, "nas", "deluser", name)
	if err != nil {
		s.nasError(w, "No se pudo eliminar: "+out)
		return
	}
	render(w, "nas_fragment.html", map[string]any{"Users": s.nasUsers(), "Msg": strings.TrimSpace(out)})
}

func (s *server) handleNASReveal(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.FormValue("name"))
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	out, err := s.runWarden(ctx, "nas", "users", "-v")
	if err != nil {
		s.nasError(w, "No pude leer las claves.")
		return
	}
	pass := ""
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, name+":") {
			pass = strings.TrimPrefix(line, name+":")
			break
		}
	}
	render(w, "nas_reveal.html", map[string]any{"Name": name, "Pass": strings.TrimSpace(pass)})
}

func (s *server) nasError(w http.ResponseWriter, msg string) {
	render(w, "nas_fragment.html", map[string]any{"Users": s.nasUsers(), "Err": msg})
}
