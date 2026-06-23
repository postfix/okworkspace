# Phase 7: Obsidian-style File Tree (folder operations & tree UX) - Pattern Map

**Mapped:** 2026-06-21
**Files analyzed:** 14 (8 backend, 6 frontend)
**Analogs found:** 14 / 14 (this is a codebase-EXTENSION + clean-REBUILD phase — the analog for every file is the file itself or its sibling, already shipped and tested)

> Nature of this phase: there is almost no greenfield. Backend files EXTEND existing single-page machinery to a folder batch; frontend components are REBUILT in place preserving every shipped behavior. The "analog" for each is therefore the current source, captured below with line ranges so the executor mirrors the established pattern exactly.

---

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|-------------------|------|-----------|----------------|---------------|
| `internal/pages/rename.go` (add `relocateFolder`/`RenameFolder`/`MoveFolder`/`ErrFolderExists`) | service | transform (batch relocate + 1 commit) | itself: `relocate` (rename.go:88-140) + `rewriteInboundLinks` (149-211) | exact (extend) |
| `internal/pages/trash.go` (add `DeleteFolder`/`RestoreGroup`/`deleteWithGroup`/group-id) | service | CRUD + event (commit) | itself: `Delete` (trash.go:45-118) + `Restore` (250-309) | exact (extend) |
| `internal/store/migrations/0008_trash_group.sql` (new) | migration | DDL | `migrations/0005_trash.sql` | exact (same family) |
| `internal/server/handlers_pages.go` (folder rename/move dispatch) | handler | request-response | `handleRenamePage` (handlers_pages.go:223-312) + `handleGetPageOrHistory` suffix-dispatch (99-124) | exact (extend) |
| `internal/server/handlers_trash.go` (grouped restore endpoint) | handler | request-response | `handleRestoreFromTrash` (handlers_trash.go:91-119) | exact (extend) |
| `internal/server/router.go` (register folder + grouped-restore routes) | route | request-response | editor subgroup (router.go:138-167) | exact (extend) |
| `internal/pages/rename_test.go` (folder relocate/collision/no-corruption) | test | — | `rename_test.go` helpers + `service_test.go:newServiceFixture` | exact |
| `internal/pages/trash_test.go` (DeleteFolder/RestoreGroup/PartialProgress) | test | — | same fixture helpers | exact |
| `web/src/components/LeftTree.tsx` (REBUILD: folder DnD + optimistic) | component | event-driven (DnD) | itself: LeftTree.tsx (whole file) | exact (rebuild) |
| `web/src/components/TreeContextMenu.tsx` (REBUILD: a11y preserved) | component | event-driven | itself: TreeContextMenu.tsx (whole file) | exact (rebuild) |
| `web/src/components/MoveDialog.tsx` (REBUILD: page+folder) | component | request-response | itself: MoveDialog.tsx | exact (rebuild) |
| `web/src/components/RenameModal.tsx` (REBUILD: page+folder) | component | request-response | itself: RenameModal.tsx | exact (rebuild) |
| `web/src/components/DeleteFolderDialog.tsx` (new) | component | request-response | `RenameModal`/`MoveDialog` Dialog shell + existing `DeleteConfirmDialog` | role-match |
| `web/src/components/TrashView.tsx` (EXTEND: grouped row) | component | CRUD | itself: TrashView.tsx | exact (extend) |
| `web/src/api/client.ts` (add `moveFolder`/`renameFolder`/`deleteFolder`/`restoreFolderGroup`) | utility | request-response | `movePage`/`renamePage`/`deletePage`/`restoreFromTrash` (client.ts:263-319) | exact (extend) |

---

## Pattern Assignments

### `internal/pages/rename.go` — add `relocateFolder` (service, batch transform)

**Analog:** the single-page `relocate` in the SAME file. Lift it to a descendant batch folded into ONE `commitPayload`.

**Core single-commit relocate pattern to mirror** (`internal/pages/rename.go:92-140`): reads `srcBytes`, builds `writes := []fileWrite{{Path:newPath,Bytes:srcBytes}}`, stages BOTH new+old (`stagePaths := []string{newPath, oldPath}`) for git rename detection, appends `rewriteInboundLinks` output, enqueues ONE `commitPayload{Writes, Removes:[]string{oldPath}, Spec:gitstore.CommitSpec{...}, Push:s.pushOnCommit}` via `EnqueueCommit`, then fires `enqueueIndexDelete(oldPath)` + `enqueueIndexUpsert(newPath)` + upsert for each rewritten path.

