---
phase: 05-collaboration
plan: 03
subsystem: collaboration
tags: [presence, sse, locks, coll-01, react, page-editor, awareness, event-source]

# Dependency graph
requires:
  - phase: 05-collaboration
    plan: 01
    provides: "internal/locks store + EditorsFor(ctx, page, conn) snapshot source (Editor{Username, You}); self-exclusion by connection id"
  - phase: 05-collaboration
    plan: 02
    provides: "getConnId() per-tab opaque connection id; authHandlers.locks wiring; PageEditor lock lifecycle + toolbar; .md-suffix dispatch pattern off the /pages/* catch-all"
  - phase: 02-storage-safety
    provides: "repo.Resolve SEC-01 path chokepoint (inherited via the lock store)"
provides:
  - "GET /api/v1/pages/{path}.md/presence — per-page editing-presence SSE stream (snapshot per ~2s tick from lockStore.EditorsFor; ctx.Done teardown + absolute max-duration cap)"
  - "handlers_presence.go: handlePresence cloning handleExtractionStatus's SSE skeleton; presenceSnapshot{editors, you_hold_lock}"
  - "client.ts subscribePresence(path, conn, onSnapshot, onState) + PresenceSnapshot/PresenceState types"
  - "PresenceIndicator component — quiet aria-live presence line (none/one/many/connecting/reconnecting/disconnected), never shows self, muted except warning-on-reconnect"
  - "EventSource test stub in src/test/setup.ts (jsdom lacks it)"
affects: [05-04 conflict dialog slice]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Presence derived from the lock store: a 'currently editing' user IS a live lock holder (EditorsFor); no separate presence registry"
    - "SSE presence handler clones handleExtractionStatus verbatim (Flusher assert, full header set, immediate emit, ticker loop, ctx.Done + max-duration cap) — coarser ~2s tick, no terminal state"
    - "subscribePresence onerror does NOT close (presence has no terminal state) — reports 'reconnecting', native EventSource auto-reconnects"
    - "PresenceIndicator is the awareness twin of AutosaveStatus: single muted aria-live span, empty when nobody else edits, never shows your own session"

key-files:
  created:
    - internal/server/handlers_presence.go
    - web/src/components/PresenceIndicator.tsx
    - web/src/components/PresenceIndicator.css
  modified:
    - internal/server/handlers_pages.go
    - web/src/api/client.ts
    - web/src/routes/PageEditor.tsx
    - web/src/test/setup.ts

key-decisions:
  - "Presence tick is ~2s (coarser than the 500ms extraction-status tick) and the cap is 30m — presence is ambient awareness over a minutes-scale lock TTL, not a live progress chip"
  - "Max-duration cap simply closes (no terminal event) — a genuinely active editor's EventSource reconnects; only a forgotten-tab goroutine is reaped (T-05-11)"
  - "YouHoldLock is derived in the handler from the same EditorsFor snapshot (any e.You) so one stream carries both presence + own-lock state (RESEARCH A2/A7)"
  - "EventSource stub added to the shared test setup (not per-test) — jsdom has no EventSource and PresenceIndicator opens one on mount, so PageEditor.test.tsx broke without it"

patterns-established:
  - "GET sub-resource via .md-anchored suffix on the existing authed /pages/* catch-all (mirrors .md/history, .md/version/) — no new route line, inherits any-authed read authority, no CSRF"
  - "Inert global polyfill in test setup for a browser API jsdom omits (EventSource), constructible + closable, never emits"

requirements-completed: [COLL-01]

# Metrics
duration: ~10min
completed: 2026-06-22
status: complete
---

# Phase 5 Plan 03: Per-Page Editing Presence (COLL-01) Summary

**Per-page editing presence delivered end-to-end: a GET `.md/presence` SSE stream that pushes a full editors snapshot every ~2s from the soft-lock store (`lockStore.EditorsFor`) with a `ctx.Done()` teardown + absolute max-duration cap so a forgotten tab can't leak a goroutine, consumed by a `subscribePresence` EventSource and rendered as the quiet, muted, aria-live `PresenceIndicator` in the editor toolbar — "{name} is editing", connecting/reconnecting states, never showing your own session — additive to and non-clobbering of Plan 02's lock lifecycle.**

## Performance

- **Duration:** ~10 min
- **Completed:** 2026-06-22
- **Tasks:** 3
- **Files:** 7 (3 created, 4 modified)

