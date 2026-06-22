package server

import (
	"encoding/json"
	"net/http"

	"github.com/postfix/okworkspace/internal/auth"
	"github.com/postfix/okworkspace/internal/locks"
)

// lockRequest is the body of the acquire/force/release lock POSTs. The ONLY
// client-supplied field is the opaque connection id (conn) — the lock's
// username/user_id are derived from the SESSION server-side, never the body
// (T-05-06). The connection id is treated as opaque: it is used solely for
// self/dedup matching as the lock SessionID, never as a path component (the
// page path is the wildcard, re-validated by the lock store's subtree guard).
type lockRequest struct {
	Conn string `json:"conn"`
}

// lockHolder is the safe projection of a lock holder surfaced to the client on a
// held-by-other acquire: ONLY the holder's username (for the SoftLockBanner copy
// "{name} is editing this page."), never their session id.
type lockHolder struct {
	Username string `json:"username"`
}

// acquireLockResponse is the JSON returned by handleAcquireLock. result is
// "acquired" (you now hold/refreshed the lock) or "held-by-other" (a different
// live session holds it). holder is present ONLY on held-by-other and carries
// only the holder's username.
type acquireLockResponse struct {
	Result string      `json:"result"`
	Holder *lockHolder `json:"holder,omitempty"`
}

// forceLockResponse is the JSON returned by handleForceLock. Force always takes
// the lock, so the result is constant "acquired".
type forceLockResponse struct {
	Result string `json:"result"`
}

// lockOwner builds the server-trusted Owner for a lock mutation. Username and
// UserID come from the SESSION-bound user (auth.CurrentUser → actorUsername /
// UserID) — NEVER from the request body. The opaque client connection id is the
// only client-supplied field and becomes the lock SessionID (self/dedup only).
// It returns ok=false (after writing a 401) when there is no session-bound user.
func (h *authHandlers) lockOwner(w http.ResponseWriter, r *http.Request, conn string) (locks.Owner, bool) {
	cur, okUser := auth.CurrentUser(r.Context())
	if !okUser {
		writeError(w, http.StatusUnauthorized, "Your session expired. Sign in again to continue.")
		return locks.Owner{}, false
	}
	return locks.Owner{
		Username:  h.actorUsername(r.Context()),
		UserID:    cur.UserID(),
		SessionID: conn,
	}, true
}

// decodeConn reads the opaque connection id from the JSON body. The connection
// id is required (it is the lock SessionID that distinguishes two tabs / live
// sessions). A missing/blank conn is a 400 — without it the lock cannot be
// attributed to a session and a release could clobber another holder.
func decodeConn(w http.ResponseWriter, r *http.Request) (string, bool) {
	var req lockRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request.")
		return "", false
	}
	if req.Conn == "" {
		writeError(w, http.StatusBadRequest, "Invalid request.")
		return "", false
	}
	return req.Conn, true
}

// handleAcquireLock acquires (or, for the same session, refreshes) the soft lock
// on a page (editor-gated, COLL-02). It is dispatched off the POST /pages/*
// catch-all by the ".md/lock" suffix; path is already stripped of the suffix by
// the dispatcher. The lock identity comes from the SESSION; only the opaque
// connection id arrives in the body. On a held-by-other result the holder's
// username (and nothing else) is surfaced so the SoftLockBanner can name them.
func (h *authHandlers) handleAcquireLock(w http.ResponseWriter, r *http.Request, path string) {
	if h.locks == nil {
		writeError(w, http.StatusInternalServerError, "Something went wrong. Check your connection and try again.")
		return
	}
	conn, ok := decodeConn(w, r)
	if !ok {
		return
	}
	owner, ok := h.lockOwner(w, r, conn)
	if !ok {
		return
	}
	cur, result, err := h.locks.Acquire(r.Context(), path, owner)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Something went wrong. Check your connection and try again.")
		return
	}
	resp := acquireLockResponse{Result: string(result)}
	if result == locks.ResultHeldByOther {
		resp.Holder = &lockHolder{Username: cur.Username}
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleForceLock unconditionally takes over the soft lock for the caller's
// session (editor-gated, COLL-02). It is LOCK-ONLY: it calls locks.Force and
// never reads, writes, or touches the page body or any revision — force-edit
// (who may type) is decoupled from save authority (is the write safe), which
// stays untouched in pages.Save (T-05-09). Dispatched off the POST /pages/*
// catch-all by the ".md/lock/force" suffix.
func (h *authHandlers) handleForceLock(w http.ResponseWriter, r *http.Request, path string) {
	if h.locks == nil {
		writeError(w, http.StatusInternalServerError, "Something went wrong. Check your connection and try again.")
		return
	}
	conn, ok := decodeConn(w, r)
	if !ok {
		return
	}
	owner, ok := h.lockOwner(w, r, conn)
	if !ok {
		return
	}
	if _, err := h.locks.Force(r.Context(), path, owner); err != nil {
		writeError(w, http.StatusInternalServerError, "Something went wrong. Check your connection and try again.")
		return
	}
	writeJSON(w, http.StatusOK, forceLockResponse{Result: string(locks.ResultAcquired)})
}

// handleReleaseLock releases the soft lock when the on-disk holder matches the
// caller's connection id (editor-gated, COLL-02). It is idempotent: a missing or
// foreign-held lock is a no-op (the store's Release only deletes a lock whose
// SessionID matches), so a session that lost the lock to a TTL takeover cannot
// clobber the new holder. 204 on success. Dispatched off the POST /pages/*
// catch-all by the ".md/lock/release" suffix.
func (h *authHandlers) handleReleaseLock(w http.ResponseWriter, r *http.Request, path string) {
	if h.locks == nil {
		writeError(w, http.StatusInternalServerError, "Something went wrong. Check your connection and try again.")
		return
	}
	conn, ok := decodeConn(w, r)
	if !ok {
		return
	}
	if err := h.locks.Release(r.Context(), path, conn); err != nil {
		writeError(w, http.StatusInternalServerError, "Something went wrong. Check your connection and try again.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