**What to preserve/mirror:**
- Move writes the page's EXISTING bytes verbatim (never re-emit through `okf.Emit`) — `srcBytes` read at line 93, written unchanged. This is the byte-stability invariant.
- Inbound-link rewrite goes ONLY through `rewriteInboundLinks` → `okf.Parse`/`okf.RewriteLinks`/`doc.Emit` (rename.go:149-211), never an AST/regex.
- Stage both old+new per moved page so `git log --follow` continuity holds per descendant.
- Collision: folder move must REJECT (new `ErrFolderExists`, modeled on the `ErrRenameCollision` sentinel at rename.go:16-19) — do NOT reuse `uniqueExactPath` auto-suffix (page-only).

**Descendant enumeration** — mirror the `filepath.WalkDir` + `isSkippedDir` + `filepath.ToSlash` + `.md`-suffix walk already in `rewriteInboundLinks` (rename.go:154-206); collect `slashRel == dir+"/index.md" || strings.HasPrefix(slashRel, dir+"/")`.

**Pitfall to encode (from RESEARCH §Pitfall 1):** merge `writes` by path (last-wins) so a moved page that also gets an inbound-link rewrite stages once. Build a `map[string][]byte` keyed by new path: apply moves first, then apply rewrites on top.

---

### `internal/pages/trash.go` — add `DeleteFolder` + `RestoreGroup` (service, CRUD + commit)

**Analog:** `Delete` (trash.go:45-118) and `Restore` (trash.go:250-309) in the SAME file.

**Delete row-INSERT shape to extend** (`internal/pages/trash.go:110-114`):
```go
_, err = s.db.ExecContext(ctx,
    `INSERT INTO trash (original_path, trash_path, title, deleted_by, deleted_at)
     VALUES (?, ?, ?, ?, ?)`,
    pagePath, trashPath, title, user, s.now().UTC().Format(time.RFC3339))
```
Add a `delete_group_id` bind param (a `deleteWithGroup` internal variant). `DeleteFolder` loops `descendantPages(dir)` calling `deleteWithGroup` with ONE shared group id.

**Restore pattern to reuse per-row** (`internal/pages/trash.go:250-309`): `RestoreGroup(groupID)` SELECTs `id FROM trash WHERE delete_group_id = ?` (parameterized bind — never interpolate), then iterates the EXISTING `Restore(id)` so `restoredAlternative` collision-suffixing (trash.go:316-350) applies per page automatically. Restore `index.md` first so the folder exists (RESEARCH Open Question 2).

**What to preserve/mirror:**
- The delete = git-mv model: write to `trashDir`, remove the old path in ONE commit (trash.go:81-92), Action `"trash"`.
- Parameterized `?` binds throughout (SQL-injection guard, RESEARCH Security Domain).
- Partial-progress is ACCEPTED (CONTEXT atomicity resolution + `ReconcileTrash` WR-01 stance at trash.go:159-176) — recoverable by the shared group id. `TestDeleteFolder_PartialProgress` asserts graceful behavior.

---

### `internal/store/migrations/0008_trash_group.sql` (new migration)

**Analog:** `internal/store/migrations/0005_trash.sql` (the table this migrates).

**Format to mirror** (header comment block explaining operational-data-only + the `CREATE TABLE IF NOT EXISTS trash (...)` body). The runner is the embedded ordered `NNNN_name.sql` applier (`migrations.go:13-40`, `//go:embed migrations/*.sql`, applied idempotently in `schema_migrations`). The new file is additive:
```sql
-- 0008_trash_group: group id linking pages trashed by one folder-delete op so the
-- whole folder restores as a unit (TREE-05). NULL for solo per-page deletes
-- (backward-compatible: existing rows read as solo deletes, no backfill needed).
ALTER TABLE trash ADD COLUMN delete_group_id TEXT;
```
The fixture (`newServiceFixture` → `st.Migrate`) applies it transitively; assert the column exists.

---

### `internal/server/handlers_pages.go` — folder rename/move dispatch (handler)

**Analog:** `handleRenamePage` (handlers_pages.go:223-312) + the suffix-dispatch idiom in `handleGetPageOrHistory` (99-124).

