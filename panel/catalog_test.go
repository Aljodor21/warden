package main

import (
	"os"
	"testing"
)

func TestParseAndRoundtripRealCatalog(t *testing.T) {
	comps, err := listComponents("../catalog")
	if err != nil {
		t.Fatalf("listComponents: %v", err)
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
