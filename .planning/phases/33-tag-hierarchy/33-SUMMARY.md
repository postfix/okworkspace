---
author: engineer
responsible: engineer
phase: 33
status: complete
---

# Phase 33: Tag Hierarchy — Summary

**Goal:** Tags form a cycle-safe parent/child hierarchy in the Phase-32 store,
shown grouped in the admin list — display/organization only, no page rewrites.

**Requirements:** TF-03

## What was built

### Task 1: Backend — parent field + cycle detection + migration

- **`internal/tagmeta/meta.go`**: Added `Parent string` to the `Meta` struct
  (normalized parent tag name; `""` = root).
- **`internal/store/migrations/0012_tag_meta_parent.sql`**: Additive migration
  `ALTER TABLE tag_meta ADD COLUMN parent TEXT NOT NULL DEFAULT ''`. The
  migration harness auto-discovers by version number — no registration needed.
- **`internal/tagmeta/store.go`**:
  - `SetMeta` gains a `parent string` parameter. It validates via
    `ValidateParent`, runs `CheckCycle` BEFORE the write (D-02a), writes the
    `parent` field into `Tags/{tag}.md` frontmatter, and upserts the cache row
    with the parent value. When all three fields are empty, the file is deleted.
  - `GetMeta` / `AllMeta` read the `parent` column from the cache.
  - `RebuildTagMeta` parses the `parent` frontmatter key and populates the cache.
  - `metaFile` renders the `parent:` line in the on-disk frontmatter.
  - `upsertCache` includes `parent` in the INSERT/UPSERT.
  - `CheckCycle` (exported) walks the parent chain from `parent` upward via the
    cache: self-parenting rejected directly; revisiting `tag` → cycle; exceeding
    `maxCycleDepth` (100) → corrupted store; reaching root or a non-existent
    parent → valid (D-02b). O(depth) — the hierarchy is shallow.
- **`internal/tagmeta/validate.go`**: Added `ValidateParent` — validates the
  parent is a valid tag name (`okf.ValidateTagCharset`) or empty. Does NOT check
  existence (non-existent parent treated as root per D-02b).
- **`internal/tagmeta/store_test.go`**: 6 new tests:
  - `TestSetMetaWithParent` — set parent, verify cache + frontmatter.
  - `TestCycleDetection_SelfParent` — self-parenting rejected.
  - `TestCycleDetection_Indirect` — 3-tag cycle rejected.
  - `TestCycleDetection_DeepChain` — 50-tag chain valid; closing the loop rejected.
  - `TestNonExistentParent` — non-existent parent accepted (D-02b).
  - `TestRebuildParityWithParent` — seed with parent, rebuild, delete cache,
    rebuild → identical rows with correct parent values.
  - Updated `snapshotMeta` to select/scan the `parent` column.

### Task 2: Backend — admin endpoint + cycle check wire-in

- **`internal/server/handlers_tags_admin.go`**:
  - `tagMetaRequest` gains `Parent string` JSON field.
  - `tagMetaResponse` gains `Parent string` JSON field.
  - `handleTagMeta` parses + normalizes `parent`, validates via
    `ValidateParent`, runs `tagmeta.CheckCycle` before the write (400 on cycle),
    passes `parent` to `SetMeta`, includes `parent` in the response + audit.
  - `handleTagList` response includes `parent` per tag (from `AllMeta`).
- **`internal/server/handlers_tags_admin_test.go`**: 4 new tests:
  - `TestTagHierarchySetParent` — admin sets parent; GET /admin/tags shows it.
  - `TestTagHierarchyCycle` — indirect cycle → 400.
  - `TestTagHierarchySelfParent` — self-parent → 400.
  - `TestTagHierarchyRootTag` — empty parent → valid (root).
  - Updated `tagListEntry` struct with `Parent` field.

### Task 3: Frontend — grouped admin rendering

- **`web/src/api/client.ts`**: `TagEntry` gains `parent?: string`; `setTagMeta`
  opts gains `parent: string`.
- **`web/src/components/admin/TagManagement.tsx`**:
  - `TagTable` computes a grouped view from the flat tag list: roots (no parent
    or orphaned parent not in the list) with children indented beneath. Uses
    `Set<string>` for membership and `Map<string, TagEntry[]>` for grouping.
    Children are rendered with a `↳` tree marker and a `(parent: X)` note.
  - `TagMetaDialog` gains a "Parent" `<select>` input: the tag vocabulary
    excluding the tag itself (self-parenting prevented in the UI; server cycle
    check is the authoritative gate). A "— root (no parent) —" option represents
    empty/absent parent.
- **`web/src/components/admin/TagManagement.css`**: Added
  `.tagmgmt-child-row`, `.tagmgmt-tree-marker`, `.tagmgmt-parent-note` styles.

## Verification

- `go build ./...` — exits 0.
- `go test ./internal/tagmeta/ -count=1 -v` — all 15 tests pass.
- `go test ./internal/server/ -run TestTagHierarchy -count=1 -v` — all 4 pass.
- `go test ./internal/server/ -run TestTagMeta -count=1 -v` — all 5 existing pass.
- `cd web && npx tsc --noEmit` — exits 0.
- Display-only (TF-03-3): `grep 'parent' internal/pages/ internal/search/` shows
  only filesystem-path parents, no tag-hierarchy auto-apply or search-expansion.
- Rebuild parity (TF-03-4): `TestRebuildParityWithParent` verifies delete-cache →
  rebuild = identical rows with correct parent values.

## Design decisions

- `checkCycle` exported as `CheckCycle` so the server handler can call it before
  the write (different package).
- Non-existent parent treated as root (D-02b) — no error, the value is stored
  as-is; the UI treats non-existent as root.
- `SetMeta` folded parent into the existing method (one endpoint, one write) per
  the context recommendation.
- `ClearMeta` passes `parent=""` (3 empty fields → file deleted).

## Commits

All code + this SUMMARY committed in the phase-33 build span.