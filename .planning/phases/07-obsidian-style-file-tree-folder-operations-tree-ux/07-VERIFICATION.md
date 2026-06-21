---
phase: 07-obsidian-style-file-tree-folder-operations-tree-ux
verified: 2026-06-21T17:05:00Z
status: passed
score: 7/7 must-haves verified
perceptual_validation: browser-automated (Playwright, 2026-06-21) — folder menu, rename (CR-01), collision, delete, grouped restore confirmed live; DnD pointer-feel is the only residual manual item (logic automated-tested)
overrides_applied: 0

# Browser-Automated Validation (2026-06-21)
#
# Drove the running app (Go binary serving the embedded SPA on :8098) with Playwright:
# - TREE-01: folder right-click menu = exactly [New page here, New folder here, Rename, Move, Delete] (role=menu/menuitem). PASS
# - TREE-02 + CR-01 fix: renaming folder zphase7→zphase7renamed navigated to /app/page/zphase7renamed/index.md
#   (NOT a bare dir → no HTTP 500) and the index page rendered cleanly; audit logged folder_rename. PASS
# - TREE-06 collision: renaming a folder onto an existing name "test" returned 409, the dialog stayed open with
#   "A folder with that name already exists there. Pick a different name or destination.", folder kept its name (no merge). PASS
# - TREE-04 delete: confirm dialog read "Delete 'zphase7renamed'? This folder and its 1 page will move to Trash…"
#   (names page count; destructive token); confirming trashed it (audit folder_trash). PASS
# - TREE-05 grouped restore: Trash showed "Folder 'zphase7renamed' · 1 page … Restore folder" alongside per-page
#   "Restore page" rows; clicking Restore folder restored by group id (audit folder_restore target=<groupid>) and
#   recreated data/repo/zphase7renamed/index.md on disk. PASS
# - UI fix live: rename-folder help text = "Pages inside this folder, and links to them, will keep working."
# - TREE-03 DnD: applyMove + dropAllowed self/descendant truth table + optimistic onMutate/onError/onSettled rollback
#   are covered by green useTreeMutations/LeftTree vitest; the move uses the same validated moveFolder backend path.
#   Residual MANUAL: native HTML5 drag pointer-feel (drop-target ring, not-allowed cursor, optimistic snappiness,
#   menu viewport 4px clamp) — Playwright can't reliably drive HTML5 DnD; needs a human drag.
#
# OBSERVATION (logged to deferred-items, not a blocker): under a SATURATED single-writer git worker (the cold-start
# search-index rebuild was hogging it, "commit wait timed out; returning success, job stays queued"), the optimistic
# delete's onSettled tree-refetch can beat the queued commit and momentarily RE-SHOW the deleted folder until the
# commit lands. The delete is correct on disk (verified) and self-corrects on the next refetch; in steady state
# (fast commits) it does not occur. Worth a future guard (e.g. delay/skip onSettled-invalidate while a commit is queued).
human_verification:
  - test: "Drag-and-drop feel — drag a folder onto another folder and onto root"
    expected: "Folder moves as a unit; destination folder row highlights during hover (drop-target ring); tree updates INSTANTLY before the network settles; dragging a folder onto itself or one of its own children shows the native not-allowed cursor, no highlight, and nothing moves"
    why_human: "Native HTML5 DnD pointer interaction, perceptual timing of optimistic update, and visual affordance (ring + cursor) are not assertable in jsdom; the automated guard (dropAllowed) and optimistic apply (useTreeMove) are tested, but the felt snappiness is perceptual"
  - test: "Optimistic update snappiness (tree reconciles after network settles)"
    expected: "Move a folder; the tree node appears under its new parent immediately (before the commit returns); after the network responds the node stays in place with no visible jump"
    why_human: "Perceptual timing — jsdom tests assert the hook fires onMutate but cannot observe wall-clock latency or whether the React render was synchronous from the user's perspective"
  - test: "Context menu viewport-edge clamp (4px clamp)"
    expected: "Right-clicking a tree row near the bottom or right edge of the viewport shows the menu fully on-screen — it does not overflow off the bottom or right"
    why_human: "Layout and viewport geometry are not available in jsdom; TreeContextMenu uses a useViewportClamp hook that positions the menu using getBoundingClientRect, which returns zeros in test environments"
