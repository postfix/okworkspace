// This file holds the BATCHED tag apply (TAG-06): applying an approved set of
// tags to MANY pages in ONE commit through the SAME single-writer commit path
// the per-page Save uses (D-04). It is the apply half of the bulk review queue —
// the admin approve endpoint (internal/server) re-validates each page's tags
// server-side, reads the STAGED base_revision from the queue, and calls
// ApplyTagsBatch so N approved pages produce exactly ONE commit (Pitfall 6), not
// one commit per page. A per-page stale base_revision or missing page is reported
// individually and EXCLUDED from the commit WITHOUT failing the rest of the batch.
package pages

import (
	"context"
	"fmt"

	"github.com/postfix/okworkspace/internal/gitstore"
	"github.com/postfix/okworkspace/internal/okf"
)

// Per-page outcome statuses returned by ApplyTagsBatch. They are NOT errors —
// the batch never aborts on a single page; each page's fate is reported in its
// own TagApplyResult so the caller (the admin approve handler) can resolve the
// applied rows and leave stale/notfound rows pending for a re-run.
const (
	// TagApplyApplied means the page's tags were written byte-stably and landed
	// in the batch commit.
	TagApplyApplied = "applied"
	// TagApplyStale means the page's staged base_revision no longer matches the
	// current committed revision (the page moved since it was suggested). The page
	// is NOT written and NOT clobbered — the optimistic-concurrency floor, applied
	// per page so one moved page never sinks the batch.
	TagApplyStale = "stale"
	// TagApplyNotFound means the page no longer exists on disk; it is skipped.
	TagApplyNotFound = "notfound"
)

// TagApplyItem is one page's approved-and-already-server-re-validated tag write
// request. Tags MUST already be normalized/capped/validated by the caller (the
// approve handler runs agent.ValidateTags per page — the batch trusts that the
// caller validated, it does not re-run validation here). BaseRevision is the
// STAGED revision the suggestion was captured against (the server's record), NOT
// a client-claimed value: ApplyTagsBatch re-checks it against the current
// committed revision and 409s (stale) the page individually if it moved.
type TagApplyItem struct {
	PagePath     string
	Tags         []string
	BaseRevision string
}

// TagApplyResult is one page's outcome: its path and one of the TagApply*
// statuses. The batch returns one result per input item, in input order.
type TagApplyResult struct {
	PagePath string `json:"page_path"`
	Status   string `json:"status"`
}