**Suffix-dispatch pattern to mirror** (handlers_pages.go:228-251): the route is `POST /pages/*`; the wildcard suffix (`/rename`, `/restore`) selects the action because chi forbids a sibling `{path:.*}` node next to the catch-all. Per RESEARCH Pitfall 5 / Assumption A4, add `/rename-folder` (or `/move-folder`) suffix handling in the SAME dispatcher, distinguishing a folder target. Reuse:
- `cleanPathString` (handlers_pages.go:84-97) on the stripped path AND on the attacker-controlled `new_parent` (WR-08, mirror line 286).
- The exactly-one-of discriminant for title-vs-parent (lines 265-270) if folder rename/move share a body.
- Map `ErrFolderExists` → HTTP 409 with UI-SPEC copy "A folder with that name already exists there…".
- `h.audit.Record` with a folder action (mirror lines 305-310).

---

### `internal/server/handlers_trash.go` — grouped restore endpoint (handler)

**Analog:** `handleRestoreFromTrash` (handlers_trash.go:91-119).

**Pattern to mirror:** parse the group id from the path param (`chi.URLParam(r,"id")` style, lines 96-101 — validate it; group id is not a path so no wildcard conflict), call `h.pages.RestoreGroup`, map `ErrTrashNotFound`→404, audit, return the restored-paths response (extend `restoreResponse` for the batched collision notice). Reuse the `trashEntryResponse` shape (handlers_trash.go:17-23) — extend `ListTrash`/response to carry `delete_group_id` so the client can group.

---

### `internal/server/router.go` — register routes (route)

**Analog:** the editor subgroup (router.go:138-167).

**Pattern to mirror:** new folder mutations register on the SAME `editor.Post("/pages/*", ...)` catch-all (line 148) — do NOT add a sibling `{path:.*}` route (the 405 trap documented inline at lines 142-148). Grouped restore mirrors the working `editor.Post("/trash/{id}/restore", ...)` (line 166) → `editor.Post("/trash/group/{id}/restore", h.handleRestoreFolderGroup)` (a non-path `{id}`, no wildcard conflict). All folder ops stay inside the `auth.RequireRole(auth.RoleEditor)` gate (line 139) — RBAC from session, never client input.

---

### `internal/pages/rename_test.go` + `trash_test.go` — Go tests

**Analog:** the fixture + drain-aware helpers already in these files.

**Helpers to reuse (do NOT hand-roll waits):**
- `newServiceFixture(t, pushOnCommit)` (`service_test.go:89-112`) — real repo+git+worker+migrated SQLite; deterministic clock.
- `waitForFile(t, r, path)` (`service_test.go:115-125`).
- `waitForGone(t, svc, path)` (`rename_test.go:13-23`).
- `waitForRevisionChange` (`rename_test.go:29-40`), `waitForCommitCount(t, root, want)` (`rename_test.go:46-58`), `commitCount` (`commitjob_test.go:44`).

**Tests to add (Wave 0):** `TestRelocateFolder` (one commit — assert via `waitForCommitCount`), `TestMoveFolder`, `TestRelocateFolder_Collision` (409, no disk touch), `TestRelocateFolder_NoCorruption` (cross-linked descendants — Pitfall 1), `TestDeleteFolder` (group id on rows), `TestRestoreGroup` (structure recreated, index.md first, batched suffix), `TestDeleteFolder_PartialProgress`.

---

### `web/src/components/LeftTree.tsx` — REBUILD (component, event-driven DnD)

**Analog:** the current LeftTree.tsx (whole file) — the rebuild MUST reproduce every behavior in the Clean-Rebuild Behavior Inventory (RESEARCH §, 14 items).

**Native DnD `dataTransfer` pattern to extend** (LeftTree.tsx:409-412, 350-364, 236-250):
```tsx
onDragStart={(e) => { e.dataTransfer.setData("application/x-okf-page", node.path);
                      e.dataTransfer.effectAllowed = "move"; }}
// drop target: if (e.dataTransfer.types.includes("application/x-okf-page")) { e.preventDefault(); setDropActive(true); }
```
Add a second key `application/x-okf-folder` carrying the folder path; both are valid drop payloads on a folder row / root zone.

