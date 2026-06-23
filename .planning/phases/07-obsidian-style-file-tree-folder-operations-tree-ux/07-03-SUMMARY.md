---
phase: 07-obsidian-style-file-tree-folder-operations-tree-ux
plan: 03
subsystem: ui
tags: [react, tanstack-query, html5-dnd, context-menu, a11y, regression-tests, vitest]

# Dependency graph
requires:
  - phase: 07-01
    provides: renameFolder/moveFolder client fns + atomic folder rename/move backend (409 collision reject)
  - phase: 07-02
    provides: deleteFolder/restoreFolderGroup client fns + delete_group_id on TrashEntry
provides:
  - "Clean-Rebuild Behavior Inventory regression net (treeBehaviors.test.tsx) pinning every shipped tree behavior"
  - "Cleanly rebuilt LeftTree (decomposed TreeRow/FolderRow/PageRow/RootDropZone + usePageDropZone hook)"
  - "Cleanly rebuilt TreeContextMenu (useViewportClamp/useFocusOnOpen/useDismissOnOutside hooks, items-array contract)"
  - "RenameModal + MoveDialog parameterized by node kind (page wired; folder branch implemented but UNREACHED until 07-04)"
affects: [07-04]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Regression-net-first clean rebuild: pin shipped behavior as black-box tests, prove GREEN against current code, rebuild keeping GREEN"
    - "Shared dialogs parameterized by a NodeKind discriminator with a per-kind COPY map (page now, folder pre-wired for 07-04)"
    - "DnD drop logic extracted into a usePageDropZone hook shared by folder rows + the root zone"

key-files:
  created:
    - web/src/components/__regression__/treeBehaviors.test.tsx
  modified:
    - web/src/components/LeftTree.tsx
    - web/src/components/TreeContextMenu.tsx
    - web/src/components/RenameModal.tsx
    - web/src/components/MoveDialog.tsx
    - web/src/components/LeftTree.test.tsx
    - web/src/components/TrashView.test.tsx

key-decisions:
  - "Regression net pinned and proven GREEN against the un-rebuilt components BEFORE any rebuild edit (the safety contract for the CONTEXT clean-rebuild override)"
  - "Folder branch of RenameModal/MoveDialog is implemented (calls renameFolder/moveFolder, 409→collision copy, dialog stays open) but left UNREACHED from LeftTree until Plan 04"
  - "MoveDialog folder kind excludes the folder itself + its own subtree from the destination options (cannot move a folder into itself)"
  - "No new tokens, no new deps, native HTML5 DnD only; the moveMut stays the shipped onSuccess-invalidate form (optimistic upgrade is Plan 04)"

patterns-established:
  - "Pattern 1: regression-net-first rebuild — black-box behavior pin green-before/green-after guards a from-scratch component rewrite"
  - "Pattern 2: NodeKind-parameterized shared dialog with per-kind copy map + 409 collision branch, pre-wired for a later wave"

requirements-completed: [TREE-01]

# Metrics
duration: 13min
completed: 2026-06-21
status: complete
---

# Phase 7 Plan 3: Clean Tree-UX Rebuild (regression-net-guarded) Summary

**Pinned the Clean-Rebuild Behavior Inventory as a 22-test regression net GREEN against the current components, then cleanly rebuilt LeftTree, TreeContextMenu, RenameModal, and MoveDialog (decomposed into focused sub-components + hooks, dialogs parameterized by node kind) with zero shipped-behavior regression and no new folder feature.**

## Performance

- **Duration:** 13 min
- **Started:** 2026-06-21T16:14:00Z
- **Completed:** 2026-06-21T16:27:00Z
- **Tasks:** 3
- **Files modified:** 7 (1 created, 6 modified)

## Accomplishments
- Pinned the Clean-Rebuild Behavior Inventory (07-RESEARCH §) as `treeBehaviors.test.tsx` — 22 black-box tests covering null-coalesce, expand/collapse, active-row highlight, page/folder/root menus, RBAC, page DnD + same-parent no-op, loading/error, TreeContextMenu a11y, and RenameModal/MoveDialog page paths. **Proven GREEN against the un-rebuilt components before any rebuild edit** (the load-bearing safety contract).
- Extended the `LeftTree.test.tsx` `vi.mock("../api/client")` factory with `renameFolder/moveFolder/deleteFolder/restoreFolderGroup` so the rebuilt components import without throwing.
- Cleanly rebuilt `LeftTree.tsx`: decomposed into `TreeRow`/`FolderRow`/`PageRow`/`RootDropZone` plus a shared `usePageDropZone` hook; extracted `menuItems` into folder/page/root builders and a `PAGE_DRAG_TYPE` constant. Every shipped behavior preserved; no new folder feature.
- Cleanly rebuilt `TreeContextMenu.tsx`: lifecycle split into `useViewportClamp`/`useFocusOnOpen`/`useDismissOnOutside` hooks; kept the items-array `{label,onSelect,danger}` contract and all a11y (role=menu/menuitem, arrow/Home/End, Tab trap, Esc, outside-click/scroll/resize close, 4px clamp) verbatim.
- Rebuilt `RenameModal.tsx` + `MoveDialog.tsx` parameterized by `kind: "page" | "folder"` (default "page"): the page path is behavior-identical; the folder branch calls `renameFolder`/`moveFolder` and surfaces a 409 as the non-fatal collision copy (dialog stays open) — but is UNREACHED from LeftTree until Plan 04.

