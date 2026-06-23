package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Health es la foto de salud del sistema que consume el dashboard.
// Todo se obtiene leyendo /proc y comandos básicos (df, docker) — sin deps.
type Health struct {
	Hostname   string      `json:"hostname"`
	OS         string      `json:"os"`
	UptimeSecs int64       `json:"uptime_secs"`
	Load       [3]float64  `json:"load"`
	Cores      int         `json:"cores"`
	Mem        MemInfo     `json:"mem"`
	Disks      []DiskInfo  `json:"disks"`
	Nets       []NetInfo   `json:"nets"`
	Containers []Container `json:"containers"`
	TimeMS     int64       `json:"time_ms"` // para calcular tasas de red en el cliente
}

type MemInfo struct {
	TotalKB int64 `json:"total_kb"`
	UsedKB  int64 `json:"used_kb"`
}

type DiskInfo struct {
	Mount string `json:"mount"`
	Total int64  `json:"total"`
	Used  int64  `json:"used"`
	Pct   int    `json:"pct"`
}

type NetInfo struct {
	Iface string `json:"iface"`
	Rx    int64  `json:"rx"`
	Tx    int64  `json:"tx"`
}

type Container struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Up     bool   `json:"up"`
}

func gatherHealth() Health {
	h := Health{
		Cores:  runtime.NumCPU(),
		TimeMS: time.Now().UnixMilli(),
	}
	h.Hostname, _ = os.Hostname()
	h.OS = osPretty()
	h.UptimeSecs = readUptime()
	h.Load = readLoad()
	h.Mem = readMem()
	h.Disks = readDisks()
	h.Nets = readNets()
	h.Containers = readContainers()
	return h
}

func osPretty() string {
	b, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return "Linux"
	}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "PRETTY_NAME=") {
			return strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), `"`)
		}
	}
	return "Linux"
}

func readUptime() int64 {
	b, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(b))
	if len(fields) == 0 {
		return 0
	}
	f, _ := strconv.ParseFloat(fields[0], 64)
	return int64(f)
}

func readLoad() [3]float64 {
	var l [3]float64
	b, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return l
	}
	f := strings.Fields(string(b))
	for i := 0; i < 3 && i < len(f); i++ {
		l[i], _ = strconv.ParseFloat(f[i], 64)
	}
	return l
}

func readMem() MemInfo {
	var m MemInfo
	b, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return m
	}
	var total, avail int64
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		v, _ := strconv.ParseInt(f[1], 10, 64)
		switch f[0] {
		case "MemTotal:":
			total = v
		case "MemAvailable:":
			avail = v
		}
	}
	m.TotalKB = total
	m.UsedKB = total - avail
	return m
}

func readDisks() []DiskInfo {
	out, err := exec.Command("df", "-P", "-B1").Output()
	if err != nil {
		return nil
	}
	var disks []DiskInfo
	for i, line := range strings.Split(string(out), "\n") {
		if i == 0 {
			continue // cabecera
		}
		f := strings.Fields(line)
		if len(f) < 6 || !strings.HasPrefix(f[0], "/dev/") {
			continue
		}
		total, _ := strconv.ParseInt(f[1], 10, 64)
		used, _ := strconv.ParseInt(f[2], 10, 64)
		pct, _ := strconv.Atoi(strings.TrimSuffix(f[4], "%"))
		disks = append(disks, DiskInfo{Mount: f[5], Total: total, Used: used, Pct: pct})
	}
	return disks
}

func readNets() []NetInfo {
	b, err := os.ReadFile("/proc/net/dev")
	if err != nil {
		return nil
	}
	var nets []NetInfo
	for _, line := range strings.Split(string(b), "\n") {
		if !strings.Contains(line, ":") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		iface := strings.TrimSpace(parts[0])
		if iface == "lo" || iface == "" {
			continue
		}
		f := strings.Fields(parts[1])
		if len(f) < 9 {
			continue
		}
		rx, _ := strconv.ParseInt(f[0], 10, 64)
		tx, _ := strconv.ParseInt(f[8], 10, 64)
		nets = append(nets, NetInfo{Iface: iface, Rx: rx, Tx: tx})
	}
	return nets
}

