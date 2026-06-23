package main

import "net/http"

type TechLogo struct {
	Name string
	Logo string // archivo en static/logos/, vacío si no hay logo oficial disponible
	URL  string
	Why  string
}

var aboutTech = []TechLogo{
	{"Go", "go.svg", "https://go.dev", "El panel web (warden-panel): un solo binario, sin frameworks, pocos MB de RAM"},
	{"HTMX", "htmx.svg", "https://htmx.org", "Interactividad del panel sin escribir JavaScript a mano, vendorizado (sin CDN)"},
	{"Alpine.js", "alpinedotjs.svg", "https://alpinejs.dev", "Estado del lado del cliente (modales, candado de admin), también vendorizado"},
	{"Bash", "gnubash.svg", "https://www.gnu.org/software/bash/", "El instalador y el comando warden — corre en cualquier Linux, sin runtime que instalar"},
	{"Docker", "docker.svg", "https://www.docker.com", "Cada app del catálogo, y el propio restic corre en un contenedor (cero instalación)"},
	{"PostgreSQL", "postgresql.svg", "https://www.postgresql.org", "La base de datos de las apps del catálogo (Immich, Docmost, tus proyectos)"},
	{"Cloudflare", "cloudflare.svg", "https://www.cloudflare.com", "Cloudflare Tunnel: exponer apps a internet sin abrir puertos del router"},
	{"Tailscale", "tailscale.svg", "https://tailscale.com", "VPN para entrar al server de forma segura desde cualquier lado"},
	{"GitHub Actions", "githubactions.svg", "https://docs.github.com/actions", "CI/CD: un self-hosted runner en tu propio server construye y despliega tus apps"},
	{"Linux", "linux.svg", "https://kernel.org", "El sistema operativo en el que vive todo esto (Debian/Ubuntu o Arch)"},
	{"restic", "", "https://restic.net", "Motor de backup: cifrado, versionado y deduplicado"},
	{"age", "", "https://github.com/FiloSottile/age", "Cifrado de secretos (contraseñas, credenciales) — simple y moderno, sin GPG"},
	{"gum", "", "https://github.com/charmbracelet/gum", "Menús bonitos en la terminal para el instalador"},
	{"ufw", "", "https://launchpad.net/ufw", "Firewall — capa simple sobre iptables/nftables"},
}

func (s *server) handleAbout(w http.ResponseWriter, r *http.Request) {
	render(w, "about.html", map[string]any{
		"Page": "about", "AdminUnlocked": s.isAdmin(r), "Tech": aboutTech,
	})
}
