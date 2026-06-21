---
phase: 02-attachments-text-extraction
verified: 2026-06-21T00:16:00Z
status: human_needed
score: 11/11
overrides_applied: 0
human_verification:
  - test: "Drag a file into a page (PDF, DOCX, TXT, PNG, JPG, SVG) using a real browser and confirm the attachment card appears with the correct name, size, uploader, and date."
    expected: "Card renders immediately with all four fields. PDF/DOCX/TXT show an extraction chip that transitions extracting → done or 'No text extracted'. PNG/JPG/SVG show a thumbnail."
    why_human: "react-dropzone drag behaviour, live SSE chip transitions, and card layout cannot be verified without a running browser session."
  - test: "Click the thumbnail on a PNG or JPG attachment card in a browser and confirm the full-size image preview dialog opens. Then test with an SVG — the thumbnail should render via <img> but clicking 'Download' should trigger a file save (not an inline navigation)."
    expected: "PNG/JPG: dialog opens with full-size <img>. SVG: thumbnail renders (browser ignores Content-Disposition for img subresource); Download button triggers a file download (application/octet-stream + attachment disposition). Direct navigation to the SVG URL also triggers a file download, not a rendered SVG document."
    why_human: "Browser rendering behaviour for img subresource vs direct navigation of SVG cannot be asserted with grep or unit tests."
  - test: "Replace an attachment (click the Replace icon on the card, choose a new file) and verify the card updates with the new name/size and the extraction chip resets to 'Extracting text…' for a PDF/DOCX/TXT."
    expected: "Same attachment ID in the URL, card name/size reflects the new file, extraction chip goes pending then settles to done/empty. The prior bytes are retained in history (not observable in UI — confirmed by code path)."
    why_human: "UI state transitions and live SSE re-subscription after replace require a real browser."
  - test: "Remove an attachment from a page. If it is the only page referencing it, verify the files are deleted (the card disappears and a subsequent download returns 404). If another page also references it, verify only the link is removed from this page and the card on the other page remains."
    expected: "Last-reference case: card disappears, download 404s. Shared case: card survives on the other page."
    why_human: "Multi-page orphan detection and cross-page card state require manual testing with a real server."
---

# Phase 2: Attachments & Text Extraction — Verification Report

**Phase Goal:** A user can attach original files to pages, download them byte-for-byte unchanged, manage them safely (replace, unlink, auto-delete orphans), and have their text extracted (PDF/DOCX/TXT) so search and the agent can read them. Requirements ATT-01..10, SEC-02.
**Verified:** 2026-06-21T00:16:00Z
**Status:** human_needed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | User can upload a file to a page (ATT-01) | VERIFIED | `handleUploadAttachment` + `Service.Upload` + `AttachmentDropzone`; `TestUploadCommits` PASS |
| 2 | Download returns byte-exact original (ATT-02) | VERIFIED | `TestDownloadByteExact` PASS: `drec.Body.Len() == len(original)` asserted; `http.ServeContent` streams without transcoding |
| 3 | Attachment card shows name, size, uploader, date (ATT-03) | VERIFIED | `AttachmentCard.tsx` renders `meta.original_name`, `humanFileSize(meta.size_bytes)`, `meta.uploader_name`, `humanDate(meta.uploaded_at)` from live react-query data |
| 4 | PNG/JPG inline preview; SVG forced-download + img-preview (ATT-04) | VERIFIED | `inlineImageTypes = {image/png, image/jpeg}` in handler; `TestInlineImageDisposition` PASS (png/jpg inline, SVG → 415 application/octet-stream + attachment); `ImagePreviewDialog` uses `<img src>` only, no `dangerouslySetInnerHTML` |
| 5 | Replace attachment (same id, new bytes, re-extract) (ATT-05) | VERIFIED | `Service.Replace` reuses id, writes new bin+meta in ONE commit via `KindCommit`, resets `extract_status` to pending and re-enqueues `KindExtract`; `TestReplaceKeepsID` PASS |
| 6 | Remove attachment link from page (ATT-06) | VERIFIED | `Service.Remove` → `unlinkPage` strips `DownloadRefPath(id)` from page Markdown and commits via `KindCommit`; `TestOrphanDelete` + `TestRemoveKeepsSharedFile` PASS |
| 7 | Orphan auto-delete when no page references (ATT-07) | VERIFIED | `Service.Remove` → `PageReferences` → removes-only commitPayload with `[bin, meta, txt]` in ONE commit; `TestOrphanDelete` asserts `len(Removes)==3` and single commit; `TestRemoveKeepsSharedFile` PASS |
| 8 | Text extracted from PDF/DOCX/TXT; "No text extracted" for scanned PDF (ATT-08) | VERIFIED | `extractPDF/extractDOCX/extractTXT` all pass; `TestExtractScannedPDFEmpty` → `("", nil)` → `status=empty`; `TestExtractJobEmptyIsSuccess` PASS; ExtractionStatus chip has 4 states including "No text extracted" |
| 9 | Uploads MIME-sniffed against allow-list; size limit enforced (ATT-09) | VERIFIED | `Service.Upload` calls `mimetype.Detect(data)` (magic bytes, not extension); `MaxBytesReader` before `ParseMultipartForm`; `TestUploadValidation` PASS (oversize → 413, ELF → 415) |
| 10 | Upload/delete auto-commit via single-writer KindCommit (ATT-10) | VERIFIED | No `os.WriteFile`/`exec.Command` in `internal/attachments/` or handlers (grep returns only comments); every write routes through `enqueueCommit` → `worker.EnqueueAndWait(kindCommit, ...)` |
| 11 | Risky downloads served Content-Disposition: attachment + CSP sandbox (SEC-02) | VERIFIED | `handleDownloadAttachment` always sets `X-Content-Type-Options: nosniff` + `Content-Security-Policy: default-src 'none'; sandbox`; non-image types (including SVG) → `Content-Disposition: attachment`; `TestDownloadDisposition` PASS |

