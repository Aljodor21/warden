package main

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

const ageKeyPath = "/etc/warden/age.key"

type SystemView struct {
	TailscaleInstalled bool
	TailscaleConnected bool
	TailscaleIP        string

	AgeKeyExists  bool
	SecretsExist  bool // hay al menos un *.tar.age guardado
	SecretsCount  int
	CloudflareSet bool // /etc/cloudflared/config.yml existe (hay túnel)
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
	if entries, err := os.ReadDir(s.siteSecretsDir()); err == nil {
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".tar.age") {
				v.SecretsCount++
			}
		}
		v.SecretsExist = v.SecretsCount > 0
	}
	return v
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
