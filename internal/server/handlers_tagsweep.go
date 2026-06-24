package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/postfix/okworkspace/internal/audit"
	"github.com/postfix/okworkspace/internal/auth"
	"github.com/postfix/okworkspace/internal/tagsweep"
)

// tagSweepUnavailable is the single generic copy returned for any server-side
// tag-sweep failure. Like searchUnavailable / graphUnavailable it names NO
// job/queue/worker/LLM/Git/SQLite internal — it references the user-facing tag
// affordance only (hidden-infra rule; details go to slog only, V7 error handling).
const tagSweepUnavailable = "Tag suggestions are unavailable. Try again in a moment."

// tagSweepEnqueuer is the worker subset the sweep-start handler needs: enqueue a
// KindTagSuggest job fire-and-forget. A separate named interface (identical in
// shape to graphEnqueuer) so the handler does not couple to the concrete
// jobs.Worker.
type tagSweepEnqueuer interface {
	Enqueue(ctx context.Context, kind, payload string) error
}

// startTagSweepRequest is the sweep-start body: all=false (default) targets only
// untagged pages (the backfill case); all=true targets every live page.
type startTagSweepRequest struct {
	All bool `json:"all"`
}

// handleStartTagSweep serves POST /api/v1/admin/tags/sweep (admin subgroup,
// already behind RequireRole(admin) + nosurf CSRF — RBAC is read from the SESSION
// role, never the request body). It enumerates the target pages SERVER-SIDE (live
// pages ∩ page_tags via the tagsweep store — the client cannot inject arbitrary
// paths, T-12-02), enqueues ONE KindTagSuggest job per target FIRE-AND-FORGET
// (worker.Enqueue, NEVER EnqueueAndWait — the CR-01 deadlock lesson), and returns
// 202 {ok,queued:N} immediately. The request WRITES NOTHING to disk; the jobs only
// STAGE pending suggestions (Pitfall 5). Zero targets → 202 queued=0 (drives the
// "every page already has tags" UX). The admin action is audited (SEC-05) with the
// hidden-infra label — scope + count only, never job/queue/LLM vocabulary.
func (h *authHandlers) handleStartTagSweep(w http.ResponseWriter, r *http.Request) {
	if h.tagSuggestions == nil || h.tagSweepJobs == nil {
		writeError(w, http.StatusInternalServerError, tagSweepUnavailable)
		return
	}

	var req startTagSweepRequest
	// An empty body is allowed (defaults to all=false); only malformed JSON is a 400.
	if r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "Invalid request.")
			return
		}
	}

	targets, err := h.tagSuggestions.Targets(r.Context(), req.All)
	if err != nil {
		slog.Error("tag sweep targets failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, tagSweepUnavailable)
		return
	}

	queued := 0
	for _, path := range targets {
		if err := h.tagSweepJobs.Enqueue(r.Context(), tagsweep.KindTagSuggest, tagsweep.SuggestPayload(path)); err != nil {
			// Best-effort: stop on the first enqueue error (rare — a DB failure). The
			// jobs already enqueued stand; the admin can re-run the sweep (re-staging
			// supersedes). The generic copy names no internal.
			slog.Error("tag sweep enqueue failed", slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, tagSweepUnavailable)
			return
		}
		queued++
	}

	scope := "untagged"
	if req.All {
		scope = "all"
	}
	actor := "unknown"
	if u, ok := auth.CurrentUser(r.Context()); ok {
		actor = strconv.FormatInt(u.UserID(), 10)
	}
	_ = h.audit.Record(r.Context(), audit.Event{
		Action: audit.ActionTagSweep,
		Actor:  actor,
		Detail: fmt.Sprintf("scope=%s count=%d", scope, queued),
		Source: "web-ui",
	})

	writeJSON(w, http.StatusAccepted, map[string]any{"ok": true, "queued": queued})
}

// handleListTagSuggestions serves GET /api/v1/admin/tags/suggestions (admin
// subgroup, already behind RequireRole(admin)). It lists the pages with a pending
// staged suggestion (the review queue read). Page paths + tags are returned as
// DATA (the SPA renders them as text children, never HTML). Fail closed 500 if the
// store is nil.
func (h *authHandlers) handleListTagSuggestions(w http.ResponseWriter, r *http.Request) {
	if h.tagSuggestions == nil {
		writeError(w, http.StatusInternalServerError, tagSweepUnavailable)
		return
	}
	pending, err := h.tagSuggestions.ListPending(r.Context())
	if err != nil {
		slog.Error("tag suggestions list failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, tagSweepUnavailable)
		return
	}
	writeJSON(w, http.StatusOK, pending)
}
