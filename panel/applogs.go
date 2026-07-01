package main

import (
	"net/http"
	"os/exec"
	"strings"
)

func (s *server) handleAppLogs(w http.ResponseWriter, r *http.Request) {
	tag := r.PathValue("tag")
	container := s.containerForTag(tag)
	logText := fetchDockerLogs(container, "100")
	render(w, "app_logs.html", map[string]any{
		"Tag":       tag,
		"Container": container,
		"Log":       logText,
	})
}

func (s *server) handleAppLogsPoll(w http.ResponseWriter, r *http.Request) {
	tag := r.PathValue("tag")
	container := s.containerForTag(tag)
	logText := fetchDockerLogs(container, "100")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte(logText))
}

func (s *server) handleAppLogsClose(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte(""))
}

func fetchDockerLogs(container, tail string) string {
	out, _ := exec.Command("docker", "logs", "--tail", tail, "--timestamps", container).CombinedOutput()
	return strings.TrimSpace(string(out))
}

// containerForTag resuelve el nombre del contenedor Docker dado un tag.
// Usa COMP_CONTAINER del .component si existe, sino el tag como fallback.
func (s *server) containerForTag(tag string) string {
	for _, dir := range s.catalogDirs() {
		if c, err := parseComponentFile(dir + "/" + tag + ".component"); err == nil {
			if c.Container != "" {
				return c.Container
			}
		}
	}
	return tag
}
