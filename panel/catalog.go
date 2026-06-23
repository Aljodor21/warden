package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Component refleja un archivo site/catalog/<tag>.component.
// Solo conocemos los campos del formato de warden — nada mágico.
type Component struct {
	Tag         string // nombre de archivo sin .component (clave real)
	Comment     string // primera línea "# ..." si existía
	Name        string
	Kind        string
	Paths       []string
	Excludes    []string
	DBType      string
	DBContainer string
	DBName      string
	DBUser      string
	Install     string
	Container   string
	Secrets     []string
	Icon        string
	CFHost      string
	CFPort      string
	Note        string
}

// Permite un comentario inline después del valor (válido en bash, ej.
// COMP_CONTAINER="excalidraw"   # contenedor principal) — sin esto, esas
// líneas no calzaban y el campo quedaba vacío en silencio.
var scalarRe = regexp.MustCompile(`^(COMP_[A-Z_]+)="(.*)"\s*(#.*)?$`)
var arrayStartRe = regexp.MustCompile(`^(COMP_[A-Z_]+)=\((.*)$`)
var quotedItemRe = regexp.MustCompile(`"((?:[^"\\]|\\.)*)"`)

// parseComponentFile lee un .component y devuelve su Component.
func parseComponentFile(path string) (*Component, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	c := &Component{}
	scalars := map[string]*string{
		"COMP_TAG":          &c.Tag,
		"COMP_NAME":         &c.Name,
		"COMP_KIND":         &c.Kind,
		"COMP_DB_TYPE":      &c.DBType,
		"COMP_DB_CONTAINER": &c.DBContainer,
		"COMP_DB_NAME":      &c.DBName,
		"COMP_DB_USER":      &c.DBUser,
		"COMP_INSTALL":      &c.Install,
		"COMP_CONTAINER":    &c.Container,
		"COMP_ICON":         &c.Icon,
		"COMP_CF_HOST":      &c.CFHost,
		"COMP_CF_PORT":      &c.CFPort,
		"COMP_NOTE":         &c.Note,
	}
	arrays := map[string]*[]string{
		"COMP_PATHS":    &c.Paths,
		"COMP_EXCLUDES": &c.Excludes,
		"COMP_SECRETS":  &c.Secrets,
	}

	sc := bufio.NewScanner(f)
	firstLine := true
	var inArrayKey string
	var inArrayBuf strings.Builder

	for sc.Scan() {
		// Tolerar indentación accidental (común al pegar contenido en una
		// terminal con prompt multilínea, o al editar a mano) — sin esto,
		// CUALQUIER espacio antes de "COMP_X=..." hace que el campo entero
		// quede vacío EN SILENCIO (^ ancla el regex al borde exacto de la
		// línea). Bug real visto: un archivo con 2 espacios de indentación
		// en TODAS sus líneas hacía que el avatar de esa app saliera "?"
		// (nombre vacío) sin ningún error visible.
		line := strings.TrimLeft(sc.Text(), " \t")

		if firstLine {
			firstLine = false
			t := strings.TrimSpace(line)
			if strings.HasPrefix(t, "#") {
				c.Comment = strings.TrimSpace(strings.TrimPrefix(t, "#"))
				continue
			}
		}

		if inArrayKey != "" {
			inArrayBuf.WriteString(" ")
			inArrayBuf.WriteString(line)
			if strings.Contains(line, ")") {
				items := extractQuoted(inArrayBuf.String())
				if dst, ok := arrays[inArrayKey]; ok {
					*dst = items
				}
				inArrayKey = ""
				inArrayBuf.Reset()
			}
			continue
		}

		if m := arrayStartRe.FindStringSubmatch(line); m != nil {
			key, rest := m[1], m[2]
			if strings.Contains(rest, ")") {
				items := extractQuoted(rest)
				if dst, ok := arrays[key]; ok {
					*dst = items
				}
			} else {
				inArrayKey = key
				inArrayBuf.WriteString(rest)
			}
			continue
		}

		if m := scalarRe.FindStringSubmatch(line); m != nil {
			key, val := m[1], unescape(m[2])
			if dst, ok := scalars[key]; ok {
				*dst = val
			}
			continue
		}
	}
	return c, sc.Err()
}

