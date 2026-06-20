---
phase: 02-attachments-text-extraction
plan: 03
subsystem: api
tags: [attachments, text-extraction, pdf, docx, sse, eventsource, jobs-worker, panic-recover, hidden-git]

# Dependency graph
requires:
  - phase: 02 (plan 01)
    provides: internal/attachments service (Upload/List/Meta), ULID id + BinPath/MetaPath/TxtPath helpers, attachments operational table with extract_status/extract_error columns, the local commitPayload mirror reusing the ONE pages.KindCommit handler, pinned (then-unused) ledongthuc/pdf + fumiama/go-docx deps, extract fixtures (text-layer/scanned-image/corrupt PDF, sample.docx, BOM+CRLF sample.txt, pixel.png)
  - phase: 02 (plan 02)
    provides: full AttachmentCard (media square + filename + meta line) with a reserved meta stack, isPreviewableImage helper, AttachmentMeta.extraction_status on the list item
  - phase: 01
    provides: jobs.Worker single-drain spine (Register/Enqueue/EnqueueAndWait), pages.KindCommit + commitPayload, repo safe-path resolver (SEC-01), authed route group + RBAC, AutosaveStatus chip + .autosave-status CSS pattern
provides:
  - pure-Go text extractors (extractPDF/extractDOCX/extractTXT) behind Extract(ext, data) with a guardExtract panic-recover chokepoint (Pitfall 5 / T-02-09)
  - KindExtract job handler (separate kind on the same worker) that reads the committed binary and commits the (possibly empty) <id>.txt via the ONE KindCommit path (ATT-10, T-02-11)
  - empty-but-succeeded extraction model: status=empty + empty .txt distinct from status=failed + no .txt (ATT-08)
  - status.go: ExtractionStatusFor / setExtractStatus / IsTerminalStatus over the extract_status column (survives job-row pruning)
  - Upload fire-and-forget enqueue of KindExtract for pdf/docx/txt only (Enqueue, not EnqueueAndWait)
  - GET /api/v1/attachments/{id}/status text/event-stream endpoint (authed, read-only)
  - client.ts subscribeExtractionStatus(id, onStatus) over native EventSource
  - ExtractionStatus chip (4 states) mirroring AutosaveStatus, wired into AttachmentCard for extractable types only
affects: [02-04 replace/remove/orphan-delete (re-runs extraction on replace; removes the .txt alongside binary+meta)]

# Tech tracking
tech-stack:
  added:
    - github.com/ledongthuc/pdf v0.0.0-20250511090121-5959a4027728 (was pinned-unused in 02-01; now a direct dep — go mod tidy)
    - github.com/fumiama/go-docx v0.0.0-20250506085032-0c30fd09304b (was pinned-unused in 02-01; now a direct dep)
  patterns:
    - "Every third-party parser call flows through guardExtract, a single recover() chokepoint, so an adversarial file panic becomes a returned error and the single drain goroutine survives (deterministically unit-tested by passing a panicking fn, not by hunting a panic fixture)"
    - "ExtractJob is a SEPARATE job kind (KindExtract) but writes its .txt by enqueuing a normal KindCommit job — no second write path (ATT-10)"
    - "Extraction READS the binary path and WRITES only the .txt path (T-02-11) — verified by grep: BinPath only in r.Read, TxtPath only in Writes"
    - "SSE extraction-status stream is dispatched on the existing /attachments/* catch-all (suffix '/status'), same trick as '/download', to avoid the chi sibling-wildcard conflict a real {id}/status route would hit"
    - "extract_status column (not the jobs table) is the SSE source of truth — survives job-row pruning and keeps the handler trivial; the DB pending value is mapped to the wire term 'extracting'"
    - "ExtractionStatus chip copies AutosaveStatus verbatim (aria-live, Loader2 size 14, .autosave-status structure) and adds only warning/destructive color states + the No-text-extracted sub-note"

