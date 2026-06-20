---
phase: 02-attachments-text-extraction
plan: 01
subsystem: api
tags: [attachments, multipart-upload, mimetype, ulid, http-servecontent, react-dropzone, sqlite, git-single-writer]

# Dependency graph
requires:
  - phase: 01 (pages vertical slices)
    provides: single-writer CommitJob (pages.KindCommit + commitPayload), jobs.Worker, repo safe-path resolver (SEC-01), editor RBAC gate, audit logger, React PageView + api/client.ts
provides:
  - internal/attachments package (types, ULID id + path helpers, meta sidecar, Upload service, orphan-ref scan)
  - byte-exact attachment upload (multipart + MaxBytesReader + MIME-sniff allow-list) and download (http.ServeContent, SEC-02 disposition)
  - attachments operational table (0006_attachments.sql) + extraction-status column
  - canonical attachments.DownloadRefPath(id) link contract (fixed for 02-04 insert + orphan scan)
  - minimal editor-gated Attachments UI (dropzone + filename card + download) mounted in PageView read mode
  - extract fixtures (text-layer/scanned/corrupt PDF, docx, BOM+CRLF txt, png)
affects: [02-02 image preview/card detail, 02-03 text extraction + SSE status, 02-04 replace/remove/orphan-delete]

# Tech tracking
tech-stack:
  added:
    - github.com/gabriel-vasile/mimetype v1.4.13 (magic-byte MIME sniff)
    - github.com/oklog/ulid/v2 v2.1.1 (opaque attachment id)
    - github.com/ledongthuc/pdf v0.0.0-20250511090121-5959a4027728 (pinned for 02-03 extraction)
    - github.com/fumiama/go-docx v0.0.0-20250506085032-0c30fd09304b (pinned for 02-03 extraction)
    - react-dropzone 15.0.0 (frontend)
  patterns:
    - "Attachments reuse the ONE pages.KindCommit handler via a byte-identical local commitPayload mirror (no second write path)"
    - "Single GET /attachments/* catch-all dispatches both per-page list and {id}/download (avoids chi sibling-wildcard conflict)"
    - "MaxBytesReader before ParseMultipartForm is the real DoS cap (ParseMultipartForm's maxMemory only spools)"
    - "Disposition decided by STORED sniffed type, never the request (SEC-02); nosniff always set"

key-files:
  created:
    - internal/attachments/types.go
    - internal/attachments/id.go
    - internal/attachments/meta.go
    - internal/attachments/service.go
    - internal/attachments/refs.go
    - internal/attachments/service_test.go
    - internal/store/migrations/0006_attachments.sql
    - internal/server/handlers_attachments.go
    - internal/server/handlers_attachments_test.go
    - web/src/components/attachments/AttachmentsSection.tsx
    - web/src/components/attachments/AttachmentDropzone.tsx
    - web/src/components/attachments/AttachmentCard.tsx
    - testdata/attachments/{text-layer,scanned-image,corrupt}.pdf, sample.docx, sample.txt, pixel.png
  modified:
    - internal/config/config.go (none — MaxUploadMB read from Storage, AllowedExtensions already present)
    - internal/audit/audit.go (ActionAttachUpload/Replace/Delete)
    - internal/server/router.go (routes + Deps.Attachments)
    - internal/server/handlers_auth.go (authHandlers.attachments)
    - cmd/okf-workspace/main.go (attachments.NewService wiring)
    - web/src/api/client.ts (AttachmentMeta + upload/list/download helpers)
    - web/src/routes/PageView.tsx (mount AttachmentsSection)

key-decisions:
  - "Read MaxUploadMB from config.Storage (not duplicated on AttachmentsConfig) per the plan's interface contract"
  - "ULID (not content-hash) id so Replace (02-04) can reuse the same id when bytes change"
  - "commitPayload/fileWrite are a local mirror of pages' unexported types, guarded by a JSON-shape test (TestCommitPayloadShape) — keeps packages decoupled while reusing one handler"
  - "GET /attachments/* single catch-all with /download suffix dispatch (chi cannot host {id}/download next to a slash-bearing list wildcard)"

patterns-established:
  - "Pattern: attachment writes flow through EnqueueAndWait on KindCommit with the soft-timeout policy copied from pages (log+succeed on ErrJobTimeout)"
  - "Pattern: hidden-Git copywriting — commit message 'Add attachment' never surfaces; UI carries zero Git vocabulary"

requirements-completed: [ATT-01, ATT-02, ATT-09, ATT-10, SEC-02]

# Metrics
duration: 35min
completed: 2026-06-21
---

# Phase 2 Plan 01: Attachment Upload + Byte-Exact Download Slice Summary