func extractQuoted(s string) []string {
	var out []string
	for _, m := range quotedItemRe.FindAllStringSubmatch(s, -1) {
		out = append(out, unescape(m[1]))
	}
	return out
}

func unescape(s string) string {
	return strings.ReplaceAll(s, `\"`, `"`)
}

func escape(s string) string {
	return strings.ReplaceAll(s, `"`, `\"`)
}

// writeComponentFile regenera el archivo .component de forma determinista.
func writeComponentFile(path string, c *Component) error {
	var b strings.Builder
	if c.Comment != "" {
		fmt.Fprintf(&b, "# %s\n", c.Comment)
	}
	fmt.Fprintf(&b, "COMP_NAME=\"%s\"\n", escape(c.Name))
	fmt.Fprintf(&b, "COMP_TAG=\"%s\"\n", escape(c.Tag))
	fmt.Fprintf(&b, "COMP_KIND=\"%s\"\n", escape(c.Kind))
	writeArray(&b, "COMP_PATHS", c.Paths)
	writeArray(&b, "COMP_EXCLUDES", c.Excludes)
	fmt.Fprintf(&b, "COMP_DB_TYPE=\"%s\"\n", escape(c.DBType))
	fmt.Fprintf(&b, "COMP_DB_CONTAINER=\"%s\"\n", escape(c.DBContainer))
	fmt.Fprintf(&b, "COMP_DB_NAME=\"%s\"\n", escape(c.DBName))
	fmt.Fprintf(&b, "COMP_DB_USER=\"%s\"\n", escape(c.DBUser))
	fmt.Fprintf(&b, "COMP_INSTALL=\"%s\"\n", escape(c.Install))
	fmt.Fprintf(&b, "COMP_CONTAINER=\"%s\"\n", escape(c.Container))
	writeArray(&b, "COMP_SECRETS", c.Secrets)
	fmt.Fprintf(&b, "COMP_ICON=\"%s\"\n", escape(c.Icon))
	fmt.Fprintf(&b, "COMP_CF_HOST=\"%s\"\n", escape(c.CFHost))
	fmt.Fprintf(&b, "COMP_CF_PORT=\"%s\"\n", escape(c.CFPort))
	fmt.Fprintf(&b, "COMP_NOTE=\"%s\"\n", escape(c.Note))
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(b.String()), 0644)
}

func writeArray(b *strings.Builder, key string, items []string) {
	if len(items) == 0 {
		fmt.Fprintf(b, "%s=()\n", key)
		return
	}
	fmt.Fprintf(b, "%s=(\n", key)
	for _, it := range items {
		fmt.Fprintf(b, "  \"%s\"\n", escape(it))
	}
	fmt.Fprintf(b, ")\n")
}

// listComponentsOne lee todos los .component de UN directorio.
func listComponentsOne(dir string) ([]*Component, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []*Component
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".component") {
			continue
		}
		c, err := parseComponentFile(dir + "/" + e.Name())
		if err != nil {
			continue
		}
		if c.Tag == "" { // el archivo no definía COMP_TAG explícito
			c.Tag = strings.TrimSuffix(e.Name(), ".component")
		}
		out = append(out, c)
	}
	return out, nil
}

// listComponentsMerged combina varias carpetas de catálogo, en el MISMO
// orden de prioridad que lib/catalog.sh: las carpetas se procesan en orden,
// y si un tag se repite, la última carpeta (normalmente site/catalog) gana
// — así el catálogo genérico del repo y el de tu sitio se ven como uno solo.
func listComponentsMerged(dirs []string) ([]*Component, error) {
	byTag := map[string]*Component{}
	var order []string
	for _, dir := range dirs {
		comps, err := listComponentsOne(dir)
		if err != nil {
			return nil, err
		}
		for _, c := range comps {
			if _, exists := byTag[c.Tag]; !exists {
				order = append(order, c.Tag)
			}
			byTag[c.Tag] = c // último gana
		}
	}
	out := make([]*Component, 0, len(order))
	for _, tag := range order {
		out = append(out, byTag[tag])
	}
	// Orden alfabético por nombre — la fusión de carpetas no tiene un orden
	// natural (depende del orden del filesystem), así que sin esto la lista
	// se ve "desordenada" para quien la mira.
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}