## Task Commits

Each task was committed atomically:

1. **Task 1: Pin the Clean-Rebuild Behavior Inventory regression net (before any rebuild)** — `1ed0a85` (test)
2. **Task 2: Clean rebuild — LeftTree + TreeContextMenu** — `6f71360` (refactor)
3. **Task 3: Clean rebuild — RenameModal + MoveDialog parameterized by node kind** — `2f13f2b` (refactor; includes one Rule 3 fixture fix)

## Files Created/Modified
- `web/src/components/__regression__/treeBehaviors.test.tsx` - **Created.** Regression net pinning the shipped behavior inventory (22 tests).
- `web/src/components/LeftTree.tsx` - Rebuilt; decomposed into sub-components + `usePageDropZone`; behavior preserved.
- `web/src/components/TreeContextMenu.tsx` - Rebuilt; lifecycle extracted into three hooks; a11y + items-array contract preserved.
- `web/src/components/RenameModal.tsx` - Rebuilt; `kind`-parameterized with per-kind copy + 409 collision branch.
- `web/src/components/MoveDialog.tsx` - Rebuilt; `kind`-parameterized; folder destinations exclude self+subtree.
- `web/src/components/LeftTree.test.tsx` - `vi.mock` factory extended with the four 07-01/07-02 folder client fns.
- `web/src/components/TrashView.test.tsx` - Fixture given the 07-02 `delete_group_id` field (Rule 3 fix; see Deviations).

## Decisions Made
- **Regression net first, proven green against current code.** The CONTEXT override explicitly mandates a clean rebuild but "no user-visible regression"; the only safe way to honor both is to pin every shipped behavior as a black-box test that is green against the un-rebuilt code, then keep it green through the rewrite. Done.
- **Folder branch implemented but unreached.** Per the plan, `RenameModal`/`MoveDialog` grew a working `kind="folder"` path (renameFolder/moveFolder, 409 collision copy, dialog stays open) so Plan 04 wires it with zero further refactor — but LeftTree never opens these dialogs for folders yet.
- **MoveDialog folder destinations exclude self + subtree.** A folder cannot be moved into itself or a descendant; `collectFolders` takes an `excludePrefix` for the folder kind. (Page kind keeps the full folder list unchanged.)
- **No optimistic update yet.** `moveMut` stays the shipped `onSuccess`-invalidate form; the `onMutate/onError/onSettled` optimistic upgrade is Plan 04 (the commit-wait remains the correctness backstop there).
- **CSS untouched.** `LeftTree.css`/`TreeContextMenu.css` are already token-only and maintainable; no new tokens introduced, classes preserved so the regression net's class assertions hold.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Fixed pre-existing TrashView.test.tsx fixture missing the 07-02 `delete_group_id` field**
- **Found during:** Task 3 (running `npm run build`, which invokes the stricter `tsc -b` over the whole project)
- **Issue:** `TrashEntry` gained a required `delete_group_id` field in 07-02, but the `SAMPLE` fixture in `TrashView.test.tsx` (last touched in 01-04) was never updated. `npx tsc --noEmit` (vitest's looser tsconfig) passed, but `tsc -b` failed `TS2741`, blocking the required `npm run build` success criterion.
- **Fix:** Added `delete_group_id: ""` to the fixture (a solo per-page delete carries no group id, per the client.ts contract).
- **Files modified:** web/src/components/TrashView.test.tsx
- **Verification:** `npm run build` succeeds; full `npm test` (215 tests) green.
- **Committed in:** `2f13f2b` (Task 3 commit)

---

**Total deviations:** 1 auto-fixed (1 blocking)
**Impact on plan:** The fix is a one-line test-fixture correction inherited from the 07-02 dependency; it unblocks the required build with no scope creep and no production-code change.

## Issues Encountered
- The plan's Task 3 verify pattern `src/components/RenameModal src/components/MoveDialog` matches no dedicated test files (none exist for those components). The regression net (`treeBehaviors.test.tsx`) is the authoritative page-path coverage for both dialogs and is green; the full suite + build confirm correctness.

## Known Stubs
None — the folder branch of RenameModal/MoveDialog is fully implemented (not a stub); it is intentionally not yet opened from LeftTree, which Plan 04 will do. No placeholder data or empty-value rendering introduced.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Clean, decomposed tree base ready for Plan 04 to layer on: folder DnD (second dataTransfer key), folder context-menu mutate actions, optimistic `["tree"]` cache updates, DeleteFolderDialog, and the grouped TrashView row.
- `RenameModal`/`MoveDialog` already accept `kind="folder"` (renameFolder/moveFolder + 409 collision copy) — Plan 04 only needs to open them for folder targets.
- Regression net stays the guardrail: any Plan 04 change that regresses a shipped behavior breaks `treeBehaviors.test.tsx`.

## Self-Check: PASSED

- Created file present: `web/src/components/__regression__/treeBehaviors.test.tsx`
- Rebuilt files present: LeftTree.tsx, TreeContextMenu.tsx, RenameModal.tsx, MoveDialog.tsx
- Task commits present: `1ed0a85`, `6f71360`, `2f13f2b`
- `cd web && npm test` → 215 passed (25 files); `npx tsc --noEmit` clean; `npm run build` ok; eslint clean on touched files.

---
*Phase: 07-obsidian-style-file-tree-folder-operations-tree-ux*
*Completed: 2026-06-21*
