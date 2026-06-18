package server

import "net/http"

// handleTree returns the nested navigation tree (SPEC §17.2). Available to any
// authenticated user (readers included) — reading the tree is not gated by the
// editor role.
func (h *authHandlers) handleTree(w http.ResponseWriter, r *http.Request) {
	if h.pages == nil {
		writeError(w, http.StatusInternalServerError, "Something went wrong. Check your connection and try again.")
		return
	}
	nodes, err := h.pages.Tree(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Couldn't load your pages — try again.")
		return
	}
	writeJSON(w, http.StatusOK, nodes)
}
