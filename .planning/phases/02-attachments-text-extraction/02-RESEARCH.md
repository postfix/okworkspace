# Phase 2: Attachments & Text Extraction - Research

**Researched:** 2026-06-21
**Domain:** File upload/storage in Git, pure-Go text extraction (PDF/DOCX/TXT), MIME sniffing, byte-exact download, async job pipeline, SSE status streaming
**Confidence:** HIGH

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

**Storage & Git model (Area 1 — accepted)**
- Attachment binaries are **versioned in Git, inside the repo** (files-as-truth, hidden Git history, copy-off-server portability). NOT stored outside Git.
- On-disk name is a **generated opaque id** (ULID or content-hash) + extension; the original filename is preserved in a metadata sidecar (SEC-02 — never trust the upload filename for the stored path).
- **Three-part attachment model** (SPEC §11): `<id>.<ext>` binary + `<id>.json` metadata sidecar + `<id>.txt` extracted-text sidecar.
- Location: a flat **`attachments/` tree at the repo root**.

**Upload validation & download safety (Area 2 — accepted)**
- Size limit from **`config.max_upload_mb`** (already in config, default 100).
- Type validation by **MIME-sniffing magic bytes** (`github.com/gabriel-vasile/mimetype`) against the config **allow-list**; reject on mismatch (ATT-09). Don't trust extension.
- Download safety (SEC-02): **images (png/jpg/svg) preview inline via `<img src>`** (an `<img>`-loaded SVG cannot execute script); **all other types are served with `Content-Disposition: attachment`**.
- Upload and delete **commit to Git via the existing single-writer CommitJob spine** (`internal/pages/commitjob.go`, `commitPayload{Writes,Removes}`) — no second write path.

**Text extraction (Area 3 — accepted)**
- Extractors: **`github.com/ledongthuc/pdf` (PDF) + `github.com/fumiama/go-docx` (DOCX) + stdlib for TXT** — all pure-Go (CGO_ENABLED=0, single-binary promise). Text-layer only; scanned/image PDFs legitimately yield nothing.
- Runs **asynchronously as an `ExtractJob`** on the existing `internal/jobs` worker, triggered on upload and replace (extends the Phase 1 job-worker spine).
- **Explicit "No text extracted" state** surfaced on the card when extraction yields empty.
- Extraction status surfaced to the UI via **SSE** (per ROADMAP note).

**Lifecycle & UX (Area 4 — accepted)**
- **Orphan deletion (ATT-07):** when the last page reference to an attachment is removed, delete the binary + both sidecars in the **same commit**. Reference detection = scan page markdown for the attachment link.
- **Replace (ATT-05):** reuse the same attachment id, write new content + updated meta via the CommitJob path (Git retains the prior version in history); re-run ExtractJob.
- **Attachment card (ATT-03):** original name, size, uploader, date (from the meta sidecar); image thumbnail for previewable types.
- **Upload UX:** `react-dropzone` drag-a-file-into-the-page, with client-side size/type pre-checks before the multipart POST.

### Claude's Discretion
- Exact id scheme (ULID vs content-hash), sidecar JSON field names, SSE endpoint shape, and component file layout are at Claude's discretion, consistent with Phase 0/1 patterns.

### Deferred Ideas (OUT OF SCOPE)
- XLSX cell-text and ZIP content extraction (upload/download-only in MVP).
- OCR for scanned/image PDFs (go-fitz/MuPDF behind a build tag) — not in single-binary MVP.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| ATT-01 | User can upload a file attachment to a page | Multipart upload (chi/net-http) → MIME-sniff → CommitJob write. See *Upload Pipeline* pattern. |
| ATT-02 | User can download an attachment in its original, unmodified form | Serve raw bytes via `http.ServeContent`/direct copy of the stored binary; the binary is stored byte-for-byte (extraction never touches it). See *Byte-Exact Download* pattern + Pitfall 4. |
| ATT-03 | Attachment card with original name, size, uploader, date | All four fields live in the `<id>.json` meta sidecar, returned by the list endpoint. See *Three-Part Model*. |
| ATT-04 | Preview image attachments (PNG/JPG/SVG) inline | Image bytes served with their real `Content-Type` and NO attachment disposition; SVG via `<img src>` only. See SEC-02 + Security Domain. |
| ATT-05 | Replace an attachment with a new version | Reuse same id; new binary + updated meta through CommitJob (Git keeps history); re-enqueue ExtractJob. See *Replace* pattern. |
| ATT-06 | Remove an attachment link from a page | Edit the page markdown (existing page-save path) to drop the link; then run orphan check. |
| ATT-07 | Delete attachment from repo when no page references it | Reference-count by scanning all page markdown for the attachment id/link; if zero refs, `Removes` the binary + both sidecars in ONE commit. See *Orphan Deletion* pattern + Pitfall 6. |
| ATT-08 | Extract text from PDF/DOCX/TXT for search/agent | `ledongthuc/pdf` + `fumiama/go-docx` + stdlib; async ExtractJob writes `<id>.txt`. See *Text Extraction* patterns + Code Examples. |
| ATT-09 | Uploads validated against size limit and allowed type (MIME-sniffed) | `MaxBytesReader` + `mimetype.DetectReader`/`Detect` against `config.AllowedExtensions`. See *Upload Validation* + Pitfall 1. |
| ATT-10 | Commit to Git automatically after upload or delete | All writes/removes route through `EnqueueCommit`/`commitPayload` (single-writer spine). See *Integration Spine*. |
| SEC-02 | Risky downloads served with `Content-Disposition: attachment` | Disposition decided by sniffed type, not filename; only png/jpg/svg are inline. See Security Domain. |
</phase_requirements>

## Summary

This phase is **almost entirely an integration exercise over spines that already exist**. The single-writer `CommitJob` (`internal/pages/commitjob.go`), the async `jobs.Worker` (`internal/jobs`), the safe-path resolver (`internal/repo`), the RBAC/editor gate (`auth.RequireRole`), and the config (`MaxUploadMB`, `AllowedExtensions`) are all in place and verified by reading the source. The two genuinely new pieces are (1) a binary upload/download pipeline and (2) an `ExtractJob` text-extraction handler — both of which plug into the existing worker and commit path with no second write path.

The two flagged research spikes resolve cleanly. **(1) Large-binary-in-Git is workable for this project's actual scale** (5 users, a wiki — not a media library) with three guardrails: a `max_upload_mb` cap (default 100; recommend the team treat ~25 MB as the comfortable working ceiling and reserve 100 as the hard limit), a MIME-sniffed allow-list, and the explicit acceptance that history is append-only (binaries are never rewritten out — that is a feature here, it is the "kept in history and can be restored" UX). The cost is real but bounded: Git stores each distinct binary version once (loose object → packed, zlib-compressed), so already-compressed formats (PDF/PNG/JPG/DOCX) gain little from packing but also do not balloon — repo growth ≈ sum of unique uploaded versions. **(2) Pure-Go extraction is validated** against the pinned versions: `ledongthuc/pdf` and `fumiama/go-docx` both compile with `CGO_ENABLED=0`, expose the exact APIs documented below, and handle text-layer documents well while yielding empty (not error) on scanned/image PDFs — which is precisely the "No text extracted" UX path the UI-SPEC already designs for.

The hidden-Git discipline from Phase 1 carries over verbatim: no Git vocabulary in the UI, all mutations through `EnqueueCommit`, byte-stable on-disk artifacts, and the safe-path resolver gating every path.

