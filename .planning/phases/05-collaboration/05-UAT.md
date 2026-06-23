---
status: testing
phase: 05-collaboration
source: [05-VERIFICATION.md]
started: 2026-06-22T16:05:00Z
updated: 2026-06-22T16:05:00Z
---

## Current Test

number: 1
name: Live cross-session presence indicator (COLL-01)
expected: |
  Each session's toolbar PresenceIndicator shows "{other user} is editing" within
  ~2s and never the current user's own name; it clears after the other session
  leaves; an EventSource disconnect briefly shows "Reconnecting…" then recovers.
awaiting: user response

## Tests

### 1. Live cross-session presence indicator (COLL-01)
expected: |
  Open the same page in two browser sessions, both in Edit. Each PresenceIndicator
  shows the OTHER user's name within ~2 ticks, never its own. It clears within a few
  ticks after the other session leaves/closes. Blocking the EventSource shows
  "Reconnecting…" then recovers once the stream returns.
result: [pending]

### 2. Soft lock warning + force-edit (COLL-02)
expected: |
  User A edits (acquires lock). User B opens the same page and sees the SoftLockBanner
  ("{A} is editing this page…") with a genuinely read-only editor — no caret, Save
  disabled. B clicks Force edit → banner clears, caret enters, Save re-enables. The
  banner never implies the next save is safe.
result: [pending]

### 3. Conflict resolution three-way (COLL-04)
expected: |
  A and B both edit; A saves first; B's save is now stale and opens DiffReviewDialog
  in conflict mode with a REAL diff, initial focus on "Save as copy" (not Overwrite).
  Exercise all three branches — Overwrite (replaces, re-409s if it races another
  save), Manual merge (keeps B's body + shows A's for reference), Save as copy
  (creates a new page and navigates there; original untouched). No silent data loss
  on any path.
result: [pending]

## Summary

total: 3
passed: 0
issues: 0
pending: 3
skipped: 0
blocked: 0

## Gaps
