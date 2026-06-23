# Phase 7: Obsidian-style File Tree (folder operations & tree UX) - Context

**Gathered:** 2026-06-21
**Status:** Ready for planning
**Mode:** Smart discuss (autonomous)

<domain>
## Phase Boundary

Make the left file tree manage folders and pages directly — right-click context
menus, drag-and-drop, and folder rename/move/delete — so organizing the workspace
feels like Obsidian. The net-new backend work is **folder operations as a unit**
(rename / move / delete-to-trash a folder, recursively relocating all contained
pages and rewriting inbound links in ONE commit via Phase 1's round-trip-safe
`relocate` + trash machinery). Folders are first-class: a folder is a `<dir>/index.md`
page, so a folder op acts on the `index.md` plus everything under the `dir/` prefix.
Storage/Git stay hidden and byte-stable (the okf golden-corpus round-trip still holds).
Excluded (Obsidian-only / other phases): canvas/base doc types, search-in-folder
(Phase 3), bookmarks, copy-path / show-in-system-explorer (paths are hidden by design).

</domain>

<decisions>
## Implementation Decisions

### Folder Operation Backend Semantics
- A folder rename/move is ONE atomic commit: the folder's `index.md` AND every
  descendant page under the `dir/` prefix are relocated together, and all inbound
  links to any moved page are rewritten in the SAME commit. Extend Phase 1's
  single-page `pages.relocate` (delete-old + write-new + inbound link rewrite,
  D-07) to a folder-batch; reuse the structural byte-scanner link rewriter from
  Phase 1 P03 (never an AST — code blocks provably uncorrupted).
- Folder move/rename REJECTS cleanly (409/400) when the target dir already exists —
  never silently merge two folders.
- Folder delete trashes every contained page (including `index.md`) using the
  existing per-page trash machinery; there is NO permanent delete this phase.

### Folder Restore Semantics — GROUPED (user override)
- **Grouped folder restore is in v1.** A folder delete records the pages it trashed
  as a GROUP (a folder-delete batch/operation id) so the whole folder can be
  restored as a unit — "Restore folder" / "Undo folder delete" — recreating the
  folder structure (paths, incl. `index.md`) in one action. Per-page restore from
  trash remains available too.
- Trash data model gains a group/batch identifier linking the pages trashed in one
  folder-delete op (so grouped restore knows the set). (Discretion: exact schema —
  a nullable `delete_group_id` on trash rows, or a sidecar table.)
- Restore collision (a page now exists at the original path) reuses the existing
  `restoredAlternative` auto-suffix; grouped restore applies it per-page.
- Folder delete shows a confirmation dialog naming the affected page count
  ("Delete folder X and move its N pages to trash?").

### Drag-and-Drop — OPTIMISTIC (user override)
- Folders become draggable AND droppable (onto another folder or the root) and move
  as a unit; existing page drag-drop is preserved.
- Guard invalid drops: a folder cannot be dropped into itself or any of its own
  descendants (no-op + "not-allowed" affordance).
- Highlight the drop target (folder/root) on drag-over.
- **Optimistic tree updates:** on a DnD move (and on folder rename/move/delete) the
  tree query cache updates IMMEDIATELY to reflect the change, then reconciles
  against the server commit result; on failure it ROLLS BACK and surfaces an error
  (TanStack Query `onMutate` snapshot → optimistic apply → `onError` rollback →
  `onSettled` invalidate). This replaces the prior "wait for the commit, then
  refetch" approach for the affected subtree (the shipped commit-wait fix remains
  the correctness backstop / reconciliation step).

### Context Menu & Tree UX — CLEAN REBUILD (user override)
- Folder context menu actions: **New page here · New folder here · Rename · Move ·
  Delete**. Page context menu keeps: Rename · Move · Delete · Version history.
- **Rebuild the tree UX cleanly (deliberate override of the ROADMAP "do NOT re-do"
  note).** Rewrite `LeftTree`, `TreeContextMenu`, `MoveDialog`, `RenameModal`, and
  the folder-scoped create flow from scratch for a consistent, maintainable
  implementation — rather than formalizing the ad-hoc components shipped during
  Phase 1 UAT (commits 69e4fb6/ee5192c/a1486bd/7e0b098/717cfe7). The rebuild MUST
  preserve every currently-shipped behavior with regression tests:
  page-level right-click menu, page drag-drop move, folder-scoped "New page/folder
  here", the dialog-footer fix, and the commit-wait / on-the-fly tree update — then
  layer the new folder operations + optimistic updates on the clean base. No
  user-visible regression is acceptable.
