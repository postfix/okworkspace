# Phase 3: Search — Pattern Map

**Mapped:** 2026-06-21
**Files analyzed:** 15 (8 backend, 7 frontend)
**Analogs found:** 15 / 15

---

## File Classification

| New / Modified File | Role | Data Flow | Closest Analog | Match Quality |
|---------------------|------|-----------|----------------|---------------|
| `internal/search/index.go` | service | file-I/O + event-driven | `internal/attachments/extractjob.go` | role-match |
| `internal/search/mapping.go` | config | transform | `internal/config/config.go` (`SearchConfig`) | partial |
| `internal/search/indexjob.go` | service (job handler) | event-driven | `internal/attachments/extractjob.go` | **exact** |
| `internal/search/rebuild.go` | utility (batch) | batch + file-I/O | `internal/attachments/extractjob.go` + `internal/pages/trash.go` | role-match |
| `internal/server/handlers_search.go` | controller | request-response | `internal/server/handlers_pages.go` | **exact** |
| `internal/server/router.go` *(modified)* | route | request-response | itself (authed group) | **exact** |
| `internal/config/config.go` *(modified)* | config | — | itself (`SearchConfig` stub) | **exact** |
| `cmd/okf-workspace/main.go` *(modified)* | config | — | itself (worker.Register + startup wiring) | **exact** |
| `web/src/components/search/SearchPalette.tsx` | component | request-response | `web/src/components/Dialog.tsx` | role-match |
| `web/src/components/search/SearchPalette.css` | config (styles) | — | `web/src/routes/AppShell.css` (`.navrow`, `.dialog-backdrop`) | **exact** |
| `web/src/components/search/SearchResultRow.tsx` | component | transform | `web/src/components/RoleBadge.tsx` + LeftTree `.navrow` | role-match |
| `web/src/api/client.ts` *(modified)* | utility (API client) | request-response | itself (`getTree` / `getPage` GET pattern) | **exact** |
| `web/src/routes/AppShell.tsx` *(modified)* | component | event-driven | itself (topbar-right `repo-health` trigger) | **exact** |
| `web/src/store/searchStore.ts` | store | event-driven | (no zustand store exists yet) | no-analog |
| `web/src/hooks/useSearch.ts` | hook | request-response | `web/src/components/LeftTree.tsx` useQuery pattern | role-match |

---

## Pattern Assignments

### `internal/search/indexjob.go` — KindIndex job handler (service, event-driven)

**Analog:** `internal/attachments/extractjob.go` (lines 1–146)

**Imports pattern** (extractjob.go lines 1–11):
```go
package attachments

import (
    "context"
    "database/sql"
    "encoding/json"
    "fmt"

    "github.com/postfix/okworkspace/internal/gitstore"
    "github.com/postfix/okworkspace/internal/jobs"
)
```
For `indexjob.go` replace `gitstore` with `bleve` and the search package itself.

**Job kind constant** (extractjob.go line 19):
```go
const KindExtract = "extract"
```
Copy as:
```go
const KindIndex = "index"
```

**Payload struct** (extractjob.go lines 25–35):
```go
type extractPayload struct {
    AttachmentID string `json:"attachment_id"`
    BinPath      string `json:"bin_path"`
    TxtPath      string `json:"txt_path"`
    Ext          string `json:"ext"`
    PagePath     string `json:"page_path"`
    User         string `json:"user"`
}
```
Copy shape — fields will be `PagePath string`, `Kind string` (page/attachment/heading), `Op string` (upsert/delete).

**Handler constructor shape** (extractjob.go lines 59–65):
```go
func ExtractHandler(r binaryReader, w enqueuer, db *sql.DB, pushOnCommit bool) jobs.Handler {
    return func(ctx context.Context, payload string) (err error) {
        defer func() {
            if rec := recover(); rec != nil {
                err = fmt.Errorf("attachments: extract handler panic: %v", rec)
            }
        }()
        var p extractPayload
        if uerr := json.Unmarshal([]byte(payload), &p); uerr != nil {
            return fmt.Errorf("attachments: extract payload: %w", uerr)
        }
```
`IndexHandler` follows this exact shape: `func IndexHandler(idx *search.Index, r *repo.Repo) jobs.Handler`. Include the `defer recover()` — Bleve panics on a corrupted index are possible.

