# Phase 2: Attachments & Text Extraction — Pattern Map

**Mapped:** 2026-06-21
**Files analyzed:** 18 (9 backend, 9 frontend)
**Analogs found:** 18 / 18 (every file has a same-codebase analog)

---

## File Classification

| New / Modified File | Role | Data Flow | Closest Analog | Match Quality |
|---------------------|------|-----------|----------------|---------------|
| `internal/attachments/service.go` | service | CRUD + file-I/O | `internal/pages/service.go` + `internal/pages/trash.go` | exact role; multi-file commit pattern from trash.go |
| `internal/attachments/commitjob.go` | service (commit wiring) | request-response | `internal/pages/commitjob.go` | exact — same `EnqueueCommit` / `commitPayload` call-site pattern |
| `internal/attachments/extractjob.go` | service (async worker) | event-driven | `internal/jobs/queue.go` (Handler typedef) + `internal/pages/commitjob.go` (CommitHandler) | role-match; first new `jobs.Handler` besides CommitHandler |
| `internal/attachments/types.go` | model | — | `internal/pages/trash.go` (`TrashEntry`) + `internal/pages/service.go` (`Page`) | role-match (struct definitions + sentinel errors) |
| `internal/server/handlers_attachments.go` | controller | request-response + file-I/O | `internal/server/handlers_pages.go` + `internal/server/handlers_trash.go` | exact — same chi, `cleanPathParam`, `writeError`/`writeJSON`, audit, RBAC gate pattern |
| `internal/server/handlers_attachments_test.go` | test | request-response | `internal/server/handlers_pages_test.go` | exact |
| `internal/server/handlers_sse.go` | controller | event-driven (SSE stream) | `internal/server/handlers_health.go` (nearest; no existing SSE) | partial; no existing SSE — use health handler structure as baseline |
| `internal/config/config.go` (modify) | config | — | itself — `AttachmentsConfig` struct already present (lines 113–116) | exact — extend existing placeholder |
| `internal/store/migrations.go` (modify) | migration | — | itself — add `attachments` table DDL in the established idempotent `CREATE TABLE IF NOT EXISTS` pattern | exact |
| `web/src/api/client.ts` (modify) | utility | request-response | itself — `mutate()` for mutations; raw `fetch` for GET list + SSE `EventSource` | exact — extend the file following existing function signatures |
| `web/src/components/AttachmentsSection.tsx` | component | request-response | `web/src/routes/PageView.tsx` (section host) + `web/src/components/TrashView.tsx` (list + empty state) | role-match |
| `web/src/components/AttachmentsSection.css` | config (CSS) | — | `web/src/routes/PageView.css` (co-located, token-only) | exact |
| `web/src/components/AttachmentCard.tsx` | component | request-response | `web/src/components/DeleteConfirmDialog.tsx` (mutation + Dialog wiring) + `web/src/components/TrashView.tsx` (row with actions) | role-match |
| `web/src/components/AttachmentCard.css` | config (CSS) | — | `web/src/components/TrashView.css` (card/row token-only styles) | role-match |
| `web/src/components/AttachmentDropzone.tsx` | component | file-I/O | `web/src/components/CreatePageModal.tsx` (form with validation + useMutation) | role-match; first `react-dropzone` usage — no existing dropzone |
| `web/src/components/ExtractionStatus.tsx` | component | event-driven | `web/src/components/AutosaveStatus.tsx` | exact — copy `.autosave-status` flex structure verbatim |
| `web/src/components/ReplaceAttachmentDialog.tsx` | component | request-response | `web/src/components/DeleteConfirmDialog.tsx` | exact — same `Dialog` + `useMutation` + `queryClient.invalidateQueries` pattern |
| `web/src/components/RemoveAttachmentDialog.tsx` | component | request-response | `web/src/components/DeleteConfirmDialog.tsx` | exact — same pattern; `destructive` prop set |

---

## Pattern Assignments

### `internal/attachments/service.go` (service, CRUD + file-I/O)

