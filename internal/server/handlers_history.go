package server

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/postfix/okworkspace/internal/audit"
	"github.com/postfix/okworkspace/internal/pages"
)

// historyEntryResponse mirrors pages.HistoryEntry for the GET /history response.
// It carries ONLY human-readable fields plus an OPAQUE version token — there is
// no SHA/hash/commit field, so no Git internals reach the UI (VER-02, T-05-02).
type historyEntryResponse struct {
	Version string `json:"version"`
	Action  string `json:"action"`
	Who     string `json:"who"`
	When    string `json:"when"`
}

// restoreVersionResponse returns the path the page was restored to (its own path;
// restore is a forward commit, never a relocation).
type restoreVersionResponse struct {
	Path string `json:"path"`
}

// handleHistory returns a page's version history (any authenticated user). The
// payload carries action/who/when + an opaque version token — never a SHA. The
// route is GET /pages/{path}/history; the wildcard is "{path}/history" and the
// "/history" suffix is stripped to recover the page path.
func (h *authHandlers) handleHistory(w http.ResponseWriter, r *http.Request) {
	if h.pages == nil {
		writeError(w, http.StatusInternalServerError, "Something went wrong. Check your connection and try again.")
		return
	}
	wild := strings.TrimPrefix(chi.URLParam(r, "*"), "/")
	path, ok := cleanPathString(w, strings.TrimSuffix(wild, "/history"))
	if !ok {
		return
	}
	entries, err := h.pages.History(r.Context(), path)
	if err != nil {
		if errors.Is(err, pages.ErrPageNotFound) {
			writeError(w, http.StatusNotFound, "This page no longer exists. It may have been moved or deleted.")
			return
		}
		writeError(w, http.StatusInternalServerError, "Something went wrong. Check your connection and try again.")
		return
	}
	out := make([]historyEntryResponse, 0, len(entries))
	for _, e := range entries {
		out = append(out, historyEntryResponse{
			Version: e.Version,
			Action:  e.Action,
			Who:     e.Who,
			When:    e.When,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleViewVersion returns a page as it existed at an opaque version token (any
// authenticated user). The route is GET /pages/{path}/version/{version}; the
// wildcard is "{path}/version/{version}". The version token is the server-issued
// handle the client received from the history list — never a user-typed SHA.
func (h *authHandlers) handleViewVersion(w http.ResponseWriter, r *http.Request) {
	if h.pages == nil {
		writeError(w, http.StatusInternalServerError, "Something went wrong. Check your connection and try again.")
		return
	}
	wild := strings.TrimPrefix(chi.URLParam(r, "*"), "/")
	// Split on the ".md/version/" boundary (not bare "/version/") so the page
	// part keeps its trailing ".md" and a page that itself lives under a folder
	// named "version" is not mis-parsed (CR-01). The page file always ends in
	// ".md", so the marker includes it; the token is everything after.
	const marker = ".md/version/"
	idx := strings.Index(wild, marker)
	if idx < 0 {
		writeError(w, http.StatusNotFound, "This page no longer exists. It may have been moved or deleted.")
		return
	}
	pathPart := wild[:idx+len(".md")]
	version := wild[idx+len(marker):]
	if version == "" || strings.ContainsRune(version, '/') {
		writeError(w, http.StatusBadRequest, "Invalid request.")
		return
	}
	path, ok := cleanPathString(w, pathPart)
	if !ok {
		return
	}
	page, err := h.pages.ViewVersion(r.Context(), path, version)
	if err != nil {
		if errors.Is(err, pages.ErrPageNotFound) {
			writeError(w, http.StatusNotFound, "This page no longer exists. It may have been moved or deleted.")
			return
		}
		writeError(w, http.StatusInternalServerError, "Something went wrong. Check your connection and try again.")
		return
	}
	writeJSON(w, http.StatusOK, pageResponse(page))
}

// handleRestoreVersion restores a page to an old version as a NEW forward commit
// (editor only, VER-03). The route is POST /pages/{path}/restore with the version
// token in the body. The current version is kept in history — nothing is
// overwritten. The restore is audited (Action restore). 404 when the page does
// not exist.
func (h *authHandlers) handleRestoreVersion(w http.ResponseWriter, r *http.Request, path, version string) {
	if h.pages == nil {
		writeError(w, http.StatusInternalServerError, "Something went wrong. Check your connection and try again.")
		return
	}
	if strings.TrimSpace(version) == "" {
		writeError(w, http.StatusBadRequest, "Invalid request.")
		return
	}
	user := h.actorUsername(r.Context())
	if err := h.pages.RestoreVersion(r.Context(), path, version, user); err != nil {
		if errors.Is(err, pages.ErrPageNotFound) {
			writeError(w, http.StatusNotFound, "This page no longer exists. It may have been moved or deleted.")
			return
		}
		writeError(w, http.StatusInternalServerError, "Something went wrong. Check your connection and try again.")
		return
	}
	_ = h.audit.Record(r.Context(), audit.Event{
		Action: audit.ActionPageRestore,
		Actor:  user,
		Target: path,
		Source: auditSourceWeb,
	})
	writeJSON(w, http.StatusOK, restoreVersionResponse{Path: path})
}
