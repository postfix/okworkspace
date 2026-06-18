package server

import (
	"errors"
	"net/http"
	"strconv"

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
}

// restoreResponse returns the (possibly auto-suffixed) path the page was restored
// to, so the SPA can navigate to it and surface the collision-suffix notice.
type restoreResponse struct {
	Path string `json:"path"`
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
			ID:           e.ID,
			Title:        e.Title,
			OriginalPath: e.OriginalPath,
			DeletedBy:    e.DeletedBy,
			DeletedAt:    e.DeletedAt,
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
