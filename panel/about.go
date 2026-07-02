package main

import "net/http"

type TechItem struct {
	Name string
	Logo string // archivo en static/logos/, vacío si no hay logo
	URL  string
	Why  string // qué hace exactamente dentro de warden
}

type TechGroup struct {
	Title string
	Items []TechItem
}

// Credit es un recurso externo que warden consume pero no produce.
type Credit struct {
	Name   string
	Author string
	URL    string
	What   string // qué es
	How    string // cómo lo usa warden
}

var aboutGroups = []TechGroup{
	{
		Title: "Panel web",
		Items: []TechItem{
			{"Go", "go.svg", "https://go.dev",
				"Servidor HTTP del panel: binario único, stdlib pura, sin frameworks, sin runtime externo. Pocos MB de RAM."},
			{"HTMX", "htmx.svg", "https://htmx.org",
				"Interactividad del panel via atributos HTML — polling en vivo, logs en tiempo real, swaps parciales. Vendorizado, sin CDN."},
			{"Alpine.js", "alpinedotjs.svg", "https://alpinejs.dev",
				"Estado reactivo del lado del cliente: modal de admin, candado de sesión, toggles. Vendorizado, sin CDN."},
		},
	},
	{
		Title: "Infraestructura y redes",
		Items: []TechItem{
			{"Docker", "docker.svg", "https://www.docker.com",
				"Cada app del catálogo corre en su propio contenedor. El propio restic también corre en Docker — cero instalación en el host."},
			{"Cloudflare Tunnel", "cloudflare.svg", "https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/",
				"Expone apps a internet sin abrir puertos del router ni tener IP pública. warden gestiona el ingress desde el catálogo."},
			{"Tailscale", "tailscale.svg", "https://tailscale.com",
				"VPN mesh para acceder al server desde cualquier red de forma segura. El panel muestra la IP Tailscale y el link de auth."},
			{"GitHub Actions", "githubactions.svg", "https://docs.github.com/actions",
				"CI/CD: un self-hosted runner en tu server construye y despliega tus propias apps directamente, sin ir a la nube."},
		},
	},
	{
		Title: "Sistema y CLI",
		Items: []TechItem{
			{"Bash", "gnubash.svg", "https://www.gnu.org/software/bash/",
				"El instalador (bootstrap.sh) y el comando warden completo. Corre en cualquier Linux sin instalar nada extra."},
			{"Linux", "linux.svg", "https://kernel.org",
				"La base de todo — Debian/Ubuntu y Arch son las distros soportadas. warden detecta el gestor de paquetes y se adapta."},
			{"restic", "", "https://restic.net",
				"Motor de backup: cifrado AES-256, deduplicado por bloque, versionado. Corre en Docker, cero instalación en el host."},
			{"age", "", "https://github.com/FiloSottile/age",
				"Cifrado de secretos (credenciales, tokens). Simple y moderno — reemplaza GPG para este uso. Escrow en site/secrets/."},
			{"ufw", "", "https://launchpad.net/ufw",
				"Firewall: capa legible sobre iptables/nftables. bootstrap.sh lo configura para exponer solo lo necesario."},
			{"gum", "", "https://github.com/charmbracelet/gum",
				"Menús interactivos en la terminal para el instalador — selección, confirmaciones, inputs bonitos sin curses."},
			{"PostgreSQL", "postgresql.svg", "https://www.postgresql.org",
				"Base de datos de las apps del catálogo que la necesitan (Immich, Docmost, proyectos propios). warden dumpea y restaura."},
		},
	},
}

var aboutCredits = []Credit{
	{
		Name:   "dharmx/walls",
		Author: "dharmx",
		URL:    "https://github.com/dharmx/walls",
		What:   "Colección curada de wallpapers de alta calidad en GitHub.",
		How:    "La sección Apariencia carga hasta 96 imágenes vía la API de árboles de GitHub y las muestra paginadas. Se cachean 24h en localStorage — sin descarga si no se usa.",
	},
	{
		Name:   "Portainer Community Templates",
		Author: "Portainer.io",
		URL:    "https://github.com/portainer/portainer/blob/develop/api/stacks/templates.json",
		What:   "JSON público con más de 100 definiciones de apps self-hosted mantenidas por la comunidad de Portainer.",
		How:    "La Tienda de warden consume ese JSON en vivo, muestra la grilla de apps disponibles y, al instalar, descarga el compose directamente. Las apps con receta curada en warden usan la suya propia.",
	},
}

func (s *server) handleAbout(w http.ResponseWriter, r *http.Request) {
	render(w, "about.html", map[string]any{
		"Page":          "about",
		"AdminUnlocked": s.isAdmin(r),
		"Groups":        aboutGroups,
		"Credits":       aboutCredits,
	})
}