**Analog:** `internal/pages/service.go` and `internal/pages/trash.go`

**Package declaration + sentinel errors** (`internal/pages/service.go` lines 1–29):
```go
package attachments

import (
    "context"
    "database/sql"
    "errors"
    "fmt"
    "time"

    "github.com/postfix/okworkspace/internal/gitstore"
    "github.com/postfix/okworkspace/internal/jobs"
    "github.com/postfix/okworkspace/internal/repo"
)

var (
    ErrAttachmentNotFound = errors.New("attachment not found")
    ErrPageNotFound       = errors.New("page not found")
    ErrTypeForbidden      = errors.New("file type not allowed")
    ErrTooLarge           = errors.New("file too large")
)
```

**Service struct + constructor** (mirror `internal/pages/service.go` lines 66–96):
```go
// Inject *repo.Repo, *gitstore.GitStore, *jobs.Worker, *sql.DB, pushOnCommit bool.
// Expose enqueuer interface (same as pages.enqueuer) so tests can inject a fake worker.
type Service struct {
    repo         *repo.Repo
    git          reviser        // BlobRevision (unused ATT-phase; keep for symmetry)
    worker       enqueuer       // Enqueue / EnqueueAndWait — copy the interface from pages/service.go lines 47–50
    db           *sql.DB
    pushOnCommit bool
    now          func() time.Time
}
```

**Multi-file commit (write binary + meta sidecar + remove on delete)** (mirror `internal/pages/trash.go` lines 79–91):
```go
// Upload: Writes = [{id.ext, bytes}, {id.json, metaJSON}]; Removes = []
// Delete: Writes = []; Removes = [id.ext, id.json, id.txt (if present)]
// Replace: Writes = [{id.ext, newBytes}, {id.json, updatedMetaJSON}]; Removes = []
// All committed through pages.EnqueueCommit (import it from pages package or copy
// the helper verbatim into attachments package — same call signature):
p := commitPayload{
    Writes:  []fileWrite{{Path: binPath, Bytes: raw}, {Path: metaPath, Bytes: metaJSON}},
    Spec: gitstore.CommitSpec{
        Paths:   []string{binPath, metaPath},
        Message: "Attach " + originalName + " to " + pagePath,
        User:    user,
        Action:  "attach",
        Source:  "web-ui",
    },
    Push: s.pushOnCommit,
}
return EnqueueCommit(ctx, s.worker, p)
```

**Safe-path guard before staging** (mirror `internal/pages/service.go` lines 250–252):
```go
// Call s.repo.Resolve(attachPath) before adding to commitPayload.Writes.
// Return the resolver error directly; it gives a clean path-safety rejection.
if _, err := s.repo.Resolve(binPath); err != nil {
    return err
}
```

**Error handling:** Return sentinel errors (`ErrAttachmentNotFound`, `ErrTypeForbidden`, `ErrTooLarge`) from service methods; map them to HTTP status codes in the handler (see handlers section below). Never return raw `os` or `git` errors directly — wrap with `fmt.Errorf("attachments: <op> %q: %w", path, err)`.

---

### `internal/attachments/commitjob.go` (commit wiring)

**Analog:** `internal/pages/commitjob.go`

The `commitPayload` / `fileWrite` / `EnqueueCommit` types and function defined in `internal/pages/commitjob.go` are the canonical single-writer path. Phase 2 has two options:

1. **Import from `pages` package** — simplest; no duplication. `attachments.Service` calls `pages.EnqueueCommit(ctx, worker, p)` directly.
2. **Duplicate the minimal types** — only if a circular import forces it.

**Reuse `KindCommit`** — do NOT register a second commit kind. Attachment writes flow through the existing `KindCommit` handler (it already handles arbitrary `fileWrite` slices).

