package main

import (
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

func cloudflareConfigured() bool {
	_, err := os.Stat("/etc/cloudflared/config.yml")
	return err == nil
}

type RunnerInfo struct {
	Service string
	Active  bool
}

// listRunners: todos los runners de GitHub Actions registrados en este
// server (no hay un token permanente de GitHub que warden guarde — el
// token de registro se usa una sola vez y se descarta; esta lista es la
// forma real de ver "qué hay autorizado", en vez de un secreto guardado).
func listRunners() []RunnerInfo {
	out, err := exec.Command("systemctl", "list-units", "--type=service", "--all",
		"--no-legend", "--plain").Output()
	if err != nil {
		return nil
	}
	var out2 []RunnerInfo
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) == 0 || !strings.HasPrefix(f[0], "actions.runner.") {
			continue
		}
		active := len(f) > 2 && f[2] == "running"
		out2 = append(out2, RunnerInfo{Service: f[0], Active: active})
	}
	return out2
}

var githubRepoRe = regexp.MustCompile(`github\.com[:/]([^/]+)/([^/.]+)(\.git)?/?$`)

// parseGitHubRepo extrae owner/repo de una URL de GitHub (https o git@).
func parseGitHubRepo(url string) (owner, repo string, ok bool) {
	m := githubRepoRe.FindStringSubmatch(strings.TrimSpace(url))
	if m == nil {
		return "", "", false
	}
	return m[1], m[2], true
}

// findRunnerService busca un runner ya registrado para owner/repo. El
// instalador de GitHub Actions nombra el servicio systemd como
// 'actions.runner.<owner>-<repo>.<nombre>.service' — buscamos ese patrón
// exacto en vez de adivinar.
func findRunnerService(owner, repo string) (service string, found bool) {
	out, err := exec.Command("systemctl", "list-units", "--type=service", "--all",
		"--no-legend", "--plain").Output()
	if err != nil {
		return "", false
	}
	return findRunnerInUnits(string(out), owner, repo)
}

// findRunnerInUnits es la lógica pura (sin tocar el sistema), separada para
// poder testearla con texto simulado de 'systemctl list-units'.
func findRunnerInUnits(unitsOutput, owner, repo string) (service string, found bool) {
	needle := "actions.runner." + owner + "-" + repo + "."
	for _, line := range strings.Split(unitsOutput, "\n") {
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		if strings.HasPrefix(f[0], needle) {
			return f[0], true
		}
	}
	return "", false
}

type UsedPort struct {
	Port string
	Tag  string
	Name string
}

// usedPorts: la lista completa de puertos ya ocupados por el catálogo —
// para mostrarla AL LLENAR el formulario (no solo rechazar al guardar).
func (s *server) usedPorts() []UsedPort {
	comps, err := listComponentsMerged(s.catalogDirs())
	if err != nil {
		return nil
	}
	var out []UsedPort
	for _, c := range comps {
		if c.CFPort != "" {
			out = append(out, UsedPort{Port: c.CFPort, Tag: c.Tag, Name: c.Name})
		}
	}
	return out
}

// portInUse: ¿algún OTRO componente del catálogo ya usa este puerto?
// (excludeTag para permitir que una app se edite a sí misma sin chocar).
func (s *server) portInUse(port, excludeTag string) (tag string, used bool) {
	if port == "" {
		return "", false
	}
	comps, err := listComponentsMerged(s.catalogDirs())
	if err != nil {
		return "", false
	}
	for _, c := range comps {
		if c.Tag == excludeTag {
			continue
		}
		if c.CFPort == port {
			return c.Tag, true
		}
	}
	return "", false
}

func (s *server) handleCheckRunner(w http.ResponseWriter, r *http.Request) {
	install := strings.TrimSpace(r.FormValue("install"))
	owner, repo, ok := parseGitHubRepo(install)
	if !ok {
		render(w, "runner_status.html", map[string]any{"Invalid": install != ""})
		return
	}
	service, found := findRunnerService(owner, repo)
	render(w, "runner_status.html", map[string]any{
		"Owner": owner, "Repo": repo, "Found": found, "Service": service, "Install": install,
	})
}
