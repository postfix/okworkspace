# Phase 5: Collaboration - Pattern Map

**Mapped:** 2026-06-22
**Files analyzed:** 13 (6 backend, 7 frontend)
**Analogs found:** 13 / 13 (every file has a real in-repo analog; `internal/locks` is net-new *as a package* but copies the `pages.Service` shape exactly)

> This phase is almost entirely *composition of shipped primitives* (RESEARCH §"Don't Hand-Roll"). Every analog below was read in the live tree this session with exact line ranges. No new architectural pattern is introduced.

---

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|-------------------|------|-----------|----------------|---------------|
| `internal/locks/lock.go` (NEW) | model | transform | `internal/pages/service.go` `Page` struct (`:104-108`) | role-match |
| `internal/locks/service.go` (NEW) | service | CRUD (file I/O) | `internal/pages/service.go` `Service` (`:84-178`) + `internal/repo/files.go` (`:27-92`) | exact (struct+`now`) |
| `internal/locks/service_test.go` (NEW) | test | — | `internal/jobs/worker_test.go` (`:40`) + `pages` table-test idiom w/ injected `now` | role-match |
| `internal/locks/gc.go` (NEW or in service.go) | service | batch (event-driven) | `internal/search/rebuild.go` `trashPrefix` walk (`:15-22`) + jobs `Handler` (`queue.go:40`) | role-match |
| `internal/server/handlers_presence.go` (NEW) | controller | streaming (SSE) + request-response | `internal/server/handlers_sse.go` `handleExtractionStatus` (`:41-117`) + `handlers_pages.go` suffix dispatch (`:112-124`) | exact (SSE) / exact (dispatch) |
| `internal/auth/session.go` (MODIFY) | config | — | `SessionUserIDKey` const (`:15`) | exact |
| `cmd/okf-workspace/main.go` (MODIFY) | config/wiring | event-driven (ticker) | `main.go` `worker.Register` (`:190-222`) + `worker.Enqueue` fire-and-forget (`:275`) | exact |
| `internal/pages/service.go` (likely UNCHANGED) | service | CRUD | own `Create`/`Save`/`uniquePath` (`:115-220`) | n/a — reused as-is |
| `web/src/components/PresenceIndicator.tsx` (NEW) | component | streaming (SSE consumer) | `web/src/components/AutosaveStatus.tsx` (`:9-29`) + `client.ts subscribeExtractionStatus` (`:459-479`) | exact (status) / exact (SSE) |
| `web/src/components/SoftLockBanner.tsx` (NEW) | component | request-response | `PageEditor.tsx` `.banner banner-warning` (`:220-237`) | exact (banner) |
| `web/src/components/DiffReviewDialog.tsx` (MODIFY) | component | request-response | its own `stale` branch + Reject/Approve footer (`:178-248`) | exact (self-extend) |
| `web/src/api/client.ts` (MODIFY) | service (API client) | streaming + request-response | `subscribeExtractionStatus` (`:459-479`) + `getPage`/`createPage`/`savePage` (`:222-253`) | exact |
| `web/src/routes/PageEditor.tsx` (MODIFY) | component | request-response | own `runSaver` 409 catch (`:116-128`) + `conflict` banner (`:220-232`) + toolbar (`:266-281`) | exact (self-extend) |

---

## Pattern Assignments

### `internal/locks/service.go` (service, file-I/O CRUD) — NEW

**Analog:** `internal/pages/service.go` (struct + injected `now func() time.Time`) + `internal/repo/files.go` (path-safe I/O chokepoint).

**Service-struct + `now` pattern** (`service.go:84-99`):
```go
type Service struct {
	repo *repo.Repo
	// ...
	now func() time.Time // overridable in tests for deterministic timestamps
}
func NewService(r *repo.Repo, /* ... */) *Service {
	return &Service{ repo: r, now: time.Now }
}
```
Copy this exactly: `locks.Service{ repo *repo.Repo; now func() time.Time; ttl time.Duration }`, `now: time.Now` in the constructor. Tests inject a fake clock (see test analog).

**Path-safe I/O — ALWAYS via repo, NEVER os.* directly** (`internal/repo/files.go`):
- `Read(rel)` (`:27-33`) → `os.ReadFile` after `Resolve`; missing file returns an `os.ErrNotExist`-wrapped error → check with `errors.Is(err, os.ErrNotExist)` for "no lock".
- `Write(rel, data)` (`:38-50`) auto-creates parent dirs via `MkdirAll` (so `.okf-workspace/locks/runbooks/deploy.md.lock` "just works" — mirror-the-tree scheme, RESEARCH A1). **NOTE non-atomic** truncate-then-write (`:49`) → treat any `json.Unmarshal` error as "no lock" (self-heals next heartbeat).
- `Remove(rel)` (`:56-65`) idempotent — missing file is NOT an error (`:61`) → perfect for `Release`/GC.
- `Exists(rel)` (`:79-92`).

