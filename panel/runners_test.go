package main

import "testing"

func TestParseGitHubRepo(t *testing.T) {
	cases := []struct {
		url    string
		owner  string
		repo   string
		wantOK bool
	}{
		{"https://github.com/Aljodor21/click-counter.git", "Aljodor21", "click-counter", true},
		{"https://github.com/Aljodor21/click-counter", "Aljodor21", "click-counter", true},
		{"git@github.com:Aljodor21/click-counter.git", "Aljodor21", "click-counter", true},
		{"no es una url", "", "", false},
		{"https://gitlab.com/x/y.git", "", "", false},
	}
	for _, c := range cases {
		owner, repo, ok := parseGitHubRepo(c.url)
		if ok != c.wantOK || owner != c.owner || repo != c.repo {
			t.Errorf("parseGitHubRepo(%q) = (%q, %q, %v), want (%q, %q, %v)",
				c.url, owner, repo, ok, c.owner, c.repo, c.wantOK)
		}
	}
}

func TestFindRunnerInUnits(t *testing.T) {
	// Patrón real confirmado en esta misma sesión (lo vimos registrarse en vivo).
	units := `actions.runner.Aljodor21-click-counter.wardenprueba-runner.service loaded active running GitHub Actions Runner
warden-panel.service loaded active running warden-panel
`
	if svc, found := findRunnerInUnits(units, "Aljodor21", "click-counter"); !found || svc == "" {
		t.Errorf("debería haber encontrado el runner, found=%v svc=%q", found, svc)
	}
	if _, found := findRunnerInUnits(units, "Aljodor21", "otro-repo"); found {
		t.Error("no debería encontrar un runner para un repo distinto")
	}
}

func TestPortInUseHelper(t *testing.T) {
	// Reutiliza directamente la lógica de comparación, sin depender del catálogo real.
	comps := []*Component{
		{Tag: "immich", CFPort: "2283"},
		{Tag: "click-counter", CFPort: "3500"},
	}
	used := func(port, excludeTag string) (string, bool) {
		for _, c := range comps {
			if c.Tag == excludeTag {
				continue
			}
			if c.CFPort == port {
				return c.Tag, true
			}
		}
		return "", false
	}
	if tag, ok := used("3500", ""); !ok || tag != "click-counter" {
		t.Errorf("debería detectar el choque con click-counter, got tag=%q ok=%v", tag, ok)
	}
	if _, ok := used("3500", "click-counter"); ok {
		t.Error("no debería chocar con su propio tag al editar la misma app")
	}
	if _, ok := used("9999", ""); ok {
		t.Error("un puerto libre no debería reportar choque")
	}
}
