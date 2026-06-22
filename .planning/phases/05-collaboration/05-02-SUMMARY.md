---
phase: 05-collaboration
plan: 02
subsystem: collaboration
tags: [locks, soft-lock, http, csrf, editor-gated, react, page-editor, force-edit, coll-02]

# Dependency graph
requires:
  - phase: 05-collaboration
    plan: 01
    provides: "internal/locks store (Acquire/Force/Release/Get + same-session-refresh), Owner shape, SessionConnectionIDKey, GC-wired lockStore in main.go"
  - phase: 02-storage-safety
    provides: "repo.Resolve SEC-01 path chokepoint (inherited via the lock store)"
provides:
  - "Three editor-gated lock HTTP endpoints (acquire/force/release) dispatched off the POST /pages/* catch-all by .md/lock[/force|/release] suffix; identity from the session, only the opaque connection id from the body"
  - "client.ts acquireLock/forceLock/releaseLock (CSRF-bearing POSTs)"
  - "SoftLockBanner warning component (role=status) + verbatim UI-SPEC copy + Force edit + busy/failed states"
  - "PageEditor soft-lock lifecycle: acquire-on-edit, ~30s heartbeat + on-save refresh, best-effort release-on-unmount, read-only-under-another's-lock surface, Force edit take-over"
  - "getConnId() — stable per-tab opaque connection id (the lock SessionID)"
