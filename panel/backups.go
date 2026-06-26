package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

const backupMarker = ".warden-backup.id"

type DiskRow struct {
	Name   string
	Size   string
	Model  string
	Role   string // SYSTEM | BACKUP | OTHER | EMPTY
	Detail string
}

type Snapshot struct {
	ID      string   `json:"short_id"`
	Time    string   `json:"time"`
	Tags    []string `json:"tags"`
	Paths   []string `json:"paths"`
	Summary struct {
		TotalBytesProcessed int64 `json:"total_bytes_processed"`
	} `json:"summary"` // puede venir vacío según la versión de restic — se trata como opcional
	WhenAgo  string // calculado: "hace 2 horas"
	WhenFull string // calculado: fecha+hora legible
}

type BackupsView struct {
	Disks          []DiskRow
	BackupMount    string // "" si no hay ninguno montado
	Snapshots      []Snapshot
	FilesCount     int
	DBCount        int
	LastBackup     string // "hace 2 horas"
	LastBackupFull string // "23 Jun 2026, 13:45"
	AgeLevel       string // ok | warn | crit — semáforo de antigüedad
	RepoSize       string // tamaño total del repositorio restic
	SnapshotsErr   string

	TimerInstalled bool
	TimerActive    bool
	TimerNext      string // próxima ejecución, legible
	TimerLast      string // última ejecución que el timer disparó, legible
	HasBackupDisk  bool   // ya hay un disco con el marcador de warden — activar el timer no preguntará nada
	DiskMounted    bool   // hay un disco de backup MONTADO ahora mismo
	MissingPass    bool   // el passfile de restic no existe — mostrar formulario para ingresarlo

	BackupRuns []BackupRun // cada corrida real (files+db juntos), para elegir cuál restaurar
}

// BackupRun: una corrida real de 'warden backup' — agrupa el snapshot de
// archivos y el de BD que se generaron juntos (siempre van pegados en el
// tiempo, uno justo después del otro). Restaurar siempre "lo más
// reciente" es una trampa real: si el backup automático corrió justo
// después de reinstalar una app (antes de que tuviera datos), ESE
// snapshot vacío se vuelve "el más nuevo" y tapa a uno viejo con
// contenido real — por eso se puede elegir cualquier corrida, no solo
// la última.
type BackupRun struct {
	FilesID, DBID     string
	WhenFull, WhenAgo string
	FilesSize, DBSize string // "" si la versión de restic no reporta tamaño
	Paths             []string
}

// systemDiskName replica bin/warden:system_disk() — el disco que contiene '/'.
func systemDiskName() string {
	out, err := exec.Command("findmnt", "-no", "SOURCE", "/").Output()
	if err != nil {
		return ""
	}
	src := strings.TrimSpace(string(out))
	chain, err := exec.Command("lsblk", "-snpo", "NAME", src).Output()
	if err != nil {
		return ""
	}
	lines := strings.Fields(strings.TrimSpace(string(chain)))
	if len(lines) == 0 {
		return ""
	}
	last := lines[len(lines)-1]
	parts := strings.Split(last, "/")
	return parts[len(parts)-1]
}

func listDisks() []DiskRow {
	out, err := exec.Command("lsblk", "-dpno", "NAME,SIZE,TYPE,MODEL").Output()
	if err != nil {
		return nil
	}
	sysd := systemDiskName()
	var rows []DiskRow
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		f := strings.Fields(line)
		if len(f) < 3 || f[2] != "disk" {
			continue
		}
		name, size := f[0], f[1]
		model := ""
		if len(f) > 3 {
			model = strings.Join(f[3:], " ")
		}
		role, detail := classifyDisk(name, sysd)
		rows = append(rows, DiskRow{Name: name, Size: size, Model: model, Role: role, Detail: detail})
	}
	return rows
}

func classifyDisk(disk, sysd string) (role, detail string) {
	short := disk[strings.LastIndex(disk, "/")+1:]
	if short == sysd {
		return "SYSTEM", "disco del sistema (/)"
	}
	if mp, id := diskMarkerMount(disk); mp != "" {
		return "BACKUP", "backup warden · montado en " + mp + " · id=" + id
	}
	if diskHasFS(disk) {
		return "OTHER", "tiene datos (no es backup warden)"
	}
	return "EMPTY", "vacío / sin formato"
}

