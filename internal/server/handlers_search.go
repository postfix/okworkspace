package server

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/postfix/okworkspace/internal/audit"
	"github.com/postfix/okworkspace/internal/auth"
	"github.com/postfix/okworkspace/internal/search"
)

// searchUnavailable is the single generic copy returned for any server-side
// search failure. It NEVER names the index, Bleve, Git, HEAD, or a commit — the
// hidden-Git rule + V7 error handling (details go to slog only).
const searchUnavailable = "Search is unavailable. Try again in a moment."

// searchEnqueuer is the worker subset the reindex handler needs: enqueue a
// KindIndex rebuild job fire-and-forget. Defined as an interface so the handler
// does not depend on the concrete jobs.Worker.
type searchEnqueuer interface {
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
