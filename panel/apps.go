package main

import (
	"os"
	"strings"
)

// AppCard es una app real (del catálogo) con su estado y su link directo —
// NO un contenedor crudo. Una app puede tener varios contenedores satélite
// (ej. docmost + docmost-postgres + docmost-redis); acá se agrupan bajo UNA
// sola tarjeta clickeable, en vez de mostrar cada contenedor suelto.
type AppCard struct {
	Tag      string
	Name     string
	Up       bool
	Link     string
	Initial  string // letra para el avatar tipográfico (sin íconos genéricos)
	Deployed bool   // true = vive en su propio repo, vía CI/CD (no la instala warden)
}

// buildAppView separa los contenedores en (a) los que pertenecen a una app
// del catálogo -> AppCards clickeables, y (b) el resto -> lista cruda para
// quien quiera el detalle técnico (sin mezclarlos).
func (s *server) buildAppView(containers []Container) (apps []AppCard, others []Container) {
	comps, _ := listComponentsMerged(s.catalogDirs())
	upByName := map[string]bool{}
	existsByName := map[string]bool{}
	for _, c := range containers {
		upByName[c.Name] = c.Up
		existsByName[c.Name] = true
	}

	claimed := map[string]bool{}
	host, _ := os.Hostname()

	for _, c := range comps {
		// COMP_CONTAINER es opcional en catálogos viejos (se agregó después
		// al diseño) — si falta, lo adivinamos: un contenedor con el mismo
		// nombre que el tag, o que empiece con "tag-" (patrón típico de
		// docker compose: <tag>-<servicio>).
		container := c.Container
		if container == "" {
			if existsByName[c.Tag] || hasAnyPrefixed(containers, c.Tag+"-") {
				container = c.Tag
			} else {
				continue // no hay ningún contenedor que coincida con esta app
			}
		} else if !existsByName[container] && !hasAnyPrefixed(containers, c.Tag+"-") {
			// COMP_CONTAINER está declarado en el catálogo (la receta), pero
			// no hay NINGÚN contenedor real con ese nombre — la app no está
			// instalada (o ya no), no se muestra como si lo estuviera.
			continue
		}
		link := ""
		switch {
		case c.CFHost != "":
			link = "https://" + c.CFHost
		case c.CFPort != "":
			link = "http://" + host + ".local:" + c.CFPort
		}
		apps = append(apps, AppCard{
			Tag: c.Tag, Name: c.Name, Up: upByName[container] || hasAnyRunning(containers, c.Tag+"-"),
			Link: link, Initial: firstLetter(c.Name), Deployed: c.IsDeployed(),
		})
		// Los contenedores satélite suelen compartir el prefijo del tag
		// (docmost-postgres, immich-redis...) — se agrupan bajo la misma app.
		for _, cc := range containers {
			if cc.Name == container || strings.HasPrefix(cc.Name, c.Tag+"-") || strings.HasPrefix(cc.Name, container) {
				claimed[cc.Name] = true
			}
		}
	}

	for _, cc := range containers {
		if !claimed[cc.Name] {
			others = append(others, cc)
		}
	}
	return apps, others
}

func hasAnyPrefixed(containers []Container, prefix string) bool {
	for _, c := range containers {
		if strings.HasPrefix(c.Name, prefix) {
			return true
		}
	}
	return false
}

func hasAnyRunning(containers []Container, prefix string) bool {
	for _, c := range containers {
		if strings.HasPrefix(c.Name, prefix) && c.Up {
			return true
		}
	}
	return false
}

func firstLetter(s string) string {
	for _, r := range s {
		return strings.ToUpper(string(r))
	}
	return "?"
}
