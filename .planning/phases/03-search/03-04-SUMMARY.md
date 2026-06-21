---
phase: 03-search
plan: 04
subsystem: api
tags: [search, bleve, incremental-index, mutation-hooks, fire-and-forget, CR-01, concurrency, race-test, admin-rebuild, hidden-git]

# Dependency graph
requires:
  - phase: 03 (plan 01 — backend search foundation)
    provides: search.KindIndex job + IndexHandler, indexPayload + Upsert/DeletePagePayload + Upsert/DeleteAttachmentPayload helpers, atomic rebuild + startup drift backstop, POST /admin/search/reindex (admin, CSRF, 202), Index.withIndex swap-guarded shared bleve.Index, harness_test.go real-index harness
  - phase: 03 (plan 03 — attachments/headings indexing)
    provides: indexPage/deletePage maintain heading sub-docs, indexAttachment/deleteAttachment, page_headings stale-cleanup, typed query results
  - phase: 02 (attachments + text extraction)
    provides: ExtractHandler extraction-done path + the CR-01 fire-and-forget lesson, attachments Upload/Replace/Remove lifecycle
  - phase: 01 (pages)
    provides: pages Create/Save/CreateFolder/Rename/Move/Delete/Restore mutation paths, EnqueueCommit single-writer spine, enqueuer seam
provides:
  - INCREMENTAL search index freshness — every page mutation (create/save/createfolder/rename/move/delete/restore) and attachment mutation (upload/replace/orphan-remove) and extraction-done enqueues a fire-and-forget search.KindIndex job so results stay live without a restart
  - CR-01 safety proof — TestExtractJob_UsesFireAndForgetEnqueue: a method-recording fake asserts the extraction-done re-index uses Enqueue and NEVER EnqueueAndWait from inside the drain goroutine (T-03-16)
  - concurrency safety proof — TestIndex_ConcurrentReadWrite: many readers + single writer + rebuild-swap-under-load, race-clean under -race (T-03-19)
  - admin "Rebuild search index" UI wired to the existing reindex endpoint (CSRF, admin-only, 202 async confirmation, no Git/Bleve vocabulary)
  - hardened Index.withIndex (holds RLock across the whole op so a swap cannot close the bleve handle mid-read)
affects: [end-of-phase verification; phase 04+ which can rely on a live search index]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Mutation sites enqueue search.KindIndex FIRE-AND-FORGET via worker.Enqueue: HTTP-handler goroutines (pages/attachments services) use Enqueue to keep search freshness off the save/upload latency path; the extraction-done handler (running ON the drain goroutine) MUST use Enqueue — never EnqueueAndWait (CR-01 deadlock)"
    - "Search freshness is best-effort: a dropped enqueue is logged at Warn and swallowed; correctness is the idempotent rebuild + startup drift check + admin rebuild (T-03-20 accept)"
    - "A rename/move is an index MOVE: delete the OLD doc (and its headings) + upsert the NEW path + re-index every link-rewritten page in the same commit"
    - "search owns the index payload shapes (Upsert/Delete Page/Attachment Payload); pages/attachments import internal/search directly — no import cycle (search reads files via repo, never imports pages/attachments)"
    - "withIndex holds the RLock for the WHOLE bleve op so a concurrent atomic swap (write lock) cannot Close() the snapshotted handle mid-read"

key-files:
  created:
    - internal/search/concurrency_test.go
  modified:
    - internal/pages/service.go
    - internal/pages/rename.go
    - internal/pages/trash.go
    - internal/pages/service_test.go
    - internal/pages/history_helpers_test.go
    - internal/attachments/service.go
    - internal/attachments/lifecycle.go
    - internal/attachments/extractjob.go
    - internal/attachments/service_test.go
    - internal/attachments/extractjob_test.go
    - internal/search/index.go
    - web/src/routes/Admin.tsx
    - web/src/routes/Admin.css
    - web/src/api/client.ts

