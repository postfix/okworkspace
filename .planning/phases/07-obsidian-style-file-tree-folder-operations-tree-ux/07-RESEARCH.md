# Phase 7: Obsidian-style File Tree (folder operations & tree UX) - Research

**Researched:** 2026-06-21
**Domain:** Folder-as-unit relocate/trash/restore over a single-writer Git commit worker (Go) + a clean-rebuilt native-DnD React tree with TanStack Query optimistic updates
**Confidence:** HIGH — this is a codebase-extension phase; every claim below is `[VERIFIED: codebase]` by reading the actual source. No new external dependencies.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

**Folder Operation Backend Semantics**
- A folder rename/move is ONE atomic commit: the folder's `index.md` AND every descendant page under the `dir/` prefix are relocated together, and all inbound links to any moved page are rewritten in the SAME commit. Extend Phase 1's single-page `pages.relocate` (delete-old + write-new + inbound link rewrite, D-07) to a folder-batch; reuse the structural byte-scanner link rewriter from Phase 1 P03 (never an AST — code blocks provably uncorrupted).
- Folder move/rename REJECTS cleanly (409/400) when the target dir already exists — never silently merge two folders.
- Folder delete trashes every contained page (including `index.md`) using the existing per-page trash machinery; there is NO permanent delete this phase.

**Folder Restore Semantics — GROUPED (user override)**
- Grouped folder restore is in v1. A folder delete records the pages it trashed as a GROUP (a folder-delete batch/operation id) so the whole folder can be restored as a unit — "Restore folder" / "Undo folder delete" — recreating the folder structure (paths, incl. `index.md`) in one action. Per-page restore from trash remains available too.
- Trash data model gains a group/batch identifier linking the pages trashed in one folder-delete op. (Discretion: exact schema — a nullable `delete_group_id` on trash rows, or a sidecar table.)
- Restore collision (a page now exists at the original path) reuses the existing `restoredAlternative` auto-suffix; grouped restore applies it per-page.
- Folder delete shows a confirmation dialog naming the affected page count.

**Drag-and-Drop — OPTIMISTIC (user override)**
- Folders become draggable AND droppable (onto another folder or the root) and move as a unit; existing page drag-drop is preserved.
- Guard invalid drops: a folder cannot be dropped into itself or any of its own descendants (no-op + "not-allowed" affordance).
- Highlight the drop target (folder/root) on drag-over.
- Optimistic tree updates: `onMutate` snapshot → optimistic apply → `onError` rollback → `onSettled` invalidate. Replaces the prior "wait for the commit, then refetch" approach for the affected subtree (commit-wait remains the correctness backstop / reconciliation step).

**Context Menu & Tree UX — CLEAN REBUILD (user override)**
- Folder context menu actions: New page here · New folder here · Rename · Move · Delete. Page context menu keeps: Rename · Move · Delete · Version history.
- Rebuild the tree UX cleanly: `LeftTree`, `TreeContextMenu`, `MoveDialog`, `RenameModal`, and the folder-scoped create flow from scratch. The rebuild MUST preserve every currently-shipped behavior with regression tests. No user-visible regression is acceptable.
- `RenameModal` / `MoveDialog` are the (rewritten) shared dialogs handling BOTH page and folder rename/move — the accessible alternative to drag-and-drop.

### Claude's Discretion
- Exact trash group-id schema and the grouped-restore query.
- Optimistic-update cache-shape details and rollback granularity.
- Component decomposition of the rebuilt tree (hooks, DnD library vs native HTML5 DnD — prefer native or the already-present `react-dropzone`/patterns; do not add heavy deps).
- Folder Move dialog destination-picker UX (reuse the page MoveDialog picker).

### Deferred Ideas (OUT OF SCOPE)
- Canvas / base doc types, bookmarks, copy-path, show-in-system-explorer (paths are hidden by design), search-in-folder (Phase 3) — out of scope.
- Permanent delete — not this phase (everything is trash-recoverable).
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| TREE-01 | Right-click context menu (folder/page actions) | `TreeContextMenu` already supports an items-array contract + full a11y; rebuild parameterizes folder vs page vs root item sets (`LeftTree.menuItems` is the existing shape). Net-new = the 3 folder actions (Rename/Move/Delete) wired to net-new endpoints. |
| TREE-02 | Folder rename/move/delete as a unit; all inbound links rewritten in ONE commit; okf round-trip holds | Extend `relocate` to `relocateFolder`: enumerate `index.md` + every `<dir>/` descendant, build ONE `commitPayload` (Writes for each moved page at its new path + every inbound-link rewrite, Removes for each old path). `rewriteInboundLinks` runs per moved page. `CommitHandler` already does multi-file write+remove+single-commit. See §Folder-batch relocate. |
| TREE-03 | DnD reorganization with optimistic tree updates | Native HTML5 DnD already shipped for pages (`application/x-okf-page`); add `application/x-okf-folder`. TanStack Query `onMutate`/`onError`/`onSettled` recipe over `["tree"]`. See §Optimistic updates. |
| TREE-04 | Folder delete recoverable (pages → trash, restorable); no permanent delete | Loop existing `Delete` over each descendant under one `delete_group_id`; per-page restore unchanged. |
| TREE-05 | Grouped restore (undo folder delete), recreating structure | Add `delete_group_id` to `trash` schema (migration 0008); `RestoreGroup(groupID)` iterates `Restore` per row. See §Grouped trash. |
| TREE-06 | Clean reject on collision (no silent merge); invalid drag (folder onto self/descendant) prevented | Folder-target collision check before relocate → `ErrFolderExists` → 409. Client path-prefix guard during drag-over. See §Collision + §DnD guard. |
</phase_requirements>

## Summary

Phase 7 is a **codebase-extension phase, not greenfield**. The two hard parts — single-commit relocate with inbound-link rewriting, and recoverable delete-to-trash — already exist and work for single pages (Phase 1, D-07/D-08). The net-new backend work is to **lift those single-page operations to folder-batch operations**: enumerate the folder's `index.md` plus every descendant `.md` under the `dir/` prefix, and fold them into ONE `commitPayload` that the existing `CommitHandler` already knows how to execute (it loops `Writes` then `Removes` then makes exactly one commit). The corruption-safety argument is inherited verbatim: every moved page's bytes are read and re-written unchanged (a move never re-emits through okf), and every inbound link is rewritten through the structural byte-scanner `okf.RewriteLinks` (never an AST), so the golden-corpus round-trip invariant holds by construction.

