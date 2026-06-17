package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/postfix/okworkspace/internal/auth"
	"github.com/postfix/okworkspace/internal/users"
)

// sessionUser adapts *users.User to auth.SessionUser so RequireRole can read
// the role from the session-bound user without the auth package importing users.
type sessionUser struct {
	id   int64
	role string
}

func (s sessionUser) UserID() int64   { return s.id }
func (s sessionUser) UserRole() string { return s.role }

// loadCurrentUser is middleware that resolves the session user id to the full
// user record and attaches it to the request context (auth.WithCurrentUser).
// It is the single place authorization data enters the request — downstream
// RequireRole reads ONLY from here, never from client input (T-00.03-01). When
// there is no valid session it responds 401 and short-circuits.
func (h *authHandlers) loadCurrentUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := h.sessions.GetInt64(r.Context(), auth.SessionUserIDKey)
		if id == 0 {
			writeError(w, http.StatusUnauthorized, "Your session expired. Sign in again to continue.")
			return
		}
		u, err := h.users.GetByID(r.Context(), id)
		if err != nil || !u.Active {
			writeError(w, http.StatusUnauthorized, "Your session expired. Sign in again to continue.")
			return
		}
		ctx := auth.WithCurrentUser(r.Context(), sessionUser{id: u.ID, role: u.Role})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// userView is the safe user shape returned to the admin screen — it never
// includes the password hash.
type userView struct {
	ID                 int64  `json:"id"`
	Username           string `json:"username"`
	DisplayName        string `json:"display_name"`
	Role               string `json:"role"`
	MustChangePassword bool   `json:"must_change_password"`
	Active             bool   `json:"active"`
}

func toUserView(u users.User) userView {
	return userView{
		ID:                 u.ID,
		Username:           u.Username,
		DisplayName:        u.DisplayName,
		Role:               u.Role,
		MustChangePassword: u.MustChangePassword,
		Active:             u.Active,
	}
}

// handleListUsers returns all users (admin only). Password hashes are stripped.
func (h *authHandlers) handleListUsers(w http.ResponseWriter, r *http.Request) {
	list, err := users.List(r.Context(), h.users)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Something went wrong. Check your connection and try again.")
		return
	}
	views := make([]userView, 0, len(list))
	for _, u := range list {
		views = append(views, toUserView(u))
	}
	writeJSON(w, http.StatusOK, views)
}

type createUserRequest struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
}

type createUserResponse struct {
	userView
	OneTimePassword string `json:"one_time_password"`
}

// handleCreateUser creates a user with a one-time password (admin only).
func (h *authHandlers) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var req createUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request.")
		return
	}
	u, otp, err := users.Create(r.Context(), h.users, users.NewUser{
		Username:    req.Username,
		DisplayName: req.DisplayName,
		Role:        req.Role,
	})
	if err != nil {
		if errors.Is(err, users.ErrInvalidRole) {
			writeError(w, http.StatusBadRequest, "Choose a role of admin, editor, or reader.")
			return
		}
		if errors.Is(err, users.ErrEmptyDisplayName) {
			writeError(w, http.StatusBadRequest, "Enter a display name.")
			return
		}
		writeError(w, http.StatusBadRequest, "Could not create the user. The username may already be taken.")
		return
	}
	writeJSON(w, http.StatusCreated, createUserResponse{userView: toUserView(*u), OneTimePassword: otp})
}

type setRoleRequest struct {
	Role string `json:"role"`
}

// handleSetRole changes a target user's role (admin only).
func (h *authHandlers) handleSetRole(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req setRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request.")
		return
	}
	if err := users.SetRole(r.Context(), h.users, id, req.Role); err != nil {
		if errors.Is(err, users.ErrInvalidRole) {
			writeError(w, http.StatusBadRequest, "Choose a role of admin, editor, or reader.")
			return
		}
		if errors.Is(err, users.ErrNotFound) {
			writeError(w, http.StatusNotFound, "That user no longer exists.")
			return
		}
		writeError(w, http.StatusInternalServerError, "Something went wrong. Check your connection and try again.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type resetPasswordResponse struct {
	OneTimePassword string `json:"one_time_password"`
}

// handleResetPassword resets a target user's password (admin only) and returns
// the one-time password once.
func (h *authHandlers) handleResetPassword(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	otp, err := users.ResetPassword(r.Context(), h.users, id)
	if err != nil {
		if errors.Is(err, users.ErrNotFound) {
			writeError(w, http.StatusNotFound, "That user no longer exists.")
			return
		}
		writeError(w, http.StatusInternalServerError, "Something went wrong. Check your connection and try again.")
		return
	}
	writeJSON(w, http.StatusOK, resetPasswordResponse{OneTimePassword: otp})
}

// handleDeactivate deactivates a target user (admin only).
func (h *authHandlers) handleDeactivate(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := users.Deactivate(r.Context(), h.users, id); err != nil {
		if errors.Is(err, users.ErrNotFound) {
			writeError(w, http.StatusNotFound, "That user no longer exists.")
			return
		}
		writeError(w, http.StatusInternalServerError, "Something went wrong. Check your connection and try again.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// pathID parses the {id} URL parameter, writing a 400 and returning ok=false on
// a malformed value.
func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	raw := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "Invalid request.")
		return 0, false
	}
	return id, true
}