**Behaviors to preserve exactly:** `nodes = data ?? []` null-coalesce (line 89), `parentOf` same-parent no-op guard (lines 30-33, 119), `forwardRef`/`useImperativeHandle` `LeftTreeHandle` (38-41, 97-105), `canEdit` RBAC (84), `.lefttree-droptarget` highlight (233, 344), active-row `navrow-active`+`aria-current` (404-406), loading/error states (195-208), the `menuItems(target)` folder/page/root branch (132-193).

**Net-new (RESEARCH §Folder context-menu items + §Native folder DnD):** the 3 folder mutate actions (Rename/Move/Delete) added to the folder branch (currently create-only at lines 133-150); `dropAllowed(dragKind, dragPath, targetFolder)` prefix guard (folder onto self/descendant/same-parent → no highlight, `cursor:not-allowed`).

**Optimistic mutation (replaces the `onSuccess`-invalidate `moveMut` at lines 107-114):** TanStack Query `onMutate` snapshot `["tree"]` → `applyMove` pure transform → `onError` rollback → `onSettled` invalidate. Recommended into a `hooks/useTreeMutations.ts`. `applyMove` MUST do a literal prefix swap identical to server `relocateFolder` (RESEARCH Pitfall 6).

---

### `web/src/components/TreeContextMenu.tsx` — REBUILD (component, a11y preserved)

**Analog:** the current TreeContextMenu.tsx (whole file). The `TreeContextMenuItem` contract (`{label, onSelect, danger?}`, lines 7-11) and ALL a11y behavior must survive the rebuild verbatim: `role="menu"`/`role="menuitem"`, open-focus-first-item (58-64), Arrow/Home/End/Enter/Space/Esc + Tab-trap (99-129), outside-click/scroll(capture)/resize close (67-86), 4px viewport clamp (41-55), focus restore on close (61-63), `treemenu-item-danger` (154). Items array is parameterized folder(5)/page(4)/root(2) by the caller — no logic change needed inside the menu.

---

### `web/src/components/RenameModal.tsx` + `MoveDialog.tsx` — REBUILD (page + folder)

**Analogs:** current RenameModal.tsx and MoveDialog.tsx.

**RenameModal pattern to mirror:** `Dialog` shell (RenameModal.tsx:51-80), empty-title client validation (42-49), mutation `onSuccess` invalidate `["tree"]`+`["page"]` + close + navigate (29-40). Parameterize by node `kind` ("page"|"folder") to call `renameFolder` vs `renamePage`; folder copy: title "Rename folder", help "Pages inside this folder, and links to them, will keep working." (UI-SPEC).

**MoveDialog pattern to mirror:** `collectFolders` tree-flatten → `<select>` with `"Top level"` `""` option (MoveDialog.tsx:16-28, 77-89), `movePage` mutation (47-58). Parameterize to call `moveFolder`; folder copy: title "Move folder", confirm "Move folder", help "Choose where this folder should live." Collision 409 surfaces as a non-fatal `role="alert"` field error (dialog stays open — UI-SPEC §Folder operations).

---

### `web/src/components/DeleteFolderDialog.tsx` (new)

**Analog:** RenameModal/MoveDialog `Dialog` shell + the shipped `DeleteConfirmDialog` (page delete). Destructive confirm, backdrop NEVER confirms (existing `Dialog` `destructive` contract), invalidate `["tree"]`+`["trash"]`. Copy (UI-SPEC): title `Delete '{folderName}'?`, body `This folder and its {N} pages will move to Trash…` (name the count N; N==1 → "its 1 page"), confirm "Delete folder", cancel "Keep folder". Navigate back to workspace if the open page was inside the deleted folder.

---

### `web/src/components/TrashView.tsx` — EXTEND (grouped row)

**Analog:** current TrashView.tsx. Reuse `.trashview-row` chrome, `relativeTime` (11-27), per-page `restoreMut` + collision notice (43-61). Add grouped entries: a single row labeled `Folder '{folderName}' · {N} pages` with a primary `Restore folder` button (`Undo2`) calling `restoreFolderGroup`; batched collision notice via the existing `.trashview-notice` `role="status"` (81-85). Group by the new `delete_group_id` field on `TrashEntry`.

---

### `web/src/api/client.ts` — EXTEND (utility)

**Analog:** `renamePage`/`movePage`/`deletePage`/`restoreFromTrash` (client.ts:263-319) and the `mutate<T>` helper (50-85, handles CSRF + empty-body + `err.status`).