func diskMarkerMount(disk string) (mount, id string) {
	out, err := exec.Command("lsblk", "-rpno", "NAME,MOUNTPOINT", disk).Output()
	if err != nil {
		return "", ""
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		mp := f[1]
		markerPath := mp + "/" + backupMarker
		if b, err := os.ReadFile(markerPath); err == nil {
			return mp, strings.TrimSpace(string(b))
		}
	}
	return "", ""
}

func diskHasFS(disk string) bool {
	out, err := exec.Command("lsblk", "-rpno", "FSTYPE", disk).Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) != ""
}

func backupMount() string {
	if m := os.Getenv("WARDEN_BACKUP_MOUNT"); m != "" {
		return m
	}
	return "/mnt/warden-backup"
}

func resticPassFile() string {
	if p := os.Getenv("RESTIC_PASS_FILE"); p != "" {
		return p
	}
	return "/root/.warden-restic-password"
}

// timerInfo lee el estado real de warden-backup.timer — no asume nada: si
// el timer no existe (porque este server nunca corrió 'warden register'),
// se reporta tal cual, sin fingir un estado.
func timerInfo() (installed, active bool, next, last string) {
	out, err := exec.Command("systemctl", "show", "warden-backup.timer",
		"--property=LoadState,ActiveState,NextElapseUSecRealtime,LastTriggerUSec").Output()
	if err != nil {
		return false, false, "", ""
	}
	props := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		kv := strings.SplitN(line, "=", 2)
		if len(kv) == 2 {
			props[kv[0]] = kv[1]
		}
	}
	installed = props["LoadState"] == "loaded"
	active = props["ActiveState"] == "active"
	next = props["NextElapseUSecRealtime"]
	last = props["LastTriggerUSec"]
	if last == "n/a" {
		last = ""
	}
	return
}

func repoSizeHuman(repo string) string {
	out, err := exec.Command("du", "-sh", repo).Output()
	if err != nil {
		return ""
	}
	f := strings.Fields(string(out))
	if len(f) == 0 {
		return ""
	}
	return f[0]
}

func ageLevel(t time.Time) string {
	age := time.Since(t)
	switch {
	case age > 72*time.Hour:
		return "crit"
	case age > 24*time.Hour:
		return "warn"
	default:
		return "ok"
	}
}

func humanAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "hace instantes"
	case d < time.Hour:
		return fmt.Sprintf("hace %d min", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("hace %d h", int(d.Hours()))
	default:
		return fmt.Sprintf("hace %d días", int(d.Hours()/24))
	}
}

func (s *server) gatherBackupsView() BackupsView {
	v := BackupsView{Disks: listDisks(), BackupMount: backupMount()}
	v.TimerInstalled, v.TimerActive, v.TimerNext, v.TimerLast = timerInfo()
	for _, d := range v.Disks {
		if d.Role == "BACKUP" {
			v.HasBackupDisk = true
			break
		}
	}
	// ¿hay un disco de backup físicamente montado ahora mismo?
	if out, err := exec.Command("findmnt", "-no", "SOURCE", v.BackupMount).Output(); err == nil {
		if src := strings.TrimSpace(string(out)); src != "" {
			if _, serr := os.Stat(v.BackupMount + "/" + backupMarker); serr == nil {
				v.DiskMounted = true
			}
		}
	}

	repo := v.BackupMount + "/restic-repo"
	passfile := resticPassFile()
	if _, err := os.Stat(repo); err != nil {
		return v
	}
	v.RepoSize = repoSizeHuman(repo)
	if _, err := os.Stat(passfile); err != nil {
		v.MissingPass = true
		return v
	}

	if _, err := exec.LookPath("docker"); err != nil {
		v.SnapshotsErr = "Falta Docker instalado (restic corre en un contenedor) — corré 'sudo ./bootstrap.sh' primero."
		return v
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "docker", "run", "--rm",
		"-e", "RESTIC_PASSWORD_FILE=/pass",
		"-v", passfile+":/pass:ro",
		"-v", repo+":/repo",
		"restic/restic", "-r", "/repo", "snapshots", "--json").Output()
	if err != nil {
		detail := err.Error()
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			detail = strings.TrimSpace(string(ee.Stderr))
		}
		v.SnapshotsErr = "No pude leer los snapshots: " + detail
		return v
	}
	var snaps []Snapshot
	if err := json.Unmarshal(out, &snaps); err != nil {
		return v
	}
	for i, sn := range snaps {
		t, perr := time.Parse(time.RFC3339Nano, sn.Time)
		if perr == nil {
			snaps[i].WhenAgo = humanAgo(t)
			snaps[i].WhenFull = t.Local().Format("02 Jan 2006, 15:04")
		}
		for _, tag := range sn.Tags {
			if tag == "files" {
				v.FilesCount++
			} else if tag == "db" {
				v.DBCount++
			}
		}
	}
	v.Snapshots = snaps
	if len(snaps) > 0 {
		last := snaps[len(snaps)-1]
		v.LastBackup = last.WhenAgo
		v.LastBackupFull = last.WhenFull
		if t, perr := time.Parse(time.RFC3339Nano, last.Time); perr == nil {
			v.AgeLevel = ageLevel(t)
		}
	}
	v.BackupRuns = groupBackupRuns(snaps)
	return v
}

