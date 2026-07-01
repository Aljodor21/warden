package main

import (
	"net/http"
	"os/exec"
	"strings"
)

func (s *server) handleAppLogs(w http.ResponseWriter, r *http.Request) {
	tag := r.PathValue("tag")
	container := s.containerForTag(tag)
	lines := r.URL.Query().Get("lines")
	if lines == "" {
		lines = "100"
	}

	out, _ := exec.Command("docker", "logs", "--tail", lines, "--timestamps", container).CombinedOutput()
	logText := strings.TrimSpace(string(out))

	render(w, "app_logs.html", map[string]any{
		"Tag":       tag,
		"Container": container,
		"Log":       logText,
	})
}

func (s *server) handleAppLogsClose(w http.ResponseWriter, r *http.Request) {
	tag := r.PathValue("tag")
	w.Write([]byte(`<div id="log-` + tag + `"></div>`))
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
