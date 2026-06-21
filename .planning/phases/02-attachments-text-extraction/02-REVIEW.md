---
phase: 02-attachments-text-extraction
reviewed: 2026-06-21T00:21:14Z
depth: standard
files_reviewed: 24
files_reviewed_list:
  - internal/attachments/service.go
  - internal/attachments/extract.go
  - internal/attachments/extractjob.go
  - internal/attachments/lifecycle.go
  - internal/attachments/refs.go
  - internal/attachments/meta.go
  - internal/attachments/id.go
  - internal/attachments/status.go
  - internal/attachments/types.go
  - internal/server/handlers_attachments.go
  - internal/server/handlers_sse.go
  - internal/server/router.go
  - internal/server/handlers_auth.go
  - internal/pages/commitjob.go
  - internal/audit/audit.go
  - internal/store/migrations/0006_attachments.sql
  - web/src/api/client.ts
  - web/src/components/attachments/AttachmentCard.tsx
  - web/src/components/attachments/AttachmentDropzone.tsx
  - web/src/components/attachments/AttachmentsSection.tsx
  - web/src/components/attachments/ExtractionStatus.tsx
  - web/src/components/attachments/ImagePreviewDialog.tsx
  - web/src/components/attachments/RemoveAttachmentDialog.tsx
  - web/src/components/attachments/ReplaceAttachmentDialog.tsx
  - web/src/routes/PageView.tsx
findings:
  critical: 1
  warning: 6
  info: 5
  total: 12
status: resolved
---
> **CR-01 RESOLVED** (commit after review): extract job now uses fire-and-forget `Enqueue` for the `.txt` commit instead of `EnqueueAndWait`, eliminating the single-drain-goroutine reentrant stall. Build + extraction/server tests green.
>
> **WARNINGS RESOLVED** (code-review --fix, 2026-06-21): WR-01, WR-02, WR-03, WR-04, WR-05 fixed with regression tests; WR-06 consciously ACCEPTED and documented in 02-CONTEXT.md. Info items IN-01 and IN-04 also addressed. `CGO_ENABLED=0 go build/test ./...` green; `web` build + `tsc --noEmit` green. See resolution log below.
>
> ## Resolution log (2026-06-21)
> - **CR-01** — Resolved pre-fix (fire-and-forget enqueue). Verified.
> - **WR-01** — FIXED. Upload/Replace now insert/upsert the DB row BEFORE enqueuing the commit and roll the row back (Upload: `deleteRow`; Replace: restore prev meta) on a real (non-timeout) commit-enqueue error, so a DB failure can never leave a committed-but-unlisted orphan. Added `TestUploadRollsBackRowOnCommitError`, `TestReplaceRevertsRowOnCommitError`.
> - **WR-02** — FIXED. Added `PageReferencesExcluding`; `Remove` excludes the just-unlinked `pagePath` from the orphan recount instead of re-reading a possibly-stale working tree. Added `TestPageReferencesExcluding`, `TestRemoveOrphansEvenIfUnlinkCommitNotLanded`.
> - **WR-03** — FIXED. SSE extraction stream now has a 10-minute absolute max-duration cap that emits a terminal `failed` event and closes, so a wedged extraction can't pin a goroutine forever.
> - **WR-04 / IN-05** — VERIFIED + HARDENED. CR-01 already makes the commit fire-and-forget and sets terminal status only after a successful enqueue (durable-by-queue). Closed the remaining gap: on a persistent enqueue failure the handler now records `status=failed` before returning so the chip can't stick on "Extracting…". Added `TestExtractJobEnqueueFailureSetsTerminalStatus`.
> - **WR-05** — FIXED. Dropped the `mt.Is(a)` branch in `allowedExt`; the allow-list is now matched by extension only (one semantics). Existing upload-validation tests (png/jpg/svg/pdf/docx/txt allowed; ELF rejected) stay green.
> - **WR-06** — ACCEPTED (no code change). "All authenticated users may read/download any attachment by id" is the intended model, matching "any authenticated user reads any page". Documented as a conscious decision in `02-CONTEXT.md` under `<decisions>`.
> - **IN-01** — FIXED. Corrected the `ResolveBin` doc comment (it resolves a path; it reads nothing).
> - **IN-04** — FIXED. Extended `humanFileSize` units with `PB`/`EB`.
> - **IN-02 / IN-03** — NOT addressed (cross-layer list-centralization / config-sourcing refactors; out of scope for this fix pass).


