---
phase: 05-collaboration
verified: 2026-06-22T12:56:37Z
status: human_needed
score: 4/4 must-haves verified
behavior_unverified: 3
overrides_applied: 0
human_verification:
  - test: "Open the same page in two browser sessions and enter Edit mode. Confirm each session sees '{name} is editing' in the toolbar within ~2 seconds and that it clears after the other session leaves or the lock expires."
    expected: "PresenceIndicator shows the other user's name, never your own. Dropping/closing the stream shows 'Reconnecting…' then recovers."
    why_human: "Live SSE cross-session behavior requires two concurrent browser sessions; automated checks confirm the subscriber + dispatch are wired but cannot exercise actual EventSource reconnect or two-tab self-exclusion at runtime."
  - test: "User A opens a page in Edit mode (acquires lock). User B opens the same page. Confirm B sees the SoftLockBanner ('Alice is editing this page.') and the editor is genuinely read-only (no caret, Save disabled). B clicks Force edit. Confirm the banner clears, the caret enters, and Save re-enables."
    expected: "Banner role=status with A's username, read-only surface while A holds, Force edit flips to editable. The banner never implies the next save is safe."
    why_human: "Requires two concurrent browser sessions and visual inspection of read-only surface; automated tests confirm components exist and are wired but cannot exercise the live lock-state flip across sessions."
  - test: "User A and B both open a page in Edit, make different edits. A saves first. B tries to save (now stale). Confirm B sees DiffReviewDialog in conflict mode with a real diff. Try each branch: Overwrite (replaces A's changes), Manual merge (keeps B's body + shows A's for reference), Save as copy (creates a new page, original untouched)."
    expected: "No silent overwrite on any path. Save as copy navigates to the new page. Overwrite that races another concurrent save 409s again and re-opens the dialog."
    why_human: "Requires two concurrent browser sessions with a real save race; the automated tests mock the 409 response (they confirm the dialog opens, autosave is gated, and the three handlers call the right API endpoints — but the human must confirm the real UX end-to-end)."
behavior_unverified_items:
  - truth: "A user can see when another user is currently editing a page (presence indicator)"
    test: "Open the same page in two browser sessions and enter Edit mode on both."
    expected: "Each session's PresenceIndicator shows the other's username within ~2 ticks; it shows nothing for the current session's own lock; the indicator clears after the other session leaves."
    why_human: "State transition 'enter edit → see presence → leave → presence clears' is a runtime SSE cross-session invariant that no automated test exercises. subscribePresence + handlePresence + EditorsFor are all wired, but the live two-session self-exclusion and clearing-on-exit cannot be asserted by grep or unit tests."
  - truth: "The system applies a soft lock while a page is being edited, and a user can still choose to force-edit"
    test: "User A edits (lock acquired). User B opens the same page and confirms read-only + SoftLockBanner. B clicks Force edit. Confirm B's editor becomes editable and A is now unprotected."
    expected: "The SoftLockBanner shows A's name; the editor surface is genuinely read-only (no caret). After force-edit, B can type and Save. A's subsequent save will 409 because their base revision is now stale."
    why_human: "The live lock-state-flip (held-by-other → acquired via force, surface-read-only → editable) is a runtime state transition across two sessions that unit/component tests mock but do not exercise."
  - truth: "On a save conflict, the user is shown a diff and can choose overwrite, manual merge, or save-as-copy (which creates a new page)"
    test: "Race two sessions to save conflicting edits; inspect the conflict dialog and exercise all three branches."
    expected: "Real diff shown; initial focus on Save as copy (not Overwrite); all three choices route through the revision-checked save path with no silent data loss."
    why_human: "The component test (PageEditor.conflict.test.tsx) mocks the 409 and confirms dialog opens + autosave gating + API call routing — but the real end-to-end race condition with actual network 409 and navigation requires two browser sessions and human judgment."
---

# Phase 5: Collaboration Verification Report