**FIRE-AND-FORGET rule** (extractjob.go lines 119–128) — THIS IS THE CRITICAL CR-01 LESSON:
```go
// FIRE-AND-FORGET enqueue (NOT EnqueueAndWait): this handler runs ON the
// single worker drain goroutine, so the KindCommit job it queues can only be
// drained AFTER this handler returns. Waiting here would deadlock the worker
// until commitWaitTimeout and stall every queued page-save/upload behind it
// (CR-01). Enqueue returns once the job row is persisted; the commit lands on
// the very next drain iteration (FIFO), through the single-writer KindCommit
// spine (ATT-10).
if cerr := w.Enqueue(ctx, kindCommit, string(raw)); cerr != nil {
```
`IndexHandler` itself **does not** re-enqueue a downstream job. But if in the future `IndexHandler` ever triggers a follow-on job, use `w.Enqueue` (fire-and-forget), never `w.EnqueueAndWait`. The handler runs on the drain goroutine.

**Error handling in handler** (extractjob.go lines 85–97):
```go
text, eerr := Extract(p.Ext, data)
if eerr != nil {
    _ = setExtractStatus(ctx, db, p.AttachmentID, ExtractionFailed, eerr.Error())
    return eerr
}
```
For `IndexHandler`: if Bleve `Index()` returns an error, log it and return the error — the worker retries with backoff (up to `MaxAttempts`). Do NOT panic or swallow silently.

---

### `internal/search/index.go` — Bleve lifecycle service (service, file-I/O)

**Analog:** `internal/attachments/extractjob.go` + `internal/pages/service.go`

**Service struct pattern** (pages/service.go lines 67–80):
```go
type Service struct {
    repo   *repo.Repo
    git    reviser
    worker enqueuer
    db     *sql.DB
    pushOnCommit bool
    now func() time.Time
}
```
`search.Index` struct holds `bleveIndex bleve.Index`, `dataDir string`. Constructor opens or creates the Bleve on-disk index under `filepath.Join(dataDir, "index")`.

**Interface injection pattern** (pages/service.go lines 47–51):
```go
type enqueuer interface {
    Enqueue(ctx context.Context, kind, payload string) error
    EnqueueAndWait(ctx context.Context, kind, payload string, timeout time.Duration) error
}
```
Define a narrow `indexer` interface if `IndexHandler` needs to call back into `search.Index`, so tests can inject a fake without a real Bleve.

**Startup wiring pattern** (main.go lines 183–194):
```go
worker := jobs.New(st.DB(), jobs.Config{})
worker.SetLogger(logger)
worker.Register(pages.KindCommit, pages.CommitHandler(contentRepo, gs))
worker.Register(attachments.KindExtract, attachments.ExtractHandler(contentRepo, worker, st.DB(), cfg.Git.PushOnCommit))
worker.Start(ctx)
defer worker.Stop()
```
Add after `attachments.KindExtract` registration:
```go
worker.Register(search.KindIndex, search.IndexHandler(searchIdx, contentRepo))
```

**Startup drift detection** should run BEFORE `worker.Start` — after `gs.Init` and after `ReconcileTrash`, in the same startup sequence block. Follow the `SelfHealStaleLock` → `PullOnStartup` → seeded checks pattern (main.go lines 154–181): add a `searchIdx.DriftCheck(ctx, gs)` that compares stored HEAD with current HEAD and enqueues a full rebuild job if they differ.

---

### `internal/search/rebuild.go` — full rebuild from files (utility, batch)

**Analog:** `internal/pages/service.go` `ReconcileTrash` (main.go lines 215–220):
```go
if pruned, err := pagesSvc.ReconcileTrash(context.Background()); err != nil {
    logger.Warn("trash reconcile failed at startup", slog.String("error", err.Error()))
} else if pruned > 0 {
    logger.Info("pruned phantom trash rows at startup", slog.Int("pruned", pruned))
}
```
Rebuild is **best-effort**: a rebuild error must not block the server from starting — log at Warn, continue. Rebuild walks the content repo (via `repo.Repo`), reads each `.md` via `repo.Read` (never `os.*` — SEC-01), parses via `okf.Parse`, and indexes each doc.

**Trash exclusion** (pages/trash.go lines 18–19):
```go
const trashDir = ".okf-workspace/trash"
```
Skip any path with the `trashDir` prefix during rebuild — trashed pages must never appear in search results (SRCH-06).

---

### `internal/search/mapping.go` — Bleve index mapping (config, transform)