**`EnqueueCommit` call pattern** (`internal/pages/commitjob.go` lines 128–142):
```go
// EnqueueCommit(ctx, s.worker, p) — blocks until the commit lands on disk.
// On ErrJobTimeout: log warning + return nil (same timeout-as-soft-success policy).
err = EnqueueCommit(ctx, s.worker, p)
if errors.Is(err, jobs.ErrJobTimeout) {
    slog.WarnContext(ctx, "attachments: commit wait timed out; returning success, job stays queued", ...)
    return nil
}
return err
```

---

### `internal/attachments/extractjob.go` (async job handler, event-driven)

**Analog:** `internal/jobs/queue.go` (`Handler` type) + `internal/pages/commitjob.go` (`CommitHandler` constructor pattern)

**Register a new kind — `KindExtract = "extract"`**. No other code changes to `internal/jobs/queue.go`.

**Handler constructor pattern** (mirror `internal/pages/commitjob.go` lines 53–109):
```go
const KindExtract = "extract"

// extractPayload is JSON-encoded in the job payload.
type extractPayload struct {
    AttachmentID string `json:"attachment_id"`
    BinPath      string `json:"bin_path"`      // repo-relative
    TxtPath      string `json:"txt_path"`      // repo-relative (<id>.txt sidecar)
    MimeType     string `json:"mime_type"`
    PagePath     string `json:"page_path"`     // for commit message
    User         string `json:"user"`
}

func ExtractHandler(r *repo.Repo, g *gitstore.GitStore, w enqueuer, notifier SSENotifier) jobs.Handler {
    return func(ctx context.Context, payload string) error {
        var p extractPayload
        if err := json.Unmarshal([]byte(payload), &p); err != nil {
            return fmt.Errorf("attachments: extract payload: %w", err)
        }
        // 1. Read binary through resolver (repo.Read).
        // 2. Extract text (pdf/docx/txt dispatch).
        // 3. Write <id>.txt sidecar through CommitJob (same commitPayload pattern).
        // 4. Notify SSE subscribers of the new status.
        // Return nil even if extraction yields empty text — "No text" is a valid state.
    }
}
```

**Enqueue after upload** (fire-and-forget, do NOT block):
```go
// Use worker.Enqueue (not EnqueueAndWait) so upload handler returns immediately.
_ = s.worker.Enqueue(ctx, KindExtract, string(raw))
```

---

### `internal/attachments/types.go` (model)

**Analog:** `internal/pages/trash.go` (`TrashEntry` lines 26–34) + `internal/pages/service.go` (`Page` lines 99–105)

```go
// AttachmentMeta is the on-disk <id>.json sidecar (SEC-02: original filename stored
// here, never used for the stored path). Serialized with encoding/json.
type AttachmentMeta struct {
    ID           string `json:"id"`            // ULID or content-hash + ext
    OriginalName string `json:"original_name"` // original upload filename (display only)
    MimeType     string `json:"mime_type"`
    SizeBytes    int64  `json:"size_bytes"`
    UploaderName string `json:"uploader_name"` // display_name from session
    UploadedAt   string `json:"uploaded_at"`   // RFC3339
    PagePath     string `json:"page_path"`     // which page owns this attachment
}

// ExtractionStatus values surfaced to the SSE stream and the GET list.
type ExtractionStatus string
const (
    ExtractionPending   ExtractionStatus = "pending"
    ExtractionDone      ExtractionStatus = "done"
    ExtractionEmpty     ExtractionStatus = "empty"    // no text layer
    ExtractionFailed    ExtractionStatus = "failed"
)

// AttachmentListItem is the GET /attachments response shape (meta + status).
type AttachmentListItem struct {
    AttachmentMeta
    ExtractionStatus ExtractionStatus `json:"extraction_status"`
}
```

---

### `internal/server/handlers_attachments.go` (controller, request-response + file-I/O)

**Analog:** `internal/server/handlers_pages.go` and `internal/server/handlers_trash.go`