**Primary recommendation:** Build a thin `internal/attachments` package mirroring `internal/pages`: an HTTP handler layer (multipart upload, list, download, replace, remove) that enqueues `commitPayload`s through the existing worker, a `KindExtract` job handler that reads the committed binary and writes the `<id>.txt` sidecar through the resolver, and an SSE endpoint that reports extraction status from the jobs table. Use **ULID** for the opaque id, store the three artifacts under `attachments/<id>.<ext>` / `<id>.json` / `<id>.txt`, and never expose the on-disk id or any Git vocabulary to the user.

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Multipart upload parse + size cap | API / Backend | — | `MaxBytesReader` + `r.FormFile` must enforce the limit server-side; client pre-check (dropzone) is UX only, never the security boundary. |
| MIME sniffing + allow-list (ATT-09) | API / Backend | — | Magic-byte detection on the server; the filename/extension from the client is untrusted (SEC-02). |
| Opaque id generation + 3-part write | API / Backend | Database/Storage (Git) | Id is server-generated; the three artifacts are written through the resolver and committed via the single-writer spine. |
| Byte-exact download (ATT-02) | API / Backend | — | Server streams the stored bytes unchanged; sets `Content-Disposition` per sniffed type (SEC-02). |
| Text extraction (ATT-08) | API / Backend (async job) | — | `ExtractJob` on the `jobs.Worker`; CPU-bound, must not block the request. |
| Extraction status stream | API / Backend (SSE) | Browser/Client | SSE endpoint reads job status; client subscribes per attachment and renders the chip. |
| Upload dropzone + previews + cards | Browser/Client | — | `react-dropzone`, `<img>` thumbnails/preview dialog, card rendering from meta JSON. |
| Editor-gated mutation controls | Browser/Client | API / Backend | Client hides controls for readers (UX); the API re-enforces `RequireRole(editor)` (authority). |
| Orphan reference detection (ATT-07) | API / Backend | Database/Storage (Git) | Scan page markdown for the attachment reference; delete via `Removes` in one commit. |

## Standard Stack

> **The stack is LOCKED in CLAUDE.md and CONTEXT.md.** Every version below was re-verified this session against the Go module proxy (`proxy.golang.org`) and the libraries' source at the pinned commit. This section makes the locked stack implementable — it does not propose alternatives to locked choices.

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `github.com/ledongthuc/pdf` | v0.0.0-20250511090121-5959a4027728 | PDF → plain text (text-layer) | Pure-Go, no cgo, no external binary — preserves `CGO_ENABLED=0` single-binary. `Reader.GetPlainText() (io.Reader, error)` for whole-doc text. [VERIFIED: proxy.golang.org @latest = this pseudo-version, 2025-05-11; source read at commit 5959a4027728] |
| `github.com/fumiama/go-docx` | v0.0.0-20250506085032-0c30fd09304b | DOCX → text | Pure-Go, maintained (last push 2025-05-06), permissive license (avoids AGPL unioffice). `Parse(io.ReaderAt, int64)` → iterate `doc.Document.Body.Items`. [VERIFIED: proxy.golang.org @latest = this pseudo-version; source read at commit 0c30fd09304b] |
| `github.com/gabriel-vasile/mimetype` | v1.4.13 | Content-sniffing MIME detection (ATT-09) | Detects by magic bytes not extension — exactly "don't trust the filename." `Detect([]byte) *MIME`, `DetectReader(io.Reader)`, `MIME.Is(string) bool`, `MIME.Extension() string` (includes leading dot). [VERIFIED: proxy.golang.org tag v1.4.13, 2026-02-01; source read at tag v1.4.13] |
| stdlib `net/http` `MaxBytesReader` / `mime/multipart` | Go 1.26 | Multipart upload parse + size cap | Stdlib; `http.MaxBytesReader` hard-caps the request body server-side before the file is buffered. [VERIFIED: Go 1.26 stdlib] |
| stdlib `net/http` `ServeContent` / `io.Copy` | Go 1.26 | Byte-exact download (ATT-02) | Streams stored bytes unchanged; supports range requests for image preview. [VERIFIED: Go 1.26 stdlib] |
| `github.com/go-chi/chi/v5` | v5.3.0 | Router (existing) | Already the project router; attachment routes mount on the existing authed/editor groups. [VERIFIED: already in go.mod] |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `github.com/oklog/ulid/v2` | v2.1.1 | Opaque attachment id generation | **Recommended over content-hash** for the on-disk id (see *Open Questions* Q1). Sortable, 26-char, collision-free, no filename leak. `ulid.Make().String()`. [VERIFIED: proxy.golang.org tag v2.1.1, 2024-04-13; 5k stars, not archived] |
| stdlib `crypto/sha256` | Go 1.26 | Optional content hash stored in meta (dedup/integrity) | Store the upload's sha256 in the meta sidecar even if the on-disk name is a ULID — cheap integrity + future dedup signal. [VERIFIED: Go 1.26 stdlib] |
| stdlib `encoding/json` | Go 1.26 | Meta sidecar (`<id>.json`) read/write | The meta sidecar is small structured JSON. [VERIFIED: Go 1.26 stdlib] |
| stdlib `bytes` (`bytes.NewReader`) | Go 1.26 | Adapt `[]byte` → `io.ReaderAt` for pdf/docx Parse | Both `pdf.NewReader` and `docx.Parse` take `io.ReaderAt`; `bytes.NewReader` satisfies it. [VERIFIED: Go 1.26 stdlib] |

### Frontend (already in CLAUDE.md locked stack — no new deps)
| Library | Version | Purpose |
|---------|---------|---------|
| `react-dropzone` | 15.0.0 | Drag-a-file-into-page upload with client size/type pre-check (UI-SPEC) [CITED: CLAUDE.md locked table] |
| `@tanstack/react-query` | 5.101.0 | Attachment list/upload server-state + invalidation [CITED: CLAUDE.md locked table] |
| `lucide-react` | (existing) | Card type icons / status icons (UI-SPEC) [CITED: UI-SPEC] |
| native `EventSource` | browser | SSE subscription for extraction status (no new dep) [VERIFIED: browser standard] |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| ULID id | content-hash (sha256) id | Content-hash gives automatic dedup but breaks "replace reuses the same id" (replace changes content → changes hash → new path). ULID keeps a stable id across replace (CONTEXT requires same id on replace). **Use ULID.** |
| `ledongthuc/pdf` (pure-Go) | `gen2brain/go-fitz` (MuPDF, cgo, OCR-capable) | go-fitz handles scanned PDFs but breaks single-binary/CGO_ENABLED=0. Explicitly DEFERRED to v2 behind a build tag (CONTEXT deferred). |
| `fumiama/go-docx` | `unidoc/unioffice` | unioffice is AGPL/commercial — conflicts with an open self-hosted tool (CLAUDE.md "What NOT to Use"). |
| binaries-in-Git | originals outside Git, referenced from metadata | Breaks files-as-truth + copy-off-server portability. **Explicitly rejected and locked** (CONTEXT Area 1). Do not relitigate. |
| Git LFS | — | LFS requires an external store/server and a `git-lfs` binary, breaking single-binary self-host + copy-off-server portability. **Rejected** — see Large-Binary Spike. |

**Installation:**
```bash
go get github.com/ledongthuc/pdf@v0.0.0-20250511090121-5959a4027728
go get github.com/fumiama/go-docx@v0.0.0-20250506085032-0c30fd09304b
go get github.com/gabriel-vasile/mimetype@v1.4.13
go get github.com/oklog/ulid/v2@v2.1.1
go mod tidy
# Pin the pseudo-versions into go.sum immediately and commit the lockfile.
# No frontend installs — react-dropzone/react-query/lucide-react already in package.json.
```

## Package Legitimacy Audit

> Go modules are not covered by the npm/pypi/crates legitimacy seam. Verified instead via the Go module proxy (authoritative latest = pinned version) + GitHub repo health signals (age, stars, maintenance, archived status, license).