decisions:
  - "Imported internal/search directly into internal/pages and internal/attachments (no import cycle exists — search reads content via repo and never imports those packages). This makes search the single owner of the KindIndex kind + payload shapes, matching the interface_contract, instead of threading a new function-value seam."
  - "Added enqueueIndexUpsert/enqueueIndexDelete helpers on each service (pages + attachments) that log+swallow enqueue failures at Warn — search freshness is best-effort (T-03-20), so a dropped enqueue never fails the user-facing mutation; the rebuild backstop reconciles."
  - "CreateFolder (seeds index.md) also enqueues an upsert — a folder index page is a searchable page; keeping it indexed avoids a stale gap until the next rebuild (Rule 2 correctness)."
  - "Rename/Move re-index the link-rewritten OTHER pages too (their body bytes changed in the same commit), not just the moved page."
  - "Attachment orphan Remove enqueues an index delete ONLY when the file was actually orphan-deleted (refs==0); when the file is kept (still referenced elsewhere), the doc is intentionally left in place."
  - "Hardened withIndex to hold the RLock across the whole op (was snapshot-then-release). The race test exposed that a swap could Close() the old handle while an in-flight query still held it (surfacing 'index closed'); holding the RLock fixes it without serializing readers against each other (bleve per-index ops are concurrency-safe)."
  - "capturingWorker / capturingAllWorker test fakes now record ONLY KindCommit payloads, so the new index enqueues don't overwrite the captured commit payload (fixed TestPushFlagThreaded / TestPushFlagReachesPayload)."

requirements-completed: [SRCH-01, SRCH-02, SRCH-03, SRCH-04, SRCH-05, SRCH-06]

# Metrics
duration: ~30m
completed: 2026-06-21
---

# Phase 3 Plan 04: Search Lifecycle Hardening Summary

**Wired the INCREMENTAL search layer: every page mutation (create/save/createfolder/rename/move/delete/restore), every attachment mutation (upload/replace/orphan-remove), and the extraction-done handler now enqueue a fire-and-forget `search.KindIndex` job so search stays live with the workspace without a restart — with a named CR-01 test proving the extraction-done re-index never deadlocks the drain goroutine, a `-race` concurrency test proving reads and writes don't race, and an admin "Rebuild search index" button wired to the existing reindex endpoint.**

## What Was Built

- **Incremental enqueue hooks at every mutation site** (Task 1):
  - `pages/service.go`: `Create`, `Save`, `CreateFolder` → `UpsertPagePayload`.
  - `pages/rename.go` `relocate` (Rename + Move): `DeletePagePayload(old)` + `UpsertPagePayload(new)` + re-index of every link-rewritten page.
  - `pages/trash.go`: `Delete` → `DeletePagePayload(original)` (removes page + heading docs; the page now lives under the trash-excluded prefix); `Restore` → `UpsertPagePayload(restored)`.
  - `attachments/service.go` `Upload` + `lifecycle.go` `Replace` → `UpsertAttachmentPayload(id)`; `Remove` (orphan delete only) → `DeleteAttachmentPayload(id)`.
  - `attachments/extractjob.go` extraction-done: after the existing fire-and-forget `.txt` `Enqueue(kindCommit, …)`, a fire-and-forget `Enqueue(search.KindIndex, UpsertAttachmentPayload(id))` re-indexes the attachment WITH its extracted text. Runs ON the drain goroutine → **`Enqueue`, never `EnqueueAndWait`** (CR-01). A failure to enqueue logs + continues; it never changes the extraction-status outcome.
  - All HTTP-handler-context enqueues are `worker.Enqueue` (fire-and-forget) so search freshness adds no latency to saves/uploads; failures are logged at Warn and swallowed (rebuild backstop reconciles).

- **Admin Rebuild UI + client mutation** (Task 2):
  - `client.ts` `reindexSearch()` — CSRF-protected `POST /api/v1/admin/search/reindex` via the existing `mutate` helper (echoes `X-CSRF-Token`).
  - `Admin.tsx` "Search" section with a "Rebuild search index" button (hidden-Git label — NOT "Reindex Bleve"), a react-query `useMutation`, an async (202) muted confirmation ("Search index rebuild started."), an inline error on failure, and disabled-while-pending.
  - `Admin.css` `.admin-section` / `.admin-section-row` from design tokens.

