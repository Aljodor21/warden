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
	TailscaleInstalled       bool
	TailscaleConnected       bool
	TailscaleIP              string
	TailscaleExitNode        bool
	TailscaleSubnetActive    bool
	TailscaleSubnet          string
	TailscaleSuggestedSubnet string

	AgeKeyExists       bool
	SecretsExist       bool // hay al menos un *.tar.age guardado
	SecretsCount       int
	CloudflareSet      bool   // /etc/cloudflared/config.yml existe (hay túnel)
	CloudflareID       string // ID del túnel configurado, si hay uno
	CloudflareTokenSet bool   // hay un API Token guardado (para borrar registros DNS)
	Runners            []RunnerInfo

	Timezone string // zona horaria activa del sistema (IANA)
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
	if v.TailscaleConnected {
		if _, err := os.Stat("/etc/warden/tailscale-exitnode"); err == nil {
			v.TailscaleExitNode = true
		}
		if b, err := os.ReadFile("/etc/warden/tailscale-subnet"); err == nil {
			if s := strings.TrimSpace(string(b)); s != "" {
				v.TailscaleSubnetActive = true
				v.TailscaleSubnet = s
			}
		}
		v.TailscaleSuggestedSubnet = detectLocalSubnet()
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

	v.Timezone = systemTimezone()

	return v
}

func (s *server) handleSystemMem(w http.ResponseWriter, r *http.Request) {
	panelBytes := panelMemBytes()
	containers := containerMemStats()
	total := panelBytes
	for _, c := range containers {
		total += c.bytes
	}
	render(w, "system_mem.html", map[string]any{
		"PanelMem":    humanBytes(panelBytes),
		"Containers":  containers,
		"TotalMemAll": humanBytes(total),
	})
}

func systemTimezone() string {
	b, err := os.ReadFile("/etc/timezone")
	if err == nil {
		if tz := strings.TrimSpace(string(b)); tz != "" {
			return tz
		}
	}
	out, err := exec.Command("timedatectl", "show", "--value", "-p", "Timezone").Output()
	if err == nil {
		if tz := strings.TrimSpace(string(out)); tz != "" {
			return tz
		}
	}
	return "UTC"
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

func detectLocalSubnet() string {
	out, err := exec.Command("ip", "route", "show", "scope", "link").Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		skip := false
		for _, bad := range []string{"lo ", "docker", "br-", "veth", "tailscale", "169."} {
			if strings.Contains(line, bad) {
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) > 0 && strings.Contains(fields[0], "/") {
			return fields[0]
		}
	}
	return ""
}

func (s *server) siteSecretsDir() string {
	return s.root + "/site/secrets" // modules/secrets.sh: SECRETS_DIR = $WARDEN_ROOT/site/secrets
}

func (s *server) handleSystem(w http.ResponseWriter, r *http.Request) {
	render(w, "system.html", map[string]any{
		"Page": "system", "AdminUnlocked": s.isAdmin(r), "Sys": s.gatherSystemView(),
	})
}

func (s *server) handleVPNExitNode(w http.ResponseWriter, r *http.Request) {
	action := r.FormValue("action") // "on" o "off"
	if action != "on" && action != "off" {
		render(w, "system_fragment.html", map[string]any{"Sys": s.gatherSystemView(), "Err": "Acción inválida."})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	out, err := s.runWarden(ctx, "vpn", "exit-node", action)
	s.renderSystemAction(w, out, err)
}

func (s *server) handleVPNSubnet(w http.ResponseWriter, r *http.Request) {
	action := r.FormValue("action") // "on" o "off"
	if action != "on" && action != "off" {
		render(w, "system_fragment.html", map[string]any{"Sys": s.gatherSystemView(), "Err": "Acción inválida."})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if action == "on" {
		subnet := strings.TrimSpace(r.FormValue("subnet"))
		if subnet == "" {
			render(w, "system_fragment.html", map[string]any{"Sys": s.gatherSystemView(), "Err": "La subred no puede estar vacía."})
			return
		}
		out, err := s.runWarden(ctx, "vpn", "subnet", "on", subnet)
		s.renderSystemAction(w, out, err)
		return
	}
	out, err := s.runWarden(ctx, "vpn", "subnet", "off")
	s.renderSystemAction(w, out, err)
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

func (s *server) handleReset(w http.ResponseWriter, r *http.Request) {
	if r.FormValue("confirm") != "BORRAR" {
		render(w, "system_fragment.html", map[string]any{"Sys": s.gatherSystemView(), "Err": "Confirmación incorrecta."})
		return
	}
	if !s.resetProc.start() {
		render(w, "reset_log.html", map[string]any{"Running": true})
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	go func() {
		defer cancel()
		runInBackground(ctx, &s.resetProc, "sudo", s.wardenBin, "reset", "--yes")
	}()
	render(w, "reset_log.html", map[string]any{"Running": true})
}

func (s *server) handleResetLog(w http.ResponseWriter, r *http.Request) {
	logText, running, done := s.resetProc.snapshot()
	render(w, "reset_log.html", map[string]any{"Log": logText, "Running": running, "Done": done})
}

func (s *server) handleSetTimezone(w http.ResponseWriter, r *http.Request) {
	tz := strings.TrimSpace(r.FormValue("tz"))
	loc, err := time.LoadLocation(tz)
	if err != nil || tz == "" {
		render(w, "system_fragment.html", map[string]any{"Sys": s.gatherSystemView(), "Err": "Zona horaria inválida: " + tz})
		return
	}
	if out, err := exec.Command("sudo", "timedatectl", "set-timezone", tz).CombinedOutput(); err != nil {
		render(w, "system_fragment.html", map[string]any{"Sys": s.gatherSystemView(), "Err": "Error al cambiar zona: " + strings.TrimSpace(string(out))})
		return
	}
	time.Local = loc
	render(w, "system_fragment.html", map[string]any{"Sys": s.gatherSystemView(), "Output": "Zona horaria cambiada a " + tz})
}
