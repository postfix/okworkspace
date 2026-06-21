package server

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/postfix/okworkspace/internal/audit"
	"github.com/postfix/okworkspace/internal/pages"
)

// trashEntryResponse mirrors pages.TrashEntry for the GET /trash response. The
// user sees the title, where it came from, who deleted it, and when — never any
// Git vocabulary.
type trashEntryResponse struct {
	ID           int64  `json:"id"`
	Title        string `json:"title"`
	OriginalPath string `json:"original_path"`
	DeletedBy    string `json:"deleted_by"`
	DeletedAt    string `json:"deleted_at"`
	// DeleteGroupID groups the rows produced by one folder-delete so the client can
	// render them as a single restorable unit (TREE-04/05). Empty for a solo delete.
	DeleteGroupID string `json:"delete_group_id"`
}

// restoreResponse returns the (possibly auto-suffixed) path the page was restored
// to, so the SPA can navigate to it and surface the collision-suffix notice.
type restoreResponse struct {
	Path string `json:"path"`
}

// restoreGroupResponse returns the (possibly auto-suffixed) paths a folder-delete
// was restored to, so the SPA can navigate to the folder and surface a per-page
// collision notice when any restored path was suffixed.
type restoreGroupResponse struct {
	Paths []string `json:"paths"`
}

// handleDeletePage moves a page to trash (editor only). The page is recoverable
// (a real commit, D-08), so this is not a destructive permanent delete. The
// delete is audited (Action "trash"). 404 when the page does not exist.
func (h *authHandlers) handleDeletePage(w http.ResponseWriter, r *http.Request) {
	if h.pages == nil {
		writeError(w, http.StatusInternalServerError, "Something went wrong. Check your connection and try again.")
		return
	}
	path, ok := cleanPathParam(w, r)
	if !ok {
		return
	}
	user := h.actorUsername(r.Context())
	if err := h.pages.Delete(r.Context(), path, user); err != nil {
		if errors.Is(err, pages.ErrPageNotFound) {
			writeError(w, http.StatusNotFound, "This page no longer exists. It may have been moved or deleted.")
			return
		}
		writeError(w, http.StatusInternalServerError, "Something went wrong. Check your connection and try again.")
		return
	}
	_ = h.audit.Record(r.Context(), audit.Event{
		Action: audit.ActionPageTrash,
		Actor:  user,
		Target: path,
		Source: auditSourceWeb,
	})
	w.WriteHeader(http.StatusNoContent)
}

// handleListTrash returns the list of deleted pages (any authenticated user). The
// response carries provenance (title, original path, who, when) — never page
// content or Git vocabulary.
func (h *authHandlers) handleListTrash(w http.ResponseWriter, r *http.Request) {
	if h.pages == nil {
		writeError(w, http.StatusInternalServerError, "Something went wrong. Check your connection and try again.")
		return
	}
	entries, err := h.pages.ListTrash(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Something went wrong. Check your connection and try again.")
		return
	}
	out := make([]trashEntryResponse, 0, len(entries))
	for _, e := range entries {
		out = append(out, trashEntryResponse{
			ID:            e.ID,
			Title:         e.Title,
			OriginalPath:  e.OriginalPath,
			DeletedBy:     e.DeletedBy,
			DeletedAt:     e.DeletedAt,
			DeleteGroupID: e.DeleteGroupID,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleRestoreFromTrash restores a trashed page to its original folder (editor
// only), auto-suffixing on a live-page collision so nothing is clobbered (D-10).
// Returns the (possibly suffixed) restored path. The restore is audited (Action
// "restore"). 404 when no trash entry matches the id.
func (h *authHandlers) handleRestoreFromTrash(w http.ResponseWriter, r *http.Request) {
	if h.pages == nil {
		writeError(w, http.StatusInternalServerError, "Something went wrong. Check your connection and try again.")
		return
	}
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "Invalid request.")
		return
	}
	user := h.actorUsername(r.Context())
	restored, err := h.pages.Restore(r.Context(), id, user)
	if err != nil {
		if errors.Is(err, pages.ErrTrashNotFound) {
			writeError(w, http.StatusNotFound, "This page is no longer in Trash. It may have already been restored.")
			return
		}
		writeError(w, http.StatusInternalServerError, "Something went wrong. Check your connection and try again.")
		return
	}
	_ = h.audit.Record(r.Context(), audit.Event{
		Action: audit.ActionPageRestore,
		Actor:  user,
		Target: restored,
		Source: auditSourceWeb,
	})
	writeJSON(w, http.StatusOK, restoreResponse{Path: restored})
}

// handleRestoreFolderGroup restores a whole folder-delete as a unit (TREE-05),
// index.md first, auto-suffixing per page on a live-page collision so nothing is
// clobbered. The group id is an OPAQUE string (not a path), validated non-empty here
// and BOUND parameterized in the service SELECT (T-07-05 SQLi guard); the route lives
// in the editor subgroup (RBAC from the session role, never client input — T-07-06).
// Returns the restored paths. 404 when no trash row matches the group id.
func (h *authHandlers) handleRestoreFolderGroup(w http.ResponseWriter, r *http.Request) {
	if h.pages == nil {
		writeError(w, http.StatusInternalServerError, "Something went wrong. Check your connection and try again.")
		return
	}
	groupID := chi.URLParam(r, "id")
	if groupID == "" || strings.ContainsRune(groupID, '\x00') {
		writeError(w, http.StatusBadRequest, "Invalid request.")
		return
	}
	user := h.actorUsername(r.Context())
	restored, err := h.pages.RestoreGroup(r.Context(), groupID, user)
	if err != nil {
		if errors.Is(err, pages.ErrTrashNotFound) {
			writeError(w, http.StatusNotFound, "This folder is no longer in Trash. It may have already been restored.")
			return
		}
		writeError(w, http.StatusInternalServerError, "Something went wrong. Check your connection and try again.")
		return
	}
	_ = h.audit.Record(r.Context(), audit.Event{
		Action: audit.ActionFolderRestore,
		Actor:  user,
		Target: groupID,
		Source: auditSourceWeb,
	})
	writeJSON(w, http.StatusOK, restoreGroupResponse{Paths: restored})
}
