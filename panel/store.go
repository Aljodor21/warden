package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// Fuente de la tienda: colección agregada de plantillas de Portainer — UN solo
// JSON con cientos de apps, cada una con ícono, descripción, imagen, puertos y
// volúmenes. La usamos para llenar la grilla, y al instalar generamos un
// docker-compose desde esos campos que el importador (modules/import.sh)
// adapta al formato de warden.
const storeTemplatesURL = "https://raw.githubusercontent.com/Lissy93/portainer-templates/main/templates.json"

type portainerTemplate struct {
	Type        int               `json:"type"`
	Title       string            `json:"title"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Logo        string            `json:"logo"`
	Categories  []string          `json:"categories"`
	Image       string            `json:"image"`
	Ports       []json.RawMessage `json:"ports"` // "8080:80/tcp" o {"80/tcp":"8080"}
	Volumes     []portainerVolume `json:"volumes"`
	Env         []portainerEnv    `json:"env"`
}

type portainerVolume struct {
	Container string `json:"container"`
}

type portainerEnv struct {
	Name    string `json:"name"`
	Default string `json:"default"`
}

// StoreApp: una app lista para la grilla.
type StoreApp struct {
	Title       string
	Description string
	Logo        string
	Category    string
	Tag         string
	Search      string // título+categoría en minúsculas, para el filtro del buscador
	Installed   bool   // ya está instalada (contenedor corriendo o en tu site/catalog)
}

// installedTags: apps que el usuario ya tiene. Cuenta lo que está en SU
// site/catalog (lo importado) + lo que está corriendo ahora en Docker. NO
// cuenta las recetas del repo (esas están disponibles, no instaladas).
func (s *server) installedTags() map[string]bool {
	set := map[string]bool{}
	if comps, err := listComponentsOne(s.siteCatalogDir); err == nil {
		for _, c := range comps {
			set[c.Tag] = true
		}
	}
	for name := range runningContainers() {
		set[name] = true
	}
	return set
}

var storeSlugRe = regexp.MustCompile(`[^a-z0-9]+`)

func storeSlug(s string) string {
	return strings.Trim(storeSlugRe.ReplaceAllString(strings.ToLower(s), "-"), "-")
}

// storeTemplatesCached baja y parsea las plantillas (solo type 1: apps de un
// contenedor). Caché de 1h para no golpear la red en cada carga.
func (s *server) storeTemplatesCached() ([]portainerTemplate, error) {
	s.storeMu.Lock()
	defer s.storeMu.Unlock()
	if s.storeTemplates != nil && time.Since(s.storeFetchedAt) < time.Hour {
		return s.storeTemplates, nil
	}
	req, err := http.NewRequest("GET", storeTemplatesURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := cfHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20)) // tope 10MB
	if err != nil {
		return nil, err
	}
	// Parseo tolerante: cada entrada por separado, salteando las malformadas,
	// así una plantilla rota no tumba toda la tienda.
	var wrap struct {
		Templates []json.RawMessage `json:"templates"`
	}
	if err := json.Unmarshal(body, &wrap); err != nil {
		return nil, err
	}
	var out []portainerTemplate
	for _, raw := range wrap.Templates {
		var t portainerTemplate
		if json.Unmarshal(raw, &t) != nil {
			continue
		}
		if t.Type == 1 && t.Image != "" {
			out = append(out, t)
		}
	}
	s.storeTemplates, s.storeFetchedAt = out, time.Now()
	return out, nil
}

// storeApps arma la lista para la grilla (deduplicada por tag).
func (s *server) storeApps() ([]StoreApp, error) {
	tpls, err := s.storeTemplatesCached()
	if err != nil {
		return nil, err
	}
	installed := s.installedTags()
	seen := map[string]bool{}
	var apps []StoreApp
	for _, t := range tpls {
		tag := storeSlug(t.Name)
		if tag == "" {
			tag = storeSlug(t.Title)
		}
		if tag == "" || seen[tag] {
			continue
		}
		seen[tag] = true
		cat := ""
		if len(t.Categories) > 0 {
			cat = t.Categories[0]
		}
		apps = append(apps, StoreApp{
			Title: t.Title, Description: t.Description, Logo: t.Logo,
			Category: cat, Tag: tag,
			Search:    strings.ToLower(t.Title + " " + cat),
			Installed: installed[tag],
		})
	}
	return apps, nil
}

// storeTemplateByTag busca la plantilla cuyo slug coincide con el tag.
func (s *server) storeTemplateByTag(tag string) *portainerTemplate {
	tpls, err := s.storeTemplatesCached()
	if err != nil {
		return nil
	}
	for i := range tpls {
		if storeSlug(tpls[i].Name) == tag || storeSlug(tpls[i].Title) == tag {
			return &tpls[i]
		}
	}
	return nil
}

// composeFromTemplate genera un docker-compose mínimo desde los campos de la
// plantilla, para que el importador lo adapte (puerto libre, datos, kind).
func composeFromTemplate(t portainerTemplate, tag string) string {
	var b strings.Builder
	b.WriteString("services:\n")
	b.WriteString("  " + tag + ":\n")
	b.WriteString("    image: " + t.Image + "\n")
	if host, container := firstPort(t.Ports); container != "" {
		b.WriteString("    ports:\n")
		fmt.Fprintf(&b, "      - \"%s:%s\"\n", host, container)
	}
	var named []string
	if len(t.Volumes) > 0 {
		var volLines []string
		for i, v := range t.Volumes {
			if v.Container == "" {
				continue
			}
			vn := fmt.Sprintf("%s-vol%d", tag, i)
			volLines = append(volLines, fmt.Sprintf("      - \"%s:%s\"\n", vn, v.Container))
			named = append(named, vn)
		}
		if len(volLines) > 0 {
			b.WriteString("    volumes:\n")
			for _, l := range volLines {
				b.WriteString(l)
			}
		}
	}
	var envLines []string
	for _, e := range t.Env {
		if e.Name != "" && e.Default != "" {
			envLines = append(envLines, fmt.Sprintf("      %s: \"%s\"\n", e.Name, escapeYAMLValue(e.Default)))
		}
	}
	if len(envLines) > 0 {
		b.WriteString("    environment:\n")
		for _, l := range envLines {
			b.WriteString(l)
		}
	}
	if len(named) > 0 {
		b.WriteString("volumes:\n")
		for _, vn := range named {
			b.WriteString("  " + vn + ":\n")
		}
	}
	return b.String()
}

// firstPort devuelve el primer par host:contenedor usable de la lista de
// puertos de Portainer (que mezcla strings y objetos).
func firstPort(ports []json.RawMessage) (host, container string) {
	for _, raw := range ports {
		var str string
		if json.Unmarshal(raw, &str) == nil {
			if h, c := parsePortString(str); c != "" {
				return h, c
			}
			continue
		}
		var m map[string]string
		if json.Unmarshal(raw, &m) == nil {
			for k, v := range m {
				if c := stripProto(k); c != "" {
					if v == "" {
						v = c
					}
					return v, c
				}
			}
		}
	}
	return "", ""
}

func parsePortString(s string) (host, container string) {
	s = stripProto(strings.TrimSpace(s))
	parts := strings.Split(s, ":")
	switch len(parts) {
	case 1:
		return parts[0], parts[0]
	case 2:
		return parts[0], parts[1]
	case 3: // ip:host:container
		return parts[1], parts[2]
	}
	return "", ""
}

func stripProto(s string) string {
	if i := strings.IndexByte(s, '/'); i >= 0 {
		return s[:i]
	}
	return s
}

func escapeYAMLValue(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}
