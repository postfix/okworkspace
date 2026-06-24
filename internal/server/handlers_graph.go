package server

import (
	"log/slog"
	"net/http"
	"strconv"
)

// graphReadUnavailable is the generic copy returned for any server-side failure of
// the graph READ endpoints (/graph, /graph/local, /graph/backlinks). Like
// searchUnavailable/graphUnavailable it names NO Git/SQLite/Bleve/index internal
// (hidden-Git rule; details go to slog only). It deliberately avoids even the word
// "index" so the body carries zero infrastructure vocabulary.
const graphReadUnavailable = "The graph is unavailable. Try again in a moment."

// graphPathRequired is the generic 400 copy when a required page path query param
// is missing from /graph/local or /graph/backlinks.
const graphPathRequired = "A page is required."

// handleGraph serves GET /api/v1/graph for any authenticated user: the whole lean
// bipartite typed-edge graph payload (page + tag nodes, link + tag edges, no page
// bodies). A nil graph dependency or a query error returns the generic copy; the
// detail is logged server-side only.
func (h *authHandlers) handleGraph(w http.ResponseWriter, r *http.Request) {
	if h.graph == nil {
		writeError(w, http.StatusInternalServerError, graphReadUnavailable)
		return
	}
	data, err := h.graph.GraphData(r.Context())
	if err != nil {
		slog.Error("graph data failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, graphReadUnavailable)
		return
	}
	writeJSON(w, http.StatusOK, data)
}

// handleGraphLocal serves GET /api/v1/graph/local?path=<page>&depth=<n> for any
// authenticated user: the neighborhood of the named page (depth default 1, clamped
// to [1,3] in the Store). path/depth are QUERY params (not path segments), so no
// cleanPathParam is needed. A missing path returns 400 with a generic copy.
func (h *authHandlers) handleGraphLocal(w http.ResponseWriter, r *http.Request) {
	if h.graph == nil {
		writeError(w, http.StatusInternalServerError, graphReadUnavailable)
		return
	}
	pagePath := r.URL.Query().Get("path")
	if pagePath == "" {
		writeError(w, http.StatusBadRequest, graphPathRequired)
		return
	}
	depth := 1
	if d, err := strconv.Atoi(r.URL.Query().Get("depth")); err == nil {
		depth = d // Store clamps to [1,3]; an invalid/missing value keeps the default 1.
	}
	data, err := h.graph.Neighborhood(r.Context(), pagePath, depth)
	if err != nil {
		slog.Error("graph neighborhood failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, graphReadUnavailable)
		return
	}
	writeJSON(w, http.StatusOK, data)
}

// handleGraphBacklinks serves GET /api/v1/graph/backlinks?path=<page> for any
// authenticated user: the list of pages linking TO the page (reverse query) with
// resolved titles. A missing path returns 400. The body is always a JSON array
// (non-nil slice => [] for none).
func (h *authHandlers) handleGraphBacklinks(w http.ResponseWriter, r *http.Request) {
	if h.graph == nil {
		writeError(w, http.StatusInternalServerError, graphReadUnavailable)
		return
	}
	pagePath := r.URL.Query().Get("path")
	if pagePath == "" {
		writeError(w, http.StatusBadRequest, graphPathRequired)
		return
	}
	entries, err := h.graph.Backlinks(r.Context(), pagePath)
	if err != nil {
		slog.Error("graph backlinks failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, graphReadUnavailable)
		return
	}
	writeJSON(w, http.StatusOK, entries)
}