Grouped trash restore needs a **one-column schema migration** (`delete_group_id` nullable on the `trash` table) plus a `RestoreGroup` service method that iterates the existing `Restore` over each row in the group. Per-page restore stays byte-identical. The frontend is a **clean rebuild that must preserve every shipped behavior** (captured exhaustively in §Clean-Rebuild Behavior Inventory below) and then layer on folder DnD (a second `dataTransfer` key), the 3 new folder context-menu actions, optimistic `["tree"]` cache updates, and the grouped-trash row. No new npm or Go dependency is introduced — native HTML5 DnD is already in use; TanStack Query, lucide-react, and the Dialog shell are all present.

**Primary recommendation:** Add a `relocateFolder` that reuses `relocate`'s exact commit/rewrite machinery over an enumerated descendant set; add a nullable `delete_group_id` to the `trash` table with a `DeleteFolder`/`RestoreGroup` pair; rebuild the tree frontend behind a regression-test net that pins all shipped behaviors first, then add folder DnD + optimistic `["tree"]` mutations.

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Folder rename/move as one commit + link rewrite | API/Backend (`pages` service) | Database/Git (single-writer worker) | Round-trip safety + atomicity can only be guaranteed server-side through the single-writer commit; the client never touches files/git. |
| Folder delete → trash (grouped) | API/Backend (`pages` service + SQLite trash) | Database (trash schema) | Trash provenance + group id is operational metadata (SQLite); content stays on disk in Git. |
| Grouped restore | API/Backend (`pages` service) | Database (trash query by group) | Same as delete; the restore set is read from SQLite, written through the commit worker. |
| Collision detection (target dir exists) | API/Backend (`relocateFolder` precheck) | — | Authoritative filesystem check; server owns the 409. |
| Tree rendering / context menu / DnD affordance | Browser/Client (React) | — | Pure presentation + interaction; reads `["tree"]` server state. |
| Optimistic tree mutation + rollback | Browser/Client (TanStack Query cache) | API (reconcile via invalidate) | Cache-shape mutation is client-only; server commit is the reconciliation backstop. |
| Invalid-drag (self/descendant) guard | Browser/Client (path-prefix check during dragover) | API (defensive re-reject) | Must be felt *during* drag-over (client), with the server as defense-in-depth. |

## Standard Stack

**No new dependencies.** Every library this phase needs is already installed and version-locked. Verified from `web/package.json` and `go.mod`.

### Core (already present — reuse)
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `@tanstack/react-query` | 5.101.0 | Server-state cache + optimistic updates | `["tree"]`/`["page"]`/`["trash"]` are already its keys; `onMutate`/`onError`/`onSettled` is its first-class optimistic-update API. `[VERIFIED: web/package.json]` |
| `lucide-react` | 0.469.0 | Tree/menu/trash icons (`ChevronRight/Down`, `FileText`, `Folder`, `Undo2`, `Trash2`) | Already the icon set; UI-SPEC forbids a new one. `[VERIFIED: web/package.json]` |
| `react-router-dom` | 7.18.0 | Navigation after move/delete | Already used by every dialog (`navigate(...)`). `[VERIFIED: web/package.json]` |
| `react` / `react-dom` | 19.2.7 | UI | Native HTML5 DnD via React synthetic `DragEvent` — no DnD lib. `[VERIFIED: web/package.json]` |
| `vitest` + `@testing-library/react` + `user-event` | 3.2.4 / 16.3.0 / 14.6.1 | Frontend tests | Existing harness (`LeftTree.test.tsx`, `TreeContextMenu.test.tsx`). `[VERIFIED: web/package.json]` |

### Backend (Go — already present)
| Package | Purpose | Why |
|---------|---------|-----|
| `internal/gitstore` | Single-writer commit (mutex + git CLI) | `Commit(spec)` stages N paths in one commit. `[VERIFIED: internal/gitstore/commit.go]` |
| `internal/okf` (`RewriteLinks`, `Parse`, `Emit`) | Structural link rewrite + byte-stable round-trip | Phase 1 P03 byte scanner; never an AST. `[VERIFIED: internal/okf/links.go]` |
| `internal/repo` | SEC-01 safe-path resolver (`Resolve`, `Read`, `Write`, `Remove`, `Exists`, `MkdirAll`) | Every path op routes through `Resolve`. `[VERIFIED: internal/repo/files.go]` |
| `internal/jobs` | Worker + `EnqueueAndWait` | Drains commit jobs; `EnqueueCommit` blocks until on disk. `[VERIFIED: internal/pages/commitjob.go]` |
| `modernc.org/sqlite` (via `internal/store`) | Trash metadata + migration runner | Embedded ordered `NNNN_name.sql` migrations. `[VERIFIED: internal/store/migrations.go]` |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Native HTML5 DnD | `@dnd-kit`, `react-dnd` | CONTEXT + UI-SPEC explicitly forbid a heavy DnD dep; native is already shipped and sufficient. Do NOT add. |
| Nullable `delete_group_id` column | Sidecar `trash_groups` table | A column is the minimal migration and keeps per-page restore queries unchanged (column is just NULL for solo deletes). Recommended. Sidecar adds a join for no benefit at this scale. |
| Looping single-page `Delete` for folder delete | A bespoke batch-delete commit | Folder delete is N separate trash rows anyway (each restorable individually); reusing `Delete` keeps the trash data model uniform. One nuance: see Pitfall 3 (atomicity of N deletes). |

**Installation:** none. (`go.mod` and `web/package.json` are unchanged by this phase.)

## Package Legitimacy Audit

> **Not applicable.** This phase installs **zero** external packages. All libraries are already present and version-locked in `go.mod` / `web/package.json` (verified). No registry lookup, slopsquat, or postinstall risk applies.

**Packages removed due to [SLOP] verdict:** none.
**Packages flagged as suspicious [SUS]:** none.

## Architecture Patterns

### System Architecture Diagram