key-files:
  created:
    - internal/attachments/extract.go
    - internal/attachments/extract_test.go
    - internal/attachments/extractjob.go
    - internal/attachments/extractjob_test.go
    - internal/attachments/status.go
    - internal/server/handlers_sse.go
    - internal/server/handlers_sse_test.go
    - web/src/components/attachments/ExtractionStatus.tsx
    - web/src/components/attachments/ExtractionStatus.css
  modified:
    - internal/attachments/service.go (extractText flag, isExtractable, Upload fire-and-forget enqueue, ExtractionStatus wrapper)
    - internal/attachments/service_test.go (fakeEnqueuer records kinds + only applies commit-kind; countKind helper)
    - internal/server/handlers_attachments.go (dispatch '/status' to handleExtractionStatus)
    - internal/server/handlers_attachments_test.go (register KindExtract; expose attach service + db on the fixture)
    - internal/server/router.go (comment: '/status' dispatched on the catch-all)
    - cmd/okf-workspace/main.go (register attachments.KindExtract handler)
    - web/src/api/client.ts (ExtractionStatusValue + subscribeExtractionStatus)
    - web/src/components/attachments/AttachmentCard.tsx (chip for extractable types, SSE subscription)
    - web/src/components/attachments/AttachmentCard.css (.attachment-card-extract)
    - go.mod (ledongthuc/pdf + fumiama/go-docx now direct)

key-decisions:
  - "Panic-recover is tested deterministically by exercising the guardExtract chokepoint with a deliberately panicking fn — the pinned parsers return clean errors (not panics) on every malformed fixture tried (header-only/trailer-garbage PDF, zip-magic/valid-zip-malformed-xml DOCX), so a fixture-based panic test would be brittle across parser versions"
  - "ExtractJob uses a separate KindExtract kind (does the work) but commits the .txt by enqueuing KindCommit — keeps the single registered commit handler and the single-writer invariant intact"
  - "SSE dispatched on the /attachments/* catch-all (suffix match) rather than a new chi route, because {id}/status next to the slash-bearing list wildcard hits the same sibling-wildcard conflict 02-01 already worked around for /download"
  - "newService defaults extractText=true so existing in-package tests exercise the enqueue path; NewService threads the real config.Attachments.ExtractText flag"
  - "No global http.Server WriteTimeout exists (main.go), so Pitfall 7 needs no active exemption now — documented in the handler so a future timeout addition exempts this route"

patterns-established:
  - "Pattern: guardExtract single recover() chokepoint for all third-party parsers + a second defense-in-depth recover() around the whole job handler"
  - "Pattern: hidden-Git copywriting continues — the chip says 'Extracting text…/Text extracted/No text extracted/Couldn't extract text' with zero Git/processing-internals vocabulary"

requirements-completed: [ATT-08]

# Metrics
duration: ~40min
completed: 2026-06-21
---

# Phase 2 Plan 03: Async Text Extraction + SSE Status Summary

**Async PDF/DOCX/TXT text extraction (ATT-08): a new `KindExtract` job on the existing single jobs worker reads the committed binary with pure-Go parsers (`ledongthuc/pdf` / `fumiama/go-docx` / stdlib), recovers any parser panic so one bad file can't kill the drain goroutine, and commits the (possibly empty) `<id>.txt` sidecar through the one `KindCommit` path — surfaced live to the card via an SSE endpoint and an `ExtractionStatus` chip that distinguishes the non-alarming empty-but-succeeded "No text extracted" case (scanned PDF → `status=empty` + empty `.txt`) from a genuine "Couldn't extract text" failure (`status=failed` + no `.txt`).**

## Performance
- **Duration:** ~40 min
- **Completed:** 2026-06-21
- **Tasks:** 3 (all TDD)
- **Files created/modified:** 19

