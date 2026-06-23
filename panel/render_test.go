package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestNASTemplateNoBrokenQuotes renderiza la página NAS real y verifica que
// no haya quedado JS roto (comillas mal anidadas) ni referencias a $store
// fuera del contexto de Alpine — el bug real que encontramos: hx-on (JS
// plano de htmx) no tiene acceso a $store, solo Alpine.store(...) funciona ahí.
func TestNASTemplateNoBrokenQuotes(t *testing.T) {
	var buf bytes.Buffer
	data := map[string]any{"Page": "nas", "Users": []string{"warden", "maria"}}
	if err := tmpl.ExecuteTemplate(&buf, "nas.html", data); err != nil {
		t.Fatalf("render falló: %v", err)
	}
	html := buf.String()

	// Las directivas hx-on de htmx NUNCA deben usar $store (no existe ahí).
	for _, line := range strings.Split(html, "\n") {
		if strings.Contains(line, "hx-on") && strings.Contains(line, "$store") {
			t.Errorf("hx-on usa $store (no existe fuera de Alpine, rompe en runtime): %s", strings.TrimSpace(line))
		}
	}

	// Comillas simples anidadas sin escapar rompen el JS generado.
	if strings.Contains(html, "''lock''") || strings.Contains(html, `\$watch('Alpine`) {
		t.Errorf("quedó una comilla mal anidada en el JS embebido")
	}

	if !strings.Contains(html, "Alpine.store('lock').request") {
		t.Error("no encontré el wiring del candado (Alpine.store('lock').request) en los forms")
	}
	if !strings.Contains(html, "$store.lock.open") {
		t.Error("el modal mismo debería seguir usando $store (está dentro de directivas Alpine)")
	}
}