# Phase 2: Code Review Report

**Reviewed:** 2026-06-21T00:21:14Z
**Depth:** standard
**Files Reviewed:** 24
**Status:** issues_found

## Summary

Phase 2 (Attachments & Text Extraction) is a careful integration over the existing single-writer / job-worker / safe-path spines, and most of the hard security requirements land correctly: the upload path hard-caps the body with `MaxBytesReader` before parsing (Pitfall 1), the real MIME type is sniffed from magic bytes with the filename never trusted (SEC-02/ATT-09), the on-disk name is an opaque ULID, downloads stream byte-exact via `http.ServeContent` with `X-Content-Type-Options: nosniff` + a `default-src 'none'; sandbox` CSP, SVG is correctly forced to download (the post-review SEC-02 fix is present and faithful), every write routes through the single-writer `commitPayload` mirror, parser panics are recovered in two layers, empty extraction is treated as success, and raw parser errors are kept server-side. The frontend mirrors the server allow-list and renders previews via `<img src>` only.

The review nonetheless surfaces one BLOCKER: the `ExtractHandler` calls `worker.EnqueueAndWait(kindCommit, ...)` from **inside the worker's single drain goroutine**, which cannot make progress on the job it is waiting for. This self-stalls the entire single-writer pipeline for the full 5s commit-wait timeout on every extraction and delays the `.txt` sidecar landing. The remaining findings are robustness and consistency issues (commit-then-DB ordering can orphan files, orphan-delete relies on a soft-success commit having actually landed, SSE has no max-duration cap) plus minor quality items.

The structural pre-pass block was not provided, so this report contains narrative findings only.

## Narrative Findings (AI reviewer)

## Critical Issues

### CR-01: ExtractHandler blocks the single drain goroutine on its own commit job (self-stall / pipeline freeze)

**File:** `internal/attachments/extractjob.go:119` (handler body 59-141); wiring `cmd/okf-workspace/main.go:188-194`; worker model `internal/jobs/worker.go:84-164`, `internal/jobs/queue.go:166-172`, `109-143`

**Issue:** `ExtractHandler` runs synchronously inside the worker's *single* drain goroutine (`loop` → `drainOne` → `h(ctx, payload)`). Inside that handler it calls:

```go
if cerr := w.EnqueueAndWait(ctx, kindCommit, string(raw), commitWaitTimeout); cerr != nil {
```

`EnqueueAndWait` enqueues a `KindCommit` job and then `WaitForJob` *polls the jobs table every `waitPollInterval` until the job is terminal or the 5s timeout elapses*. But the only goroutine that can drain that `KindCommit` job is the very goroutine now blocked inside `WaitForJob`. The commit therefore cannot run until the extract handler returns, and the extract handler will not return until the commit runs — so `WaitForJob` always blocks for the full `commitWaitTimeout` (5s) and returns `ErrJobTimeout`, which the handler swallows as a soft success.

Consequences:
- Every single extraction freezes the entire single-writer worker for ~5 seconds. During that window no page saves, no other uploads, no other commits can drain (they are all FIFO behind the stalled handler). At 5 users this is a severe, user-visible "everything hangs after an upload" stall.
- The `.txt` sidecar commit only lands *after* the extract job is marked done and the drain loop comes back around to the queued commit — so status is set to `done`/`empty` (extractjob.go:133-139) before the artifact is actually on disk.
- The whole point of making extraction `Enqueue` (fire-and-forget) in `service.go:202-207` is defeated: the extraction itself is async, but it then synchronously stalls the shared worker.