**Analog:** `internal/config/config.go` `SearchConfig` (lines 107–110):
```go
type SearchConfig struct {
    Enabled bool   `yaml:"enabled"`
    Engine  string `yaml:"engine"`
}
```
Phase 3 expands this struct to add `IndexDir string` (defaults to `filepath.Join(DataDir, "index")`). The mapping builder function lives in `search/mapping.go` and is called only by `search.Index` constructor, keeping the Bleve API surface local to the `search` package.

---

### `internal/server/handlers_search.go` — search HTTP handler (controller, request-response)

**Analog:** `internal/server/handlers_pages.go` (lines 1–177)

**Imports pattern** (handlers_pages.go lines 1–13):
```go
package server

import (
    "encoding/json"
    "errors"
    "net/http"
    "strings"

    "github.com/go-chi/chi/v5"

    "github.com/postfix/okworkspace/internal/audit"
    "github.com/postfix/okworkspace/internal/pages"
)
```
Replace `pages` with `search` package; keep the same package (`server`).

**writeJSON / writeError helpers** (handlers_auth.go lines 66–74):
```go
func writeJSON(w http.ResponseWriter, status int, v any) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    _ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
    writeJSON(w, status, map[string]string{"error": message})
}
```
These are package-level helpers in the `server` package — use them directly, do NOT re-define.

**Handler method pattern** (handlers_pages.go lines 127–147):
```go
func (h *authHandlers) handleGetPage(w http.ResponseWriter, r *http.Request) {
    if h.pages == nil {
        writeError(w, http.StatusInternalServerError, "Something went wrong. Check your connection and try again.")
        return
    }
    path, ok := cleanPathParam(w, r)
    if !ok {
        return
    }
    page, err := h.pages.Get(r.Context(), path)
    if err != nil {
        if errors.Is(err, pages.ErrPageNotFound) {
            writeError(w, http.StatusNotFound, "This page no longer exists. It may have been moved or deleted.")
            return
        }
        writeError(w, http.StatusInternalServerError, "Something went wrong. Check your connection and try again.")
        return
    }
    writeJSON(w, http.StatusOK, pageResponse(page))
}
```
`handleSearch` follows this shape:
1. guard `h.search == nil` → 500
2. read `q` from `r.URL.Query().Get("q")`; empty query returns 200 with `[]` (fast path, no Bleve call)
3. call `h.search.Query(r.Context(), q)` → results
4. on error: `writeError(w, http.StatusInternalServerError, "Search is unavailable. Try again in a moment.")`
5. on success: `writeJSON(w, http.StatusOK, results)`

No `cleanPathParam` needed — query is a URL parameter, not a path wildcard.

**Admin reindex handler** — same shape; guard nil, call `h.search.Enqueue(ctx, KindRebuild)`, return 202 with `{"ok":true}`.

---

### `internal/server/router.go` *(modified)* — route mounting

**Analog:** itself, lines 84–165

**Authed group mounting** (router.go lines 84–117):
```go
api.Group(func(authed chi.Router) {
    authed.Use(h.loadCurrentUser)
    authed.Get("/tree", h.handleTree)
    authed.Get("/pages/*", h.handleGetPageOrHistory)
    authed.Get("/trash", h.handleListTrash)
    authed.Get("/attachments/*", h.handleGetAttachment)
    // ...
})
```
Add `GET /search` here — any authenticated user may search (CONTEXT.md Area 4). Append:
```go
authed.Get("/search", h.handleSearch)
```

**Admin group mounting** (router.go lines 157–165):
```go
authed.Group(func(admin chi.Router) {
    admin.Use(auth.RequireRole(auth.RoleAdmin))
    admin.Post("/admin/users", h.handleCreateUser)
    // ...
})
```
Add `POST /admin/search/reindex` here:
```go
admin.Post("/admin/search/reindex", h.handleReindex)
```

**Deps struct** (router.go lines 19–42) — add:
```go
// Search is the search service backing GET /search and POST /admin/search/reindex.
// Optional; when nil those routes return a 500 (same optional-dependency pattern as Pages/Attachments).
Search *search.Index
```

---

### `internal/config/config.go` *(modified)*

**Analog:** itself, lines 107–110 (`SearchConfig` stub already exists)