**Phase Goal:** A small team can edit concurrently without silently overwriting each other — seeing who is editing, getting soft-lock warnings, and resolving conflicts through a clear diff with safe choices.
**Verified:** 2026-06-22T12:56:37Z
**Status:** human_needed
**Re-verification:** No — initial verification

---

## Goal Achievement

### Observable Truths

All four roadmap success criteria are verified at the code level. Three of the four involve runtime state transitions (cross-session SSE presence, lock-state-flip, live save race) that are present and wired in code with component/unit tests covering the mocked path, but require human UAT to prove the full invariant holds across two live browser sessions.

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | A user can see when another user is currently editing a page (presence indicator) | PRESENT_BEHAVIOR_UNVERIFIED | `internal/server/handlers_presence.go`: `handlePresence` streams `EditorsFor` snapshot per tick with `ctx.Done()` teardown + 30-min cap. `internal/locks/service.go`: `EditorsFor` correctly sets `You: SessionID == connID`. `web/src/components/PresenceIndicator.tsx`: all 6 UI-SPEC states implemented; filters `!e.you`. `web/src/api/client.ts`: `subscribePresence` EventSource GET. Mounted in `PageEditor.tsx:546`. Cross-session live behavior is a runtime state transition — see Human Verification #1. |
| 2 | The system applies a soft lock while a page is being edited, and a user can still choose to force-edit | PRESENT_BEHAVIOR_UNVERIFIED | `internal/server/handlers_locks.go`: `handleAcquireLock`, `handleForceLock`, `handleReleaseLock` all present with identity from session (`actorUsername`), not request body. `internal/server/handlers_pages.go:250-271`: suffix dispatch `.md/lock/force` → `.md/lock/release` → `.md/lock` in correct longest-first order, inheriting editor gate + CSRF. `web/src/components/SoftLockBanner.tsx`: `role="status"` warning banner with Force edit button; verbatim UI-SPEC copy. `PageEditor.tsx:94`: `readOnly = lockedBy !== null`; `readOnly={readOnly}` on editor surface; `disabled={readOnly}` on Save. Live two-session lock-state-flip is a runtime state transition — see Human Verification #2. |
| 3 | Saves use optimistic concurrency with a per-document revision, and a stale save is rejected rather than silently overwriting | VERIFIED | `internal/pages/service.go:200`: `if current != baseRevision { return ErrStaleRevision }` — the 409 floor, untouched this phase. `internal/pages/forceedit_test.go`: `TestForceEditStillRejectsStaleSave` PASSES (`go test ./internal/pages/ -run TestForceEditStillRejectsStaleSave -count=1`): force-lock + landed commit + stale save = `ErrStaleRevision`; stale body never lands; fresh-revision save succeeds. Force is lock-only — confirmed by test asserting `Revision(page) == rev0` after `locks.Force`. |
| 4 | On a save conflict, the user is shown a diff and can choose overwrite, manual merge, or save-as-copy (which creates a new page) | PRESENT_BEHAVIOR_UNVERIFIED | `internal/pages/saveascopy_test.go`: `TestSaveAsCopyLeavesOriginal` PASSES: copy is a fresh deduped path; original body/revision byte-identical. `web/src/components/DiffReviewDialog.tsx`: `mode="conflict"` with 3-button risk-ranked footer; `safeFocusRef` on Save-as-copy (never Overwrite); identical-versions guard. `DiffReviewDialog.test.tsx`: 9 tests PASS — 3 buttons present, focus NOT on Overwrite, real diff rendered, Esc cancels. `PageEditor.conflict.test.tsx`: 4 tests PASS — 409 opens dialog, autosave gated, Save-as-copy calls `createPage` then `savePage(newPath)` never `originalPath`, Overwrite calls `getPage` then `savePage(originalPath, freshRev)`. Real-race end-to-end requires human — see Human Verification #3. |

**Score:** 4/4 truths verified (1 fully proven by passing test, 3 present + wired with component/unit test coverage; runtime cross-session transitions unverified)

---

## Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/locks/lock.go` | Lock model + Owner + Editor types (CONTEXT-locked JSON shape) | VERIFIED | `type Lock struct` with all JSON-tagged fields; `Owner` and `Editor` types present; no raw os.* access |
| `internal/locks/service.go` | `NewService` + Acquire/Refresh/Force/Release/Get/List/EditorsFor; injected clock; repo-only I/O | VERIFIED | All 7 methods present; `s.repo.Read/Write/Remove` for all I/O; `lockPath` subtree guard (`ErrUnsafePagePath`); `now: time.Now` default |
| `internal/locks/gc.go` | `KindGC` const + `GCHandler(*Service)` + `Service.GC` | VERIFIED | `const KindGC = "lock_gc"`, `GCHandler`, `Service.GC` all present; walks via `s.walk()` → `repo.Remove` |
| `internal/locks/service_test.go` | 7 deterministic tests: acquire lifecycle, refresh, force-lock-only, expiry+GC, path safety, torn lock, EditorsFor snapshot | VERIFIED | All 7 tests pass: `TestAcquireLifecycle`, `TestRefreshHolderOnly`, `TestForceTakesOwnershipLockOnly`, `TestExpiryAndGC`, `TestLockPathSafety`, `TestTornLockTreatedAsNoLock`, `TestEditorsForSnapshot` |
| `internal/auth/session.go` | `SessionConnectionIDKey` const added | VERIFIED | `const SessionConnectionIDKey = "connection_id"` at line 19 |
| `cmd/okf-workspace/main.go` | `locks.NewService` construction + `locks.KindGC` worker registration + ctx-gated GC ticker | VERIFIED | `lockStore := locks.NewService(contentRepo, lockExpiry)` line 241; `worker.Register(locks.KindGC, locks.GCHandler(lockStore))` line 242; ctx-gated ticker goroutine present; `Locks: lockStore` in deps at line 354 |
| `internal/server/handlers_locks.go` | Editor-gated acquire/force/release handlers; identity from session | VERIFIED | `handleAcquireLock`, `handleForceLock`, `handleReleaseLock` present; `lockOwner()` builds Owner from `actorUsername` + session UserID; body supplies only `conn` field |
| `web/src/components/SoftLockBanner.tsx` | Warning banner (role=status) with holder name + Force edit button; UI-SPEC states | VERIFIED | `role="status"`, `aria-live="polite"`, Lock/Loader2/AlertTriangle icons; verbatim copy "{name} is editing this page." + "Your changes won't be saved until you take over."; busy/failed states |
| `web/src/routes/PageEditor.tsx` | Lock lifecycle: acquire on edit / heartbeat / release on unmount; read-only-under-lock; Force edit wiring | VERIFIED | `acquireLock` on edit mount; ~30s heartbeat; `releaseLock` on unmount cleanup; `readOnly = lockedBy !== null`; `readOnly={readOnly}` on editor; `forceLock` in `onForceEdit`; `SoftLockBanner`, `PresenceIndicator`, `DiffReviewDialog` all mounted |
| `internal/server/handlers_presence.go` | SSE presence stream from `EditorsFor`; ctx.Done + max-duration cap | VERIFIED | `handlePresence` present; `presenceTick = 2s`, `presenceMaxDuration = 30min`; ctx.Done() + deadline.C in select loop; `X-Accel-Buffering: no`; `h.locks.EditorsFor` as snapshot source |
| `web/src/components/PresenceIndicator.tsx` | 6 UI-SPEC states; filters self; aria-live | VERIFIED | All 6 states (none/one/many/connecting/reconnecting/disconnected); `filter(e => !e.you)`; `aria-live="polite"` |
| `internal/pages/forceedit_test.go` | `TestForceEditStillRejectsStaleSave` — COLL-03 load-bearing rule | VERIFIED | Test exists and PASSES: `go test ./internal/pages/ -run TestForceEditStillRejectsStaleSave` → `PASS (0.13s)` |
| `internal/pages/saveascopy_test.go` | `TestSaveAsCopyLeavesOriginal` — copy is fresh deduped page; original untouched | VERIFIED | Test exists and PASSES: `go test ./internal/pages/ -run TestSaveAsCopyLeavesOriginal` → `PASS (0.17s)` |
| `web/src/components/DiffReviewDialog.tsx` | conflict mode with 3-button footer; safe initial focus (never Overwrite) | VERIFIED | `mode?: "review" | "conflict"` prop; `safeFocusRef` on Save-as-copy; Overwrite in isolated `diff-conflict-overwrite` div; risk sub-line present; identical-versions guard |
| `web/src/routes/PageEditor.conflict.test.tsx` | 409→dialog; autosave gated; Save-as-copy route; Overwrite route | VERIFIED | 4 tests PASS: dialog opens on 409, autosave debounce does not re-fire while open, Save-as-copy calls `createPage`+`savePage(newPath)` never `originalPath`, Overwrite calls `getPage`+`savePage(originalPath, freshRev)` |

