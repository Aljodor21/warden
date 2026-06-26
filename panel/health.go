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
	AvailKB int64 `json:"avail_kb"`
}

type CoreStat struct {
	Idle  int64
	Total int64
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
	m.AvailKB = avail
	m.UsedKB = total - avail
	// Usar RAM física real (suma de regiones "System RAM" en /proc/iomem)
	// en vez de MemTotal, que excluye lo que el kernel se reserva para sí mismo.
	if phys := readPhysRAMKB(); phys > total {
		m.TotalKB = phys
	} else {
		m.TotalKB = total
	}
	return m
}

// readPhysRAMKB suma las regiones "System RAM" de /proc/iomem para obtener
// el total físico visible al OS (más cercano al hardware real que MemTotal).
func readPhysRAMKB() int64 {
	b, err := os.ReadFile("/proc/iomem")
	if err != nil {
		return 0
	}
	var total int64
	for _, line := range strings.Split(string(b), "\n") {
		if !strings.Contains(line, "System RAM") {
			continue
		}
		parts := strings.SplitN(strings.TrimSpace(line), " : ", 2)
		if len(parts) < 1 {
			continue
		}
		addrs := strings.SplitN(strings.TrimSpace(parts[0]), "-", 2)
		if len(addrs) != 2 {
			continue
		}
		start, e1 := strconv.ParseInt(addrs[0], 16, 64)
		end, e2 := strconv.ParseInt(addrs[1], 16, 64)
		if e1 != nil || e2 != nil {
			continue
		}
		total += end - start + 1
	}
	return total / 1024
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
	Label    string
	Pct      int
	FreePct  int
	Detail   string // valor principal (usado)
	FreeStr  string // solo RAM: "X libres"
	TotalStr string // solo RAM: "Y total"
	Level    string
	Arc      float64 // stroke-dasharray para SVG donut (r=30, circ≈188.5)
	Color    string  // hex del trazo según Level
}

type CoreLoad struct {
	N     int
	Pct   int
	Level string
	Color string
}

type ProcRow struct {
	Pid    int
	Name   string
	CPU    string
	Mem    string
	CPUPct float64
}

