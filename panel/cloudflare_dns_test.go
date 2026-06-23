package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRootDomain(t *testing.T) {
	cases := map[string]string{
		"clicks.servelejo.site": "servelejo.site",
		"servelejo.site":        "servelejo.site",
		"a.b.c.example.com":     "example.com",
	}
	for in, want := range cases {
		if got := rootDomain(in); got != want {
			t.Errorf("rootDomain(%q) = %q, want %q", in, got, want)
		}
	}
}

// withFakeCloudflareToken apunta cfTokenPath/cfAPIBaseURL a un entorno
// aislado para el test, y restaura los valores reales al terminar.
func withFakeCloudflareToken(t *testing.T, baseURL string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "token")
	if err := os.WriteFile(path, []byte("fake-token\n"), 0600); err != nil {
		t.Fatal(err)
	}
	origPath, origURL := cfTokenPathVar, cfAPIBaseURL
	cfTokenPathVar = path
	cfAPIBaseURL = baseURL
	t.Cleanup(func() { cfTokenPathVar, cfAPIBaseURL = origPath, origURL })
}

// TestDeleteDNSRecordFlow corre la función REAL deleteDNSRecord contra un
// servidor que simula las 3 llamadas de la API de Cloudflare (buscar zona,
// buscar registro, borrar) — confirma que se encadenan bien.
func TestDeleteDNSRecordFlow(t *testing.T) {
	var sawDelete bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "DELETE":
			sawDelete = true
			json.NewEncoder(w).Encode(map[string]any{"success": true})
		case strings.Contains(r.URL.Path, "dns_records"):
			json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"result":  []map[string]string{{"id": "rec456"}},
			})
		default: // /zones?name=...
			json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"result":  []map[string]string{{"id": "zone123", "name": "servelejo.site"}},
			})
		}
	}))
	defer srv.Close()
	withFakeCloudflareToken(t, srv.URL)

	msg, err := deleteDNSRecord("clicks.servelejo.site")
	if err != nil {
		t.Fatalf("deleteDNSRecord falló: %v", err)
	}
	if !sawDelete {
		t.Error("nunca llegó la llamada DELETE al servidor simulado")
	}
	if !strings.Contains(msg, "borrado") {
		t.Errorf("mensaje inesperado: %q", msg)
	}
}

// TestDeleteDNSRecordNoZone: si la zona no existe, error claro, sin
// intentar nada más (ni inventar que funcionó).
func TestDeleteDNSRecordNoZone(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"success": true, "result": []map[string]string{}})
	}))
	defer srv.Close()
	withFakeCloudflareToken(t, srv.URL)

	_, err := deleteDNSRecord("clicks.servelejo.site")
	if err == nil {
		t.Fatal("esperaba un error cuando no se encuentra la zona")
	}
}