**Imports pattern** (mirror `internal/server/handlers_pages.go` lines 1–13):
```go
package server

import (
    "encoding/json"
    "errors"
    "io"
    "net/http"
    "strings"

    "github.com/go-chi/chi/v5"

    "github.com/postfix/okworkspace/internal/attachments"
    "github.com/postfix/okworkspace/internal/audit"
)
```

**Path extraction pattern** (`internal/server/handlers_pages.go` lines 76–97):
```go
// Every handler that takes a page path uses cleanPathParam.
// The attachment id is a chi URL param: chi.URLParam(r, "id") — no traversal risk
// (ULID/hash is opaque), but still validate it is non-empty and NUL-free.
path, ok := cleanPathParam(w, r)
if !ok {
    return
}
```

**nil-guard before every handler** (`internal/server/handlers_pages.go` lines 129–133):
```go
if h.attachments == nil {
    writeError(w, http.StatusInternalServerError, "Something went wrong. Check your connection and try again.")
    return
}
```

**Error mapping** (mirror `internal/server/handlers_pages.go` lines 138–146):
```go
if errors.Is(err, attachments.ErrAttachmentNotFound) {
    writeError(w, http.StatusNotFound, "That attachment no longer exists.")
    return
}
if errors.Is(err, attachments.ErrTypeForbidden) {
    writeError(w, http.StatusUnprocessableEntity, "That file type isn't allowed.")
    return
}
if errors.Is(err, attachments.ErrTooLarge) {
    writeError(w, http.StatusRequestEntityTooLarge, "That file is too large.")
    return
}
writeError(w, http.StatusInternalServerError, "Something went wrong. Check your connection and try again.")
```

**Multipart upload — no existing analog; use stdlib pattern:**
```go
func (h *authHandlers) handleUploadAttachment(w http.ResponseWriter, r *http.Request) {
    // Limit the body BEFORE parsing multipart so an oversized upload is rejected
    // before any bytes are buffered (SEC-02). MaxUploadMB comes from h.config.
    r.Body = http.MaxBytesReader(w, r.Body, int64(h.config.Storage.MaxUploadMB)<<20)
    if err := r.ParseMultipartForm(32 << 20); err != nil {
        writeError(w, http.StatusRequestEntityTooLarge, "That file is too large.")
        return
    }
    f, header, err := r.FormFile("file")
    // ... read io.ReadAll(f), call service.Upload, ...
}
```

**Download handler — serve file bytes with Content-Disposition:**
```go
func (h *authHandlers) handleDownloadAttachment(w http.ResponseWriter, r *http.Request) {
    // GET only; no CSRF needed (read).
    // Service returns ([]byte, AttachmentMeta, error).
    // For images: w.Header().Set("Content-Disposition", "inline")
    // For everything else: w.Header().Set("Content-Disposition", `attachment; filename="<original>"`)
    // Always set Content-Type from the stored mime_type (never infer from request).
    w.Header().Set("Content-Type", meta.MimeType)
    w.Header().Set("Content-Disposition", contentDisposition(meta))
    _, _ = w.Write(raw)
}
```

**Audit pattern** (`internal/server/handlers_pages.go` lines 170–176):
```go
_ = h.audit.Record(r.Context(), audit.Event{
    Action: audit.ActionAttachUpload,  // add new constants to internal/audit/audit.go
    Actor:  h.actorUsername(r.Context()),
    Target: pagePath + "/" + meta.ID,
    Source: auditSourceWeb,
})
```

---

### `internal/server/handlers_sse.go` (controller, SSE stream)

**Analog:** `internal/server/handlers_health.go` (nearest available; no existing SSE in codebase)

**No existing SSE handler — use stdlib SSE pattern:**
```go
func (h *authHandlers) handleExtractionStatus(w http.ResponseWriter, r *http.Request) {
    // loadCurrentUser middleware has already run; session is authenticated.
    attachmentID := chi.URLParam(r, "id")
    if attachmentID == "" {
        writeError(w, http.StatusBadRequest, "Invalid request.")
        return
    }
    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")
    flusher, ok := w.(http.Flusher)
    if !ok {
        writeError(w, http.StatusInternalServerError, "SSE not supported.")
        return
    }
    // Subscribe to notifier; on each status change write:
    // fmt.Fprintf(w, "data: %s\n\n", jsonStatus)
    // flusher.Flush()
    // On r.Context().Done() return (client disconnect).
}
```

