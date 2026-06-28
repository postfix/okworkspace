# Milestone v1.3 Requirements — Tag Management UX

## Overview

Workspace-wide tag rename, merge, and delete across all pages. Every operation rides the existing byte-stable `SetTagsFrontmatter` + single-writer `EnqueueCommit` path. Admin-only preview→approve safety contract. Derived graph store stays consistent after bulk mutations.

---

## Tag Management Operations

### TM-01: Tag Rename

**Requirement:** Rename a tag across every page that carries it. The operation must preview affected pages before committing and handle edge cases consistently.

**Acceptance criteria:**
- [ ] Admin endpoint `POST /api/v1/admin/tags/rename` accepts `{old_name, new_name}`
- [ ] Preview returns list of affected pages (path + current tags)
- [ ] Approve applies rename through `okf.SetTags` + single-writer commit (batched, N pages → 1 commit)
- [ ] Tag name normalization: trim whitespace, fold to lowercase, reject empty
- [ ] Conflict: if `new_name` already exists on some pages, merge (existing `new_name` pages keep it; `old_name` pages swap to `new_name`)
- [ ] Case-only rename (e.g. "Ops" → "ops") treated as no-op after normalization
- [ ] Stale revision (409) on individual pages doesn't sink the batch; per-page status in response
- [ ] Audit log entry for tag rename (actor, old_name, new_name, pages_affected)

**Security:**
- Admin-only (editor/reader → 403)
- CSRF required on approve
- `new_name` validated against tag charset (alphanumeric, dash, underscore, space)
- Server-side re-validation of tag list before write (never trust client)

**Notes:**
- Rides `SetTagsFrontmatter` byte-stable builder — only the `tags:` YAML region changes
- Uses batched `EnqueueCommit` with `[]fileWrite` payload (Phase 12 pattern)

---

### TM-02: Tag Merge

**Requirement:** Merge a source tag into a target tag across all pages. Pages carrying the source tag lose it and gain the target tag (if not already present). Pages already carrying both keep only the target.

**Acceptance criteria:**
- [ ] Admin endpoint `POST /api/v1/admin/tags/merge` accepts `{source, target}`
- [ ] Preview returns list of affected pages grouped by action (add-target, remove-source, both, no-change)
- [ ] Approve applies merge through `okf.SetTags` + single-writer commit
- [ ] Target already present on a page → source removed, target untouched (no duplicate)
- [ ] Source == target after normalization → rejected 400
- [ ] Source tag ceases to exist in the workspace after merge (no orphan tag nodes)
- [ ] Per-page 409 handling same as TM-01
- [ ] Audit log entry for tag merge

**Security:** Same as TM-01

---

### TM-03: Tag Delete

**Requirement:** Remove a tag from every page that carries it. All-or-nothing with preview.

**Acceptance criteria:**
- [ ] Admin endpoint `POST /api/v1/admin/tags/delete` accepts `{tag}`
- [ ] Preview returns list of affected pages and their current tags
- [ ] Approve removes tag from all pages through `okf.SetTags` + single-writer commit
- [ ] After deletion, tag node removed from graph (no orphan tag nodes)
- [ ] Per-page 409 handling same as TM-01
- [ ] Audit log entry

**Security:** Same as TM-01. Additionally require confirmation dialog with affected-page count.

---

### TM-04: Tag Admin UI

**Requirement:** Admin-only tag management surface integrated into the existing admin panel.

**Acceptance criteria:**
- [ ] New "Tag Management" section in Admin panel (`/app/admin`)
- [ ] Tag list showing all workspace tags with page counts
- [ ] "Rename" action → inline rename dialog with preview
- [ ] "Merge" action → two-tag picker with preview
- [ ] "Delete" action → confirmation dialog showing affected page count
- [ ] Preview shows paginated affected-page list
- [ ] Approve button with pending/progress state
- [ ] Error states: network failure, per-page 409, server validation error

**Non-goals:**
- Bulk multi-tag operations (select N tags → delete/merge)
- Tag color/description metadata
- Tag hierarchy/parent-child relationships

---

### TM-05: Graph Store Consistency

**Requirement:** After any bulk tag mutation (rename/merge/delete), the derived graph store must reflect the new tag state without requiring a manual admin reindex.

**Acceptance criteria:**
- [ ] Graph store incrementally updated after batched tag commit
- [ ] Tag rename: old tag node removed, new tag node created (or old updated in-place)
- [ ] Tag merge: source tag node removed, target tag degree updated
- [ ] Tag delete: tag node removed, all affected page nodes' tag edges removed
- [ ] Rebuilding graph store from disk reproduces identical adjacency
- [ ] `go test ./internal/graph/... -race` clean after tag mutations

**Notes:**
- Graph store is a derived cache — correctness of the rebuild path is the invariant
- Incremental update is a performance optimization; the admin reindex backstop always exists

---

## Traceability

| REQ-ID | Description |
|--------|-------------|
| TM-01 | Tag rename — rename a tag across all pages |
| TM-02 | Tag merge — merge source tag into target |
| TM-03 | Tag delete — remove a tag from all pages |
| TM-04 | Tag admin UI — admin surface for tag operations |
| TM-05 | Graph store consistency — derived store stays correct after mutations |

---

## Out of Scope

- **Bulk multi-tag operations** — Select N tags and delete/merge at once. Deferred.
- **Tag metadata** — Colors, descriptions, hierarchy. Deferred.
- **Non-admin tag management** — Editors cannot perform workspace-wide tag ops. Per-page tagging (Phase 11) remains editor-gated.
- **Undo for tag operations** — No dedicated undo; Git history provides recovery.
- **Tag usage analytics** — No dashboard for "most used tags" or "tag trends".