type HealthView struct {
	Hostname      string
	OS            string
	Uptime        string
	Cores         int
	CPU           Gauge
	CoreLoads     []CoreLoad
	RAM           Gauge
	Disks         []Gauge
	DownRate      string
	UpRate        string
	NetHist       NetSparkline
	TopProcs      []ProcRow
	InstalledApps []AppCard
	DeployedApps  []AppCard
	Others        []Container
	Tools         []ToolLink
	UpCount       int
	TotalCount    int
	OthersUp      int
	OthersTotal   int
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

func gaugeColor(lv string) string {
	switch lv {
	case "crit":
		return "#f87171"
	case "warn":
		return "#f59e0b"
	default:
		return "#38bdf8"
	}
}

func readCoreStat() []CoreStat {
	b, err := os.ReadFile("/proc/stat")
	if err != nil {
		return nil
	}
	var cores []CoreStat
	for _, line := range strings.Split(string(b), "\n") {
		if len(line) < 5 || !strings.HasPrefix(line, "cpu") || line[3] == ' ' {
			continue // salta la línea agregada "cpu  ..."
		}
		f := strings.Fields(line)
		if len(f) < 5 {
			continue
		}
		var v [10]int64
		for i := 1; i < len(f) && i <= 10; i++ {
			v[i-1], _ = strconv.ParseInt(f[i], 10, 64)
		}
		idle := v[3] + v[4] // idle + iowait
		total := v[0] + v[1] + v[2] + v[3] + v[4] + v[5] + v[6] + v[7]
		cores = append(cores, CoreStat{Idle: idle, Total: total})
	}
	return cores
}

func (s *server) coreRates() []CoreLoad {
	cur := readCoreStat()
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	var loads []CoreLoad
	if len(s.prevCores) == len(cur) && !s.prevCoreAt.IsZero() {
		for i, c := range cur {
			p := s.prevCores[i]
			dTotal := c.Total - p.Total
			dIdle := c.Idle - p.Idle
			pct := 0
			if dTotal > 0 {
				pct = int((dTotal - dIdle) * 100 / dTotal)
				if pct < 0 {
					pct = 0
				}
				if pct > 100 {
					pct = 100
				}
			}
			lv := level(pct)
			loads = append(loads, CoreLoad{N: i, Pct: pct, Level: lv, Color: gaugeColor(lv)})
		}
	}
	s.prevCores = cur
	s.prevCoreAt = now
	return loads
}

func readTopProcs() []ProcRow {
	out, err := exec.Command("ps", "-eo", "pid,comm,%cpu,%mem", "--sort=-%cpu").Output()
	if err != nil {
		return nil
	}
	var rows []ProcRow
	for i, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if i == 0 {
			continue
		}
		if len(rows) >= 10 {
			break
		}
		f := strings.Fields(line)
		if len(f) < 4 {
			continue
		}
		pid, _ := strconv.Atoi(f[0])
		cpuF, _ := strconv.ParseFloat(f[2], 64)
		if cpuF > 100 {
			cpuF = 100
		}
		rows = append(rows, ProcRow{
			Pid: pid, Name: f[1], CPU: f[2], Mem: f[3], CPUPct: cpuF,
		})
	}
	return rows
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

type NetSample struct{ DownBps, UpBps float64 }

type NetSparkline struct {
	DownPoly string // puntos del polígono de relleno (bajada)
	UpPoly   string // puntos del polígono de relleno (subida)
	DownLine string // puntos de la línea (bajada)
	UpLine   string // puntos de la línea (subida)
	MaxLabel string // etiqueta del máximo visible
}

func computeSparkline(samples []NetSample) NetSparkline {
	if len(samples) == 0 {
		return NetSparkline{}
	}
	var maxBps float64
	for _, s := range samples {
		if s.DownBps > maxBps {
			maxBps = s.DownBps
		}
		if s.UpBps > maxBps {
			maxBps = s.UpBps
		}
	}
	if maxBps == 0 {
		maxBps = 1
	}
	const W, H = 400.0, 60.0
	n := len(samples)
	xStep := W / float64(max(n-1, 1))
	yFor := func(bps float64) float64 { return H - (bps/maxBps)*H*0.92 }

	var dl, ul, dp, up strings.Builder
	fmt.Fprintf(&dp, "0.0,%.1f ", H)
	fmt.Fprintf(&up, "0.0,%.1f ", H)
	for i, s := range samples {
		x := float64(i) * xStep
		dy, uy := yFor(s.DownBps), yFor(s.UpBps)
		if i > 0 {
			dl.WriteByte(' ')
			ul.WriteByte(' ')
		}
		fmt.Fprintf(&dl, "%.1f,%.1f", x, dy)
		fmt.Fprintf(&ul, "%.1f,%.1f", x, uy)
		fmt.Fprintf(&dp, "%.1f,%.1f ", x, dy)
		fmt.Fprintf(&up, "%.1f,%.1f ", x, uy)
	}
	lastX := float64(n-1) * xStep
	fmt.Fprintf(&dp, "%.1f,%.1f", lastX, H)
	fmt.Fprintf(&up, "%.1f,%.1f", lastX, H)
	return NetSparkline{
		DownLine: dl.String(), UpLine: ul.String(),
		DownPoly: dp.String(), UpPoly: up.String(),
		MaxLabel: humanRate(maxBps),
	}
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

func (s *server) buildHealthView(h Health, downBps, upBps float64, reqHost ...string) HealthView {
	// Registrar muestra y leer historial
	s.mu.Lock()
	s.netHistory[s.netHistIdx] = NetSample{DownBps: downBps, UpBps: upBps}
	s.netHistIdx++
	if s.netHistIdx >= 40 {
		s.netHistIdx = 0
		s.netHistFull = true
	}
	n, start := s.netHistIdx, 0
	if s.netHistFull {
		n, start = 40, s.netHistIdx
	}
	samples := make([]NetSample, n)
	for i := 0; i < n; i++ {
		samples[i] = s.netHistory[(start+i)%40]
	}
	s.mu.Unlock()

	v := HealthView{
		Hostname: h.Hostname, OS: h.OS, Cores: h.Cores,
		Uptime:   humanUptime(h.UptimeSecs),
		DownRate: humanRate(downBps), UpRate: humanRate(upBps),
		NetHist:  computeSparkline(samples),
	}
	var apps []AppCard
	apps, v.Others = s.buildAppView(h.Containers)
	toolHost := h.Hostname
	if len(reqHost) > 0 && reqHost[0] != "" {
		toolHost = reqHost[0]
	}
	v.Tools = gatherTools(h.Containers, toolHost)
	for _, a := range apps {
		v.TotalCount++
		if a.Up {
			v.UpCount++
		}
		if a.Deployed {
			v.DeployedApps = append(v.DeployedApps, a)
		} else {
			v.InstalledApps = append(v.InstalledApps, a)
		}
	}
	for _, o := range v.Others {
		v.OthersTotal++
		if o.Up {
			v.OthersUp++
		}
	}

	// CPU: carga de 1 min relativa a la cantidad de núcleos.
	cpuPct := 0
	if h.Cores > 0 {
		cpuPct = int(h.Load[0] / float64(h.Cores) * 100)
	}
	if cpuPct > 100 {
		cpuPct = 100
	}
	cpuLv := level(cpuPct)
	v.CPU = Gauge{Label: "CPU", Pct: cpuPct, Level: cpuLv,
		Detail: fmt.Sprintf("%.2f carga · %d núcleos", h.Load[0], h.Cores),
		Arc: float64(cpuPct) / 100.0 * 188.5, Color: gaugeColor(cpuLv)}

	// RAM (meminfo viene en KB).
	ramPct := 0
	if h.Mem.TotalKB > 0 {
		ramPct = int(h.Mem.UsedKB * 100 / h.Mem.TotalKB)
	}
	ramLv := level(ramPct)
	freePct := 0
	if h.Mem.TotalKB > 0 {
		freePct = int(h.Mem.AvailKB * 100 / h.Mem.TotalKB)
	}
	v.RAM = Gauge{
		Label: "RAM", Pct: ramPct, FreePct: freePct, Level: ramLv,
		Detail:   humanBytes(h.Mem.UsedKB * 1024),
		FreeStr:  humanBytes(h.Mem.AvailKB * 1024),
		TotalStr: humanBytes(h.Mem.TotalKB * 1024),
		Arc: float64(ramPct) / 100.0 * 188.5, Color: gaugeColor(ramLv),
	}

	for _, d := range h.Disks {
		lv := level(d.Pct)
		v.Disks = append(v.Disks, Gauge{
			Label: d.Mount, Pct: d.Pct, Level: lv,
			Detail: fmt.Sprintf("%s / %s", humanBytes(d.Used), humanBytes(d.Total)),
			Arc: float64(d.Pct) / 100.0 * 188.5, Color: gaugeColor(lv),
		})
	}

	v.CoreLoads = s.coreRates()
	v.TopProcs = readTopProcs()
	return v
}
