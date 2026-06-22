# Phase 5: Collaboration - Context

**Gathered:** 2026-06-22
**Status:** Ready for planning

<domain>
## Phase Boundary

Let a small team (~5) edit concurrently without silently overwriting each other: presence (see who is currently editing), soft locks (warn + allow force-edit), optimistic concurrency hardening (per-document revision; stale saves rejected), and conflict resolution (a real diff with overwrite / manual-merge / save-as-copy). Covers COLL-01..COLL-04. Hardens and completes the optimistic-concurrency floor scaffolded in Phase 1, and reuses the `DiffReviewDialog` built in Phase 4.

Out of scope (explicitly deferred to SPEC §13.2): realtime/CRDT collaboration (Yjs), WebSocket collab rooms, cursor awareness, per-paragraph merge. Soft locks + optimistic concurrency are the deliberate MVP tradeoff for a 5-user team.

</domain>

<decisions>
## Implementation Decisions

### Soft Lock Mechanics
- **File-based soft locks** in `.okf-workspace/locks/{normalized-page}.lock` as JSON `{username, user_id, session_id, locked_at, expires_at}`, written/read via `repo.Write`/`repo.Read` (path-safety through `repo.Resolve` — no direct fs access). New package `internal/locks/`.
- **Acquired on entering edit mode** (not on view); **refreshed on heartbeat + on save**; best-effort released on leave/unmount.
- **TTL: ~30s heartbeat, ~2min expiry.** A periodic `lock_gc` job reaps lock files past `expires_at` (session end / crash never causes a permanent lock).
- **Force-edit:** when another session holds the lock, the editor is **read-only** with a "Force edit" button. Force-edit takes ownership (replaces the lock) — but the **optimistic-concurrency revision check STILL runs on save** (force-edit is NOT a bypass; see Concurrency Hardening). This is the load-bearing safety rule.

### Presence (COLL-01)
- **Delivered over a per-page SSE stream** (`GET /api/v1/pages/{path}/presence`), reusing the existing `internal/server/handlers_sse.go` Flusher pattern and the Phase 4 fetch-stream SSE consumer on the frontend.
- **Granularity = "currently editing"** (lock / edit-mode holders), MVP — not passive viewers.
- **Indicator UI:** a small "Alice is editing" indicator near the editor header + the soft-lock banner; minimal, Obsidian-style (reuse tokens.css; no avatar stack in MVP).
- **Heartbeat/identity:** presence keyed by session + a per-browser connection id; the SSE connection itself is the heartbeat (drop → "leave" after TTL); the client reconnects on drop. Add `SessionConnectionIDKey` to the session.

### Conflict Resolution (COLL-04)
- **Trigger:** on a stale-save **409** (`ErrStaleRevision`), fetch the current server version and open the **`DiffReviewDialog` in a new `conflict` mode** (old = server version, new = my version) — a real diff, never prose.
- **Three choices (SPEC §13.1):**
  - **Overwrite** — save my version against the *current* revision (force past the conflict).
  - **Manual merge** — open my version in the editor with the server version visible for reference; resolve and do a normal save.
  - **Save as copy** — create a new page ("Original Title (Copy)", auto-deduplicated via the existing `pages.Service.Create`/`uniquePath`) with my content; **the original page is never modified**.
- **Dialog reuse:** extend `DiffReviewDialog` with a `conflict` mode + 3-button footer (Overwrite / Manual merge / Save as copy), preserving the real-diff trust contract. The Phase 4 Approve/Reject mode is unchanged.

### Concurrency Hardening & Stale Locks
- **Revision check enforced even after force-edit:** the save always does the `BaseRevision` check → 409 → conflict UI if a commit landed in between. No silent overwrite, ever, including past a forced lock.
- **Stale-lock cleanup:** the periodic `lock_gc` job deletes expired locks; best-effort release on editor unmount.
- **Autosave interaction:** the Phase 1 autosave (debounced, coalesced saver) keeps working; a 409 from autosave surfaces the conflict UI rather than silently dropping the edit.
- **Save-as-copy semantics:** the new page starts with a fresh (empty/new) revision and never carries the conflicted base revision; the original is untouched.

