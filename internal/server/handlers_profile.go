package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/postfix/okworkspace/internal/audit"
	"github.com/postfix/okworkspace/internal/auth"
	"github.com/postfix/okworkspace/internal/users"
)

// profileUpdateRequest carries a self-service display-name change. It
// DELIBERATELY has no role field — a user can never change their own role
// (D-06, T-00.03-02). Any extra JSON keys a client sends (e.g. "role") are
// ignored by the decoder.
type profileUpdateRequest struct {
	DisplayName string `json:"display_name"`
}

// handleUpdateProfile updates the CURRENT user's display name only. The target
// id comes from the session-bound user, never the request body, so a user can
// only edit their own account.
func (h *authHandlers) handleUpdateProfile(w http.ResponseWriter, r *http.Request) {
	cur, ok := auth.CurrentUser(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "Your session expired. Sign in again to continue.")
		return
	}
	var req profileUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request.")
		return
	}
	if err := users.UpdateOwnProfile(r.Context(), h.users, cur.UserID(), req.DisplayName); err != nil {
		if errors.Is(err, users.ErrEmptyDisplayName) {
			writeError(w, http.StatusBadRequest, "Enter a display name.")
			return
		}
		writeError(w, http.StatusInternalServerError, "Something went wrong. Check your connection and try again.")
		return
	}
	_ = h.audit.Record(r.Context(), audit.Event{
		Action: audit.ActionProfileChange,
		Actor:  h.actorUsername(r.Context()),
		Detail: "display_name",
		Source: auditSourceWeb,
	})
	w.WriteHeader(http.StatusNoContent)
}

// passwordChangeRequest carries a self-service password change. No role field
// (D-06).
type passwordChangeRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// handleChangePassword changes the CURRENT user's password (verifies the old
// one, enforces >=12 chars, clears must_change_password). Operates only on the
// session-bound user's id.
func (h *authHandlers) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	cur, ok := auth.CurrentUser(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "Your session expired. Sign in again to continue.")
		return
	}
	var req passwordChangeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request.")
		return
	}
	err := users.ChangeOwnPassword(r.Context(), h.users, cur.UserID(), req.CurrentPassword, req.NewPassword)
	switch {
	case errors.Is(err, users.ErrWeakPassword):
		writeError(w, http.StatusBadRequest, "Choose a longer password — at least 12 characters.")
		return
	case errors.Is(err, users.ErrWrongPassword):
		writeError(w, http.StatusBadRequest, "Your current password is incorrect.")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "Something went wrong. Check your connection and try again.")
		return
	}
	// SEC-05: record the self-service password change. The new password is NEVER
	// recorded (T-00.04-02).
	_ = h.audit.Record(r.Context(), audit.Event{
		Action: audit.ActionProfileChange,
		Actor:  h.actorUsername(r.Context()),
		Detail: "password",
		Source: auditSourceWeb,
	})
	w.WriteHeader(http.StatusNoContent)
}
