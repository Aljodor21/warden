package main

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"
	"time"
)

// Sesión de "modo admin": se desbloquea UNA vez por sesión del navegador
// (botón "Admin" arriba), no en cada acción. La cookie es de SESIÓN (sin
// Max-Age/Expires) — el navegador la borra solo al cerrarse, así que cerrar
// sesión/el navegador vuelve a pedir la clave, como pidió Al. Además hay un
// tope absoluto (adminMaxAge) por si el navegador queda abierto mucho tiempo.

const adminCookie = "warden_admin"
const adminMaxAge = 2 * time.Hour

type adminSessions struct {
	mu     sync.Mutex
	tokens map[string]time.Time // token -> vence
}

func newAdminSessions() *adminSessions {
	return &adminSessions{tokens: map[string]time.Time{}}
}

func (a *adminSessions) issue() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	tok := hex.EncodeToString(b)
	a.mu.Lock()
	a.tokens[tok] = time.Now().Add(adminMaxAge)
	a.mu.Unlock()
	return tok
}

func (a *adminSessions) valid(tok string) bool {
	if tok == "" {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	exp, ok := a.tokens[tok]
	if !ok || time.Now().After(exp) {
		delete(a.tokens, tok)
		return false
	}
	return true
}

func (a *adminSessions) revoke(tok string) {
	a.mu.Lock()
	delete(a.tokens, tok)
	a.mu.Unlock()
}

func (s *server) isAdmin(r *http.Request) bool {
	if s.passwordHash == "" {
		return true // sin clave configurada = sin candado (solo pruebas locales)
	}
	c, err := r.Cookie(adminCookie)
	if err != nil {
		return false
	}
	return s.adminSess.valid(c.Value)
}

func (s *server) handleAdminUnlock(w http.ResponseWriter, r *http.Request) {
	if !checkPassword(r.FormValue("pass"), s.passwordHash) {
		render(w, "admin_status.html", map[string]any{"Unlocked": false, "Err": "Clave incorrecta."})
		return
	}
	tok := s.adminSess.issue()
	http.SetCookie(w, &http.Cookie{
		Name: adminCookie, Value: tok, Path: "/",
		HttpOnly: true, SameSite: http.SameSiteStrictMode,
		// Sin Expires/MaxAge -> cookie de sesión: se borra al cerrar el navegador.
	})
	render(w, "admin_status.html", map[string]any{"Unlocked": true})
}

func (s *server) handleAdminStatus(w http.ResponseWriter, r *http.Request) {
	render(w, "admin_status.html", map[string]any{"Unlocked": s.isAdmin(r)})
}

func (s *server) handleAdminLock(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(adminCookie); err == nil {
		s.adminSess.revoke(c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: adminCookie, Value: "", Path: "/", MaxAge: -1})
	render(w, "admin_status.html", map[string]any{"Unlocked": false})
}

// requireAdmin envuelve un handler de acción (POST) exigiendo sesión admin
// activa. Si no, devuelve el fragmento de error indicado en vez de ejecutar
// (extraData arma el resto del contexto que ese fragmento necesite, ej. la
// lista de usuarios del NAS para no dejar el formulario vacío).
func (s *server) requireAdmin(tmplName string, extraData func() map[string]any, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.isAdmin(r) {
			data := extraData()
			data["Err"] = "Desbloqueá el modo admin (botón arriba) para hacer esto."
			render(w, tmplName, data)
			return
		}
		next(w, r)
	}
}
