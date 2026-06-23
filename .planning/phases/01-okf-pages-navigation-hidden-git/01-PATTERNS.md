# Phase 1: OKF Pages, Navigation & Hidden Git - Pattern Map

**Mapped:** 2026-06-18
**Files analyzed:** 22 new/modified
**Analogs found:** 19 / 22 (3 genuinely new — no analog)

> ~90% of this phase is wiring existing Phase-0 spines. The genuinely new, subtle code is confined to `internal/okf` (byte-stable round-trip) and the rename link-rewrite scan — those have NO analog and must follow RESEARCH.md Patterns 1 & 4, not a copied file.

## File Classification

### Backend (Go)

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|-------------------|------|-----------|----------------|---------------|
| `internal/okf/okf.go` (Parse/Doc) | service (lib) | transform | — | **no analog** (new round-trip core) |
| `internal/okf/repair.go` | service (lib) | transform | — | **no analog** |
| `internal/okf/links.go` (Find/Rewrite) | service (lib) | transform | — | **no analog** |
| `internal/okf/emit.go` | service (lib) | transform | `internal/repo/files.go` (byte I/O) | partial |
| `internal/okf/golden/*_test.go` | test | transform | `internal/repo/path_test.go`, `internal/jobs/worker_test.go` | role-match |
| `internal/pages/service.go` | service | CRUD | `internal/users/manage.go` (service ops + sentinel errors) | role-match |
| `internal/pages/tree.go` | service | transform | `internal/repo/files.go` `Tree()` walk | exact |
| `internal/pages/trash.go` | service | CRUD | `internal/users/manage.go` + `gitstore/commit.go` | role-match |
| `internal/pages/commitjob.go` | service (worker handler) | event-driven | `internal/gitstore/commit.go` + `internal/jobs/worker.go` Handler | exact |
| `internal/gitstore/push.go` (Push) | service | request-response | `internal/gitstore/git.go` `PullOnStartup` / `Init` | exact |
| `internal/gitstore/history.go` (log/show) | service | request-response | `internal/gitstore/health.go` (git-read methods) | role-match |
| `internal/server/handlers_pages.go` | controller | CRUD | `internal/server/handlers_users.go` | exact |
| `internal/server/handlers_tree.go` | controller | request-response | `internal/server/handlers_health.go` / `handlers_users.go` | role-match |
| `internal/server/router.go` (extend) | route | request-response | `internal/server/router.go` (existing authed group) | exact |
| `internal/store/migrations/0004_drafts.sql` | migration | CRUD | `internal/store/migrations/0002_jobs.sql` | exact |
| `cmd/okf-workspace/main.go` (extend) | config (wiring) | — | `cmd/okf-workspace/main.go` (existing worker.Register) | exact |

### Frontend (React/TS)

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|-------------------|------|-----------|----------------|---------------|
| `web/src/api/client.ts` (extend) | service | request-response | `web/src/api/client.ts` (`mutate`/`me`/`health`) | exact |
| `web/src/routes/PageView.tsx` | component | request-response | `web/src/routes/AppShell.tsx` (useQuery) | role-match |
| `web/src/routes/PageEditor.tsx` | component | request-response | `web/src/routes/Admin.tsx` (useMutation + Dialog) | role-match |
| `web/src/components/LeftTree.tsx` | component | request-response | `web/src/routes/AppShell.tsx` `PLACEHOLDER_TREE` (replaces it) | role-match |
| `web/src/components/CreatePageModal.tsx` | component | request-response | `web/src/components/Dialog.tsx` + `Admin.tsx` create flow | exact |
| `web/src/components/HistoryDialog.tsx` | component | request-response | `web/src/components/Dialog.tsx` | role-match |
| `web/src/components/LinkPicker.tsx` | component | request-response | `web/src/components/Dialog.tsx` | role-match |
| `web/src/stores/recent.ts` | store | event-driven | — | **no analog** (first zustand store) |

## Pattern Assignments

### `internal/pages/commitjob.go` (worker handler, event-driven)

**Analogs:** `internal/jobs/worker.go` (Handler type + Register), `internal/gitstore/commit.go` (CommitSpec/Commit)