**End-to-end attachment slice: editor drag-drops a file → server caps + MIME-sniffs it, assigns a ULID, writes binary + JSON meta sidecar through the existing single-writer CommitJob in one commit, and the file downloads byte-for-byte with the SEC-02 disposition — plus the internal/attachments foundation (types/id/meta/refs/service) for slices 02-02/03/04.**

## Performance

- **Duration:** ~35 min
- **Completed:** 2026-06-21
- **Tasks:** 3 (all TDD)
- **Files modified/created:** ~25

## Accomplishments
- Byte-exact upload→commit→download stack proven by a real-router e2e test (download bytes == upload bytes, ATT-02).
- ATT-09 security boundary in place from task one: MaxBytesReader hard cap (413) + magic-byte MIME-sniff allow-list (415) — the filename is never trusted.
- SEC-02 download safety: non-images served `Content-Disposition: attachment`, images (png/jpg/svg) inline, `X-Content-Type-Options: nosniff` always.
- ATT-10 single-writer invariant: every upload produces exactly one pages.KindCommit payload (binary + meta), reusing the one registered handler — no os.WriteFile/git anywhere.
- internal/attachments foundation + attachments table + four pinned pure-Go deps + extract fixtures laid for the next three slices, with the canonical DownloadRefPath link contract fixed now.
- Minimal editor-gated Attachments UI (dropzone with client size/type pre-check, filename card, Download) mounted in PageView read mode with zero Git vocabulary.

## Task Commits

Each task was committed atomically (all TDD; RED test landed first, GREEN implementation after):

1. **Task 1: Pin deps + fixtures + failing e2e tests** - `6de924a` (test) — RED
2. **Task 2: attachments package foundation (config/migration/types/id/meta/refs/service)** - `3740fc7` (feat)
3. **Task 3: HTTP upload + byte-exact download handlers, router/main wiring, minimal UI** - `e25e0b4` (feat) — turned the e2e tests GREEN

## Files Created/Modified
See frontmatter `key-files`. Highlights:
- `internal/attachments/service.go` — Upload orchestration (cap, sniff, ULID, sha256, one-commit write, row insert), List, Meta, ResolveBin.
- `internal/server/handlers_attachments.go` — upload (MaxBytesReader-before-parse), byte-exact download (ServeContent + SEC-02 disposition), list dispatch.
- `internal/store/migrations/0006_attachments.sql` — operational attachments table + page_path index.
- `web/src/components/attachments/*` — section/dropzone/card, token-only CSS.

## Decisions Made
See frontmatter `key-decisions`. No architectural (Rule 4) changes were required.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Installed react-dropzone (locked frontend dep, not yet in package.json)**
- **Found during:** Task 3 (dropzone component)
- **Issue:** react-dropzone is a CLAUDE.md-locked dependency (15.0.0) but was absent from web/package.json, so the dropzone import would not resolve.
- **Fix:** `npm install react-dropzone@15.0.0`. This is a pre-approved, audited package (CLAUDE.md locked table), not a slopsquat-risk install, so no package-legitimacy checkpoint was warranted.
- **Files modified:** web/package.json, web/package-lock.json
- **Verification:** `npm run build` (tsc + vite) succeeds; ESLint clean.
- **Committed in:** `e25e0b4` (Task 3 commit)

---

**Total deviations:** 1 auto-fixed (1 blocking).
**Impact on plan:** Necessary to complete the planned UI task; the dep was already specified in the locked stack. No scope creep.

## Issues Encountered
- `go mod tidy` initially demoted/removed the four new deps because no source imported them yet during the RED state. Resolved by re-running `go get` so the pseudo-versions stay pinned in go.sum (mimetype/ulid became direct once Task 2 imported them; pdf/docx remain pinned indirect until 02-03 imports the extractors). This matches the CLAUDE.md eino pseudo-version guidance.
- Verification greps for `os.WriteFile`/`exec.Command` and frontend Git vocabulary return only comment/doc lines (the single-writer rule description and the hidden-Git rule comment) — no actual calls and no rendered copy, consistent with the plan's "matches/comments only" allowance.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- 02-02 (image preview/card detail): AttachmentMeta + list endpoint + download URL ready; AttachmentCard is intentionally minimal and ready to extend with thumbnail/meta-line.
- 02-03 (text extraction + SSE): pdf/docx deps pinned, fixtures (text-layer/scanned/corrupt/docx/BOM-txt) checked in, extract_status column + ExtractionStatus consts present, TxtPath helper defined.
- 02-04 (replace/remove/orphan-delete): canonical DownloadRefPath + PageReferences scan defined now so insert/scan can't drift; commit spine supports Removes in the same payload.

## Self-Check: PASSED

---
*Phase: 02-attachments-text-extraction*
*Completed: 2026-06-21*