## Accomplishments
- `internal/server/handlers_presence.go`: `handlePresence` clones `handleExtractionStatus`'s skeleton verbatim — Flusher assert → 500, the full SSE header set (`text/event-stream` + `no-cache` + `keep-alive` + `X-Accel-Buffering: no`), `ctx := r.Context()`, immediate first emit, then a `select { <-ctx.Done() | <-deadline.C | <-ticker.C }` loop. Differences: a coarser `presenceTick` (2s), a `presenceMaxDuration` cap (30m), and `emit` reading `lockStore.EditorsFor(ctx, path, conn)` and marshalling `presenceSnapshot{Editors, YouHoldLock}`. The opaque `?conn=` is read from the query (self/dedup only, never a path component).
- `handlers_pages.go`: a `.md/presence` suffix case at the TOP of `handleGetPageOrHistory`'s switch (anchored on `.md`, beside the `.md/history` / `.md/version/` cases) → `cleanPathString(TrimSuffix(wild, "/presence"))` → `handlePresence`. No new route line — it rides the existing `authed.Get("/pages/*", …)`.
- `client.ts`: `subscribePresence(path, conn, onSnapshot, onState)` cloning `subscribeExtractionStatus` (GET EventSource, `JSON.parse` per frame, keep-last on parse error, `() => es.close()` unsubscribe). `onerror` reports `"reconnecting"` and does NOT close (presence has no terminal state — native auto-reconnect). Exported `PresenceSnapshot` (`{editors:{username,you}[], you_hold_lock}`) + `PresenceState` types.
- `PresenceIndicator.tsx` + `.css`: a single muted `aria-live="polite"` span with `aria-label="Who else is editing"`, deriving `others = editors.filter(e => !e.you)` so it NEVER shows yourself. Six UI-SPEC states with verbatim copy — none (empty span) / one (`Pencil` + "{name} is editing") / many (`Users` + "{name} and {N} others are editing") / connecting (`Loader2 .spinner` + "Connecting…") / reconnecting (`AlertTriangle` warning + "Reconnecting…") / disconnected (`WifiOff` + "Presence unavailable"). Names render as plain auto-escaped React text (T-05-13). Token-only CSS, muted except the warning reconnect state, reduced-motion-gated `.spinner`.
- `PageEditor.tsx`: `<PresenceIndicator path={path} conn={connId.current} />` mounted in `.pageeditor-toolbar` LEFT of the flex spacer (before `.pageeditor-mode`'s `margin-left:auto`), reusing the Plan-02 `getConnId()` id. Editor-only (PageEditor IS Edit mode). Plan-02's lock lifecycle (acquire/heartbeat/release, SoftLockBanner, read-only-under-lock, Force edit) is untouched — presence is purely additive.

## Task Commits

1. **Task 1: Presence SSE handler + `.md/presence` GET dispatch** — `affb238` (feat)
2. **Task 2: subscribePresence consumer + PresenceIndicator component** — `eb320cd` (feat)
3. **Task 3: Mount PresenceIndicator in the editor toolbar** — `146b428` (feat)

## Files Created/Modified
- `internal/server/handlers_presence.go` (created) — `handlePresence` SSE handler + `presenceSnapshot` type + `presenceTick`/`presenceMaxDuration` consts.
- `internal/server/handlers_pages.go` (modified) — `.md/presence` GET suffix dispatch.
- `web/src/api/client.ts` (modified) — `subscribePresence` + `PresenceSnapshot`/`PresenceState`.
- `web/src/components/PresenceIndicator.tsx` + `.css` (created) — the quiet presence line.
- `web/src/routes/PageEditor.tsx` (modified) — toolbar mount.
- `web/src/test/setup.ts` (modified) — inert `EventSource` stub for jsdom.

## Decisions Made
- **Coarser tick, generous cap:** presence reads a minutes-scale lock TTL, not a sub-second progress chip, so a 2s tick keeps "Alice is editing" current while staying cheap at 5 users; the 30m cap reaps a forgotten tab without cutting off a real editor (its EventSource reconnects).
- **`onerror` does not close:** unlike the extraction stream (which closes on its terminal status), presence has no terminal state. `subscribePresence` reports `"reconnecting"` and lets the native EventSource auto-reconnect — the connection IS the heartbeat.
- **One snapshot stream carries both presence + own-lock state:** `YouHoldLock` is derived in the handler from the same `EditorsFor` result (`any e.You`), so a consumer never needs a second request to reconcile presence with its own lock (RESEARCH A2/A7).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Added an inert EventSource stub to the shared test setup**
- **Found during:** Task 3 (frontend self-check)
- **Issue:** jsdom does not implement `EventSource`. Mounting `PresenceIndicator` in the `PageEditor` toolbar made every `PageEditor.test.tsx` render call `subscribePresence` → `new EventSource(...)` on mount, throwing `ReferenceError: EventSource is not defined` and failing 4 tests. This is a blocking test-infra gap directly caused by this slice's toolbar mount, not a product bug.
- **Fix:** added a minimal, inert `MockEventSource` (constructible + `close()`able, never emits) to `src/test/setup.ts` guarded by `typeof globalThis.EventSource === "undefined"`. Tests that assert SSE behaviour drive the component through the mocked `api/client` seam, so the stub only needs to exist — it does not emit.
- **Files modified:** web/src/test/setup.ts
- **Verification:** `npx vitest run` → 280/280 pass; `npx tsc -b` clean.
- **Committed in:** `146b428` (Task 3 commit)

---

**Total deviations:** 1 auto-fixed (1 blocking test-infra global). No scope creep, no new dependencies.

## Threat Mitigations Applied
- **T-05-11 (DoS / SSE goroutine exhaustion):** `handlePresence` mirrors `handleExtractionStatus` — `<-ctx.Done()` tears the goroutine down on client disconnect AND a `presenceMaxDuration` (30m) cap closes a forgotten tab. Grep-verified: `ctx.Done` = 1, `X-Accel-Buffering` = 1 in handlers_presence.go.
- **T-05-12 (Information disclosure):** `presenceSnapshot` surfaces only `editors:[{username, you}]` + `you_hold_lock` — never session ids, user ids, or another user's connection id. `EditorsFor` already projects `Lock` → `Editor{Username, You}` only.
- **T-05-13 (XSS):** usernames render as plain React text (auto-escaped) in `PresenceIndicator`; no `dangerouslySetInnerHTML`.
- **T-05-14 (Spoofing, accepted):** `conn` is opaque, query-only, used solely to mark `you:true` / derive `YouHoldLock` — never a path component, grants no privilege.

## Known Stubs
None — presence is wired to the real lock store (`EditorsFor`), the real SSE stream, and the real toolbar mount. Live two-session cross-presence ("B sees A editing within a tick; closing A clears it after TTL; killing the stream shows Reconnecting…") is a manual UAT item per VALIDATION.md, not a stub.

## User Setup Required
None — no new dependencies (lucide-react already present; `Pencil`/`Users`/`AlertTriangle`/`Loader2`/`WifiOff` are existing icons).

## Next Phase Readiness
- COLL-01 is complete; Plan 04 (conflict `DiffReviewDialog`) is the last Phase 5 slice. The presence stream's `you_hold_lock` field is available if Plan 04 wants own-lock reconciliation, though Plan 04 is conflict-on-save, a separate surface.

---
*Phase: 05-collaboration*
*Completed: 2026-06-22*

## Self-Check: PASSED

- Files created/modified all present on disk (7/7): handlers_presence.go, handlers_pages.go, client.ts, PresenceIndicator.tsx, PresenceIndicator.css, PageEditor.tsx, test/setup.ts.
- Task commits all present in git history (affb238, eb320cd, 146b428).
- **Backend:** `CGO_ENABLED=0 go build ./...` OK; `go vet ./...` OK; `go test ./...` all green (incl. internal/server, internal/locks).
- **Frontend:** `npx vitest run` → 280/280 passed; `npx tsc -b` clean; `npx tsc --noEmit` clean.
- **Plan greps:** `HasSuffix(wild, ".md/presence'` = 1; `X-Accel-Buffering` in handlers_presence.go = 1; `ctx.Done` = 1; `EditorsFor` in handlers_presence.go = 2; `is editing` in PresenceIndicator.tsx ≥ 1; `Reconnecting` ≥ 1; `subscribePresence` in client.ts = 2; `PresenceIndicator` in PageEditor.tsx = 2.
- **05-02 integrity:** PageEditor's lock lifecycle (acquireLock/forceLock/releaseLock, heartbeat, SoftLockBanner, read-only surface) is untouched — the presence change is a single additive import + toolbar mount.
