---
phase: 07-obsidian-style-file-tree-folder-operations-tree-ux
plan: 02
subsystem: api
tags: [go, sqlite, migration, trash, folder-operations, crypto-rand, react, typescript, chi]

# Dependency graph
requires:
  - phase: 07-01
    provides: "descendantPages folder enumeration; the /pages/* POST catch-all suffix-dispatch convention; the editor-gated router subgroup and client.ts editing convention"
  - phase: 01
    provides: "the per-page Delete/Restore trash machinery (git mv into .okf-workspace/trash, restoredAlternative collision auto-suffix), the single-writer commit worker, the SQLite migration runner"
provides:
  - "pages.DeleteFolder — per-page-looped folder delete under one shared opaque delete_group_id (TREE-04)"
  - "pages.RestoreGroup — grouped folder restore, index.md first, per-page collision auto-suffix (TREE-05)"
  - "pages.deleteWithGroup — shared delete body (empty group -> SQL NULL solo; non-empty -> grouped)"
  - "migration 0008_trash_group.sql — nullable delete_group_id TEXT on trash"
  - "TrashEntry.DeleteGroupID surfaced through ListTrash and the trash listing HTTP response"
  - "POST /pages/{dir}/delete-folder and POST /trash/group/{id}/restore (editor-gated)"
  - "client.ts deleteFolder / restoreFolderGroup; TrashEntry.delete_group_id"
affects: [07-03 tree DnD, 07-04 context menu + DeleteFolderDialog + TrashView grouped row]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Grouped trash = loop the EXISTING per-page Delete/Restore over a descendant set under one shared opaque delete_group_id (NOT one mega-commit; partial progress recoverable by group id, mirrors ReconcileTrash WR-01)"
    - "Nullable additive column (delete_group_id) so existing/solo rows read NULL -> empty string; per-page path byte-identical"
    - "Opaque group id from crypto/rand (stdlib), bound parameterized in INSERT/SELECT — never interpolated (SQLi guard)"
    - "Grouped restore orders index.md FIRST (isFolderIndex) so the folder exists before descendants land"

key-files:
  created:
    - internal/store/migrations/0008_trash_group.sql
  modified:
    - internal/pages/trash.go
    - internal/pages/trash_test.go
    - internal/store/store_test.go
    - internal/server/handlers_trash.go
    - internal/server/handlers_pages.go
    - internal/server/router.go
    - internal/audit/audit.go
    - web/src/api/client.ts

key-decisions:
  - "delete_group_id is a nullable, additive ALTER TABLE (no table redeclare, no index) — existing/solo deletes read NULL, no backfill; an index would be gold-plating at 5-user scale"
  - "DeleteFolder loops per-page deleteWithGroup (each page its own commit + restorable row) instead of one atomic commit — the RESOLVED atomicity decision; partial progress is coherent and restorable by group id"
  - "RestoreGroup reuses the EXISTING per-page Restore (so restoredAlternative collision auto-suffix applies per page automatically) and restores index.md first via isFolderIndex ordering"
  - "Grouped restore route is /trash/group/{id}/restore (NOT a /pages/* path) because the group id is an opaque non-path token — avoids the chi sibling-wildcard conflict"
  - "Empty group id binds as SQL NULL via sql.NullString so the solo per-page Delete row is byte-identical to before the refactor"

patterns-established:
  - "Folder-batch trash op = enumerate via descendantPages, generate one crypto/rand group id, loop the existing per-page primitive under it"
  - "Surface a nullable group column as the empty string at the service/HTTP boundary so the client groups only genuine folder-delete rows"

requirements-completed: [TREE-04, TREE-05]

# Metrics
duration: 7min
completed: 2026-06-21
status: complete
---

# Phase 7 Plan 02: Grouped Folder Delete-to-Trash and Grouped Restore Summary

**Grouped folder delete/restore layered on the existing per-page trash: a nullable `delete_group_id` (migration 0008) tags every row of one folder delete; `DeleteFolder` loops the existing per-page delete under one crypto/rand group id, and `RestoreGroup` loops the existing per-page restore index.md-first with per-page collision auto-suffix — per-page delete/restore stay byte-identical (group NULL).**

## Performance

- **Duration:** ~7 min
- **Started:** 2026-06-21T13:04:25Z
- **Completed:** 2026-06-21T13:10:40Z
- **Tasks:** 3 (Task 2 is TDD: RED then GREEN)
- **Files modified:** 8 (1 created, 7 modified)

## Accomplishments
- Migration `0008_trash_group.sql` adds a nullable, additive `delete_group_id TEXT` to `trash` (existing/solo rows read NULL — no backfill); asserted via `TestMigrateTrashGroupColumn` (column nullable + schema version 8 recorded).
- `DeleteFolder` trashes a folder's index.md + every descendant under ONE shared opaque group id by looping the existing per-page `deleteWithGroup` (TREE-04); partial progress is coherent and restorable by group id (RESOLVED atomicity decision, mirrors ReconcileTrash WR-01).
- `RestoreGroup` restores the whole group index.md-first, reusing the existing per-page `Restore` so the `restoredAlternative` collision auto-suffix applies per page automatically — a live page is never clobbered (TREE-05).
- Solo per-page `Delete`/`Restore` are unchanged: empty group id binds as SQL NULL via `sql.NullString` (`TestSoloDeleteStoresNullGroup` + all existing trash tests still green).
- Wired editor-gated `POST /pages/{dir}/delete-folder` (suffix dispatch, cleanPathString SEC-01) and `POST /trash/group/{id}/restore` (opaque non-path id, validated, parameterized); `delete_group_id` surfaced in the trash listing; `deleteFolder`/`restoreFolderGroup` added to the API client.