The comment at extractjob.go:119-127 anticipates the timeout but misreads it as a "slow drain" — it is in fact a guaranteed self-deadlock against the timeout, not an occasional slow path.

**Fix:** The extract handler must not wait on a job that only it can drain. Use fire-and-forget `Enqueue` for the `.txt` commit so the queued commit drains on the next loop iteration after the handler returns:

```go
// extractjob.go — replace the EnqueueAndWait block with a non-blocking enqueue.
raw, merr := json.Marshal(cp)
if merr != nil {
    return fmt.Errorf("attachments: marshal extract commit payload: %w", merr)
}
if cerr := w.Enqueue(ctx, kindCommit, string(raw)); cerr != nil {
    return fmt.Errorf("attachments: enqueue extracted-text commit %q: %w", p.AttachmentID, cerr)
}
```

Because status is now set before the commit is guaranteed durable, set status only after a successful *enqueue* (the commit is durable-by-queue) — or, preferably, move the status flip into the commit completion path. Note the `enqueuer` interface used here (`extractjob.go:59`, `service.go:41-44`) already exposes `Enqueue`, so no signature change is needed. Add a regression test that registers both handlers on one real `*jobs.Worker`, runs an extract end-to-end, and asserts the `.txt` lands without a multi-second stall.

## Warnings

### WR-01: Upload/Replace commit the binary to Git before the DB row, so a DB failure orphans on-disk files

**File:** `internal/attachments/service.go:183-193` (Upload); `internal/attachments/lifecycle.go:108-116` (Replace)

**Issue:** `enqueueCommit` durably writes `attachments/<id>.<ext>` + `<id>.json` to the working tree and Git first; only then does `insertRow` run. If `insertRow` returns an error the function returns it and the handler 500s — but the binary + meta sidecar are already committed in Git history (and on disk) with no operational row. The file is then invisible to `List` (which reads only the DB), un-downloadable through the UI, never extracted, and never orphan-deletable (the orphan path keys off the row). The comment at service.go:187-193 acknowledges treating a row failure as fatal but does not reconcile the now-orphaned committed bytes.

**Fix:** Either (a) insert the operational row *before* enqueuing the commit (so a DB failure aborts cleanly with nothing committed), accepting a brief window where the row exists without the file; or (b) on `insertRow` error after a successful commit, do not 500 silently — log loudly and still return the meta, since the on-disk sidecar is the source of truth and `List` can be backfilled from disk. Given `List` reads only the DB, (a) is the safer fix:

```go
if err := s.insertRow(ctx, meta); err != nil {
    return AttachmentMeta{}, err
}
if err := s.enqueueCommit(ctx, p); err != nil {
    _ = s.deleteRow(ctx, meta.ID) // roll back the row if the commit never lands
    return AttachmentMeta{}, err
}
```

### WR-02: Orphan-delete ref-count can keep a now-orphaned file if the unlink commit timed out

**File:** `internal/attachments/lifecycle.go:162-174` (Remove), `221-253` (unlinkPage); `internal/attachments/service.go:328-335` (enqueueCommit soft-success)

**Issue:** `Remove` calls `unlinkPage` (which `enqueueCommit`s the edited page) and then `PageReferences` scans the working tree for remaining references. `enqueueCommit` treats `ErrJobTimeout` as success (service.go:329-333) and returns nil *without the page edit necessarily being on disk yet*. If the commit-wait times out (e.g. the worker is stalled — see CR-01, which makes this far more likely), `PageReferences` reads the *pre-edit* page, still finds the link, counts `refs > 0`, and returns `false, nil` — leaving the binary + sidecars in place even though the user removed the last reference. The attachment becomes a silent orphan that the UI reports as "removed."

**Fix:** The unlink-then-recount sequence requires the unlink to be durably on disk. Either make the unlink commit a hard wait (do not swallow `ErrJobTimeout` for this specific step), or re-derive the post-edit reference count from the in-memory edited body for `pagePath` plus a disk scan of the *other* pages, rather than re-reading `pagePath` from disk:

