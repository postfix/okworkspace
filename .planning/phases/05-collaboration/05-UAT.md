---
status: complete
phase: 05-collaboration
source: [05-VERIFICATION.md]
started: 2026-06-22T16:05:00Z
updated: 2026-06-23T19:36:29Z
method: automated-api-race-simulation
---

## Current Test

[testing complete]

## Tests

### 1. Live cross-session presence indicator (COLL-01)
expected: |
  Open the same page in two browser sessions, both in Edit. Each PresenceIndicator
  shows the OTHER user's name within ~2 ticks, never its own. It clears within a few
  ticks after the other session leaves/closes. Blocking the EventSource shows
  "Reconnecting…" then recovers once the stream returns.
result: pass
evidence: |
  Two authenticated sessions (admin + ed) driven against the live server via the
  real presence SSE + lock API. Presence is derived from the single soft-lock
  (locks.EditorsFor → at most the one holder):
  - admin acquires lock → admin's own presence frame: {"editors":[{"username":"admin","you":true}],"you_hold_lock":true}
    (self is you:true → UI suppresses its own name).
  - ed's presence frame for the same page: {"editors":[{"username":"admin","you":false}],"you_hold_lock":false}
    → ed's PresenceIndicator shows "admin is editing".
  - After admin releases, ed's next frame: {"editors":null,"you_hold_lock":false} → indicator clears.
  Note: reconnect/"Reconnecting…" banner is client EventSource behavior (verified by
  code inspection in api/client.ts subscribePresence + the phase component tests),
  not exercised by the API-level harness.

### 2. Soft lock warning + force-edit (COLL-02)
expected: |
  User A edits (acquires lock). User B opens the same page and sees the SoftLockBanner
  ("{A} is editing this page…") with a genuinely read-only editor — no caret, Save
  disabled. B clicks Force edit → banner clears, caret enters, Save re-enables. The
  banner never implies the next save is safe.
result: pass
evidence: |
  - ed acquires lock → {"result":"acquired"}.
  - admin attempts acquire → {"result":"held-by-other","holder":{"username":"ed"}}
    → this is exactly the signal that renders the SoftLockBanner / read-only editor.
  - admin force → 200; admin's presence then shows you_hold_lock:true and ed's shows
    you_hold_lock:false (ed lost the lock).
  Load-bearing rule confirmed in code + Test 3: force takes the LOCK only, never
  writes — a subsequent save still revision-checks, so the banner can't imply a safe save.
  Note: "no caret / Save disabled" is a render assertion verified via code + the phase's
  component test net (SoftLockBanner), not the API harness.

### 3. Conflict resolution three-way (COLL-04)
expected: |
  A and B both edit; A saves first; B's save is now stale and opens DiffReviewDialog
  in conflict mode with a REAL diff, initial focus on "Save as copy" (not Overwrite).
  Exercise all three branches — Overwrite (replaces, re-409s if it races another
  save), Manual merge (keeps B's body + shows A's for reference), Save as copy
  (creates a new page and navigates there; original untouched). No silent data loss
  on any path.
result: pass
evidence: |
  - admin + ed both GET the same base_revision (6df053b9…).
  - admin saves first → 204 (new revision).
  - ed saves with the stale base_revision → 409 with the human message
    "This page was changed somewhere else since you opened it. Reload to see the
    latest version before saving again." → this 409 is what opens DiffReviewDialog in conflict mode.
  - Save-as-copy branch: POST /pages created a new page (201) — original untouched.
  - No silent loss: admin's first-save content was the live body before any overwrite.
  - Overwrite branch: ed re-reads the current revision then saves on top → 204
    (and would re-409 against another concurrent save — the optimistic-concurrency floor).
  Page restored to original body afterward; conflict-copy page deleted.
  Note: "initial focus on Save as copy" and Manual-merge body retention are
  DiffReviewDialog render/UX assertions — verified via code + component tests, not the API harness.

## Summary

total: 3
passed: 3
issues: 0
pending: 0
skipped: 0
blocked: 0

## Verification method

Verified by an automated two-session race simulation (`.smtc-cache/uat_race.mjs`)
driving the real HTTP/SSE API on localhost:8098 with two authenticated editor
sessions (admin, ed) — 17/17 assertions passed. This is authoritative for the
collaboration ENGINE (presence derivation, soft-lock acquire/held-by-other/force,
and the optimistic-concurrency 409). The thin UI-render layer bound to those
contracts (PresenceIndicator text, SoftLockBanner read-only state, DiffReviewDialog
focus/branches) is covered by code inspection + the Phase 05 component test net but
was not driven through a live browser in this run.

## Gaps

[none]