Expand `SearchConfig`:
```go
type SearchConfig struct {
    Enabled  bool   `yaml:"enabled"`
    Engine   string `yaml:"engine"`
    IndexDir string `yaml:"index_dir"` // default: filepath.Join(DataDir, "index")
}
```
Apply default in `applyDefaults()` — follow the existing pattern (lines 145–155):
```go
func (c *Config) applyDefaults() {
    if c.Auth.SessionCookieName == "" {
        c.Auth.SessionCookieName = DefaultSessionCookieName
    }
    // ...
}
```
Add:
```go
if c.Search.IndexDir == "" {
    c.Search.IndexDir = filepath.Join(c.Storage.DataDir, "index")
}
```

---

### `cmd/okf-workspace/main.go` *(modified)* — startup wiring

**Analog:** itself, lines 183–220

**Worker registration pattern** (main.go lines 183–194):
```go
worker := jobs.New(st.DB(), jobs.Config{})
worker.SetLogger(logger)
worker.Register(pages.KindCommit, pages.CommitHandler(contentRepo, gs))
worker.Register(attachments.KindExtract, attachments.ExtractHandler(...))
worker.Start(ctx)
defer worker.Stop()
```
Insert before `worker.Start`:
```go
searchIdx, err := search.Open(cfg.Search.IndexDir)
if err != nil {
    return fmt.Errorf("open search index: %w", err)
}
defer searchIdx.Close()
worker.Register(search.KindIndex, search.IndexHandler(searchIdx, contentRepo))
```

**Best-effort startup action pattern** (main.go lines 215–220):
```go
if pruned, err := pagesSvc.ReconcileTrash(context.Background()); err != nil {
    logger.Warn("trash reconcile failed at startup", slog.String("error", err.Error()))
} else if pruned > 0 {
    logger.Info("pruned phantom trash rows at startup", slog.Int("pruned", pruned))
}
```
After this block, add drift check:
```go
if drifted, err := searchIdx.DriftCheck(ctx, gs); err != nil {
    logger.Warn("search drift check failed; search index may be stale", slog.String("error", err.Error()))
} else if drifted {
    if err := worker.Enqueue(ctx, search.KindIndex, search.RebuildPayload()); err != nil {
        logger.Warn("could not enqueue search rebuild", slog.String("error", err.Error()))
    } else {
        logger.Info("search index drift detected; rebuild queued")
    }
}
```

---

### `web/src/components/search/SearchPalette.tsx` — ⌘K palette (component, event-driven)

**Analog:** `web/src/components/Dialog.tsx` (lines 1–141)

**Focus-trap pattern** (Dialog.tsx lines 53–94) — DO NOT re-implement from scratch; replicate exactly:
```typescript
useEffect(() => {
    if (!open) return;
    previouslyFocused.current = document.activeElement;
    const node = dialogRef.current;
    const focusable = node?.querySelector<HTMLElement>(
      'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])',
    );
    focusable?.focus();

    function onKeyDown(e: KeyboardEvent) {
      if (e.key === "Escape") {
        e.preventDefault();
        onCancelRef.current();
        return;
      }
      // Tab trap ...
    }
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("keydown", onKeyDown);
      (previouslyFocused.current as HTMLElement | null)?.focus?.();
    };
}, [open]);
```
The palette must reproduce `previouslyFocused` restore on close (Dialog.tsx line 89) and the `onCancelRef` stable-ref pattern (lines 46–51) so re-renders do not re-fire the focus effect.

**onCancelRef stable-ref pattern** (Dialog.tsx lines 46–51):
```typescript
const onCancelRef = useRef(onCancel);
useEffect(() => {
    onCancelRef.current = onCancel;
}, [onCancel]);
```

**Backdrop click pattern** (Dialog.tsx lines 100–106):
```typescript
<div
  className="dialog-backdrop"
  onMouseDown={(e) => {
    if (e.target === e.currentTarget) onCancel();
  }}
>
```
SearchPalette reuses `.dialog-backdrop` CSS class (same scrim, same z-index 100). Backdrop click closes — search is read-only so there is no destructive-action guard.

**Early return when not open** (Dialog.tsx line 96):
```typescript
if (!open) return null;
```
Same in SearchPalette — gate renders on `open` state from the zustand store.

**Imports pattern** (Dialog.tsx lines 1–3):
```typescript
import { useEffect, useRef, type ReactNode } from "react";
import "./Dialog.css";
```
For SearchPalette:
```typescript
import { useEffect, useRef, useState } from "react";
import { Search } from "lucide-react";
import { useQuery } from "@tanstack/react-query";
import { search } from "../../api/client";
import { useSearchStore } from "../../store/searchStore";
import SearchResultRow from "./SearchResultRow";
import "./SearchPalette.css";
```

