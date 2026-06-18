package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/postfix/okworkspace/internal/audit"
	"github.com/postfix/okworkspace/internal/auth"
	"github.com/postfix/okworkspace/internal/users"
)

// actorUsername resolves the session-bound user's username for the audit actor
// field. Falls back to "unknown" if the user cannot be resolved.
func (h *authHandlers) actorUsername(ctx context.Context) string {
	cur, ok := auth.CurrentUser(ctx)
	if !ok {
		return "unknown"
	}
	u, err := h.users.GetByID(ctx, cur.UserID())
	if err != nil {
		return "unknown"
	}
	return u.Username
}

// targetUsername resolves a target user's username by id for the audit target
// field. Falls back to the numeric id string when the user cannot be resolved.
func (h *authHandlers) targetUsername(ctx context.Context, id int64) string {
	u, err := h.users.GetByID(ctx, id)
	if err != nil {
		return strconv.FormatInt(id, 10)
	}
	return u.Username
}

// sessionUser adapts *users.User to auth.SessionUser so RequireRole can read
// the role from the session-bound user without the auth package importing users.
type sessionUser struct {
	id   int64
	role string
}

func (s sessionUser) UserID() int64    { return s.id }
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
		// CR-01: enforce the forced-password-change gate SERVER-SIDE, not just in
		// the SPA. A user holding a temporary/one-time password obtains a valid
		// session, so without this check they could call any authenticated
		// endpoint (profile edits, and if admin, all user-management routes) by
		// issuing the HTTP request directly. While must_change_password is set we
		// reject every authenticated route EXCEPT the self-service password
		// change. (GET /api/v1/auth/me is NOT in this group — it is served on the
		// unauthenticated /auth subtree — so the SPA can still read the flag.)
		if u.MustChangePassword && r.URL.Path != changePasswordPath {
			writeError(w, http.StatusForbidden, "Set a new password to continue.")
			return
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// changePasswordPath is the single self-service route exempted from the
// must_change_password gate (CR-01). It MUST match the route registered in
// router.go (authed.Put("/profile/password", ...)) exactly.
const changePasswordPath = "/api/v1/profile/password"

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
		if errors.Is(err, users.ErrInvalidUsername) {
			writeError(w, http.StatusBadRequest, "Usernames can use letters, numbers, dots, dashes, and underscores (max 64 characters).")
			return
		}
		writeError(w, http.StatusBadRequest, "Could not create the user. The username may already be taken.")
		return
	}
	_ = h.audit.Record(r.Context(), audit.Event{
		Action: audit.ActionUserCreate,
		Actor:  h.actorUsername(r.Context()),
		Target: u.Username,
		Detail: "role=" + u.Role,
		Source: auditSourceWeb,
	})
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
		if errors.Is(err, users.ErrLastAdmin) {
			writeError(w, http.StatusConflict, "This is the last admin — promote another admin before changing this role.")
			return
		}
		writeError(w, http.StatusInternalServerError, "Something went wrong. Check your connection and try again.")
		return
	}
	_ = h.audit.Record(r.Context(), audit.Event{
		Action: audit.ActionUserRoleChange,
		Actor:  h.actorUsername(r.Context()),
		Target: h.targetUsername(r.Context(), id),
		Detail: "role=" + req.Role,
		Source: auditSourceWeb,
	})
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
	// Resolve the target name BEFORE the reset (the reset does not change it,
	// but doing so here keeps the lookup adjacent to the audit call).
	target := h.targetUsername(r.Context(), id)
	otp, err := users.ResetPassword(r.Context(), h.users, id)
	if err != nil {
		if errors.Is(err, users.ErrNotFound) {
			writeError(w, http.StatusNotFound, "That user no longer exists.")
			return
		}
		writeError(w, http.StatusInternalServerError, "Something went wrong. Check your connection and try again.")
		return
	}
	// SEC-05: record WHO reset WHOSE password. The one-time password is NEVER
	// recorded (T-00.04-02).
	_ = h.audit.Record(r.Context(), audit.Event{
		Action: audit.ActionUserResetPassword,
		Actor:  h.actorUsername(r.Context()),
		Target: target,
		Source: auditSourceWeb,
	})
	writeJSON(w, http.StatusOK, resetPasswordResponse{OneTimePassword: otp})
}

// handleDeactivate deactivates a target user (admin only).
func (h *authHandlers) handleDeactivate(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	target := h.targetUsername(r.Context(), id)
	if err := users.Deactivate(r.Context(), h.users, id); err != nil {
		if errors.Is(err, users.ErrNotFound) {
			writeError(w, http.StatusNotFound, "That user no longer exists.")
			return
		}
		if errors.Is(err, users.ErrLastAdmin) {
			writeError(w, http.StatusConflict, "This is the last admin — promote another admin before deactivating this account.")
			return
		}
		writeError(w, http.StatusInternalServerError, "Something went wrong. Check your connection and try again.")
		return
	}
	_ = h.audit.Record(r.Context(), audit.Event{
		Action: audit.ActionUserDeactivate,
		Actor:  h.actorUsername(r.Context()),
		Target: target,
		Source: auditSourceWeb,
	})
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
