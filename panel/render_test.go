package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestNASPageRendersCleanly verifica que la página NAS renderiza sin que
// queden referencias rotas (el bug real que encontramos: $store de Alpine
// no existe fuera de directivas Alpine, así que el viejo diseño "candado
// por acción" via hx-on rompía en silencio). El diseño actual usa una
// sesión de admin por cookie en vez de pedir la clave en cada botón.
func TestNASPageRendersCleanly(t *testing.T) {
	var buf bytes.Buffer
	data := map[string]any{"Page": "nas", "Users": []string{"warden", "maria"}, "AdminUnlocked": false}
	if err := tmpl.ExecuteTemplate(&buf, "nas.html", data); err != nil {
		t.Fatalf("render falló: %v", err)
	}
	html := buf.String()
	if strings.Contains(html, "$store") {
		t.Error("$store no debería aparecer — el candado ahora es una sesión por cookie, no un modal de Alpine")
	}
	if !strings.Contains(html, `id="admin-modal"`) {
		t.Error("no encontré el modal de admin en la nav")
	}
}

// TestAdminSessionLifecycle prueba el ciclo completo: bloqueado por defecto,
// clave incorrecta rechazada, clave correcta desbloquea, y sin la cookie
// (= cerrar el navegador) vuelve a pedirla — el comportamiento que Al pidió.
func TestAdminSessionLifecycle(t *testing.T) {
	s := &server{passwordHash: sha256Hex("abc123"), adminSess: newAdminSessions()}

	req := httptest.NewRequest("GET", "/", nil)
	if s.isAdmin(req) {
		t.Fatal("no debería estar admin sin cookie")
	}

	rec := httptest.NewRecorder()
	unlockReq := httptest.NewRequest("POST", "/admin/unlock", strings.NewReader("pass=mal"))
	unlockReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	s.handleAdminUnlock(rec, unlockReq)
	if strings.Contains(rec.Body.String(), "unlocked") {
		t.Fatal("una clave incorrecta no debería desbloquear nada")
	}

	rec2 := httptest.NewRecorder()
	unlockReq2 := httptest.NewRequest("POST", "/admin/unlock", strings.NewReader("pass=abc123"))
	unlockReq2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	s.handleAdminUnlock(rec2, unlockReq2)
	cookies := rec2.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("clave correcta debería haber puesto una cookie de sesión")
	}
	cookie := cookies[0]
	if cookie.Expires.IsZero() == false || cookie.MaxAge != 0 {
		t.Errorf("la cookie debe ser de SESIÓN (sin Expires/MaxAge) para que se borre al cerrar el navegador, tiene Expires=%v MaxAge=%d", cookie.Expires, cookie.MaxAge)
	}

	reqWithCookie := httptest.NewRequest("GET", "/", nil)
	reqWithCookie.AddCookie(cookie)
	if !s.isAdmin(reqWithCookie) {
		t.Fatal("con la cookie de la sesión desbloqueada, isAdmin debería ser true")
	}

	reqNoCookie := httptest.NewRequest("GET", "/", nil)
	if s.isAdmin(reqNoCookie) {
		t.Fatal("sin la cookie (otra sesión/navegador cerrado) debería volver a pedir la clave")
	}
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
