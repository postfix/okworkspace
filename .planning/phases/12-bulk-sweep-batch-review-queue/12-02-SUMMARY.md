---
phase: 12-bulk-sweep-batch-review-queue
plan: 02
subsystem: backend
status: complete
tags: [tagging, batch-apply, review-queue, single-writer, byte-stability, optimistic-concurrency, admin-rbac, csrf, key-free]
requires:
  - "pages.Service single-writer commit path (commitPayload.Writes carries N file writes in ONE commit) + Save 409 floor (Phase 1/D-04)"
  - "okf.SetTags + okf.Repair + okf.Parse/Emit byte-stable frontmatter editor (11-01)"
  - "agent.ValidateTags(raw, vocab) — exported normalize/cap/dedupe/reject (11-02)"
  - "tagsweep.Store + tag_suggestions staging table (pending rows w/ staged base_revision) (12-01)"
  - "server admin subgroup (RequireRole admin + nosurf CSRF) + actorUsername/actorRole/audit helpers + validIdentifier/maxApplyTags (11-02/12-01)"
provides:
  - "pages.ApplyTagsBatch(ctx, []TagApplyItem, actor) ([]TagApplyResult, error) — batched single-writer tag apply: N pages -> ONE commit, per-page outcomes (applied/stale/notfound), one stale/missing page never sinks the batch"
  - "pages.SetTagsFrontmatter — the ONE shared byte-stable tags-region builder (per-page handler + batch both delegate; cannot drift)"
  - "tagsweep.Store.GetPending(ctx, pagePath) (PendingEntry, bool, error) — read the STAGED base_revision (server's source of truth on apply)"
  - "tagsweep.Store.ResolvePending(ctx, pagePaths) — flip approved pending rows to resolved (parameterized IN, no SQLi); queue shrinks"
  - "POST /api/v1/admin/tags/approve (admin) — batched approve: re-validates per page, uses staged base_revision, ONE commit, resolves applied rows, returns per-page results"
affects:
  - internal/pages/batch.go
  - internal/pages/batch_test.go
  - internal/tagsweep/store.go
  - internal/tagsweep/store_test.go
  - internal/server/handlers_tagsweep.go
  - internal/server/handlers_approve_test.go
  - internal/server/handlers_agent.go
  - internal/server/router.go
tech-stack:
  added: []
  patterns:
    - "batched single-writer commit: build ONE commitPayload with N fileWrites -> EnqueueCommit -> exactly one commit (reuses the existing CommitHandler/CommitSpec multi-write payload, no new commit primitive needed)"
    - "per-page outcome list (applied/stale/notfound) instead of all-or-nothing error: a stale/missing page is excluded from the commit, the batch continues"
    - "shared byte-stable tags-region builder moved into internal/pages (SetTagsFrontmatter) so the per-page apply handler delegates to it — single implementation, cannot drift"
    - "server reads the STAGED base_revision via GetPending (never a client value); ApplyTagsBatch re-checks it against the current committed revision per page"
    - "batch serialized against namespace mutations via lockMutation; per-page revision re-check covers concurrent content edits"
key-files:
  created:
    - internal/pages/batch.go
    - internal/pages/batch_test.go
    - internal/server/handlers_approve_test.go
  modified:
    - internal/tagsweep/store.go
    - internal/tagsweep/store_test.go
    - internal/server/handlers_tagsweep.go
    - internal/server/handlers_agent.go
    - internal/server/router.go
decisions:
  - "The existing commitPayload already carries []fileWrite -> one commit, so NO new batch commit primitive was needed: ApplyTagsBatch builds a single commitPayload with every non-stale page's write and calls the SAME EnqueueCommit the single-file Save uses. The batched-commit invariant (Pitfall 6) falls out of reusing the existing multi-write payload on the SAME single drain worker."
  - "setTagsFrontmatter (the byte-stable tags-region builder) was MOVED from internal/server into internal/pages as exported SetTagsFrontmatter; the server's setTagsFrontmatter now delegates. Per-page apply (handleApplyTags) and the batch share ONE implementation so they cannot drift (the plan's key_link requirement)."
  - "A page that normalizes to empty OR is no longer pending is recorded status=skipped in the response (NOT a 400 for the whole batch), mirroring handleApplyTags' fail-soft-per-page discipline; only structural input errors (bad page_path, over-cap, NUL) 400 the request."
  - "ResolvePending failure AFTER a successful commit is logged and swallowed (the tags ARE applied; a still-pending row is harmless because re-approve is idempotent/byte-stable). Failing the response would mislead the admin into re-approving."