func readContainers() []Container {
	out, err := exec.Command("docker", "ps", "-a",
		"--format", "{{.Names}}\t{{.State}}\t{{.Status}}").Output()
	if err != nil {
		return nil
	}
	var cs []Container
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		f := strings.SplitN(line, "\t", 3)
		if len(f) < 3 {
			continue
		}
		cs = append(cs, Container{Name: f[0], Status: f[2], Up: f[1] == "running"})
	}
	return cs
}

func (s *server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(gatherHealth())
}

// --- Vista para el dashboard (todo precalculado, el template solo imprime) ---

type Gauge struct {
	Label  string
	Pct    int
	Detail string
	Level  string // ok | warn | crit (clase CSS de color)
}

type HealthView struct {
	Hostname   string
	OS         string
	Uptime     string
	Cores      int
	CPU        Gauge
	RAM        Gauge
	Disks      []Gauge
	DownRate   string
	UpRate     string
	Containers []Container
	UpCount    int
	TotalCount int
}

func level(pct int) string {
	switch {
	case pct >= 90:
		return "crit"
	case pct >= 70:
		return "warn"
	default:
		return "ok"
	}
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

func humanRate(bps float64) string {
	switch {
	case bps < 1024:
		return fmt.Sprintf("%.0f B/s", bps)
	case bps < 1024*1024:
		return fmt.Sprintf("%.0f KB/s", bps/1024)
	default:
		return fmt.Sprintf("%.1f MB/s", bps/1024/1024)
	}
}

func humanUptime(s int64) string {
	d, h, m := s/86400, (s%86400)/3600, (s%3600)/60
	switch {
	case d > 0:
		return fmt.Sprintf("%dd %dh", d, h)
	case h > 0:
		return fmt.Sprintf("%dh %dm", h, m)
	default:
		return fmt.Sprintf("%dm", m)
	}
}

func buildHealthView(h Health, downBps, upBps float64) HealthView {
	v := HealthView{
		Hostname: h.Hostname, OS: h.OS, Cores: h.Cores,
		Uptime:   humanUptime(h.UptimeSecs),
		DownRate: humanRate(downBps), UpRate: humanRate(upBps),
		Containers: h.Containers,
	}

	// CPU: carga de 1 min relativa a la cantidad de núcleos.
	cpuPct := 0
	if h.Cores > 0 {
		cpuPct = int(h.Load[0] / float64(h.Cores) * 100)
	}
	if cpuPct > 100 {
		cpuPct = 100
	}
	v.CPU = Gauge{Label: "CPU", Pct: cpuPct, Level: level(cpuPct),
		Detail: fmt.Sprintf("%.2f carga · %d núcleos", h.Load[0], h.Cores)}

	// RAM (meminfo viene en KB).
	ramPct := 0
	if h.Mem.TotalKB > 0 {
		ramPct = int(h.Mem.UsedKB * 100 / h.Mem.TotalKB)
	}
	v.RAM = Gauge{Label: "RAM", Pct: ramPct, Level: level(ramPct),
		Detail: fmt.Sprintf("%s / %s", humanBytes(h.Mem.UsedKB*1024), humanBytes(h.Mem.TotalKB*1024))}

	for _, d := range h.Disks {
		v.Disks = append(v.Disks, Gauge{
			Label: d.Mount, Pct: d.Pct, Level: level(d.Pct),
			Detail: fmt.Sprintf("%s / %s", humanBytes(d.Used), humanBytes(d.Total)),
		})
	}

	v.TotalCount = len(h.Containers)
	for _, c := range h.Containers {
		if c.Up {
			v.UpCount++
		}
	}
	return v
}
