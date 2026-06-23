---
phase: 07-obsidian-style-file-tree-folder-operations-tree-ux
reviewed: 2026-06-21T00:00:00Z
depth: standard
files_reviewed: 16
files_reviewed_list:
  - internal/pages/rename.go
  - internal/pages/trash.go
  - internal/okf/links.go
  - internal/server/handlers_pages.go
  - internal/server/handlers_trash.go
  - internal/server/router.go
  - internal/store/migrations/0008_trash_group.sql
  - internal/audit/audit.go
  - web/src/components/hooks/useTreeMutations.ts
  - web/src/components/LeftTree.tsx
  - web/src/components/TreeContextMenu.tsx
  - web/src/components/DeleteFolderDialog.tsx
  - web/src/components/MoveDialog.tsx
  - web/src/components/RenameModal.tsx
  - web/src/components/TrashView.tsx
  - web/src/api/client.ts
findings:
  critical: 1
  warning: 1
  info: 2
  total: 4
status: issues_found
---

# Phase 07: Code Review Report

**Reviewed:** 2026-06-21
**Depth:** standard
**Files Reviewed:** 16
**Status:** issues_found

## Summary

The Phase 07 implementation covers backend folder rename/move/delete and grouped trash restore, plus the React tree DnD and context-menu UI. The backend is solid: byte-stability is correctly honoured (okf.Emit is called only when links genuinely change; RawFront is re-attached verbatim when FrontDirty=false), the prefix-match guard in `descendantPages` and `dropAllowed` both use a `dir+"/"+` boundary (no `foo`/`foobar` false match), SQL is fully parameterised, RBAC comes from the session role on every mutating route, and the grouped-trash restore correctly orders index.md first.

One blocker was found: the folder rename and move dialogs navigate the SPA to the raw folder directory path instead of the folder's index.md, which triggers a 500 error. One warning covers a drop-highlight state leak in the DnD implementation. Two info items flag cosmetic issues.

---

## Critical Issues

### CR-01: Folder rename/move dialogs navigate to bare folder dir, not index.md — causes 500

**Files:**
- `web/src/components/RenameModal.tsx:75`
- `web/src/components/MoveDialog.tsx:98`

**Issue:** When `kind="folder"`, both `RenameModal.onSuccess` and `MoveDialog.onSuccess` call `navigate(`/app/page/${res.path}`)`. The server returns the new folder directory (e.g. `"guides"` or `"newparent/guides"` — not an `.md` path) for `handleRenameFolder` / `handleMoveFolder`. PageView calls `getPage("guides")`, which calls `repo.Exists("guides")` (returns `true` — `os.Stat` succeeds on a directory), then `repo.Read("guides")` which calls `os.ReadFile` on a directory — an OS error. The handler returns HTTP 500 with the generic error copy. Every folder rename or move via the tree context menu ends in a 500 error splash screen.

The correct post-mutation path for a folder is `res.path + "/index.md"` (the folder's home page).

**Fix:** Differentiate the navigation target in each dialog:

In `RenameModal.tsx`:
```typescript
onSuccess: (res) => {
  queryClient.invalidateQueries({ queryKey: ["tree"] });
  queryClient.invalidateQueries({ queryKey: ["page"] });
  onClose();
  const target =
    kind === "folder" ? `${res.path}/index.md` : res.path;
  navigate(`/app/page/${target}`);
},
```

In `MoveDialog.tsx`:
```typescript
onSuccess: (res) => {
  queryClient.invalidateQueries({ queryKey: ["tree"] });
  queryClient.invalidateQueries({ queryKey: ["page"] });
  onClose();
  const target =
    kind === "folder" ? `${res.path}/index.md` : res.path;
  navigate(`/app/page/${target}`);
},
```

---

## Warnings

### WR-01: DnD onDragLeave clears highlight without relatedTarget guard — causes visible flicker

**File:** `web/src/components/LeftTree.tsx:401`

**Issue:** `useNodeDropZone.onDragLeave` unconditionally sets `active` to `false`. The HTML5 DnD specification fires `dragleave` on a container element whenever the drag pointer enters a child element (the pointer fires `dragenter` on the child first, then `dragleave` on the parent). For `FolderRow`, which has a button, icon, label span, and a nested `<ul>` of children, this means the `lefttree-droptarget` highlight class is cleared for one frame every time the dragged item passes over any child. The highlight snaps back on the next `dragover` (which bubbles from the child), producing a visible flicker on folder rows with many children.

```typescript
// Current (flickers):
function onDragLeave() {
  setActive(false);
}

// Fix: only clear when the pointer actually leaves the container
function onDragLeave(e: DragEvent) {
  // relatedTarget is the element being entered; if it is inside this container,
  // the drag has not left — suppress the clear.
  if (
    e.currentTarget instanceof Element &&
    e.relatedTarget instanceof Node &&
    e.currentTarget.contains(e.relatedTarget)
  ) {
    return;
  }
  setActive(false);
}
```

The `onDragLeave` signature in `useNodeDropZone` would need to accept a `DragEvent` parameter, which means the callers (`FolderRow` and `RootDropZone`) pass `drop.onDragLeave` to `onDragLeave={drop.onDragLeave}` — no change needed at the JSX call sites since React passes the event automatically.

---

## Info

### IN-01: `countFolderPages` underreports — dialog copy undercounts index.md files in nested folders

**File:** `web/src/components/hooks/useTreeMutations.ts:186-198`

**Issue:** `countFolderPages` counts only `type="page"` tree nodes. The tree API never surfaces a folder's `index.md` as a page node — it is represented as the folder node itself. So for a folder that contains two sub-folders (each with their own `index.md`) plus three pages, `countFolderPages` returns 3, but the actual `DeleteFolder` moves 3 + 2 (subfolder index.md files) + 1 (the root folder's own index.md) = 6 files to trash. The `DeleteFolderDialog` copy reads "its 3 pages" while 6 files actually move.

