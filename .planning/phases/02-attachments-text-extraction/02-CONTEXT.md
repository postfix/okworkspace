# Phase 2: Attachments & Text Extraction - Context

**Gathered:** 2026-06-21
**Status:** Ready for planning
**Mode:** Smart discuss (autonomous) — 4 grey areas, all recommendations accepted by user

<domain>
## Phase Boundary

A user can attach original files to pages, download them byte-for-byte unchanged, manage
them safely (replace, unlink, auto-delete orphans), and have their text extracted (PDF/
DOCX/TXT) so search (Phase 3) and the agent (Phase 4) can read them.

In scope: ATT-01..ATT-09, SEC-02. Out of scope: search indexing (Phase 3), agent reads
(Phase 4), XLSX/ZIP cell/content extraction (upload/download-only this phase).
</domain>

<decisions>
## Implementation Decisions

### Storage & Git model (Area 1 — accepted)
- Attachment binaries are **versioned in Git, inside the repo** (files-as-truth, hidden
  Git history, copy-off-server portability). NOT stored outside Git.
- On-disk name is a **generated opaque id** (ULID or content-hash) + extension; the
  original filename is preserved in a metadata sidecar (SEC-02 — never trust the upload
  filename for the stored path).
- **Three-part attachment model** (SPEC §11): `<id>.<ext>` binary + `<id>.json` metadata
  sidecar + `<id>.txt` extracted-text sidecar.
- Location: a flat **`attachments/` tree at the repo root**.

### Upload validation & download safety (Area 2 — accepted)
- Size limit from **`config.max_upload_mb`** (already in config, default 100).
- Type validation by **MIME-sniffing magic bytes** (`github.com/gabriel-vasile/mimetype`)
  against the config **allow-list**; reject on mismatch (ATT-09). Don't trust extension.
- Download safety (SEC-02): **images (png/jpg/svg) preview inline via `<img src>`**
  (an `<img>`-loaded SVG cannot execute script); **all other types are served with
  `Content-Disposition: attachment`**.
- Upload and delete **commit to Git via the existing single-writer CommitJob spine**
  (`internal/pages/commitjob.go`, `commitPayload{Writes,Removes}`) — no second write path.

### Text extraction (Area 3 — accepted)
- Extractors: **`github.com/ledongthuc/pdf` (PDF) + `github.com/fumiama/go-docx` (DOCX) +
  stdlib for TXT** — all pure-Go (CGO_ENABLED=0, single-binary promise). Text-layer only;
  scanned/image PDFs legitimately yield nothing.
- Runs **asynchronously as an `ExtractJob`** on the existing `internal/jobs` worker,
  triggered on upload and replace (extends the Phase 1 job-worker spine).
- **Explicit "No text extracted" state** surfaced on the card when extraction yields empty.
- Extraction status surfaced to the UI via **SSE** (per ROADMAP note).

### Lifecycle & UX (Area 4 — accepted)
- **Orphan deletion (ATT-07):** when the last page reference to an attachment is removed,
  delete the binary + both sidecars in the **same commit**. Reference detection = scan
  page markdown for the attachment link.
- **Replace (ATT-05):** reuse the same attachment id, write new content + updated meta via
  the CommitJob path (Git retains the prior version in history); re-run ExtractJob.
- **Attachment card (ATT-03):** original name, size, uploader, date (from the meta sidecar);
  image thumbnail for previewable types.
- **Upload UX:** `react-dropzone` drag-a-file-into-the-page, with client-side size/type
  pre-checks before the multipart POST.

### Claude's Discretion
- Exact id scheme (ULID vs content-hash), sidecar JSON field names, SSE endpoint shape,
  and component file layout are at Claude's discretion, consistent with Phase 0/1 patterns.
</decisions>

<code_context>
## Existing Code Insights

- **Single-writer Git spine** to reuse: `internal/pages/commitjob.go` (`KindCommit`,
  `commitPayload{Writes,Removes,Push}`, `EnqueueCommit`/`EnqueueAndWait`). Attachments
  must write through this, never `os.WriteFile`/`git` directly (mirrors Phase 1 invariant).
- **Job worker** to extend: `internal/jobs/queue.go` (already designed as the reused spine
  for CommitJob/ExtractJob/IndexJob). Register a new `KindExtract` handler.
- **Config** already has `MaxUploadMB` and `AllowedExtensions` (`internal/config/config.go`).
- **Safe-path resolver** (SEC-01) in `internal/repo` must gate every attachment path.
- **RBAC**: mutating routes are editor-gated from the session (`auth.RequireRole`), per the
  Phase 1 router pattern (`internal/server/router.go`).
- **Frontend**: `web/src/api/client.ts` `mutate()` helper (CSRF + Sec-Fetch-Site), the
  page editor/read views, and the optimistic-concurrency `base_revision` pattern.
</code_context>

<specifics>
## Specific Ideas

- Mirror the Phase 1 "hidden Git" discipline: no Git vocabulary in attachment UI/UX.
- Byte-for-byte download fidelity (ATT-02) is a hard requirement — store/serve the original
  bytes unchanged; extraction never mutates the original.
- The three-part sidecar model must keep originals copyable off-server as their real files.
</specifics>

<deferred>
## Deferred Ideas

- XLSX cell-text and ZIP content extraction (upload/download-only in MVP).
- OCR for scanned/image PDFs (go-fitz/MuPDF behind a build tag) — not in single-binary MVP.
</deferred>