```go
// In Remove: the edited body for pagePath is already known in unlinkPage;
// count other-page refs from disk and treat pagePath as unlinked regardless of
// commit-wait latency, instead of trusting a possibly-stale working tree read.
```

### WR-03: SSE extraction stream has no server-side max-duration cap

**File:** `internal/server/handlers_sse.go:86-97`

**Issue:** The stream loops on a 500ms ticker until a terminal status or `ctx.Done()`. If extraction is wedged (status stuck at `pending`/`extracting` — e.g. the worker stalled per CR-01, or the row never advances), the stream runs forever, one goroutine + one DB query every 500ms per connected client, until the client disconnects. There is no upper bound and no idle/heartbeat-based teardown. At 5 users this is bounded but a tab left open on a never-completing extraction holds a server goroutine indefinitely.

**Fix:** Add a generous absolute cap (e.g. 5–10 minutes) after which the stream emits a terminal event and closes, so a stuck extraction cannot pin a goroutine forever:

```go
deadline := time.NewTimer(10 * time.Minute)
defer deadline.Stop()
for {
    select {
    case <-ctx.Done():
        return
    case <-deadline.C:
        _, _ = fmt.Fprintf(w, "data: {\"status\":%q}\n\n", "failed")
        flusher.Flush()
        return
    case <-ticker.C:
        if terminal, err := emit(); err != nil || terminal {
            return
        }
    }
}
```

### WR-04: ExtractHandler returns a hard error and retries when the `.txt` commit fails, re-running extraction

**File:** `internal/attachments/extractjob.go:119-131`

**Issue:** Independent of CR-01, when the `.txt` commit genuinely fails (non-timeout), the handler returns the error, so the worker retries the *entire* `KindExtract` job — re-reading the binary and re-extracting (CPU-bound PDF/DOCX parse) on every retry, even though the only failed step was the final commit. On a persistently failing commit this burns `MaxAttempts` worth of full re-extractions and ends with `status=done`/`empty` already written at extractjob.go:133-139 only if the commit eventually succeeds — otherwise the row is left at `pending` with no terminal state and no `.txt`.

**Fix:** Once CR-01 is fixed by switching to fire-and-forget `Enqueue`, the commit no longer blocks; an enqueue failure is rare and cheap to retry. If you keep any wait, separate "extraction succeeded" from "commit landed" so a commit retry does not re-run extraction (e.g. cache the extracted text or split into two job kinds). At minimum, set a terminal status on the row before returning the commit error so the chip does not hang on "Extracting…".

### WR-05: `mt.Is(a)` allow-list branch silently widens the accepted-type check

**File:** `internal/attachments/service.go:301-316`

**Issue:** `allowedExt` is documented as matching the configured allow-list by extension, but the loop also returns `true` when `mt.Is(a)` for any allow-list entry `a`. The config is "normally extensions" (e.g. `pdf`, `png`), and `mimetype.MIME.Is` compares against MIME strings/aliases — so for extension-style entries `mt.Is("pdf")` is harmless (always false). However, if an operator ever adds a MIME-style entry (e.g. `image/*` is not supported, but `application/zip`), this branch accepts the type while returning the *sniffed* extension, which may not be a type the rest of the system expects (extractable set, inline-image set, download disposition all key off ext/mime independently). The dual-mode matching is an undocumented footgun that can desync the allow-list from the extraction/preview logic.

**Fix:** Pick one allow-list semantics. If the list is extensions, drop the `mt.Is(a)` branch entirely; if MIME strings are to be supported, normalize the config once at load and match explicitly, returning a canonical extension you control rather than `mt.Extension()`:

```go
for _, a := range s.allowedExtensions {
    if strings.EqualFold(strings.TrimPrefix(a, "."), ext) {
        return ext, true
    }
}
return ext, false
```

### WR-06: Any authenticated user can download any attachment by id regardless of page (no per-resource scoping)

**File:** `internal/server/handlers_attachments.go:243-299` (download), `215-233` (list); `internal/server/router.go:117`