```
                        FOLDER OPERATION (rename / move / delete)
                                       │
  ┌────────────────────────────── BROWSER (React) ──────────────────────────────┐
  │  TreeContextMenu (folder items)   LeftTree row drag  ──►  drop on folder/root │
  │        │                                  │ (application/x-okf-folder)        │
  │        ▼                                  ▼                                   │
  │  RenameModal / MoveDialog          useFolderMove mutation                     │
  │  DeleteFolderDialog (count N)            │ onMutate: snapshot ["tree"],       │
  │        │                                 │           apply move to cache      │
  │        └───────────────┬─────────────────┘ (OPTIMISTIC, no spinner)          │
  └────────────────────────┼──────────────────────────────────────────────────── │
                           │ POST /pages/{dir}/rename-folder  (or /delete-folder) │
                           ▼
  ┌──────────────────────────── GO BACKEND (single process) ────────────────────┐
  │  handlers_pages.go  ── editor RBAC gate (session role) ── cleanPathString    │
  │        │                                                                      │
  │        ▼                                                                      │
  │  pages.Service.RenameFolder / MoveFolder / DeleteFolder                       │
  │        │                                                                      │
  │        ▼                                                                      │
  │  relocateFolder(oldDir, newDir):                                             │
  │     1. enumerate index.md + every <oldDir>/ descendant .md                    │
  │     2. collision precheck: newDir already exists? → ErrFolderExists (409)     │
  │     3. for each page: read bytes (UNCHANGED), compute new path                │
  │     4. for each page: rewriteInboundLinks(oldPath→newPath) across whole repo │
  │     5. fold ALL writes + ALL removes into ONE commitPayload                   │
  │        │                                                                      │
  │        ▼                                                                      │
  │  EnqueueCommit ──► jobs.Worker (single drain goroutine) ──► CommitHandler:    │
  │     for w in Writes: repo.Write(w)   (SEC-01 Resolve)                         │
  │     for rm in Removes: repo.Remove(rm)                                        │
  │     gitstore.Commit(spec)  ── ONE commit, N files ── git rename detection     │
  └──────────────────────────────────────────────────────────────────────────────┘
                           │ tree refetch (onSettled invalidate) reconciles cache
                           ▼
                    Markdown files on disk (files-as-truth, byte-stable)
```

The folder DELETE path forks at the service: instead of `relocateFolder`, `DeleteFolder` loops the existing per-page `Delete` over the enumerated descendant set, tagging each new trash row with a shared `delete_group_id`. Grouped restore reverses it: `RestoreGroup(id)` reads all rows with that group id and iterates `Restore`.

### Recommended Project Structure (files touched/added)
```
internal/pages/
├── rename.go         # EXTEND: add relocateFolder, RenameFolder, MoveFolder, ErrFolderExists
├── trash.go          # EXTEND: DeleteFolder (group id), RestoreGroup, ListTrash grouping
├── tree.go           # reference only (tree shape the client mirrors optimistically)
└── *_test.go         # ADD: folder relocate / grouped-restore / collision table tests
internal/store/migrations/
└── 0008_trash_group.sql   # ADD: ALTER TABLE trash ADD COLUMN delete_group_id TEXT
internal/server/
├── handlers_pages.go # EXTEND: dispatch folder rename/move on the /pages/* catch-all
├── handlers_trash.go # EXTEND: grouped restore endpoint
└── router.go         # EXTEND: register new POST routes (see Pitfall 5 — sibling-wildcard)
web/src/
├── api/client.ts     # ADD: renameFolder, moveFolder, deleteFolder, listTrash(grouped), restoreFolderGroup
├── components/
│   ├── LeftTree.tsx           # REBUILD (preserve all behaviors; add folder DnD + optimistic)
│   ├── TreeContextMenu.tsx    # REBUILD (preserve a11y; folder item set)
│   ├── RenameModal.tsx        # REBUILD (page + folder, parameterized by node kind)
│   ├── MoveDialog.tsx         # REBUILD (page + folder picker)
│   ├── DeleteFolderDialog.tsx # ADD (confirmation with page count N)
│   ├── TrashView.tsx          # EXTEND (grouped folder row + Restore folder)
│   └── hooks/useTreeMutations.ts  # ADD (optimistic onMutate/onError/onSettled recipe)
```

### Pattern 1: Folder-batch relocate (the core net-new backend pattern)
**What:** Lift single-page `relocate` to a folder by enumerating the descendant set and folding all writes/removes into ONE `commitPayload`.
**When to use:** Folder rename and folder move (both are a `relocateFolder(oldDir, newDir)`).
**Why it's correct:** `CommitHandler` already executes a multi-file payload (`for fw := range p.Writes { r.Write(...) }` then `for rm := range p.Removes { r.Remove(...) }` then exactly one `g.Commit`). `relocate` already produces this exact shape for one page. A folder is "loop the descendants into one payload."

```go
// Source: PATTERN derived from internal/pages/rename.go relocate() [VERIFIED: codebase]
// relocateFolder relocates oldDir (and every .md under it, incl. index.md) to
// newDir in ONE commit, rewriting every inbound link to every moved page.
func (s *Service) relocateFolder(ctx context.Context, oldDir, newDir, action, user string) error {
    // 1. Enumerate descendants: index.md + every <oldDir>/**/*.md (skip .git/.okf-workspace).
    //    Reuse the WalkDir + isSkippedDir + ToSlash + HasPrefix(oldDir+"/") pattern
    //    already in rewriteInboundLinks / Tree.
    oldPaths := s.descendantPages(oldDir) // []string, repo-relative

    // 2. Collision precheck (TREE-06): if newDir already exists on disk → reject,
    //    never silently merge.
    if exists, _ := s.repo.Exists(newDir); exists {
        return ErrFolderExists
    }

    var writes []fileWrite
    var removes []string
    stagePaths := []string{}
    // 3. For each moved page: write its EXISTING bytes at the new path (a move never
    //    re-emits through okf — bytes are byte-identical), stage old+new for git
    //    rename detection.
    for _, oldPath := range oldPaths {
        newPath := newDir + strings.TrimPrefix(oldPath, oldDir) // prefix swap
        src, err := s.repo.Read(oldPath)
        if err != nil { return err }
        writes = append(writes, fileWrite{Path: newPath, Bytes: src})
        removes = append(removes, oldPath)
        stagePaths = append(stagePaths, newPath, oldPath)
    }
    // 4. Rewrite EVERY inbound link to EVERY moved page (one pass per moved page;
    //    a page already in `writes` may get rewritten too — merge by path so the
    //    last write wins and we never stage two writes for one path).
    for _, oldPath := range oldPaths {
        newPath := newDir + strings.TrimPrefix(oldPath, oldDir)
        rewrites, rwPaths, err := s.rewriteInboundLinks(oldPath, newPath)
        if err != nil { return err }
        writes = mergeWritesByPath(writes, rewrites) // see Pitfall 1
        stagePaths = append(stagePaths, rwPaths...)
    }
    // 5. ONE commit through the single-writer worker.
    return EnqueueCommit(ctx, s.worker, commitPayload{
        Writes:  writes,
        Removes: removes,
        Spec: gitstore.CommitSpec{Paths: stagePaths, Message: ..., User: user, Action: action, Source: "web-ui"},
        Push:   s.pushOnCommit,
    })
    // (then enqueue index delete(oldPath)/upsert(newPath) per moved page, fire-and-forget)
}
```

### Pattern 2: Grouped trash + restore
**What:** Tag every trash row from one folder-delete with a shared `delete_group_id`; restore the group by iterating per-page `Restore`.
**Example:**

