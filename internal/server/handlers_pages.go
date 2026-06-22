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

// createFolderResponse returns the created folder's repo-relative path, for
// parity with createPageResponse (so the client receives a non-empty JSON body
// on success instead of a bare 201).
type createFolderResponse struct {
	Path string `json:"path"`
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

// handleGetPageOrHistory dispatches the GET /pages/* catch-all. The dispatch is
// anchored on the ".md" page-file boundary so that real pages which happen to
// live in a folder named "version" or "history" (e.g. docs/version/notes.md) are
// NOT mis-routed to the sub-resource handlers and rendered permanently
// unreadable (CR-01). Every page file ends in ".md"; the sub-resources are
// "<page>.md/history" and "<page>.md/version/<token>", so:
//   - history iff trimming the "/history" suffix leaves a path ending in ".md"
//   - version iff the wildcard contains the literal segment ".md/version/"
//   - otherwise it is a plain page read.
//
// chi cannot host a sibling `{path}/history` route next to the `/pages/*`
// wildcard (the sibling-wildcard conflict Plans 02-04 hit), so the suffixes are
// dispatched here on the SAME catch-all.
func (h *authHandlers) handleGetPageOrHistory(w http.ResponseWriter, r *http.Request) {
	wild := strings.TrimPrefix(chi.URLParam(r, "*"), "/")
	switch {
	case strings.HasSuffix(wild, ".md/presence"):
		// Per-page editing-presence SSE stream (COLL-01). Anchored on ".md" like
		// the other sub-resources so a real page in a folder named "presence" is
		// never mis-routed. Rides this any-authed catch-all (read-only, no editor
		// gate, no CSRF — same authority as reading the page).
		path, ok := cleanPathString(w, strings.TrimSuffix(wild, "/presence"))
		if !ok {
			return
		}
		h.handlePresence(w, r, path)
		return
	case strings.HasSuffix(wild, "/history") && strings.HasSuffix(strings.TrimSuffix(wild, "/history"), ".md"):
		h.handleHistory(w, r)
		return
	case strings.Contains(wild, ".md/version/"):
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
	// Soft-lock sub-actions (COLL-02) share this same /pages/* POST catch-all by
	// suffix (the sibling-wildcard conflict forbids separate routes). The longer
	// suffixes (".md/lock/force", ".md/lock/release") MUST be checked BEFORE the
	// bare ".md/lock" so the bare branch does not swallow them. Each is anchored on
	// the ".md" page-file boundary so a real page in a folder named "lock" is not
	// mis-routed. Identity is filled from the session inside the handlers; only the
	// opaque connection id comes from the body.
	if strings.HasSuffix(wild, ".md/lock/force") {
		path, ok := cleanPathString(w, strings.TrimSuffix(wild, "/lock/force"))
		if !ok {
			return
		}
		h.handleForceLock(w, r, path)
		return
	}
	if strings.HasSuffix(wild, ".md/lock/release") {
		path, ok := cleanPathString(w, strings.TrimSuffix(wild, "/lock/release"))
		if !ok {
			return
		}
		h.handleReleaseLock(w, r, path)
		return
	}
	if strings.HasSuffix(wild, ".md/lock") {
		path, ok := cleanPathString(w, strings.TrimSuffix(wild, "/lock"))
		if !ok {
			return
		}
		h.handleAcquireLock(w, r, path)
		return
	}
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
	// Folder rename/move share the SAME /pages/* catch-all by suffix (a folder is a
	// dir/index.md page, so a sibling {path:.*} route would 405 against the wildcard —
	// the sibling-wildcard conflict Plans 02-04 hit). These branches MUST precede the
	// plain /rename branch so a "/rename-folder" wildcard is not swallowed by it.
	if strings.HasSuffix(wild, "/rename-folder") {
		h.handleRenameFolder(w, r, strings.TrimSuffix(wild, "/rename-folder"))
		return
	}
	if strings.HasSuffix(wild, "/move-folder") {
		h.handleMoveFolder(w, r, strings.TrimSuffix(wild, "/move-folder"))
		return
	}
	if strings.HasSuffix(wild, "/delete-folder") {
		h.handleDeleteFolder(w, r, strings.TrimSuffix(wild, "/delete-folder"))
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
		// Validate the attacker-controlled destination parent the same way page
		// paths are validated (reject absolute / NUL / ".." segments) BEFORE it is
		// used to build the move target, so a traversal-shaped new_parent fails with
		// a clean 400 rather than relying solely on the resolver to 500 (WR-08).
		cleanParent, okParent := cleanPathString(w, parent)
		if !okParent {
			return
		}
		action = audit.ActionPageMove
		newPath, err = h.pages.Move(r.Context(), path, cleanParent, user)
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

// renameFolderRequest is the POST /pages/{dir}/rename-folder body.
type renameFolderRequest struct {
	NewName string `json:"new_name"`
}

// moveFolderRequest is the POST /pages/{dir}/move-folder body. NewParent is the
// destination folder ("" = root) and is attacker-controlled, so it is re-validated
// via cleanPathString before the service builds the move target (WR-08).
type moveFolderRequest struct {
	NewParent string `json:"new_parent"`
}

// handleRenameFolder renames a folder (its index.md + every descendant page) in ONE
// commit, rewriting all inbound links (TREE-02). A target-dir collision returns 409
// with the UI-SPEC copy and touches no disk (TREE-06). Editor-gated via the router
// subgroup (auth.RequireRole from the session role — never client input).
func (h *authHandlers) handleRenameFolder(w http.ResponseWriter, r *http.Request, rawDir string) {
	dir, ok := cleanPathString(w, rawDir)
	if !ok {
		return
	}
	var req renameFolderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request.")
		return
	}
	user := h.actorUsername(r.Context())
	newDir, err := h.pages.RenameFolder(r.Context(), dir, req.NewName, user)
	if err != nil {
		h.writeFolderError(w, err)
		return
	}
	_ = h.audit.Record(r.Context(), audit.Event{
		Action: audit.ActionFolderRename,
		Actor:  user,
		Target: newDir,
		Source: auditSourceWeb,
	})
	writeJSON(w, http.StatusOK, renamePageResponse{Path: newDir})
}

// handleMoveFolder moves a folder subtree into a new parent in ONE commit, with the
// same atomicity, collision (409), and RBAC guarantees as handleRenameFolder. The
// attacker-controlled new_parent is re-validated via cleanPathString (WR-08).
func (h *authHandlers) handleMoveFolder(w http.ResponseWriter, r *http.Request, rawDir string) {
	dir, ok := cleanPathString(w, rawDir)
	if !ok {
		return
	}
	var req moveFolderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request.")
		return
	}
	// new_parent "" means root (a valid destination); only validate when non-empty so
	// a root move is not rejected by cleanPathString's empty-path guard.
	newParent := strings.TrimSpace(req.NewParent)
	if newParent != "" {
		cleaned, okParent := cleanPathString(w, newParent)
		if !okParent {
			return
		}
		newParent = cleaned
	}
	user := h.actorUsername(r.Context())
	newDir, err := h.pages.MoveFolder(r.Context(), dir, newParent, user)
	if err != nil {
		h.writeFolderError(w, err)
		return
	}
	_ = h.audit.Record(r.Context(), audit.Event{
		Action: audit.ActionFolderMove,
		Actor:  user,
		Target: newDir,
		Source: auditSourceWeb,
	})
	writeJSON(w, http.StatusOK, renamePageResponse{Path: newDir})
}

// writeFolderError maps a folder rename/move service error to its HTTP status:
// ErrFolderExists -> 409 (TREE-06, with the exact UI-SPEC collision copy),
// ErrPageNotFound -> 404, ErrTitleRequired -> 400, anything else -> 500.
func (h *authHandlers) writeFolderError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, pages.ErrFolderExists):
		writeError(w, http.StatusConflict, "A folder with that name already exists there. Pick a different name or destination.")
	case errors.Is(err, pages.ErrPageNotFound):
		writeError(w, http.StatusNotFound, "This folder no longer exists. It may have been moved or deleted.")
	case errors.Is(err, pages.ErrTitleRequired):
		writeError(w, http.StatusBadRequest, "Give the folder a name.")
	default:
		writeError(w, http.StatusInternalServerError, "Something went wrong. Check your connection and try again.")
	}
}