**Score:** 11/11 truths verified

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/attachments/service.go` | Upload, List, Meta, ResolveBin, ExtractionStatus | VERIFIED | 360 lines; all methods implemented with real logic; no stubs |
| `internal/attachments/extract.go` | PDF/DOCX/TXT extractors + guardExtract | VERIFIED | `Extract`, `guardExtract`, `extractPDF`, `extractDOCX`, `extractTXT` all present and substantive |
| `internal/attachments/extractjob.go` | KindExtract job handler | VERIFIED | `ExtractHandler` registers `KindExtract` on worker; reads binary, writes .txt via `KindCommit` |
| `internal/attachments/lifecycle.go` | Replace, Remove, orphan delete | VERIFIED | `Replace`, `Remove`, `unlinkPage`, `stripAttachmentLinks`, `deleteRow` all implemented |
| `internal/attachments/refs.go` | PageReferences canonical-match scan | VERIFIED | Scans all pages for `DownloadRefPath(id)` substring |
| `internal/attachments/status.go` | ExtractionStatus read/write | VERIFIED | `ExtractionStatusFor`, `setExtractStatus`, `IsTerminalStatus` |
| `internal/attachments/id.go` | ULID id, BinPath/MetaPath/TxtPath/DownloadRefPath | VERIFIED | ULID-based, three sidecar helpers, single canonical DownloadRefPath |
| `internal/store/migrations/0006_attachments.sql` | attachments table with extract_status column | VERIFIED | Table with id, page_path, original_name, mime_type, size_bytes, uploader_name, uploaded_at, extract_status, extract_error |
| `internal/server/handlers_attachments.go` | Upload, download, list, replace, delete handlers | VERIFIED | 318 lines; all handlers implemented; SEC-02 disposition logic present |
| `internal/server/handlers_sse.go` | SSE extraction-status stream | VERIFIED | Polls DB on 500ms ticker, emits `{status}` events, closes on terminal status |
| `web/src/components/attachments/AttachmentCard.tsx` | Full card: media square, name, meta line, Download, chip, dialogs | VERIFIED | 196 lines; renders all required fields from live data; no stubs |
| `web/src/components/attachments/AttachmentsSection.tsx` | Mounts cards + dropzone; wired into PageView | VERIFIED | useQuery `["attachments", pagePath]` → `listAttachments(pagePath)`; mounted in PageView.tsx line 106 |
| `web/src/components/attachments/AttachmentDropzone.tsx` | Drag-drop with client-side pre-checks | VERIFIED | Client size check (`file.size > maxUploadMB * 1024 * 1024`) and rejection handler; POST to `/api/v1/attachments` |
| `web/src/components/attachments/ImagePreviewDialog.tsx` | SVG-safe inline preview via `<img src>` only | VERIFIED | Only `<img src={downloadAttachmentUrl(meta.id)}>` — no `dangerouslySetInnerHTML` |
| `web/src/components/attachments/ExtractionStatus.tsx` | 4-state chip (extracting/done/empty/failed) | VERIFIED | All four states including "No text extracted" amber sub-note |
| `web/src/components/attachments/ReplaceAttachmentDialog.tsx` | Editor-only replace dialog | VERIFIED | `confirmLabel="Replace file"`, "kept in history and can be restored" copy, no Git vocabulary |
| `web/src/components/attachments/RemoveAttachmentDialog.tsx` | Editor-only remove dialog | VERIFIED | `confirmLabel="Remove file"`, describes orphan-delete consequence |
| `testdata/attachments/` | Fixtures: text-layer/scanned/corrupt PDF, DOCX, TXT, PNG, JPG, SVG | VERIFIED | All 8 fixture files present |

### Key Link Verification

| From | To | Via | Status | Details |
|------|-----|-----|--------|---------|
| `AttachmentsSection` | `GET /api/v1/attachments/{pagePath}` | `listAttachments()` in react-query | VERIFIED | useQuery queryFn calls `listAttachments(pagePath)` which fetches `/api/v1/attachments/${pagePath}` |
| `AttachmentDropzone` | `POST /api/v1/attachments` | `uploadAttachment()` via useMutation | VERIFIED | mutationFn calls `uploadAttachment(pagePath, file)` which posts to `/api/v1/attachments` |
| `AttachmentCard` → thumbnail | `GET /api/v1/attachments/{id}/download` | `downloadAttachmentUrl(meta.id)` | VERIFIED | `<img src={downloadAttachmentUrl(meta.id)}>` |
| `AttachmentCard` SSE | `GET /api/v1/attachments/{id}/status` | `subscribeExtractionStatus(meta.id, ...)` via native EventSource | VERIFIED | `subscribeExtractionStatus` opens `EventSource` for extractable types; wired in `useEffect` |
| `Service.Upload` | `pages.KindCommit` handler | `worker.EnqueueAndWait("commit", ...)` | VERIFIED | `enqueueCommit` marshals `commitPayload` and calls `EnqueueAndWait`; `TestCommitPayloadShape` asserts JSON shape matches the registered handler |
| `ExtractHandler` | `KindCommit` handler (for .txt) | `w.EnqueueAndWait("commit", ...)` | VERIFIED | Extract handler commits `.txt` via `KindCommit`, never directly; `TestExtractJobWritesTxt` PASS |
| `Service.Remove` | `PageReferences` scan → orphan delete | `DownloadRefPath(id)` canonical string shared by insert and scan | VERIFIED | Both `refs.go` (scan) and `lifecycle.go` (edit) use `DownloadRefPath(id)`; `TestOrphanDelete` confirms |
| Router | attachment routes under editor subgroup | `RequireRole` middleware | VERIFIED | `PUT /attachments/*` and `DELETE /attachments/*` registered under `editor` subgroup in `router.go`; `TestReplaceRemoveEditorOnly` PASS (reader → 403) |
| `handleDownloadAttachment` | SEC-02 disposition | `isInlineImage(meta.MimeType)` | VERIFIED | Handler always sets `X-Content-Type-Options: nosniff` + CSP `sandbox`; `inlineImageTypes` excludes SVG; `TestInlineImageDisposition` and `TestDownloadDisposition` PASS |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|---------------|--------|--------------------|--------|
| `AttachmentsSection` | `attachments` from `useQuery` | `listAttachments(pagePath)` → `GET /api/v1/attachments/{pagePath}` → `Service.List()` → SQLite `attachments` table | Yes — DB query reads rows inserted at upload time | FLOWING |
| `AttachmentCard` | `meta` prop | Passed from `AttachmentsSection` items array (react-query data) | Yes — each field (`original_name`, `size_bytes`, `uploader_name`, `uploaded_at`, `extraction_status`) populated by `Service.insertRow` at upload | FLOWING |
| `ExtractionStatus` chip | `extractStatus` state | Seeded from `meta.extraction_status`; updated by `subscribeExtractionStatus` → SSE → `ExtractionStatusFor` → SQLite `extract_status` column | Yes — `KindExtract` job handler calls `setExtractStatus` with real status | FLOWING |
| `handleDownloadAttachment` | Binary bytes | `Service.ResolveBin(id, ext)` → `repo.Resolve(BinPath(id, ext))` → `os.Open(abs)` → `http.ServeContent` | Yes — reads the committed file from the Git working tree, never transcodes | FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Go build succeeds (CGO_ENABLED=0) | `CGO_ENABLED=0 go build ./...` | No output (success) | PASS |
| All Go tests pass | `CGO_ENABLED=0 go test ./... -count=1` | All 13 packages ok; 0 failures | PASS |
| Frontend build succeeds | `npm run build` (tsc -b + vite build) | `built in 371ms`, no errors | PASS |
| TypeScript type-check clean | `npx tsc --noEmit` | No output (success) | PASS |
| Byte-exact download | `go test ./internal/server/ -run TestDownloadByteExact` | PASS | PASS |
| SVG served as attachment | `go test ./internal/server/ -run TestInlineImageDisposition` | PASS (SVG → Content-Disposition: attachment + application/octet-stream) | PASS |
| Empty extraction state | `go test ./internal/attachments/ -run TestExtractScannedPDFEmpty` | PASS (`""`, no error — "No text extracted" path) | PASS |
| Orphan delete in one commit | `go test ./internal/attachments/ -run TestOrphanDelete` | PASS (`len(Removes)==3` in final commit payload) | PASS |
| Reader rejected from lifecycle routes | `go test ./internal/server/ -run TestReplaceRemoveEditorOnly` | PASS (reader → 403 on PUT and DELETE) | PASS |
| SSE stream closes on terminal status | `go test ./internal/server/ -run TestExtractionSSEStream` | PASS | PASS |
| SSE auth-gated | `go test ./internal/server/ -run TestExtractionSSEAuthRequired` | PASS | PASS |
| Single-writer invariant (no direct FS writes) | `grep -rn "os.WriteFile\|exec.Command" internal/attachments/` | Only comments — no actual calls | PASS |
| Hidden-Git: no Git vocab in attachment UI | grep on `web/src/components/attachments/` | Only "kept in history and can be restored" (CONTEXT-approved hidden-Git copy) | PASS |
| Raw parse error never sent to client | grep `handlers_sse.go`, `ExtractionStatus.tsx` for `extract_error` | No matches — `extract_error` is only stored in DB, never emitted | PASS |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|------------|-------------|--------|----------|
| ATT-01 | 02-01 | User can upload a file attachment to a page | SATISFIED | `handleUploadAttachment` + `Service.Upload` + `AttachmentDropzone`; `TestUploadCommits` PASS |
| ATT-02 | 02-01 | Download returns original, unmodified form | SATISFIED | `TestDownloadByteExact` PASS; `http.ServeContent` streams byte-exact |
| ATT-03 | 02-02 | Card with original name, size, uploader, date | SATISFIED | `AttachmentCard.tsx` renders all four fields from live API data |
| ATT-04 | 02-02 | Preview image attachments (PNG/JPG/SVG) inline | SATISFIED | PNG/JPG: inline via `<img>`; SVG: `<img>` thumbnail works (browser ignores `Content-Disposition` for subresources), server forces download for XSS safety; `TestInlineImageDisposition` PASS |
| ATT-05 | 02-04 | User can replace an attachment with a new version | SATISFIED | `Service.Replace` + `handleReplaceAttachment`; `TestReplaceKeepsID` PASS |
| ATT-06 | 02-04 | User can remove an attachment link from a page | SATISFIED | `Service.Remove` → `unlinkPage` + canonical `DownloadRefPath`; PASS |
| ATT-07 | 02-04 | System deletes attachment when no page references it | SATISFIED | `PageReferences` scan + removes-only commitPayload; `TestOrphanDelete` asserts 3-artifact single-commit delete |
| ATT-08 | 02-03 | Extract text from PDF/DOCX/TXT; "No text extracted" state | SATISFIED | All extractors pass; `TestExtractScannedPDFEmpty` + `TestExtractJobEmptyIsSuccess` PASS; chip distinguishes `empty` from `failed` |
| ATT-09 | 02-01 | Uploads validated: size limit + MIME-sniffed allowed type | SATISFIED | `MaxBytesReader` before parse; `mimetype.Detect` (magic bytes); `TestUploadValidation` PASS (413 + 415) |
| ATT-10 | 02-01, 02-04 | Auto-commit to Git on upload or delete | SATISFIED | All writes route through `worker.EnqueueAndWait("commit", ...)` via single `KindCommit` handler; no `os.WriteFile`/`exec.Command` in attachment code |
| SEC-02 | 02-01, 02-02 | Risky downloads served Content-Disposition: attachment | SATISFIED | `inlineImageTypes` excludes SVG; CSP `default-src 'none'; sandbox` + `nosniff` on every download; `TestDownloadDisposition` + `TestInlineImageDisposition` PASS |

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| `web/src/components/attachments/AttachmentCard.tsx` | 74 | `const Icon = typeIconFor(meta)` inside render — `react-hooks/static-components` ESLint finding | INFO | No runtime bug; ESLint finding pre-existing from 02-02; build/tsc pass; documented in `deferred-items.md` |

No TBD/FIXME/XXX/HACK markers found in any Phase 2 files. No stubs detected (all components render real API data; no hardcoded empty arrays or placeholder returns in wired paths).

### Human Verification Required

#### 1. File drag-drop upload + card render

**Test:** Open the app in a browser, navigate to a page, and drag a PDF into the attachment drop zone. Observe the card appear.
**Expected:** Card appears with the correct original filename, human-formatted size ("1.4 MB"), uploader name, and date. The extraction chip shows "Extracting text…" then transitions to "Text extracted" or "No text extracted" without a page refresh.
**Why human:** react-dropzone drag behaviour, live SSE chip state transitions, and the visual card layout cannot be verified programmatically.

#### 2. Image preview dialog (PNG/JPG) and SVG download safety

**Test:** Upload a PNG or JPG; click its thumbnail — the full-size preview dialog should open. Upload an SVG; click its thumbnail — the dialog also opens. Then use DevTools to directly navigate to the SVG's download URL (e.g., `/api/v1/attachments/{id}/download`).
**Expected:** PNG/JPG: dialog opens with full-size `<img>`. SVG: thumbnail renders (browser ignores `Content-Disposition` for `<img>` subresources). Direct SVG URL navigation triggers a file-save dialog (not an inline rendered SVG document), confirming `Content-Disposition: attachment`.
**Why human:** Whether the browser triggers a download vs. renders the SVG as a top-level document (the XSS vector) requires a real browser navigation test.

#### 3. Replace lifecycle + SSE chip reset

**Test:** Upload a PDF attachment. Note the "Text extracted" state on the chip. Click the Replace button, choose a different PDF. Observe the chip.
**Expected:** Card updates immediately to show the new filename/size. Chip resets to "Extracting text…" then settles to "Text extracted" for the new content. The card's URL (download link) is unchanged (same id).
**Why human:** The SSE re-subscription after a replace mutation and the chip live-transition are browser-only observable behaviours.

#### 4. Orphan delete and shared-file retention

**Test:** Upload a file and insert the same attachment link on two pages. Remove the link from Page A. Verify Page B's card is intact. Then remove the link from Page B. Verify the file is deleted (card gone, download returns 404).
**Expected:** After unlinking from Page A only: card on Page B still present. After unlinking from Page B: card disappears from Page B, download URL returns "That attachment no longer exists."
**Why human:** Multi-page ref counting and the resulting delete require a live server and manual navigation across pages.

---

### Gaps Summary

No gaps found. All 11 must-haves are VERIFIED with passing tests and substantive, wired implementations. The 4 items above require human browser-based verification because they involve drag-drop UX, live SSE transitions, browser rendering of SVG, and multi-page state — none of which are automatable at the code-scan level.

The only notable open item is the `react-hooks/static-components` ESLint finding on `AttachmentCard.tsx` line 74 (`const Icon = typeIconFor(meta)` created inside render). This is a pre-existing cosmetic lint finding documented in `deferred-items.md` — it does not cause incorrect behaviour and does not block the phase goal.

---

_Verified: 2026-06-21T00:16:00Z_
_Verifier: Claude (gsd-verifier)_
