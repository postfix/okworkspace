---
status: testing
phase: 01-okf-pages-navigation-hidden-git
source: [01-VERIFICATION.md]
started: 2026-06-19T00:30:00Z
updated: 2026-06-19T00:30:00Z
---

## Current Test

number: 1
name: Cross-folder and same-directory in-app link navigation (PAGE-08 / D-06)
expected: |
  Both a cross-folder relative link (e.g. a page in `docs/` linking to `runbooks/deploy.md`)
  and a same-directory link navigate to the correct in-app page in Read mode — no broken
  `/app/page/../...` route and no jump to the wrong/root page.
awaiting: user response

## Tests

### 1. Cross-folder and same-directory in-app link navigation (PAGE-08 / D-06)
expected: In Read mode, clicking a cross-folder relative link (page in `docs/` → `runbooks/deploy.md`) and a same-directory link both navigate to the correct page. (WR-02/WR-06 was fixed in code, commit `0c8421e`, and unit-tested in `web/src/lib/mdlink.test.ts`; this is a light in-browser regression confirmation.)
result: [pending]

### 2. Autosave does not self-conflict in a single-user session (WR-03)
expected: Open a page, type to trigger autosave, then stop and wait ~7–8s (spanning the 1s draft save and the 6s idle commit). Status shows Saving… → Draft saved → Saving… → Saved. NO "This page was changed somewhere else" 409 banner appears in a single-user session (try once on a throttled/slow network in DevTools).
result: [pending]

### 3. Holistic non-technical-user round trip (phase goal)
expected: |
  A user with no Git knowledge can complete the full loop entirely from the UI, with Git
  never visible:
  - Create a page by typing only a title; it appears in the left tree (no filename/path shown).
  - Edit title, tags, description, and body; autosave shows draft status; Save records the change.
  - View the page rendered as Markdown (no raw HTML executes).
  - Insert a link to another page via the picker and click it to navigate in-app.
  - Rename and move the page; existing inbound links still resolve.
  - Delete the page to Trash, then Restore it (no live page is clobbered).
  - Open version history: rows read like "Edited by <name> · 2 hours ago" with NO commit
    hashes or Git vocabulary; restoring an old version creates a new entry (nothing disappears).
result: [pending]

## Summary

total: 3
passed: 0
issues: 0
pending: 3
skipped: 0
blocked: 0

## Gaps

- truth: "After login, the app renders the workspace (left tree + page area) instead of a blank screen"
  status: resolved
  reason: "UAT blocker found during setup (before Test 1 could run): the SPA white-screened with `Uncaught TypeError: Cannot read properties of null (reading 'map')`. Root cause — the tree endpoint serialized a Go nil slice to JSON `null` for an empty/seed-empty repo root, and `LeftTree`'s react-query `data = []` default only guards `undefined`, not `null`, so `nodes.map` crashed. Classic Go-nil-slice→JSON-null integration bug; mocked unit tests returned `[]` and missed it. The trash/history endpoints already use `make([]T,0,…)` and were unaffected."
  severity: blocker
  test: 0
  resolution: "Fixed in commit d0329fa — backend `Tree()` returns `[]Node{}` (never nil); `LeftTree` coalesces null→`[]`. Regression tests added both layers (TestTreeEmptyRepoSerializesToArray; LeftTree null-data test). All gates green."
  artifacts: [internal/pages/tree.go, web/src/components/LeftTree.tsx]
  missing: []