```sql
-- Source: ADD internal/store/migrations/0008_trash_group.sql [DESIGN — follows existing migration style]
ALTER TABLE trash ADD COLUMN delete_group_id TEXT;   -- NULL for solo page deletes
```
```go
// DeleteFolder loops the existing per-page Delete with a shared group id.
func (s *Service) DeleteFolder(ctx context.Context, dir, user string) error {
    groupID := newGroupID() // e.g. crypto/rand hex, or s.now() + dir slug
    for _, p := range s.descendantPages(dir) {
        // Delete already: git-mv to trash, records the row. Extend Delete (or add an
        // internal deleteWithGroup) to write delete_group_id on the INSERT.
        if err := s.deleteWithGroup(ctx, p, user, groupID); err != nil { return err }
    }
    return nil
}
// RestoreGroup restores every row sharing groupID, reusing per-page Restore
// (so restoredAlternative collision-suffixing applies per page automatically).
func (s *Service) RestoreGroup(ctx context.Context, groupID, user string) ([]string, error) {
    ids := s.trashIDsForGroup(ctx, groupID) // SELECT id FROM trash WHERE delete_group_id = ?
    var restored []string
    for _, id := range ids {
        p, err := s.Restore(ctx, id, user) // existing method, unchanged
        if err != nil { return restored, err }
        restored = append(restored, p)
    }
    return restored, nil
}
```

### Pattern 3: Optimistic `["tree"]` update (TanStack Query)
**What:** Apply the move to the cached tree immediately; roll back on error; invalidate to reconcile.
**Example:**
```ts
// Source: TanStack Query optimistic-update API [CITED: tanstack.com/query/v5/docs/framework/react/guides/optimistic-updates]
const moveMut = useMutation({
  mutationFn: ({ src, dest, kind }) =>
    kind === "folder" ? moveFolder(src, dest) : movePage(src, dest),
  onMutate: async ({ src, dest, kind }) => {
    await queryClient.cancelQueries({ queryKey: ["tree"] });       // stop in-flight refetch
    const prev = queryClient.getQueryData<TreeNode[]>(["tree"]);   // snapshot
    queryClient.setQueryData<TreeNode[]>(["tree"], (old) =>
      applyMove(old ?? [], src, dest, kind));                      // pure tree transform
    return { prev };                                               // rollback context
  },
  onError: (_err, _vars, ctx) => {
    if (ctx?.prev) queryClient.setQueryData(["tree"], ctx.prev);   // ROLLBACK
    setError("We couldn't move that just now — it's back where it was. Check your connection and try again.");
  },
  onSettled: () => {
    queryClient.invalidateQueries({ queryKey: ["tree"] });         // reconcile w/ server
    queryClient.invalidateQueries({ queryKey: ["page"] });
  },
});
```
`applyMove` is a pure helper over the `TreeNode[]` shape (`{type, path, title, children?}`) that removes the node from its old parent and re-inserts under the destination, rewriting `path` (and, for a folder, every descendant `path`) by prefix swap — mirroring server semantics so the optimistic view matches the eventual refetch.

### Anti-Patterns to Avoid
- **Re-emitting a moved page through `okf.Emit`.** A move must write the page's EXISTING bytes verbatim (`relocate` reads `srcBytes` and writes them unchanged). Re-emitting risks a byte difference and breaks the round-trip gate. Only the *inbound-link rewrite* of OTHER pages goes through okf (and only when `RewriteLinks` reports `changed`).
- **Two writes for one path in a single payload.** A folder move where a moved page also contains an inbound link to a sibling moved page can produce both a "move write" and a "rewrite write" for the same new path. Merge by path (last-wins) before enqueuing (Pitfall 1).
- **AST-based link rewriting.** Forbidden by D-07/P03. Use `okf.RewriteLinks` (structural byte scanner) only.
- **Adding a DnD library.** Forbidden by CONTEXT + UI-SPEC. Extend the native `dataTransfer` pattern.
- **Silently merging folders on collision.** TREE-06 requires a clean 409; never write into an existing target dir.
- **Storing tree/ephemeral DnD state in TanStack Query cache.** Open-menu, open-dialog, drop-active flags stay component-local/zustand (UI-SPEC).

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Single-commit multi-file write+delete | A new git batch routine | `commitPayload` + `CommitHandler` (loops Writes then Removes then one `gitstore.Commit`) | Already exists, already serialized by the single-writer mutex, already SEC-01-gated. `[VERIFIED: commitjob.go]` |
| Inbound-link rewrite | A regex/substring replace | `okf.RewriteLinks` | Skips fenced/inline code, escapes, external URLs; matches on *resolved* target not substring; preserves all other bytes. `[VERIFIED: okf/links.go]` |
| Collision auto-suffix on restore | A new suffixer | `restoredAlternative` + `uniqueExactPath` | Already produces "(restored)" title + re-slugged free path. `[VERIFIED: trash.go]` |
| Safe path resolution | `os.*` calls | `repo.Resolve/Read/Write/Remove/Exists/MkdirAll` | SEC-01 chokepoint; blocks `../`/absolute/symlink-escape. `[VERIFIED: repo/files.go]` |
| Context-menu a11y (focus trap, arrow nav, viewport clamp, outside-click/scroll/resize close) | A new menu | The existing `TreeContextMenu` contract (rebuild preserves it) | Already implements role=menu/menuitem, arrow/Home/End/Enter/Esc/Tab-trap, 4px clamp, restore-focus-on-close. `[VERIFIED: TreeContextMenu.tsx]` |
| Drain-aware test waits | `time.Sleep` guesses | `waitForFile`/`waitForGone`/`waitForRevisionChange`/`waitForCommitCount` | Existing polling helpers that key off committed state, not working-tree races. `[VERIFIED: rename_test.go / service_test.go]` |
| DB migration | Ad-hoc `CREATE TABLE` at startup | A new `NNNN_name.sql` under `internal/store/migrations/` | The embedded ordered-migration runner applies it idempotently in a tx. `[VERIFIED: migrations.go]` |

**Key insight:** Nearly all the hard machinery is built and tested. The phase's risk is **not** in writing new algorithms — it is in (a) correctly enumerating the descendant set and merging the payload, (b) the one-column trash migration + transaction boundaries for N-page restore, and (c) rebuilding the frontend without dropping a shipped behavior. The regression-test net (below) is the load-bearing safety mechanism.

## Runtime State Inventory

> This phase is a refactor/rebuild of the tree UX + a trash-schema migration. Inventory of state that a grep of source files would NOT surface:

