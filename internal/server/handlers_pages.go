package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/postfix/okworkspace/internal/audit"
	"github.com/postfix/okworkspace/internal/pages"
)

// pageResponse mirrors pages.Page for the GET /pages response.
type pageResponse struct {
	Frontmatter string `json:"frontmatter"`
	Body        string `json:"body"`
	Revision    string `json:"revision"`
}

// createPageRequest is the POST /pages body (title-only create, D-12). Folder is
// the selected destination folder (empty = repo root).
type createPageRequest struct {
	Folder string `json:"folder"`
	Title  string `json:"title"`
}

// createPageResponse returns the created page's path so the SPA can navigate to
// it; the slugged filename is internal, surfaced only as a route token.
type createPageResponse struct {
	Path string `json:"path"`
}

// savePageRequest is the PUT /pages/{path} body. BaseRevision carries the
// optimistic-concurrency token the client read at open time (the 409 floor).
type savePageRequest struct {
	Body         string `json:"body"`
	Frontmatter  string `json:"frontmatter"`
	BaseRevision string `json:"base_revision"`
}

// createFolderRequest is the POST /folders body.
type createFolderRequest struct {
	Parent string `json:"parent"`
	Name   string `json:"name"`
}

// renamePageRequest is the POST /pages/{path}/rename body. Exactly ONE of the
// two fields must be non-empty: NewTitle dispatches to Rename (slug a new
// filename in the same folder), NewParent dispatches to Move (relocate to
// another folder). Both-set or neither-set is a 400 (the discriminant is
// exactly-one-of).
type renamePageRequest struct {
	NewTitle  string `json:"new_title"`
	NewParent string `json:"new_parent"`
}

// renamePageResponse returns the new repo-relative path so the SPA can navigate
// to the renamed/moved page.
type renamePageResponse struct {
	Path string `json:"path"`
}

// cleanPathParam reads the `*` wildcard, trims a leading slash, and rejects
// traversal/absolute/NUL inputs before the service re-resolves through the repo
// resolver (defense in depth — T-02-01). It writes a 400 and returns ok=false on
// a rejected path.
func cleanPathParam(w http.ResponseWriter, r *http.Request) (string, bool) {
	return cleanPathString(w, strings.TrimPrefix(chi.URLParam(r, "*"), "/"))
}

// cleanPathString validates an already-extracted page path string: it rejects
// empty / NUL / absolute / traversal inputs before the service re-resolves
// through the repo resolver (defense in depth — T-02-01). It writes a 400 and
// returns ok=false on a rejected path.
func cleanPathString(w http.ResponseWriter, p string) (string, bool) {
	p = strings.TrimPrefix(p, "/")
	if p == "" || strings.ContainsRune(p, 0x00) || strings.HasPrefix(p, "/") {
		writeError(w, http.StatusBadRequest, "Invalid request.")
		return "", false
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			writeError(w, http.StatusBadRequest, "Invalid request.")
			return "", false
		}
	}
	return p, true
}

// handleGetPageOrHistory dispatches the GET /pages/* catch-all: a wildcard ending
// in "/history" returns the version history (VER-02), one containing "/version/"
// returns an old version (VER-03 view), and anything else is a plain page read.
// chi cannot host a sibling `{path}/history` route next to the `/pages/*`
// wildcard (the sibling-wildcard conflict Plans 02-04 hit), so the suffixes are
// dispatched here on the SAME catch-all.
func (h *authHandlers) handleGetPageOrHistory(w http.ResponseWriter, r *http.Request) {
	wild := strings.TrimPrefix(chi.URLParam(r, "*"), "/")
	switch {
	case strings.HasSuffix(wild, "/history"):
		h.handleHistory(w, r)
		return
	case strings.Contains(wild, "/version/"):
		h.handleViewVersion(w, r)
		return
	default:
		h.handleGetPage(w, r)
	}
}

// handleGetPage returns a page's frontmatter, body, and revision (any
// authenticated user). 404 when the page does not exist.
func (h *authHandlers) handleGetPage(w http.ResponseWriter, r *http.Request) {
	if h.pages == nil {
		writeError(w, http.StatusInternalServerError, "Something went wrong. Check your connection and try again.")
		return
	}
	path, ok := cleanPathParam(w, r)
	if !ok {
		return
	}
	page, err := h.pages.Get(r.Context(), path)
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

// handleCreatePage creates a page from a title (editor only). Returns 201 with
// the new page's path.
func (h *authHandlers) handleCreatePage(w http.ResponseWriter, r *http.Request) {
	if h.pages == nil {
		writeError(w, http.StatusInternalServerError, "Something went wrong. Check your connection and try again.")
		return
	}
	var req createPageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request.")
		return
	}
	path, err := h.pages.Create(r.Context(), req.Folder, req.Title, h.actorUsername(r.Context()))
	if err != nil {
		if errors.Is(err, pages.ErrTitleRequired) {
			writeError(w, http.StatusBadRequest, "Give your page a title to create it.")
			return
		}
		writeError(w, http.StatusInternalServerError, "Something went wrong. Check your connection and try again.")
		return
	}
	_ = h.audit.Record(r.Context(), audit.Event{
		Action: audit.ActionPageCreate,
		Actor:  h.actorUsername(r.Context()),
		Target: path,
		Source: auditSourceWeb,
	})
	writeJSON(w, http.StatusCreated, createPageResponse{Path: path})
}