metrics:
  duration: ~40m
  completed: 2026-06-24
  tasks: 2
  files: 8
---

# Phase 12 Plan 02: Batched Approve (TAG-06 Apply Half) Summary

Built the apply half of the bulk review queue (TAG-06): an admin-only batched
approve that routes one OR many pages' staged suggestions through the SAME
Phase-11 byte-stable apply (server re-validates/normalizes -> `okf.SetTags` -> the
single-writer commit) but commits in BATCHES — N approved pages produce EXACTLY
ONE commit, never one-per-page (Pitfall 6). A per-page stale `base_revision` 409s
that page individually WITHOUT sinking the rest of the batch; resolved rows leave
the pending queue. The client tag list and any claimed revision are NEVER trusted
— the server re-validates per page and reads the STAGED base_revision from the
queue.

## What Was Built

### Task 1 — batched single-writer apply + resolve transition — commit f1fd448
- `internal/pages/batch.go`: `ApplyTagsBatch(ctx, []TagApplyItem, actor) ([]TagApplyResult, error)`.
  For each item: `repo.Exists` (notfound -> skip), `Revision` vs the staged
  `BaseRevision` (stale -> skip, no write, no clobber), then build the new
  frontmatter byte-stably via `SetTagsFrontmatter` -> the SAME `assemble -> Repair
  -> Emit` pipeline `Save` uses (`emitForWrite`). All non-stale/non-notfound writes
  are collected into ONE `commitPayload` and committed through the existing
  `EnqueueCommit` single-writer path -> ONE commit. Per-page results returned;
  never an all-or-nothing error for per-page conditions. Index/graph refresh fired
  fire-and-forget (mirrors Save). `SetTagsFrontmatter` is the exported shared
  byte-stable tags-region builder.
- `internal/tagsweep/store.go`: `GetPending(ctx, pagePath) (PendingEntry, bool, error)`
  (read the staged base_revision; `(_, false, nil)` when absent) + `ResolvePending(ctx, pagePaths)`
  (UPDATE ... SET status='resolved' WHERE status='pending' AND page_path IN (?...),
  parameterized — no SQLi; empty slice = no-op).
- `internal/server/handlers_agent.go`: `setTagsFrontmatter` now delegates to
  `pages.SetTagsFrontmatter` (single implementation, no drift).
- `internal/pages/batch_test.go` (real git harness): BATCHED-COMMIT GATE (3 pages
  -> `commitCount` delta == 1, all applied, bodies byte-identical, tags present),
  STALE-WITHOUT-SINKING (page 2 mutated out-of-band -> stale, not clobbered, pages
  1+3 applied in ONE commit), NOTFOUND (missing path skipped, rest apply),
  IDEMPOTENT (re-apply same tags -> no dup, no corruption).
- `internal/tagsweep/store_test.go`: GetPending returns staged row / `(false,nil)`;
  ResolvePending flips only named rows (others stay pending, ListPending shrinks);
  empty-slice no-op.

### Task 2 — admin batched approve endpoint + route — commit ee6bc2a
- `internal/server/handlers_tagsweep.go`: `handleApproveTagSuggestions` (admin
  subgroup). Decodes `{approvals:[{page_path, tags}]}`. Per approval: validate
  page_path (`validIdentifier`), cap (`maxApplyTags`), reject NUL, RE-VALIDATE via
  `agent.ValidateTags(tags, nil)` (empty-after-normalize -> skipped, not a 400),
  read STAGED base_revision via `GetPending` (no pending row -> skipped). Build
  `[]pages.TagApplyItem` -> `pages.ApplyTagsBatch` (ONE commit). `ResolvePending`
  the applied paths only (stale/notfound stay pending). One batch audit event with
  non-secret counts (applied/stale/notfound/skipped) — never the tags/content.
  200 with the per-page results array.