Lock path: `".okf-workspace/locks/" + pagePath + ".lock"` — page path is already validated by `cleanPathString` (`handlers_pages.go:84-97`) AND re-validated by `repo.Resolve`. Never hand-roll a slug joiner.

**Operations** (RESEARCH §Seam 1): `Acquire` / `Refresh` (holder-only, bumps `ExpiresAt`) / `Force` (unconditional replace, lock-only — NEVER touches page or revision) / `Release` (delete only if on-disk `session_id` matches caller) / `Get` / `List` / `EditorsFor(ctx, page, now)`. `Get` returns `(Lock{}, false, nil)` on missing/torn/expired.

### `internal/locks/lock.go` (model) — NEW

**Analog:** `pages.Service` `Page` JSON struct (`service.go:104-108`). CONTEXT-locked shape:
```go
type Lock struct {
	Username  string    `json:"username"`
	UserID    int64     `json:"user_id"`
	SessionID string    `json:"session_id"`
	LockedAt  time.Time `json:"locked_at"`
	ExpiresAt time.Time `json:"expires_at"`
}
```

### `internal/locks/gc.go` (service, batch) — NEW

**Analogs:** `internal/search/rebuild.go` `trashPrefix` (`:15-22`) for the `.okf-workspace/` walk idiom; `internal/jobs/queue.go` `Handler` (`:40`) for the handler signature.

`Handler = func(ctx context.Context, payload string) error` (`queue.go:40`). Define `KindGC string` + `GCHandler(store *Service) jobs.Handler` that calls `store.GC(ctx)` → `List` `.okf-workspace/locks/`, `Remove` each lock whose `ExpiresAt < now()`. Idempotent via `repo.Remove`. Empty payload (GC scans everything), mirroring `search.RebuildPayload()` use.

### `internal/server/handlers_presence.go` (controller, SSE + request-response) — NEW

**Analog A — SSE stream:** `handlers_sse.go handleExtractionStatus` (`:41-117`). Copy the skeleton VERBATIM:
- Flusher assert (`:50-54`) → 500 "Streaming is not supported." if not ok.
- Headers (`:56-60`): `Content-Type: text/event-stream`, `Cache-Control: no-cache`, `Connection: keep-alive`, `X-Accel-Buffering: no`.
- `ctx := r.Context()` (`:62`); `emit()` closure writes `fmt.Fprintf(w, "data: %s\n\n", b); flusher.Flush()` (`:84-85`).
- Emit current snapshot immediately (`:91`), then `ticker` loop (`:101-116`) with `select { case <-ctx.Done(): return; case <-deadline.C: return; case <-ticker.C: emit }`. Reuse the `sseTick` (`:15`) + `sseMaxDuration` (`:24`) constants pattern — the deadline cap prevents goroutine leak on a forgotten tab.
- Snapshot frame (RESEARCH A2/A7): `data: {"editors":[{"username":"alice","you":false}],"you_hold_lock":true}\n\n`, computed from `locks.EditorsFor(ctx, page, now)`.

**Analog B — routing via suffix dispatch (avoids sibling-wildcard conflict):** `handlers_pages.go handleGetPageOrHistory` (`:112-124`). Add `case strings.HasSuffix(wild, ".md/presence"): h.handlePresence(...)` in the same `switch`. Lock mutation endpoints (`.md/lock`, `.md/lock/force`, `.md/lock/release`) dispatch the same way off the PUT/POST catch-all.

**Analog C — handler conventions:** `handlers_pages.go` — `cleanPathParam` (`:76-78`), `writeJSON`/`writeError`, `h.actorUsername(r.Context())` for **server-trusted** username (NEVER from request body — SEC). Presence GET = any-authed (no CSRF, like `handleExtractionStatus`); lock mutations = editor-gated + inherit global CSRF.

### `internal/auth/session.go` (config) — MODIFY

**Analog:** `SessionUserIDKey = "user_id"` (`:15`). Add one line beside it:
```go
const SessionConnectionIDKey = "connection_id"
```
(Per RESEARCH A3 the connection id is most naturally client-generated and passed as a query param + lock `session_id`; storing it in SCS is optional — add the const regardless per CONTEXT.)

### `cmd/okf-workspace/main.go` (wiring) — MODIFY

