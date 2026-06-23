---
phase: 07-obsidian-style-file-tree-folder-operations-tree-ux
plan: 01
subsystem: api
tags: [go, chi, git, okf, bleve, folder-operations, atomic-commit, link-rewrite, react, typescript]

# Dependency graph
requires:
  - phase: 01
    provides: "single-page relocate (delete-old + write-new + inbound link rewrite), the okf round-trip-safe structural link rewriter, the single-writer commit worker, the /pages/* POST catch-all suffix-dispatch pattern"
provides:
  - "pages.relocateFolder — atomic one-commit folder relocate (index.md + every dir/ descendant + all inbound links)"
  - "pages.RenameFolder / pages.MoveFolder public service methods"
  - "pages.descendantPages folder enumeration"
  - "pages.ErrFolderExists collision sentinel (HTTP 409, no silent merge)"
  - "okf.RewriteLinksMoved — link rewrite that resolves from old dir and emits from new dir (cross-linked moving siblings)"
  - "POST /pages/{dir}/rename-folder and /pages/{dir}/move-folder routes (editor-gated, path-safe)"
  - "client.ts renameFolder / moveFolder API functions"
affects: [07-02 grouped-trash folder delete, 07-03 tree DnD, 07-04 context menu + dialogs]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Folder op = batch over a dir/index.md page + every dir/ descendant, relocated in ONE commit"
    - "Moved-page bytes written verbatim (never re-emitted through okf.Emit) to preserve byte-stability"
    - "Unified single-pass inbound link rewrite over the whole moves set (resolve from current dir, emit from final dir)"
    - "Folder collision REJECTS (ErrFolderExists -> 409) instead of auto-suffixing (folders are not pages)"

key-files:
  created: []
  modified:
    - internal/pages/rename.go
    - internal/pages/rename_test.go
    - internal/okf/links.go
    - internal/server/handlers_pages.go
    - internal/server/router.go
    - internal/audit/audit.go
    - web/src/api/client.ts

key-decisions:
  - "Added okf.RewriteLinksMoved (resolveDir for matching, emitDir for recomputation) so a moved page linking to a moved sibling rewrites once and correctly, instead of producing a stale ../ back-reference"
  - "Replaced per-move calls to the single-page rewriteInboundLinks with one unified rewriteFolderInboundLinks pass to eliminate Pitfall 1 double-staging by construction (each page keyed once by its final path)"
  - "RewriteLinksMoved treats a byte-identical recomputed destination as unchanged, so a moving page carrying only bare-sibling links is copied verbatim (byte-stability preserved)"
  - "move-folder permits new_parent=\"\" (root) and only runs cleanPathString when non-empty, so a root move is not rejected by the empty-path guard"

patterns-established:
  - "Folder relocate atomicity: collision precheck FIRST (before any payload/disk), then one commitPayload with all writes+removes+stagePaths"
  - "Each descendant stages new+old paths so git rename detection / --follow continuity holds per file"

requirements-completed: [TREE-02, TREE-06]

# Metrics
duration: 18min
completed: 2026-06-21
status: complete
---

# Phase 7 Plan 01: Backend Folder Rename/Move (Atomic Relocate) Summary

**Atomic folder rename/move that relocates a dir/index.md plus every descendant and rewrites all inbound links in ONE byte-stable commit, rejecting target-dir collisions with a clean 409 (ErrFolderExists) before any disk write.**

## Performance

- **Duration:** ~18 min
- **Completed:** 2026-06-21
- **Tasks:** 3
- **Files modified:** 7

## Accomplishments
- `relocateFolder` lifts the single-page Phase-1 `relocate` to a folder batch: index.md + every `dir/` descendant relocate together, with all inbound links to every moved page rewritten in the SAME commit (TREE-02).
- Collision precheck returns `ErrFolderExists` BEFORE building any payload or touching disk; folders never auto-suffix or silently merge (TREE-06), surfaced as HTTP 409 with the exact UI-SPEC copy.
- Cross-linked moving siblings (a→b and b→a) rewrite exactly once with no double-staging (Pitfall 1), via a new `okf.RewriteLinksMoved` and a unified single-pass repo scan keyed by each page's final path.
- Moved-page bytes are written verbatim (never re-emitted), keeping the okf golden round-trip green by construction.
- Folder rename/move wired on the existing `/pages/*` POST catch-all by suffix under the editor RBAC gate, with `dir` and `new_parent` re-validated via `cleanPathString` (WR-08); `renameFolder`/`moveFolder` added to the API client.

## Task Commits