**ARIA attributes** (Dialog.tsx lines 108–112):
```typescript
<div
  className="dialog"
  role="dialog"
  aria-modal="true"
  aria-label={title}
  ref={dialogRef}
>
```
SearchPalette panel:
```typescript
<div
  className="palette-panel"
  role="dialog"
  aria-modal="true"
  aria-label="Search"
  ref={panelRef}
>
```
Input gets `role="combobox"`, `aria-expanded`, `aria-controls`, `aria-activedescendant`.

---

### `web/src/components/search/SearchPalette.css`

**Analog:** `web/src/routes/AppShell.css` (`.navrow`, lines 99–141) + `web/src/components/Dialog.css` (`.dialog-backdrop`)

**Reuse `.dialog-backdrop`** directly — do NOT re-declare the scrim in SearchPalette.css. The backdrop CSS class lives in `Dialog.css` and is already globally available.

**Navrow tokens to copy** (AppShell.css lines 99–107):
```css
.navrow {
  display: flex;
  align-items: center;
  gap: var(--space-sm);
  min-height: var(--hit-min-height);
  padding: var(--space-sm);
  border-radius: var(--radius-sm);
  font-size: var(--font-size-body);
}
```
Result rows use `.navrow` geometry directly (re-apply the class, do not re-declare).

**Active row treatment** (`--color-tree-active-bg` + inset accent bar):
```css
.palette-result-row--active {
  background: var(--color-tree-active-bg);
  box-shadow: inset 2px 0 0 var(--color-accent);
  color: var(--color-accent);
}
```

**Token constraints (from tokens.css):**
- Panel: `background: var(--color-bg)`, `border: 1px solid var(--color-border)`, `border-radius: var(--radius-md)`, `box-shadow: var(--shadow-popover)`
- Panel `max-width: 640px` (only non-token dimension; acceptable per UI-SPEC)
- Top-anchored: `top: 15vh` (Obsidian convention; not a token; document inline)
- Results list: `max-height: min(60vh, 480px)`, `overflow-y: auto`
- All padding/gap from `var(--space-xs/sm/md/lg)` — zero hard-coded px values

---

### `web/src/components/search/SearchResultRow.tsx` — typed result row (component, transform)

**Analog:** `web/src/components/RoleBadge.tsx` (the badge chip) + AppShell.css `.navrow`

**RoleBadge pattern** (RoleBadge.tsx lines 1–9):
```typescript
import "./RoleBadge.css";

export default function RoleBadge({ role }: { role: string }) {
  const label = role.charAt(0).toUpperCase() + role.slice(1);
  return <span className="role-badge">{label}</span>;
}
```
Type badge for search results copies `.role-badge` CSS class directly (same surface fill, hairline border, `--radius-sm`, label type). Text: `Page` / `Heading` / `Attachment`.

**RoleBadge.css tokens** (lines 1–11):
```css
.role-badge {
  display: inline-flex;
  align-items: center;
  padding: var(--space-xs) var(--space-sm);
  font-size: var(--font-size-label);
  line-height: var(--line-height-label);
  color: var(--color-text);
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-sm);
}
```

**Row anatomy** (from UI-SPEC lines 160–176):
```typescript
// [type-icon] [Title (semibold, highlight=bold)] [type-badge]
//             [snippet — muted, label size, 1-2 lines]
//             [sub-line: "in {page title}" — muted, heading/attachment only]
```
Matched-term highlight: weight-only bold (`font-weight: 600`), no background, no accent color. Sanitize Bleve fragment HTML to `<strong>` only — no `dangerouslySetInnerHTML` of raw server HTML.

---

### `web/src/api/client.ts` *(modified)* — search API function

**Analog:** itself, `getTree` / `getPage` GET pattern (lines 213–235):
```typescript
export async function getTree(): Promise<TreeNode[]> {
  const res = await fetch("/api/v1/tree", { credentials: "same-origin" });
  if (!res.ok) {
    throw new Error("Couldn't load your pages — try again.");
  }
  return (await res.json()) as TreeNode[];
}
```

