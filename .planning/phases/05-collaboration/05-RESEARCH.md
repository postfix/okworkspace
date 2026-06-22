# Phase 5: Collaboration - Research

**Researched:** 2026-06-22
**Domain:** Concurrent editing for a 5-user self-hosted wiki — presence (SSE), file-based soft locks, optimistic-concurrency hardening, conflict resolution (3-way diff). All net-new pieces bolt onto seams already shipped in Phases 1–4.
**Confidence:** HIGH (every integration seam was read in the live tree this session; the only LOW items are deliberate design choices left to the planner's discretion per CONTEXT).

> **Scope discipline (per objective):** this phase is well-specified (SPEC §13.1 + 05-CONTEXT.md). This document does NOT re-research the domain. It (1) confirms each integration seam against the live code with `file:func` citations, (2) locks the small net-new shapes, (3) produces a Validation Architecture for VALIDATION.md, and (4) gives a smallest-safe slice ordering. No external packages are added — so there is **no Package Legitimacy Audit and no Environment Availability section** (no new dependencies).

---

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
**Soft Lock Mechanics**
- File-based soft locks in `.okf-workspace/locks/{normalized-page}.lock` as JSON `{username, user_id, session_id, locked_at, expires_at}`, written/read via `repo.Write`/`repo.Read` (path-safety through `repo.Resolve` — no direct fs access). New package `internal/locks/`.
- Acquired on entering edit mode (not on view); refreshed on heartbeat + on save; best-effort released on leave/unmount.
- TTL: ~30s heartbeat, ~2min expiry. A periodic `lock_gc` job reaps lock files past `expires_at` (session end / crash never causes a permanent lock).
- Force-edit: when another session holds the lock, the editor is read-only with a "Force edit" button. Force-edit takes ownership (replaces the lock) — but the optimistic-concurrency revision check STILL runs on save (force-edit is NOT a bypass). **Load-bearing safety rule.**

**Presence (COLL-01)**
- Delivered over a per-page SSE stream (`GET /api/v1/pages/{path}/presence`), reusing `internal/server/handlers_sse.go` Flusher pattern + the Phase 4 fetch-stream SSE consumer.
- Granularity = "currently editing" (lock / edit-mode holders), MVP — not passive viewers.
- Indicator UI: a small "Alice is editing" indicator near the editor header + the soft-lock banner; minimal, Obsidian-style (reuse tokens.css; no avatar stack).
- Heartbeat/identity: presence keyed by session + a per-browser connection id; the SSE connection itself is the heartbeat (drop → "leave" after TTL); client reconnects on drop. Add `SessionConnectionIDKey` to the session.

**Conflict Resolution (COLL-04)**
- Trigger: on a stale-save 409 (`ErrStaleRevision`), fetch the current server version and open `DiffReviewDialog` in a new `conflict` mode (old = server version, new = my version) — a real diff, never prose.
- Three choices: Overwrite (save my version against the *current* revision), Manual merge (open my version in the editor with the server version visible; resolve and normal-save), Save as copy (create a new page via `pages.Service.Create`/`uniquePath`; the original page is never modified).
- Dialog reuse: extend `DiffReviewDialog` with a `conflict` mode + 3-button footer, preserving the real-diff trust contract. Phase 4 Approve/Reject mode unchanged.

**Concurrency Hardening & Stale Locks**
- Revision check enforced even after force-edit; save always does the BaseRevision check → 409 → conflict UI. No silent overwrite, ever.
- Stale-lock cleanup: periodic `lock_gc`; best-effort release on unmount.
- Autosave interaction: Phase 1 autosave keeps working; a 409 from autosave surfaces the conflict UI rather than silently dropping the edit.
- Save-as-copy semantics: the new page starts with a fresh revision and never carries the conflicted base revision; the original is untouched.

### Claude's Discretion
- Exact lock-file path normalization scheme, connection-id generation, SSE event frames (`presence`/`heartbeat` shapes).
- Heartbeat cadence + GC interval within the stated TTL envelope.
- Precise read-only/force-edit visual treatment and the manual-merge editor layout (defer to UI-SPEC).
- Whether presence and lock state share one stream or two endpoints.

### Deferred Ideas (OUT OF SCOPE)
- Realtime/CRDT collaboration (Yjs), WebSocket collab rooms, live cursor awareness, per-paragraph merge — SPEC §13.2.
- Passive-viewer presence (who's reading, not editing) — editing presence only.
- Avatar stacks / rich presence — minimal indicator in MVP.
</user_constraints>

---

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| COLL-01 | User can see when another user is currently editing a page (presence) | New `GET /api/v1/pages/{path}/presence` SSE endpoint built on the **confirmed** `handlers_sse.go` Flusher pattern (`handleExtractionStatus`); new `internal/locks` store is the presence source of truth (lock holders = editors); frontend `subscribePresence(path)` reuses the **confirmed** `EventSource` consumer (`subscribeExtractionStatus`, `client.ts:459`). `PresenceIndicator` mounts in `.pageeditor-toolbar` (`PageEditor.tsx:266`). |
| COLL-02 | Soft lock applied while editing; user can still force-edit | New `internal/locks` package writes `.okf-workspace/locks/*.lock` via the **confirmed** `repo.Write`/`repo.Read`/`repo.Exists`/`repo.Remove` chokepoint (`internal/repo/files.go`). New acquire/refresh/force/release editor-gated endpoints. `SoftLockBanner` + read-only `LivePreviewEditor` (`mode` already supports read-only via `PageView`). |
| COLL-03 | Saves use optimistic concurrency with a per-document revision | **Already shipped & confirmed** (`pages.Service.Save` revision-check-before-write, `service.go:186`; `ErrStaleRevision` → 409 at `handlers_pages.go:201`; client sends `base_revision`, `client.ts:248`; `PageEditor.tsx:118` catches 409). This phase **hardens** it: force-edit must NOT bypass it (no code path skips `Save`'s check) + autosave 409 routes to the conflict dialog. |
| COLL-04 | On conflict, show a diff with overwrite / manual-merge / save-as-copy | Extend the **confirmed** `DiffReviewDialog` (`DiffReviewDialog.tsx`) with a `conflict` mode + 3-button footer; client fetches the server version via the **confirmed** `getPage(path)` (`client.ts:222`) to diff on 409; save-as-copy uses the **confirmed** `pages.Service.Create`+`uniquePath` (`service.go:115`/`318`) → `createPage` (`client.ts:239`). |
</phase_requirements>

---

## Summary

Phase 5 adds four collaboration affordances to a wiki whose concurrency *floor* (COLL-03) already exists and was verified line-by-line this session. The only net-new backend surface is one small package (`internal/locks`) plus a handful of editor-gated endpoints and one SSE stream; the only net-new frontend surface is two small components (`PresenceIndicator`, `SoftLockBanner`), a `subscribePresence` consumer, and a `conflict` branch in the already-general `DiffReviewDialog`. Every byte of I/O routes through the existing `repo.Resolve` chokepoint and the existing single-writer job worker — no new architectural patterns are introduced.

The load-bearing invariant is that **force-edit takes the *lock* but never the *revision check*.** Force-edit and locks are advisory; `pages.Service.Save` is the authority — it re-reads the committed revision and returns `ErrStaleRevision` (409) regardless of who holds the lock. The plan must keep force-edit a lock-only operation and route every 409 (explicit save *or* autosave) into the conflict dialog. This makes "stale lock from a crashed session" harmless: at worst you force past a ghost lock, and if a real commit landed you still get the conflict dialog.

**Primary recommendation:** Build it as four thin vertical slices in dependency order — (1) lock store + GC job + unit tests, (2) lock endpoints + soft-lock banner + read-only editor, (3) presence SSE + indicator, (4) conflict dialog 3-way + the three handlers. Keep the lock package pure (inject a clock; no HTTP/SSE knowledge) so the deterministic tests in the Validation Architecture run with zero network.

---

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Soft-lock store (acquire/refresh/force/release/list) | API / Backend (`internal/locks`) | Database/Storage (lock files on disk via `repo`) | Lock truth must be server-authoritative and survive a single-tab refresh; files (not SQLite) keep "data stays as plain files" and reuse the `repo.Resolve` SEC-01 chokepoint. |
| Stale-lock GC | API / Backend (`internal/jobs` `lock_gc` handler + main.go ticker) | — | The single-writer worker already serializes file mutations; GC is just another registered handler enqueued on a ticker. |
| Presence stream | API / Backend (SSE handler) → Browser (EventSource) | — | Presence is derived from the lock store; SSE pushes it; the browser renders. Connection = heartbeat. |
| Optimistic concurrency (revision check) | API / Backend (`pages.Service.Save`) | — | **Already owned here.** The check must stay server-side and unconditional (never a client/lock bypass). |
| Conflict resolution UX | Browser (`DiffReviewDialog` conflict mode + `PageEditor` handlers) | API (re-save / create-copy endpoints — all existing) | The 3-way choice is a UI decision; each choice maps to an *existing* backend call (`Save` at current rev / `Create`). No new conflict endpoint is needed. |

**Tier sanity note for the planner:** there is a real temptation to put "is this page locked?" logic in the browser. It must not live there — the browser only *renders* lock/presence state it receives from the server; acquire/force/release are server calls, and the save-time revision check is the only authority that prevents data loss.

---

## Confirmed Integration Seams (live-code, this session)

Every claim below was read in the working tree on 2026-06-22.

### 1. Lock store — `internal/locks/` is net-new `[VERIFIED: codebase]`
- `ls internal/` confirms **no `locks` package exists** → fully net-new.
- I/O chokepoint confirmed in `internal/repo/files.go`: `Read(rel)` (`:27`), `Write(rel, data)` (`:38`, creates parent dirs via `MkdirAll`), `Exists(rel)` (`:79`), `Remove(rel)` (`:56`, idempotent — missing file is not an error → ideal for release/GC), `MkdirAll(rel)` (`:69`). All route through `Resolve` (`internal/repo/path.go:83`).
- **Path-normalization requirement (Claude's discretion, but constrained):** the lock filename must round-trip through `repo.Resolve`, which **rejects** `..` segments, absolute paths, NUL, and (defense-in-depth) percent sequences (`path.go:114-148`). A page path like `runbooks/deploy.md` contains `/`. Two safe schemes:
  - **(A) Mirror the tree:** `.okf-workspace/locks/runbooks/deploy.md.lock` — keeps `/`, so `repo.Write` auto-creates `.okf-workspace/locks/runbooks/`. Resolves cleanly (no `..`). Human-readable on disk (matches files-as-truth ethos). **Recommended.**
  - (B) Flat slug: replace `/` with a separator into one filename. Avoid: a separator collision is possible and it's less debuggable. Prefer (A).
  - Either way, the input page path is **already validated** by `cleanPathString`/`cleanPathParam` at the handler boundary (`handlers_pages.go:76-97`) before it reaches the lock store, and `repo.Resolve` re-validates — so a traversal-shaped path can never escape `.okf-workspace/locks/`. `[VERIFIED: codebase]`
- **Lock-file JSON shape** (CONTEXT-locked fields):
  ```go
  // internal/locks/lock.go
  type Lock struct {
      Username  string    `json:"username"`
      UserID    int64     `json:"user_id"`
      SessionID string    `json:"session_id"`
      LockedAt  time.Time `json:"locked_at"`
      ExpiresAt time.Time `json:"expires_at"`
  }
  ```
- **Operations** (struct + injected deps + `now func() time.Time`, matching the established service pattern — see `pages.Service` `now` field, `service.go:84`):
  - `Acquire(ctx, pagePath, owner) (Lock, AcquireResult, error)` — if no live lock (or the existing one is expired or is *this* session), write/refresh and return "acquired"; if a *different* live session holds it, return the holder + "held-by-other" (do NOT overwrite). Read existing via `repo.Read` → `json.Unmarshal`; treat unmarshal error / missing file as "no lock."
  - `Refresh(ctx, pagePath, owner) error` — heartbeat: only the current holder may refresh; re-write with `ExpiresAt = now + expiry`. This is how **heartbeat updates `expires_at`** — each heartbeat (and each save) re-writes the file with a bumped `ExpiresAt`.
  - `Force(ctx, pagePath, owner) (Lock, error)` — unconditionally replace the lock with `owner` (take ownership). Lock-only — it does NOT touch the page or its revision.
  - `Release(ctx, pagePath, sessionID) error` — best-effort delete via `repo.Remove` **only if** the on-disk lock's `session_id` matches (don't delete someone else's lock after a TTL takeover); idempotent.
  - `Get(ctx, pagePath) (Lock, bool, error)` and `List(ctx) ([]Lock, error)` — for presence + GC. List walks `.okf-workspace/locks/` (mirror the `internal/search/rebuild.go trashPrefix` walk pattern over `.okf-workspace/`).
- **Atomicity caveat (Pitfall):** `repo.Write` → `os.WriteFile` (`files.go:49`) is **not atomic** (truncate-then-write). At 5 users a torn lock read is rare and self-heals on the next heartbeat, but the safer pattern (planner's discretion) is write-to-temp-then-`os.Rename`. `repo` has no Rename helper today; either add one (routes through `Resolve`) or accept the non-atomic write given the scale + the unmarshal-error-means-no-lock fallback. **Recommend: accept non-atomic for MVP; document it.** `[VERIFIED: codebase]`

### 2. Presence SSE — `handlers_sse.go` Flusher pattern confirmed `[VERIFIED: codebase]`
- Template = `handleExtractionStatus` (`internal/server/handlers_sse.go:41`): asserts `w.(http.Flusher)` (`:50`), sets `Content-Type: text/event-stream` + `Cache-Control: no-cache` + `Connection: keep-alive` + `X-Accel-Buffering: no` (`:56-60`), loops on a `time.Ticker` with `select { case <-ctx.Done(): return; case <-ticker.C: emit }` (`:101-116`), and has an absolute `sseMaxDuration` cap (`:24`, `:99`) so a wedged stream can't leak a goroutine. **Reuse this exact skeleton.**
- **Goroutine cleanup is already correct in the template:** `ctx := r.Context()` (`:62`) + the `<-ctx.Done()` case means a dropped client tears the goroutine down. Mirror it. Add the same `sseMaxDuration`-style cap so a forgotten editor tab is reaped.
- **No global WriteTimeout** is set (`handlers_sse.go:37-40` comment; confirmed `srv := &http.Server{Addr, Handler}` in `main.go:319` has none) → long-lived SSE is safe.
- **Routing:** mount the presence stream on the **authed** group (any authenticated user may observe presence, like reading a page) — `router.go:100` `authed.Group`. Because chi cannot host a sibling wildcard next to `/pages/*`, dispatch presence the **same way** history/version are dispatched: in `handleGetPageOrHistory` (`handlers_pages.go:112`) add a branch `strings.HasSuffix(wild, ".md/presence")` → `h.handlePresence`. This mirrors the established `.md/history` / `.md/version/` suffix dispatch (`:115-118`) and avoids the sibling-wildcard conflict the comments repeatedly warn about. `[VERIFIED: codebase]`
- **SSE event frames (Claude's discretion — recommended shape):** keep it dead simple and reuse the `EventSource` default-message consumer (no `event:` names needed if you push one JSON snapshot per tick):
  ```
  data: {"editors":[{"username":"alice","you":false}],"you_hold_lock":true}\n\n
  ```
  Pushing a full **snapshot** of current editors per tick (rather than join/edit/idle/leave deltas) is simpler, idempotent, drop-tolerant, and matches the extraction-status "emit current state each tick" model. The client renders presence from the latest snapshot. (Join/leave/idle are then *derived client-side* by diffing snapshots if ever needed — not required for the MVP indicator.) The server computes the snapshot from `locks.List` filtered to this page, with `expires_at > now`.
- **Connection id (`SessionConnectionIDKey`):** CONTEXT says add it to the session. Confirmed the SCS session pattern: `SessionUserIDKey = "user_id"` is a string const in `internal/auth/session.go:15`. Add `SessionConnectionIDKey = "connection_id"` there. **Generation (Claude's discretion):** a per-browser id is more naturally **client-generated** (e.g. `crypto.randomUUID()` persisted in `sessionStorage`) and sent as a query param on the presence stream + as the `session_id` field for locks — because one SCS session (one browser) is exactly the granularity CONTEXT wants ("keyed by session + a per-browser connection id"). Storing it server-side in SCS is optional; the simplest correct design is: **`session_id` for a lock = the SCS session token's identity (user) + the client-supplied connection id**, so two tabs in one browser are distinguishable. Recommend: derive lock ownership `SessionID` from a stable per-tab/browser uuid passed by the client; keep `user_id`/`username` from the session (server-trusted, never client-supplied — mirrors `actorUsername`, `handlers_users.go:19`). `[VERIFIED: codebase]`
- **Frontend consumer confirmed reusable:** `subscribeExtractionStatus` (`client.ts:459`) is the exact `EventSource` GET pattern — `new EventSource(url)`, `onmessage` → `JSON.parse(e.data)`, `onerror` → `es.close()`, returns an unsubscribe. `subscribePresence(path, connId, onSnapshot)` is a near-copy. **Reconnect:** native `EventSource` auto-reconnects on transient drops; the UI-SPEC "Reconnecting…" state maps to `onerror` before re-open. (For the editing indicator we don't need the POST-body fetch-stream variant `subscribeAgentChat` — a GET EventSource suffices.) `[VERIFIED: codebase]`

### 3. Optimistic concurrency (COLL-03) — exact signatures confirmed `[VERIFIED: codebase]`
- `func (s *Service) Save(ctx, path, body, frontmatter, baseRevision, user string) error` (`service.go:186`). The 409 floor is **before any write** (`:196-202`): re-reads `current, _ := s.Revision(ctx, path)`; `if current != baseRevision { return ErrStaleRevision }`. Enqueues nothing on stale.
- `func (s *Service) Revision(ctx, path string) (string, error)` (`service.go:176`) → `git.BlobRevision` (the Git blob SHA at HEAD — the opaque per-document revision; never surfaced to the user).
- `ErrStaleRevision` sentinel (`service.go:29`) → `handlers_pages.go:201` `errors.Is(err, pages.ErrStaleRevision)` → `409` with hidden-Git copy.
- Client: `savePage(path, {body, frontmatter, base_revision})` (`client.ts:248`); `PageEditor.tsx:111-120` catches `status === 409` → `setConflict(true)`. The Phase 1 coalescing single-flight saver (`runSaver`, `PageEditor.tsx:92`) already advances `baseRevision.current` after each save and re-reads via `getPage` (`:139-147`).
- **The load-bearing rule is implementable as-is, with no `Save` change:** force-edit is a **separate** call to the lock endpoint; it never calls `Save` and never alters `baseRevision`. When the forced editor then saves, `runSaver` calls `savePage` with the unchanged `base_revision` it read at open time → if a commit landed in between, `Save`'s `current != baseRevision` check fires → 409 → conflict dialog. **Nothing in the save path is aware of locks**, which is exactly the safety property. `[VERIFIED: codebase]`
- **Hardening deltas this phase:**
  1. Today a 409 sets a `conflict` *banner* with a Reload button (`PageEditor.tsx:220-232`). This phase **supersedes** that banner with the conflict **dialog** for the save-collision path (UI-SPEC §Copywriting "KEEP / SUPERSEDE"). Keep the banner component or repurpose `conflict` state to drive the dialog.
  2. **Autosave 409** already routes through the same `runSaver` catch (`:118`), so an autosave collision already sets `conflict` — wire it to open the dialog (not silently drop). Confirmed the saver does NOT drop the edit: on 409 it `return`s with the editor content intact. `[VERIFIED: codebase]`

### 4. Conflict flow (COLL-04) — Create/uniquePath + overwrite path confirmed `[VERIFIED: codebase]`
- **Save-as-copy:** `func (s *Service) Create(ctx, folder, title, user string) (string, error)` (`service.go:115`) → `uniquePath(folder, title)` (`service.go:318`) appends `-2`, `-3`, … until free. **But `Create` scaffolds a *blank* doc** (`:129-136` — `okf.Repair` + set title) — it does NOT take a body. So "Save as copy with my content" is **two steps**: `Create` to get a fresh path + revision, then `Save` that path with my body. The new page's revision is whatever `Create`'s commit produced (a fresh revision, never the conflicted base) → satisfies CONTEXT "fresh revision, original untouched." Client: `createPage(folder, title)` → `{path}` (`client.ts:239`), then `getPage(newPath)` for its fresh revision, then `savePage(newPath, {body: mine, base_revision: freshRev})`, then navigate. The original page is never written. `[VERIFIED: codebase]`
  - **Title for the copy:** "{Original Title} (Copy)". Read the original title from the frontmatter the editor holds (`readField(frontmatter, "title")`, used at `PageEditor.tsx:248`). `uniquePath` auto-dedups if "(Copy)" already exists.
  - **Folder for the copy:** same folder as the original (derive from the page path's dirname).
- **Overwrite:** "save my version against the *current* revision." Implement client-side: on Overwrite, `getPage(path)` → read `fresh.revision`, then `savePage(path, {body: mine, frontmatter: mine, base_revision: fresh.revision})`. If yet another commit landed between the fetch and the save, `Save` 409s **again** → re-open the dialog with the newer server version (never a silent clobber — UI-SPEC §Trust "busy state"). No new endpoint; reuses `Save`. `[VERIFIED: codebase]`
- **Manual merge:** pure client — close the dialog, keep my body in the editor, surface the server version for reference (exact layout is Claude's discretion per UI-SPEC), then a normal save (which itself revision-checks). No backend call beyond the eventual `Save`.
- **Fetching the server version to diff on 409:** `getPage(path)` (`client.ts:222`) returns `{frontmatter, body, revision}`. Diff `oldText = server.body` (their version), `newText = mine` (UI-SPEC). The dialog already accepts `oldText`/`newText` props (`DiffReviewDialog.tsx:23-24`). `[VERIFIED: codebase]`
- **DiffReviewDialog extension:** it is *already* written to be reused in Phase 5 (`DiffReviewDialog.tsx:7-9` says so). Add a discriminated `conflict` prop set (or `mode?: "review" | "conflict"`) that swaps the footer for three buttons (Overwrite `.btn-ghost-destructive`/`.btn-destructive`, Manual merge `.btn-secondary`, Save as copy `.btn-secondary`) and lands initial focus on a **safe** option (NOT Overwrite) — the component's existing focus model already focuses the *safe* button first (`:56-79` focuses `rejectRef`), so the conflict mode keeps that inversion by pointing the focus ref at Save-as-copy/Manual-merge. The diff/focus-trap/Esc/backdrop-cancel stay inherited. `[VERIFIED: codebase]`

### 5. Stale-lock GC — jobs Register/enqueue pattern confirmed `[VERIFIED: codebase]`
- `worker.Register(kind string, h jobs.Handler)` (`worker.go:53`); `Handler = func(ctx, payload string) error` (`queue.go:40`). Existing registrations in `main.go:190/196/222`.
- `worker.Enqueue(ctx, kind, payload)` (`queue.go:74`) — fire-and-forget; the single drain goroutine serializes all handlers (`worker.go:84` `loop`). GC mutating lock files on the same single writer is correct.
- **`lock_gc` design:** register `worker.Register(locks.KindGC, locks.GCHandler(lockStore))` in `main.go` next to the others (before `worker.Start(ctx)` at `:224`). The handler calls `lockStore.GC(ctx)` which `List`s `.okf-workspace/locks/`, and `Remove`s each lock whose `expires_at < now`. Idempotent (`repo.Remove` tolerates a missing file, `files.go:61`).
- **Periodic enqueue:** `runServe(ctx, …)` (`main.go:80`) gives a cancellable `ctx`. Launch a ticker goroutine **before** `srv.ListenAndServe()`:
  ```go
  go func() {
      t := time.NewTicker(lockGCInterval) // e.g. 60s — within the 2-min expiry envelope
      defer t.Stop()
      for {
          select {
          case <-ctx.Done():
              return
          case <-t.C:
              _ = worker.Enqueue(context.Background(), locks.KindGC, "")
          }
      }
  }()
  ```
  `ctx` cancellation stops the ticker on shutdown (the same `ctx` already drives `worker.Start(ctx)`). Payload is empty (GC scans everything). This mirrors the startup `worker.Enqueue(..., search.RebuildPayload())` fire-and-forget calls (`main.go:275/290`). `[VERIFIED: codebase]`
  - **Caveat:** `main.go` currently has **no graceful `srv.Shutdown`** (`:319-320` is a bare blocking `ListenAndServe`). The ticker goroutine is cleaned up by `ctx.Done()` when the process-level context cancels, which is sufficient at this scale. Don't add graceful-shutdown scope creep unless the planner already has it queued. `[VERIFIED: codebase]`

---

## Standard Stack

**No new libraries.** Everything reuses what is already in `go.mod` / `package.json` and confirmed in use:

| Layer | Reused dependency | Already used at | Purpose this phase |
|-------|-------------------|------------------|--------------------|
| Backend SSE | stdlib `net/http` `Flusher` + `time.Ticker` | `handlers_sse.go` | presence stream |
| Backend lock I/O | `internal/repo` (`Read/Write/Exists/Remove/MkdirAll`) | `internal/search/rebuild.go`, `internal/users/seed.go` | `.okf-workspace/locks/*.lock` |
| Backend JSON | stdlib `encoding/json` | everywhere | lock-file marshal |
| Backend jobs | `internal/jobs` (`Register`/`Enqueue`) | `main.go` | `lock_gc` |
| Backend session | `github.com/alexedwards/scs/v2` | `internal/auth/session.go` | `SessionConnectionIDKey` |
| Frontend SSE | `EventSource` (native) | `client.ts subscribeExtractionStatus` | `subscribePresence` |
| Frontend diff | `react-diff-viewer-continued` 4.2.2 | `DiffReviewDialog.tsx` | conflict diff (already wired) |
| Frontend icons | `lucide-react` | shared | `Pencil`/`Users`/`Lock`/`AlertTriangle`/`Loader2`/`WifiOff` |
| Frontend state | `zustand` 5.0.14 + `@tanstack/react-query` | shared | ephemeral lock/presence/conflict state |

**Installation:** none.

---

## Architecture Patterns

### System Architecture Diagram (data flow)

```
                         ┌─────────────────────────── Browser (PageEditor, Edit mode) ───────────────────────────┐
 enter Edit mode  ─────► │ POST acquire-lock ──┐                                                                   │
                         │                     ▼                                                                   │
                         │   ┌──── held-by-other? ── yes ──► render SoftLockBanner (warning) + read-only editor    │
                         │   │                                   │  click "Force edit"                             │
                         │   │                                   ▼                                                 │
                         │   │                             POST force-lock ──► editable                            │
                         │   └── no/own ──► editable                                                               │
                         │                                                                                         │
 every ~30s / on save ─► │ POST refresh-lock (heartbeat bumps expires_at)                                         │
                         │                                                                                         │
 open Edit mode ───────► │ subscribePresence(path) ── EventSource GET /pages/{path}/presence ──┐                   │
                         │        ▲ snapshot per tick                                           │                  │
                         │ PresenceIndicator ◄────────────────────────────────────────────────┘                  │
                         │                                                                                         │
 type / autosave / Save► │ savePage(path, base_revision) ──► PUT /pages/{path}                                     │
                         └──────────────────────────────────────────────┬──────────────────────────────────────┘
                                                                         ▼
   ┌──────────────────────────────────── Go backend ───────────────────────────────────────────────────────────┐
   │ locks.Service  (Acquire/Refresh/Force/Release/Get/List)  ──writes/reads──►  repo.Resolve ──► .okf-workspace/│
   │      ▲                                                                                        locks/*.lock   │
   │      │ List(page, now) snapshot                                                                              │
   │ handlePresence (SSE Flusher, ticker, ctx.Done cleanup, max-duration cap) ──► snapshot frames                │
   │                                                                                                              │
   │ pages.Service.Save ── re-reads git.BlobRevision ── current != base_revision? ── yes ──► ErrStaleRevision    │
   │      │ no                                                          │                          │ (409)        │
   │      ▼ enqueue CommitJob (single-writer worker)                   │                          ▼              │
   │   git commit (hidden)                                             │            Browser: fetch server ver,    │
   │                                                                   │            open DiffReviewDialog(conflict)│
   │ jobs worker ── lock_gc handler (ticker-enqueued) ── List ── Remove expired locks ──► repo.Remove             │
   └──────────────────────────────────────────────────────────────────────────────────────────────────────────┘

 Conflict dialog choices (browser, all map to EXISTING backend calls):
   Overwrite     ─► getPage(rev) → savePage(path, freshRev)  (409 again ⇒ re-open dialog, never silent clobber)
   Manual merge  ─► keep my body, show server body for reference, normal Save (revision-checks)
   Save as copy  ─► createPage(folder,"Title (Copy)") → savePage(newPath, mine) → navigate; original untouched
```

### Pattern 1: Snapshot-per-tick SSE (not delta events)
**What:** the presence stream emits a full editors snapshot each tick, mirroring `handleExtractionStatus`'s "emit current state each tick."
**When to use:** small, drop-tolerant, idempotent presence at 5-user scale.
**Why:** no server-side per-connection delta bookkeeping; a reconnect just resumes getting snapshots; the client never accumulates stale join/leave state.

### Pattern 2: Force-edit is lock-only; Save is the authority
**What:** force-edit calls a lock endpoint and never `Save`; the revision check in `Save` is untouched and unconditional.
**When to use:** always, this phase. It is the safety contract.
**Why:** decouples "who may type" (advisory locks) from "is this write safe" (authoritative revision check), so a stale/ghost lock can never cause data loss.

### Anti-Patterns to Avoid
- **Lock as a save bypass.** Never let force-edit (or lock ownership) skip `Save`'s `current != baseRevision` check. That would reintroduce silent overwrite — the exact thing COLL-03/04 prevent.
- **Direct fs access for lock files.** Never `os.WriteFile` a lock path directly; always `repo.Write`/`repo.Read`/`repo.Remove` (SEC-01). A raw path would bypass `Resolve`.
- **Storing lock truth in the browser.** The browser renders lock/presence; it never decides them.
- **Delta-event presence with server-side per-connection state.** Over-engineered for 5 users and leaks goroutine bookkeeping; use snapshots.
- **Git vocabulary in any user-facing string** (UI-SPEC hidden-Git rule): no "commit/merge conflict/HEAD/SHA/branch." Use "changed somewhere else", "your version", "their version", "save a copy".

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Path safety for lock files | a custom slug→path joiner with manual `..` checks | `repo.Write/Read/Remove` → `repo.Resolve` (`path.go:83`) | The resolver already handles `..`, absolute, NUL, percent-encoding, symlink escape + os.Root. Re-implementing it is the #1 traversal-bug source. |
| Optimistic concurrency | a new revision/etag scheme for conflict mode | existing `pages.Service.Save` + `ErrStaleRevision` (`service.go:186`) | Already correct and tested; every choice (overwrite/copy/merge) routes through it. |
| Save-as-copy dedup | a "does Title (Copy) exist?" loop | `pages.Service.Create`/`uniquePath` (`service.go:318`) | Already appends `-2/-3…` safely and commits via the single writer. |
| SSE plumbing | a bespoke streaming handler | clone `handleExtractionStatus` (`handlers_sse.go:41`) | It already nails Flusher assert, headers, `X-Accel-Buffering`, ctx cleanup, max-duration cap. |
| Frontend SSE consumer | a new EventSource wrapper | clone `subscribeExtractionStatus` (`client.ts:459`) | Same onmessage/onerror/unsubscribe contract; native reconnect. |
| Periodic background work | a new scheduler | `internal/jobs` Register + a `time.Ticker` enqueue | Single-writer drain already serializes file mutations. |
| Conflict diff UI | a second diff dialog | extend `DiffReviewDialog` (built to be reused, `:7`) | Preserves the real-diff trust contract + focus inversion. |

**Key insight:** this phase is almost entirely *composition* of shipped primitives. The genuinely new code is small: one Go package (`internal/locks`), one SSE handler + one lock-endpoints handler, a GC job, and three small frontend additions. Resist building anything the seams already provide.

---

## Common Pitfalls

### Pitfall 1: Force-edit silently bypassing the revision check
**What goes wrong:** a tempting "you forced the lock, so just save" shortcut overwrites a concurrent commit.
**Why it happens:** conflating lock ownership with write safety.
**How to avoid:** force-edit is a lock-only endpoint; the subsequent save still passes the unchanged `base_revision` to `Save`. Verify with the deterministic test below.
**Warning signs:** any code path that sets/forces a lock and then writes without re-checking `Revision`.

### Pitfall 2: Save-as-copy mutating the original
**What goes wrong:** writing the copy to the original path, or carrying the conflicted base revision.
**Why it happens:** reusing the same `savePage(path, …)` call by mistake.
**How to avoid:** `Create` a *new* path first, fetch its *fresh* revision, then `Save` the new path. Never touch the original path in the copy flow.
**Warning signs:** the original page's revision changes after a save-as-copy.

### Pitfall 3: SSE goroutine / lock-file leaks
**What goes wrong:** a forgotten editor tab pins a presence goroutine, or a crashed session leaves a permanent lock.
**Why it happens:** missing `ctx.Done()` cleanup / missing GC.
**How to avoid:** mirror `handleExtractionStatus`'s `<-ctx.Done()` + `sseMaxDuration` cap; the `lock_gc` ticker reaps expired locks; `Release` on unmount is best-effort only (GC is the backstop).
**Warning signs:** goroutine count climbs with open tabs; a lock file outlives its `expires_at`.

### Pitfall 4: Non-atomic lock write torn read
**What goes wrong:** a concurrent read sees a half-written lock JSON.
**Why it happens:** `repo.Write` is truncate-then-write (`files.go:49`), not atomic.
**How to avoid:** treat a JSON unmarshal error as "no lock" (self-heals next heartbeat); or add a `repo.Rename` temp-then-rename helper. At 5 users the unmarshal-fallback is acceptable for MVP.
**Warning signs:** sporadic "no lock" when a holder is active — harmless if the fallback re-acquires on the next heartbeat.

### Pitfall 5: Autosave-vs-conflict interaction
**What goes wrong:** an autosave 409 silently drops the in-flight edit, or the conflict dialog opens repeatedly while typing continues.
**Why it happens:** the debounced `runSaver` keeps firing.
**How to avoid:** the saver already `return`s on 409 with editor content intact (`PageEditor.tsx:118`). On 409, open the dialog AND stop scheduling further autosaves until the conflict is resolved (gate `scheduleAutosave` on `!conflict`). Resolve → reset `conflict` and advance `baseRevision`.
**Warning signs:** the dialog re-opens every second; edits lost after a 409.

### Pitfall 6: Two tabs / one browser indistinguishable
**What goes wrong:** the same user in two tabs fights its own lock, or presence shows "you" as another editor.
**Why it happens:** keying locks/presence by user only, not by a per-tab/browser connection id.
**How to avoid:** include the client-generated connection id in lock `session_id` and presence identity; the presence snapshot marks `you:true` for your own connection so the indicator never shows yourself (UI-SPEC).
**Warning signs:** "Alice is editing" shown to Alice; a refresh self-locks.

---

## Code Examples

### Lock store skeleton (Go) — pattern, not literal
```go
// internal/locks/service.go — struct + injected deps + now func (matches pages.Service)
type Service struct {
    repo *repo.Repo
    now  func() time.Time
    ttl  time.Duration // expiry, e.g. 2*time.Minute
}

func (s *Service) lockPath(pagePath string) string {
    // Mirror the tree under .okf-workspace/locks/ so repo.Resolve accepts it
    // (page paths are pre-validated by cleanPathString; Resolve re-validates).
    return ".okf-workspace/locks/" + pagePath + ".lock"
}

func (s *Service) Get(ctx context.Context, pagePath string) (Lock, bool, error) {
    raw, err := s.repo.Read(s.lockPath(pagePath))
    if errors.Is(err, os.ErrNotExist) { return Lock{}, false, nil }
    if err != nil { return Lock{}, false, err }
    var l Lock
    if json.Unmarshal(raw, &l) != nil { return Lock{}, false, nil } // torn/garbage ⇒ no lock
    if s.now().After(l.ExpiresAt) { return Lock{}, false, nil }       // expired ⇒ no live lock
    return l, true, nil
}
// Acquire: Get → if held by a DIFFERENT live session, return held-by-other; else write+return acquired.
// Refresh: only current holder; re-write ExpiresAt = now+ttl  (heartbeat bumps expires_at).
// Force:   unconditionally write owner (lock-only; never touches the page).
// Release: Remove only if on-disk session_id == caller's (idempotent via repo.Remove).
```
*Note: `repo.Read` wraps `os.ReadFile`; a missing file returns an `os.ErrNotExist`-wrapped error — check with `errors.Is`. `[VERIFIED: codebase]`*

### Presence SSE handler — clone the confirmed template
```go
// Reuse handleExtractionStatus's skeleton verbatim (handlers_sse.go:41):
//   Flusher assert → SSE headers (incl. X-Accel-Buffering: no) → emit snapshot
//   immediately → ticker loop with select{ <-ctx.Done(): return; <-deadline.C: cap; <-ticker.C: emit }.
emit := func() error {
    eds := s.locks.EditorsFor(ctx, pagePath, s.now()) // List filtered to page, expires_at>now
    b, _ := json.Marshal(presenceSnapshot{Editors: eds, ...})
    _, _ = fmt.Fprintf(w, "data: %s\n\n", b)
    flusher.Flush()
    return nil
}
```

### subscribePresence — clone subscribeExtractionStatus (client.ts:459)
```ts
export function subscribePresence(
  path: string, connId: string,
  onSnapshot: (s: PresenceSnapshot) => void,
): () => void {
  const es = new EventSource(`/api/v1/pages/${path}/presence?conn=${connId}`);
  es.onmessage = (e) => { try { onSnapshot(JSON.parse(e.data)); } catch { /* keep last */ } };
  es.onerror = () => { /* native EventSource auto-reconnects ⇒ surface "Reconnecting…" */ };
  return () => es.close();
}
```

---

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| 409 conflict → warning banner + "Reload page" (Phase 1) | 409 → `DiffReviewDialog` conflict mode (overwrite/merge/copy) | This phase | Richer, data-loss-safe resolution; banner copy superseded (UI-SPEC). |
| No editing visibility | Per-page presence over SSE (editing-only) | This phase | Users see "Alice is editing" before colliding. |
| No locks | File-based soft locks + force-edit + GC | This phase | Warns before concurrent edit; never blocks (advisory). |

**Deprecated/outdated:** the standalone Phase-1 conflict *banner* is superseded for the save-collision path. Keep the calm voice; the dialog is the richer surface.

---

## Runtime State Inventory

> This is a feature phase, not a rename/migration. New runtime state is *introduced*, not migrated.

| Category | Items Found | Action Required |
|----------|-------------|------------------|
| Stored data | **New:** `.okf-workspace/locks/*.lock` JSON files (ephemeral, GC-reaped). No existing data references a lock by key. | None to migrate; ensure GC + best-effort release. |
| Live service config | None — locks are intra-app; no external service holds collaboration state. | None. |
| OS-registered state | None — the `lock_gc` ticker is an in-process goroutine, not an OS scheduler entry. | None. |
| Secrets/env vars | None — no new secret. `SessionConnectionIDKey` is a session field, not a secret. | None. |
| Build artifacts | None — no new build step; SPA bundles the two new components into the existing `embed.FS`. | None. |

**Should lock files be Git-committed?** No. `.okf-workspace/locks/` is ephemeral operational state, like the search index (which lives **outside** the repo). Locks live *inside* the repo dir but must **not** be committed — they're written via `repo.Write` (not the CommitJob), so they never enter a commit unless explicitly staged. Confirm `.okf-workspace/locks/` is not picked up by any commit path (the single-writer CommitJob only stages explicit `Paths`, `service.go:267`) — it is not. Consider a `.gitignore` entry for `.okf-workspace/locks/` for cleanliness (planner discretion). `[VERIFIED: codebase]`

---

## Validation Architecture

> nyquist_validation is treated as enabled (no `.planning/config.json` opt-out for this phase). This section drives VALIDATION.md.

### Test Framework
| Property | Value |
|----------|-------|
| Backend framework | Go stdlib `testing` (table tests; inject `now func() time.Time` into `locks.Service` for deterministic expiry — mirrors `pages.Service.now`) |
| Backend config file | none (Go test discovery) |
| Backend quick run | `go test ./internal/locks/...` |
| Backend full suite | `go test ./...` |
| Frontend framework | Vitest + Testing Library (existing — `DiffReviewDialog`/`PageEditor` have prior tests) |
| Frontend config file | `web/vitest.config.*` (existing) |
| Frontend quick run | `cd web && npx vitest run src/components/DiffReviewDialog.test.tsx` |
| Frontend full suite | `cd web && npx vitest run` |

### Phase Requirements → Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| COLL-03 | **Stale-409 still fires after force-edit** — force a lock, land a commit, save with the old base_revision ⇒ `ErrStaleRevision` | unit (Go) | `go test ./internal/pages/ -run TestSaveStaleAfterForce -x` (the existing `Save` stale test already proves the floor; add a variant asserting a lock/force does not alter it) | ❌ Wave 0 (add lock/force variant) |
| COLL-02 | **Lock acquire / held-by-other / force / release** lifecycle | unit (Go) | `go test ./internal/locks/ -run TestAcquireLifecycle -x` | ❌ Wave 0 |
| COLL-02 | **Lock expiry + GC** — a lock past `expires_at` is treated as absent (`Get` returns false) and reaped by `GC` | unit (Go) | `go test ./internal/locks/ -run TestExpiryAndGC -x` | ❌ Wave 0 |
| COLL-02 | **Lock path round-trips through repo.Resolve** — a nested page path (`a/b/c.md`) and a traversal-shaped path are handled safely | unit (Go) | `go test ./internal/locks/ -run TestLockPathSafety -x` | ❌ Wave 0 |
| COLL-04 | **Save-as-copy never mutates the original** — original revision/body unchanged; copy exists at a deduped path with my body | unit (Go) | `go test ./internal/pages/ -run TestSaveAsCopyLeavesOriginal -x` | ❌ Wave 0 |
| COLL-04 | **Conflict-overwrite saves at current rev** — overwrite path fetches current rev then saves; a rev moved mid-overwrite 409s again | unit (Go) + component | `go test ./internal/pages/ -run TestOverwriteAtCurrentRev` ; `npx vitest run src/routes/PageEditor.conflict.test.tsx` | ❌ Wave 0 |
| COLL-04 | **DiffReviewDialog conflict mode** — 3 buttons render; initial focus is NOT Overwrite; real diff always rendered; Esc/backdrop cancel | component (Vitest) | `cd web && npx vitest run src/components/DiffReviewDialog.test.tsx` | ⚠️ extend existing test file |
| COLL-01 | **Presence snapshot** — `EditorsFor(page, now)` returns only live (`expires_at>now`) lock holders for that page; excludes self by connection id | unit (Go) | `go test ./internal/locks/ -run TestEditorsForSnapshot -x` | ❌ Wave 0 |

### Manual / Perceptual (UAT — not automatable, drive with Playwright per app-validation recipe)
- **Presence indicator live:** two browser sessions, both open Edit on the same page → each sees "{other} is editing" within a tick; closing one clears the other's indicator after TTL.
- **Soft-lock banner + read-only:** session B opening a page session A is editing sees the warning banner + read-only editor; "Force edit" flips to editable.
- **Conflict dialog 3-way:** force a real save collision (A saves, B saves stale) → B sees the dialog with a real diff; Save-as-copy creates a copy and leaves A's page intact; Overwrite replaces; Manual merge keeps B's body with A's visible.
- **Reconnecting state:** kill the SSE connection → indicator shows "Reconnecting…" (warning) then recovers.

### Sampling Rate
- **Per task commit:** `go test ./internal/locks/... ./internal/pages/...` (sub-second).
- **Per wave merge:** `go test ./...` + `cd web && npx vitest run`.
- **Phase gate:** full suite green before `/gsd-verify-work`; then the manual/Playwright UAT items above.

### Wave 0 Gaps (create before implementation)
- [ ] `internal/locks/service_test.go` — acquire/force/release/expiry/GC/path-safety/snapshot (COLL-01/02)
- [ ] `internal/pages/conflict_test.go` (or extend `service_test.go`) — save-as-copy-leaves-original, overwrite-at-current-rev, stale-after-force (COLL-03/04)
- [ ] extend `web/src/components/DiffReviewDialog.test.tsx` — conflict-mode footer + safe-focus + always-diff
- [ ] `web/src/routes/PageEditor.conflict.test.tsx` — 409 opens dialog; autosave 409 does not drop the edit
- [ ] Framework install: none (Go testing + Vitest already present)

---

## Security Domain

> `security_enforcement` treated as enabled. Untrusted-input surface per CLAUDE.md SPEC §21.

### Applicable ASVS Categories
| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V1 Architecture | yes | Lock/presence are advisory; the authoritative write-safety control is the server-side revision check in `Save` (never client/lock-bypassable). |
| V2 Authentication | yes (reuse) | Session-bound user via SCS; lock ownership `username`/`user_id` come from the session (`actorUsername`), **never** client body — mirror `handlers_users.go:19`. |
| V4 Access Control | yes | Lock acquire/force/release endpoints editor-gated (`auth.RequireRole(RoleEditor)`, `router.go:163`); presence read is any-authed (matches page-read model). |
| V5 Input Validation | yes | Page path for lock files validated by `cleanPathString`/`cleanPathParam` **and** re-validated by `repo.Resolve` — the lock path can never escape `.okf-workspace/locks/`. Client connection id is opaque; treat as untrusted, use only for self/dedup matching, never as a path component. |
| V6 Cryptography | no | No crypto introduced. Connection id is an opaque identifier, not a secret. |
| CSRF | yes (reuse) | Lock mutations are POST/PUT → inherit global nosurf CSRF; the SSE presence stream is a GET (no CSRF, like the extraction-status GET). |

### Known Threat Patterns for this stack
| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Path traversal via crafted page path → lock file escapes the repo | Tampering | `repo.Resolve` (rejects `..`/absolute/NUL/percent/symlink-escape + os.Root) — never construct lock paths by hand. |
| Client-supplied username/role in a lock body → spoofed ownership/privilege | Spoofing / EoP | Derive `username`/`user_id`/role from the session only; the client supplies only the opaque connection id. |
| Force-edit used to bypass the revision check → silent overwrite | Tampering | Force-edit is lock-only; `Save`'s `current != baseRevision` check is unconditional. Covered by `TestSaveStaleAfterForce`. |
| SSE goroutine exhaustion via many idle editor tabs | DoS | `<-ctx.Done()` cleanup + `sseMaxDuration`-style cap (clone `handleExtractionStatus`). |
| Permanent lock from a crashed session → denial of editing | DoS | TTL + `lock_gc` ticker reap; force-edit always available as the escape hatch. |
| Stored XSS via a username rendered in the presence indicator | Tampering | Render names as React text (auto-escaped), never `dangerouslySetInnerHTML`; usernames are already constrained at creation. |

---

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | Mirror-the-tree lock path scheme (`.okf-workspace/locks/{page}.lock`) is preferred over a flat slug | Seam 1 | Low — both pass `Resolve`; mirror is more debuggable. Planner may pick either. `[ASSUMED]` |
| A2 | Snapshot-per-tick presence (not join/leave deltas) is the right granularity | Seam 2 / Pattern 1 | Low — simpler and drop-tolerant at 5 users; deltas can be derived client-side later. `[ASSUMED]` |
| A3 | Connection id is client-generated (`crypto.randomUUID()` in `sessionStorage`) and passed as a query param + lock `session_id` | Seam 2 | Low — alternative is server-minted in SCS; either distinguishes tabs. Planner's discretion per CONTEXT. `[ASSUMED]` |
| A4 | Non-atomic lock write is acceptable for MVP (unmarshal-error ⇒ no-lock fallback) rather than adding a `repo.Rename` | Seam 1 / Pitfall 4 | Low — torn read self-heals next heartbeat at 5 users. If the planner wants atomicity, add a resolver-gated `Rename`. `[ASSUMED]` |
| A5 | `lock_gc` interval ≈60s (within the 2-min expiry envelope) | Seam 5 | Low — any interval < expiry works; CONTEXT leaves cadence to discretion. `[ASSUMED]` |
| A6 | Lock files must NOT be Git-committed (ephemeral op state) | Runtime State Inventory | Low — committing them would pollute history; confirmed the CommitJob only stages explicit paths. A `.gitignore` entry is optional polish. `[ASSUMED]` |
| A7 | One stream for presence (snapshot includes `you_hold_lock`) rather than separate presence+lock streams | Seam 2 | Low — CONTEXT explicitly leaves "one stream or two" to discretion; one snapshot stream is simplest. `[ASSUMED]` |

---

## Open Questions

1. **Presence + lock: one endpoint or two?**
   - What we know: CONTEXT leaves this to Claude's discretion; the snapshot can carry both `editors[]` and `you_hold_lock`.
   - What's unclear: whether the lock acquire/force/release should be REST endpoints separate from the SSE presence stream (they should — mutations are POST/PUT with CSRF; presence is a GET stream).
   - Recommendation: **one SSE presence stream (GET)** carrying the snapshot, plus **separate editor-gated POST/PUT lock endpoints**. Dispatch both off the `/pages/*` catch-all by suffix (`.md/presence`, `.md/lock`, `.md/lock/force`, `.md/lock/release`) to avoid the sibling-wildcard conflict.

2. **Does `repo` need a `Rename` helper for atomic lock writes?**
   - What we know: `repo` has `Write/Read/Remove/MkdirAll/Exists` but no `Rename`.
   - What's unclear: whether MVP tolerates non-atomic writes (A4).
   - Recommendation: accept non-atomic for MVP; revisit only if torn reads are observed. If added, the `Rename` must route through `Resolve` for both src and dst.

---

## Smallest-Safe Vertical Slice Ordering

Dependency-ordered; each slice is independently testable and lands its own tests.

1. **Slice 1 — Lock store + GC + tests (backend-only, no UI).**
   New `internal/locks` (`Lock`, `Service` with injected `now`, `Acquire/Refresh/Force/Release/Get/List/EditorsFor`), the `lock_gc` job handler + `KindGC`, and the main.go ticker wiring. Full unit suite (acquire/force/release/expiry/GC/path-safety/snapshot). **Adds `SessionConnectionIDKey` const.** No HTTP yet. *Why first:* everything else depends on the store; it's pure and fast to test.

2. **Slice 2 — Lock endpoints + soft-lock banner + read-only editor.**
   Editor-gated acquire/force/release endpoints (dispatched off `/pages/*` by suffix); `SoftLockBanner` component; `PageEditor` acquires on entering Edit, refreshes on heartbeat + save, releases on unmount; read-only `LivePreviewEditor` under another's lock; "Force edit" → force endpoint. *Why second:* delivers COLL-02 end-to-end without touching the save path.

3. **Slice 3 — Presence SSE + indicator.**
   `handlePresence` SSE (clone `handleExtractionStatus`); `subscribePresence` (clone `subscribeExtractionStatus`); `PresenceIndicator` in the toolbar with connecting/reconnecting states. *Why third:* reads the lock store from Slice 1; independent of conflict.

4. **Slice 4 — Conflict dialog 3-way + the three handlers + save-as-copy.**
   Extend `DiffReviewDialog` with `conflict` mode (3-button footer, safe-focus); `PageEditor` routes both explicit and autosave 409s to the dialog (supersede the Phase-1 banner); wire Overwrite (getPage→savePage at fresh rev), Manual merge (keep body + show server), Save as copy (createPage→savePage→navigate). *Why last:* depends on nothing from 2/3 but is the richest UI; the COLL-03 hardening (force-edit doesn't bypass) is *verified* here because the conflict path is what proves it.

**Parallelism note:** Slices 3 and 4 are independent of each other (both depend only on Slice 1 / existing save path) and could run in parallel waves; Slice 2 should land before Slice 3 if presence is sourced purely from locks.

---

## Sources

### Primary (HIGH confidence — read in the live tree this session)
- `internal/pages/service.go` — `Save`/`Revision`/`ErrStaleRevision`/`Create`/`uniquePath`/`now`
- `internal/server/handlers_pages.go` — `handleSavePage` 409 mapping, `cleanPathString`/`cleanPathParam`, suffix dispatch
- `internal/server/handlers_sse.go` — `handleExtractionStatus` Flusher/ticker/ctx-cleanup/max-duration template
- `internal/repo/path.go` + `internal/repo/files.go` — `Resolve`, `Read/Write/Exists/Remove/MkdirAll`
- `internal/auth/session.go` + `internal/auth/rbac.go` — `SessionUserIDKey`, `CurrentUser`, `RequireRole`
- `internal/server/handlers_users.go` — `actorUsername`
- `internal/jobs/queue.go` + `internal/jobs/worker.go` — `Register`/`Enqueue`/`Handler`/single-writer drain
- `internal/server/router.go` — authed + editor groups, `/pages/*` catch-all dispatch
- `cmd/okf-workspace/main.go` — worker registration, `worker.Start(ctx)`, `runServe(ctx)` lifecycle, server start
- `web/src/routes/PageEditor.tsx` — autosave/`runSaver`/409 `conflict` state/toolbar
- `web/src/components/DiffReviewDialog.tsx` — reusable diff/trust contract/focus inversion
- `web/src/api/client.ts` — `savePage`/`createPage`/`getPage`/`subscribeExtractionStatus`/`subscribeAgentChat`
- `.planning/REQUIREMENTS.md` (COLL-01..04), `SPEC.md §13.1`, `05-CONTEXT.md`, `05-UI-SPEC.md`

### Secondary / Tertiary
- None — no external lookups were needed; this phase composes existing, in-repo primitives.

---

## Metadata

**Confidence breakdown:**
- Integration seams: HIGH — every seam confirmed against the live tree with `file:func:line`.
- Standard stack: HIGH — no new dependencies; all reused libs confirmed in use.
- Architecture / patterns: HIGH — derived directly from shipped patterns (`handleExtractionStatus`, jobs worker, `pages.Service`).
- Net-new design details (path scheme, connection id, frames, GC cadence): MEDIUM — correct and safe, but explicitly left to planner discretion by CONTEXT (logged as A1–A7).

**Research date:** 2026-06-22
**Valid until:** 2026-07-22 (stable internal codebase; re-confirm only if the save path, repo resolver, or SSE handler change before planning).