## Accomplishments
- **ATT-08 extractors (pure-Go, CGO_ENABLED=0):** `Extract(ext, data)` dispatches to `extractPDF` (`pdf.NewReader` + `GetPlainText`), `extractDOCX` (`docx.Parse` + Body.Items paragraph/table `.String()`), and `extractTXT` (UTF-8 BOM strip, CRLF→LF, TrimSpace). All flow through `guardExtract`, a single `recover()` chokepoint, and all TrimSpace so a whitespace-only doc reads as empty.
- **The empty guarantee (ATT-08):** a scanned/image PDF extracts `""` with NO error → an empty `<id>.txt` is committed and the row is set to `status=empty`, which renders the amber "No text extracted" chip — explicitly distinct from a corrupt file (`status=failed`, no `.txt`).
- **Panic safety (Pitfall 5 / T-02-09):** the recover guard is proven deterministically by passing a deliberately panicking fn through `guardExtract` (the pinned parsers return clean errors, not panics, on every malformed fixture tried), plus a second defense-in-depth `recover()` wraps the whole job handler.
- **KindExtract job (ATT-10 / T-02-11):** a separate job kind that READS the binary and WRITES only the `.txt` by enqueuing a normal `KindCommit` — the binary is never re-written (grep-verified), and there is no second commit/write path.
- **Status spine:** `status.go` (`ExtractionStatusFor` / `setExtractStatus` / `IsTerminalStatus`) reads/writes the `extract_status` column (the SSE source of truth, surviving job-row pruning); the raw parser error is stored server-side in `extract_error` and NEVER sent to the client (T-02-12).
- **Live SSE + chip:** `GET /api/v1/attachments/{id}/status` streams `text/event-stream` (auth-gated, read-only), closing on a terminal status or client disconnect; `subscribeExtractionStatus` (native `EventSource`) feeds the `ExtractionStatus` chip, which is rendered ONLY for pdf/docx/txt — images/other types show no chip.

## Task Commits
1. **Task 1: pure-Go PDF/DOCX/TXT extractors with panic-recover guard** — `68b3a5b` (feat, TDD)
2. **Task 2: KindExtract job handler, status column + enqueue-after-upload** — `3c3db95` (feat, TDD)
3. **Task 3: SSE extraction-status endpoint + ExtractionStatus chip on the card** — `6437036` (feat, TDD)

## Files Created/Modified
See frontmatter `key-files`. Highlights:
- `internal/attachments/extract.go` — `Extract` + `guardExtract` + the three pure extractors.
- `internal/attachments/extractjob.go` — `KindExtract` handler (read binary → extract → commit `.txt` via `KindCommit` → set status); `extractPayload`; `binaryReader` seam.
- `internal/attachments/status.go` — extraction-status read/write over the operational column.
- `internal/server/handlers_sse.go` — the `text/event-stream` extraction-status handler.
- `web/src/components/attachments/ExtractionStatus.tsx` / `.css` — the 4-state chip (AutosaveStatus clone + warning/destructive states + sub-note).
- `web/src/api/client.ts` — `subscribeExtractionStatus` over `EventSource`.

## Decisions Made
See frontmatter `key-decisions`. No architectural (Rule 4) changes were required.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Panic test exercises the guardExtract chokepoint directly instead of a panic fixture**
- **Found during:** Task 1 (TestExtractPanicRecovered)
- **Issue:** The plan's behavior lists "a deliberately malformed input that panics inside the parser". Probing the pinned parsers with several malformed inputs (header-only PDF, trailer-garbage PDF, zip-magic DOCX, valid-zip-but-malformed-XML DOCX) showed they return clean *errors*, not panics — so no fixture reliably triggers a parser panic on these versions.
- **Fix:** Refactored all three extractors to flow through a single `guardExtract(kind, fn)` recover() chokepoint, and the panic test passes a deliberately panicking `fn` through it. This proves the Pitfall-5 guarantee deterministically and version-independently (every extractor uses the same guard), which is strictly stronger than a fixture that may or may not panic.
- **Files modified:** internal/attachments/extract.go, internal/attachments/extract_test.go
- **Committed in:** `68b3a5b` (Task 1)

**2. [Rule 3 - Blocking] ExtractHandler constructor takes a `binaryReader` + `pushOnCommit`, not `(*repo.Repo, *jobs.Worker, *gitstore.GitStore)`**
- **Found during:** Task 2 (ExtractHandler signature)
- **Issue:** The RESEARCH Pattern-6 sketch passed a `*gitstore.GitStore`, but extraction never commits directly — it enqueues a `KindCommit` job (single-writer invariant). Passing the GitStore would invite a second write path.
- **Fix:** `ExtractHandler(r binaryReader, w enqueuer, db *sql.DB, pushOnCommit bool)`: it reads via the `binaryReader` (repo) and commits the `.txt` via the `enqueuer` (worker) using `KindCommit`. The `binaryReader`/`enqueuer` interfaces keep the handler unit-testable without standing up a real worker/gitstore.
- **Files modified:** internal/attachments/extractjob.go, cmd/okf-workspace/main.go
- **Committed in:** `3c3db95` (Task 2)