Add these types and function:
```typescript
export type SearchResultKind = "page" | "heading" | "attachment";

export interface SearchResult {
  kind: SearchResultKind;
  title: string;
  path: string;          // page path to navigate to (owning page for heading/attachment)
  snippet: string;       // Bleve fragment with highlight markers (sanitized client-side)
  anchor?: string;       // heading anchor for deep-link (#heading-slug), kind="heading" only
  page_title?: string;   // owning page title, kind="heading" | "attachment" only
}

export async function search(q: string): Promise<SearchResult[]> {
  if (!q.trim()) return [];
  const res = await fetch(`/api/v1/search?q=${encodeURIComponent(q)}`, {
    credentials: "same-origin",
  });
  if (!res.ok) {
    throw new Error("Search is unavailable. Try again in a moment.");
  }
  return (await res.json()) as SearchResult[];
}
```

---

### `web/src/store/searchStore.ts` — palette open/closed state (store, event-driven)

**Analog:** none in codebase (first zustand store). Use CLAUDE.md locked-stack pattern: `zustand` v5.0.14.

**Minimal shape** (consistent with AppShell.tsx zustand usage context):
```typescript
import { create } from "zustand";

interface SearchStore {
  open: boolean;
  setOpen: (open: boolean) => void;
}

export const useSearchStore = create<SearchStore>((set) => ({
  open: false,
  setOpen: (open) => set({ open }),
}));
```

---

### `web/src/hooks/useSearch.ts` — debounced search query hook (hook, request-response)

**Analog:** `web/src/components/LeftTree.tsx` `useQuery` pattern (lines 9, 77–81):
```typescript
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

const { data, isLoading, isError } = useQuery<TreeNode[]>({
    queryKey: ["tree"],
    queryFn: getTree,
});
```

`useSearch` wraps `useQuery` with a debounced query string:
```typescript
import { useQuery } from "@tanstack/react-query";
import { search, type SearchResult } from "../api/client";

export function useSearch(rawQuery: string) {
  // debounce rawQuery ~200ms before setting the active query key
  const q = useDebouncedValue(rawQuery, 200);
  return useQuery<SearchResult[]>({
    queryKey: ["search", q],
    queryFn: () => search(q),
    enabled: q.trim().length > 0,
    staleTime: 30_000,
    placeholderData: (prev) => prev, // keep previous results visible while loading
  });
}
```
`queryKey: ["search", q]` — follow the `["tree"]` / `["me"]` / `["health"]` naming convention established in the codebase.

---

### `web/src/routes/AppShell.tsx` *(modified)* — top-bar search trigger

**Analog:** itself, lines 1–60 (the `repo-health` affordance in `.topbar-right`, lines 51–56):
```typescript
{repoHealth?.ok && (
    <span className="repo-health" title={repoHealth.detail}>
      <span className="repo-health-dot" aria-hidden="true" />
      <span>Storage healthy</span>
    </span>
)}
```

Add the search trigger **left** of `UserMenu`, inside `.topbar-right`:
```typescript
<button
  type="button"
  className="btn btn-ghost navrow-action"
  onClick={() => setSearchOpen(true)}
  aria-label="Search workspace (⌘K)"
>
  <Search size={16} />
  <span>Search</span>
  <kbd className="keycap">⌘K</kbd>
</button>
```
Wire the global `⌘K` / `Ctrl K` listener in a `useEffect` in AppShell (same `document.addEventListener("keydown", ...)` pattern as Dialog.tsx lines 63–85). Call `useSearchStore` for open/close state.

---

## Shared Patterns

### Job handler constructor shape
**Source:** `internal/attachments/extractjob.go` lines 59–65
**Apply to:** `internal/search/indexjob.go`
```go
func IndexHandler(idx *search.Index, r *repo.Repo) jobs.Handler {
    return func(ctx context.Context, payload string) (err error) {
        defer func() {
            if rec := recover(); rec != nil {
                err = fmt.Errorf("search: index handler panic: %v", rec)
            }
        }()
        var p indexPayload
        if uerr := json.Unmarshal([]byte(payload), &p); uerr != nil {
            return fmt.Errorf("search: index payload: %w", uerr)
        }
```

### Fire-and-forget Enqueue (NOT EnqueueAndWait) inside a handler
**Source:** `internal/attachments/extractjob.go` lines 119–135 (CR-01 lesson)
**Apply to:** `internal/search/indexjob.go` — if the index handler ever needs to enqueue a follow-on job, use `w.Enqueue`, never `w.EnqueueAndWait`. The handler runs ON the drain goroutine; waiting would deadlock the worker.