| Category | Items Found | Action Required |
|----------|-------------|------------------|
| Stored data | **SQLite `trash` table** — existing rows have no `delete_group_id`. Migration 0008 adds the column as nullable; existing rows get NULL (correctly treated as solo per-page deletes). No data backfill needed. | Code edit (migration) — additive, backward-compatible. |
| Stored data | **Git history** of every page is continuous via rename detection (`git log --follow`). A folder move must stage old+new for EACH descendant so `--follow` stays intact per page. | Code edit — ensure `stagePaths` includes both old and new per page (Pattern 1 step 3). |
| Live service config | None — this app is a single self-hosted Go binary; no external service holds the renamed strings. (No n8n, Datadog, Tailscale, etc.) | None — verified by reading CLAUDE.md stack (single binary + data dir). |
| OS-registered state | None — no Task Scheduler / launchd / systemd unit embeds page or folder paths. systemd unit name is the binary, not content. | None. |
| Secrets/env vars | None — no secret/env var references a page or folder path. | None. |
| Build artifacts | **`web/dist` embedded SPA** (`//go:embed`) — the rebuilt React tree must be re-built so the embedded binary serves the new components. Vitest/tsc build is part of the existing `npm run build` → `internal/web/dist`. | Build step — rebuild SPA; existing pipeline (STATE: "SPA built into internal/web/dist"). |
| Build artifacts | **Frontend test mocks** — `LeftTree.test.tsx` mocks `../api/client` with a fixed function list; new client functions (renameFolder/moveFolder/deleteFolder/restoreFolderGroup) must be added to the mock or the rebuilt component throws on import. | Code edit (test) — extend the `vi.mock` factory. |

**The canonical question — after every source file is updated, what runtime systems still hold the old string?** Answer: only the SQLite `trash` table (handled additively by the nullable column) and the embedded SPA bundle (handled by rebuild). Git history is intentionally preserved (continuity is a feature). Nothing else.

## Common Pitfalls

### Pitfall 1: Double-staging a path in one folder-move commit
**What goes wrong:** A moved page that links to a sibling moved page generates both a "move write" (its bytes at the new path) and an "inbound-link rewrite write" (its bytes with the sibling link rewritten) for the SAME new path. Two `fileWrite`s for one path → the second `repo.Write` overwrites the first; if ordering is wrong, the link rewrite is lost.
**Why it happens:** `relocate`'s two phases (move write + `rewriteInboundLinks`) are independent; for a single page they never collide, but within a folder batch they can.
**How to avoid:** Merge `writes` by path with last-write-wins, ensuring the rewritten version (computed from the moved bytes) is the one that lands. Crucially, run `rewriteInboundLinks` against the page's NEW path but feed it the page's MOVED bytes, or compute rewrites after staging moves and reconcile. Simplest correct approach: build a `map[string][]byte` keyed by new path, apply moves first, then apply rewrites on top (a rewrite of an already-moved page edits the map entry).
**Warning signs:** A test where page A and page B are both inside the moved folder AND link to each other; after the move, one link points at a stale path.

### Pitfall 2: `rewriteInboundLinks` is O(repo) per page → O(pages² ) per folder
**What goes wrong:** `rewriteInboundLinks` walks the ENTIRE repo for one `(oldRel, newRel)` pair. Calling it once per descendant means walking the whole repo N times for an N-page folder.
**Why it happens:** The single-page function isn't designed for batching.
**How to avoid:** At 5 users / small wiki scale this is acceptable for v1 (verify with a test on a folder of, say, 20 pages). If a planner wants it tighter, add a batched `rewriteInboundLinksMulti(map[oldRel]newRel)` that walks the repo ONCE and applies all matching rewrites per file. Recommend: ship the simple loop first (correctness), note the batched variant as an optional optimization — do NOT prematurely optimize.
**Warning signs:** Slow folder move in a test with many descendants; profiler shows repeated `filepath.WalkDir`.

### Pitfall 3: Non-atomic N-page folder delete / grouped restore
**What goes wrong:** `DeleteFolder` loops per-page `Delete`, each its own commit + its own SQLite row. If the process dies mid-loop, the folder is half-trashed (some pages in trash with the group id, some still live). Same for `RestoreGroup`.
**Why it happens:** Each `Delete`/`Restore` is independently a commit; there is no folder-level transaction across commits. The existing `ReconcileTrash` already documents this residual risk (WR-01) for single deletes.
**How to avoid:** Accept partial-progress semantics consistent with the existing trash model, BUT make them recoverable: because every trashed page carries the same `delete_group_id`, a half-completed delete still presents as a grouped trash entry the user can restore or re-delete. Document this as the chosen tradeoff (mirrors the existing `ReconcileTrash` WR-01 stance). Do NOT attempt a single mega-commit for delete (it would break the per-page-restorable trash model — each page needs its own trash row + trash path). For grouped restore, restore in a deterministic order (index.md first so the folder exists) and surface the batched collision notice if any page was suffixed.
**Warning signs:** A test that kills the loop after K of N deletes and asserts the trash group is still coherent and restorable.

### Pitfall 4: Folder collision check must precede ANY write
**What goes wrong:** If `relocateFolder` stages writes before checking `repo.Exists(newDir)`, a colliding move could partially merge before the reject.
**Why it happens:** Ordering bug.
**How to avoid:** Run the `Exists(newDir)` precheck FIRST and return `ErrFolderExists` before building the payload (Pattern 1 step 2). Map `ErrFolderExists` → HTTP 409 with the UI-SPEC copy "A folder with that name already exists there." Note: unlike page move (which auto-suffixes via `uniqueExactPath`), folder move must REJECT, not suffix (CONTEXT: never silent merge).
**Warning signs:** A test moving folder X into a parent that already contains an X — must 409, must not touch disk.

### Pitfall 5: chi sibling-wildcard conflict for the new folder routes
**What goes wrong:** Adding a new route like `POST /folders/{path:.*}/rename` next to the existing `/pages/*` and `/folders` routes can hit the same sibling-wildcard 405 that Phase 1/2 hit (STATE decisions: "chi cannot host a `{path:.*}` regex node and the `/pages/*` catch-all as siblings").
**Why it happens:** chi's router rejects a regex-wildcard node sibling to a catch-all wildcard on the same path prefix.
**How to avoid:** Mirror the established Phase-1 pattern: dispatch folder rename/move on a catch-all by suffix. Options: (a) reuse the `/pages/*` POST catch-all and dispatch on a `/rename-folder` (or `/move-folder`) suffix inside `handleRenamePage` (it already branches on `/rename` vs `/restore` suffixes), distinguishing a folder target by node kind; or (b) add a `/folders/*` catch-all POST and dispatch on suffix. Recommend (a) for consistency — the dispatcher already exists and folders are `index.md` pages. For grouped restore, `POST /trash/group/{id}/restore` (group id is not a path → no wildcard conflict, mirrors the working `/trash/{id}/restore`).
**Warning signs:** A 405 on the new folder route in an integration test.

