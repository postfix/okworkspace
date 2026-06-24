package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/alexedwards/scs/v2"
	"github.com/justinas/nosurf"

	"github.com/postfix/okworkspace/internal/agent"
	"github.com/postfix/okworkspace/internal/attachments"
	"github.com/postfix/okworkspace/internal/audit"
	"github.com/postfix/okworkspace/internal/auth"
	"github.com/postfix/okworkspace/internal/config"
	"github.com/postfix/okworkspace/internal/graph"
	"github.com/postfix/okworkspace/internal/locks"
	"github.com/postfix/okworkspace/internal/pages"
	"github.com/postfix/okworkspace/internal/search"
	"github.com/postfix/okworkspace/internal/tagsweep"
	"github.com/postfix/okworkspace/internal/users"
)

// genericLoginError is the single message returned for any failed login, so an
// attacker cannot distinguish unknown-user from wrong-password (no enumeration).
const genericLoginError = "Invalid username or password."

// auditSourceWeb is the audit Source for all HTTP-API-driven events.
const auditSourceWeb = "web-ui"

// auditRecorder is the subset of *audit.Logger the handlers call. Defined as an
// interface so the server can inject a no-op when audit is unconfigured and so
// audit failures (already non-fatal in audit.Record) never break a request.
type auditRecorder interface {
	Record(ctx context.Context, e audit.Event) error
}

// nopAudit is the no-op recorder used when Deps.Audit is nil.
type nopAudit struct{}

func (nopAudit) Record(context.Context, audit.Event) error { return nil }

type authHandlers struct {
	sessions *scs.SessionManager
	users    *users.Repository
	config   config.Config
	audit    auditRecorder
	// pages is the page lifecycle service (tree/get/create/save/folder).
	// Optional: when nil the page/tree handlers return a 500 rather than panic,
	// following the existing optional-dependency pattern.
	pages *pages.Service
	// attachments is the attachment lifecycle service (upload/list/download).
	// Optional: when nil the attachment handlers return a 500, following the same
	// optional-dependency pattern as pages.
	attachments *attachments.Service
	// search is the full-text search index (GET /search + admin reindex).
	// Optional: when nil the search handlers return a 500 with the generic copy.
	search *search.Index
	// searchJobs enqueues the rebuild job for the admin reindex (fire-and-forget).
	// Optional: when nil reindex returns a 500.
	searchJobs searchEnqueuer
	// graphJobs enqueues the from-files rebuild job for the admin graph reindex
	// (fire-and-forget). Optional: when nil the graph reindex returns a 500.
	graphJobs graphEnqueuer
	// graph is the derived link/tag adjacency store backing the authed graph READ
	// endpoints (/graph, /graph/local, /graph/backlinks). Optional: when nil those
	// reads return a 500 with the generic copy, following the same
	// optional-dependency pattern as search.
	graph *graph.Store
	// agent is the Eino agent service backing POST /agent/chat (Ask). Optional:
	// when nil the handler returns a 500; when constructed-but-disabled the
	// handler returns a structured "agent off" error rather than hanging.
	agent *agent.Service
	// locks is the soft-lock store backing the acquire/force/release endpoints
	// (COLL-02). Optional: when nil the lock handlers return a 500, following the
	// same optional-dependency pattern as pages. The HTTP layer fills Owner's
	// identity FROM THE SESSION; only the opaque connection id comes from the body.
	locks *locks.Service
	// tagSuggestions is the bulk-sweep staging store backing the admin sweep-start
	// (target enumeration) + review-queue read endpoints (TAG-05). Optional: when
	// nil those routes return a 500, following the same optional-dependency pattern.
	tagSuggestions *tagsweep.Store
	// tagSweepJobs enqueues one KindTagSuggest job per target page for the admin
	// sweep-start (fire-and-forget). Optional: when nil sweep-start returns a 500. In
	// main.go this is the SAME single worker passed as SearchJobs/GraphJobs.
	tagSweepJobs tagSweepEnqueuer
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type meResponse struct {
	Username           string `json:"username"`
	DisplayName        string `json:"display_name"`
	Role               string `json:"role"`
	MustChangePassword bool   `json:"must_change_password"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// handleCSRF returns the per-session CSRF token for the SPA to echo in the
// X-CSRF-Token header on mutating requests.
func (h *authHandlers) handleCSRF(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"csrf_token": nosurf.Token(r)})
}

// handleLogin authenticates credentials, stores the user id in the session, and
// renews the session token to prevent fixation.
func (h *authHandlers) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request.")
		return
	}

	uid, err := auth.Authenticate(r.Context(), h.users, req.Username, req.Password)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			writeError(w, http.StatusUnauthorized, genericLoginError)
			return
		}
		writeError(w, http.StatusInternalServerError, "Something went wrong. Check your connection and try again.")
		return
	}

	u, err := h.users.GetByID(r.Context(), uid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Something went wrong. Check your connection and try again.")
		return
	}

	// Anti session-fixation: renew the token on privilege change.
	if err := h.sessions.RenewToken(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "Something went wrong. Check your connection and try again.")
		return
	}
	h.sessions.Put(r.Context(), auth.SessionUserIDKey, u.ID)

	// SEC-05: record the login (non-fatal — audit.Record never breaks the path).
	_ = h.audit.Record(r.Context(), audit.Event{
		Action: audit.ActionLogin,
		Actor:  u.Username,
		Source: auditSourceWeb,
	})

	writeJSON(w, http.StatusOK, meResponse{
		Username:           u.Username,
		DisplayName:        u.DisplayName,
		Role:               u.Role,
		MustChangePassword: u.MustChangePassword,
	})
}

// handleLogout destroys the session.
func (h *authHandlers) handleLogout(w http.ResponseWriter, r *http.Request) {
	// Capture the actor BEFORE destroying the session so the audit row names who
	// logged out. An unknown/expired session records an empty actor.
	actor := "unknown"
	if id := h.sessions.GetInt64(r.Context(), auth.SessionUserIDKey); id != 0 {
		if u, err := h.users.GetByID(r.Context(), id); err == nil {
			actor = u.Username
		}
	}
	if err := h.sessions.Destroy(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "Something went wrong. Check your connection and try again.")
		return
	}
	_ = h.audit.Record(r.Context(), audit.Event{
		Action: audit.ActionLogout,
		Actor:  actor,
		Source: auditSourceWeb,
	})
	w.WriteHeader(http.StatusNoContent)
}

// handleMe returns the session-bound user, or 401 if there is no session.
func (h *authHandlers) handleMe(w http.ResponseWriter, r *http.Request) {
	id := h.sessions.GetInt64(r.Context(), auth.SessionUserIDKey)
	if id == 0 {
		writeError(w, http.StatusUnauthorized, "Your session expired. Sign in again to continue.")
		return
	}
	u, err := h.users.GetByID(r.Context(), id)
	if err != nil {
		// Session points at a missing user — treat as unauthenticated.
		writeError(w, http.StatusUnauthorized, "Your session expired. Sign in again to continue.")
		return
	}
	writeJSON(w, http.StatusOK, meResponse{
		Username:           u.Username,
		DisplayName:        u.DisplayName,
		Role:               u.Role,
		MustChangePassword: u.MustChangePassword,
	})
}
