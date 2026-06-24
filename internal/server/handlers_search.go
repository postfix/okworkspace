package server

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/postfix/okworkspace/internal/audit"
	"github.com/postfix/okworkspace/internal/auth"
	"github.com/postfix/okworkspace/internal/graph"
	"github.com/postfix/okworkspace/internal/search"
)

// searchUnavailable is the single generic copy returned for any server-side
// search failure. It NEVER names the index, Bleve, Git, HEAD, or a commit — the
// hidden-Git rule + V7 error handling (details go to slog only).
const searchUnavailable = "Search is unavailable. Try again in a moment."

// graphUnavailable is the generic copy returned for any server-side link/graph
// rebuild failure. Like searchUnavailable it names NO Git/Bleve/index internal —
// it references the link affordance only (hidden-Git rule; details go to slog).
const graphUnavailable = "The link index is unavailable. Try again in a moment."

// searchEnqueuer is the worker subset the reindex handler needs: enqueue a
// KindIndex rebuild job fire-and-forget. Defined as an interface so the handler
// does not depend on the concrete jobs.Worker.
type searchEnqueuer interface {
	Enqueue(ctx context.Context, kind, payload string) error
}

// graphEnqueuer is the worker subset the graph rebuild handler needs: enqueue a
// KindGraph rebuild job fire-and-forget. A separate named interface (identical in
// shape to searchEnqueuer) so the graph handler does not couple to the search
// enqueuer field.
type graphEnqueuer interface {
	Enqueue(ctx context.Context, kind, payload string) error
}

// handleSearch serves GET /api/v1/search?q=... for any authenticated user. An
// empty q returns 200 [] with no index call (fast path). On success it returns the
// typed page results as a JSON array; on any error it returns the generic copy and
// logs the detail server-side only.
func (h *authHandlers) handleSearch(w http.ResponseWriter, r *http.Request) {
	if h.search == nil {
		writeError(w, http.StatusInternalServerError, searchUnavailable)
		return
	}
	q := r.URL.Query().Get("q")
	results, err := h.search.Query(r.Context(), q)
	if err != nil {
		slog.Error("search query failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, searchUnavailable)
		return
	}
	writeJSON(w, http.StatusOK, results)
}

// handleReindex serves POST /api/v1/admin/search/reindex (admin subgroup, already
// behind RequireRole(admin) + nosurf CSRF). It enqueues a rebuild-from-files job
// fire-and-forget and returns 202 Accepted. The admin action is audited (SEC-05)
// with the hidden-Git label — never "reindex Bleve".
func (h *authHandlers) handleReindex(w http.ResponseWriter, r *http.Request) {
	if h.search == nil || h.searchJobs == nil {
		writeError(w, http.StatusInternalServerError, searchUnavailable)
		return
	}
	if err := h.searchJobs.Enqueue(r.Context(), search.KindIndex, search.RebuildPayload()); err != nil {
		slog.Error("search reindex enqueue failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, searchUnavailable)
		return
	}
	actor := "unknown"
	if u, ok := auth.CurrentUser(r.Context()); ok {
		actor = strconv.FormatInt(u.UserID(), 10)
	}
	_ = h.audit.Record(r.Context(), audit.Event{
		Action: audit.ActionSearchReindex,
		Actor:  actor,
		Detail: "rebuild search index",
		Source: "web-ui",
	})
	writeJSON(w, http.StatusAccepted, map[string]bool{"ok": true})
}

// handleGraphReindex serves POST /api/v1/admin/graph/reindex (admin subgroup,
// already behind RequireRole(admin) + nosurf CSRF). It enqueues a from-files
// rebuild of the derived link/tag graph fire-and-forget and returns 202 Accepted
// — the request never blocks on the rebuild. The admin action is audited (SEC-05)
// with the hidden-Git label — never "reindex Bleve"/"git". A structural clone of
// handleReindex.
func (h *authHandlers) handleGraphReindex(w http.ResponseWriter, r *http.Request) {
	if h.graphJobs == nil {
		writeError(w, http.StatusInternalServerError, graphUnavailable)
		return
	}
	if err := h.graphJobs.Enqueue(r.Context(), graph.KindGraph, graph.RebuildPayload()); err != nil {
		slog.Error("graph reindex enqueue failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, graphUnavailable)
		return
	}
	actor := "unknown"
	if u, ok := auth.CurrentUser(r.Context()); ok {
		actor = strconv.FormatInt(u.UserID(), 10)
	}
	_ = h.audit.Record(r.Context(), audit.Event{
		Action: audit.ActionGraphReindex,
		Actor:  actor,
		Detail: "rebuild graph index",
		Source: "web-ui",
	})
	writeJSON(w, http.StatusAccepted, map[string]bool{"ok": true})
}
