---
status: complete
phase: 01-okf-pages-navigation-hidden-git
source: [01-VERIFICATION.md]
started: 2026-06-19T00:30:00Z
updated: 2026-06-21T00:00:00Z
---

## Current Test

[testing complete]

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
  NOTE — a SEPARATE, more serious autosave bug was found during this test (silent lost trailing write + false "Saved"); the WR-03 no-409 criterion itself passed. That bug has since been FIXED (commit 7985857, quick task autosave-trailing-write) and re-verified live; see Gaps entry (status: resolved).

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
result: pass
note: |
  Verified by Claude end-to-end via headless browser (Playwright) + API against a live server:
  - Create by title → appears in left tree by title (no path shown). [Tests 1 setup + tree snapshot]
  - Edit + autosave → cycles Saving…→Saved, persists; lost-write bug fixed (Test 2). [Test 2]
  - Rendered Markdown in Read mode; raw-HTML off (react-markdown + rehype-sanitize per VERIFICATION).
  - Link picker present; in-app link navigation works. [Test 1]
  - RENAME keeps inbound links: renamed Beta→Bravo (beta.md→bravo.md); Alpha's body link auto-rewrote
    to bravo.md and resolves to /app/page/bravo.md in Read mode. [verified live]
  - DELETE to Trash + RESTORE: deleted note.md → appeared in Trash with provenance (title/original_path/
    deleted_by/deleted_at) → restored → note.md back (200), no live page clobbered. [verified live]
  - VERSION HISTORY UI is clean: rows read "Renamed by admin · 1 minute ago" / "Edited by admin · …"
    with View/Restore actions; NO SHA, NO git vocabulary visible in the UI (grep over rendered text). [verified live]
  - Restore-version = forward commit (VER-03) accepted from code (TestRestoreForwardCommit) + the working
    trash-restore above; not re-clicked in browser.
  Minor gap logged separately: the history API's `version` field leaks the raw 40-char Git SHA over the
  wire (UI hides it) — see Gaps "history-api-leaks-raw-sha".

## Summary

total: 3
passed: 3
issues: 1
pending: 0
skipped: 0
blocked: 0

## Gaps

- truth: "Version history exposes NO Git commit SHAs to the client (VER-02), including over the API"
  status: open
  reason: |
    Found by Claude during Test 3. The history UI is clean (no SHA/git vocab shown — confirmed live), BUT the
    GET /api/v1/pages/{path}/history JSON response serializes the raw 40-char Git commit SHA as the `version`
    field (e.g. "version":"03504223a50a7c5100e0ed64893d953a2de3f435"). VERIFICATION.md (truth #6) claimed
    "gitstore.Commit.Token (the SHA) is never serialized to HistoryEntry" and described `version` as an
    "opaque version token" — that is inaccurate: the token IS the SHA in the clear, visible in DevTools/network.
    User-facing impact is low (UI never displays it), but it contradicts the hidden-Git principle and the
    explicit VER-02 verification claim; the version token used by view/restore-version is not actually opaque.
  severity: minor
  test: 3
  artifacts: ["internal/pages/history.go (HistoryEntry.Version = commit SHA)", "internal/gitstore/history.go"]
  missing: ["encode the version token (e.g. opaque/HMAC or short opaque id) so the raw 40-hex SHA is not exposed over the API, OR consciously accept it and correct the VERIFICATION/decision wording to 'SHA used as version token, hidden in UI only'"]

- truth: "Autosave persists every edit (no silent lost write), and the 'Saved' status only shows when content is actually on the server"
  status: resolved
  resolution: |
    Fixed by quick task autosave-trailing-write (commit 7985857). Root cause: the draft(1s)+idle(6s) two-timer scheme fired overlapping/late saves so a stale snapshot could commit after a newer one and clobber it (git history during UAT showed "...SECOND-B" committed then a stale "...FIRST-A" save overwriting it 3s later). Replaced with a single serialized, single-flight, coalescing saver (loop saves until server == editor, reading fresh content + advanced base_revision each iteration) + seed-editor-state-once. Re-verified LIVE via headless browser on the exact repro: type A → autosave → type B → idle → both persist in HEAD; status "Saved" matches editor; 2 PUTs both 204, no 409; stress bursts also persist exactly. Regression test added; 110/110 frontend tests pass; WR-03 no-409 still holds.
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