**Issue:** The download and list reads are mounted in the authed group with no check that the requesting user has any relationship to the attachment's page. Any logged-in user who learns/guesses an attachment id (ULIDs are sortable and time-ordered, so enumeration of recently-uploaded ids is feasible) can fetch the byte-exact original. For this project the page-read model is "any authenticated user reads any page," so this is *consistent* with the existing authorization model and therefore not a BLOCKER — but it should be a conscious decision, not an accident, because attachments often carry more sensitive payloads than page text, and the time-ordered id makes targeted guessing easier than page-path guessing.

**Fix:** Confirm the "all authenticated users may read all attachments" decision is intended and documented (it matches page reads). If per-page or per-role attachment confidentiality is ever required, gate `handleDownloadAttachment`/`handleListAttachments` on the same authority used for the owning page. No code change if the shared-read model is accepted; record the decision in the phase notes.

## Info

### IN-01: `ResolveBin` doc comment describes a different function

**File:** `internal/attachments/service.go:286-295`

**Issue:** The comment says "Open resolves and reads the byte-exact binary ... returns the raw bytes," but `ResolveBin` only resolves and returns the absolute path string — it reads nothing.

**Fix:** Update the comment to describe what `ResolveBin` actually returns (a resolved absolute path for `http.ServeContent`).

### IN-02: Duplicated extractable-type and inline-image lists across server and client with no shared source

**File:** `internal/attachments/service.go:49`, `internal/attachments/extract.go:24-31`, `internal/server/handlers_attachments.go:28-31`, `web/src/api/client.ts:468-475`, `web/src/components/attachments/AttachmentCard.tsx:22-26,50-54`

**Issue:** The set of extractable types, the inline-image set, and the previewable-image set are each hand-maintained in multiple places (Go service vs Go handler vs TS client vs TS card). Drift between them produces subtle bugs (a type that previews but shows no chip, or a chip with no extraction). This is acknowledged inline but remains a maintenance hazard.

**Fix:** Centralize each list once per layer (one Go `var`, one TS `const`) and reference it everywhere; ideally expose the server allow-list/extractable set to the client via the config/health payload instead of re-declaring it (the dropzone already takes `allowedTypes` as a prop — extend that pattern).

### IN-03: `maxUploadMB` and `allowedTypes` are hard-coded in the page view rather than sourced from config

**File:** `web/src/routes/PageView.tsx:109-110`

**Issue:** `maxUploadMB={100}` and `allowedTypes={["pdf","docx","txt","png","jpg","svg"]}` are literals. If the server's `config.Storage.MaxUploadMB` or `config.Attachments.AllowedExtensions` differ, the client pre-check and hint text mislead the user (the server remains the real boundary, so this is UX-only). Magic values.

**Fix:** Fetch the upload limit and allowed types from a config/me/health endpoint and thread them into `AttachmentsSection` so client and server agree.

### IN-04: `humanFileSize` caps the unit at TB and shows wrong magnitude beyond PB

**File:** `web/src/api/client.ts:435-447`

**Issue:** The `units` array stops at `TB`; a value ≥ 1000 TB renders as a large "… TB" number rather than PB. Not reachable under the 100 MB cap, purely cosmetic.

**Fix:** Either document the bound or extend `units` with `PB`/`EB`. Low priority.

### IN-05: Status set to terminal before the `.txt` artifact is durably committed

**File:** `internal/attachments/extractjob.go:104-139`

**Issue:** `setExtractStatus(... done/empty ...)` is written after the commit-wait returns, but because the wait soft-succeeds on timeout (and, per CR-01, always does), the row can read `done` while the `.txt` is still only queued. The SSE chip then says "Text extracted" before the sidecar exists on disk. Self-resolving once CR-01 is fixed (fire-and-forget enqueue makes the commit durable-by-queue), but worth verifying the ordering after the fix.

**Fix:** After switching to `Enqueue`, set the status only once the enqueue succeeds (the commit is durable in the job queue), and add a test asserting the row never reports a terminal status without a corresponding queued/landed `.txt` commit.

---

_Reviewed: 2026-06-21T00:21:14Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