**Handler signature to register** (`internal/jobs/queue.go` lines ~31-33):
```go
type Handler func(ctx context.Context, payload string) error
```
Register on the worker exactly as the existing no-op stub does in `cmd/okf-workspace/main.go:184`:
```go
worker.Register("commit", func(_ context.Context, _ string) error { return nil })
```
Replace that stub. The handler unmarshals a JSON payload, calls `okf.Emit` → `repo.Write` (per write) → `gitstore.Commit`, then optional `Push`. **Never** call `git` or `repo.Write` from an HTTP handler — enqueue via `worker.Enqueue(ctx, "commit", payloadJSON)` (`internal/jobs/queue.go`).

**CommitSpec to reuse verbatim** (`internal/gitstore/commit.go` lines 13-19) — do NOT add a parallel type:
```go
type CommitSpec struct {
    Paths   []string // repo-relative paths to stage (resolved before staging)
    Message string
    User    string   // becomes Git author name
    Action  string   // e.g. "edit", "rename", "trash", "restore"
    Source  string   // "web-ui"
}
```
`Commit` already holds the single-writer mutex (lines 33-34) and resolves every path through `g.repo.Resolve` (lines 39-44) — reuse, do not re-stage. Set `Action` to the page action; `buildMessage` (lines 70-84) writes the `Action:/Source:/User:` trailer that the history view (VER-02) parses back out.

---

### `internal/gitstore/push.go` (service, request-response) — VER-04

**Analog:** `internal/gitstore/git.go` `PullOnStartup` (line 100) and `Init` (line 71) — same single-writer + `g.git(ctx, args...)` shape.

**Pattern:** acquire `g.mu.Lock()` (mirror `commit.go:33`), call `g.git(ctx, "push", remote, branch)` via the existing `g.git` wrapper (`git.go:49`, which uses an `exec.Command` arg slice — no shell string, preserving the command-injection mitigation). On non-ff/diverged rejection, set a `diverged` flag and return nil (alert, never force) — mirror the divergence handling already surfaced through `Health` (`health.go:87`). Config gate reads existing `config.GitConfig` keys (`RemoteEnabled`/`PushOnCommit`/`Remote`/`Branch`) — no new keys (RESEARCH Runtime State Inventory). See RESEARCH "Push (NEW)" code example, lines 416-431.

---

### `internal/gitstore/history.go` (service, request-response) — VER-02/03

**Analog:** `internal/gitstore/health.go` git-read methods (`IsEmpty` line 66, `Health` line 87) — read-only `g.git(ctx, ...)` calls that parse stdout.

**Pattern:** History = `g.git(ctx, "log", "--follow", "--format=...", "--", path)`, parse into `{when, who, action}`, recovering `action` from the `Action:` trailer `buildMessage` wrote. Restore = `g.git(ctx, "show", rev+":"+path)` to read old bytes, then enqueue a normal CommitJob writing them as a new forward commit (never `reset`). No raw SHAs surfaced (VER-02). RESEARCH Pattern 4.

---

### `internal/pages/tree.go` (service, transform) — NAV-01

**Analog:** `internal/repo/files.go` `Tree()` (lines 81-110) — `filepath.WalkDir`, repo-relative slash paths, `.git` skip, sorted.

**Extend, don't replace:** the existing `Tree()` returns flat `TreeItem{Path, IsDir}`. Phase 1 needs the nested SPEC §17.2 shape and must additionally skip `.okf-workspace`, and read ONLY the frontmatter region of each `.md` (via `okf.Parse`, not the whole body) to fill `Title` (fallback to base name). Reuse the `.git` skip idiom (lines 96-101):
```go
if slashRel == ".git" || strings.HasPrefix(slashRel, ".git/") {
    if d.IsDir() { return filepath.SkipDir }
    return nil
}
```
Target response shape (RESEARCH Code Examples, lines 406-411):
```go
type Node struct {
    Type     string `json:"type"`     // "folder" | "page"
    Path     string `json:"path"`
    Title    string `json:"title"`
    Children []Node `json:"children,omitempty"`
}
```

---

### `internal/pages/service.go` + `trash.go` (service, CRUD)

**Analog:** `internal/users/manage.go` — service-layer ops with sentinel errors (`ErrNotFound`, `ErrInvalidRole`, …) that handlers map to HTTP codes via `errors.Is`.

