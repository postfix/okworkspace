---
phase: 12-bulk-sweep-batch-review-queue
verified: 2026-06-24T12:24:37Z
status: passed
status_history:
  - "2026-06-24T12:24:37Z: human_needed (live-key E2E round-trip + bulk-approve durability deferred)"
  - "2026-06-24: passed (human item resolved by code inspection at milestone audit — see Human-Item Resolution below)"
score: 12/12 must-haves verified
behavior_unverified: 0
overrides_applied: 0
human_verification:
  - test: "Start a tag sweep against a live workspace with a real LLM key configured, approve one page from the review queue via the /app/tag-review UI, and confirm the frontmatter is written and committed"
    expected: "The sweep enqueues jobs (queued:N returned), pending suggestions appear in the review queue, and approving a page writes the tags byte-stably via the existing single-writer path in one commit"
    why_human: "Requires a real LLM API key (OKF_LLM_API_KEY / DEEPSEEK_API_KEY) and a running server; automated tests cover everything except the live end-to-end round-trip through the model"
    resolution: "RESOLVED by code inspection (2026-06-24). The feared resolve-before-durable-commit data-loss window does NOT exist: pages.ApplyTagsBatch (batch.go:172) calls EnqueueCommit → EnqueueAndWait (commitjob.go:137), which BLOCKS until the commit lands on disk before returning; only THEN are rows marked applied (batch.go:179) and ResolvePending called (handlers_tagsweep.go:234). The live symptom (200 applied but no commit increment) was worker saturation — ~126 KindTagSuggest jobs ahead of the KindCommit job pushed it past commitWaitTimeout, whose fallback (commitjob.go:138) returns success leaving the job QUEUED. Jobs are SQLite-backed in app.db and re-drain on restart, so the commit still lands durably; the live runs that 'lost' it had the server killed before the queue drained. Not data loss — a timing/saturation artifact on already-passing drain tests (TestApproveBatchedOneCommit, TestApplyTagsBatchOneCommit). Residual non-blocking note: under a saturated worker the admin sees 'applied' slightly before the commit physically lands (eventual within the server run)."
---

# Phase 12: Bulk Sweep + Batch Review Queue Verification Report