// ApplyTagsBatch applies each item's approved tags to its page byte-stably and
// commits ALL successfully-applied pages in ONE commit through the single-writer
// path (the SAME drain Save uses — no second concurrent writer, Pitfall 6). It
// returns a per-page result list and does NOT return an all-or-nothing error for
// per-page conditions: a page that is missing (notfound) or whose staged
// base_revision is stale is reported in its result and excluded from the commit
// while the other pages still apply. A non-nil error is returned ONLY for an
// infrastructure failure (read/emit/commit-enqueue) that prevents the batch from
// committing at all.
//
// Byte-stability: each applied page's tags lines are rewritten via the SAME
// SetTagsFrontmatter → assemble → Repair → Emit pipeline Save uses, so the body
// and every other frontmatter key are byte-identical (only the tags change). The
// commit is serialized through the single-writer worker exactly like every other
// mutation; staging N writes into one commitPayload yields ONE commit for the
// batch.
//
// Idempotent/resumable: re-running the batch for an already-applied page re-emits
// the same byte-stable tags (a no-op diff) — there is no corruption and a stale
// page on a re-run simply 409s again, so an interrupted batch can be safely
// re-driven from the still-pending queue rows.
func (s *Service) ApplyTagsBatch(ctx context.Context, items []TagApplyItem, actor string) ([]TagApplyResult, error) {
	results := make([]TagApplyResult, 0, len(items))

	// Serialize the whole batch against namespace mutations (create/rename/move/
	// trash) so a concurrent rename cannot move a page out from under the per-page
	// revision check between our read and the single commit. Content edits to the
	// SAME pages are serialized by the single-writer commit queue + the per-page
	// revision re-check below (a save that lands after our revision read makes that
	// page stale → excluded), so this lock only needs to cover the structural
	// namespace race, mirroring how lockMutation guards Create/CreateFolder.
	defer s.lockMutation()()

	writes := make([]fileWrite, 0, len(items))
	committedPaths := make([]string, 0, len(items))
	// applyIdx maps each staged write back to its result index so we can flip the
	// status to "applied" only after the commit actually lands.
	applyIdx := make([]int, 0, len(items))

	for _, it := range items {
		idx := len(results)
		results = append(results, TagApplyResult{PagePath: it.PagePath, Status: ""})

		exists, err := s.repo.Exists(it.PagePath)
		if err != nil {
			return nil, fmt.Errorf("pages: batch exists %q: %w", it.PagePath, err)
		}
		if !exists {
			results[idx].Status = TagApplyNotFound
			continue
		}

		// Per-page optimistic-concurrency floor: compare the STAGED base_revision
		// against the current committed revision. A moved page yields "stale" and is
		// excluded from the commit (no write, no clobber) WITHOUT aborting the batch.
		current, err := s.Revision(ctx, it.PagePath)
		if err != nil {
			return nil, fmt.Errorf("pages: batch revision %q: %w", it.PagePath, err)
		}
		if current != it.BaseRevision {
			results[idx].Status = TagApplyStale
			continue
		}

		raw, err := s.repo.Read(it.PagePath)
		if err != nil {
			return nil, fmt.Errorf("pages: batch read %q: %w", it.PagePath, err)
		}
		doc, err := okf.Parse(raw)
		if err != nil {
			return nil, fmt.Errorf("pages: batch parse %q: %w", it.PagePath, err)
		}

		// Build the new frontmatter region byte-stably via the SAME helper the
		// per-page apply uses (okf.SetTags owns the tags edit), then run it through
		// the SAME assemble → Repair → Emit pipeline Save uses so the written bytes
		// are byte-identical to a normal save (only the tags lines differ).
		newFront, err := SetTagsFrontmatter(string(doc.RawFront), string(doc.Body), it.Tags)
		if err != nil {
			return nil, fmt.Errorf("pages: batch set tags %q: %w", it.PagePath, err)
		}
		out, err := s.emitForWrite(newFront, string(doc.Body))
		if err != nil {
			return nil, fmt.Errorf("pages: batch emit %q: %w", it.PagePath, err)
		}

		// Validate the path through the resolver before staging it into the commit
		// (the CommitJob re-resolves as a backstop; failing here is cleaner).
		if _, err := s.repo.Resolve(it.PagePath); err != nil {
			return nil, fmt.Errorf("pages: batch resolve %q: %w", it.PagePath, err)
		}
		writes = append(writes, fileWrite{Path: it.PagePath, Bytes: out})
		committedPaths = append(committedPaths, it.PagePath)
		applyIdx = append(applyIdx, idx)
	}

	// Nothing applied (every page was stale/notfound) → no commit, return outcomes.
	if len(writes) == 0 {
		return results, nil
	}

	// ONE commit for the WHOLE batch through the single-writer spine (D-04 /
	// Pitfall 6): a single commitPayload carrying every page's write, enqueued on
	// the SAME worker. This is the load-bearing batched-commit invariant — N
	// applied pages produce exactly ONE commit, never one-per-page.
	p := commitPayload{
		Writes: writes,
		Spec: gitstore.CommitSpec{
			Paths:   committedPaths,
			Message: batchCommitSubject(len(writes)),
			User:    actor,
			Action:  "tag-apply-batch",
			Source:  "web-ui",
		},
		Push: s.pushOnCommit,
	}
	if err := EnqueueCommit(ctx, s.worker, p); err != nil {
		return nil, fmt.Errorf("pages: batch commit: %w", err)
	}

	// The commit landed: mark every staged page applied and keep the derived
	// index/graph fresh (fire-and-forget, never blocking — the rebuild backstop
	// reconciles a dropped enqueue, mirroring Save).
	for _, idx := range applyIdx {
		results[idx].Status = TagApplyApplied
	}
	for _, path := range committedPaths {
		s.enqueueIndexUpsert(ctx, path)
		s.enqueueGraphUpsert(ctx, path)
	}
	return results, nil
}

// emitForWrite reassembles a page from a frontmatter region + body, repairs any
// missing required frontmatter (byte-safe via okf.Repair), and re-emits the
// byte-stable bytes to write — the EXACT pipeline Save runs (assemble → Parse →
// Repair → Emit), factored so the batch and Save cannot drift.
func (s *Service) emitForWrite(frontmatter, body string) ([]byte, error) {
	doc, err := okf.Parse(assemble(frontmatter, body))
	if err != nil {
		return nil, fmt.Errorf("pages: parse for write: %w", err)
	}
	okf.Repair(doc, s.now())
	out, err := doc.Emit()
	if err != nil {
		return nil, fmt.Errorf("pages: emit for write: %w", err)
	}
	return out, nil
}

// batchCommitSubject renders the hidden-commit subject for a batched tag apply.
func batchCommitSubject(n int) string {
	if n == 1 {
		return "Apply tags to 1 page"
	}
	return fmt.Sprintf("Apply tags to %d pages", n)
}

// SetTagsFrontmatter returns the new frontmatter REGION string for a page whose
// `tags` key is set to the given (already-normalized) tags byte-stably via
// okf.SetTags — the body and every other frontmatter key stay unchanged. It is
// the ONE place the byte-stable tags-region write is owned, shared by BOTH the
// per-page apply handler (internal/server) AND the batch above so they cannot
// drift. It assembles the page from its frontmatter + body via the canonical
// assembler, parses, sets the tags sequence, re-emits, re-parses the emitted
// bytes, and returns the resulting RawFront region (the caller hands that region
// to the writer, which owns the final ---fence--- assembly).
func SetTagsFrontmatter(frontmatter, body string, tags []string) (string, error) {
	doc, err := okf.Parse(AssembleSource(frontmatter, body))
	if err != nil {
		return "", fmt.Errorf("pages: set-tags parse: %w", err)
	}
	okf.SetTags(doc, tags)
	emitted, err := doc.Emit()
	if err != nil {
		return "", fmt.Errorf("pages: set-tags emit: %w", err)
	}
	doc2, err := okf.Parse(emitted)
	if err != nil {
		return "", fmt.Errorf("pages: set-tags re-parse emitted: %w", err)
	}
	return string(doc2.RawFront), nil
}