- `internal/server/router.go`: `admin.Post("/admin/tags/approve", h.handleApproveTagSuggestions)`
  under the existing `RequireRole(admin)` subgroup + global nosurf CSRF; comment
  notes server re-validates + uses the staged base_revision (client never trusted).
- `internal/server/handlers_approve_test.go` (KEY-FREE; real pages.Service +
  tagsweep.Store + worker + git sharing ONE repo/db): `TestApproveTagSuggestionsAdminOnly`
  (editor 403, admin 200), `TestApproveBatchedOneCommit` (3 pages -> delta==1 +
  ListPending emptied), `TestApproveStaleDoesNotSinkBatch` (page 2 stale, stays
  pending; 1+3 applied in one commit, resolved), `TestApproveRevalidatesServerSide`
  (tampered/over-cap/garbage list normalized+capped to <=5 before write, proven by
  reading frontmatter), `TestApproveUsesStagedBaseRevision` (request omits any
  base_revision; still applies via the staged value).

## Deviations from Plan

### Adjustments

**1. [Adaptation - real commit path] No new batch commit primitive was needed**
The plan allowed "if the current commit path is strictly one-file-per-commit, extend
the CommitSpec/payload to carry multiple file writes." It already does: the existing
`commitPayload.Writes []fileWrite` + `CommitHandler` write every file and create
exactly one commit. `ApplyTagsBatch` simply builds ONE `commitPayload` with all
non-stale page writes and calls the existing `EnqueueCommit`. The batched-commit
invariant falls out of reusing the existing multi-write payload on the SAME single
drain worker — no second writer, no commit-spec change.

**2. [Adaptation - shared helper moved, not factored in place]**
The plan said "factor that helper into a reusable location if it currently lives in
the server package — prefer moving the byte-stable region builder so BOTH share ONE
implementation." Done: `setTagsFrontmatter` moved into `internal/pages` as exported
`SetTagsFrontmatter`; the server's `setTagsFrontmatter` is now a one-line delegate.
Both the per-page apply (`handleApplyTags`) and the batch use the identical builder.

**3. [Adaptation - skipped status for pre-batch drops]**
Pages dropped BEFORE the batch (empty-after-normalize, or no longer pending) are
returned as `status="skipped"` in the results array (not silently omitted, not a
400 for the whole batch), so the UI can surface them. The batch itself returns
applied/stale/notfound. The audit event counts all four buckets.

No new dependency added; CGO-free single binary preserved; the apply path is
KEY-FREE (never calls the LLM); admin-only via the session role.

## Self-Check: PASSED

Files created (all FOUND):
- internal/pages/batch.go, internal/pages/batch_test.go
- internal/server/handlers_approve_test.go

Files modified (all FOUND):
- internal/tagsweep/store.go, internal/tagsweep/store_test.go
- internal/server/handlers_tagsweep.go, internal/server/handlers_agent.go, internal/server/router.go

Commits (all FOUND in git log): f1fd448, ee6bc2a

Verification output (real, pasted):
- `CGO_ENABLED=0 go build ./...` -> exit 0
- `go vet ./internal/server/` -> clean (exit 0)
- `env -u DEEPSEEK_API_KEY -u OKF_LLM_API_KEY go test ./internal/pages/ ./internal/server/ ./internal/tagsweep/` -> ok (all three)
- BATCHED-COMMIT GATE (Pitfall 6) PROVEN:
  - `TestApplyTagsBatchOneCommit` -> PASS (3 pages, commit delta == 1)
  - `TestApproveBatchedOneCommit` -> PASS (HTTP seam, 3 pages, delta == 1, queue emptied)
- STALE-WITHOUT-SINKING PROVEN:
  - `TestApplyTagsBatchStaleDoesNotSink` -> PASS
  - `TestApproveStaleDoesNotSinkBatch` -> PASS (stale page stays pending, 1+3 applied in one commit)
- Server-side re-validation + staged base_revision: `TestApproveRevalidatesServerSide`,
  `TestApproveUsesStagedBaseRevision` -> PASS