**Analog:** worker registration block (`:186-226`) + fire-and-forget enqueue (`:275`).
- Construct `lockStore := locks.NewService(contentRepo, ...)` next to `pagesSvc` (`:231`).
- `worker.Register(locks.KindGC, locks.GCHandler(lockStore))` beside the other `Register` calls — **before** `worker.Start(ctx)` (`:224`).
- Launch the GC ticker goroutine before `srv.ListenAndServe()`, gated on `ctx.Done()` (the same `ctx` driving `worker.Start(ctx)`): `t := time.NewTicker(lockGCInterval /* ~60s */)` → `case <-t.C: _ = worker.Enqueue(context.Background(), locks.KindGC, "")`. Mirrors the `worker.Enqueue(..., search.RebuildPayload())` fire-and-forget at `:275`.

### `internal/pages/service.go` — UNCHANGED (reused as-is by COLL-03/04)

Do NOT modify. The conflict flows route through existing calls:
- **409 floor** `Save(...baseRevision...)` (`:186-220`) — check is BEFORE any write (`:195-202`); `current != baseRevision → ErrStaleRevision`. **Force-edit must never bypass this** (load-bearing rule). Mapped to 409 at `handlers_pages.go:201-203`.
- **Save-as-copy** = `Create(folder, title, user)` (`:115-143`, scaffolds a *blank* doc — takes no body) then a second `Save` of the new path with my body. Fresh revision; original untouched.
- **Revision** (`:176-178`) opaque blob SHA — never user-surfaced.

### `web/src/components/PresenceIndicator.tsx` (component, SSE consumer) — NEW

**Analog A — status component:** `AutosaveStatus.tsx` (`:9-29`). Same restraint: `<span aria-live="polite" />` (empty when idle, `:11`); `Loader2 size={14} aria-hidden className="...spinner"` for connecting (`:16`); muted Label text. Never shows your own session.

**Analog B — SSE consumer:** `client.ts subscribePresence` (new, below). Mounts in `.pageeditor-toolbar` left of the mode segment (`PageEditor.tsx:266`). States per UI-SPEC: none / one (`Pencil`) / many (`Users`) / connecting (`Loader2`) / reconnecting (`AlertTriangle` warning) / disconnected (`WifiOff`).

### `web/src/components/SoftLockBanner.tsx` (component) — NEW

**Analog:** the `.banner banner-warning` block in `PageEditor.tsx:220-237`. Copy the structure: `<div className="banner banner-warning" role="status">` (NOT `alert` — informative; editor stays usable via Force edit), flex row `Lock` icon (`--color-warning`) · lead text (semibold lead clause) · spacer · `<button className="btn btn-secondary">Force edit</button>` (mirrors the "Reload page" `.btn-secondary` placement at `:224-230`). Copy strings verbatim from UI-SPEC Copywriting Contract.

### `web/src/components/DiffReviewDialog.tsx` (component) — MODIFY (self-extend)

**Analog:** its OWN `stale` branch + Approve/Reject footer (`:178-248`). The component already declares it is "REUSED in Phase 5 (conflict resolution)" (`:7-9`).
- Add `mode?: "review" | "conflict"` (or a discriminated `conflict` prop set). The `review` path (Reject/Approve, `:219-248`) stays UNCHANGED.
- Conflict footer = 3 buttons: **Overwrite** (`.btn-ghost-destructive` or `.btn-destructive` — the ONLY destructive control), **Manual merge** (`.btn-secondary`), **Save as copy** (`.btn-secondary`).
- **Preserve the focus inversion** (`:56-57`, `:78-79`): the existing model focuses `rejectRef` (the SAFE control) on open. Point the conflict-mode focus ref at Save-as-copy / Manual merge — **NEVER Overwrite** (`:78` comment: "do NOT 'fix' it by focusing Approve"). DOM-order the safe button first so the focus trap lands there.
- Inherited untouched: focus-trap/Tab-wrap (`:87-100`), Esc + backdrop = cancel-only (`:82-84`, `:117-119`), real-diff-always-rendered (`:158-175`), `noChange` identical-versions guard (`:73`), `splitView` toggle, `diffStyles` mono theme (`:260-285`).
- `oldText = server version` (theirs), `newText = mine` per UI-SPEC.

### `web/src/api/client.ts` (API client) — MODIFY

**Analog A — `subscribePresence`:** clone `subscribeExtractionStatus` (`:459-479`) near-verbatim — `new EventSource(\`/api/v1/pages/${path}/presence?conn=${connId}\`)`, `onmessage` → `JSON.parse(e.data)`, `onerror` → surface "Reconnecting…" (native EventSource auto-reconnects), returns `() => es.close()`.