// groupBackupRuns empareja cada snapshot de "files" con el de "db" más
// cercano en el tiempo (siempre se generan juntos, segundos aparte, en
// la misma corrida de 'warden backup') — para que el panel pueda ofrecer
// "restaurar desde esta corrida" sin asumir que la más reciente es la
// que tiene contenido real.
func groupBackupRuns(snaps []Snapshot) []BackupRun {
	var filesSnaps, dbSnaps []Snapshot
	for _, sn := range snaps {
		for _, tag := range sn.Tags {
			if tag == "files" {
				filesSnaps = append(filesSnaps, sn)
			} else if tag == "db" {
				dbSnaps = append(dbSnaps, sn)
			}
		}
	}
	var runs []BackupRun
	for _, fs := range filesSnaps {
		ft, err := time.Parse(time.RFC3339Nano, fs.Time)
		if err != nil {
			continue
		}
		run := BackupRun{FilesID: fs.ID, WhenFull: fs.WhenFull, WhenAgo: fs.WhenAgo, Paths: fs.Paths}
		if fs.Summary.TotalBytesProcessed > 0 {
			run.FilesSize = humanFileSize(fs.Summary.TotalBytesProcessed)
		}
		// El "compañero" de db: el más cercano en el tiempo, dentro de 1 minuto.
		var best Snapshot
		bestDiff := time.Hour
		for _, ds := range dbSnaps {
			dt, err := time.Parse(time.RFC3339Nano, ds.Time)
			if err != nil {
				continue
			}
			diff := dt.Sub(ft)
			if diff < 0 {
				diff = -diff
			}
			if diff < bestDiff {
				bestDiff = diff
				best = ds
			}
		}
		if bestDiff <= time.Minute {
			run.DBID = best.ID
			if best.Summary.TotalBytesProcessed > 0 {
				run.DBSize = humanFileSize(best.Summary.TotalBytesProcessed)
			}
		}
		runs = append(runs, run)
	}
	// Más reciente primero — el usuario decide cuál usar, no se le impone.
	for i, j := 0, len(runs)-1; i < j; i, j = i+1, j-1 {
		runs[i], runs[j] = runs[j], runs[i]
	}
	return runs
}

func (s *server) handleBackupsPage(w http.ResponseWriter, r *http.Request) {
	render(w, "backups.html", map[string]any{
		"Page": "backups", "AdminUnlocked": s.isAdmin(r), "B": s.gatherBackupsView(),
	})
}

