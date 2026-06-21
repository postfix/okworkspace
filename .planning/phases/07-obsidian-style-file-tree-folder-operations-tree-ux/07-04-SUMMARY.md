---
phase: 07-obsidian-style-file-tree-folder-operations-tree-ux
plan: 04
subsystem: ui
tags: [react, tanstack-query, optimistic-updates, html5-dnd, folder-ops, grouped-trash, vitest]

# Dependency graph
requires:
  - phase: 07-01
    provides: renameFolder/moveFolder client fns + atomic folder rename/move backend (409 collision reject)
  - phase: 07-02
    provides: deleteFolder/restoreFolderGroup client fns + delete_group_id on TrashEntry
  - phase: 07-03
    provides: cleanly rebuilt LeftTree/TreeContextMenu/RenameModal/MoveDialog (folder branch pre-wired) + regression net
provides:
  - "useTreeMutations hook: optimistic onMutate/onError/onSettled over [\"tree\"] + pure applyMove/dropAllowed/countFolderPages helpers"
  - "Folder drag-and-drop (application/x-okf-folder) with self/descendant/same-parent guard (TREE-06 client)"
  - "5-action folder context menu (New page/folder here, Rename, Move, Delete) wired to the 07-01/07-02 backend"
  - "DeleteFolderDialog naming the affected page count N"
  - "Grouped TrashView folder row + Restore folder (restoreFolderGroup)"
affects: []

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Optimistic ['tree'] cache update: onMutate cancel+snapshot+applyMove → onError rollback → onSettled invalidate (CONTEXT override of wait-then-refetch)"
    - "Pure applyMove does the literal prefix swap identical to server relocateFolder (Pitfall 6) so the post-reconcile refetch never jumps the node"
    - "dropAllowed validity computed DURING dragover (reads the active-drag path from a module ref since dataTransfer.getData is empty mid-drag) → no highlight + native not-allowed for invalid drops"
    - "Two custom dataTransfer keys (x-okf-page / x-okf-folder) share one drop target, disambiguated by type list"

key-files:
  created:
    - web/src/components/hooks/useTreeMutations.ts
    - web/src/components/hooks/useTreeMutations.test.tsx
    - web/src/components/DeleteFolderDialog.tsx
  modified:
    - web/src/components/LeftTree.tsx
    - web/src/components/LeftTree.css
    - web/src/components/LeftTree.test.tsx
    - web/src/components/TrashView.tsx
    - web/src/components/TrashView.test.tsx
    - web/src/components/__regression__/treeBehaviors.test.tsx

key-decisions:
  - "applyMove is a pure, non-mutating TreeNode[] transform (literal prefix swap == server relocateFolder) so the optimistic tree equals the eventual refetch (Pitfall 6); covered by a vitest asserting descendant paths"
  - "dropAllowed reads the dragged path from a module-level activeDragPath ref because the HTML5 spec returns '' from dataTransfer.getData() during dragover — the only way the affordance can be correct DURING drag-over"
  - "Invalid drops do NOT preventDefault (so the browser shows native cursor:not-allowed) and apply no highlight — quiet non-colored rejection per UI-SPEC; destructive red stays reserved for Delete"
  - "Reader folder menu is suppressed entirely (empty menu never renders) rather than opening an empty popup"
  - "Grouped TrashView row derives the folder name from the common-ancestor index.md path; one row per delete_group_id keeps the original newest-first ordering"

patterns-established:
  - "Centralized optimistic tree-mutation hook (useTreeMove/useFolderDelete) consumed by LeftTree + DeleteFolderDialog"
  - "Module-ref active-drag path to make a dragover-time validity guard possible under the HTML5 DnD data-availability rule"

requirements-completed: [TREE-03, TREE-06]

# Metrics
duration: 10min
completed: 2026-06-21
status: complete
---

# Phase 7 Plan 4: Folder Operations + Optimistic Tree UX Summary

**Layered the net-new folder operations onto the Plan 03 base: a centralized optimistic `["tree"]` mutation hook (onMutate snapshot → applyMove → onError rollback → onSettled invalidate) whose pure `applyMove` mirrors the server's literal prefix swap, native folder drag-and-drop with a dragover-time self/descendant/same-parent guard, the 5-action folder context menu wired to the 07-01/07-02 backend, a `DeleteFolderDialog` naming the affected page count N, and a grouped `TrashView` "Restore folder" row — all green against the 07-03 regression net with no new deps or tokens.**