- `RenameModal` / `MoveDialog` are the (rewritten) shared dialogs handling BOTH page
  and folder rename/move — the accessible alternative to drag-and-drop.
- Formalize **TREE-01..TREE-06** in `REQUIREMENTS.md` (mapped to the 4 success
  criteria + the context menu + drag-and-drop) before planning — done by the
  orchestrator, mirroring the EDIT-01..04 formalization in Phase 6.

### Folder Delete/Restore Atomicity — RESOLVED (2026-06-21, post-research)
- Folder **rename/move IS atomic** — one commit relocates `index.md` + all `dir/`
  descendants + rewrites all inbound links (the byte-stability-critical path).
- Folder **delete/restore is per-page-looped under a shared group id** and is NOT
  strictly all-or-nothing across commits — this matches the existing `ReconcileTrash`
  (WR-01) stance and is recoverable by design (trash). A `TestDeleteFolder_PartialProgress`
  test asserts graceful partial-progress behavior. Strict all-or-nothing folder
  delete was considered and rejected as gold-plating that diverges from the shipped
  trash pattern. Grouped restore restores `index.md` first so the folder exists.

### Claude's Discretion
- Exact trash group-id schema and the grouped-restore query.
- Optimistic-update cache-shape details and rollback granularity.
- Component decomposition of the rebuilt tree (hooks, DnD library vs native HTML5 DnD —
  prefer native or the already-present `react-dropzone`/patterns; do not add heavy deps).
- Folder Move dialog destination-picker UX (reuse the page MoveDialog picker).

</decisions>

<code_context>
## Existing Code Insights

### Reusable / Reference Assets (backend — REUSE, do not rewrite)
- `internal/pages/rename.go` — `Rename(oldPath,newTitle)`, `Move(oldPath,newParentDir)`,
  and the core `relocate(oldPath,newPath,action,user)` (single-commit relocate +
  inbound link rewrite). Extend to a folder-batch variant.
- `internal/pages/trash.go` — `ListTrash`, `Restore(id)`, `restoredAlternative`
  (collision auto-suffix), `enqueueTrashedAttachmentDeletes`, `ReconcileTrash`.
  Grouped restore builds on `Restore` + a new group id.
- `internal/pages/service.go` — `CreateFolder(parent,name)` (folder = `<dir>/index.md`),
  `Create(folder,title)`, `uniquePath`.
- `internal/server/handlers_pages.go` (`handleRenamePage`, `handleCreateFolder`),
  `internal/server/handlers_trash.go`, `internal/server/handlers_tree.go` — the route
  surfaces to extend with folder ops + grouped restore.
- okf round-trip + structural link rewriter (Phase 1 P03) — the corruption-safety
  invariant for every relocate.

### Frontend (REBUILD cleanly, preserving behavior)
- `web/src/components/LeftTree.tsx` — current tree + page drag-drop.
- `web/src/components/TreeContextMenu.tsx` — current right-click menu (page actions).
- `web/src/components/MoveDialog.tsx`, `web/src/components/RenameModal.tsx`,
  `web/src/components/CreateFolderModal.tsx`, `web/src/components/PageActionMenu.tsx`,
  `web/src/components/CreatePageModal.tsx` — current dialogs.
- Tests to keep green / extend: `PageActionMenu.test.tsx`, `LeftTree.test.tsx`.

### Established Patterns
- State: TanStack Query for server state (`["tree"]`, `["page", path]`); zustand for
  ephemeral UI. Optimistic updates use Query `onMutate`/`onError`/`onSettled`.
- Backend writes go through the single-writer commit worker (one mutex, git CLI).
- Folders are `index.md` pages; the tree is derived from page paths.

### Integration Points
- New/extended backend endpoints for folder rename/move/delete + grouped trash restore.
- Tree DnD + context menu wired to those endpoints with optimistic cache updates.

</code_context>

<specifics>
## Specific Ideas

- Reference UX is Obsidian's file explorer (right-click menu, drag-drop, folder ops).
  The team are ex-Obsidian users; "feels like Obsidian" is the felt-quality bar.
- User deliberately chose the more thorough options: grouped folder restore,
  optimistic tree updates, and a clean rebuild of the tree UX (over formalizing the
  ad-hoc shipped components). Phase 7 is intentionally larger than the ROADMAP note
  implied.

</specifics>

<deferred>
## Deferred Ideas

- Canvas / base doc types, bookmarks, copy-path, show-in-system-explorer (paths are
  hidden by design), search-in-folder (Phase 3) — out of scope.
- Permanent delete — not this phase (everything is trash-recoverable).

</deferred>