---

# Phase 7: Obsidian-style File Tree (folder operations & tree UX) Verification Report

**Phase Goal:** Manage folders and pages directly in the file tree — right-click context menus, drag-and-drop, and folder rename/move/delete as a unit — so organizing feels like Obsidian. Folder ops relocate all contained pages and rewrite inbound links in ONE commit (byte-stable round-trip holds); folder delete is trash-recoverable with GROUPED restore; tree updates OPTIMISTICALLY; the tree UX was a CLEAN REBUILD that must not regress shipped behavior.

**Verified:** 2026-06-21T17:05:00Z
**Status:** human_needed
**Re-verification:** No — initial verification

---

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|---------|
| 1 | TREE-01: Right-click folder shows 5 actions (New page/folder, Rename, Move, Delete); page shows Rename/Move/Version history/Delete; reader sees Version history only | VERIFIED | `LeftTree.tsx:158-203` implements `folderMenuItems` (5 items, Delete is `danger:true`) and `pageMenuItems` (RBAC-gated); `treeBehaviors.test.tsx` inventory #6, #7, #7(reader) assert this; 22/22 regression tests green |
| 2 | TREE-02: Folder rename/move = ONE commit relocating index.md + all dir/ descendants + rewriting all inbound links; okf byte-stable round-trip holds | VERIFIED | `rename.go:212-303` implements `relocateFolder` with collision precheck first, single `EnqueueCommit` with all writes+removes; `okf.RewriteLinksMoved` rewrites cross-linked siblings correctly; `TestRelocateFolder`, `TestMoveFolder`, `TestRelocateFolder_NoCorruption` all pass; `TestGoldenRoundTrip` green |
| 3 | TREE-03: Folder+page DnD with OPTIMISTIC tree updates (onMutate snapshot → applyMove → onError rollback → onSettled invalidate); automated vitest exists | VERIFIED | `useTreeMutations.ts:220-242` implements `useTreeMove` with full onMutate/onError/onSettled pattern; `applyMove` is a pure prefix-swap mirror of server `relocateFolder`; `LeftTree.tsx:122-134` wires `useTreeMove`; 17 tests in `useTreeMutations.test.tsx` cover applyMove correctness, dropAllowed truth table, optimistic apply+rollback |
| 4 | TREE-04: Folder delete → pages to trash (shared group id), restorable; no permanent delete | VERIFIED | `trash.go:153-178` implements `DeleteFolder` enumerating descendants via `descendantPages`, assigning one `crypto/rand` group id, looping `deleteWithGroup`; migration `0008_trash_group.sql` adds nullable `delete_group_id TEXT`; `TestDeleteFolder` passes |
| 5 | TREE-05: Grouped restore (RestoreGroup, index.md first) recreates folder; per-page restore unchanged; TrashView grouped row with Restore folder button | VERIFIED | `trash.go:395-453` implements `RestoreGroup` with `isFolderIndex` sort (index.md first), per-page `Restore` loop with collision auto-suffix; `handlers_trash.go:139-166` wires `/trash/group/{id}/restore`; `TrashView.tsx:71-100,201-222` renders grouped rows with `Restore folder` button calling `restoreFolderGroup`; `TestRestoreGroup` passes; TrashView tests green |
| 6 | TREE-06: Folder move/rename rejects collision (ErrFolderExists → 409, no merge); invalid drag (self/descendant) prevented by dropAllowed path-prefix guard | VERIFIED | `rename.go:213-219` precheck returns `ErrFolderExists` before any disk write; `handlers_pages.go:414-415` maps it to HTTP 409 with UI-SPEC copy; `dropAllowed` in `useTreeMutations.ts:57-70` rejects self (`targetFolder === dragPath`), descendant (`startsWith(dragPath+"/")`), and same-parent; `LeftTree.tsx:129` re-checks on drop; `TestRelocateFolder_Collision` passes; `LeftTree.test.tsx` folder DnD guard tests pass |
| 7 | Clean rebuild: regression net exists, was pinned BEFORE rebuild, and no shipped behavior regressed | VERIFIED | `web/src/components/__regression__/treeBehaviors.test.tsx` created in Plan 03 Task 1 (commit `1ed0a85`) before any rebuild edit; 22 black-box tests covering null-coalesce, expand/collapse, active-row, page/folder/root menus, RBAC, DnD, loading/error, TreeContextMenu a11y, RenameModal/MoveDialog page paths; 22/22 green post-rebuild |