**Patterns to copy:**
- Define package sentinel errors (e.g. `ErrPageNotFound`, `ErrStaleRevision`, `ErrSlugCollision`) so `handlers_pages.go` can `errors.Is`-map them (mirrors `handlers_users.go:150-162`).
- All path I/O through `repo.Read`/`repo.Write`/`repo.Exists`/`repo.MkdirAll` (`internal/repo/files.go`) — never `os.*` directly; the resolver is the SEC-01 chokepoint.
- Trash: `git mv` into `.okf-workspace/trash/` via a CommitJob (D-08); record original-path + deleted-by + timestamp in a SQLite `trash` table; collision-suffix on restore (D-10). Defensive `repo.MkdirAll(".okf-workspace/trash")` on first delete (RESEARCH A1).
- Revision = `git rev-parse HEAD:<path>` blob SHA (RESEARCH Pattern 3) — zero extra hashing.

---

### `internal/server/handlers_pages.go` (controller, CRUD)

**Analog:** `internal/server/handlers_users.go` — the canonical handler file. Copy its exact shape.

**Imports pattern** (`handlers_users.go` lines 3-15):
```go
import (
    "encoding/json"
    "errors"
    "net/http"
    "github.com/go-chi/chi/v5"
    "github.com/postfix/okworkspace/internal/audit"
    "github.com/postfix/okworkspace/internal/auth"
)
```

**Decode → service → error-map → write** (lines 138-173):
```go
var req createUserRequest
if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
    writeError(w, http.StatusBadRequest, "Invalid request.")
    return
}
u, otp, err := users.Create(r.Context(), h.users, ...)
if err != nil {
    if errors.Is(err, users.ErrInvalidRole) { writeError(w, http.StatusBadRequest, "..."); return }
    ...
}
_ = h.audit.Record(r.Context(), audit.Event{Action: ..., Actor: h.actorUsername(r.Context()), Target: ..., Source: auditSourceWeb})
writeJSON(w, http.StatusCreated, ...)
```

**Response helpers** (`handlers_auth.go` lines 56-64) — reuse, do not redefine:
```go
func writeJSON(w http.ResponseWriter, status int, v any) { ... }
func writeError(w http.ResponseWriter, status int, message string) {
    writeJSON(w, status, map[string]string{"error": message})
}
```

**URL param parse** — mirror `pathID` (`handlers_users.go` lines 280-288) for the `{path}` param via `chi.URLParam(r, "path")`; validate before passing to the service (reject `..`/absolute/NUL before slug — the service still re-resolves through `repo.Resolve`).

**Optimistic-concurrency floor:** read `base_revision` from the PUT body, recompute current revision, `writeError(w, http.StatusConflict, ...)` (409) on mismatch BEFORE enqueuing the CommitJob.

**Audit:** record page create/edit/delete/rename/restore via `h.audit.Record(...)` with `Source: auditSourceWeb` (`handlers_auth.go:23`).

---

### `internal/server/router.go` (route) — extend authed group

**Analog:** existing `router.go` lines 73-91 (authed group + RBAC subgroup).

**Mount inside the existing `api.Group(func(authed chi.Router){ authed.Use(h.loadCurrentUser); ... })`** so session/CSRF/RBAC/audit are inherited. Reads (`GET /tree`, `GET /pages/{path}`, `GET /pages/{path}/history`) available to any authenticated user; mutations gated by an editor RBAC subgroup mirroring the admin subgroup (lines 83-90):
```go
authed.Group(func(editor chi.Router) {
    editor.Use(auth.RequireRole(auth.RoleEditor)) // RoleEditor/RoleReader from internal/auth/rbac.go
    editor.Post("/pages", h.handleCreatePage)
    editor.Put("/pages/{path}", h.handleSavePage)
    editor.Delete("/pages/{path}", h.handleDeletePage)
    editor.Post("/pages/{path}/rename", h.handleRenamePage)
    editor.Post("/pages/{path}/restore", h.handleRestoreVersion)
    editor.Post("/folders", h.handleCreateFolder)
})
```
Note `RequireRole` semantics: confirm whether admin should also pass the editor gate (RESEARCH V4). `chi`'s `{path}` is single-segment — use `{path:.*}` or a wildcard route for slash-bearing page paths.