**Route registration** (extend `internal/server/router.go` inside the `authed` group — any authenticated user can stream extraction status, same as reading a page):
```go
// GET only, no CSRF (read-only stream).
authed.Get("/attachments/{id}/status", h.handleExtractionStatus)
```

---

### `internal/config/config.go` (modify — extend `AttachmentsConfig`)

**Analog:** itself. `AttachmentsConfig` is already a placeholder at lines 113–116.

**Add `MaxUploadMB` forwarding** — `StorageConfig.MaxUploadMB` (line 53) already exists. The `AttachmentsConfig` needs only `AllowedExtensions` (already there) and `ExtractText` (already there). Wire `config.Storage.MaxUploadMB` into the upload handler directly rather than duplicating it.

---

### `internal/store/migrations.go` (modify — add `attachments` table)

**Analog:** itself. Follow the idempotent `CREATE TABLE IF NOT EXISTS` pattern used by all existing migrations.

```sql
-- Add after existing table DDL:
CREATE TABLE IF NOT EXISTS attachments (
    id            TEXT PRIMARY KEY,          -- ULID
    page_path     TEXT NOT NULL,
    original_name TEXT NOT NULL,
    mime_type     TEXT NOT NULL,
    size_bytes    INTEGER NOT NULL,
    uploader_name TEXT NOT NULL,
    uploaded_at   TEXT NOT NULL,
    extract_status TEXT NOT NULL DEFAULT 'pending',
    extract_error TEXT
);
```

---

### `web/src/api/client.ts` (modify — add attachment API functions)

**Analog:** itself. Follow the established patterns exactly:
- GETs: raw `fetch` + `credentials: "same-origin"` + specific error messages (pattern: `listTrash`, lines 304–311).
- Mutations: `mutate<T>(path, body, method)` helper (lines 50–85).
- File upload (multipart): raw `fetch` with FormData — **do NOT use `mutate()`** (which sets `Content-Type: application/json`). Fetch the CSRF token via `ensureCSRF()` and set `X-CSRF-Token` header manually.
- SSE: `new EventSource(url)` — no CSRF needed (GET).

**New interfaces to add** (mirror `Page` at lines 203–207, `TrashEntry` at lines 290–296):
```typescript
export interface AttachmentMeta {
  id: string;
  original_name: string;
  mime_type: string;
  size_bytes: number;
  uploader_name: string;
  uploaded_at: string;
  page_path: string;
  extraction_status: "pending" | "done" | "empty" | "failed";
}
```

**Upload function — multipart pattern (no existing analog; use `ensureCSRF` manually):**
```typescript
export async function uploadAttachment(
  pagePath: string,
  file: File,
): Promise<AttachmentMeta> {
  const token = await ensureCSRF();
  const form = new FormData();
  form.append("file", file);
  form.append("page_path", pagePath);
  const res = await fetch("/api/v1/attachments", {
    method: "POST",
    credentials: "same-origin",
    headers: { [CSRF_HEADER]: token },
    body: form,
    // Do NOT set Content-Type — the browser must set it with the multipart boundary.
  });
  // error handling mirrors mutate(): try res.json() for error message
  ...
}
```

**SSE subscription:**
```typescript
export function subscribeExtractionStatus(
  id: string,
  onStatus: (status: string) => void,
): () => void {
  const es = new EventSource(`/api/v1/attachments/${id}/status`);
  es.onmessage = (e) => onStatus(e.data);
  return () => es.close();
}
```

---

### `web/src/components/ExtractionStatus.tsx` (component, event-driven)

**Analog:** `web/src/components/AutosaveStatus.tsx` — copy the entire structure verbatim.