1. **Task 1: Wave 0 — failing folder-relocate tests + ErrFolderExists sentinel** - `d08370e` (test, TDD RED)
2. **Task 2: Implement relocateFolder + RenameFolder/MoveFolder + descendantPages** - `f279e93` (feat, TDD GREEN)
3. **Task 3: Wire folder rename/move HTTP routes + client functions** - `bcfacc6` (feat)

## Files Created/Modified
- `internal/pages/rename.go` - Added `ErrFolderExists`, `descendantPages`, `relocateFolder`, `rewriteFolderInboundLinks`, `movedDestinations`, public `RenameFolder`/`MoveFolder`.
- `internal/pages/rename_test.go` - Added `TestRelocateFolder`, `TestMoveFolder`, `TestRelocateFolder_Collision`, `TestRelocateFolder_NoCorruption` + `seedFolderPage` helper.
- `internal/okf/links.go` - Added `RewriteLinksMoved` (separate resolve/emit dirs; byte-identical recompute is a no-op).
- `internal/server/handlers_pages.go` - Added `handleRenameFolder`/`handleMoveFolder` suffix branches, `renameFolderRequest`/`moveFolderRequest`, `writeFolderError` (409/404/400/500 mapping).
- `internal/server/router.go` - Comment noting folder rename/move share the `/pages/*` editor-gated catch-all.
- `internal/audit/audit.go` - Added `ActionFolderRename`, `ActionFolderMove`.
- `web/src/api/client.ts` - Added `renameFolder`, `moveFolder` (409 surfaced via `err.status`).

## Decisions Made
- **`okf.RewriteLinksMoved` (new, Rule 2 - missing critical correctness):** the plan referenced reusing `rewriteInboundLinks`, but that single-`fromDir` rewriter produces an incorrect `../oldfolder/...` back-reference when the LINKING page is itself moving (cross-linked siblings). Split the directory used for link resolution from the directory used to recompute the replacement path so a moved sibling link stays a correct bare reference.
- **Unified `rewriteFolderInboundLinks` single pass** instead of N per-move calls: scans each page once and keys results by final path, making Pitfall 1 double-staging impossible by construction rather than by post-hoc merge.
- **Byte-identical recompute treated as unchanged** in `RewriteLinksMoved`: a moving page whose only links are bare siblings (text unchanged) is not re-emitted, preserving byte-stability.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing Critical] Added okf.RewriteLinksMoved and a unified rewrite pass**
- **Found during:** Task 2 (GREEN) — `TestRelocateFolder_NoCorruption` failed: a moved sibling's cross-link was rewritten to `../guides/b.md` instead of staying `b.md`.
- **Issue:** The plan's prescribed reuse of `rewriteInboundLinks` computes the replacement relative path from the linking page's CURRENT directory, which is wrong when that page is itself being relocated — yielding a stale back-reference into the old folder name. The single-page rewriter cannot express "resolve here, emit there."
- **Fix:** Added `okf.RewriteLinksMoved(body, resolveDir, emitDir, oldRel, newRel)` and a unified `rewriteFolderInboundLinks` pass that applies every old→new mapping per page using the page's current dir for resolution and its final dir for emission. A byte-identical recompute is treated as no-change to keep verbatim byte-stability.
- **Files modified:** internal/okf/links.go, internal/pages/rename.go
- **Verification:** `TestRelocateFolder_NoCorruption` (cross-link rewrite + code-block byte-stability) passes; okf golden round-trip stays green.
- **Committed in:** `f279e93` (Task 2 commit)

---

**Total deviations:** 1 auto-fixed (1 missing critical correctness).
**Impact on plan:** The deviation is the correct realization of the plan's stated must-have ("A moved page that links to a sibling moved page lands a single correct write"). No scope creep — the new okf function is additive and the existing single-page `relocate`/`rewriteInboundLinks` path is untouched.

## Issues Encountered
- None beyond the cross-link rewrite correctness issue documented above (caught by the Wave-0 RED test, resolved in GREEN).

## User Setup Required
None - no external service configuration required. This plan installs ZERO new dependencies (no change to go.mod or package.json).

## Next Phase Readiness
- Backend atomic folder rename/move + collision rejection is the foundation for Plan 02 (grouped folder delete/trash) and Plans 03/04 (tree DnD, context menu, shared rename/move dialogs).
- `renameFolder`/`moveFolder` client functions are ready to wire into the rebuilt tree UX with optimistic cache updates.
- No blockers.

## Self-Check: PASSED

All created/modified files exist on disk and all three task commits (`d08370e`, `f279e93`, `bcfacc6`) are present in git history.

---
*Phase: 07-obsidian-style-file-tree-folder-operations-tree-ux*
*Completed: 2026-06-21*