// handleSavePage writes a new version of a page (editor only). A stale
// base_revision returns 409 (the optimistic-concurrency floor) BEFORE any write.
func (h *authHandlers) handleSavePage(w http.ResponseWriter, r *http.Request) {
	if h.pages == nil {
		writeError(w, http.StatusInternalServerError, "Something went wrong. Check your connection and try again.")
		return
	}
	path, ok := cleanPathParam(w, r)
	if !ok {
		return
	}
	var req savePageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request.")
		return
	}
	err := h.pages.Save(r.Context(), path, req.Body, req.Frontmatter, req.BaseRevision, h.actorUsername(r.Context()))
	if err != nil {
		if errors.Is(err, pages.ErrPageNotFound) {
			writeError(w, http.StatusNotFound, "This page no longer exists. It may have been moved or deleted.")
			return
		}
		if errors.Is(err, pages.ErrStaleRevision) {
			writeError(w, http.StatusConflict, "This page was changed somewhere else since you opened it. Reload to see the latest version before saving again.")
			return
		}
		writeError(w, http.StatusInternalServerError, "We couldn't save your page just now. Your changes are kept here — check your connection and try Save again.")
		return
	}
	_ = h.audit.Record(r.Context(), audit.Event{
		Action: audit.ActionPageEdit,
		Actor:  h.actorUsername(r.Context()),
		Target: path,
		Source: auditSourceWeb,
	})
	w.WriteHeader(http.StatusNoContent)
}

// handleRenamePage dispatches a rename OR a move on the single /rename endpoint
// (editor only). The request body discriminates the operation: a non-empty
// new_title calls s.Rename (and audits Action "rename"); a non-empty new_parent
// calls s.Move (and audits Action "move"). Exactly one of the two must be
// present — both or neither returns 400 and performs no mutation. Inbound links
// are rewritten and committed atomically with the move by the service (D-07).
func (h *authHandlers) handleRenamePage(w http.ResponseWriter, r *http.Request) {
	if h.pages == nil {
		writeError(w, http.StatusInternalServerError, "Something went wrong. Check your connection and try again.")
		return
	}
	// The route is POST /pages/*; the wildcard is "{path}/rename" or
	// "{path}/restore" (the sibling-wildcard conflict forbids separate routes, so
	// both POST sub-actions are dispatched here on the same catch-all).
	wild := strings.TrimPrefix(chi.URLParam(r, "*"), "/")
	if strings.HasSuffix(wild, "/restore") {
		path, ok := cleanPathString(w, strings.TrimSuffix(wild, "/restore"))
		if !ok {
			return
		}
		var rreq struct {
			Version string `json:"version"`
		}
		if err := json.NewDecoder(r.Body).Decode(&rreq); err != nil {
			writeError(w, http.StatusBadRequest, "Invalid request.")
			return
		}
		h.handleRestoreVersion(w, r, path, rreq.Version)
		return
	}
	if !strings.HasSuffix(wild, "/rename") {
		writeError(w, http.StatusNotFound, "This page no longer exists. It may have been moved or deleted.")
		return
	}
	path, ok := cleanPathString(w, strings.TrimSuffix(wild, "/rename"))
	if !ok {
		return
	}
	var req renamePageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request.")
		return
	}
	title := strings.TrimSpace(req.NewTitle)
	parent := strings.TrimSpace(req.NewParent)

	// Exactly-one-of discriminant: both set or neither set is rejected before any
	// mutation.
	hasTitle := title != ""
	hasParent := parent != ""
	if hasTitle == hasParent {
		writeError(w, http.StatusBadRequest, "Choose a new title or a new folder, not both.")
		return
	}

	user := h.actorUsername(r.Context())
	var (
		newPath string
		err     error
		action  string
	)
	if hasTitle {
		action = audit.ActionPageRename
		newPath, err = h.pages.Rename(r.Context(), path, title, user)
	} else {
		action = audit.ActionPageMove
		newPath, err = h.pages.Move(r.Context(), path, parent, user)
	}
	if err != nil {
		if errors.Is(err, pages.ErrPageNotFound) {
			writeError(w, http.StatusNotFound, "This page no longer exists. It may have been moved or deleted.")
			return
		}
		if errors.Is(err, pages.ErrTitleRequired) {
			writeError(w, http.StatusBadRequest, "Give your page a title.")
			return
		}
		writeError(w, http.StatusInternalServerError, "Something went wrong. Check your connection and try again.")
		return
	}
	_ = h.audit.Record(r.Context(), audit.Event{
		Action: action,
		Actor:  user,
		Target: newPath,
		Source: auditSourceWeb,
	})
	writeJSON(w, http.StatusOK, renamePageResponse{Path: newPath})
}

// handleCreateFolder creates a folder (seeded with a blank index.md, editor
// only).
func (h *authHandlers) handleCreateFolder(w http.ResponseWriter, r *http.Request) {
	if h.pages == nil {
		writeError(w, http.StatusInternalServerError, "Something went wrong. Check your connection and try again.")
		return
	}
	var req createFolderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request.")
		return
	}
	if err := h.pages.CreateFolder(r.Context(), req.Parent, req.Name, h.actorUsername(r.Context())); err != nil {
		if errors.Is(err, pages.ErrTitleRequired) {
			writeError(w, http.StatusBadRequest, "Give your folder a name to create it.")
			return
		}
		writeError(w, http.StatusInternalServerError, "Something went wrong. Check your connection and try again.")
		return
	}
	_ = h.audit.Record(r.Context(), audit.Event{
		Action: audit.ActionFolderCreate,
		Actor:  h.actorUsername(r.Context()),
		Target: req.Parent + "/" + req.Name,
		Source: auditSourceWeb,
	})
	w.WriteHeader(http.StatusCreated)
}
