---
status: testing
phase: 01-okf-pages-navigation-hidden-git
source: [01-VERIFICATION.md]
started: 2026-06-19T00:30:00Z
updated: 2026-06-21T00:00:00Z
---

## Current Test

number: 3
name: Holistic non-technical-user round trip (phase goal)
expected: |
  Full UI loop with Git never visible. PARTIALLY verified by Claude (create/edit/save/
  render/link-navigation done via Playwright). Remaining: rename/move keeps inbound links,
  delete-to-trash + restore, version history reads with no Git vocabulary. PAUSED pending
  user decision on the autosave lost-write gap found in Test 2.
awaiting: user decision (fix autosave gap now, or continue Test 3)

## Tests

### 1. Cross-folder and same-directory in-app link navigation (PAGE-08 / D-06)
expected: In Read mode, clicking a cross-folder relative link (page in `docs/` → `runbooks/deploy.md`) and a same-directory link both navigate to the correct page. (WR-02/WR-06 was fixed in code, commit `0c8421e`, and unit-tested in `web/src/lib/mdlink.test.ts`; this is a light in-browser regression confirmation.)
result: pass
note: |
  Verified by Claude via headless browser (Playwright) against a live server. Seeded docs/guide.md with a cross-folder link `[Deploy runbook](../runbooks/deploy.md)` and a same-directory link `[Setup](setup.md)`.
  - Rendered hrefs in Read mode: ../runbooks/deploy.md -> /app/page/runbooks/deploy.md ; setup.md -> /app/page/docs/setup.md (no broken /app/page/../...).
  - Clicking the cross-folder link navigated to /app/page/runbooks/deploy.md (heading "Deploy").
  - Clicking the same-directory link navigated to /app/page/docs/setup.md (heading "Setup").
  - mdlink.test.ts: 16/16 unit tests green.

### 2. Autosave does not self-conflict in a single-user session (WR-03)
expected: Open a page, type to trigger autosave, then stop and wait ~7–8s (spanning the 1s draft save and the 6s idle commit). Status shows Saving… → Draft saved → Saving… → Saved. NO "This page was changed somewhere else" 409 banner appears in a single-user session (try once on a throttled/slow network in DevTools).
result: pass
note: |
  Verified by Claude via headless browser (Playwright) against a live server. Edited docs/guide.md in the editor with character-by-character typing, then waited across the full draft+idle-commit window (>8s), then did two rapid bursts to deliberately overlap the 1s draft and 6s commit timers.
  - Autosave status cycled Saving… → Saved (no false conflict at any point).
  - NO "changed somewhere else"/conflict banner appeared.
  - Browser console: 0 errors except a pre-login 401 on /auth/me (expected before sign-in); NO 409 responses.
  - Server log: no 409/stale/conflict lines.
  - The WR-03 caveat from VERIFICATION (false 409 under overlapping timers) did not reproduce.
  CAVEAT — a SEPARATE, more serious autosave bug was found during this test (silent lost trailing write + false "Saved"); see Gaps "autosave-drops-trailing-edit". The WR-03 no-409 criterion itself passes; the new bug is tracked separately.

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
passed: 2
issues: 1
pending: 1
skipped: 0
blocked: 0

## Gaps

- truth: "Autosave persists every edit (no silent lost write), and the 'Saved' status only shows when content is actually on the server"
  status: failed
  reason: |
    Found by Claude via headless browser (Playwright) against a live server while running Test 2. Reproduced TWICE: a trailing edit made shortly after a prior autosave cycle is NEVER sent to the server (no PUT) and is silently lost, while the status indicator shows "Saved".
    Exact repro on docs/guide.md in the editor:
      1. Type text A (e.g. " AAA111") → autosave fires a PUT (204), status "Saved".
      2. ~1.5s later type text B (e.g. " BBB222").
      3. Stop and wait (waited 17s total).
      4. Result: PUT request bodies show the last PUT ended at A — B was never transmitted. git HEAD and the working tree contain A but NOT B. Status indicator still reads "Saved" while B sits in the editor.
    Network proof: GET/PUT trace shows the final PUT body ends "...AAA111" with no BBB222; no 409s (this is NOT the WR-03 conflict — it's a separate trailing-edit/baseline bug).
    Recovery: clicking the explicit "Save page" button DOES persist B (commit b25d9f8), so React state held B — the autosave change-detection/debounce dropped it.
    Impact: silent data loss of a user's last edit when they rely on autosave + the "Saved" indicator and then navigate/refresh. Directly threatens the project's core "data must remain" value.
    Hypothesis (unconfirmed): the autosave "last-saved baseline" is advanced optimistically (when a save is scheduled, or via a stale closure) so the trailing edit is treated as already-saved; status is derived from that baseline, hence the false "Saved".
  severity: major
  test: 2
  artifacts: [web/src/routes/PageEditor.tsx, web/src/api/client.ts]
  missing: ["autosave must re-arm/flush after each save settles so a trailing edit is always sent", "the 'Saved' status must reflect server-confirmed content, not an optimistic baseline", "regression test for type→save→type→idle preserving the trailing edit"]

- truth: "After login, the app renders the workspace (left tree + page area) instead of a blank screen"
  status: resolved
  reason: "UAT blocker found during setup (before Test 1 could run): the SPA white-screened with `Uncaught TypeError: Cannot read properties of null (reading 'map')`. Root cause — the tree endpoint serialized a Go nil slice to JSON `null` for an empty/seed-empty repo root, and `LeftTree`'s react-query `data = []` default only guards `undefined`, not `null`, so `nodes.map` crashed. Classic Go-nil-slice→JSON-null integration bug; mocked unit tests returned `[]` and missed it. The trash/history endpoints already use `make([]T,0,…)` and were unaffected."
  severity: blocker
  test: 0
  resolution: "Fixed in commit d0329fa — backend `Tree()` returns `[]Node{}` (never nil); `LeftTree` coalesces null→`[]`. Regression tests added both layers (TestTreeEmptyRepoSerializesToArray; LeftTree null-data test). All gates green."
  artifacts: [internal/pages/tree.go, web/src/components/LeftTree.tsx]
  missing: []

- truth: "Creating a folder (and saving/deleting pages) succeeds without a client error"
  status: resolved
  reason: "UAT blocker: 'New folder' showed `Unexpected end of JSON input`. The server created the folder (201 + audit) but the client's `mutate()` helper called `res.json()` on any non-204 success, throwing on the empty 201 body. Same path backs savePage/deletePage/rename — would have failed Test 1's Save step too."
  severity: blocker
  test: 0
  resolution: "Fixed in commit aea21b1 — `mutate()` reads the body as text and only JSON.parse's when non-empty (empty 2xx → undefined); `handleCreateFolder` also returns a `{path}` JSON body for parity. Regression tests added (web/src/api/client.test.ts: 201-empty, 200-empty, 204, JSON-body, error-body). All gates green (vitest 97)."
  artifacts: [web/src/api/client.ts, web/src/api/client.test.ts, internal/server/handlers_pages.go]
  missing: []
