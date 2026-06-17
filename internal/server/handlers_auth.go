package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/alexedwards/scs/v2"
	"github.com/justinas/nosurf"

	"github.com/postfix/okworkspace/internal/auth"
	"github.com/postfix/okworkspace/internal/config"
	"github.com/postfix/okworkspace/internal/users"
)

// genericLoginError is the single message returned for any failed login, so an
// attacker cannot distinguish unknown-user from wrong-password (no enumeration).
const genericLoginError = "Invalid username or password."

type authHandlers struct {
	sessions *scs.SessionManager
	users    *users.Repository
	config   config.Config
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type meResponse struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
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

	writeJSON(w, http.StatusOK, meResponse{
		Username:    u.Username,
		DisplayName: u.DisplayName,
		Role:        u.Role,
	})
}

// handleLogout destroys the session.
func (h *authHandlers) handleLogout(w http.ResponseWriter, r *http.Request) {
	if err := h.sessions.Destroy(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "Something went wrong. Check your connection and try again.")
		return
	}
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
		Username:    u.Username,
		DisplayName: u.DisplayName,
		Role:        u.Role,
	})
}