**Score:** 7/7 truths verified

---

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|---------|--------|---------|
| `internal/pages/rename.go` | `relocateFolder`, `RenameFolder`, `MoveFolder`, `descendantPages`, `ErrFolderExists` | VERIFIED | All 5 symbols present and substantive |
| `internal/pages/trash.go` | `DeleteFolder`, `RestoreGroup`, `deleteWithGroup`, grouped `ListTrash`, `DeleteGroupID` on `TrashEntry` | VERIFIED | All present; `deleteWithGroup` uses `sql.NullString` for SQL NULL on solo deletes |
| `internal/store/migrations/0008_trash_group.sql` | `ALTER TABLE trash ADD COLUMN delete_group_id TEXT` | VERIFIED | File exists, correct SQL, migration runner applies it |
| `internal/server/handlers_pages.go` | `handleRenameFolder`, `handleMoveFolder`, `handleDeleteFolder` suffix dispatch | VERIFIED | Lines 251-262 dispatch all three suffixes before the existing `/rename` branch |
| `internal/server/handlers_trash.go` | `handleRestoreFolderGroup`, `DeleteGroupID` in listing | VERIFIED | Handler present at line 139; `trashEntryResponse.DeleteGroupID` copied in `handleListTrash` |
| `internal/server/router.go` | `editor.Post("/trash/group/{id}/restore", ...)` | VERIFIED | Line 175 |
| `web/src/components/hooks/useTreeMutations.ts` | `applyMove`, `dropAllowed`, `useTreeMove`, `useFolderDelete` with onMutate/onError/onSettled | VERIFIED | All exports present; full optimistic pattern implemented |
| `web/src/components/DeleteFolderDialog.tsx` | Destructive confirm naming page count N | VERIFIED | `countFolderPages` from cache; N==1 singular; `Delete folder`/`Keep folder` labels |
| `web/src/components/TrashView.tsx` | Grouped folder row + Restore folder (restoreFolderGroup) | VERIFIED | `buildRows` folds by `delete_group_id`; grouped row renders `Restore folder` |
| `web/src/components/__regression__/treeBehaviors.test.tsx` | 22-test regression net pinned before rebuild | VERIFIED | 22 tests, all green |
| `web/src/components/LeftTree.tsx` | `application/x-okf-folder` drag type, 5-action folder menu, `useTreeMove` wired | VERIFIED | Lines 16-17, 158-183, 122 |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|----|--------|---------|
| `handlers_pages.go:handleRenamePage` | `pages.RenameFolder` | suffix dispatch `/rename-folder` | VERIFIED | Line 252 → `handleRenameFolder` → `h.pages.RenameFolder` |
| `handlers_pages.go:handleRenamePage` | `pages.MoveFolder` | suffix dispatch `/move-folder` | VERIFIED | Line 255 → `handleMoveFolder` → `h.pages.MoveFolder` |
| `handlers_pages.go:handleRenamePage` | `pages.DeleteFolder` | suffix dispatch `/delete-folder` | VERIFIED | Line 259 → `handleDeleteFolder` → `h.pages.DeleteFolder` |
| `pages.relocateFolder` | `EnqueueCommit` | one multi-file commit | VERIFIED | `rename.go:285` — single `EnqueueCommit` with all writes+removes |
| `pages.relocateFolder` | `rewriteFolderInboundLinks` | unified link rewrite pass | VERIFIED | `rename.go:259` — `okf.RewriteLinksMoved` applied per moved page |
| `pages.DeleteFolder` | `deleteWithGroup` (shared group id) | crypto/rand group id loop | VERIFIED | `trash.go:167-176` |
| `pages.RestoreGroup` | existing `Restore(id)` | index.md-first sorted loop | VERIFIED | `trash.go:428-451` |
| `router.go` | `handleRestoreFolderGroup` | `editor.Post("/trash/group/{id}/restore")` | VERIFIED | Line 175 |
| `LeftTree.tsx` | `useTreeMove` (optimistic move) | folder/page DnD + menu dispatch | VERIFIED | Line 122; `dropNode` uses it |
| `LeftTree.tsx` | `moveFolder/deleteFolder` (client) | folder menu actions | VERIFIED | Lines 170-182; `DeleteFolderDialog` uses `useFolderDelete` |
| `TrashView.tsx` | `restoreFolderGroup` (client) | grouped Restore folder button | VERIFIED | Line 140, 216 |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|--------------|--------|--------------------|--------|
| `TrashView.tsx` | `data` (trash entries) | `useQuery(["trash"], listTrash)` → `GET /trash` → `pages.ListTrash` → SQLite `trash` table | Yes — SQLite query with `delete_group_id` column; real data from DB | FLOWING |
| `LeftTree.tsx` | `data` (tree nodes) | `useQuery(["tree"], getTree)` → `GET /tree` (existing) | Yes — existing tree query; folder nodes reflect real dir/index.md structure | FLOWING |
| `useTreeMove` (onMutate) | `["tree"]` cache | `applyMove(old, src, dest, kind)` | Yes — pure transform then server reconcile via onSettled invalidate | FLOWING |
| `DeleteFolderDialog.tsx` | `pageCount` | `countFolderPages(queryClient.getQueryData(["tree"]))` | Yes — reads live cache populated by same `getTree` query | FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Go folder relocate tests green | `go test ./internal/pages/ -run 'TestRelocateFolder\|TestMoveFolder'` | ok (0.392s) | PASS |
| Go folder collision test green | `go test ./internal/pages/ -run 'TestRelocateFolder_Collision'` | PASS (0.05s) | PASS |
| Go no-corruption test green | `go test ./internal/pages/ -run 'TestRelocateFolder_NoCorruption'` | PASS (0.15s) | PASS |
| Go folder delete/restore tests green | `go test ./internal/pages/ -run 'TestDeleteFolder\|TestRestoreGroup'` | ok (0.544s) | PASS |
| Go partial-progress test green | `go test ./internal/pages/ -run 'TestDeleteFolder_PartialProgress'` | PASS (0.16s) | PASS |
| okf golden round-trip unaffected | `go test ./internal/okf/ -run TestGoldenRoundTrip` | PASS 8/8 subtests | PASS |
| All Go packages green | `go test ./internal/pages/ ./internal/store/ ./internal/server/ ./internal/okf/` | 4 packages ok | PASS |
| useTreeMutations vitest (17 tests) | `npx vitest run src/components/hooks/useTreeMutations.test.tsx` | 17 passed | PASS |
| Regression net vitest (22 tests) | `npx vitest run src/components/__regression__/treeBehaviors.test.tsx` | 22 passed | PASS |
| Full frontend suite | `npx vitest run` | 244/244 passed (27 files) | PASS |
| TypeScript clean | `npx tsc --noEmit` | no output (clean) | PASS |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|------------|-------------|--------|---------|
| TREE-01 | Plans 03, 04 | Right-click menus with correct action sets | SATISFIED | `LeftTree.tsx` folderMenuItems (5) / pageMenuItems (4 editor, 1 reader); regression tests #6/#7 |
| TREE-02 | Plan 01 | Folder rename/move = ONE commit, inbound links rewritten, byte-stable | SATISFIED | `relocateFolder` single `EnqueueCommit`; `TestRelocateFolder` / `TestRelocateFolder_NoCorruption` green |
| TREE-03 | Plan 04 | DnD with optimistic updates + rollback; automated vitest | SATISFIED | `useTreeMove` onMutate/onError/onSettled; 17 automated tests |
| TREE-04 | Plan 02 | Folder delete to trash (group id), restorable; no permanent delete | SATISFIED | `DeleteFolder` + migration 0008; `TestDeleteFolder` green |
| TREE-05 | Plans 02, 04 | Grouped restore (index.md first); TrashView grouped row | SATISFIED | `RestoreGroup` + handler + `TrashView` grouped row; `TestRestoreGroup` green |
| TREE-06 | Plans 01, 04 | Collision → 409 (no merge); invalid drag prevented | SATISFIED | `ErrFolderExists` precheck + 409 mapping; `dropAllowed` guard; `TestRelocateFolder_Collision` green |

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| (none) | — | No TBD/FIXME/XXX markers, no stub implementations, no empty handlers | — | — |