**Analog B — lock mutation calls:** clone the `mutate()`-based `savePage`/`createPage` (`:239-253`) for `acquireLock`/`forceLock`/`releaseLock` (POST/PUT, inherit CSRF via `mutate`).

**Analog C — conflict fetch:** reuse existing `getPage(path)` (`:222-235`, returns `{frontmatter, body, revision}`) to fetch the server version on 409 and to read the fresh revision for Overwrite / save-as-copy.

### `web/src/routes/PageEditor.tsx` (component) — MODIFY (self-extend)

**Analog:** its OWN existing flows.
- **409 → dialog (supersede banner):** `runSaver` already catches 409 → `setConflict(true)` with editor content intact (`:116-128`). Repurpose `conflict` state to open the `DiffReviewDialog` (conflict mode) instead of the `:220-232` banner. Both explicit Save (`:209-212`) and autosave (`:167-170`) route through the same catch → both surface the dialog.
- **Gate autosave on `!conflict`:** `scheduleAutosave` (`:167-170`) must stop re-arming while a conflict is open (Pitfall 5) — otherwise the dialog re-opens every debounce.
- **Mount points:** `PresenceIndicator` in `.pageeditor-toolbar` left of `.pageeditor-mode` (`:266-281`); `SoftLockBanner` in the top notice slot (same slot as the `conflict`/`saveError` banners, `:218-237`).
- **Read-only under another's lock:** `LivePreviewEditor` already supports a read-only surface (PageView uses it); disable the Save CTA while read-only.
- **Three handlers:** Overwrite (`getPage` → `savePage` at fresh rev; 409 again → re-open dialog); Manual merge (keep body, show server for reference, normal save); Save-as-copy (`createPage(folder, "{title} (Copy)")` → `getPage(newPath)` for fresh rev → `savePage(newPath, mine)` → navigate; title from `readField(frontmatter, "title")`, used at `:248`).
- **Lock lifecycle:** acquire on entering Edit, refresh on heartbeat + on save, best-effort `releaseLock` on unmount (mirror the cleanup `useEffect` at `:172-177`).

---

## Shared Patterns

### Path safety (SEC-01 chokepoint)
**Source:** `internal/repo/files.go` `Read/Write/Remove/Exists/MkdirAll` (`:27-92`) → `Resolve`.
**Apply to:** ALL lock file I/O. Never `os.WriteFile`/`os.ReadFile` a lock path directly — the resolver handles `..`/absolute/NUL/percent/symlink-escape.

### Server-trusted identity (never client-supplied)
**Source:** `h.actorUsername(r.Context())` (used at `handlers_pages.go:161,195,208`).
**Apply to:** lock ownership `username`/`user_id` — from the session only; the client supplies ONLY the opaque connection id.

### SSE plumbing
**Source:** `handlers_sse.go handleExtractionStatus` (`:41-117`) — Flusher assert + headers (incl. `X-Accel-Buffering: no`) + `ctx.Done()` cleanup + `sseMaxDuration` cap.
**Apply to:** the presence stream. Don't write a bespoke streaming handler.

### Optimistic concurrency (unconditional, the safety authority)
**Source:** `pages.Service.Save` revision check `:195-202` → `ErrStaleRevision` → 409 (`handlers_pages.go:201`).
**Apply to:** every conflict choice. Force-edit is lock-only and NEVER skips this check.

### `.banner banner-warning` + `aria-live`
**Source:** `PageEditor.tsx:220-237` (banner), `AutosaveStatus.tsx:9-29` (`aria-live="polite"` status).
**Apply to:** `SoftLockBanner` (role="status") + `PresenceIndicator` (aria-live).

### Suffix dispatch off `/pages/*` catch-all
**Source:** `handlers_pages.go handleGetPageOrHistory` (`:112-124`).
**Apply to:** `.md/presence` (GET) + `.md/lock[/force|/release]` (POST/PUT) — avoids the sibling-wildcard conflict.

### Job register + fire-and-forget ticker enqueue
**Source:** `main.go:190-222` (Register), `:275` (Enqueue); `jobs.Handler` (`queue.go:40`).
**Apply to:** `lock_gc`.

---

## No Analog Found

None. Every file maps to a real in-repo analog. `internal/locks/` is net-new *as a package* but its `Service{ ..., now func() time.Time }` shape, file I/O, and test idiom are copied directly from `pages.Service` + `internal/repo` + the jobs/test patterns above.

---

## Metadata

**Analog search scope:** `internal/{locks,pages,server,repo,auth,jobs,search}`, `cmd/okf-workspace`, `web/src/{components,routes,api}`.
**Files scanned:** ~14 (all line ranges confirmed live this session).
**Pattern extraction date:** 2026-06-22