### Pitfall 6: Optimistic apply must mirror server path semantics exactly
**What goes wrong:** The optimistic `applyMove` rewrites paths client-side; if it computes new descendant paths differently from the server's prefix-swap, the post-`onSettled` refetch "jumps" the node to a different place, causing a visible flicker.
**Why it happens:** Two independent implementations of the same path math.
**How to avoid:** Make `applyMove` do a literal prefix swap (`oldDir` → `newDir` on every affected `path`) identical to `relocateFolder`. Cover with a vitest that asserts the optimistic tree equals the server's eventual tree for a representative move. Also handle the collision case: the optimistic view assumes success; on a 409 the `onError` rollback restores the snapshot and the dialog shows the collision copy (the dialog stays open).
**Warning signs:** A node flickers/relocates after the spinner-less optimistic move settles.

## Code Examples

### Enumerate folder descendants (reuse the existing walk pattern)
```go
// Source: PATTERN from internal/pages/tree.go Tree() + rename.go rewriteInboundLinks() [VERIFIED: codebase]
func (s *Service) descendantPages(dir string) ([]string, error) {
    root := s.repo.Root()
    prefix := strings.TrimSuffix(dir, "/") + "/"
    var out []string
    err := filepath.WalkDir(root, func(abs string, d fs.DirEntry, werr error) error {
        if werr != nil { return werr }
        if abs == root { return nil }
        rel, _ := filepath.Rel(root, abs)
        slashRel := filepath.ToSlash(rel)
        if isSkippedDir(slashRel) {
            if d.IsDir() { return filepath.SkipDir }
            return nil
        }
        if d.IsDir() || !strings.HasSuffix(slashRel, ".md") { return nil }
        if slashRel == dir+"/index.md" || strings.HasPrefix(slashRel, prefix) {
            out = append(out, slashRel)
        }
        return nil
    })
    return out, err
}
```

### Folder context-menu items (rebuilt LeftTree — the net-new folder actions)
```ts
// Source: PATTERN from current LeftTree.menuItems [VERIFIED: LeftTree.tsx] — extended w/ folder ops
function folderMenuItems(folder): TreeContextMenuItem[] {
  if (!canEdit) return [];                    // RBAC mirror of server gate
  return [
    { label: "New page here",   onSelect: () => openCreate({kind:"page",   folder: folder.path}) },
    { label: "New folder here", onSelect: () => openCreate({kind:"folder", parent: folder.path}) },
    { label: "Rename",          onSelect: () => openRename({kind:"folder", path: folder.path, title: folder.title}) },
    { label: "Move",            onSelect: () => openMove({kind:"folder",   path: folder.path}) },
    { label: "Delete", danger: true, onSelect: () => openDeleteFolder(folder) }, // confirm w/ count N
  ];
}
```

