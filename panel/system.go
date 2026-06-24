package main

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

const ageKeyPath = "/etc/warden/age.key"

type SystemView struct {
	TailscaleInstalled bool
	TailscaleConnected bool
	TailscaleIP        string

	AgeKeyExists       bool
	SecretsExist       bool // hay al menos un *.tar.age guardado
	SecretsCount       int
	CloudflareSet      bool   // /etc/cloudflared/config.yml existe (hay túnel)
	CloudflareID       string // ID del túnel configurado, si hay uno
	CloudflareTokenSet bool   // hay un API Token guardado (para borrar registros DNS)
	Runners            []RunnerInfo

	PanelMem    string         // RAM del propio proceso warden-panel
	Containers  []ContainerMem // uno por contenedor corriendo, RAM usada
	TotalMemAll string         // panel + todos los contenedores, para responder "cuánto pesa todo esto"
}

// ContainerMem: una fila de 'docker stats' — cuánta RAM usa un contenedor
// puntual, para responder "¿cuánto le agregó X al total?" en vez de solo
// ver el total general del sistema (que ya se ve en el Dashboard).
type ContainerMem struct {
	Name  string
	Mem   string
	bytes int64
}

func (s *server) gatherSystemView() SystemView {
	v := SystemView{}

	if _, err := exec.LookPath("tailscale"); err == nil {
		v.TailscaleInstalled = true
		out, err := exec.Command("tailscale", "ip", "-4").Output()
		if err == nil {
			ip := strings.TrimSpace(string(out))
			if ip != "" {
				v.TailscaleConnected = true
				v.TailscaleIP = ip
			}
		}
	}

	if _, err := os.Stat(ageKeyPath); err == nil {
		v.AgeKeyExists = true
	}
	v.CloudflareSet = cloudflareConfigured()
	v.CloudflareID = cloudflareTunnelID()
	v.CloudflareTokenSet = cloudflareTokenExists()
	v.Runners = listRunners()
	if entries, err := os.ReadDir(s.siteSecretsDir()); err == nil {
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".tar.age") {
				v.SecretsCount++
			}
		}
		v.SecretsExist = v.SecretsCount > 0
	}

	panelBytes := panelMemBytes()
	v.PanelMem = humanBytes(panelBytes)
	v.Containers = containerMemStats()
	total := panelBytes
	for _, c := range v.Containers {
		total += c.bytes
	}
	v.TotalMemAll = humanBytes(total)

	return v
}

// panelMemBytes: RAM (RSS) del propio proceso warden-panel — vive nativo en
// el host, no en un contenedor, así que no aparece en 'docker stats'.
func panelMemBytes() int64 {
	b, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "VmRSS:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				kb, _ := strconv.ParseInt(fields[1], 10, 64)
				return kb * 1024
			}
		}
	}
	return 0
}

// containerMemStats: cuánta RAM usa cada contenedor corriendo — responde
// "¿cuánto le suma X al total?" en vez de solo el porcentaje general del
// sistema que ya se ve en el Dashboard.
func containerMemStats() []ContainerMem {
	out, err := exec.Command("docker", "stats", "--no-stream", "--format", "{{.Name}}\t{{.MemUsage}}").Output()
	if err != nil {
		return nil
	}
	var rows []ContainerMem
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		// MemUsage viene como "12.34MiB / 1.944GiB" — solo nos importa el usado.
		used := strings.TrimSpace(strings.SplitN(parts[1], "/", 2)[0])
		b := parseDockerMem(used)
		rows = append(rows, ContainerMem{Name: parts[0], Mem: used, bytes: b})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].bytes > rows[j].bytes })
	return rows
}

// parseDockerMem convierte "12.34MiB" / "512KiB" / "1.2GiB" a bytes.
// El orden de los sufijos importa: "B" también coincide con el final de
// "MiB"/"GiB" — hay que probar los más largos primero (un map, al iterar
// en orden no garantizado, puede chequear "B" antes que "MiB" y truncar
// mal el número).
func parseDockerMem(s string) int64 {
	type unit struct {
		suffix string
		mult   float64
	}
	units := []unit{
		{"TiB", 1 << 40}, {"GiB", 1 << 30}, {"MiB", 1 << 20}, {"KiB", 1 << 10}, {"B", 1},
	}
	for _, u := range units {
		if strings.HasSuffix(s, u.suffix) {
			numStr := strings.TrimSuffix(s, u.suffix)
			n, err := strconv.ParseFloat(numStr, 64)
			if err != nil {
				return 0
			}
			return int64(n * u.mult)
		}
	}
	return 0
}

func (s *server) siteSecretsDir() string {
	return s.root + "/site/secrets" // modules/secrets.sh: SECRETS_DIR = $WARDEN_ROOT/site/secrets
}

func (s *server) handleSystem(w http.ResponseWriter, r *http.Request) {
	render(w, "system.html", map[string]any{
		"Page": "system", "AdminUnlocked": s.isAdmin(r), "Sys": s.gatherSystemView(),
	})
}

func (s *server) handleVPNInstall(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second) // tailscale up puede tardar
	defer cancel()
	out, err := s.runWarden(ctx, "vpn")
	s.renderSystemAction(w, out, err)
}

func (s *server) handleSecretsInit(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	out, err := s.runWarden(ctx, "secrets", "init")
	s.renderSystemAction(w, out, err)
}

func (s *server) handleSecretsSave(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	out, err := s.runWarden(ctx, "secrets", "save")
	s.renderSystemAction(w, out, err)
}

func (s *server) renderSystemAction(w http.ResponseWriter, out string, err error) {
	data := map[string]any{"Sys": s.gatherSystemView(), "Output": strings.TrimSpace(out)}
	if err != nil {
		data["Err"] = "Falló: " + out
	}
	render(w, "system_fragment.html", data)
}
