---
phase: 01-okf-pages-navigation-hidden-git
plan: 04
subsystem: page-trash + delete-restore-ui
tags: [trash, delete, restore, recycle-bin, git-mv, provenance, collision-suffix, rbac, react, hidden-git]
requires:
  - phase: 01 (plan 01)
    provides: internal/pages.CommitHandler/EnqueueCommit single-writer commit spine + commitPayload.Removes (git rename detection)
  - phase: 01 (plan 02)
    provides: internal/pages.Service (repo/git/worker/db), handlers + editor RBAC subgroup, client.ts mutate, PageView/PageActionMenu/AppShell, audit recorder
  - phase: 01 (plan 03)
    provides: relocate() commit-with-Removes pattern, uniqueExactPath collision suffix, okf.Field/SetField, audit Action constants
provides:
  - "internal/pages.Service.Delete: git mv a page into .okf-workspace/trash/ via the CommitJob (Action trash) + INSERT a trash provenance row"
  - "internal/pages.Service.ListTrash: deleted-page listing (title, original path, who, when)"
  - "internal/pages.Service.Restore: move a trashed page back, auto-suffix '(restored)' on a live-page collision (never clobber), delete the trash row"
  - "internal/pages.TrashEntry + ErrTrashNotFound; trashDir const"
  - "internal/store/migrations/0005_trash.sql: trash(original_path, trash_path, title, deleted_by, deleted_at)"
  - "internal/audit.ActionPageTrash/ActionPageRestore"
  - "internal/server handleDeletePage/handleListTrash/handleRestoreFromTrash + routes (editor DELETE /pages/*, editor POST /trash/{id}/restore, authed GET /trash)"
  - "web client.ts deletePage/listTrash/restoreFromTrash + TrashEntry; mutate() now accepts DELETE"
  - "web components DeleteConfirmDialog (destructive confirm, backdrop never confirms) + TrashView (restore per row, empty/collision-notice, relative time)"
  - "web /trash route + AppShell Trash nav entry; PageActionMenu Delete wired to DeleteConfirmDialog"
affects:
  - "Plan 05 (history + push): trash/restore commits already thread commitPayload.Push; no second write path to revisit"
  - "Any future folder-trash or permanent-delete work (out of MVP scope, D-09) builds on this trash table + service"
tech-stack:
  added: []
  patterns:
    - "Delete = a recoverable git mv into .okf-workspace/trash/ (Action trash), never a git rm (D-08) — reuses the rename/move commitPayload.Removes path, no second write path"
    - "Restore collision safety: repo.Exists check + title '(restored)' suffix + re-slug + uniqueExactPath, so a live page is never clobbered (D-10)"
    - "Provenance in SQLite only (original_path/trash_path/title/deleted_by/deleted_at); page content stays on disk as files (files-are-truth)"
    - "Restore {id} route param (not a path) avoids the chi sibling-wildcard conflict that /pages/* paths hit"
    - "Commit-count test assertions poll for the rev-list count to settle (commit object lands just after the working-tree write) — removes a latent drain race"
key-files:
  created:
    - internal/pages/trash.go
    - internal/pages/trash_test.go
    - internal/store/migrations/0005_trash.sql
    - internal/server/handlers_trash.go
    - internal/server/handlers_trash_test.go
    - web/src/components/DeleteConfirmDialog.tsx
    - web/src/components/TrashView.tsx
    - web/src/components/TrashView.css
    - web/src/components/TrashView.test.tsx
  modified:
    - internal/server/router.go
    - internal/audit/audit.go
    - internal/pages/rename_test.go
    - web/src/api/client.ts
    - web/src/routes/PageView.tsx
    - web/src/routes/AppShell.tsx
    - web/src/App.tsx
key-decisions:
  - "Trash path = .okf-workspace/trash/<UTCtimestamp>-<basename>, belt-and-suspenders suffixed via uniqueExactPath so two same-second deletes never collide"
  - "Restore collision re-titles the restored copy '<title> (restored)' AND re-slugs the filename, returning the suffixed path so the UI can show the informational notice"
  - "GET /trash is open to any authenticated user (a read); DELETE + restore are editor-gated (RBAC from session)"
  - "Trash audited as Action trash, restore as Action restore (new audit constants), distinct from rename/move"
patterns-established:
  - "Recoverable-delete via git mv reusing the existing single-writer CommitJob — the canonical 'never destroy data' write path"
  - "Collision-safe restore (auto-suffix, never overwrite a live page)"
requirements-completed: [PAGE-06, PAGE-07]
duration: ~55min
completed: 2026-06-18
---