### Claude's Discretion
- Exact lock-file path normalization scheme, the connection-id generation, and the SSE event frames (`presence`/`heartbeat` shapes).
- Heartbeat cadence + GC interval within the stated TTL envelope.
- The precise read-only/force-edit visual treatment and the manual-merge editor layout (defer specifics to the UI-SPEC).
- Whether presence and lock state share one stream or two endpoints.

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- **Optimistic concurrency (COLL-03) already exists** (Phase 1): `pages.Service.Save(...baseRevision...)` returns `ErrStaleRevision`; `pages.Service.Revision(ctx, path)` = Git blob SHA at HEAD; `handlers_pages.go handleSavePage` maps `ErrStaleRevision`→409; frontend `client.ts savePage` sends `base_revision` and `PageEditor.tsx` already catches 409 into a `conflict` state.
- **SSE infra** (`internal/server/handlers_sse.go`): `text/event-stream` + `http.Flusher` + ticker (extraction-status stream) — the template for the presence stream. Frontend SSE consumer pattern exists from Phase 4 (`subscribeAgentChat` fetch-stream) and the attachment extraction-status consumer.
- **`DiffReviewDialog`** (`web/src/components/DiffReviewDialog.tsx`, built Phase 4): props `{open, title, oldText, newText, summary?, onApprove, onReject, stale?, busy?}` — extend with a `conflict` mode + 3-button footer.
- **App-private dir**: `.okf-workspace/` already holds `manifest.json` (`internal/users/seed.go`) and `trash/` (`internal/search/rebuild.go` `trashPrefix`); locks go in `.okf-workspace/locks/`. All I/O via `repo.Read/Write/Exists/MkdirAll` → `repo.Resolve` (SEC-01 chokepoint).
- **Sessions/current user**: `auth.CurrentUser(ctx)` + the `actorUsername(ctx)` helper (`handlers_users.go`) extract the username for lock ownership + presence; SCS session manager (`internal/auth/session.go`).
- **Job worker** (`internal/jobs`): `Worker.Register(kind, handler)` + FIFO drain — register a `lock_gc` handler, enqueue periodically from `cmd/okf-workspace/main.go` (ticker).
- **Save-as-copy**: `pages.Service.Create(folder, title, user)` + `uniquePath()` auto-dedup; `client.ts createPage` → returns the new path.
- **Editor**: `web/src/routes/PageEditor.tsx` (save flow, autosave, the existing 409 `conflict` state + banner at ~line 220) + `LivePreviewEditor.tsx` (read-only/editable toggle) — where presence indicator + soft-lock banner + conflict dialog mount.

### Established Patterns
- Backend service = struct + injected deps + `now func() time.Time`; constructed in main.go; thin chi handlers calling the service; sentinel errors mapped to HTTP via `errors.Is`.
- Optimistic concurrency via `BaseRevision` → 409; CSRF + `RequireRole(RoleEditor)` on mutating routes.
- SSE = Flusher + `X-Accel-Buffering: no` + ctx-cancellation + `defer flush/close`.
- Frontend dialogs on the shared `Dialog.tsx`; zustand for ephemeral cross-component state (e.g. `agentContext`, `agentPanel`).

### Integration Points
- New package `internal/locks/` (lock store + manager) — SPEC §16 service shape.
- New `internal/server/handlers_presence.go` (or extend handlers) — `GET /api/v1/pages/{path}/presence` SSE; lock acquire/refresh/force/release endpoints.
- `internal/auth/session.go` — add `SessionConnectionIDKey`.
- `cmd/okf-workspace/main.go` — wire the lock store + lock_gc ticker.
- Frontend: `PresenceIndicator` + `SoftLockBanner` components, a `subscribePresence(path)` in `client.ts`, the `conflict` mode in `DiffReviewDialog`, and the conflict handlers (overwrite / manual-merge / save-as-copy) in `PageEditor.tsx`.

</code_context>

<specifics>
## Specific Ideas

- The load-bearing safety rule (ROADMAP note): the revision check must STILL run when a user force-edits past a soft lock; stale locks (session end/crash) must never cause silent data loss.
- Conflict UX is well-specified (SPEC §13.1) — overwrite / manual-merge / save-as-copy — and reuses the Phase 4 DiffReviewDialog. No phase research needed per the ROADMAP.
- Soft locks live in `.okf-workspace/locks/` with user + heartbeat TTL; presence delivered over SSE.

</specifics>

<deferred>
## Deferred Ideas

- Realtime/CRDT collaboration (Yjs), WebSocket collab rooms, live cursor awareness, per-paragraph merge — SPEC §13.2, out of MVP.
- Passive-viewer presence (who's reading, not editing) — MVP shows editing presence only.
- Avatar stacks / rich presence — minimal indicator in MVP.

</deferred>