**3. [Rule 3 - Blocking] fakeEnqueuer extended to record job kinds and skip-apply non-commit payloads**
- **Found during:** Task 2 (TestUploadEnqueuesExtract)
- **Issue:** The existing `fakeEnqueuer.Enqueue` unconditionally unmarshalled every payload as a `commitPayload`; the new `KindExtract` payload is a different shape, so it would fail to apply.
- **Fix:** `Enqueue`/`EnqueueAndWait` now record the `kind`; only `kindCommit` payloads are applied to the repo, others are recorded fire-and-forget. Added a `countKind` helper. Existing `TestUploadCommits` still passes (it counts applied commit payloads, which excludes the fire-and-forget extract enqueue).
- **Files modified:** internal/attachments/service_test.go
- **Committed in:** `3c3db95` (Task 2)

**Total deviations:** 3 auto-fixed (all Rule 3 blocking, all signature/test-harness adaptations to fit the existing single-writer + interface-seam patterns). No scope creep; no architectural changes.

## Known Stubs
None. The chip is driven entirely by the live `extract_status` column over SSE (seeded from the list item); no hardcoded/placeholder content. Replace-triggered re-extraction and orphan-delete of the `.txt` are intentionally deferred to 02-04 per the phase plan.

## Threat Surface
No new surface beyond the plan's threat_model. The SSE route is read-only under the authed group (T-02-13); extraction reads a copy of the bytes and writes only the `.txt` (T-02-11, grep-verified); the raw parser error stays server-side (T-02-12); the drain goroutine is panic-protected (T-02-09). No global write timeout exists, so the SSE stream is not killed (T-02-10) — documented for any future timeout addition.

## Verification
- `CGO_ENABLED=0 go test ./internal/attachments/ -count=1` → ok (6 extractor fidelity tests incl. empty + panic; 4 ExtractJob tests: writes-txt / empty-is-success / parse-error-fails / upload-enqueues-only-extractable).
- ATT-08 empty guarantee: `TestExtractScannedPDFEmpty` (`""`, no error) + `TestExtractJobEmptyIsSuccess` (`status=empty`, empty `.txt`) prove empty-but-succeeded ≠ failed.
- `go test ./internal/server/ -run TestExtractionSSE -count=1` → ok (stream emits a terminal `data:` event + closes; unauthenticated request rejected before streaming).
- Binary never mutated: `grep -n "BinPath\|TxtPath" internal/attachments/extractjob.go` → `BinPath` only in `r.Read`, `TxtPath` only in `Writes`.
- `CGO_ENABLED=0 go test ./... -count=1` → all packages green; `go build ./...` clean.
- `cd web && npm run build && npx tsc --noEmit` → both pass.
- Hidden-Git: vocab grep on `ExtractionStatus.tsx` + `client.ts` → `NO-GIT-VOCAB`.
- `go mod tidy`: ledongthuc/pdf + fumiama/go-docx are now direct deps (go.mod committed; go.sum unchanged — already pinned in 02-01).

## User Setup Required
None — extraction runs automatically on upload when `attachments.extract_text` is enabled in config (default behavior); no external service or configuration changes.

## Next Phase Readiness
- 02-04 (replace/remove/orphan-delete): `TxtPath(id)` is the third artifact to `Removes` alongside `BinPath`/`MetaPath` in the orphan-delete commit; Replace should re-enqueue `KindExtract` (reuse `Service.enqueueExtract`) after the new bytes commit; the `extract_status` row resets to pending on replace so the chip transitions live again.

## Self-Check: PASSED
All created files exist on disk and all three task commits (`68b3a5b`, `3c3db95`, `6437036`) are present in git history.

---
*Phase: 02-attachments-text-extraction*
*Completed: 2026-06-21*