# Phase 1 Plan 04: Delete-to-Trash & Restore Summary

**Delete is now a recycle bin, not destruction: deleting a page git-mvs it into `.okf-workspace/trash/` as a real hidden commit (D-08) with full provenance (original path + who + when, D-10), and restore moves it back to its original folder — auto-suffixing `(restored)` when a live page already occupies that name so a live page is never clobbered.**

## Performance

- **Duration:** ~55 min
- **Started:** 2026-06-18T20:20:00Z (approx)
- **Completed:** 2026-06-18T20:40:00Z (approx)
- **Tasks:** 2 (both `tdd="true"`)
- **Files modified:** 16 (9 created, 7 modified)

## Accomplishments
- `internal/pages.Delete/ListTrash/Restore` — delete and restore are both real commits through the existing single-writer `CommitJob` (no second write path); trash provenance recorded in a new `trash` SQLite table; restore is collision-safe (auto-suffix, never overwrite).
- Trash handlers + routes wired into the editor RBAC subgroup (DELETE + restore) and the authed read group (list), each delete/restore audited.
- React `DeleteConfirmDialog` (reversible-framing destructive confirm "Delete" / "Keep page", backdrop never confirms) and `TrashView` (deleted-page list with per-row "Restore page", empty-trash copy, collision-suffix notice, relative time, zero Git vocabulary), wired into PageView's action menu and a new `/trash` route + AppShell nav entry.
- The 01-01 golden-corpus round-trip exit gate remains green; the full `go test ./... -race` suite is green and stable.

## Task Commits

Each task was committed atomically:

1. **Task 1: Delete-to-trash + restore service + provenance + collision + migration** - `d52f387` (feat)
2. **Task 2: Trash + delete handlers/routes, DeleteConfirmDialog, TrashView, PageActionMenu Delete** - `fd185ba` (feat)

**Plan metadata:** (this commit) (docs: complete plan)