---

## Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| `internal/locks/service.go` | `internal/repo/files.go` | `s.repo.Read/Write/Remove` — no raw `os.*` on lock paths | WIRED | Grep: no `os.WriteFile/ReadFile/Remove` in `internal/locks/*.go`; every I/O call uses `s.repo.*` |
| `cmd/okf-workspace/main.go` | `internal/locks/gc.go` | `worker.Register(locks.KindGC, locks.GCHandler(lockStore))` before `worker.Start` | WIRED | Line 242: `worker.Register(locks.KindGC, locks.GCHandler(lockStore))` |
| `web/src/routes/PageEditor.tsx` | `internal/server/handlers_locks.go` | `acquireLock/forceLock/releaseLock` → `POST /api/v1/pages/{path}/lock[/force|/release]` | WIRED | `acquireLock`, `forceLock`, `releaseLock` imported from `../api/client` and called in PageEditor; client.ts POSTs to the correct endpoints |
| `internal/server/handlers_locks.go` | `internal/locks/service.go` | `h.locks.Acquire/Force/Release` with Owner from session | WIRED | `h.locks.Acquire`, `h.locks.Force`, `h.locks.Release` in handlers_locks.go; `Locks` field set to `lockStore` in deps |
| `internal/server/handlers_pages.go` | `internal/server/handlers_locks.go` | suffix dispatch `.md/lock[/force|/release]` off the POST /pages/* catch-all | WIRED | Lines 250-271: `HasSuffix(wild, ".md/lock/force")` → `handleForceLock`; `".md/lock/release"` → `handleReleaseLock`; `".md/lock"` → `handleAcquireLock` (longest-suffix-first order) |
| `internal/server/handlers_pages.go` | `internal/server/handlers_presence.go` | GET suffix dispatch `.md/presence` off the /pages/* catch-all | WIRED | Line 115: `HasSuffix(wild, ".md/presence")` → `handlePresence` |
| `internal/server/handlers_presence.go` | `internal/locks/service.go` | `h.locks.EditorsFor(ctx, path, conn)` | WIRED | Line 86: `eds, err := h.locks.EditorsFor(ctx, path, conn)` |
| `web/src/components/PresenceIndicator.tsx` | `internal/server/handlers_presence.go` | `subscribePresence` EventSource GET `/api/v1/pages/{path}/presence?conn=…` | WIRED | `subscribePresence` imported and called on mount; EventSource URL contains `/presence?conn=` |
| `web/src/routes/PageEditor.tsx` | `web/src/components/DiffReviewDialog.tsx` | conflict state opens `DiffReviewDialog mode='conflict'` with `oldText=serverBody, newText=mine` | WIRED | Line 478-490: `<DiffReviewDialog open={conflict !== null} mode="conflict" oldText={conflict?.serverBody ?? ""} ...>` |

---

## Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| COLL-03: stale save 409s after force-edit | `go test ./internal/pages/ -run TestForceEditStillRejectsStaleSave -count=1` | PASS (0.13s) | PASS |
| COLL-04: save-as-copy never mutates original | `go test ./internal/pages/ -run TestSaveAsCopyLeavesOriginal -count=1` | PASS (0.17s) | PASS |
| Lock store 7-test suite | `go test ./internal/locks/ -count=1` | PASS — 7 tests (0.004s) | PASS |
| DiffReviewDialog conflict mode (9 tests) | `cd web && npx vitest run src/components/DiffReviewDialog.test.tsx` | 9 passed (187ms) — note: 1 `act()` warning (non-blocking, cosmetic) | PASS |
| PageEditor conflict flow (4 tests) | `cd web && npx vitest run src/routes/PageEditor.conflict.test.tsx` | 4 passed (242ms) | PASS |
| Full Go build + vet | `go build ./... && go vet ./internal/locks/... ./internal/server/... ./cmd/okf-workspace/...` | Clean — no errors | PASS |
| TypeScript type check | `cd web && npx tsc --noEmit` | Clean — no errors | PASS |

---

## Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|------------|-------------|--------|----------|
| COLL-01 | 05-03-PLAN.md | Per-page editing presence: PresenceIndicator + SSE presence stream | SATISFIED | `handlePresence` + `EditorsFor` + `subscribePresence` + `PresenceIndicator` + mounted in PageEditor toolbar |
| COLL-02 | 05-01-PLAN.md, 05-02-PLAN.md | Soft locks: SoftLockBanner, acquire/force/release endpoints, WITHOUT modifying the save path | SATISFIED | Lock store + 3 handler endpoints (editor-gated, CSRF-inherited) + SoftLockBanner + read-only surface + heartbeat + release-on-unmount; `pages/service.go` unchanged |
| COLL-03 | 05-04-PLAN.md | LOAD-BEARING: force-edit never bypasses optimistic concurrency; stale save still 409s after force-edit | SATISFIED | `TestForceEditStillRejectsStaleSave` PASSES; `pages/service.go:200` check untouched; `git diff HEAD~5 -- internal/pages/service.go` empty (no changes) |
| COLL-04 | 05-04-PLAN.md | Conflict resolution: overwrite / manual-merge / save-as-copy via DiffReviewDialog with safe defaults | SATISFIED | `DiffReviewDialog` conflict mode + 3 handlers in PageEditor + `TestSaveAsCopyLeavesOriginal` PASSES + component tests green |

All 4 COLL requirements are accounted for, sourced from plan frontmatter, and match the REQUIREMENTS.md status (all marked Complete).

---

## Anti-Patterns Found

No blockers found.

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| `web/src/components/DiffReviewDialog.tsx` | 150 | `return null` | Info | Standard React guard for `!open` — not a stub; component renders substantively when `open === true` |
| `web/src/components/DiffReviewDialog.test.tsx` | (Esc test) | `act()` warning in test output | Info | A React testing-library act() coalescing warning during the Esc keyboard event test. Tests all pass; this is a test harness cosmetic issue, not a production defect |

No `TBD`, `FIXME`, or `XXX` markers found in any phase-modified file.
No raw `os.WriteFile/ReadFile/Remove` calls in `internal/locks/*.go` (all I/O via `repo.*`).
No Git vocabulary (`merge conflict`, `HEAD`, `SHA`, `branch`, `commit`) in user-facing DiffReviewDialog copy.
Identity in lock handlers comes from `actorUsername` + session UserID — no client-supplied username field.

---

## Human Verification Required

Three items require two concurrent browser sessions. The automated substrate (endpoints, SSE handler, component wiring, lock service, tests) is fully present and verified. These items test the runtime cross-session invariants.

### 1. Live cross-session presence indicator (COLL-01)

**Test:** Open the same page in two browser sessions (e.g. two tabs in different browser profiles). Both enter Edit mode. Confirm each session's toolbar PresenceIndicator shows `{other username} is editing` within ~2 seconds and does NOT show the current user's own name. Then have one session leave Edit mode or close the tab. Confirm the other session's indicator clears within a few ticks (TTL reap). Kill the server-side SSE stream (or use browser dev tools to block the EventSource): confirm the indicator briefly shows `Reconnecting…` then recovers to showing presence again once the stream comes back.

**Expected:** PresenceIndicator accurately reflects who else is editing in near-real-time; never shows your own session; recovers from SSE disconnect.

**Why human:** Two concurrent browser sessions with live SSE and EventSource reconnect behavior are required. Unit tests and grep confirm all wiring, but the runtime self-exclusion invariant (SessionID == connID filtering), the tick-driven update cadence, and the EventSource reconnect state cannot be proven without a live two-session scenario.

---

### 2. Soft lock warning + force-edit (COLL-02)

**Test:** User A opens a page in Edit mode (acquires the lock). User B opens the same page. Confirm B's editor shows the SoftLockBanner (`{A's name} is editing this page. Your changes won't be saved until you take over.`) and the editor surface is genuinely read-only: cursor cannot enter the editor body, Save button is disabled. B clicks `Force edit`. Confirm the banner disappears, the wash lifts, the caret enters the editor, and Save re-enables. Confirm the banner never implies the subsequent save is safe (no "you're safe to save" copy).

**Expected:** Lock-state-flip is visually immediate; read-only is genuine (no caret) not merely visual; Force edit is the only escape hatch from the warning state.

**Why human:** The live lock-state-flip across two sessions (held-by-other → acquired via force) is a runtime state transition. Component tests confirm the SoftLockBanner renders and `readOnly={readOnly}` is wired, but the live two-session scenario and visual inspection of "no caret" are required for full confidence.

---

### 3. Conflict resolution three-way (COLL-04)

**Test:** Two sessions open the same page in Edit mode. Both make different edits. Session A saves first (succeeds). Session B tries to save: confirm a real DiffReviewDialog opens in conflict mode with a real diff (left = A's saved version, right = B's unsaved version). Initial focus must be on `Save as copy` (not `Overwrite`). Exercise each branch: (a) `Overwrite` — A's changes are replaced by B's; no silent clobber (if another save lands first, the dialog re-opens). (b) `Manual merge` — dialog closes, B's editor body is kept, A's version is visible for reference. (c) `Save as copy` — a new page is created at `{title} (Copy)`, the original page is byte-identical to A's saved version, and the browser navigates to the new page. Confirm Esc/backdrop closes the dialog without applying anything.

**Expected:** No silent data loss on any path. Initial focus on a safe option. Save as copy creates a new page and leaves the original untouched.

**Why human:** A real concurrent-edit race with an actual network 409 is required. Component tests mock the 409 and confirm all three handler API call sequences and autosave gating, but the real UX (diff quality, navigation timing, original-page integrity visible in the tree) requires a live two-session scenario.

---

## Gaps Summary

No automated gaps. All four COLL requirements have evidence in the codebase:

- COLL-01: SSE handler, `EditorsFor`, `subscribePresence`, `PresenceIndicator`, and toolbar mount all exist and are wired.
- COLL-02: Lock store, 3 HTTP endpoints (editor-gated, CSRF-inherited), `SoftLockBanner`, read-only surface, heartbeat, and release-on-unmount all exist and are wired. `pages/service.go` is unchanged.
- COLL-03: `TestForceEditStillRejectsStaleSave` PASSES. `pages/service.go:200` is the untouched 409 floor. Force is provably lock-only.
- COLL-04: `TestSaveAsCopyLeavesOriginal` PASSES. `DiffReviewDialog` conflict mode with 3-button safe-focus footer exists. All three PageEditor conflict handlers are wired. `PageEditor.conflict.test.tsx` covers the mocked 409 path.

The `human_needed` status reflects three behavior-dependent truths whose runtime state transitions (live cross-session SSE presence, lock-state-flip, real save race + conflict dialog) require human UAT. All automated checks pass.

---

_Verified: 2026-06-22T12:56:37Z_
_Verifier: Claude (gsd-verifier)_