### Native folder DnD + self/descendant guard (TREE-06)
```ts
// Source: PATTERN extending current LeftTree page-DnD [VERIFIED: LeftTree.tsx]
// Drag a folder row:
onDragStart={(e) => { e.dataTransfer.setData("application/x-okf-folder", node.path);
                      e.dataTransfer.effectAllowed = "move"; }}
// Drop target validity (computed DURING dragover so the affordance is correct):
function dropAllowed(dragKind, dragPath, targetFolder) {
  if (dragKind === "folder") {
    // invalid: onto itself or any descendant (prefix check), or same-parent no-op
    if (targetFolder === dragPath) return false;
    if (targetFolder.startsWith(dragPath + "/")) return false;
    if (parentOf(dragPath) === targetFolder) return false;
  } else { // page
    if (parentOf(dragPath) === targetFolder) return false; // same-parent no-op
  }
  return true;
}
// onDragOver: only preventDefault()+highlight when dropAllowed; else leave resting + cursor:not-allowed.
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| "Wait for commit, then refetch `["tree"]`" (Phase 1 UAT commit-wait fix) | Optimistic `onMutate` apply + `onError` rollback + `onSettled` invalidate; commit-wait stays as the reconciliation backstop | This phase (CONTEXT override) | Tree updates feel instant; the commit-wait still guarantees correctness on reconcile. |
| Ad-hoc tree components shipped during Phase 1 UAT | Clean rebuild of LeftTree/TreeContextMenu/RenameModal/MoveDialog | This phase (CONTEXT override) | Must preserve every shipped behavior with regression tests. |
| Per-page trash only | Grouped folder-delete + grouped restore via `delete_group_id` | This phase | Folder delete is reversible as a unit. |
| Folders had create-only context menu (no backend folder rename/move/delete) | Full folder ops via `relocateFolder` | This phase | Current `LeftTree.menuItems` folder branch returns only the 2 create actions — net-new is the 3 mutate actions. `[VERIFIED: LeftTree.tsx lines 133–150]` |

**Deprecated/outdated:** none introduced. `@uiw/react-md-editor` was already removed in Phase 6 (STATE 06-04) — unrelated to this phase.

## Clean-Rebuild Behavior Inventory (regression-test BEFORE/AROUND the rebuild)

> Every shipped behavior the rebuild MUST preserve, extracted from the current components. The planner should write regression tests pinning these FIRST, then rebuild. `[VERIFIED: codebase]`

**LeftTree.tsx (current):**
1. Renders folders + pages from `getTree` (`["tree"]` query); folders sort first, then by title (server-side `assembleTree`).
2. Coalesces `null`/`undefined` tree to `[]` (empty-repo serializes to JSON `null` — `nodes.map` would white-screen).
3. Folders expand/collapse (NAV-02), expanded-by-default, caret with `aria-expanded` + accessible label.
4. Active page row highlighted via `/app/page/:path` match → `navrow-active` + `aria-current="page"` (NAV-04).
5. `forwardRef` `LeftTreeHandle` exposes `openCreatePage`/`openCreateFolder` for the parent's top buttons (root-scoped create).
6. Right-click folder → "New page here" / "New folder here" (folder-scoped create).
7. Right-click page (editor) → Rename / Move / Version history / Delete; (reader) → Version history only (RBAC).
8. Right-click blank nav space → root create ("New page" / "New folder").
9. Page rows draggable (editor only) via `application/x-okf-page`; `effectAllowed = "move"`.
10. Drop a page onto a folder row → move into folder; drop onto root drop-zone → move to root.
11. Same-parent no-op drop guarded (`parentOf(pagePath) === destFolder` → no move).
12. Drop-target highlight via `lefttree-droptarget` on folder row + root zone.
13. Loading state (`role="status"` "Loading…"); error state (`role="alert"` "Couldn't load your pages — try again.").
14. `canEdit` derived from `me()` role; readers get no DnD, no mutate menu items.

**TreeContextMenu.tsx (current):** role=menu/menuitem; opens focus on first item; Arrow Up/Down wrap; Home/End jump; Enter/Space select; Esc closes; Tab trapped (wraps); focus restored on close; closes on outside-click / scroll (capture) / resize; 4px viewport clamp; `danger` item → `treemenu-item-danger`.

**RenameModal.tsx (current):** page rename via `renamePage`; empty-title client validation ("Give your page a title."); `onSuccess` invalidates `["tree"]`+`["page"]`, closes, navigates to new path; `onError` inline `role="alert"`; help "Links to this page will keep working."; confirm "Rename".

**MoveDialog.tsx (current):** flattens tree to folder list + "Top level" (`""`); `movePage`; `onSuccess` invalidates + navigates; help "Choose where this page should live." / "Links to this page will keep working."; confirm "Move page".

**DeleteConfirmDialog.tsx (current):** `deletePage`; destructive confirm; backdrop NEVER confirms; `onSuccess` invalidates `["tree"]`+`["trash"]`, navigates `/app`; copy "It will move to Trash…"; confirm "Delete" / cancel "Keep page".

**TrashView.tsx (current):** `["trash"]` query; per-row Restore (`restoreFromTrash`); relative-time rendering; collision notice when restored path ≠ original ("…restored as '{title} (restored)'."); empty state; loading/error; invalidates `["tree"]`+`["trash"]` on restore.

**Dialog.tsx contract:** `onCancel` = Esc + Cancel button + backdrop click; `destructive` signals backdrop-must-not-confirm; `busy` disables + shows loading; `confirmLabel`/`cancelLabel`.

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | Nullable `delete_group_id TEXT` column (not a sidecar table) is the right schema. | Pattern 2 | Low — explicitly Claude's discretion; column is minimal + backward-compatible. If wrong, swap to sidecar with no API change. |
| A2 | Looping per-page `Delete`/`Restore` (non-atomic across commits) is acceptable for folder delete/restore, consistent with the existing `ReconcileTrash` WR-01 stance. | Pitfall 3 | Medium — if the team wants strict folder-atomicity, the trash model would need a redesign (single-commit batch trash with per-page rows). Recommend confirming the partial-progress tradeoff in discuss-phase. |
| A3 | The O(pages²) inbound-link rewrite is acceptable at this scale for v1 (simple loop ships first; batched variant deferred). | Pitfall 2 | Low — 5 users / small wiki; verify with a 20-page-folder test. Batched rewrite is a pure optimization, no API change. |
| A4 | Reusing the `/pages/*` POST catch-all suffix-dispatch for folder rename/move (option a) is preferred over a new `/folders/*` catch-all. | Pitfall 5 | Low — both work; (a) matches the established Phase-1 dispatcher. Planner may choose either. |
| A5 | A folder move REJECTS on collision (no auto-suffix), unlike page move which auto-suffixes. | Pitfall 4 | Low — directly from CONTEXT ("never silently merge"); the auto-suffix path is page-only. |

## Open Questions

1. **Folder delete/restore atomicity (A2).**
   - What we know: each page is an independent commit + trash row; the group id ties them together for listing/restore.
   - What's unclear: whether the team wants strict all-or-nothing folder delete, or accepts the existing per-page partial-progress + reconcile model.
   - Recommendation: ship the per-page-with-group-id model (recoverable by design); flag A2 for a one-line confirmation in discuss-phase.

2. **Restore order within a group.**
   - What we know: `index.md` must exist for the folder to render; descendants restore into it.
   - What's unclear: whether restore order matters for collision suffixing.
   - Recommendation: restore `index.md` first, then descendants; surface the batched "(restored)" notice if any page collided. Cover with a test.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| `git` CLI | single-writer commit (existing) | ✓ (assumed — already required by Phases 0–6) | — | none (core constraint) |
| Go toolchain | backend build/test | ✓ | 1.26.0 (CLAUDE.md locked) | none |
| Node + npm | SPA build/test | ✓ | Node 20.19.6 (CLAUDE.md) | none |

**Missing dependencies with no fallback:** none — all infrastructure is the same as the already-shipped Phases 0–6. `[ASSUMED]` based on prior phases building successfully; planner's Wave 0 should run `go build ./...` + `npm run build` to confirm.

## Validation Architecture

> `workflow.nyquist_validation` not found explicitly disabled — section included.

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Backend: Go `testing` (table-driven, real repo+git+worker fixture). Frontend: `vitest` 3.2.4 + `@testing-library/react` 16.3.0 + `user-event` 14.6.1. |
| Config file | Backend: none (go test). Frontend: `web/vite.config.ts` / `vitest` in `web/package.json` (`"test": "vitest run"`). |
| Quick run command | Backend: `go test ./internal/pages/ -run TestFolder -x`. Frontend: `cd web && npx vitest run src/components/LeftTree.test.tsx`. |
| Full suite command | `go test ./...` and `cd web && npm run test`. |

### Phase Requirements → Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| TREE-02 | Folder rename/move = ONE commit; all inbound links rewritten; round-trip holds | unit (Go) | `go test ./internal/pages/ -run TestRelocateFolder` | ❌ Wave 0 (`rename_test.go` has the single-page harness to extend) |
| TREE-02 | No corruption (code-block link-like text untouched) | unit (Go) | `go test ./internal/okf/ -run TestRewriteLinks` + a folder-level corpus test | ✅ (okf round-trip) / ❌ folder test Wave 0 |
| TREE-04 | Folder delete → N trash rows w/ shared group id; pages restorable | unit (Go) | `go test ./internal/pages/ -run TestDeleteFolder` | ❌ Wave 0 (`trash_test.go` harness exists) |
| TREE-05 | Grouped restore recreates structure; per-page restore unchanged | unit (Go) | `go test ./internal/pages/ -run TestRestoreGroup` | ❌ Wave 0 |
| TREE-06 | Folder collision → 409 (no merge) | unit (Go) | `go test ./internal/pages/ -run TestRelocateFolder_Collision` | ❌ Wave 0 |
| TREE-06 | Invalid drag (self/descendant) prevented | unit (vitest) | `npx vitest run src/components/LeftTree.test.tsx` | ✅ (extend) |
| TREE-03 | Optimistic apply + rollback on error | unit (vitest) | `npx vitest run src/components/LeftTree.test.tsx` | ✅ (extend) |
| TREE-01 | Folder/page/root menu item sets + a11y | unit (vitest) | `npx vitest run src/components/TreeContextMenu.test.tsx` | ✅ (extend) |
| ALL (regression) | Clean-Rebuild Behavior Inventory preserved | unit (vitest) | `npx vitest run` | ✅ (pin BEFORE rebuild) |

### Sampling Rate
- **Per task commit:** the affected quick command (Go package run or the single vitest file).
- **Per wave merge:** `go test ./...` + `cd web && npm run test`.
- **Phase gate:** full suite green + okf golden-corpus round-trip green before `/gsd-verify-work`.

### Wave 0 Gaps
- [ ] `internal/pages/rename_test.go` — add `TestRelocateFolder`, `TestMoveFolder`, `TestRelocateFolder_Collision`, `TestRelocateFolder_NoCorruption` (cross-linked descendants — Pitfall 1). Reuse `newServiceFixture` + `waitForFile`/`waitForGone`/`commitCount`.
- [ ] `internal/pages/trash_test.go` — add `TestDeleteFolder` (group id on rows), `TestRestoreGroup` (structure recreated, collision-suffix batched), `TestDeleteFolder_PartialProgress` (Pitfall 3).
- [ ] `internal/store/migrations/0008_trash_group.sql` — add; covered transitively by `st.Migrate` in the fixture (assert column exists).
- [ ] `web/src/components/LeftTree.test.tsx` — extend `vi.mock("../api/client")` with `moveFolder/renameFolder/deleteFolder/restoreFolderGroup`; add folder-DnD guard + optimistic-rollback tests.
- [ ] `web/src/components/__regression__` — pin the Clean-Rebuild Behavior Inventory before rebuilding.
- Framework install: none — all present.

## Security Domain

> `security_enforcement` not disabled — section included. This phase handles untrusted input (folder paths, drag payloads, group ids).

### Applicable ASVS Categories
| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V1 Architecture | yes | Single-writer commit + SEC-01 resolver are the chokepoints; folder ops must not bypass them. |
| V4 Access Control | yes | Folder rename/move/delete + grouped restore are EDITOR-gated via `auth.RequireRole(auth.RoleEditor)` read from the SESSION role, never client input (mirror existing page-mutation subgroup in `router.go`). Tree read + trash list stay any-authenticated. |
| V5 Input Validation | yes | Every folder path + destination parent runs through `cleanPathString` (rejects empty/NUL/absolute/`..` segments) AND the `repo.Resolve` chokepoint before any disk op. Folder Move `new_parent` is attacker-controlled — validate exactly like the existing page-move path (`WR-08`). Group id from the client (`/trash/group/{id}/restore`) must be validated/parameterized (SQL bind, never interpolated). |
| V6 Cryptography | no (group id is an opaque identifier, not a secret) | If using random group ids, `crypto/rand` is fine; no secret material. |

### Known Threat Patterns for Go + SQLite + native DnD
| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Path traversal via folder path / `new_parent` (`../`, absolute) | Tampering / EoP | `cleanPathString` + `repo.Resolve` (SEC-01); never `os.*` directly. `[VERIFIED: handlers_pages.go, repo/path.go]` |
| Folder dir escaping when staging a `dir/` prefix (symlink in a descendant) | Tampering | Every staged path re-resolved by `gitstore.Commit` add-loop (`g.repo.Resolve(p)`); a folder op stages each descendant individually, so each is re-validated. `[VERIFIED: gitstore/commit.go]` |
| SQL injection via `delete_group_id` | Tampering | Parameterized queries only (`?` binds) — existing trash code uses bind params throughout. `[VERIFIED: trash.go]` |
| RBAC bypass (reader performs folder op) | EoP | Editor gate from session role on the new routes; client menu hides items but server is authoritative. `[VERIFIED: router.go editor subgroup]` |
| Stored XSS via folder title surfaced in tree/menu/dialog | (Tampering) | Titles render as React text nodes (no `dangerouslySetInnerHTML`); read mode sanitizes (Phase 1). No raw HTML from folder names. |

## Sources

### Primary (HIGH confidence — read this session)
- `internal/pages/rename.go` — `relocate`, `rewriteInboundLinks`, `uniqueExactPath`, collision sentinel. The single-page pattern to lift to folder-batch.
- `internal/pages/trash.go` — `Delete`, `Restore`, `restoredAlternative`, `ReconcileTrash` (WR-01), `ListTrash`, trash row INSERT shape.
- `internal/pages/service.go` — `CreateFolder` (`<dir>/index.md`), `Create`, `uniquePath`, `enqueueWrite`, index enqueues.
- `internal/pages/commitjob.go` — `commitPayload` (Writes/Removes/Spec/Push), `CommitHandler` (multi-file write+remove+one-commit), `EnqueueCommit` (commit-wait).
- `internal/pages/tree.go` — `Tree`/`assembleTree`/`isSkippedDir`/`pageTitle`; the `TreeNode` shape the client mirrors.
- `internal/gitstore/commit.go` — `Commit` stages N paths in one commit, re-resolves each (SEC-01).
- `internal/okf/links.go` — `RewriteLinks`/`FindLinks` structural byte scanner.
- `internal/repo/files.go` — `Read/Write/Remove/Exists/MkdirAll` (SEC-01 resolver).
- `internal/store/migrations.go` + `migrations/0005_trash.sql` — embedded ordered idempotent migrations; current trash schema.
- `internal/server/handlers_pages.go` / `handlers_trash.go` / `handlers_tree.go` / `router.go` — the HTTP surface + chi sibling-wildcard dispatch pattern + editor RBAC subgroup.
- `web/src/components/LeftTree.tsx` / `TreeContextMenu.tsx` / `MoveDialog.tsx` / `RenameModal.tsx` / `TrashView.tsx` / `DeleteConfirmDialog.tsx` — current frontend behaviors (rebuild inventory).
- `web/src/api/client.ts` — `getTree`/`movePage`/`renamePage`/`listTrash`/`restoreFromTrash` signatures.
- `internal/pages/service_test.go` / `rename_test.go` — the test harness (`newServiceFixture`, drain-aware wait helpers).
- `web/package.json` / CLAUDE.md — version locks (no new deps).

### Secondary (MEDIUM confidence)
- TanStack Query v5 optimistic-updates guide — `onMutate`/`onError`/`onSettled` + `cancelQueries` recipe. `[CITED: tanstack.com/query/v5/docs/framework/react/guides/optimistic-updates]`

### Tertiary (LOW confidence)
- none.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — all deps already locked + verified in `package.json`/`go.mod`; nothing new.
- Architecture (folder-batch relocate, grouped trash, optimistic tree): HIGH — derived directly from verified existing single-page machinery; the lift is mechanical.
- Pitfalls: HIGH — each is grounded in a specific behavior of the verified source (double-staging, O(repo) walk, WR-01 non-atomicity, chi sibling-wildcard, optimistic/server path parity).
- Grouped-restore atomicity tradeoff (A2): MEDIUM — recommended approach is consistent with the existing model but worth one confirmation.

**Research date:** 2026-06-21
**Valid until:** 2026-07-21 (stable — internal codebase; only TanStack Query external reference, which is stable on v5).