- **Concurrency race test + a real concurrency fix** (Task 2):
  - `internal/search/concurrency_test.go` `TestIndex_ConcurrentReadWrite` — 8 reader goroutines (`Query`) + 1 writer (interleaved `indexPage`/`deletePage`/`indexAttachment`) + a rebuild-swap-under-load goroutine against the ONE shared `bleve.Index`, asserting no panic / no data race under `-race`.
  - **Fix (Rule 1):** `Index.withIndex` now holds the `RLock` for the whole bleve op (was snapshot-then-release), so a concurrent atomic swap can't `Close()` the handle mid-read.

## Tasks Completed

| Task | Name | Commit | Key files |
| ---- | ---- | ------ | --------- |
| 1 | Incremental KindIndex enqueues at every page+attachment mutation + extraction-done (CR-01) | 422a3a7 | pages/{service,rename,trash}.go, attachments/{service,lifecycle,extractjob}.go + their tests |
| 2 | Admin Rebuild UI + reindexSearch client + concurrency race test (+ withIndex fix) | 7330182 | search/concurrency_test.go, search/index.go, web/src/routes/Admin.{tsx,css}, web/src/api/client.ts |
| 3 | End-of-phase human-verify (13 browser checks) | DEFERRED | — (see "Deferred Browser Verification") |

## Tests

New/updated tests, all passing:
- `TestExtractJob_UsesFireAndForgetEnqueue` (CR-01 / T-03-16) — method-recording fake proves the extraction-done re-index uses `Enqueue` and the handler never calls `EnqueueAndWait`.
- `TestIndex_ConcurrentReadWrite` (T-03-19) — race-clean under `-race`.
- Pages: `TestCreateEnqueuesIndexUpsert`, `TestSaveEnqueuesIndexUpsert`, `TestRenameEnqueuesIndexMove`, `TestMoveEnqueuesIndexMove`, `TestDeleteEnqueuesIndexDelete`, `TestRestoreEnqueuesIndexUpsert`.
- Attachments: `TestUploadEnqueuesIndex`, `TestReplaceEnqueuesIndex`, `TestRemoveEnqueuesIndexDelete`.

Gates (all green):
- `CGO_ENABLED=0 go build ./...`
- `CGO_ENABLED=0 go test ./...`
- `CGO_ENABLED=1 go test ./internal/search/ ./internal/pages/ ./internal/attachments/ ./internal/server/ -race` (race detector is a test-time tool only; the shipped binary stays `CGO_ENABLED=0`)
- `cd web && npx tsc --noEmit` + `npx eslint src/routes/Admin.tsx src/api/client.ts` + `npm run build` + `npx vitest run` (116 tests)

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] `withIndex` could read a closed bleve handle during a rebuild swap**
- **Found during:** Task 2 (writing `TestIndex_ConcurrentReadWrite`).
- **Issue:** `withIndex` snapshotted the index pointer under the RLock, released the lock, then ran the bleve op. A concurrent `swapDir` (write lock) could `Close()` that snapshotted handle while an in-flight `Query` was still using it, surfacing `search: index is closed` under load (a real data-lifecycle race — exactly the T-03-19 hazard the test targets).
- **Fix:** `withIndex` now holds the `RLock` for the whole duration of the op (`defer s.mu.RUnlock()`), so a swap (write lock) blocks until in-flight ops finish. Many readers still run concurrently; only a swap briefly blocks them. Bleve per-index ops are concurrency-safe, so this adds correctness without serializing readers against each other.
- **Files modified:** internal/search/index.go
- **Verification:** `TestIndex_ConcurrentReadWrite` fails before the fix (`index is closed`), passes after; full search package green under `-race`.
- **Commit:** 7330182