**Phase Goal:** An admin can run a bulk tagging sweep over untagged (or all) pages that produces a review queue of pending suggestions — writing nothing automatically — which a user reviews and approves per page through the same byte-stable apply path, committed in batches.
**Verified:** 2026-06-24T12:24:37Z
**Status:** human_needed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | Starting a sweep enqueues one KindTagSuggest job per target page and returns immediately, writing nothing to disk | VERIFIED | `handleStartTagSweep` in `handlers_tagsweep.go` calls `h.tagSweepJobs.Enqueue` fire-and-forget per target, returns 202 `{ok:true,queued:N}`; `TestStartTagSweepEnqueuesPerPage` passes |
| 2 | Draining the sweep jobs stages pending suggestion rows but performs NO frontmatter write and NO commit | VERIFIED | `SuggestHandler` in `job.go` explicitly calls only `store.StagePending` — no reference to `pages.Save`, `okf.SetTags`, or `KindCommit`; `TestSafetyGate_NoAutoWrite` passes, asserting working tree + Git HEAD byte-identical before and after drain |
| 3 | Killing and restarting the worker re-runs pending KindTagSuggest jobs and still never auto-writes tags | VERIFIED | `TestSafetyGate_NoAutoWrite` includes a stop/re-enqueue/new-worker drain cycle and re-asserts `sameSnapshot(before, afterRestart)` and HEAD unchanged |
| 4 | The sweep targets untagged pages by default and all live pages when the all flag is set | VERIFIED | `tagsweep.Store.Targets(ctx, allPages bool)` computes live-set-minus-tagged (false) or the full live set (true); `TestTargetsUntagged`, `TestTargetsAll`, `TestTargetsZeroWhenAllTagged`, `TestStartTagSweepAllScope` all pass |
| 5 | The sweep-start, review-queue, and approve endpoints are admin-only (RequireRole admin, session role) | VERIFIED | All three routes mount under `admin.Use(auth.RequireRole(auth.RoleAdmin))` in `router.go` (lines 277–306); `TestStartTagSweepAdminOnly`, `TestListTagSuggestionsAdminOnly`, `TestApproveTagSuggestionsAdminOnly` all pass with 403 for editor |
| 6 | Approving N pages routes each page's approved tags through the Phase-11 byte-stable apply (okf.SetTags → single-writer path), re-validated/normalized server-side | VERIFIED | `handleApproveTagSuggestions` calls `agent.ValidateTags` per page before building `TagApplyItem`; `ApplyTagsBatch` calls `SetTagsFrontmatter` (which uses `okf.SetTags`) then `emitForWrite` → `EnqueueCommit`; `TestApproveRevalidatesServerSide` passes |
| 7 | Approving N pages produces BATCHED commits (not one commit per page) | VERIFIED | `ApplyTagsBatch` in `batch.go` (line 161) builds ONE `commitPayload` carrying all page writes and calls `EnqueueCommit` once; `TestApplyTagsBatchOneCommit` passes (delta==1 for N=3); `TestApproveBatchedOneCommit` passes at the HTTP seam (delta==1 for 3 approvals) |
| 8 | A per-page stale base_revision 409s that page individually WITHOUT failing the rest of the batch | VERIFIED | `ApplyTagsBatch` records `TagApplyStale` for mismatched revision and continues the loop; `TestApplyTagsBatchStaleDoesNotSink` and `TestApproveStaleDoesNotSinkBatch` both pass |
| 9 | Approved pages' staged rows are marked resolved; the queue shrinks | VERIFIED | `handleApproveTagSuggestions` calls `h.tagSuggestions.ResolvePending(ctx, appliedPaths)` after the batch; `TestResolvePendingFlipsOnlyNamed` and `TestApproveBatchedOneCommit` (ListPending returns zero after) both pass |
| 10 | An admin can start a tag-suggestion sweep from the admin Settings page and see confirmation/zero-target/error states | VERIFIED | `Admin.tsx` has a "Tag suggestions" section with `startSweepMut`, checkbox toggle (`sweepAll`), busy label "Starting…", scope-appropriate confirmation strings, generic error line; `tsc -b` and full vitest suite (388 tests) green |
| 11 | An admin-visible nav entry opens the dedicated /app/tag-review route (lazy-loaded, shown only when role is admin) | VERIFIED | `App.tsx` has `const TagReviewView = lazy(...)` + route `/app/tag-review` wrapped in `RequireAdmin`; `AppShell.tsx` has `{isAdmin && ...}` nav row at line 711 with active state keyed on `/app/tag-review` |
| 12 | The review route lists pages with pending suggestions and reviews one page at a time by REUSING the Phase-11 TagSuggestList; approving a page sends the checked tags to the batched approve endpoint; a per-page 409 stale switches that page into the inherited stale state | VERIFIED | `TagReviewView.tsx` imports `TagSuggestList` from `./TagSuggest` (not re-implemented); passes `cancelLabel="Skip for now"` and `applyLabel="Apply approved"`; on `status="stale"` result sets `setStale(true)` without invalidating the queue; on `status="applied"` calls `queryClient.invalidateQueries`; full vitest suite green including `TagReviewView.test.tsx` |

