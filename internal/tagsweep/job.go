package tagsweep

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/postfix/okworkspace/internal/jobs"
)

// KindTagSuggest is the job kind registered on the EXISTING single jobs worker for
// the bulk tagging sweep. The admin sweep-start endpoint enqueues it
// FIRE-AND-FORGET (worker.Enqueue, NEVER EnqueueAndWait — the CR-01 deadlock
// lesson) once PER target page. The serial single drain is the natural LLM
// rate-limiter (PITFALLS Pitfall 6); there is NO parallel LLM caller. Mirrors
// graph.KindGraph / search.KindIndex exactly.
const KindTagSuggest = "tag_suggest"

// suggestPayload is the JSON enqueued for a KindTagSuggest job: the single page to
// suggest tags for. The payload shape is owned by this package (mirrors
// graph.graphPayload).
type suggestPayload struct {
	Path string `json:"path"`
}

// SuggestPayload builds the JSON payload for a KindTagSuggest job naming one page
// (mirrors graph.UpsertPagePayload).
func SuggestPayload(path string) string {
	raw, _ := json.Marshal(suggestPayload{Path: path})
	return string(raw)
}

// suggester is the narrow consumer interface the job needs: the Phase-11
// SuggestTags mode for ONE page, returning the suggested tags, their
// existing-vs-new flags, and the base revision captured at suggest time. It is
// satisfied structurally by *agent.Service — so this package does NOT import
// internal/agent (mirrors how internal/agent declared vocabularyReader for
// *graph.Store). The job REUSES Phase-11 suggestion verbatim; it never
// re-implements it.
type suggester interface {
	SuggestTags(ctx context.Context, path string) (tags []string, existing []bool, baseRev string, err error)
}

// SuggestHandler returns the jobs.Handler registered for KindTagSuggest (mirrors
// graph.GraphHandler's constructor shape). Given a payload naming one page it
// calls the injected suggester for that page and STAGES the (tag, existing) pairs
// + base_revision into tag_suggestions as a pending row. It NEVER calls
// pages.Save, NEVER writes a file, and NEVER enqueues a commit — staging only.
// This is the load-bearing safety guarantee (PITFALLS Pitfall 5, go/no-go): the
// sweep PRODUCES pending rows; a write happens solely via a human-approved apply.
//
// The whole body runs under a defer recover() so a parser/model panic on one page
// becomes a returned error — the single drain goroutine survives (mirrors
// GraphHandler verbatim). A suggester error is RETURNED (the worker applies its
// retry/backoff) — it never stages a partial/garbage row and never swallows the
// error. This handler NEVER re-enqueues; if it ever did, it would use w.Enqueue
// (fire-and-forget), never EnqueueAndWait (CR-01).
func SuggestHandler(store *Store, s suggester) jobs.Handler {
	return func(ctx context.Context, payload string) (err error) {
		defer func() {
			if rec := recover(); rec != nil {
				err = fmt.Errorf("tagsweep: handler panic: %v", rec)
			}
		}()

		var p suggestPayload
		if uerr := json.Unmarshal([]byte(payload), &p); uerr != nil {
			return fmt.Errorf("tagsweep: payload: %w", uerr)
		}

		tags, existing, baseRev, serr := s.SuggestTags(ctx, p.Path)
		if serr != nil {
			// RETURN the error so the worker retries with backoff. Stage NOTHING —
			// a partial/garbage row must never reach the review queue.
			return fmt.Errorf("tagsweep: suggest tags for %q: %w", p.Path, serr)
		}

		// Zip tags + existing flags into staged suggestions. SuggestTags returns
		// parallel slices (existing[i] is the flag for tags[i]); guard against a
		// short existing slice defensively (default new=false).
		sugg := make([]Suggestion, 0, len(tags))
		for i, tag := range tags {
			ex := false
			if i < len(existing) {
				ex = existing[i]
			}
			sugg = append(sugg, Suggestion{Tag: tag, Existing: ex})
		}

		return store.StagePending(ctx, p.Path, sugg, baseRev)
	}
}
