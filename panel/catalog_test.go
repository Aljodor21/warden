package main

import (
	"os"
	"testing"
)

func TestParseAndRoundtripRealCatalog(t *testing.T) {
	comps, err := listComponentsOne("../catalog")
	if err != nil {
		t.Fatalf("listComponentsOne: %v", err)
	}
	if len(comps) == 0 {
		t.Fatal("esperaba al menos un componente en ../catalog")
	}
	for _, c := range comps {
		t.Logf("tag=%s name=%q kind=%s paths=%v secrets=%v cfhost=%q cfport=%q",
			c.Tag, c.Name, c.Kind, c.Paths, c.Secrets, c.CFHost, c.CFPort)
		if c.Name == "" {
			t.Errorf("%s: COMP_NAME vino vacío, el parser falló", c.Tag)
		}
	}
}

func TestRoundtripPreservesFields(t *testing.T) {
	src := "../catalog/immich.component"
	c, err := parseComponentFile(src)
	if err != nil {
		t.Fatal(err)
	}
	c.Tag = "immich"

	tmp := t.TempDir() + "/immich.component"
	if err := writeComponentFile(tmp, c); err != nil {
		t.Fatal(err)
	}

	c2, err := parseComponentFile(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if c2.Name != c.Name || c2.DBContainer != c.DBContainer || len(c2.Paths) != len(c.Paths) {
		t.Fatalf("el roundtrip no preservó los datos: %+v vs %+v", c, c2)
	}
	if len(c2.Paths) > 0 && c2.Paths[0] != c.Paths[0] {
		t.Fatalf("ruta no coincide: %q vs %q", c2.Paths[0], c.Paths[0])
	}

	b, _ := os.ReadFile(tmp)
	t.Logf("archivo regenerado:\n%s", b)
}

// TestMergedCatalogCombinesRepoAndSite reproduce el bug real reportado por
// Al: en su VM, site/catalog solo tenía 1 componente propio (click-counter),
// y el dashboard mostraba "1/1" en vez de ver también Immich/Docmost/NAS/
// Excalidraw — porque el panel solo leía site/catalog, ignorando catalog/
// del repo. listComponentsMerged debe ver AMBAS carpetas combinadas, igual
// que hace lib/catalog.sh (catalog_each) en bash.
func TestMergedCatalogCombinesRepoAndSite(t *testing.T) {
	siteDir := t.TempDir()
	os.WriteFile(siteDir+"/click-counter.component", []byte(`COMP_TAG="click-counter"
COMP_NAME="Click Counter"
`), 0644)

	comps, err := listComponentsMerged([]string{"../catalog", siteDir})
	if err != nil {
		t.Fatal(err)
	}
	tags := map[string]bool{}
	for _, c := range comps {
		tags[c.Tag] = true
	}
	for _, want := range []string{"immich", "docmost", "nas", "excalidraw", "click-counter"} {
		if !tags[want] {
			t.Errorf("esperaba ver '%s' en el catálogo combinado, no apareció (tags vistos: %v)", want, tags)
		}
	}
}

// TestInlineCommentDoesNotBreakParsing reproduce el bug real reportado por
// Al: excalidraw.component tiene `COMP_CONTAINER="excalidraw"   # comentario`
// (un comentario después del valor, válido en bash) — el parser exigía que
// la línea terminara justo en la comilla de cierre, así que el campo
// quedaba vacío en silencio y el punto de estado salía siempre gris,
// aunque el contenedor estuviera corriendo de verdad.
func TestInlineCommentDoesNotBreakParsing(t *testing.T) {
	c, err := parseComponentFile("../catalog/excalidraw.component")
	if err != nil {
		t.Fatal(err)
	}
	if c.Container != "excalidraw" {
		t.Errorf("COMP_CONTAINER con comentario inline no se parseó: got %q, want %q", c.Container, "excalidraw")
	}
}

// TestMergedCatalogSiteOverridesRepo: si el mismo tag existe en las dos
// carpetas, site/catalog debe ganar (igual que en bash).
func TestMergedCatalogSiteOverridesRepo(t *testing.T) {
	siteDir := t.TempDir()
	os.WriteFile(siteDir+"/immich.component", []byte(`COMP_TAG="immich"
COMP_NAME="Mi Immich personalizado"
`), 0644)

	comps, err := listComponentsMerged([]string{"../catalog", siteDir})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range comps {
		if c.Tag == "immich" && c.Name != "Mi Immich personalizado" {
			t.Errorf("site/catalog debería ganar sobre catalog/ del repo, pero quedó: %q", c.Name)
		}
	}
}
