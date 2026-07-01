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

// maxJSONBodyBytes caps the size of a JSON request body on mutating /api/v1
// routes (F-02). JSON writes are text — a page body, tag lists, user/profile
// fields — far below the 100 MiB multipart attachment cap (which has its OWN
// MaxBytesReader in the upload/replace handlers, tied to storage.max_upload_mb).
// Without this cap an authenticated user could stream an unbounded JSON body
// into json.Decode and pin server memory (memory-pressure DoS). 10 MiB is
// generous for any real page/tag/user write.
const maxJSONBodyBytes = 10 << 20 // 10 MiB

// limitJSONBody caps the request body on mutating methods (POST/PUT/PATCH/
// DELETE) that carry a JSON (i.e. non-multipart) body. It is mounted once on the
// /api/v1 group so every JSON write endpoint (pages create/save, tag-admin
// rename/merge/delete, users/profile, agent applies, …) is covered without
// editing each handler — no per-handler drift.
//
// Multipart uploads are skipped: the attachment upload/replace handlers apply
// their own, larger MaxBytesReader cap (storage.max_upload_mb), so double-capping
// here would wrongly reject legitimate large uploads. Safe methods (GET/HEAD/
// OPTIONS) carry no write body and pass through untouched.
//
// An oversize body returns 413 with a generic message. The Content-Length header
// is checked up front for the clean fast path; the body is ALSO wrapped in
// http.MaxBytesReader so a chunked/length-omitting body still cannot exceed the
// cap (in that exotic case the decoder surfaces the limit as its own 400 — the
// DoS is still prevented, the body is hard-capped either way).
func limitJSONBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
			if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
				if r.ContentLength > maxJSONBodyBytes {
					writeError(w, http.StatusRequestEntityTooLarge, "That request is too large.")
					return
				}
				r.Body = http.MaxBytesReader(w, r.Body, maxJSONBodyBytes)
			}
		}
		next.ServeHTTP(w, r)
	})
}

// isAPIPath reports whether the request targets the JSON API surface.
func isAPIPath(r *http.Request) bool {
	return strings.HasPrefix(r.URL.Path, "/api/")
}
