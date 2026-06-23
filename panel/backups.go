package main

import (
	"context"
	"encoding/json"
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
	ID    string   `json:"short_id"`
	Time  string   `json:"time"`
	Tags  []string `json:"tags"`
	Paths []string `json:"paths"`
}

type BackupsView struct {
	Disks        []DiskRow
	BackupMount  string // "" si no hay ninguno montado
	Snapshots    []Snapshot
	LastBackup   string // fecha legible del snapshot más reciente, o ""
	SnapshotsErr string
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

func (s *server) gatherBackupsView() BackupsView {
	v := BackupsView{Disks: listDisks(), BackupMount: backupMount()}

	repo := v.BackupMount + "/restic-repo"
	passfile := resticPassFile()
	if _, err := os.Stat(repo); err != nil {
		return v
	}
	if _, err := os.Stat(passfile); err != nil {
		v.SnapshotsErr = "Falta la contraseña restic (" + passfile + ")"
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
		v.SnapshotsErr = "No pude leer los snapshots (¿el disco está montado?)"
		return v
	}
	var snaps []Snapshot
	if err := json.Unmarshal(out, &snaps); err == nil {
		v.Snapshots = snaps
		if len(snaps) > 0 {
			v.LastBackup = snaps[len(snaps)-1].Time
		}
	}
	return v
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
	s.bmu.Lock()
	if s.backupRunning {
		s.bmu.Unlock()
		data := map[string]any{"B": s.gatherBackupsView(), "Running": true}
		render(w, "backups_fragment.html", data)
		return
	}
	s.backupRunning = true
	s.bmu.Unlock()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		_, _ = s.runWarden(ctx, "backup")
		s.bmu.Lock()
		s.backupRunning = false
		s.bmu.Unlock()
	}()

	render(w, "backups_fragment.html", map[string]any{"B": s.gatherBackupsView(), "Running": true})
}
