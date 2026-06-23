package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// El token de Cloudflare API (DNS:Edit) es DISTINTO del cert.pem que usa
// 'cloudflared' para los túneles — ese solo sirve para Argo Tunnel, no
// para la API REST general. Se guarda aparte, igual de protegido.
// (variable, no const: los tests la apuntan a un archivo temporal)
var cfTokenPathVar = "/etc/warden/cloudflare-api-token"

func cloudflareTokenExists() bool {
	_, err := os.Stat(cfTokenPathVar)
	return err == nil
}

func readCloudflareToken() (string, error) {
	b, err := os.ReadFile(cfTokenPathVar)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

func saveCloudflareToken(token string) error {
	if err := os.MkdirAll(filepath.Dir(cfTokenPathVar), 0755); err != nil {
		return err
	}
	return os.WriteFile(cfTokenPathVar, []byte(strings.TrimSpace(token)+"\n"), 0600)
}

var cfHTTPClient = &http.Client{Timeout: 15 * time.Second}

// cfAPIBaseURL es inyectable para tests (apunta a un servidor simulado en
// vez de la API real de Cloudflare).
var cfAPIBaseURL = "https://api.cloudflare.com/client/v4"

func cfAPIRequest(method, url, token string, out any) error {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := cfHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("Cloudflare API devolvió %d", resp.StatusCode)
	}
	return nil
}

type cfZoneList struct {
	Result []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"result"`
	Success bool `json:"success"`
}

type cfRecordList struct {
	Result []struct {
		ID string `json:"id"`
	} `json:"result"`
	Success bool `json:"success"`
}

// rootDomain extrae el dominio de 2 niveles (ej. "servelejo.site" de
// "clicks.servelejo.site"). No maneja TLDs compuestos (.co.uk) — suficiente
// para el caso real de uso, y si falla, el error de la API lo deja claro.
func rootDomain(hostname string) string {
	parts := strings.Split(hostname, ".")
	if len(parts) < 2 {
		return hostname
	}
	return strings.Join(parts[len(parts)-2:], ".")
}

// deleteDNSRecord borra el registro DNS (CNAME) de 'hostname' usando la
// API de Cloudflare. Si la zona o el registro no existen, no es un error
// fatal (puede que ya estuviera borrado) — se reporta tal cual.
func deleteDNSRecord(hostname string) (string, error) {
	token, err := readCloudflareToken()
	if err != nil {
		return "", fmt.Errorf("no pude leer el token guardado: %w", err)
	}
	domain := rootDomain(hostname)

	var zones cfZoneList
	if err := cfAPIRequest("GET",
		cfAPIBaseURL+"/zones?name="+domain, token, &zones); err != nil {
		return "", fmt.Errorf("buscando la zona '%s': %w", domain, err)
	}
	if !zones.Success || len(zones.Result) == 0 {
		return "", fmt.Errorf("no encontré la zona '%s' en tu cuenta de Cloudflare (¿el token tiene acceso a ese dominio?)", domain)
	}
	zoneID := zones.Result[0].ID

	var records cfRecordList
	if err := cfAPIRequest("GET",
		cfAPIBaseURL+"/zones/"+zoneID+"/dns_records?name="+hostname, token, &records); err != nil {
		return "", fmt.Errorf("buscando el registro DNS de '%s': %w", hostname, err)
	}
	if !records.Success || len(records.Result) == 0 {
		return "no había ningún registro DNS para borrar (puede que ya no existiera)", nil
	}
	recordID := records.Result[0].ID

	if err := cfAPIRequest("DELETE",
		cfAPIBaseURL+"/zones/"+zoneID+"/dns_records/"+recordID, token, nil); err != nil {
		return "", fmt.Errorf("borrando el registro: %w", err)
	}
	return "registro DNS de '" + hostname + "' borrado", nil
}

func (s *server) handleSaveCloudflareToken(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.FormValue("token"))
	if token == "" {
		render(w, "system_fragment.html", map[string]any{"Sys": s.gatherSystemView(), "Err": "El token no puede estar vacío."})
		return
	}
	if err := saveCloudflareToken(token); err != nil {
		render(w, "system_fragment.html", map[string]any{"Sys": s.gatherSystemView(), "Err": "No pude guardar el token: " + err.Error()})
		return
	}
	render(w, "system_fragment.html", map[string]any{"Sys": s.gatherSystemView(), "Output": "Token guardado."})
}