**2. [Rule 3 - Blocking] Test fakes captured the wrong payload after adding index enqueues**
- **Found during:** Task 1 (running the pages suite).
- **Issue:** `capturingWorker` / `capturingAllWorker` recorded the LAST enqueued payload regardless of kind. After mutations began enqueuing a `search.KindIndex` job AFTER the commit, `TestPushFlagThreaded` / `TestPushFlagReachesPayload` read the index payload (Push=false) instead of the commit payload and failed.
- **Fix:** Both fakes now record ONLY `KindCommit` payloads (the index job is correctly ignored by those commit-payload assertions).
- **Files modified:** internal/pages/service_test.go, internal/pages/history_helpers_test.go
- **Commit:** 422a3a7

### Minor scope notes (within plan)
- The payload helper constructors (`UpsertPagePayload`/`DeletePagePayload`/`UpsertAttachmentPayload`/`DeleteAttachmentPayload`) already existed from 03-01/03-03 — this plan only added the enqueue CALL sites, as the interface_contract anticipated ("Reuses, does not extend").
- Added `CreateFolder` index-upsert (its seeded `index.md` is a searchable page) and re-indexing of rename/move link-rewritten pages — both within the plan's "keeps search live across every mutation" intent.

## Deferred Browser Verification (Task 3 checkpoint — end-of-phase)

Per `human_verify_mode: end-of-phase`, Task 3 is a browser-only human-verify checkpoint covering the full Phase 3 search UX (the 13 checks in 03-04-PLAN.md: ⌘K palette open/focus/placeholder, title/body/tag/attachment-filename/attachment-text/heading results, keyboard nav + Esc, no-match echo, incremental indexing after edit-and-save, disappear-after-trash, admin Rebuild button, and no Git/index/Bleve vocabulary anywhere). All automated gates above are green and the binary builds `CGO_ENABLED=0`; the 13 browser checks are left for the orchestrator's phase verification + browser run (not blocked here per the executor's checkpoint_handling instruction).

## Threat Model Coverage

| Threat ID | Disposition | How addressed |
|-----------|-------------|---------------|
| T-03-16 (self-inflicted deadlock, extraction-done enqueue) | mitigated | Extraction-done re-index uses fire-and-forget `Enqueue` only; `TestExtractJob_UsesFireAndForgetEnqueue` proves (via a method-recording fake) the handler used `Enqueue` and never `EnqueueAndWait` from the drain goroutine. |
| T-03-17 (info disclosure, trash delete) | mitigated | `Delete` enqueues an index delete (page + heading docs) so trashed content leaves results immediately; rebuild backstop reconciles a missed enqueue. |
| T-03-18 (EoP, admin Rebuild) | mitigated | Endpoint is admin-only + CSRF (03-01); `reindexSearch` sends `X-CSRF-Token` via the shared `mutate` helper; the button only renders in the admin route. |
| T-03-19 (data race, shared bleve.Index) | mitigated | One shared index, many readers + single writer; swap under write mutex; `withIndex` holds the RLock across the op; `TestIndex_ConcurrentReadWrite` race-clean under `-race`. |
| T-03-20 (availability, incremental enqueue failure) | accept | Index freshness is best-effort; a dropped enqueue is logged at Warn and reconciled by the startup drift rebuild + admin rebuild (rebuild is the correctness layer). |

## Known Stubs
None. Every mutation path now enqueues the correct incremental index job, proven by per-path enqueue-assertion tests. The only deferral is the end-of-phase browser human-verify (Task 3), recorded above.

## Self-Check: PASSED
- internal/search/concurrency_test.go: FOUND
- internal/search/index.go (withIndex RLock-held): FOUND
- internal/pages/{service,rename,trash}.go index enqueues: FOUND
- internal/attachments/{service,lifecycle,extractjob}.go index enqueues: FOUND
- web/src/routes/Admin.tsx (reindex), web/src/api/client.ts (reindexSearch): FOUND
- Commits 422a3a7, 7330182: FOUND in git log
- Gates: CGO_ENABLED=0 go build ./... green; CGO_ENABLED=0 go test ./... green; -race subset green; web build + tsc + eslint + vitest (116) green.

---
*Phase: 03-search*
*Completed: 2026-06-21*
