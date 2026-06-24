package main

import "os/exec"

// ToolLink: herramientas de sistema instaladas por sus propios módulos
// (NO viven en el catálogo de apps, así que buildAppView nunca las ve) —
// solo se muestran si de verdad están instaladas/corriendo.
type ToolLink struct {
	Name string
	URL  string
}

func gatherTools(containers []Container, host string) []ToolLink {
	var tools []ToolLink

	out, err := exec.Command("systemctl", "is-active", "cockpit.socket").Output()
	if err == nil && string(out) != "" {
		tools = append(tools, ToolLink{Name: "Cockpit", URL: "https://" + host + ".local:9090"})
	}

	up := map[string]bool{}
	for _, c := range containers {
		up[c.Name] = c.Up
	}
	if up["backrest"] {
		tools = append(tools, ToolLink{Name: "Backrest", URL: "http://" + host + ".local:9898"})
	}
	if up["ntfy"] {
		tools = append(tools, ToolLink{Name: "ntfy", URL: "http://" + host + ".local:8080"})
	}
	return tools
}