## Performance

- **Duration:** ~10 min
- **Started:** 2026-06-21T13:26:10Z
- **Completed:** 2026-06-21T13:35:45Z
- **Tasks:** 3 implementation tasks committed atomically (Task 4 is a human-verify checkpoint — see Deferred Verification)
- **Files:** 9 (3 created, 6 modified)

## Accomplishments

- **`useTreeMutations.ts` (Task 1):** exports the pure helpers `applyMove`, `dropAllowed`, `parentOf`, `basename`, `destDir`, `countFolderPages`, plus the optimistic mutations `useTreeMove` (page+folder), `useFolderDelete`, and `useFolderRename`. `applyMove` removes the node from its old parent, rewrites its path (every descendant for a folder) by a literal prefix swap identical to server `relocateFolder`, and re-inserts under the destination — non-mutating so the rollback snapshot stays intact. 17 vitest cases pin the helper truth tables and the optimistic apply + rollback.
- **LeftTree folder DnD + menu + dialogs (Task 2):** folder rows are now draggable (`application/x-okf-folder`) and droppable for both page and folder drags; the drop-validity guard runs during `dragover` (reading the dragged path from a module-level `activeDragPath` ref because `dataTransfer.getData()` is empty mid-drag) and only highlights + `preventDefault`s when `dropAllowed` is true — an invalid drop leaves the row resting and shows the native `cursor:not-allowed`. The folder context menu grew to 5 actions (editor only; reader menu suppressed); Rename/Move open the `kind="folder"` dialogs from 07-03; Delete opens the new `DeleteFolderDialog`. The shipped `onSuccess`-invalidate `moveMut` was replaced by the optimistic `useTreeMove` for both page and folder moves, with a rollback banner.
- **`DeleteFolderDialog.tsx` (Task 2):** destructive confirm (backdrop never confirms) titled `Delete '{folder}'?` with the UI-SPEC body naming the page count N (`its 1 page` for N==1), confirm `Delete folder` / cancel `Keep folder`, optimistic prune + `["tree"]`/`["trash"]` reconcile, and a navigate-home guard when the open page lived inside the deleted folder.
- **Grouped `TrashView` (Task 3):** `buildRows` folds the flat listing by `delete_group_id` into one `Folder '{name}' · {N} pages` row per group (folder name derived from the common-ancestor `index.md` path) plus unchanged solo per-page rows, preserving newest-first order. The grouped row's primary `Restore folder` button calls `restoreFolderGroup` and invalidates `["tree"]`/`["trash"]`; the batched `Some pages already existed…` notice surfaces via the existing `role="status"` region when any restored path was auto-suffixed.

## Task Commits

1. **Task 1: useTreeMutations — optimistic move/delete + applyMove/dropAllowed** — `be26154` (feat)
2. **Task 2: Folder DnD + 5-action folder menu + DeleteFolderDialog in LeftTree** — `d968cdb` (feat)
3. **Task 3: Grouped TrashView folder row + Restore folder** — `c083e2a` (feat)

## Files Created/Modified

- `web/src/components/hooks/useTreeMutations.ts` — **Created.** Optimistic tree mutations + pure helpers.
- `web/src/components/hooks/useTreeMutations.test.tsx` — **Created.** 17 cases: helper truth tables + optimistic apply/rollback.
- `web/src/components/DeleteFolderDialog.tsx` — **Created.** Folder-delete confirm naming page count N.
- `web/src/components/LeftTree.tsx` — Folder DnD (draggable+droppable, guard), 5-action folder menu, folder dialogs, optimistic move.
- `web/src/components/LeftTree.css` — Token-only grab/grabbing cursor for draggable folder rows (no new tokens).
- `web/src/components/LeftTree.test.tsx` — Net-new folder-menu, folder DnD guard, optimistic folder-move, DeleteFolderDialog coverage.
- `web/src/components/TrashView.tsx` — Grouped folder row + Restore folder + batched collision notice.
- `web/src/components/TrashView.test.tsx` — Grouped render + Restore folder + batched notice tests; mocks restoreFolderGroup.
- `web/src/components/__regression__/treeBehaviors.test.tsx` — Inventory #6 updated to the net-new 5-action folder menu (see Deviations).

## Decisions Made

