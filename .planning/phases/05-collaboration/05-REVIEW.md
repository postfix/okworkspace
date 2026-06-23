---
phase: 05-collaboration
reviewed: 2026-06-22T12:56:11Z
depth: deep
files_reviewed: 16
files_reviewed_list:
  - internal/locks/lock.go
  - internal/locks/service.go
  - internal/locks/gc.go
  - internal/locks/service_test.go
  - internal/server/handlers_locks.go
  - internal/server/handlers_presence.go
  - internal/server/handlers_pages.go
  - internal/server/router.go
  - internal/server/handlers_auth.go
  - internal/auth/session.go
  - internal/pages/forceedit_test.go
  - internal/pages/saveascopy_test.go
  - web/src/routes/PageEditor.tsx
  - web/src/components/DiffReviewDialog.tsx
  - web/src/components/PresenceIndicator.tsx
  - web/src/components/SoftLockBanner.tsx
  - web/src/components/DiffReviewDialog.test.tsx
  - web/src/routes/PageEditor.conflict.test.tsx
  - web/src/api/client.ts
  - web/src/lib/connId.ts
findings:
  critical: 0
  warning: 1
  info: 2
  total: 3
status: resolved
resolution: All 3 findings fixed in commit after review (WR-01 forcedRef heartbeat guard, IN-01 SessionConnectionIDKey removed, IN-02 save-as-copy compensating delete + tree invalidate). Backend build/vet/test green; frontend tsc + vitest 289/289 green.
---

# Phase 5: Code Review Report

**Reviewed:** 2026-06-22T12:56:11Z
**Depth:** deep
**Files Reviewed:** 16 source + 4 test files
**Status:** resolved (was: issues_found — 0 blockers, 1 warning, 2 info; all 3 fixed post-review)

## Summary

Phase 5 (Collaboration) is a high-quality, defensively-written slice. I focused the
adversarial review on the seven concurrency/security failure modes called out in
the prompt and found **no Critical/BLOCKER issues**. The load-bearing invariants
hold:

- **COLL-03 (force-edit never bypasses optimistic concurrency) — VERIFIED.**
  `locks.Force` (service.go:151-153) is a single `s.write` of the lock file; it
  never reads/writes the page body or any revision. `handleForceLock`
  (handlers_locks.go:115-133) calls only `locks.Force`. The save path
  (`handleSavePage` → `pages.Save`) is untouched and still 409s on a stale
  `base_revision` **before any write**. `TestForceEditStillRejectsStaleSave`
  proves the full sequence end-to-end (forced lock + landed commit ⇒ stale save
  still `ErrStaleRevision`, stale bytes never land, fresh-revision save succeeds).
  This is the correct decoupling of *who may type* from *is the write safe*.

- **Path safety — VERIFIED.** `lockPath` (service.go:75-82) re-derives and
  `filepath.Clean`s the mirror-the-tree path and rejects anything that does not
  stay under `.okf-workspace/locks/`. I independently exercised the guard against
  `../../etc/passwd`, `../app.db`, `../../app.db`, and `x/../../app.db` — all
  rejected with `ErrUnsafePagePath`; only in-subtree paths pass. Every lock-file
  touch also routes through `repo.Read/Write/Remove` (the SEC-01 resolver
  chokepoint), and the GC/presence walk reads back through `repo.Read`, never
  `os.*`. The HTTP layer additionally rejects `..`/NUL/absolute segments via
  `cleanPathString` before the store is reached (defense in depth).

- **Identity trust boundary — VERIFIED.** The lock `Username`/`UserID` are filled
  from `auth.CurrentUser`/`actorUsername` (session-bound), never from the request
  body. The only client-supplied field is the opaque `conn` (the lock
  `SessionID`, used solely for self/dedup matching, never as a path component).
  The `Owner` type is intentionally distinct from on-disk `Lock` to make a
  client-named username un-passable.

- **Lock store races / TTL / GC — sound.** Acquire/Refresh/Release all funnel
  through one `write`; Release deletes only when the on-disk `SessionID` matches
  (a TTL-takeover loser cannot clobber the new holder); torn/garbage reads
  self-heal as "no live lock"; GC reaps only `now.After(ExpiresAt)` files and is
  idempotent. The injected clock makes expiry deterministic. There is no
  meaningful GC-reaps-a-just-renewed-lock window: GC and Get both compare against
  the same `ExpiresAt`, and a renew rewrites the file with a future expiry.

- **SSE presence — sound.** `handlePresence` (handlers_presence.go) tears down on
  `ctx.Done()` (client disconnect) and an absolute 30-minute max-duration cap
  (T-05-11), flushes per tick, asserts `http.Flusher`, and surfaces only
  `username`+`you` (never session/user id). It is a GET under the authed group
  (CSRF-exempt, same authority as reading the page). No goroutine/EventSource leak.

- **Auth/CSRF — correct.** Lock mutations (acquire/force/release) are POSTs under
  the editor subgroup (`RequireRole(editor)`) and inherit the global nosurf CSRF;
  presence is a GET, appropriately exempt.