// handleDeleteFolder moves a whole folder (its index.md + every descendant) to
// trash under ONE shared delete_group_id so it can be restored as a unit (TREE-04).
// It is dispatched on the existing /pages/* POST catch-all by the "/delete-folder"
// suffix (a sibling {path:.*} route would 405 against the wildcard). The
// attacker-controlled dir is re-validated via cleanPathString (SEC-01/WR-08) before
// the service walks it. Editor-gated via the router subgroup (RBAC from the session
// role, never client input — T-07-06). 404 when the folder has no pages; 204 on
// success.
func (h *authHandlers) handleDeleteFolder(w http.ResponseWriter, r *http.Request, rawDir string) {
	if h.pages == nil {
		writeError(w, http.StatusInternalServerError, "Something went wrong. Check your connection and try again.")
		return
	}
	dir, ok := cleanPathString(w, rawDir)
	if !ok {
		return
	}
	user := h.actorUsername(r.Context())
	if err := h.pages.DeleteFolder(r.Context(), dir, user); err != nil {
		if errors.Is(err, pages.ErrPageNotFound) {
			writeError(w, http.StatusNotFound, "This folder no longer exists. It may have been moved or deleted.")
			return
		}
		writeError(w, http.StatusInternalServerError, "Something went wrong. Check your connection and try again.")
		return
	}
	_ = h.audit.Record(r.Context(), audit.Event{
		Action: audit.ActionFolderTrash,
		Actor:  user,
		Target: dir,
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
	// Trim so a root-level folder (empty Parent) does not produce a
	// leading-slash target like "/myfolder" (IN-01).
	folderPath := strings.Trim(req.Parent+"/"+req.Name, "/")
	_ = h.audit.Record(r.Context(), audit.Event{
		Action: audit.ActionFolderCreate,
		Actor:  h.actorUsername(r.Context()),
		Target: folderPath,
		Source: auditSourceWeb,
	})
	writeJSON(w, http.StatusCreated, createFolderResponse{Path: folderPath})
}
