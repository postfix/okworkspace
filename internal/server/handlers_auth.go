package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/alexedwards/scs/v2"
	"github.com/justinas/nosurf"

	"github.com/postfix/okworkspace/internal/audit"
	"github.com/postfix/okworkspace/internal/auth"
	"github.com/postfix/okworkspace/internal/config"
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