- **React lifecycle — mostly correct.** The lock-acquire effect clears its
  interval and best-effort-releases on unmount/path change; the presence
  subscription returns its unsubscribe; the autosave debounce timer is cleared on
  unmount; the saver is single-flight with refs that prevent stale-closure /
  lost-write; **autosave is correctly gated while the conflict dialog is open**
  (proven by the conflict test); and `DiffReviewDialog` deliberately focuses the
  *safe* control (Save-as-copy in conflict mode, Reject in review mode) — never the
  destructive Overwrite/Approve (proven by tests).

Backend `go test ./internal/locks/... ./internal/pages/...` is green and the
package builds. The findings below are a UI-state polish issue and two
dead-code/cleanup nits — none is load-bearing.

## Narrative Findings (AI reviewer)

### WR-01: Force-edit can be transiently re-locked by an in-flight stale heartbeat (comment claims a guard that does not exist)

**File:** `web/src/routes/PageEditor.tsx:142-153`
**Severity:** Warning

The lock-acquire effect runs `tryAcquire` on a 30s interval and unconditionally
sets `setLockedBy(...)` from each response:

```ts
async function tryAcquire() {
  try {
    const res = await acquireLock(path, conn);
    if (cancelled) return;
    // Don't override an in-progress force-edit takeover with a stale heartbeat.
    setLockedBy(res.result === "held-by-other" ? res.holder?.username ?? "Someone" : null);
  } catch { /* ... */ }
}
```

The comment on line 146 explicitly promises that a stale heartbeat will not
override a force-edit takeover, but **there is no code implementing that guard.**
Race: a heartbeat `acquireLock` request is dispatched while the other session still
holds the lock (it will resolve `held-by-other`); the user then clicks **Force
edit**, which writes the lock for this `conn` and sets `lockedBy = null` (surface
becomes editable). The earlier in-flight heartbeat then resolves with the now-stale
`held-by-other` snapshot and calls `setLockedBy(holder)`, **washing the editor back
to read-only** even though this session now holds the lock. It self-heals on the
next heartbeat (≤30s) because a same-session `acquireLock` returns `acquired`, so it
is a transient UI glitch, not a data-safety issue (the save path is unaffected) —
hence Warning, not Blocker. But it can yank the caret mid-edit and is exactly the
case the comment claims to handle.

**Fix:** Track a force-edit "generation"/flag and ignore heartbeat results that
predate it (or short-circuit a heartbeat whose result is `held-by-other` while a
force-in-progress/just-completed flag is set). Minimal version:

```ts
const forcedRef = useRef(false); // set true in onForceEdit success, cleared on path change
// in tryAcquire, after `if (cancelled) return;`
if (res.result === "held-by-other" && forcedRef.current) return; // we just took over
setLockedBy(res.result === "held-by-other" ? res.holder?.username ?? "Someone" : null);
```
Reset `forcedRef.current = false` in the effect body so a genuine later takeover by
someone else can still re-lock this session.

### IN-01: `SessionConnectionIDKey` is dead code — declared but never read or written

**File:** `internal/auth/session.go:17-19`
**Severity:** Info

```go
// SessionConnectionIDKey is the SCS session key holding the client-generated
// connection id used as the soft-lock session_id (COLL-02 presence/lock owner).
const SessionConnectionIDKey = "connection_id"
```

A repo-wide grep shows this exported constant has **no consumer** anywhere in
`internal/` or `web/`. The final design takes the lock `SessionID` from the request
body's opaque `conn` field (handlers_locks.go), not from the session — so this
constant (introduced in commit `836f24d`) is a vestige of an abandoned approach. It
is harmless but misleads a future reader into thinking the connection id is
session-stored (it is not), and its doc-comment contradicts the actual trust model.

**Fix:** Remove the constant and its comment. If a server-stored connection id is
wanted later (e.g. to stop the client choosing its own `SessionID`), re-introduce it
together with the code that writes it at login and reads it in `lockOwner`.

### IN-02: Save-as-copy can leave an orphaned empty page if the copy's save fails

**File:** `web/src/routes/PageEditor.tsx:424-451`
**Severity:** Info

`onConflictSaveAsCopy` creates the new page first, then `getPage` + `savePage` the
copy:

```ts
const { path: newPath } = await createPage(folder, `${title} (Copy)`);
const fresh = await getPage(newPath);
await savePage(newPath, { body: bodyRef.current, ... });
```

If `getPage(newPath)` or `savePage(newPath, ...)` throws (transient network/server
error), the `catch` surfaces a recoverable save-error but the already-created
**empty `{title} (Copy)` page is left behind** in the tree (and `tree` is not
invalidated on the failure path, so it may not even be visible until a refetch).
Re-trying then creates `{title} (Copy)-2`, accumulating empty stubs. Not a data-loss
or safety issue (the original is untouched and the user's body is preserved in the
editor), so Info — but it is a small cleanliness/UX wart on the failure path.

**Fix (optional):** On the copy-save failure, either delete the just-created empty
page (`deletePage(newPath)`) before surfacing the error, or move the create to
*after* a successful body save is not possible here (createPage mints the path), so
prefer the compensating delete, or document that an empty copy may remain and is
trashable. At minimum, invalidate the `tree` query in the catch so the stub is
visible/recoverable.

---

_Reviewed: 2026-06-22T12:56:11Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: deep_