**Pattern to mirror:** each new function is a thin `mutate<T>(path, body, method)` call. `renameFolder`/`moveFolder` POST to the `/pages/{dir}/...` catch-all suffix (mirror `movePage` body `{new_parent}` at 275-282); `deleteFolder` POSTs the folder-delete route; `restoreFolderGroup` POSTs `/trash/group/{id}/restore` (mirror `restoreFromTrash` at 317-319). Extend `TrashEntry` (289-295) with `delete_group_id`. NOTE (RESEARCH Runtime State): `LeftTree.test.tsx`'s `vi.mock("../api/client")` factory must add these 4 names or the rebuilt component throws on import.

---

## Shared Patterns

### Single-commit multi-file write+remove
**Source:** `commitPayload` + `CommitHandler` + `EnqueueCommit` (`internal/pages/commitjob.go`), used identically by `relocate` (rename.go:111-125), `Delete` (trash.go:81-93), `Restore` (trash.go:286-298).
**Apply to:** `relocateFolder`, `DeleteFolder`, `RestoreGroup`. Fold all writes+removes into ONE payload; the handler loops Writes then Removes then makes exactly one `gitstore.Commit` through the single-writer mutex. Never hand-roll a git batch.

### Round-trip-safe inbound-link rewrite
**Source:** `rewriteInboundLinks` → `okf.Parse`/`okf.RewriteLinks`/`doc.Emit` (rename.go:149-211).
**Apply to:** every moved page in `relocateFolder`. Structural byte scanner only — never AST/regex (D-07/P03 corruption-safety invariant; the okf golden-corpus round-trip gate).

### SEC-01 path safety + input validation
**Source:** `cleanPathString`/`cleanPathParam` (handlers_pages.go:76-97) at the HTTP edge; `repo.Resolve/Read/Write/Remove/Exists/MkdirAll` chokepoint (`internal/repo/files.go`).
**Apply to:** every folder path + `new_parent` destination + group id. Validate at the handler, re-resolve at the worker (defense in depth). Group id uses parameterized `?` SQL binds.

### Editor RBAC from session
**Source:** `editor` subgroup `auth.RequireRole(auth.RoleEditor)` (router.go:138-139).
**Apply to:** all new folder mutate routes + grouped restore. Client menu hides items (`canEdit`) but the server gate is authoritative.

### TanStack Query mutation + invalidate
**Source:** the shipped `onSuccess`-invalidate mutations (LeftTree.tsx:107-114, MoveDialog.tsx:47-58, RenameModal.tsx:29-40, TrashView.tsx:43-61).
**Apply to:** all dialogs (keep `onSuccess` invalidate) AND the new optimistic DnD/folder-op mutations (`onMutate` snapshot → `onError` rollback → `onSettled` invalidate — the net-new layer on top of the established invalidate-on-settle).

### Embedded ordered migration
**Source:** `migrations.go` runner (`//go:embed migrations/*.sql`, idempotent in `schema_migrations`) + `0005_trash.sql` format.
**Apply to:** `0008_trash_group.sql`. Additive `ALTER TABLE`, backward-compatible NULL.

### Drain-aware test fixture
**Source:** `newServiceFixture` (service_test.go:89-112) + `waitForFile`/`waitForGone`/`waitForRevisionChange`/`waitForCommitCount`/`commitCount`.
**Apply to:** every new Go test. Key off committed state, never `time.Sleep`.

---

## No Analog Found

None. Every file in this phase extends or rebuilds existing, shipped, tested code; no novel role or data-flow is introduced. (`DeleteFolderDialog.tsx` is "new" only as a file — its analog is the existing `Dialog`-shell dialogs + `DeleteConfirmDialog`.)

---

## Metadata

**Analog search scope:** `internal/pages/`, `internal/server/`, `internal/store/migrations/`, `web/src/components/`, `web/src/api/`.
**Files scanned:** rename.go, trash.go, service.go, service_test.go, rename_test.go, commitjob_test.go, handlers_pages.go, handlers_trash.go, router.go, migrations.go, 0005_trash.sql, LeftTree.tsx, TreeContextMenu.tsx, MoveDialog.tsx, RenameModal.tsx, TrashView.tsx, client.ts.
**Pattern extraction date:** 2026-06-21