**The EnqueueAndWait pattern** IS appropriate in page/attachment mutations that need to block until the index update is confirmed — use `worker.EnqueueAndWait(ctx, search.KindIndex, payload, indexWaitTimeout)` in `pages.Service.Save`, `pages.Service.Delete`, etc. (where caller is an HTTP handler goroutine, NOT the drain goroutine).

### HTTP error / JSON response helpers
**Source:** `internal/server/handlers_auth.go` lines 66–74
**Apply to:** `internal/server/handlers_search.go`
```go
func writeJSON(w http.ResponseWriter, status int, v any) { ... }
func writeError(w http.ResponseWriter, status int, message string) { ... }
```
Package-level helpers already in the `server` package — use them, do not re-define.

### RBAC gating in router
**Source:** `internal/server/router.go` lines 84–165
**Apply to:** Route placement for `GET /search` (authed group, any role) and `POST /admin/search/reindex` (admin subgroup with `auth.RequireRole(auth.RoleAdmin)`).

### Safe-path resolver (SEC-01)
**Source:** `internal/attachments/extractjob.go` lines 80–87 (uses `r.Read(p.BinPath)`)
**Apply to:** `internal/search/rebuild.go` — all file reads during full rebuild must go through `repo.Repo.Read(relPath)` or `repo.Repo.Walk(...)`, never `os.Open` directly.

### Hidden-Git rule
**Source:** Carried from Phases 0–2 — see UI-SPEC and all handler user-facing strings
**Apply to:** All new handler error messages and frontend copy — no "commit", "index", "Bleve", "HEAD", "repo", "Git" visible to the user. Use "Search is unavailable" not "Bleve index error". On the admin reindex endpoint, the action label is "Rebuild search index" not "Reindex Bleve".

### react-query key naming
**Source:** `web/src/components/LeftTree.tsx` lines 77–81, `web/src/routes/AppShell.tsx` lines 19–23
**Apply to:** `web/src/hooks/useSearch.ts`
```typescript
// Established keys: ["tree"], ["me"], ["health"], ["page", path], ["history", path]
// New: ["search", q]
queryKey: ["search", q]
```

### CSS token discipline
**Source:** `web/src/styles/tokens.css` (all `var(--…)`)
**Apply to:** `web/src/components/search/SearchPalette.css` and `SearchResultRow.css`
- Zero hard-coded hex or px values except the two documented exceptions: `max-width: 640px` (palette width) and `top: 15vh` / `max-height: min(60vh, 480px)` (palette positioning). All padding, gap, color, radius, shadow from existing `var(--…)` tokens.

---

## No Analog Found

| File | Role | Data Flow | Reason |
|------|------|-----------|--------|
| `web/src/store/searchStore.ts` | store | event-driven | No zustand store exists yet in the codebase — first use of the locked stack library. Implement from the minimal zustand v5 `create` pattern per CLAUDE.md |

---

## Mutation Enqueue Hooks — Where to Wire Them

The following existing files need `search.KindIndex` enqueue calls added after their commit lands. Each uses `w.Enqueue` (fire-and-forget from HTTP handler context — NOT the drain goroutine):

| Mutation site | File | After which operation |
|--------------|------|-----------------------|
| Page create | `internal/pages/service.go` `Create()` | After `EnqueueCommit` succeeds |
| Page save | `internal/pages/service.go` `Save()` | After `EnqueueCommit` succeeds |
| Page rename/move | `internal/pages/rename.go` | After `EnqueueCommit` succeeds |
| Page delete (trash) | `internal/pages/trash.go` `Delete()` | After `EnqueueCommit` succeeds (index the trashed path as a delete-op) |
| Extraction done | `internal/attachments/extractjob.go` | After the `.txt` KindCommit is enqueued (fire-and-forget, handler context) |
| Attachment upload | `internal/attachments/service.go` `Upload()` | After `EnqueueAndWait` commit succeeds |
| Attachment replace | `internal/attachments/service.go` `Replace()` | After `EnqueueAndWait` commit succeeds |
| Attachment remove | `internal/attachments/lifecycle.go` | After `EnqueueAndWait` commit succeeds |

---

## Metadata

**Analog search scope:** `internal/jobs/`, `internal/pages/`, `internal/attachments/`, `internal/server/`, `internal/config/`, `internal/okf/`, `cmd/okf-workspace/`, `web/src/`
**Files scanned:** 14 source files read
**Pattern extraction date:** 2026-06-21
