package main

import (
	"encoding/base64"
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

// cloudflareTunnelDomain averigua a QUÉ dominio está atado el túnel (el que se
// eligió en 'cloudflared tunnel login'). El cert.pem trae un bloque "ARGO
// TUNNEL TOKEN" en base64 con el zoneID adentro; con el API token resolvemos su
// nombre. Best-effort: si algo falla (sin cert, sin token, API no accesible),
// devuelve "" y el panel cae a listar todos los dominios de la cuenta.
func cloudflareTunnelDomain() string {
	var pem string
	for _, p := range []string{"/etc/cloudflared/cert.pem", "/root/.cloudflared/cert.pem"} {
		if b, err := os.ReadFile(p); err == nil {
			pem = string(b)
			break
		}
	}
	const begin = "-----BEGIN ARGO TUNNEL TOKEN-----"
	const end = "-----END ARGO TUNNEL TOKEN-----"
	i, j := strings.Index(pem, begin), strings.Index(pem, end)
	if i < 0 || j <= i {
		return ""
	}
	b64 := strings.Join(strings.Fields(pem[i+len(begin):j]), "")
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return ""
	}
	var tok map[string]any
	if json.Unmarshal(raw, &tok) != nil {
		return ""
	}
	zoneID, _ := tok["zoneID"].(string)
	if zoneID == "" {
		zoneID, _ = tok["zone_id"].(string)
	}
	if zoneID == "" {
		return ""
	}
	token, err := readCloudflareToken()
	if err != nil {
		return ""
	}
	var zone struct {
		Result  struct{ Name string } `json:"result"`
		Success bool                  `json:"success"`
	}
	if cfAPIRequest("GET", cfAPIBaseURL+"/zones/"+zoneID, token, &zone) != nil || !zone.Success {
		return ""
	}
	return zone.Result.Name
}

// rootDomainFromHost extrae el dominio de 2 niveles de un hostname completo
// (ej. "servelejo.site" de "clicks.servelejo.site").
func rootDomainFromHost(host string) string {
	parts := strings.Split(strings.TrimSpace(host), ".")
	if len(parts) < 2 {
		return host
	}
	return strings.Join(parts[len(parts)-2:], ".")
}

// cloudflareZones lista los dominios (zonas) que el API token guardado puede
// ver en tu cuenta de Cloudflare. Responde "¿qué dominios tengo para elegir?"
// al ponerle un subdominio a una app. Sin token o si la API falla, devuelve
// nil — es info opcional, no crítica.
func cloudflareZones() []string {
	token, err := readCloudflareToken()
	if err != nil {
		return nil
	}
	var zones cfZoneList
	if err := cfAPIRequest("GET", cfAPIBaseURL+"/zones?per_page=50", token, &zones); err != nil || !zones.Success {
		return nil
	}
	var out []string
	for _, z := range zones.Result {
		if z.Name != "" {
			out = append(out, z.Name)
		}
	}
	return out
}

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