- **applyMove mirrors the server prefix swap exactly (Pitfall 6).** A literal `oldDir → newDir` rewrite over the moved node + every descendant path, pure/non-mutating, so the optimistic tree equals the eventual refetch and the node never visibly jumps on reconcile. A vitest asserts the rewritten descendant path.
- **Dragover-time guard via a module ref.** The HTML5 DnD spec makes `dataTransfer.getData()` return `""` during `dragover` (data is only readable on `drop`), but the affordance must be correct *during* drag. `activeDragPath` is set on `dragstart` and read by `dropAllowed` so the highlight/not-allowed decision is right before the drop.
- **Quiet invalid-drop affordance.** Invalid drops do not `preventDefault` (native `cursor:not-allowed` shows) and apply no highlight — UI-SPEC's non-colored rejection; destructive red is reserved for Delete only.
- **Reader folder menu suppressed.** Rather than opening an empty popup, the menu is not rendered when `menuItems` is empty for the target (mirrors the server RBAC gate).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Updated regression-net inventory #6 to the net-new 5-action folder menu**
- **Found during:** Task 2 (wiring the folder mutate actions)
- **Issue:** `treeBehaviors.test.tsx` inventory #6 pinned the OLD create-only folder menu and asserted `queryByRole("menuitem", { name: /^rename$/i })` is null. Adding the folder Rename/Move/Delete actions (the deliberate net-new feature of this plan) necessarily breaks that pin — it asserts the absence of a feature this plan adds, not a shipped behavior that must be preserved.
- **Fix:** Narrowed inventory #6 to assert only that the create-here actions are preserved (the actual regression contract); the net-new rename/move/delete coverage lives in `LeftTree.test.tsx`.
- **Files modified:** web/src/components/__regression__/treeBehaviors.test.tsx
- **Verification:** Full `npm test` (240 tests) green; the create-here regression assertions stay GREEN.
- **Committed in:** `d968cdb` (Task 2 commit)

---

**Total deviations:** 1 auto-fixed (1 bug). No architectural (Rule 4) changes; no auth gates. CLAUDE.md honored (native HTML5 DnD, no DnD library, token-only CSS, no new deps).

## Known Stubs

None. Every folder action is wired to a real 07-01/07-02 backend client function; no placeholder data or empty-value rendering introduced.

## Threat Flags

None. No new network endpoints or trust boundaries: the drag path is client-set but the server `relocateFolder`/`deleteFolder` re-validates (T-07-11/12/13/14 dispositions hold); folder names render as React text (no `dangerouslySetInnerHTML`); the grouped-restore id is the server-issued `delete_group_id`.

## Deferred Verification (Task 4 — checkpoint:human-verify)

Per the orchestrator's sequential-executor instruction, the perceptual checkpoint was NOT halted on; it is deferred to phase-level human verification (the verifier runs next). **All automated coverage is green** (`npm test` 240/240, `tsc --noEmit` clean, `npm run build` embeds the SPA); only perceptual DnD feel remains. Manual steps for the phase-level verifier:

1. Run the app, log in as an **editor**, open the workspace tree.
2. Right-click a folder → menu shows New page here / New folder here / Rename / Move / Delete; **Delete is the only red item**.
3. Drag a folder onto another folder → it moves as a unit and the tree updates **instantly** (before the network settles); drag onto the **root** drop zone → moves to top level; drag onto **itself or a child** → **cursor not-allowed, no highlight, nothing moves**.
4. Rename then Move a folder via the dialogs (keyboard path); rename/move onto an existing folder name → `A folder with that name already exists there…` and the dialog **stays open** (no silent merge).
5. Delete a folder → confirm dialog **names the page count N**; confirm → pages move to Trash. Open Trash → the grouped `Folder '{name}' · N pages` row → **Restore folder** → the structure reappears.
6. Right-click a row near the bottom/right viewport edge → the menu stays fully on-screen (4px clamp).
7. As a **reader**, confirm folders/pages show no mutate actions and rows are not draggable.
8. (Optional) Force a move failure (offline) → the tree **snaps back** and the rollback banner shows `We couldn't move that just now — it's back where it was…`.

## Self-Check: PASSED

- Created files present: `web/src/components/hooks/useTreeMutations.ts`, `web/src/components/hooks/useTreeMutations.test.tsx`, `web/src/components/DeleteFolderDialog.tsx`
- Task commits present: `be26154`, `d968cdb`, `c083e2a`
- `cd web && npm test` → 240 passed (26 files); `npx tsc --noEmit` clean; `npm run build` ok.

---
*Phase: 07-obsidian-style-file-tree-folder-operations-tree-ux*
*Completed: 2026-06-21*