---

### `internal/store/migrations/0004_drafts.sql` (migration, CRUD)

**Analog:** `internal/store/migrations/0002_jobs.sql` — exact template (header comment + `CREATE TABLE IF NOT EXISTS` + supporting index). Migrations are auto-discovered by `loadMigrations()` (`migrations.go:70-98`) from the `NNNN_name.sql` filename — no code change needed to register.

```sql
-- 0004_drafts: autosave drafts (operational/derived only — NEVER canonical page
-- content; the .md file on disk is truth, D-02). Keyed by page path + user.
CREATE TABLE IF NOT EXISTS drafts (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    page_path   TEXT NOT NULL,
    user_id     INTEGER NOT NULL,
    body        TEXT NOT NULL DEFAULT '',
    frontmatter TEXT NOT NULL DEFAULT '',
    updated_at  TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE(page_path, user_id)
);
```
Add optional `trash` table (original_path, deleted_by, deleted_at) in the same or a paired migration per D-10.

---

### `cmd/okf-workspace/main.go` (wiring) — extend

**Analog:** existing wiring, lines 150-193. The worker, gitstore, and `server.New(server.Deps{...})` already exist.

**Changes:** replace the no-op `worker.Register("commit", ...)` stub (line 184) with the real CommitJob handler from `internal/pages`; pass the pages service + repo + gitstore into `server.Deps` (extend the struct in `router.go:18-33` following the existing optional-dependency pattern).

---

### `web/src/api/client.ts` (service) — extend

**Analog:** existing `mutate`/`login`/`logout`/`me`/`health` (lines 50-90+).

**Pattern:** reuse the existing `mutate<T>(path, body, method)` helper (lines 50-82) for all page mutations — it already attaches CSRF (`X-CSRF-Token`), `credentials: "same-origin"`, surfaces `{error}` messages, and attaches `err.status` (used for the **409** conflict path). Add typed interfaces beside `Me`/`RepoHealth` (lines 5-30) for `TreeNode`, `Page` (`{frontmatter, body, revision}`), `HistoryEntry`. Add `getTree()`, `getPage(path)`, `savePage(path, {body, frontmatter, base_revision})` (PUT), `createPage`, `renamePage`, `deletePage`, `getHistory`, `restoreVersion` using `mutate`/`fetch`.

---

### `web/src/components/CreatePageModal.tsx` + `HistoryDialog.tsx` + `LinkPicker.tsx` (components)

**Analog:** `web/src/components/Dialog.tsx` (focus-trapped modal) + `web/src/routes/Admin.tsx` create flow (`useMutation` + `Dialog`, lines 3,18,69-86).

**Pattern (Admin.tsx):**
```tsx
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import Dialog from "../components/Dialog";
const queryClient = useQueryClient();
const createMut = useMutation({
  mutationFn: () => createUser(...),
  onSuccess: () => queryClient.invalidateQueries({ queryKey: USERS_KEY }),
});
```
CreatePageModal = title-only field (D-12); on success invalidate the `["tree"]` query. Wrap in `<Dialog>` — backdrop-click never confirms (Dialog contract). HistoryDialog lists versions (no SHAs) + Restore action; LinkPicker emits a relative `.md` path into the editor (D-06).

---

### `web/src/routes/PageEditor.tsx` (component) — autosave + Save

**Analog:** `web/src/routes/Admin.tsx` (mutation + query) and `AppShell.tsx` (useQuery).

**Pattern:** `@uiw/react-md-editor` edits the **raw Markdown body string** (protects round-trip — never a block model); a frontmatter form edits title/tags/description. Debounce (~1s) autosave PUT → draft; explicit Save or ~5-10s idle issues a "commit now" PUT (D-03, client-driven per RESEARCH Open Q2). On 409 (`err.status === 409`) surface a stale-save message (full conflict UX is Phase 5). Use `zustand` for dirty-draft/editor-mode state (matches the prescribed store pattern).

---

### `web/src/components/LeftTree.tsx` + `web/src/routes/AppShell.tsx` (replace PLACEHOLDER_TREE)