| Package | Registry | Age | Signal | Source Repo | Verdict | Disposition |
|---------|----------|-----|--------|-------------|---------|-------------|
| `github.com/ledongthuc/pdf` | Go proxy | created 2017-03 | 606★, pushed 2025-05-11, not archived | github.com/ledongthuc/pdf | OK | Approved (locked) |
| `github.com/fumiama/go-docx` | Go proxy | created 2023-02 | 297★, pushed 2025-05-06, not archived, permissive | github.com/fumiama/go-docx | OK | Approved (locked) |
| `github.com/gabriel-vasile/mimetype` | Go proxy | created 2018-07 | 1984★, pushed 2026-06-19, not archived | github.com/gabriel-vasile/mimetype | OK | Approved (locked) |
| `github.com/oklog/ulid/v2` | Go proxy | created 2016-12 | 5039★, pushed 2025-06-09, not archived | github.com/oklog/ulid | OK | Approved (Claude's-discretion id choice) |

**Packages removed due to [SLOP] verdict:** none
**Packages flagged as suspicious [SUS]:** none

All four packages resolve from the authoritative Go module proxy, are actively maintained, and are non-archived. Versions match CLAUDE.md exactly (the three locked) or are the current stable tag (oklog/ulid v2.1.1). The pseudo-versions for pdf/go-docx must be pinned in `go.sum` and the lockfile committed (mirrors the eino guidance in CLAUDE.md).

## Architecture Patterns

### System Architecture Diagram

```
                         ┌──────────────── Browser (React, editor-gated) ───────────────┐
                         │  AttachmentDropzone ─(client size/type pre-check, UX only)─┐  │
                         │  AttachmentCard / preview dialog        ExtractionStatus    │  │
                         └───────┬───────────────────┬───────────────────┬────────────┘  │
                          POST multipart        GET list/download    EventSource(SSE)
                                 │                    │                    │
   ┌─────────────────────────── API (chi, /api/v1, authed+editor groups) ───────────────────────┐
   │                             │                    │                    │                      │
   │   handleUpload ────────────►│                    │                    │                      │
   │     1. MaxBytesReader(max_upload_mb)  ◄── HARD server cap (ATT-09)    │                      │
   │     2. r.FormFile -> read bytes                  │                    │                      │
   │     3. mimetype.Detect(bytes) -> allow-list check (reject mismatch)   │                      │
   │     4. id = ulid.Make(); ext from sniffed MIME                        │                      │
   │     5. build meta JSON {orig_name, size, sha256, uploader, mime, date}│                      │
   │     6. EnqueueCommit{Writes: <id>.<ext>, <id>.json}  ────────┐        │                      │
   │     7. enqueue ExtractJob(id)                                │        │                      │
   │   handleDownload  -> resolver.Read(<id>.<ext>) -> ServeContent (byte-exact, ATT-02)          │
   │     Content-Disposition decided by sniffed MIME (SEC-02): png/jpg/svg inline, else attachment│
   │   handleList -> read all <id>.json under attachments/ -> cards (ATT-03)                       │
   │   handleSSE   -> poll jobs table for ExtractJob status -> text/event-stream ──────────────►  │
   └─────────────────────────────┬──────────────────────────────┬────────┴──────────────────────┘
                                  │                              │
                    ┌─────────────▼──────────┐      ┌────────────▼─────────────┐
                    │  jobs.Worker (single    │      │  jobs.Worker (same drain)│
                    │  drain goroutine)       │      │                          │
                    │  KindCommit handler ────┼──► repo.Write (resolver) + gitstore.Commit
                    │   writes <id>.<ext>,    │      │  KindExtract handler:    │
                    │   <id>.json, <id>.txt   │      │   1. read <id>.<ext>     │
                    │   ALL through resolver  │      │   2. switch on ext:      │
                    └─────────────────────────┘     │      pdf/docx/txt extract│
                                                     │   3. EnqueueCommit       │
                                                     │      <id>.txt (may be    │
                                                     │      empty -> "no text") │
                                                     └──────────────────────────┘
                                  │
                    ┌─────────────▼──────────────┐
                    │ Git working tree (repo)     │
                    │  attachments/<id>.<ext>     │  byte-exact original
                    │  attachments/<id>.json      │  meta sidecar
                    │  attachments/<id>.txt       │  extracted text (or empty)
                    │  hidden Git history         │  retains every version
                    └─────────────────────────────┘
```

A reader can trace the primary use case (upload → extract → download): client POSTs multipart → server caps + sniffs + assigns ULID → enqueues a commit for binary+meta and an ExtractJob → worker commits the two files, then the ExtractJob reads the committed binary, extracts text, and commits `<id>.txt` → SSE reports status to the card → later, download streams the byte-exact binary back with the SEC-02 disposition.

### Recommended Project Structure
```
internal/attachments/          # NEW — mirrors internal/pages structure
├── service.go                 # Upload/List/Replace/Remove orchestration; builds commitPayloads
├── extractjob.go              # KindExtract handler (pdf/docx/txt -> <id>.txt); registered on worker
├── extract.go                 # pure extract funcs: extractPDF/extractDOCX/extractTXT([]byte) (string, error)
├── meta.go                    # meta sidecar struct + JSON read/write through resolver
├── id.go                      # ULID id generation + path helpers (attachments/<id>.<ext|json|txt>)
├── refs.go                    # orphan reference scan (ATT-07): does any page markdown reference <id>?
├── *_test.go                  # extract fidelity tests against fixture PDFs/DOCX/TXT
internal/server/
├── handlers_attachments.go    # NEW — multipart upload, list, download, replace, remove, SSE
│                              #       (mounts on existing authed + editor groups in router.go)
web/src/components/attachments/ # NEW — per UI-SPEC Component Inventory
├── AttachmentsSection.tsx     # + AttachmentsSection.css
├── AttachmentDropzone.tsx
├── AttachmentCard.tsx
├── ExtractionStatus.tsx
├── ReplaceAttachmentDialog.tsx
├── RemoveAttachmentDialog.tsx
web/src/api/
├── attachments.ts             # NEW — upload/list/download/replace/remove + SSE subscribe (reuse client.ts mutate())
testdata/attachments/          # NEW — fixture files for extract fidelity tests
├── text-layer.pdf  scanned-image.pdf  sample.docx  sample.txt  utf8-bom.txt
```

### Pattern 1: Upload pipeline (multipart + server-side cap + MIME sniff)
**What:** Parse a multipart upload, hard-cap its size server-side, sniff its real type, reject on allow-list miss, then enqueue a commit.
**When to use:** `handleUpload` and the replace handler.
**Example:**
```go
// Source: Go stdlib net/http + gabriel-vasile/mimetype v1.4.13 (source-verified)
func (h *attachHandlers) handleUpload(w http.ResponseWriter, r *http.Request) {
    maxBytes := int64(h.cfg.Storage.MaxUploadMB) * 1024 * 1024
    // HARD server-side cap BEFORE buffering the file (ATT-09). MaxBytesReader
    // makes ParseMultipartForm/FormFile fail once the body exceeds the limit.
    r.Body = http.MaxBytesReader(w, r.Body, maxBytes+1024) // +slack for form overhead
    if err := r.ParseMultipartForm(32 << 20); err != nil { // 32MB in-memory, rest spooled to temp
        writeJSONError(w, http.StatusRequestEntityTooLarge, "That file is too large.")
        return
    }
    file, hdr, err := r.FormFile("file")
    if err != nil { writeJSONError(w, http.StatusBadRequest, "No file."); return }
    defer file.Close()

    data, err := io.ReadAll(file) // bounded by MaxBytesReader above
    if err != nil { writeJSONError(w, http.StatusRequestEntityTooLarge, "That file is too large."); return }

    // Sniff REAL type from magic bytes — never trust hdr.Filename's extension (SEC-02).
    mt := mimetype.Detect(data) // *mimetype.MIME ; reads only the first few KB internally
    if !allowed(h.cfg.Attachments.AllowedExtensions, mt) {
        writeJSONError(w, http.StatusUnsupportedMediaType, "That file type isn't allowed.")
        return
    }
    id := ulid.Make().String()
    ext := strings.TrimPrefix(mt.Extension(), ".") // mimetype Extension() includes leading dot
    // ... build meta, enqueue commit (Pattern 4), enqueue ExtractJob ...
}
```

### Pattern 2: PDF text extraction (pure-Go, text-layer)
**What:** Extract whole-document plain text from PDF bytes.
**When to use:** `extractPDF` inside the ExtractJob.
**Example:**
```go
// Source: github.com/ledongthuc/pdf @ 5959a4027728 (source-verified signatures):
//   func NewReader(f io.ReaderAt, size int64) (*Reader, error)
//   func (r *Reader) GetPlainText() (reader io.Reader, err error)
func extractPDF(data []byte) (string, error) {
    rdr, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
    if err != nil {
        return "", fmt.Errorf("attachments: pdf open: %w", err)
    }
    tr, err := rdr.GetPlainText() // whole-doc text; NO args (per-page GetPlainText takes a fonts map)
    if err != nil {
        return "", fmt.Errorf("attachments: pdf text: %w", err)
    }
    var buf bytes.Buffer
    if _, err := buf.ReadFrom(tr); err != nil {
        return "", fmt.Errorf("attachments: pdf read: %w", err)
    }
    return buf.String(), nil // EMPTY string is valid: scanned/image PDF has no text layer -> "No text extracted"
}
```

### Pattern 3: DOCX text extraction (pure-Go)
**What:** Parse a DOCX and concatenate paragraph/table text.
**When to use:** `extractDOCX` inside the ExtractJob.
**Example:**
```go
// Source: github.com/fumiama/go-docx @ 0c30fd09304b (source-verified):
//   func Parse(reader io.ReaderAt, size int64) (doc *Docx, err error)
//   (*Paragraph).String() and (*Table).String() render their text (README example uses fmt.Println(it))
func extractDOCX(data []byte) (string, error) {
    doc, err := docx.Parse(bytes.NewReader(data), int64(len(data)))
    if err != nil {
        return "", fmt.Errorf("attachments: docx parse: %w", err)
    }
    var b strings.Builder
    for _, it := range doc.Document.Body.Items {
        switch v := it.(type) {
        case *docx.Paragraph:
            b.WriteString(v.String())
            b.WriteByte('\n')
        case *docx.Table:
            b.WriteString(v.String())
            b.WriteByte('\n')
        }
    }
    return strings.TrimSpace(b.String()), nil
}
```

### Pattern 4: TXT extraction (stdlib, encoding-aware)
**What:** Decode a plain-text upload, normalizing a UTF-8 BOM and line endings.
**When to use:** `extractTXT`.
**Example:**
```go
// Source: Go stdlib (source-verified)
func extractTXT(data []byte) (string, error) {
    // Strip a UTF-8 BOM if present; normalize CRLF -> LF. (Latin-1/UTF-16 are out of
    // MVP scope — store as-is; the byte-exact original is always downloadable anyway.)
    data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
    s := strings.ReplaceAll(string(data), "\r\n", "\n")
    return strings.TrimSpace(s), nil
}
```

### Pattern 5: Write all three artifacts through the EXISTING single-writer spine
**What:** Every attachment write/delete builds a `commitPayload` and calls `EnqueueCommit` — never `os.WriteFile`/`git` directly. (Mirrors `internal/pages` invariant exactly.)
**When to use:** Upload, replace, extract-result write, orphan delete.
**Example:**
```go
// Source: internal/pages/commitjob.go (read this session) — reuse commitPayload{Writes,Removes,Spec,Push}
// NOTE: commitPayload and fileWrite are currently UNEXPORTED in package pages. The plan must either
// (a) add an exported attachment commit helper in package pages, or (b) export commitPayload/fileWrite /
// EnqueueCommit, or (c) give attachments its own commitPayload mirror that marshals to the SAME JSON the
// registered KindCommit handler unmarshals. Option (c) keeps packages decoupled and reuses ONE handler.
p := commitPayload{
    Writes: []fileWrite{
        {Path: "attachments/" + id + "." + ext, Bytes: binaryData},      // byte-exact original
        {Path: "attachments/" + id + ".json", Bytes: metaJSON},
    },
    Spec: gitstore.CommitSpec{
        Paths:   []string{"attachments/" + id + "." + ext, "attachments/" + id + ".json"},
        Message: "Add attachment",     // hidden-Git: no "commit" vocabulary surfaces to the user
        User:    user, Action: "attach", Source: "web-ui",
    },
    Push: s.pushOnCommit,
}
return EnqueueCommit(ctx, s.worker, p) // blocks until on disk, then list refetch sees it (Phase-1 race fix)
```

### Pattern 6: ExtractJob on the existing worker
**What:** Register a `KindExtract` handler; enqueue it after upload/replace. It reads the committed binary, extracts, and commits `<id>.txt`.
**When to use:** Worker wiring in `main.go` + enqueue in the upload/replace path.
**Example:**
```go
// Source: internal/jobs/queue.go + worker.go (read this session)
const KindExtract = "extract"

func ExtractHandler(r *repo.Repo, w *jobs.Worker, g *gitstore.GitStore) jobs.Handler {
    return func(ctx context.Context, payload string) error {
        var p extractPayload // {ID, Ext} JSON
        if err := json.Unmarshal([]byte(payload), &p); err != nil { return err }
        data, err := r.Read("attachments/" + p.ID + "." + p.Ext) // resolver-gated
        if err != nil { return fmt.Errorf("attachments: read for extract: %w", err) }

        var text string
        switch strings.ToLower(p.Ext) {
        case "pdf":  text, err = extractPDF(data)
        case "docx": text, err = extractDOCX(data)
        case "txt":  text, err = extractTXT(data)
        default:     return nil // non-extractable type: no .txt, card shows no chip
        }
        if err != nil { return err } // worker retries w/ backoff; terminal failure -> "Couldn't extract text"

        // EMPTY text is a SUCCESS, not a failure: write an empty <id>.txt so the
        // card can distinguish "extracted nothing" (No text extracted) from "not yet run".
        return enqueueExtractCommit(ctx, w, p.ID, []byte(text)) // commits attachments/<id>.txt
    }
}
// Register in main.go alongside the existing CommitHandler:
//   worker.Register(pages.KindCommit, pages.CommitHandler(repo, gitstore))
//   worker.Register(attachments.KindExtract, attachments.ExtractHandler(repo, worker, gitstore))
```

### Pattern 7: Byte-exact download with SEC-02 disposition
**What:** Stream the stored binary unchanged; set `Content-Disposition` from the sniffed type.
**When to use:** `handleDownload`.
**Example:**
```go
// Source: Go stdlib net/http (source-verified). ServeContent sets Content-Type/Length,
// handles Range requests (needed for <img> preview), and never mutates bytes (ATT-02).
func (h *attachHandlers) handleDownload(w http.ResponseWriter, r *http.Request) {
    meta := h.loadMeta(id) // orig_name, mime
    abs, err := h.repo.Resolve("attachments/" + id + "." + meta.Ext) // SEC-01
    if err != nil { http.Error(w, "not found", http.StatusNotFound); return }
    f, err := os.Open(abs); if err != nil { http.NotFound(w, r); return }
    defer f.Close()

    inline := isInlineImage(meta.MIME) // ONLY image/png, image/jpeg, image/svg+xml
    if inline {
        w.Header().Set("Content-Type", meta.MIME)
        w.Header().Set("Content-Disposition", "inline")
    } else {
        // SEC-02: risky types are download-only; quote the ORIGINAL filename for the user.
        w.Header().Set("Content-Disposition",
            fmt.Sprintf("attachment; filename*=UTF-8''%s", url.PathEscape(meta.OrigName)))
        w.Header().Set("Content-Type", "application/octet-stream")
    }
    // Harden against content-type confusion regardless of branch:
    w.Header().Set("X-Content-Type-Options", "nosniff")
    http.ServeContent(w, r, meta.OrigName, meta.ModTime, f) // byte-exact, range-capable
}
```

### Pattern 8: SSE extraction status (stdlib net/http)
**What:** A `text/event-stream` endpoint that reports an attachment's extraction status (extracting / extracted / none / failed) by polling the jobs table, so the card chip updates live.
**When to use:** `handleExtractionSSE`; client subscribes with `EventSource`.
**Example:**
```go
// Source: Go stdlib net/http SSE pattern (source-verified primitives: Flusher, context.Done)
func (h *attachHandlers) handleExtractionSSE(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")
    flusher, ok := w.(http.Flusher)
    if !ok { http.Error(w, "stream unsupported", http.StatusInternalServerError); return }

    ticker := time.NewTicker(500 * time.Millisecond); defer ticker.Stop()
    for {
        select {
        case <-r.Context().Done(): // client disconnected (EventSource closed)
            return
        case <-ticker.C:
            status := h.extractionStatus(r.Context(), id) // "extracting"|"extracted"|"none"|"failed"
            fmt.Fprintf(w, "data: {\"status\":%q}\n\n", status)
            flusher.Flush()
            if status != "extracting" { return } // terminal -> close stream
        }
    }
}
```
> Pin per-connection timeouts (idle/read) generously or exempt the SSE route from any global write timeout — a server `WriteTimeout` will silently kill long-lived SSE streams (Pitfall 7). The UI-SPEC already specifies "on disconnect, fall back to last-known state (no error flash)," so a closed stream is graceful by design.

### Anti-Patterns to Avoid
- **Trusting `hdr.Filename` extension for type or for the stored path.** Always sniff with mimetype and always name on disk with the server-generated ULID. (SEC-02; the whole point of the opaque id.)
- **Re-encoding/normalizing the binary on the way in or out.** ATT-02 demands byte-for-byte fidelity. Extraction reads a *copy* of the bytes and never writes back to the binary path.
- **Writing the binary with `os.WriteFile` or shelling `git add` directly.** Breaks the single-writer invariant (Phase-1 lesson). Everything goes through `EnqueueCommit`/`commitPayload`.
- **Treating empty extracted text as a job failure.** Empty is the correct, expected result for a scanned PDF — it must produce a (possibly empty) `<id>.txt` and the "No text extracted" chip, NOT a retry loop or a "Couldn't extract text" error.
- **Parsing multipart without `MaxBytesReader`.** `ParseMultipartForm`'s own `maxMemory` only controls in-memory vs. temp-spool; it does NOT cap total size — an attacker can spool a huge file to disk. Cap with `MaxBytesReader` (Pitfall 1).
- **Inlining SVG via `dangerouslySetInnerHTML` or server `Content-Disposition: inline; Content-Type: image/svg+xml` rendered into the DOM.** Serve SVG to an `<img src>` only (an `<img>`-loaded SVG cannot run script). Matches the Phase-1 stored-XSS guard.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| File type detection | Extension/string parsing of filename | `mimetype.Detect` (magic bytes) | Filenames lie; magic-byte sniffing is the security boundary (SEC-02/ATT-09). |
| PDF text extraction | A PDF tokenizer / content-stream parser | `ledongthuc/pdf` `GetPlainText` | PDF text extraction (font maps, CMaps, content streams) is deceptively hard; the lib handles text-layer docs. |
| DOCX text extraction | A zip+XML walker | `fumiama/go-docx` `Parse` | DOCX is a zip of OOXML; the lib already models paragraphs/tables/runs. |
| Opaque id | `time.Now().UnixNano()` strings / custom counters | `oklog/ulid` `ulid.Make()` | Sortable, collision-resistant, fixed-length, well-tested. |
| Upload size cap | Counting bytes manually mid-stream | `http.MaxBytesReader` | Stdlib enforces the cap at the body layer before buffering/spooling. |
| Byte-exact streaming + range | Manual `w.Write` loops | `http.ServeContent` | Handles Range (image preview), Content-Length, conditional requests, and never mutates bytes. |
| Git binary versioning | A custom blob store / dedup layer | the existing `gitstore.Commit` via CommitJob | Git already content-addresses + compresses + retains history; the spine exists. |

**Key insight:** Almost nothing in this phase is novel. The binary store, version history, async pipeline, path safety, and auth are all solved by spines already in the repo. The only library-specific risk is extraction *fidelity*, which is bounded by the "No text extracted" UX the UI-SPEC already designs for — there is no fidelity bar to hit beyond "text-layer documents produce usable text; scanned ones legitimately produce none."

## Large-Binary-in-Git Spike (PRIMARY research focus #1)

**Verdict: the locked decision is workable for this project. Confirmed, with documented caveats. Do not relitigate.**

### How Git stores the binaries (mechanics)
- On `git add`, each file becomes a **loose object**: zlib-DEFLATE-compressed, content-addressed by sha1/sha256. `git gc`/auto-gc later moves loose objects into **packfiles**. [VERIFIED: Git 2.47.3 present in env; standard Git object model]
- Already-compressed formats (**PDF, PNG, JPG, DOCX**) are high-entropy, so zlib gains little and **Git's delta compression between versions is also poor** for them. Net effect: each *distinct uploaded version* costs ≈ its own (slightly-compressed) size in the object store, roughly once. [VERIFIED: Git object/packing behavior; HIGH for the qualitative claim]
- **History is append-only by design here.** A removed attachment's bytes remain in history (that is the "kept in history and can be restored" UX). This is a *feature*, not a leak, for a 5-person internal wiki. [CITED: CONTEXT Area 1 + UI-SPEC copywriting]

### Practical consequences at THIS scale (5 users, internal wiki)
- **Repo growth ≈ Σ unique uploaded versions.** With a 100 MB hard cap and realistic wiki usage (a few PDFs/images per page), the repo grows linearly and slowly. This is not a media library or a CI artifact store — the pathological "GB-scale binaries churned daily" case does not apply. [ASSUMED — usage profile; risk: low; see Assumptions A1]
- **Clone/backup cost = full repo size** (history included). For copy-off-server portability this is exactly what we want: one `cp -r` / one clone yields every original + every version. The cost scales with total uploaded volume, acceptable at this scale. [VERIFIED: Git clone semantics]
- **No history rewrite needed or wanted.** Avoiding `git filter-repo`/BFG later is a goal, not a fallback — rewriting would break the hidden-Git history and any restore. The guardrails below keep us out of the situation where a rewrite would ever be tempting. [VERIFIED: matches CONTEXT lock]

### Guardrails to keep it sane (the "do it well" part)
1. **`max_upload_mb` cap (already in config, default 100).** RECOMMEND documenting ~25 MB as the comfortable team working ceiling and keeping 100 MB as the hard reject limit. Enforce server-side with `MaxBytesReader` (Pitfall 1). [VERIFIED: config.go has `MaxUploadMB`; threshold is ASSUMED — A2]
2. **MIME-sniffed allow-list (`config.AllowedExtensions`).** Reject everything not on the list by *sniffed* type — keeps out arbitrary large blobs and executables. [VERIFIED: config.go has `AllowedExtensions`; ATT-09]
3. **Three-part model keeps bloat predictable.** The `.json` meta and `.txt` extracted-text sidecars are tiny text — they delta-compress well and add negligible size next to the binary. [VERIFIED: model from CONTEXT]
4. **One commit per logical action** (upload = binary+meta in one commit; extract = one `.txt` commit; orphan-delete = binary+both sidecars removed in one commit). Avoids commit-spam in history and keeps `git gc` effective. [VERIFIED: matches CommitJob batching design]
5. **Let Git auto-gc run** (default). No special configuration needed at this scale; do NOT add aggressive repacking that would fight the high-entropy binaries. [ASSUMED — A3; risk: low]
6. **`Content-Disposition: attachment` + `X-Content-Type-Options: nosniff`** on download for non-image types — orthogonal to bloat but part of "serve byte-exact originals safely" (SEC-02). [VERIFIED: SEC-02]

### What could still bite (and the answer)
- *"What if someone uploads 100 × 90 MB files?"* — That is ~9 GB of history; still a single `cp`/clone, still works, just large. The cap + allow-list + 5-user social scale make this implausible, and if it ever happens it is a deliberate choice the team made, recoverable by a (deferred, explicitly out-of-scope) history-rewrite escape hatch. Document the cap; don't engineer for the pathological case. [ASSUMED — A1/A2]
- *"Byte-exact serving"* — verified: store the uploaded bytes unchanged, serve with `http.ServeContent`/`io.Copy`, never transcode. A round-trip test (upload bytes == download bytes) is the ATT-02 exit check (see Validation). [VERIFIED]

## PDF/DOCX/TXT Extraction-Fidelity Spike (PRIMARY research focus #2)

**Verdict: pure-Go extraction is validated against the pinned versions. APIs confirmed by reading source. Build it; gate quality with fixture tests.**

### Confirmed current APIs (source-read at pinned commits)
| Lib | Import | Open/Parse | Whole-doc text | Notes |
|-----|--------|-----------|----------------|-------|
| ledongthuc/pdf | `github.com/ledongthuc/pdf` | `Open(string) (*os.File,*Reader,error)` **or** `NewReader(io.ReaderAt, int64) (*Reader, error)` | `(*Reader).GetPlainText() (io.Reader, error)` | Prefer `NewReader(bytes.NewReader(data), int64(len(data)))` — we hold bytes, not a path. Per-page `Page.GetPlainText(map[string]*Font)` exists but the whole-doc Reader method is simpler. [VERIFIED: page.go L64, L521; pdf.go L105/L125] |
| fumiama/go-docx | `github.com/fumiama/go-docx` | `Parse(io.ReaderAt, int64) (*Docx, error)` | iterate `doc.Document.Body.Items`, type-switch `*docx.Paragraph`/`*docx.Table`, call `.String()` | README's own example uses `fmt.Println(it)` which invokes `String()`. [VERIFIED: docx.go L93; structpara.go L194 `(*Paragraph).String()`] |
| stdlib | `bytes`,`strings` | — | trim BOM, normalize CRLF | UTF-16/Latin-1 out of scope; original is always downloadable. [VERIFIED: stdlib] |

### What they extract well vs. poorly
- **Well:** text-layer PDFs (digitally generated — exports from Word/LaTeX/most tools), standard DOCX paragraphs and tables, UTF-8 TXT. Good enough for the search/agent use-case (the only consumers — extraction is never shown verbatim to the user as a primary artifact). [VERIFIED: lib feature scope + source]
- **Poorly / not at all:**
  - **Scanned/image-only PDFs** → `GetPlainText` returns an empty string (no text layer). This is the EXPECTED "No text extracted" path, not an error. [VERIFIED: lib has no OCR; CONTEXT + UI-SPEC anticipate this]
  - **Complex PDF layout** (multi-column, heavy tables, ligatures/CMap-encoded fonts) → text may be reordered, run-together, or lose spacing. Acceptable for search indexing; do not promise verbatim layout. [ASSUMED — A4; common pure-Go PDF limitation]
  - **DOCX embedded objects, headers/footers, footnotes, text boxes** → may be omitted (the Body.Items walk covers body paragraphs/tables). Acceptable for MVP search. [ASSUMED — A5]
- **Encoding/whitespace:** normalize CRLF→LF and strip a UTF-8 BOM for TXT; `strings.TrimSpace` the assembled output for all three so a doc with only whitespace is treated as "no text." [VERIFIED: stdlib approach]

### Error handling contract (drives the four UI states)
| Outcome | ExtractJob result | `<id>.txt` | UI chip (UI-SPEC) |
|---------|-------------------|-----------|-------------------|
| Text found | success | text bytes | "Text extracted" (success/green) |
| Empty (scanned PDF / blank doc) | **success** | empty file | "No text extracted" (warning/amber) + sub-note |
| Parse error (corrupt/encrypted PDF, bad zip) | error → retry, then terminal fail | not written | "Couldn't extract text" (destructive/red) |
| Non-extractable type (image/other) | n/a — no ExtractJob enqueued | none | no chip at all |

> **Key:** distinguish *empty-but-succeeded* from *failed*. Write an (empty) `.txt` on success-with-no-text so the card knows extraction ran. Reserve "Couldn't extract text" for genuine parse errors after the worker's retry/backoff is exhausted. A panic inside a third-party parser must be recovered inside the handler so one bad file never kills the single drain goroutine (Pitfall 5).

### Fidelity test plan (the spike's deliverable as repeatable tests)
Place fixtures in `testdata/attachments/` and assert in `internal/attachments/extract_test.go`:
- `text-layer.pdf` → non-empty, contains a known sentinel string.
- `scanned-image.pdf` → empty string, no error (the "No text extracted" guarantee).
- `sample.docx` → contains paragraph + table cell text.
- `sample.txt` (CRLF + UTF-8 BOM) → BOM stripped, LF-normalized, sentinel present.
- `corrupt.pdf` / truncated zip → returns an error (drives "Couldn't extract text").

## Runtime State Inventory

> Not a rename/refactor/migration phase — this is greenfield feature work. Inventory is N/A. The one stateful concern (Git history retains binaries) is covered in the Large-Binary Spike and is intentional, not a migration.

## Common Pitfalls

### Pitfall 1: Multipart upload with no real size cap
**What goes wrong:** `r.ParseMultipartForm(maxMemory)` is read as "the size limit," but `maxMemory` only decides in-RAM vs. temp-file spooling — the total upload is unbounded and can fill the disk.
**Why it happens:** The stdlib API name is misleading; the cap lives elsewhere.
**How to avoid:** Wrap `r.Body` in `http.MaxBytesReader(w, r.Body, maxBytes)` BEFORE parsing, sized from `config.MaxUploadMB`. Return 413 on the resulting error.
**Warning signs:** Temp dir growing during a large upload; OOM/disk-full under a big POST.

### Pitfall 2: Trusting the client filename for type or storage path
**What goes wrong:** A `.png` that is actually an HTML/SVG/executable slips past, or a crafted filename (`../`, absolute) influences the path.
**Why it happens:** Extension and `hdr.Filename` are attacker-controlled.
**How to avoid:** Sniff with `mimetype.Detect`; check the *sniffed* type against the allow-list; name on disk with the server ULID; keep the original name ONLY in the meta sidecar; never build the path from the filename. (`repo.Resolve` is the backstop.)
**Warning signs:** Files on disk named after user input; type checks that read `strings.HasSuffix(filename, ...)`.

### Pitfall 3: SVG inline rendering as a script vector
**What goes wrong:** An uploaded SVG containing `<script>` executes if rendered into the DOM or served `inline` and navigated to directly.
**Why it happens:** SVG is XML and can carry script.
**How to avoid:** Serve SVG only to an `<img src>` (an `<img>`-loaded SVG cannot run script). For all image types set `X-Content-Type-Options: nosniff`. Never inline SVG markup into HTML. (Matches Phase-1 stored-XSS guard: raw-HTML plugin OFF.)
**Warning signs:** `dangerouslySetInnerHTML` near attachment preview; SVG opened as a top-level document.

### Pitfall 4: Mutating the binary on the way in or out (breaks ATT-02)
**What goes wrong:** Image re-encoding, line-ending normalization on a "text" upload, or a transcoding download path changes bytes, failing the byte-exact requirement.
**Why it happens:** Helper libraries or "be nice" normalization sneak in.
**How to avoid:** Store the uploaded bytes verbatim; extraction operates on a copy and writes only `<id>.txt`. Download via `http.ServeContent`/`io.Copy` with no transformation. Add an upload-bytes == download-bytes round-trip test.
**Warning signs:** Any image/PDF library imported on the upload or download path; a download handler that re-builds content.

### Pitfall 5: A third-party parser panic kills the single drain goroutine
**What goes wrong:** A malformed PDF/DOCX triggers a panic deep in the parser; because the jobs worker is a single goroutine, the whole async pipeline (commits included) dies.
**Why it happens:** Pure-Go parsers on adversarial input can panic, not just error.
**How to avoid:** `defer recover()` inside the ExtractJob handler (and ideally inside each `extractX` func), converting a panic into a returned error so the worker's retry/terminal-fail path handles it and the drain survives.
**Warning signs:** Worker goroutine exits; jobs stop draining after a specific upload.

### Pitfall 6: Orphan-delete reference scan misses a reference (or deletes a shared file)
**What goes wrong:** An attachment referenced by two pages is deleted when one link is removed; or a still-referenced file is GC'd.
**Why it happens:** Reference detection only checks the current page, or matches the link loosely.
**How to avoid:** On unlink, scan ALL page markdown for the attachment id/link; delete the binary+sidecars only when the count hits zero, in ONE commit. Match the canonical link form the upload inserts (define it once).
**Warning signs:** Broken image/download links after a delete; "deleted" files still referenced elsewhere.

### Pitfall 7: SSE stream killed by the server write timeout
**What goes wrong:** A global `http.Server.WriteTimeout` silently terminates the long-lived `text/event-stream` connection mid-extraction.
**Why it happens:** SSE is a long-lived response; a write deadline applies to it.
**How to avoid:** Exempt the SSE route from a global write timeout (or set none/very large for it); close the stream yourself on terminal status or `r.Context().Done()`. The UI already degrades gracefully on disconnect (last-known state, no error flash), so a dropped stream is safe.
**Warning signs:** Extraction chip stuck on "Extracting…" then dropping without reaching a terminal state.

## Code Examples

(See *Architecture Patterns* 1–8 above — each is a verified, source-grounded example for: upload+cap+sniff, PDF/DOCX/TXT extraction, single-writer commit, ExtractJob, byte-exact download, and SSE. They are the canonical task references.)

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Git LFS / external object store for binaries | Files-in-Git for small-team internal tools that prize portability | Project decision | Keeps single-binary + copy-off-server; LFS would add a server dep. Locked. |
| cgo PDF (MuPDF/poppler) | Pure-Go `ledongthuc/pdf` | Project decision | Preserves `CGO_ENABLED=0`; trades OCR/scanned support (deferred to v2). |
| `ParseMultipartForm` as the cap | `MaxBytesReader` for the real cap | Long-standing Go guidance | Prevents disk-fill DoS. |

**Deprecated/outdated:**
- `unidoc/unioffice` for DOCX — AGPL/commercial; rejected in CLAUDE.md. Use `fumiama/go-docx`.
- `h2non/filetype` for sniffing — stale (2021), smaller signature set. Use `gabriel-vasile/mimetype`.

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | Real usage is wiki-scale (a handful of modest attachments per page), so binaries-in-Git grows slowly | Large-Binary Spike | If the team treats it as a media dump, repo/clone size grows faster than expected — mitigated by the cap + allow-list; recoverable. |
| A2 | ~25 MB comfortable working ceiling (vs. 100 MB hard cap) is a sensible team norm | Large-Binary Spike guardrail 1 | Wrong threshold just means a different cap; cap is config-driven, no code change. Confirm with user. |
| A3 | Git default auto-gc is sufficient; no custom repack needed at this scale | Large-Binary Spike guardrail 5 | If repo gets large, a manual `git gc` may be wanted later; low risk now. |
| A4 | Complex multi-column/table PDF layout may extract with imperfect ordering/spacing | Extraction Spike | Only affects search recall, not correctness; "No text extracted" path unaffected. |
| A5 | DOCX headers/footers/footnotes/text-boxes may be omitted by the Body.Items walk | Extraction Spike | Reduces extracted coverage for some docs; acceptable for MVP search; could be extended later. |

## Open Questions

1. **Opaque id: ULID vs content-hash?** (Claude's discretion per CONTEXT)
   - What we know: ULID is sortable, stable across replace, no filename leak; content-hash gives free dedup but a new hash on every replace (breaks "replace reuses the same id").
   - What's unclear: whether dedup of identical uploads is ever wanted.
   - Recommendation: **ULID** (`oklog/ulid/v2`), and ALSO store the upload's sha256 in the meta sidecar for integrity + a future dedup signal. Resolved — no blocker.

2. **`commitPayload`/`fileWrite` are unexported in package `pages`.** (Integration detail)
   - What we know: attachments must commit through the SAME registered `KindCommit` handler/JSON shape.
   - What's unclear: export from `pages`, add an exported helper, or mirror the struct in `attachments`.
   - Recommendation: have `attachments` marshal its own struct to the identical JSON the existing `KindCommit` handler unmarshals (keeps packages decoupled, reuses one handler). Planner picks; low risk.

3. **Canonical attachment link form in page markdown.** (Drives ATT-06/ATT-07 reference scan)
   - What we know: orphan detection scans page markdown for the reference; replace/unlink must match it.
   - What's unclear: exact link syntax (e.g. `![](/attachments/<id>...)` vs an API download URL) — UI-SPEC implies a download endpoint, not an inline path.
   - Recommendation: define ONE canonical link/markup the upload inserts and the scan matches; document it in the plan so insert and scan can never drift.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go toolchain | build (all) | ✓ | go1.26.0 | — |
| git CLI | binary commit via single-writer spine (ATT-10) | ✓ | 2.47.3 | — (CLI-first is a locked decision) |
| `ledongthuc/pdf` | PDF extraction (ATT-08) | ✗ (not yet in go.mod) | pin v0.0.0-20250511090121-5959a4027728 | none — required; `go get` it |
| `fumiama/go-docx` | DOCX extraction (ATT-08) | ✗ (not yet in go.mod) | pin v0.0.0-20250506085032-0c30fd09304b | none — required; `go get` it |
| `gabriel-vasile/mimetype` | MIME sniffing (ATT-09) | ✗ (not yet in go.mod) | v1.4.13 | none — required; `go get` it |
| `oklog/ulid/v2` | opaque id | ✗ (not yet in go.mod) | v2.1.1 | could use crypto/rand-based id, but ULID preferred |
| react-dropzone / react-query / lucide-react | upload UX, list, icons | ✓ (in CLAUDE.md locked package.json) | per CLAUDE.md | native input fallback exists but not needed |

**Missing dependencies with no fallback:** the three locked Go libs + oklog/ulid — all installable via `go get` (Wave 0 task). No external service or system binary beyond git (present).
**Missing dependencies with fallback:** ULID (crypto/rand fallback exists but ULID recommended).

## Validation Architecture

> nyquist_validation is enabled (config workflow.nyquist_validation = true).

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go `testing` (stdlib) — existing project convention (`*_test.go` throughout `internal/`) |
| Config file | none — `go test ./...` |
| Quick run command | `go test ./internal/attachments/ -count=1` |
| Full suite command | `go test ./... && (cd web && npm run build)` |

### Phase Requirements → Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| ATT-01 | Upload writes binary+meta via commit | integration | `go test ./internal/server/ -run TestUploadAttachment` | ❌ Wave 0 |
| ATT-02 | Download bytes == upload bytes (byte-exact) | integration | `go test ./internal/server/ -run TestDownloadByteExact` | ❌ Wave 0 |
| ATT-03 | List returns name/size/uploader/date from meta | unit | `go test ./internal/attachments/ -run TestMetaSidecar` | ❌ Wave 0 |
| ATT-04 | Image served inline; SVG only via img | integration | `go test ./internal/server/ -run TestInlineImageDisposition` | ❌ Wave 0 |
| ATT-05 | Replace reuses id, new bytes, re-extract | integration | `go test ./internal/attachments/ -run TestReplaceKeepsID` | ❌ Wave 0 |
| ATT-07 | Orphan delete removes 3 files in one commit | integration | `go test ./internal/attachments/ -run TestOrphanDelete` | ❌ Wave 0 |
| ATT-08 | PDF/DOCX/TXT extract; scanned→empty | unit | `go test ./internal/attachments/ -run TestExtract` | ❌ Wave 0 (needs fixtures) |
| ATT-09 | Oversize rejected (413); bad type rejected (415) | integration | `go test ./internal/server/ -run TestUploadValidation` | ❌ Wave 0 |
| ATT-10 | Upload/delete produce a commit | integration | `go test ./internal/attachments/ -run TestUploadCommits` | ❌ Wave 0 |
| SEC-02 | Non-image → Content-Disposition: attachment | integration | `go test ./internal/server/ -run TestDownloadDisposition` | ❌ Wave 0 |

### Sampling Rate
- **Per task commit:** `go test ./internal/attachments/ -count=1`
- **Per wave merge:** `go test ./... -count=1`
- **Phase gate:** full suite green + `cd web && npm run build` before `/gsd-verify-work`; manual UAT of upload→extract→download→preview→replace→remove.

### Wave 0 Gaps
- [ ] `go get` the four packages + `go mod tidy` + commit go.sum (pin pseudo-versions)
- [ ] `testdata/attachments/` fixtures: `text-layer.pdf`, `scanned-image.pdf`, `sample.docx`, `sample.txt` (CRLF+BOM), `corrupt.pdf`
- [ ] `internal/attachments/extract_test.go` — extract fidelity (covers ATT-08, the "No text extracted" guarantee)
- [ ] `internal/server/handlers_attachments_test.go` — upload/download/validation/disposition (ATT-01/02/04/09, SEC-02)
- [ ] A `commitPayload` JSON-shape test ensuring the attachments struct marshals to what `KindCommit` unmarshals (guards Open Question 2)

## Security Domain

> security_enforcement = true, ASVS level 1, block_on = high.

### Applicable ASVS Categories
| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | yes (inherited) | Existing session auth; attachment routes behind `loadCurrentUser`. |
| V4 Access Control | yes | Mutations behind `auth.RequireRole(editor)` on the existing editor group; reads available to any authenticated user (mirrors pages). Authority from session, never client input. |
| V5 Input Validation | **yes (core)** | `MaxBytesReader` size cap; `mimetype.Detect` magic-byte sniff against allow-list (ATT-09); server-generated ULID path (never the filename); `repo.Resolve` on every path (SEC-01). |
| V6 Cryptography | minor | `crypto/sha256` for the integrity hash in meta (not a security control, an integrity signal); no secrets handled in this phase. |
| V12 Files & Resources | **yes (core)** | Untrusted upload surface: size cap, type allow-list, opaque storage name, `Content-Disposition: attachment` for risky types, `X-Content-Type-Options: nosniff`, SVG only via `<img>`, path resolver. |

### Known Threat Patterns for Go file-upload + Git-backed storage
| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Path traversal via filename | Tampering | Server ULID path + `repo.Resolve` (SEC-01); filename only in meta. |
| Unrestricted upload size (disk-fill DoS) | DoS | `http.MaxBytesReader` from `config.MaxUploadMB` (Pitfall 1). |
| Disguised file type (e.g. HTML/exe as .png) | Spoofing/Tampering | Magic-byte sniff + allow-list; reject sniffed-type mismatch (ATT-09). |
| Stored XSS via SVG/HTML | Tampering/Elevation | SVG via `<img src>` only; `Content-Disposition: attachment` + `nosniff` for non-images (SEC-02, Pitfall 3). |
| Content-type confusion on download | Spoofing | `X-Content-Type-Options: nosniff`; explicit Content-Type per sniffed type. |
| Parser panic on malformed upload (drain DoS) | DoS | `recover()` in the ExtractJob; worker retry/terminal-fail (Pitfall 5). |
| Privilege bypass on mutation | Elevation | `RequireRole(editor)` from session; client hiding is UX only. |

No findings rated high block this phase — every threat has a standard, in-stack mitigation already designed into the patterns above.

## Sources

### Primary (HIGH confidence)
- `proxy.golang.org/<module>/@latest` — verified pinned versions: ledongthuc/pdf (v0.0.0-20250511090121-5959a4027728), fumiama/go-docx (v0.0.0-20250506085032-0c30fd09304b), gabriel-vasile/mimetype (v1.4.13), oklog/ulid/v2 (v2.1.1).
- Source read at pinned commits: `ledongthuc/pdf` page.go (L64 `GetPlainText`, L521 page-level), pdf.go (L105 `Open`, L125 `NewReader`); `fumiama/go-docx` docx.go (L93 `Parse`), structpara.go (L194 `(*Paragraph).String()`), README example; `gabriel-vasile/mimetype` mimetype.go (`Detect`/`DetectReader`/`DetectFile`/`SetLimit`), mime.go (`Extension` includes leading dot, `Is`, `Parent`).
- Repo source read this session: `internal/jobs/{queue,worker}.go`, `internal/pages/{commitjob,service}.go`, `internal/gitstore/{commit,read}.go`, `internal/repo/{path,files}.go`, `internal/config/config.go`, `internal/server/router.go`.
- GitHub API repo health: stars/age/archived/pushed for all four libs (all non-archived, maintained).
- Go 1.26 + git 2.47.3 confirmed present in env.

### Secondary (MEDIUM confidence)
- CLAUDE.md locked stack table + "What NOT to Use" (project authority); CONTEXT.md + UI-SPEC.md (phase decisions).
- gsd classify-confidence seam: context7+verified → MEDIUM, websearch → LOW.

### Tertiary (LOW confidence)
- General Git large-binary storage guidance (web search was unavailable this session; large-binary mechanics rest on the well-established Git object/pack model and are marked VERIFIED for mechanics, ASSUMED for usage-profile claims A1–A3).

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — all versions verified on the Go proxy and APIs read from source at the pinned commits.
- Architecture: HIGH — all integration spines read from the live repo source; the design reuses them verbatim.
- Extraction fidelity: HIGH for APIs/contract, MEDIUM for edge-case layout coverage (A4/A5).
- Large-binary policy: HIGH for Git mechanics and the locked-decision workability; MEDIUM for the usage-profile assumptions (A1–A3) — these are guardrail thresholds, not blockers.
- Pitfalls: HIGH — grounded in stdlib semantics, the libs' nature, and Phase-0/1 invariants.

**Research date:** 2026-06-21
**Valid until:** 2026-07-21 (stable; the pinned Go libs are pseudo-versioned and slow-moving — re-verify pdf/docx pseudo-versions only if a new upstream commit is intentionally adopted).