**Direct copy pattern** (`web/src/components/AutosaveStatus.tsx` lines 1–34):
```tsx
import { Check, Loader2, FileText, AlertCircle } from "lucide-react";
import "./AutosaveStatus.css";  // reuse the same CSS file — no new CSS for this component

export type ExtractionState = "pending" | "extracting" | "done" | "empty" | "failed";

export default function ExtractionStatus({ state }: { state: ExtractionState }) {
  if (state === "pending" || state === "extracting") {
    return (
      <span className="autosave-status autosave-muted" aria-live="polite">
        <Loader2 size={14} aria-hidden="true" className="autosave-spinner" />
        Extracting text…
      </span>
    );
  }
  if (state === "done") {
    return (
      <span className="autosave-status" style={{ color: "var(--color-success)" }} aria-live="polite">
        <Check size={14} aria-hidden="true" />
        Text extracted
      </span>
    );
  }
  if (state === "empty") {
    return (
      <span className="autosave-status" style={{ color: "var(--color-warning)" }} aria-live="polite">
        <FileText size={14} aria-hidden="true" />
        No text extracted
      </span>
    );
  }
  // failed
  return (
    <span className="autosave-status" style={{ color: "var(--color-destructive)" }} aria-live="polite">
      <AlertCircle size={14} aria-hidden="true" />
      Couldn't extract text
    </span>
  );
}
```

Key conventions:
- `aria-live="polite"` on the span (same as `AutosaveStatus`).
- Icons at size 14, `aria-hidden="true"`.
- CSS class `autosave-status` (existing — no new CSS file).
- `autosave-spinner` class on the `Loader2` for the spinning animation.
- Color via `var(--…)` tokens inline (or via a new utility class in `AutosaveStatus.css`).

---

### `web/src/components/ReplaceAttachmentDialog.tsx` and `RemoveAttachmentDialog.tsx`

**Analog:** `web/src/components/DeleteConfirmDialog.tsx` — copy structure exactly.

**Full pattern** (`web/src/components/DeleteConfirmDialog.tsx` lines 1–69):
```tsx
import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import Dialog from "./Dialog";
import { removeAttachment } from "../api/client";

export default function RemoveAttachmentDialog({ open, attachmentId, pagePath, filename, onClose }) {
  const [error, setError] = useState<string | null>(null);
  const queryClient = useQueryClient();

  const removeMut = useMutation({
    mutationFn: () => removeAttachment(pagePath, attachmentId),
    onSuccess: () => {
      // Invalidate the attachment list query for this page.
      queryClient.invalidateQueries({ queryKey: ["attachments", pagePath] });
      onClose();
    },
    onError: () => {
      setError("We couldn't remove that attachment just now. Try again.");
    },
  });

  return (
    <Dialog
      open={open}
      title="Remove attachment"
      onCancel={onClose}
      onConfirm={() => { setError(null); removeMut.mutate(); }}
      confirmLabel="Remove"
      cancelLabel="Cancel"
      destructive      // RemoveDialog only; ReplaceDialog omits this
      busy={removeMut.isPending}
    >
      <p className="dialog-message">
        Remove "{filename}" from this page? If no other page uses it, the file is deleted.
      </p>
      {error && <p className="field-help" role="alert">{error}</p>}
    </Dialog>
  );
}
```

**React-query key convention:** `["attachments", pagePath]` — mirrors `["page", path]` used by `PageView.tsx` line 34.

---

### `web/src/components/AttachmentCard.tsx` (component)

**Analog:** `web/src/components/TrashView.tsx` (row with action buttons) + `web/src/components/DeleteConfirmDialog.tsx` (mutation + Dialog open state)