No debt markers or stub implementations found in any Phase 7 files. The CodeMirror Phase 6 timing flake (`livePreview.test.ts`, `headingAnchors.test.ts`) was documented in `deferred-items.md` and is NOT a Phase 7 file — both pass in isolation and are excluded from this scope.

### Human Verification Required

The following items require a running application to verify. All correctness is automated; only perceptual/visual qualities remain.

#### 1. Drag-and-Drop Feel (DnD pointer interaction + visual affordance)

**Test:** Run the app, log in as an editor, open the workspace tree. Drag a folder onto another folder — confirm it moves as a unit and the target folder row shows a drop-target highlight (ring). Drag a folder onto the root drop zone — confirm it moves to the top level. Drag a folder onto itself or one of its own children — confirm the native cursor shows not-allowed, there is no drop-target highlight, and nothing moves.

**Expected:** The drag-over highlight appears on valid drop targets only. The not-allowed cursor appears on invalid drops. The tree updates immediately on a valid drop before the server responds.

**Why human:** Native HTML5 DnD pointer events and visual ring are not assertable in jsdom. The `dropAllowed` guard logic and the optimistic `useTreeMove` hook are fully tested (17 automated cases) but the felt quality requires a live browser.

#### 2. Optimistic Update Snappiness

**Test:** Move a folder via drag-and-drop while watching the tree. The moved folder should appear under its new parent immediately (before the network commit returns). After the network responds, the folder should stay in place with no visible jump.

