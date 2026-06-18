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

// cleanPathParam reads the {path} wildcard, trims a leading slash, and rejects
// traversal/absolute/NUL inputs before the service re-resolves through the repo
// resolver (defense in depth — T-02-01). It writes a 400 and returns ok=false on
// a rejected path.
func cleanPathParam(w http.ResponseWriter, r *http.Request) (string, bool) {
	p := strings.TrimPrefix(chi.URLParam(r, "*"), "/")
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