affects: [05-03 presence/SSE slice, 05-04 conflict dialog slice]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Sub-action dispatch off the single POST /pages/* catch-all by .md-anchored suffix (sibling-wildcard conflict avoidance), longer suffixes checked first"
    - "Server-trusted lock Owner: Username=actorUsername(session), UserID=CurrentUser().UserID(); ONLY the opaque conn id is client-supplied (T-05-06)"
    - "Acquire-doubles-as-heartbeat: a ~30s interval re-calls acquireLock (same-session Acquire re-stamps TTL); on-save refresh on the runSaver success branch"
    - "Force edit is lock-only on both ends — handleForceLock calls locks.Force, onForceEdit calls forceLock alone (never savePage, never base_revision)"

key-files:
  created:
    - internal/server/handlers_locks.go
    - web/src/components/SoftLockBanner.tsx
    - web/src/components/SoftLockBanner.css
    - web/src/lib/connId.ts
  modified:
    - internal/server/handlers_pages.go
    - internal/server/handlers_auth.go
    - internal/server/router.go
    - cmd/okf-workspace/main.go
    - web/src/api/client.ts
    - web/src/routes/PageEditor.tsx
    - web/src/routes/PageEditor.css

key-decisions:
  - "Lock suffix branches placed at the TOP of handleRenamePage and anchored on .md (.md/lock/force|/lock/release|/lock) so the force/release variants win over the bare /lock and a page in a folder named 'lock' is never mis-routed"
  - "conn is required (blank conn => 400): without a SessionID a release could not be attributed and could clobber another holder; the heartbeat/release contract needs a stable id"
  - "Acquire is the heartbeat refresh (no dedicated refresh endpoint this slice) — the store already treats a same-session Acquire as a refresh, so one endpoint covers acquire + heartbeat + on-save refresh"
  - "getConnId persists in sessionStorage (reload keeps the lock as a refresh, not a new held-by-other) with an in-memory fallback for private-mode/disabled storage"
  - "Save path (pages.Save / base_revision optimistic concurrency) is provably untouched — COLL-02 is lock-only; force-edit is take-over, not a save bypass (T-05-09)"

patterns-established:
  - "Editor-gated POST sub-action via the /pages/* catch-all inheriting RequireRole(RoleEditor) + global nosurf CSRF with zero new route lines"
  - "Read-only-under-another's-lock = genuine read-only editor (LivePreviewEditor readOnly, no caret) + disabled Save + muted hint, never a cosmetic dim"

requirements-completed: [COLL-02]

# Metrics
duration: ~6min
completed: 2026-06-22
status: complete
---

# Phase 5 Plan 02: Soft-Lock HTTP Slice (COLL-02) Summary

**Soft locks delivered end-to-end as a user-visible affordance: three editor-gated lock endpoints (acquire/force/release) dispatched off the `/pages/*` catch-all by `.md/lock` suffix with session-trusted identity, the `SoftLockBanner` warning component, and the `PageEditor` lock lifecycle — acquire-on-edit, ~30s heartbeat + on-save refresh, best-effort release, a genuinely read-only surface under another's lock, and a take-over-only Force edit — all without touching the save path.**

## Performance

- **Duration:** ~6 min
- **Completed:** 2026-06-22
- **Tasks:** 3
- **Files:** 11 (4 created, 7 modified)

## Accomplishments
- `internal/server/handlers_locks.go`: `handleAcquireLock`/`handleForceLock`/`handleReleaseLock` on the existing `authHandlers`. Identity is built from the SESSION (`lockOwner` → `actorUsername` + `auth.CurrentUser().UserID()`); the ONLY client-supplied field is the opaque `conn` id (the lock SessionID). Acquire surfaces only the holder's username on held-by-other; Force is lock-only; Release is idempotent (204).
- `handlers_pages.go`: `.md/lock/force` / `.md/lock/release` / `.md/lock` suffix dispatch at the top of `handleRenamePage` (longer suffixes first, `.md`-anchored). Rename/folder/restore routing unchanged.
- `router.go` / `handlers_auth.go` / `main.go`: `lockStore` wired through `Deps.Locks` → `authHandlers.locks`. Lock POSTs inherit the editor group's `RequireRole(RoleEditor)` + global nosurf CSRF with no new route line.
- `client.ts`: `acquireLock`/`forceLock`/`releaseLock` cloning the `mutate()` CSRF pattern; body carries only `{ conn }`.
- `SoftLockBanner.tsx` + `.css`: `role="status"` warning banner, verbatim UI-SPEC copy, semibold lead clause, Force edit button (`aria-label="Take over editing this page"`), busy ("Taking over…") + failed (retry copy) states; token-only CSS reusing `.banner`/`.banner-warning` + `.spinner`, reduced-motion-gated.
- `connId.ts`: stable per-tab opaque id (`crypto.randomUUID`, sessionStorage-persisted, in-memory fallback).
- `PageEditor.tsx` + `.css`: acquire-on-edit + ~30s heartbeat (`acquireLock` doubles as refresh) + on-save refresh in the `runSaver` success branch + best-effort `releaseLock` on unmount/path change; held-by-other ⇒ `SoftLockBanner` + genuinely read-only `LivePreviewEditor` (no caret) + read-only hint + disabled Save; Force edit calls `forceLock` alone.

## Task Commits

1. **Task 1: Editor-gated lock endpoints + suffix dispatch + main wiring** — `36b634c` (feat)
2. **Task 2: Lock client calls + SoftLockBanner component** — `2612229` (feat)
3. **Task 3: PageEditor lock lifecycle + read-only surface + Force edit** — `da9a052` (feat)

## Files Created/Modified
- `internal/server/handlers_locks.go` (created) — acquire/force/release handlers + `lockOwner`/`decodeConn` helpers (session-trusted identity, opaque conn).
- `internal/server/handlers_pages.go` (modified) — `.md/lock[...]` POST suffix dispatch.
- `internal/server/handlers_auth.go` (modified) — `locks *locks.Service` field on `authHandlers`.
- `internal/server/router.go` (modified) — `Deps.Locks` + handler wiring.
- `cmd/okf-workspace/main.go` (modified) — pass `lockStore` into `server.Deps`.
- `web/src/api/client.ts` (modified) — `acquireLock`/`forceLock`/`releaseLock`.
- `web/src/components/SoftLockBanner.tsx` + `.css` (created) — warning banner.
- `web/src/lib/connId.ts` (created) — `getConnId()`.
- `web/src/routes/PageEditor.tsx` + `.css` (modified) — lock lifecycle + read-only surface.

## Decisions Made
- **Suffix order + `.md` anchoring:** the lock branches sit at the top of `handleRenamePage`, checked `.md/lock/force` → `.md/lock/release` → `.md/lock` so a longer suffix never gets swallowed by the bare `/lock`, and the `.md` anchor prevents mis-routing a real page in a folder literally named `lock`.
- **Acquire = heartbeat:** no separate refresh endpoint this slice. The Slice-1 store already re-stamps the TTL on a same-session `Acquire`, so the ~30s interval, the initial acquire, and the on-save refresh all reuse `acquireLock`.
- **`conn` required:** a blank connection id is a 400 — without a SessionID a release cannot be attributed and could clobber a new holder; the lifecycle depends on a stable id.
- **Save path untouched:** verified by diff — `handlers_pages.go` has no `handleSavePage`/`pages.Save`/`base_revision` changes, and `onForceEdit` contains no `savePage`/`baseRevision`. Force edit is take-over, not a save bypass (T-05-09).

## Deviations from Plan

None — plan executed exactly as written. (The `dangerouslySetInnerHTML` reference in `SoftLockBanner.tsx` is an explanatory comment documenting that holderName is rendered as auto-escaped React text and that the sink is deliberately NOT used — T-05-10; a PostToolUse security hook flagged the substring, no actual sink exists.)

## Threat Mitigations Applied
- **T-05-06 (Spoofing/EoP):** lock Username/UserID from the session (`actorUsername` + `CurrentUser().UserID()`), only the opaque `conn` from the body. `grep -ci 'req.*username' handlers_locks.go` = 0.
- **T-05-07 (EoP):** acquire/force/release inherit `editor.Use(RequireRole(RoleEditor))` off the editor group (no separate route).
- **T-05-08 (CSRF):** lock POSTs inherit the global nosurf CSRF; the client uses the existing `mutate()` token path.
- **T-05-09 (Tampering / save bypass):** Force edit is lock-only on both ends; the save-time revision check (`pages.Save`) is provably untouched.
- **T-05-10 (XSS):** `holderName` renders as auto-escaped React text; no `dangerouslySetInnerHTML`.

## Known Stubs
None — every surface is wired to real data (session identity, the live lock store, the real read-only editor surface). The two-session live behavior (B sees A's lock; Force edit flips B editable) is a manual UAT item per VALIDATION.md, not a stub.

## User Setup Required
None — no new dependencies (lucide-react already present).

## Next Phase Readiness
- The opaque `conn` id (`getConnId`) and the held-by-other holder shape are ready for the Plan 03 presence/SSE stream (same connection id).
- The existing 409 conflict banner is intentionally KEPT — Plan 04 supersedes it with the conflict `DiffReviewDialog`.

---
*Phase: 05-collaboration*
*Completed: 2026-06-22*

## Self-Check: PASSED

- Files created/modified all present on disk (11/11).
- Task commits all present in git history (36b634c, 2612229, da9a052).
- `CGO_ENABLED=0 go build ./...`, `go vet ./...`, `go test ./...` all green (incl. internal/server, internal/locks).
- `cd web && npx vitest run` → 280/280 passed; `npx tsc -b` clean; `npx tsc --noEmit` clean.
- Save path prohibition verified by diff: no `pages.Save`/`base_revision`/`savePage` changes; `onForceEdit` calls `forceLock` alone.
- Plan greps: `HasSuffix(wild, ".md/lock'` = 3; `actorUsername` in handlers_locks.go = 2; client-supplied username = 0; SoftLockBanner copy = 1.