**Pattern:**
- `canEdit` prop driven by `meData?.role === "editor" || meData?.role === "admin"` — exact same gate as `PageView.tsx` line 27.
- Two action buttons (Replace / Remove) rendered only when `canEdit`.
- `className="btn btn-ghost"` for Replace, `className="btn btn-ghost-destructive"` for Remove — matching Phase 1 row-action pattern.
- `aria-label` on icon-only buttons (no visible label text).
- Image thumbnail: `<img src={downloadUrl} ... />` for png/jpg/svg; lucide `FileText` icon for others.
- SVG served via `<img src>` only — never `dangerouslySetInnerHTML` (stored-XSS guard from `MarkdownProse.tsx`).
- `ExtractionStatus` chip rendered only for extractable types (pdf/docx/txt).

---

### `web/src/components/AttachmentDropzone.tsx` (component, file-I/O)

**Analog:** `web/src/components/CreatePageModal.tsx` (form + validation + `useMutation`) — nearest; no existing `react-dropzone` usage.

**Key conventions (no existing analog — derive from UI-SPEC):**
- Client-side pre-checks (size > `maxUploadMB`, MIME/extension not in allowlist) **before** the POST. Reject state shown inline (`.field-error`), no network round-trip.
- `useDropzone` from `react-dropzone`; `accept` prop built from `allowedExtensions` config (fetched via a `/api/v1/config` endpoint or threaded as props from `AttachmentsSection`).
- Upload via `uploadAttachment()` client function (multipart, CSRF-aware — see api/client.ts section).
- `useMutation` from `@tanstack/react-query` for the upload call; `queryClient.invalidateQueries({ queryKey: ["attachments", pagePath] })` on success.
- State machine: idle → drag-over → uploading → success (transient) / error. Driven by `isDragActive` from `useDropzone` + `isPending` from `useMutation`.

---

### `web/src/routes/PageView.tsx` (modify — mount `AttachmentsSection`)

**Analog:** itself (lines 75–124).

**Mount point:** After `<MarkdownProse>` inside `<article className="pageview">`, separated by `--space-xl`. In read mode only (not mounted in `PageEditor.tsx` this phase).

```tsx
// Add after MarkdownProse inside the article:
<AttachmentsSection
  pagePath={path}
  canEdit={canEdit}
  maxUploadMb={/* from a /api/v1/config GET or a constant */}
  allowedExtensions={/* same source */}
/>
```

**Import pattern** (mirror existing imports at lines 1–16 of PageView.tsx):
```tsx
import AttachmentsSection from "../components/AttachmentsSection";
```

---

## Shared Patterns

### 1. Single-Writer Commit Gate
**Source:** `internal/pages/commitjob.go` lines 128–142, `internal/pages/service.go` lines 247–265
**Apply to:** `internal/attachments/service.go` (Upload, Replace, Delete methods)

Every attachment write (binary + meta sidecar) and every delete (removes) flows through `EnqueueCommit` with `commitPayload{Writes, Removes, Spec, Push}`. Never call `os.WriteFile`, `repo.Write` directly from HTTP handlers, or shell out to git. The CommitHandler already handles arbitrary `fileWrite` slices — no new commit kind needed.

### 2. Safe-Path Resolver Gate (SEC-01)
**Source:** `internal/pages/service.go` lines 250–252, `internal/repo/path.go`
**Apply to:** `internal/attachments/service.go` before every `commitPayload` construction

```go
if _, err := s.repo.Resolve(path); err != nil {
    return err
}
```

Call `repo.Resolve` on both the binary path (`attachments/<id>.<ext>`) and the sidecar paths before staging them. The CommitHandler re-resolves as a backstop, but failing here gives a clean error.

### 3. Editor RBAC Gate
**Source:** `internal/server/router.go` lines 106–123 (`auth.RequireRole(auth.RoleEditor)` subgroup)
**Apply to:** `internal/server/router.go` — register upload/replace/delete under the existing `editor` subgroup; download and SSE status under the `authed` group (any authenticated user).

