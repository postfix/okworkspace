package server

import (
	"net/http"
	"strings"

	"github.com/alexedwards/scs/v2"
	"github.com/justinas/nosurf"
)

// csrfProtect wraps next with nosurf double-submit CSRF protection. nosurf only
// enforces a valid token on unsafe methods (POST/PUT/DELETE/PATCH); safe
// methods (GET/HEAD/OPTIONS) pass through but still receive the CSRF cookie, so
// the SPA can read the token via GET /api/v1/csrf. CSRF is applied to ALL
// mutating routes — no partial coverage (T-00.01-03, SEC-04).
func csrfProtect(next http.Handler, secure bool) http.Handler {
	h := nosurf.New(next)
	h.SetBaseCookie(http.Cookie{
		Name:     "okf_csrf",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
	})
	// Static SPA assets and the SPA fallback are safe GETs; nosurf already
	// exempts safe methods, so no path exemption is required for them. We do
	// not exempt any mutating API route.
	return h
}

// sessionLoad wraps next with the SCS LoadAndSave middleware so every request
// has its session loaded and persisted.
func sessionLoad(sm *scs.SessionManager, next http.Handler) http.Handler {
	return sm.LoadAndSave(next)
}

// isAPIPath reports whether the request targets the JSON API surface.
func isAPIPath(r *http.Request) bool {
	return strings.HasPrefix(r.URL.Path, "/api/")
}