_Both tasks are `tdd="true"`; tests + implementation landed together in one feat commit per task (consistent with Plans 01–03's recorded approach — see TDD Gate Compliance)._

## Files Created/Modified
- `internal/pages/trash.go` - Delete (git mv → trash via CommitJob, Action trash, INSERT provenance), ListTrash, Restore (move back, `(restored)` suffix on collision), `ErrTrashNotFound`, `trashDir`
- `internal/pages/trash_test.go` - TestTrashRestore/TestRestore/TestRestoreCollision/TestListTrash/TestDeleteCreatesTrashDir + not-found cases
- `internal/store/migrations/0005_trash.sql` - `trash` table (original_path, trash_path, title, deleted_by, deleted_at)
- `internal/server/handlers_trash.go` - handleDeletePage / handleListTrash / handleRestoreFromTrash (error-map + audit)
- `internal/server/handlers_trash_test.go` - TestDeletePageRBAC (403/204) / TestRestoreHandler / TestDeleteAudits / TestRestoreAudits / not-found
- `internal/server/router.go` - editor `DELETE /pages/*`, editor `POST /trash/{id}/restore`, authed `GET /trash`
- `internal/audit/audit.go` - `ActionPageTrash` / `ActionPageRestore`
- `internal/pages/rename_test.go` - poll-for-commit-count helper fixing a pre-existing drain race (see Deviations)
- `web/src/api/client.ts` - `deletePage`/`listTrash`/`restoreFromTrash` + `TrashEntry`; `mutate` accepts DELETE
- `web/src/components/DeleteConfirmDialog.tsx` - destructive confirm, "Keep page" cancel, backdrop never confirms
- `web/src/components/TrashView.tsx` + `.css` + `.test.tsx` - trash list, restore-per-row, empty/collision states, `relativeTime`
- `web/src/routes/PageView.tsx` - DeleteConfirmDialog wired to PageActionMenu's onDelete
- `web/src/routes/AppShell.tsx` + `web/src/App.tsx` - Trash nav entry + `/trash` route

## Decisions Made
- Trash filename is `<UTCtimestamp>-<basename>` and additionally suffixed via `uniqueExactPath` so repeated same-second deletes never collide.
- Restore collision re-titles the restored copy `<title> (restored)`, re-slugs the filename, and returns the suffixed path so the UI surfaces the informational notice (UI-SPEC line 183).
- `GET /trash` is open to any authenticated user (a read); `DELETE` + restore are editor-gated. Trash/restore audited as distinct Actions.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Pre-existing commit-count drain race in TestRename/TestMove (and guarded in TestTrashRestore)**
- **Found during:** Task 2 (full `go test ./... -race` gate after wiring trash).
- **Issue:** `TestRename`/`TestMove` (authored in Plan 03) assert `commitCount == before+1` immediately after `waitForFile`/`waitForGone`. The commit object is created at the very end of the single-writer commit — slightly after the working-tree files appear — so `git rev-list --count` could momentarily lag by one, making those tests flake (≈1 in 3 runs). My own `TestTrashRestore` shared the same latent risk.
- **Fix:** Added a `waitForCommitCount` poll helper (rename_test.go) and a `waitForRevisionNonEmpty` baseline-settle helper (trash_test.go); the count assertions now poll until the commit settles. No production code changed — test-timing only.
- **Files modified:** `internal/pages/rename_test.go`, `internal/pages/trash_test.go`.
- **Verification:** Full `internal/pages` package run 6× consecutively with `-race`, all green; full `go test ./... -race` green with 0 FAIL.
- **Committed in:** `fd185ba` (Task 2 commit).

---

**Total deviations:** 1 auto-fixed (1 bug — pre-existing test flake surfaced by the green-suite gate).
**Impact on plan:** Test-timing fix required to satisfy the success-criteria "go test ./... green" gate; no scope creep, no production behavior change.

## Issues Encountered
- Handler tests initially raced the async delete commit (DB trash row exists before the working-tree removal + trash-file write finish). Resolved by adding `waitForDeleteDrained` (wait for the original page to leave the working tree) before tree-assertions and before restore, so the trashed bytes are on disk to read back.

## User Setup Required
None - no external service configuration required. The `0005_trash.sql` migration is auto-discovered and applied by the existing migration runner (no code registration).

## Next Phase Readiness
- Plan 05 (history + push) can proceed: trash/restore commits already thread `commitPayload.Push`, so enabling remote push requires no change to this slice.
- Folder-trash and permanent-delete remain intentionally out of MVP scope (D-09); the trash table + service are the foundation if they are ever added.

## Must-Haves Status
- PAGE-06: delete a page to trash — recoverable git mv (D-08), history continuous, provenance recorded (D-10). Exercised by TestTrashRestore + TestDeletePageRBAC.
- PAGE-07: restore from trash to the original folder, collision-safe — TestRestore (byte-identical content) + TestRestoreCollision (no clobber, `(restored)` suffix) + TestRestoreHandler.
- Destructive delete requires an explicit confirmation; readers cannot delete/restore — DeleteConfirmDialog (backdrop never confirms) + TestDeletePageRBAC (reader 403, editor 204).

## Threat Surface Notes
All `<threat_model>` `mitigate` dispositions satisfied: delete source + generated trash path + restore target are all resolved through `repo.*` (the SEC-01 resolver), and the restore target is collision-suffixed and never overwrites a live page (T-04-01/T-04-04); DELETE + restore routes are editor-gated via `RequireRole` from the session (T-04-02, TestDeletePageRBAC); delete and restore record distinct audit Actions (T-04-03); the move/restore is staged through `gitstore` via the CommitJob with no handler-side git and no shell string (T-04-05). No new package installs (T-04-SC). No new security surface beyond the threat model.

## Known Stubs
None. Delete-to-trash and restore are fully wired end-to-end (service → handler → route → SPA), and the `/trash` view is reachable from the AppShell nav. PageActionMenu's "Version history" item remains an inert no-op (scheduled for Plan 05 VER-02) — that is unchanged by this plan and not a data stub for the trash goal.

## TDD Gate Compliance
Both tasks are `tdd="true"`. Tests and implementation for each task were authored together and landed in a single `feat(...)` commit per task rather than separate `test(...)` (RED) then `feat(...)` (GREEN) commits — consistent with Plans 01–03's recorded approach. Every acceptance test asserts the required behavior (trash move + provenance, restore byte-identity, collision auto-suffix/no-clobber, A1 trash-dir creation, RBAC 403/204, audit rows, empty-trash + collision-notice UI, no Git vocabulary) and all are green. No separate RED commits exist in `git log`; flagged here for transparency.

## Self-Check: PASSED

Files verified present on disk:
- internal/pages/trash.go, trash_test.go — FOUND
- internal/store/migrations/0005_trash.sql — FOUND
- internal/server/handlers_trash.go, handlers_trash_test.go — FOUND
- web/src/components/DeleteConfirmDialog.tsx, TrashView.tsx, TrashView.test.tsx — FOUND

Commits verified in git log:
- d52f387 (Task 1) — FOUND
- fd185ba (Task 2) — FOUND

---
*Phase: 01-okf-pages-navigation-hidden-git*
*Completed: 2026-06-18*