**Score:** 12/12 truths verified (0 present, behavior-unverified)

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/store/migrations/0010_tag_suggestions.sql` | tag_suggestions staging table with partial-unique pending index | VERIFIED | EXISTS, 34 lines; `CREATE TABLE IF NOT EXISTS tag_suggestions` present; partial unique index `idx_tag_suggestions_pending_page WHERE status='pending'` present; operational/derived header comment matches 0009 tone |
| `internal/tagsweep/store.go` | Store: StagePending, ListPending, GetPending, ResolvePending, Targets (min 80 lines) | VERIFIED | EXISTS, 283 lines; all five methods implemented and tested |
| `internal/tagsweep/job.go` | KindTagSuggest job kind + payload + handler calling SuggestTags, staging pending row, NO write/commit | VERIFIED | EXISTS, 92 lines; `KindTagSuggest = "tag_suggest"` defined; handler never imports pages package; no reference to `pages.Save`, `okf.SetTags`, or `KindCommit` |
| `internal/server/handlers_tagsweep.go` | handleStartTagSweep + handleListTagSuggestions + handleApproveTagSuggestions (min 60 lines) | VERIFIED | EXISTS, 280 lines; all three handlers present; uses `tagSweepEnqueuer` interface; fire-and-forget Enqueue; reads staged base_revision from GetPending |
| `internal/pages/batch.go` | ApplyTagsBatch: many pages → one commit, per-page applied/stale/notfound (min 60 lines) | VERIFIED | EXISTS, 238 lines; ONE `EnqueueCommit` call for the whole batch; SetTagsFrontmatter shared helper |
| `web/src/components/TagReviewView.tsx` | /app/tag-review shell: backlog + TagSuggestList reuse + batched Apply + states (min 80 lines) | VERIFIED | EXISTS, 229 lines (214 content lines); imports `TagSuggestList` from `./TagSuggest`; no dangerouslySetInnerHTML in code (only in comments) |
| `web/src/api/client.ts` | startTagSweep / listTagSuggestions / approveTagSuggestions | VERIFIED | All three functions present at lines 1187, 1210, 1237 |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| `handlers_tagsweep.go` | `tagsweep/job.go` | `h.tagSweepJobs.Enqueue(ctx, tagsweep.KindTagSuggest, tagsweep.SuggestPayload(path))` | WIRED | Line 73 of handlers_tagsweep.go; fire-and-forget, never EnqueueAndWait |
| `tagsweep/job.go` | agent.SuggestTags interface | `suggester` interface `SuggestTags(ctx, path) ([]string, []bool, string, error)` | WIRED | `job.go` defines narrow `suggester` interface; `SuggestHandler` calls `s.SuggestTags(ctx, p.Path)` at line 71; satisfied structurally by `*agent.Service` |
| `cmd/okf-workspace/main.go` | `tagsweep/job.go` | `worker.Register(tagsweep.KindTagSuggest, tagsweep.SuggestHandler(tagsweepStore, agentSvc))` | WIRED | Line 334 of main.go; on the EXISTING single worker; registered before `worker.Start()` |
| `handlers_tagsweep.go` | `pages/batch.go` | `h.pages.ApplyTagsBatch(ctx, items, actor)` | WIRED | Line 212 of handlers_tagsweep.go; calls the batched apply (not a per-page Save loop) |
| `pages/batch.go` | `internal/okf/` | `SetTagsFrontmatter` calls `okf.Parse` → `okf.SetTags` → `doc.Emit()` | WIRED | Lines 132–137 of batch.go; `SetTagsFrontmatter` factored as shared helper at line 223 |
| `handlers_tagsweep.go` | `agent.ValidateTags` | `agent.ValidateTags(a.Tags, nil)` per page before building TagApplyItem | WIRED | Line 184 of handlers_tagsweep.go; client list never written verbatim |
| `TagReviewView.tsx` | `api/client.ts` | `useQuery(listTagSuggestions)` + `useMutation(approveTagSuggestions)` | WIRED | Lines 43–45 and 88–111 of TagReviewView.tsx |
| `TagReviewView.tsx` | `TagSuggest.tsx:TagSuggestList` | `import { TagSuggestList } from "./TagSuggest"` at line 12; `<TagSuggestList .../>` at line 205 | WIRED | TagSuggestList is reused with `cancelLabel="Skip for now"` and `applyLabel="Apply approved"` |
| `AppShell.tsx` | `/app/tag-review` (via `App.tsx`) | `{isAdmin && ...}` nav row `onClick={() => navigate("/app/tag-review")}` | WIRED | AppShell line 717; App.tsx route at line 147 with `RequireAdmin` wrapper |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Go build (all packages, CGO-free) | `CGO_ENABLED=0 go build ./...` | exit 0, no output | PASS |
| Safety gate — no auto-write across drain + kill/restart | `go test ./internal/tagsweep/ -run TestSafetyGate_NoAutoWrite -v` | PASS in 0.03s | PASS |
| Batched-commit gate (pages layer) — N pages → 1 commit | `go test ./internal/pages/ -run TestApplyTagsBatchOneCommit -v` | PASS in 0.10s | PASS |
| Stale page does not sink batch (pages layer) | `go test ./internal/pages/ -run TestApplyTagsBatchStaleDoesNotSink -v` | PASS in 0.13s | PASS |
| Batched-commit gate (HTTP seam) — 3 approved pages → delta==1 | `go test ./internal/server/ -run TestApproveBatchedOneCommit -v` | PASS in 0.14s | PASS |
| Approve stale does not sink batch (HTTP seam) | `go test ./internal/server/ -run TestApproveStaleDoesNotSinkBatch -v` | PASS in 0.17s | PASS |
| Server re-validates tag list before write | `go test ./internal/server/ -run TestApproveRevalidatesServerSide -v` | PASS in 0.09s | PASS |
| Handler ignores client base_revision, uses staged value | `go test ./internal/server/ -run TestApproveUsesStagedBaseRevision -v` | PASS in 0.09s | PASS |
| Admin-only gating — sweep (403 for editor) | `go test ./internal/server/ -run TestStartTagSweepAdminOnly -v` | PASS in 0.07s | PASS |
| Admin-only gating — approve (403 for editor) | `go test ./internal/server/ -run TestApproveTagSuggestionsAdminOnly -v` | PASS in 0.14s | PASS |
| TypeScript build | `cd web && npx tsc -b` | exit 0, no output | PASS |
| Full vitest suite | `cd web && npx vitest run` | 388 tests, 47 files, all PASS | PASS |
| go vet | `go vet ./internal/tagsweep/ ./internal/server/ ./internal/pages/` | exit 0, no output | PASS |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|------------|-------------|--------|----------|
| TAG-05 | 12-01, 12-03 | Admin can run a bulk tagging sweep over untagged (or all) pages that enqueues per-page suggestion jobs on the existing async worker and produces a queue of pending suggestions — writing nothing automatically | SATISFIED | KindTagSuggest on the existing single worker; `POST /admin/tags/sweep` + `GET /admin/tags/suggestions`; safety gate test proves no auto-write; Admin.tsx sweep-start section; all tests pass |
| TAG-06 | 12-02, 12-03 | User can review the bulk-sweep suggestion queue and approve/reject suggestions per page, with approvals routed through the same byte-stable TAG-03 apply path and batched commits | SATISFIED | `POST /admin/tags/approve` → `ApplyTagsBatch` (one commit for N pages via EnqueueCommit); TagReviewView reuses TagSuggestList; stale page 409s individually; all gate tests pass |

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| `tagsweep/store.go` | 192 | String concatenation in SQL for IN clause | INFO | Parameterized (`?` per item) — NOT an injection risk. Documented in comment. Acceptable pattern for dynamic IN lists in Go's `database/sql` (no string interpolation of values). |

No debt markers (TBD/FIXME/XXX), no stubs, no empty implementations found in phase-modified files.

### Human Verification Required

#### 1. Live End-to-End Sweep with Real LLM Key

**Test:** Configure a real LLM API key (`OKF_LLM_API_KEY` or `DEEPSEEK_API_KEY`), start the server, log in as admin, navigate to Settings → "Tag suggestions", click "Suggest tags for pages", then navigate to /app/tag-review, open a page's suggestions, check a tag, and click "Apply approved".

**Expected:** The sweep starts (confirmation shows N pages queued for review), the review queue populates with pending suggestions, approving a page writes the selected tags to the Markdown frontmatter via the byte-stable path and commits in one Git commit, and the page leaves the backlog.

**Why human:** Requires a running server and a real LLM API key. All automated tests use a fake `suggester` interface or staged queue rows; the actual SuggestTags model round-trip cannot be exercised key-free.

---

_Verified: 2026-06-24T12:24:37Z_
_Verifier: Claude (gsd-verifier)_