```go
// Under authed (any authenticated user):
authed.Get("/attachments/{pagePath}", h.handleListAttachments)
authed.Get("/attachments/{id}/download", h.handleDownloadAttachment)
authed.Get("/attachments/{id}/status", h.handleExtractionStatus)

// Under editor (editor/admin only):
editor.Post("/attachments", h.handleUploadAttachment)
editor.Put("/attachments/{id}", h.handleReplaceAttachment)
editor.Delete("/attachments/{id}", h.handleDeleteAttachment)
```

Frontend mirrors this: `canEdit` prop (derived from `meData?.role`) gates Upload/Replace/Remove affordances, matching `PageView.tsx` line 27.

### 4. `writeError` / `writeJSON` Response Formatting
**Source:** `internal/server/middleware.go` (defines `writeError`/`writeJSON` used throughout `handlers_pages.go`)
**Apply to:** `internal/server/handlers_attachments.go`, `internal/server/handlers_sse.go`

All handlers use `writeError(w, statusCode, "User-facing message.")` and `writeJSON(w, statusCode, struct{})`. The user-facing message is the full sentence from the UI-SPEC copywriting contract. Never return raw error strings from the service layer.

### 5. Audit Recording
**Source:** `internal/server/handlers_pages.go` lines 170–176, `internal/audit/audit.go`
**Apply to:** `internal/server/handlers_attachments.go` (Upload, Replace, Delete handlers)

Add new action constants to `internal/audit/audit.go`:
```go
const (
    ActionAttachUpload  = "attach_upload"
    ActionAttachReplace = "attach_replace"
    ActionAttachDelete  = "attach_delete"
)
```

Record after the service call succeeds (same placement as existing handlers).

### 6. React-Query Key Convention
**Source:** `web/src/routes/PageView.tsx` line 34 (`queryKey: ["page", path]`)
**Apply to:** All attachment query/mutation hooks

Use `["attachments", pagePath]` as the canonical query key for a page's attachment list. Invalidate this key on Upload, Replace, and Delete mutations.

### 7. Dialog + useMutation Component Pattern
**Source:** `web/src/components/DeleteConfirmDialog.tsx` (full file)
**Apply to:** `ReplaceAttachmentDialog.tsx`, `RemoveAttachmentDialog.tsx`

Structure:
1. Local `error` state for mutation errors.
2. `useMutation` with `onSuccess` → `queryClient.invalidateQueries` + `onClose()`.
3. `onError` → set local error string.
4. `Dialog` with `busy={mutation.isPending}`.
5. Error displayed as `<p className="field-help" role="alert">`.

### 8. Token-Only Styling (no new hex/px)
**Source:** `web/src/styles/tokens.css`, established by Phase 0/1
**Apply to:** All new `.css` files (`AttachmentsSection.css`, `AttachmentCard.css`)

Reference `var(--…)` tokens only. Never introduce new hex color values or px sizes not already in a token. Co-locate each component's CSS in `<ComponentName>.css` alongside the `.tsx` file.

---

## No Analog Found

| File | Role | Data Flow | Reason |
|------|------|-----------|--------|
| Multipart upload handler | controller | file-I/O | No existing `r.ParseMultipartForm` / `r.FormFile` in codebase; use stdlib `net/http` multipart API with `http.MaxBytesReader` size guard |
| SSE handler | controller | event-driven | No existing SSE endpoint; use stdlib `text/event-stream` pattern with `http.Flusher` |
| `react-dropzone` integration | component | file-I/O | First use of `react-dropzone` in the codebase; follow its own docs for `useDropzone` hook + `isDragActive` state |
| MIME-sniff validation | service | — | First use of `github.com/gabriel-vasile/mimetype`; read magic bytes from the uploaded file head and compare against `config.Attachments.AllowedExtensions` |
| `ExtractJob` text extraction | service | event-driven | First async job besides CommitJob; handler constructor mirrors CommitHandler but extracts text instead of writing git commits |

---

## Metadata

**Analog search scope:** `internal/` (all packages), `web/src/` (all components/routes/api)
**Files scanned:** 12 source files read in full
**Pattern extraction date:** 2026-06-21