// Un backup real puede tardar varios minutos — mucho más que el
// WriteTimeout del servidor HTTP (30s). Corre en segundo plano; el botón
// solo lo dispara y avisa, sin bloquear la página.
func (s *server) handleBackupNow(w http.ResponseWriter, r *http.Request) {
	if !s.backupProc.start() {
		render(w, "backups_fragment.html", map[string]any{"B": s.gatherBackupsView(), "Running": true})
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	go func() {
		defer cancel()
		runInBackground(ctx, &s.backupProc, "sudo", s.wardenBin, "backup")
	}()
	render(w, "backups_fragment.html", map[string]any{"B": s.gatherBackupsView(), "Running": true})
}

func (s *server) handleBackupNowLog(w http.ResponseWriter, r *http.Request) {
	logText, running, done := s.backupProc.snapshot()
	render(w, "backup_log.html", map[string]any{"Log": logText, "Running": running, "Done": done})
}

// Activar el timer es rápido y seguro de ejecutar sin TTY SOLO si ya hay un
// disco de backup detectado (si no, 'warden register' pregunta cuál usar —
// el botón ni aparece en ese caso, ver gatherBackupsView/HasBackupDisk).
func (s *server) handleRegisterTimer(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	out, err := s.runWarden(ctx, "register")
	data := map[string]any{"B": s.gatherBackupsView()}
	if err != nil {
		data["Err"] = "Falló: " + out
	}
	render(w, "backups_fragment.html", data)
}

// --- Gestión de disco desde el panel ---

func diskFirstPart(dev string) string {
	for _, p := range []string{dev + "p1", dev + "1"} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func (s *server) handleDiskMount(w http.ResponseWriter, r *http.Request) {
	dev := strings.TrimSpace(r.FormValue("dev"))
	if dev == "" || !strings.HasPrefix(dev, "/dev/") {
		s.renderBackupsErr(w, "Disco inválido.")
		return
	}
	mount := backupMount()
	if out, err := exec.Command("findmnt", "-no", "SOURCE", mount).Output(); err == nil {
		if strings.TrimSpace(string(out)) != "" {
			s.renderBackupsErr(w, "Ya hay un disco montado en "+mount+". Desmontalo primero.")
			return
		}
	}
	part := diskFirstPart(dev)
	if part == "" {
		s.renderBackupsErr(w, "No encontré partición en "+dev+". Preparalo primero.")
		return
	}
	if err := os.MkdirAll(mount, 0755); err != nil {
		s.renderBackupsErr(w, "No pude crear "+mount+": "+err.Error())
		return
	}
	if out, err := exec.Command("mount", part, mount).CombinedOutput(); err != nil {
		s.renderBackupsErr(w, "Error al montar: "+strings.TrimSpace(string(out)))
		return
	}
	data := map[string]any{"B": s.gatherBackupsView(), "Msg": "Disco montado en " + mount + "."}
	render(w, "backups_fragment.html", data)
}

func (s *server) handleDiskUnmount(w http.ResponseWriter, r *http.Request) {
	mount := backupMount()
	if out, err := exec.Command("umount", mount).CombinedOutput(); err != nil {
		s.renderBackupsErr(w, "Error al desmontar: "+strings.TrimSpace(string(out)))
		return
	}
	data := map[string]any{"B": s.gatherBackupsView(), "Msg": "Disco desmontado de " + mount + "."}
	render(w, "backups_fragment.html", data)
}

// handleDiskPrepare corre en background porque parted+mkfs puede tardar
// más que el WriteTimeout HTTP (30s) en un disco grande.
func (s *server) handleDiskPrepare(w http.ResponseWriter, r *http.Request) {
	dev := strings.TrimSpace(r.FormValue("dev"))
	if dev == "" || !strings.HasPrefix(dev, "/dev/") {
		s.renderBackupsErr(w, "Disco inválido.")
		return
	}
	sysd := systemDiskName()
	shortDev := dev[strings.LastIndex(dev, "/")+1:]
	if shortDev == sysd {
		s.renderBackupsErr(w, dev+" es el disco del sistema — no lo toco.")
		return
	}
	if !s.diskPrep.start() {
		render(w, "disk_prep_log.html", map[string]any{"Running": true})
		return
	}
	ctx, cancel := bgCtx3min()
	go func() {
		defer cancel()
		runDiskPrepare(ctx, &s.diskPrep, dev, backupMount(), resticPassFile())
	}()
	render(w, "disk_prep_log.html", map[string]any{"Running": true})
}

func (s *server) handleDiskPrepareLog(w http.ResponseWriter, r *http.Request) {
	logText, running, done := s.diskPrep.snapshot()
	render(w, "disk_prep_log.html", map[string]any{
		"Log": logText, "Running": running, "Done": done,
	})
}

func runDiskPrepare(ctx context.Context, proc *bgProcess, dev, mount, passfile string) {
	logf := func(msg string) { proc.Write([]byte(msg + "\n")) } //nolint:errcheck

	logf("Particionando " + dev + " (GPT)...")
	if out, err := exec.CommandContext(ctx, "parted", "-s", dev, "mklabel", "gpt", "mkpart", "primary", "ext4", "0%", "100%").CombinedOutput(); err != nil {
		logf("ERROR: " + strings.TrimSpace(string(out)) + " — " + err.Error())
		proc.finish()
		return
	}
	time.Sleep(time.Second) // esperar a que el kernel vea la nueva partición

	part := diskFirstPart(dev)
	if part == "" {
		logf("ERROR: no apareció la partición en " + dev)
		proc.finish()
		return
	}

	logf("Formateando " + part + " como ext4...")
	if out, err := exec.CommandContext(ctx, "mkfs.ext4", "-F", "-q", part).CombinedOutput(); err != nil {
		logf("ERROR al formatear: " + strings.TrimSpace(string(out)))
		proc.finish()
		return
	}

	logf("Montando en " + mount + "...")
	exec.CommandContext(ctx, "umount", mount).Run() //nolint:errcheck
	if err := os.MkdirAll(mount, 0755); err != nil {
		logf("ERROR: no pude crear " + mount)
		proc.finish()
		return
	}
	if out, err := exec.Command("mount", part, mount).CombinedOutput(); err != nil {
		logf("ERROR al montar: " + strings.TrimSpace(string(out)))
		proc.finish()
		return
	}

	logf("Escribiendo marcador warden...")
	uuidOut, _ := exec.Command("uuidgen").Output()
	if len(strings.TrimSpace(string(uuidOut))) == 0 {
		uuidOut, _ = os.ReadFile("/proc/sys/kernel/random/uuid")
	}
	os.WriteFile(mount+"/"+backupMarker, []byte(strings.TrimSpace(string(uuidOut))+"\n"), 0644) //nolint:errcheck
	os.MkdirAll(mount+"/restic-repo", 0755)                                                     //nolint:errcheck

	if _, err := os.Stat(passfile); err != nil {
		logf("Generando clave restic en " + passfile + "...")
		out, err := exec.Command("openssl", "rand", "-base64", "32").Output()
		if err != nil {
			logf("ERROR al generar clave: " + err.Error())
			proc.finish()
			return
		}
		os.WriteFile(passfile, out, 0600) //nolint:errcheck
	}

	pass, err := os.ReadFile(passfile)
	if err != nil {
		logf("ERROR: no pude leer la clave en " + passfile + ": " + err.Error())
		proc.finish()
		return
	}
	logf("")
	logf("══════════════════════════════════════════")
	logf("  CLAVE DE CIFRADO DEL BACKUP:")
	logf("  " + strings.TrimSpace(string(pass)))
	logf("══════════════════════════════════════════")
	logf("  Sin esta clave NO podés restaurar aunque tengas el disco.")
	logf("  Guardala en un gestor de contraseñas.")
	logf("")
	logf("Disco listo. Activá el backup automático con el botón de arriba.")
	proc.finish()
}

func (s *server) handleSetPassfile(w http.ResponseWriter, r *http.Request) {
	pass := strings.TrimSpace(r.FormValue("passphrase"))
	if pass == "" {
		s.renderBackupsErr(w, "La clave no puede estar vacía.")
		return
	}

	// Validar contra el repo antes de guardar permanentemente.
	repo := backupMount() + "/restic-repo"
	if _, err := os.Stat(repo); err == nil {
		tmp, err := os.CreateTemp("", "warden-pass-*")
		if err == nil {
			tmp.WriteString(pass + "\n")
			tmp.Close()
			defer os.Remove(tmp.Name())

			ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
			defer cancel()
			out, err := exec.CommandContext(ctx, "docker", "run", "--rm",
				"-e", "RESTIC_PASSWORD_FILE=/pass",
				"-v", tmp.Name()+":/pass:ro",
				"-v", repo+":/repo:ro",
				"restic/restic", "-r", "/repo", "snapshots", "--no-lock", "--json").CombinedOutput()
			if err != nil {
				detail := strings.TrimSpace(string(out))
				if strings.Contains(detail, "wrong password") || strings.Contains(detail, "MAC verification") {
					s.renderBackupsErr(w, "Clave incorrecta — no coincide con la del repositorio. Revisá que la copiaste bien.")
				} else {
					s.renderBackupsErr(w, "No pude verificar la clave: "+detail)
				}
				return
			}
		}
	}

	pf := resticPassFile()
	if err := os.WriteFile(pf, []byte(pass+"\n"), 0600); err != nil {
		s.renderBackupsErr(w, "No pude guardar la clave en "+pf+": "+err.Error())
		return
	}
	data := map[string]any{"B": s.gatherBackupsView(), "Msg": "Clave verificada y guardada. Ya podés ver los snapshots."}
	render(w, "backups_fragment.html", data)
}

func (s *server) renderBackupsErr(w http.ResponseWriter, msg string) {
	render(w, "backups_fragment.html", map[string]any{"B": s.gatherBackupsView(), "Err": msg})
}