This is a copy inaccuracy, not a data-safety issue — nothing is lost. The simplest accurate copy is "this folder and all its pages" without a hard count, or the count should include subfolder index pages. Noting here for correctness.

### IN-02: `descendantPages` — first condition in the include-guard is redundant dead code

**File:** `internal/pages/rename.go:191`

**Issue:**
```go
if slashRel == prefix+"index.md" || strings.HasPrefix(slashRel, prefix) {
```
`prefix` is `dir + "/"`. `slashRel == prefix+"index.md"` means `slashRel == dir+"/index.md"`. Since `dir+"/index.md"` starts with `dir+"/"` (= `prefix`), the first condition is always subsumed by `strings.HasPrefix(slashRel, prefix)`. The first clause can be removed without changing behaviour.

```go
// Simplified (semantically identical):
if strings.HasPrefix(slashRel, prefix) {
    pages = append(pages, slashRel)
}
```

---

## What was NOT found (verification notes)

- **Byte-stability (TREE-02):** Confirmed sound. `relocateFolder` stores verbatim `srcBytes` in `byPath[newPath]` for each moved page. `rewriteFolderInboundLinks` only calls `doc.Emit()` when `changed=true` (genuine link rewrites). When `!changed` the function returns `nil` for that page, leaving the verbatim copy in `byPath` intact. `okf.Emit` uses `doc.RawFront` verbatim when `FrontDirty=false` and passes `doc.Body` (the post-splice bytes) through unchanged — no non-link bytes are touched.
- **Folder-prefix correctness (TREE-06):** Both `descendantPages` (Go) and `dropAllowed`/`rewritePath`/`collectFolders` (TypeScript) use `dir+"/"` as the prefix boundary. A folder named `foo` never matches `foobar`.
- **SQL injection (T-07-05):** `deleteWithGroup`, `ListTrash`, `RestoreGroup` all use `?` placeholders. `delete_group_id` is bound as `sql.NullString`, never interpolated.
- **RBAC (T-07-06):** All mutating folder routes (`/rename-folder`, `/move-folder`, `/delete-folder`, `/trash/group/{id}/restore`) are in the editor subgroup (`auth.RequireRole(auth.RoleEditor)`) and derive the role from the session, not client input.
- **Path safety (SEC-01):** `cleanPathString` applied to every attacker-controlled input before it reaches the service. `new_parent` for both page-move and folder-move is separately re-validated. `repo.Resolve` enforces the lexical + symlink + OS-Root defense-in-depth stack.
- **Grouped restore ordering:** `sort.SliceStable` places all `index.md` files first, then alphabetically — shallow-before-deep order is guaranteed.
- **CSRF:** All new client mutations go through `mutate()` which calls `ensureCSRF()`. No new GET-as-mutation patterns introduced.
- **Optimistic rollback:** `useTreeMove.onMutate` calls `cancelQueries` before snapshotting, preventing in-flight refetch from overwriting the snapshot. `onError` restores from `ctx.prev`. `onSettled` reconciles from server.
- **`dropAllowed` self/descendant guard:** Correctly uses `` `${dragPath}/` `` prefix so `foo` cannot drop into `foobar` but can drop into `foo/bar`.
- **`activeDragPath` module-level variable:** Set on `dragstart`, cleared on `dragend`. Since `dragend` fires after `drop`, there is no window where it is stale during a drop event's processing. Correct for a single-instance tree.

---

_Reviewed: 2026-06-21_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
