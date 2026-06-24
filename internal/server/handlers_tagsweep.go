package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/postfix/okworkspace/internal/agent"
	"github.com/postfix/okworkspace/internal/audit"
	"github.com/postfix/okworkspace/internal/auth"
	"github.com/postfix/okworkspace/internal/pages"
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

// approveTagSuggestionsRequest is the POST /admin/tags/approve body: the human's
// per-page approval set (page_path + the CHECKED tags). The tags are the client's
// approval selection and are NEVER trusted verbatim — the server re-validates and
// normalizes each page's list before write (T-12-06). Any client-supplied
// base_revision is intentionally absent: the handler reads the STAGED base_revision
// from the queue (T-12-07), so a moved page 409s individually.
type approveTagSuggestionsRequest struct {
	Approvals []struct {
		PagePath string   `json:"page_path"`
		Tags     []string `json:"tags"`
	} `json:"approvals"`
}

// maxApproveBatch caps how many pages one approve request may carry. A review-queue
// batch is small (a human curated it); this bounds an asymmetric oversized request
// without rejecting any realistic approval.
const maxApproveBatch = 1000

// handleApproveTagSuggestions serves POST /api/v1/admin/tags/approve (admin
// subgroup, behind RequireRole(admin) + nosurf CSRF — RBAC from the SESSION role,
// never the body, T-12-08). It approves one OR many pages' staged suggestions and
// routes them through the SAME Phase-11 byte-stable apply, committed in a BATCH
// (one commit, not one-per-page — Pitfall 6). For each approval it:
//   - validates page_path, rejects NUL in tags, caps the list (maxApplyTags);
//   - RE-VALIDATES + normalizes the tags via agent.ValidateTags(nil vocab) — the
//     raw client list is NEVER written (T-12-06). A page that normalizes to empty
//     is SKIPPED (recorded), not a 400 for the whole batch;
//   - reads the STAGED base_revision from tagsweep.GetPending (T-12-07) — the
//     queue is the server's record of which revision was suggested against; a page
//     that is no longer pending is SKIPPED.
//
// It then calls pages.ApplyTagsBatch (ONE commit for the batch), resolves the rows
// that returned status=applied (leaving stale/notfound rows PENDING for a re-run),
// audits a single batch event with non-secret counts only, and returns 200 with the
// per-page results so the UI can decrement the backlog / show the stale state.
func (h *authHandlers) handleApproveTagSuggestions(w http.ResponseWriter, r *http.Request) {
	if h.tagSuggestions == nil || h.pages == nil {
		writeError(w, http.StatusInternalServerError, tagSweepUnavailable)
		return
	}

	var req approveTagSuggestionsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request.")
		return
	}
	if len(req.Approvals) == 0 {
		writeError(w, http.StatusBadRequest, "There are no approvals to apply.")
		return
	}
	if len(req.Approvals) > maxApproveBatch {
		writeError(w, http.StatusBadRequest, "Too many pages in one approval.")
		return
	}

	ctx := r.Context()
	var items []pages.TagApplyItem
	// skipped tracks pages the server dropped BEFORE the batch (no pending row, or
	// empty-after-normalize) — reported in the response as status="skipped" so the
	// UI can surface them without failing the whole batch.
	skipped := []pages.TagApplyResult{}

	for _, a := range req.Approvals {
		path := strings.TrimSpace(a.PagePath)
		if path == "" || !validIdentifier(path) {
			writeError(w, http.StatusBadRequest, "Invalid request.")
			return
		}
		if len(a.Tags) > maxApplyTags {
			writeError(w, http.StatusBadRequest, "Too many tags to apply.")
			return
		}
		for _, t := range a.Tags {
			if strings.ContainsRune(t, '\x00') {
				writeError(w, http.StatusBadRequest, "Invalid request.")
				return
			}
		}

		// RE-VALIDATE + normalize server-side (nil vocab — the existing/new flag is
		// irrelevant on the write). A list that normalizes to empty is dropped from
		// the batch (recorded skipped), NOT a 400 for the whole request.
		normalized, _, err := agent.ValidateTags(a.Tags, nil)
		if err != nil {
			skipped = append(skipped, pages.TagApplyResult{PagePath: path, Status: "skipped"})
			continue
		}

		// Read the STAGED base_revision from the queue — the server's record of which
		// revision the suggestion was made against (NEVER a client value). A page no
		// longer pending is skipped (already resolved or never suggested).
		entry, ok, err := h.tagSuggestions.GetPending(ctx, path)
		if err != nil {
			slog.Error("tag approve get pending failed", slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, tagSweepUnavailable)
			return
		}
		if !ok {
			skipped = append(skipped, pages.TagApplyResult{PagePath: path, Status: "skipped"})
			continue
		}

		items = append(items, pages.TagApplyItem{
			PagePath:     path,
			Tags:         normalized,
			BaseRevision: entry.BaseRevision,
		})
	}

	actor := h.actorUsername(ctx)
	results, err := h.pages.ApplyTagsBatch(ctx, items, actor)
	if err != nil {
		slog.Error("tag approve apply batch failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, tagSweepUnavailable)
		return
	}

	// Resolve ONLY the pages that actually applied — stale/notfound rows stay
	// PENDING so the queue can show the stale state and the admin can re-run.
	var appliedPaths []string
	applied, stale, notfound := 0, 0, 0
	for _, res := range results {
		switch res.Status {
		case pages.TagApplyApplied:
			appliedPaths = append(appliedPaths, res.PagePath)
			applied++
		case pages.TagApplyStale:
			stale++
		case pages.TagApplyNotFound:
			notfound++
		}
	}
	if len(appliedPaths) > 0 {
		if err := h.tagSuggestions.ResolvePending(ctx, appliedPaths); err != nil {
			// The tags ARE applied + committed; failing the response now would mislead
			// the admin into re-approving. Log and continue — a still-pending row is
			// harmless (a re-approve re-applies byte-stably / no-ops, idempotent).
			slog.Error("tag approve resolve pending failed (tags already applied)",
				slog.String("error", err.Error()))
		}
	}

	// One batch audit event with non-secret counts only — never the tags or page
	// content (mirrors handleApplyTags' discipline).
	_ = h.audit.Record(ctx, audit.Event{
		Action: audit.ActionAgentPatchApproval,
		Actor:  actor,
		Detail: fmt.Sprintf("mode=approve-tags-batch applied=%d stale=%d notfound=%d skipped=%d role=%s",
			applied, stale, notfound, len(skipped), h.actorRole(ctx)),
		Source: "web-ui",
	})

	// Return per-page results (applied/stale/notfound from the batch + skipped from
	// pre-checks) so the UI can decrement the backlog for applied pages and switch a
	// stale page into the inherited stale state.
	out := make([]pages.TagApplyResult, 0, len(results)+len(skipped))
	out = append(out, results...)
	out = append(out, skipped...)
	writeJSON(w, http.StatusOK, out)
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