**Expected:** The tree updates feel instant (no loading spinner or perceptible delay). The post-reconcile refetch does not cause a visible node jump because `applyMove` mirrors the server's literal prefix swap.

**Why human:** Perceptual timing cannot be asserted in automated tests. The hook's `onMutate` fires synchronously with the drag drop event, but wall-clock snappiness requires a live browser with a real backend.

#### 3. Context Menu Viewport-Edge Clamp (4px clamp)

**Test:** Right-click a tree row near the bottom edge and near the right edge of the viewport window. Confirm the context menu appears fully on-screen in both cases — it should not overflow off the bottom or the right side.

**Expected:** The menu is clamped to stay at least 4px within the viewport edges in all directions.

**Why human:** `getBoundingClientRect` returns zeros in jsdom; the `useViewportClamp` hook's repositioning logic is correct by code inspection but its output depends on actual layout geometry only available in a real browser.

---

## Gaps Summary

None. All 7 must-haves are VERIFIED by code inspection and automated tests. The three items in Human Verification are perceptual qualities (DnD feel, snappiness, menu geometry) for which the correctness layer (guards, hooks, handlers) is fully automated and green. These are expected human-verify items for any phase that ships native DnD and a viewport-clamped context menu.

The CodeMirror Phase 6 timing flake is documented in `deferred-items.md` and is not a Phase 7 concern.

---

_Verified: 2026-06-21T17:05:00Z_
_Verifier: Claude (gsd-verifier)_