**Analog:** `AppShell.tsx` lines 12-17 (`PLACEHOLDER_TREE`) and lines 66-81 (render loop with `Folder`/`FileText` from `lucide-react`).

**Pattern:** replace the static `PLACEHOLDER_TREE` and its disabled `navrow-disabled` rows with a live `useQuery<TreeNode>({ queryKey: ["tree"], queryFn: getTree })`. Keep the existing `Folder`/`FileText` lucide icons and `.navrow` CSS classes; add expand/collapse (NAV-02) and current-page highlight driven by the `/app/page/:path` route (NAV-04). Recents come from the new `stores/recent.ts` zustand store (NAV-05), persisted to localStorage.

## Shared Patterns

### Single-writer Git mutation (apply to ALL backend write paths)
**Source:** `internal/gitstore/commit.go:25-66` + `internal/jobs/worker.go` Handler + `internal/jobs/queue.go` `Enqueue`
**Apply to:** save, rename+rewrite, trash, restore, version-restore, push.
Handlers enqueue a `"commit"` job; the single worker goroutine runs `okf.Emit` → `repo.Write` → `gitstore.Commit` (+optional Push). Never `git`/`os.Write` from a handler. The worker (`worker.go:84-109`) is the serialization point; `Commit` (`commit.go:33`) holds the mutex too — double-safe.

### Path safety (apply to EVERY path)
**Source:** `internal/repo/files.go` (`Read`/`Write`/`MkdirAll`/`Exists` all route through `Resolve`)
**Apply to:** every page/folder/trash/rename-target/restore-target/link-target path.
Never touch `os.*` with a user-derived path; go through `repo.*`. `gitstore.Commit` re-resolves staged paths (`commit.go:40`) as a backstop.

### HTTP handler shape (apply to all new handlers)
**Source:** `internal/server/handlers_users.go` + `writeJSON`/`writeError` (`handlers_auth.go:56-64`)
**Apply to:** `handlers_pages.go`, `handlers_tree.go`.
Decode JSON → call service → `errors.Is`-map sentinel errors to status codes → `audit.Record(..., Source: auditSourceWeb)` on mutations → `writeJSON`/`writeError`. Authorization always from session via `loadCurrentUser` + `auth.RequireRole` — never client input.

### Migration template (apply to new SQL)
**Source:** `internal/store/migrations/0002_jobs.sql` + auto-discovery in `migrations.go:70-98`
**Apply to:** `0004_drafts.sql` (+ optional trash/revisions). `CREATE TABLE IF NOT EXISTS`, header comment noting operational-only/files-are-truth, drop a file named `NNNN_name.sql` — no code registration.

### Frontend mutation + CSRF (apply to all SPA writes)
**Source:** `web/src/api/client.ts` `mutate<T>` (lines 50-82) + `Admin.tsx` `useMutation`/`invalidateQueries`
**Apply to:** every page/folder mutation. CSRF + credentials are handled inside `mutate`; `err.status` carries the 409 for stale-save handling.

## No Analog Found

| File | Role | Data Flow | Reason |
|------|------|-----------|--------|
| `internal/okf/okf.go` / `repair.go` / `links.go` | service (lib) | transform | Byte-stable OKF round-trip is greenfield. No existing parse/emit code. Follow RESEARCH Pattern 1 (opaque body + `yaml.Node` surgical frontmatter) & Pattern 4 (structural link rewrite). This is THE phase exit gate — golden corpus first. |
| `internal/okf/golden/` corpus | test fixtures | transform | No corpus exists. Test *structure* can follow `internal/jobs/worker_test.go` table-test style, but fixtures are net-new (must include code blocks containing `---`, GFM tables, ref links, a CRLF fixture per A4). |
| `web/src/stores/recent.ts` | store | event-driven | First `zustand` store in the SPA (zustand installed but unused). No analog; follow the prescribed zustand + localStorage pattern (NAV-05). |

## Metadata

**Analog search scope:** `internal/{gitstore,jobs,repo,store,server,users,auth}`, `cmd/okf-workspace`, `web/src/{api,routes,components}`
**Files scanned:** ~20 Go + ~6 TS source files read
**Pattern extraction date:** 2026-06-18