## Task Commits

Each task was committed atomically:

1. **Task 1: Migration 0008 — nullable delete_group_id + column assertion** - `a429729` (feat)
2. **Task 2 (RED): failing DeleteFolder/RestoreGroup/deleteWithGroup tests** - `31df93c` (test, TDD RED)
3. **Task 2 (GREEN): DeleteFolder + RestoreGroup + deleteWithGroup + grouped ListTrash** - `d617dc2` (feat, TDD GREEN)
4. **Task 3: wire folder-delete + grouped-restore routes + client functions** - `5296d10` (feat)

_Note: Task 2 is a TDD task (RED commit `31df93c` then GREEN commit `d617dc2`)._

## Files Created/Modified
- `internal/store/migrations/0008_trash_group.sql` - **Created.** `ALTER TABLE trash ADD COLUMN delete_group_id TEXT` (nullable, additive; header comment mirrors 0005).
- `internal/store/store_test.go` - Added `TestMigrateTrashGroupColumn` (PRAGMA table_info asserts the nullable column; schema_migrations records version 8).
- `internal/pages/trash.go` - Added `DeleteGroupID` to `TrashEntry`; refactored `Delete` -> `deleteWithGroup` (empty group -> SQL NULL); added `DeleteFolder`, `RestoreGroup`, `newDeleteGroupID` (crypto/rand), `isFolderIndex`; `ListTrash` now selects/scans `delete_group_id`.
- `internal/pages/trash_test.go` - Added `TestDeleteFolder`, `TestRestoreGroup`, `TestDeleteFolder_PartialProgress`, `TestSoloDeleteStoresNullGroup` + `seedFolderForDelete`/`trashRowsByGroup` helpers.
- `internal/server/handlers_trash.go` - `trashEntryResponse.DeleteGroupID` (copied in `handleListTrash`); `restoreGroupResponse`; `handleRestoreFolderGroup` (validate non-empty/NUL id, 404 on ErrTrashNotFound, audit ActionFolderRestore).
- `internal/server/handlers_pages.go` - `/delete-folder` suffix branch in `handleRenamePage`; `handleDeleteFolder` (cleanPathString, audit ActionFolderTrash, 204).
- `internal/server/router.go` - `editor.Post("/trash/group/{id}/restore", h.handleRestoreFolderGroup)` next to the per-page restore route.
- `internal/audit/audit.go` - Added `ActionFolderTrash` and `ActionFolderRestore`.
- `web/src/api/client.ts` - Added `deleteFolder`, `restoreFolderGroup`; `delete_group_id` on the `TrashEntry` interface.

## Decisions Made
- **Nullable additive ALTER** (no table redeclare, no index): existing rows and every future solo delete read NULL, no backfill; an equality-only index is gold-plating at 5-user scale.
- **Per-page-looped DeleteFolder** (each page its own commit + restorable trash row) rather than one atomic mega-commit — the RESOLVED atomicity decision; a mid-loop failure leaves the trashed rows coherent under the shared group id, still restorable as a group.
- **RestoreGroup reuses per-page Restore** so `restoredAlternative` per-page collision suffixing applies automatically; index.md is restored first via `isFolderIndex` stable ordering so the folder exists before descendants.
- **Non-path grouped-restore route** `/trash/group/{id}/restore` — the opaque group id avoids the chi sibling-wildcard conflict the `/pages/*` catch-all hits.
- **All group-id SQL parameterized** (`?` binds via `sql.NullString` on INSERT, plain `?` on SELECT) — never string-interpolated (T-07-05 SQLi guard).

## Deviations from Plan

None - plan executed exactly as written. (The plan permitted either a `restored []string` or a `collided bool` on the restore response; chose `restoreGroupResponse{paths: []string}` and mirrored it as `restoreFolderGroup(): { paths: string[] }` in the client, exactly as the action note offered.)

## Issues Encountered
- An early draft of the new test helper referenced a non-existent `*repoForTrashTests` type and omitted `database/sql`/`internal/repo` imports; corrected to the real `*repo.Repo` fixture type before the RED commit. No impact on behavior.

## Threat Surface Scan
All new surface is covered by the plan's threat register: the grouped-restore `{id}` and folder `dir` are bound/validated (T-07-05 parameterized, T-07-07 cleanPathString + descendantPages repo-root walk), both routes live in the `editor` RBAC subgroup (T-07-06). No new endpoints, auth paths, or trust-boundary schema beyond the planned `delete_group_id` column. No threat flags.

## Known Stubs
None — DeleteFolder/RestoreGroup are fully wired end-to-end (migration -> service -> HTTP -> client). The frontend DeleteFolderDialog + grouped TrashView row that consume `deleteFolder`/`restoreFolderGroup`/`delete_group_id` are Plan 04's scope, as designed.

## User Setup Required
None - no external service configuration required. This plan installs ZERO new dependencies (no change to go.mod or package.json); the group id uses stdlib crypto/rand.

## Next Phase Readiness
- Backend grouped folder delete/restore + the `delete_group_id` listing field are ready for Plan 04's DeleteFolderDialog and grouped TrashView row.
- `deleteFolder`/`restoreFolderGroup` client functions are ready to wire into the tree context menu / Trash view with optimistic cache updates.
- No blockers.

## Self-Check: PASSED

---
*Phase: 07-obsidian-style-file-tree-folder-operations-tree-ux*
*Completed: 2026-06-21*
