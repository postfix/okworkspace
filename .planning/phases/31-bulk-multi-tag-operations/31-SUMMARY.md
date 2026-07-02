---
author: engineer
responsible: engineer
phase: 31
phase_type: implementation
status: complete
hard_bar: false
security_relevant: true
design_fork: false
wave: 1
depends_on: []
files_modified:
  - internal/audit/audit.go
  - internal/server/handlers_tags_admin.go
  - internal/server/handlers_tags_admin_test.go
  - internal/server/router.go
  - web/src/api/client.ts
  - web/src/components/admin/TagManagement.tsx
  - web/src/components/admin/TagManagement.css
requirements: [TF-01]
autonomous: true
---

# Phase 31: Bulk Multi-Tag Operations - Summary

**Goal:** An admin adds and/or removes multiple tags across a selected set of
pages in ONE byte-stable preview→approve batch commit, reusing
`pages.ApplyTagsBatch`.

**Result:** Complete — all acceptance criteria met.

## What was built

### Backend (Task 1)

1. **`audit.ActionTagBulk = "tag_bulk"`** — new audit action constant in the
   tag-management action block (`internal/audit/audit.go`), mirroring
   `ActionTagRename`/`Merge`/`Delete`.

2. **`tagBulkRequest`** — request type with `page_paths` (explicit selection),
   `filter_tags` (existing-tag filter union), `add_tags`, `remove_tags`, and
   `preview`. Reuses the existing `tagPreviewResponse` / `tagApplyResponse`
   shapes — no new response types.

3. **`computeBulkTags(addTags, removeTags)`** — compute callback returning
   `(current ∪ addTags) \ removeTags`, normalized + deduped, preserving
   original current ordering then appending new tags in addTags order. The
   `changed` bool is order-insensitive (via `sameTagSet` helper). Plugs into
   the existing `buildTagApplyItems` compute-callback seam.

4. **`sameTagSet(a, b)`** — order-insensitive normalized-set comparison
   helper.

5. **`handleTagBulk`** — `POST /admin/tags/bulk` handler:
   - Decodes `tagBulkRequest`, normalizes all inputs.
   - Validates: at least one of `page_paths`/`filter_tags` AND at least one of
     `add_tags`/`remove_tags` required (400 otherwise).
   - Re-validates each added tag via `okf.ValidateTagCharset` (frontmatter
     charset gate — added tags are written; removed/filter tags are matched,
     not written).
   - Resolves the page set server-side: explicit paths ∪ pages matching any
     filter-tag (via `h.graph.PagesByTag`), deduped + sorted.
   - Calls `h.buildTagApplyItems(ctx, pageSet, computeFn, "", "")` —
     `bodyOld=""` disables the body channel (frontmatter-only, D-02).
   - Preview: returns `tagPreviewResponse{AffectedPages, TotalAffected}`.
   - Apply: calls `h.pages.ApplyTagsBatch(ctx, items, actor)`, returns
     `tagApplyResponse{Action: "bulk", Results}`. Audits
     `audit.ActionTagBulk` non-fatally.

6. **Route registration** — `admin.Post("/admin/tags/bulk", h.handleTagBulk)`
   inside the admin tag route subgroup (inherits `RequireRole(admin)` + nosurf
   CSRF).

### Tests (Task 1)

Six tests in `internal/server/handlers_tags_admin_test.go`:

- **`TestBulkTagRBAC`** — editor gets 403 on both preview and apply.
- **`TestBulkTagPreview`** — 3 pages, preview add `[a,b]` + remove `[c]` →
  `total_affected=3`, per-page `current_tags`/`new_tags` correct.
- **`TestBulkTagGolden`** — add `[a,b]` + remove `[c]` across 3 pages →
  correct per-page frontmatter in exactly 1 commit; graph rebuild shows `c`
  gone, `a` on all 3 pages.
- **`TestBulkTagStale`** — feeds `ApplyTagsBatch` a stale `BaseRevision` for
  p2 (the exact condition a mid-apply edit produces): p2 reports "stale", p1
  applies, batch completes in 1 commit, stale page not clobbered. Also
  verifies the HTTP handler returns per-page statuses.
- **`TestBulkTagFilterSelection`** — `filter_tags: ["ops"]` (no explicit
  paths) → preview resolves to all pages with `ops`.
- **`TestBulkTagValidation`** — invalid charset (`<script>`) in add_tags →
  400; empty page set + empty add/remove → 400.

### Frontend (Task 2)

1. **`bulkTag` API function** in `web/src/api/client.ts` — mirrors
   `renameTag`/`mergeTag`/`deleteTag`; reuses `TagPreview` /
   `TagApplyResponse` types.

2. **`BulkTagDialog` component** in `web/src/components/admin/TagManagement.tsx`:
   - Page-set selector: "Filter by tag" comma-separated input (union) + "Or
     select pages" checkbox list over the page tree (via `getTree()`,
     flattened to `type==="page"` paths).
   - Tag operations: "Tags to add" + "Tags to remove" comma-separated inputs.
   - Preview button → per-page `current_tags → new_tags` table (reuses the
     existing preview-table layout).
   - Apply button → calls `bulkTag` apply, invalidates `TAGS_KEY` query.
   - Cancel button closes the dialog.

3. **"Bulk add/remove" button** in `TagTable` (beside the search bar), opens
   `BulkTagDialog` via `useState` slot.

4. **CSS** in `TagManagement.css` — `.tagmgmt-bulk-*` styles mirroring the
   existing dialog patterns; page-checkbox list scrollable (max-height 300px).

## Invariants preserved

- **No new write path:** the op routes through the existing
  `buildTagApplyItems` + `ApplyTagsBatch` seam — never a parallel commit.
- **Byte-stability:** `BodyOld=""` disables the body channel (frontmatter-only
  op, D-02). The golden test reads frontmatter after apply — only tags change.
- **Admin-only RBAC + CSRF:** inherited from the admin route subgroup
  (`RequireRole(admin)` + nosurf). Proven by `TestBulkTagRBAC`.
- **Server-side re-validation:** each added tag re-validated via
  `ValidateTagCharset`. The client tag list is never trusted.
- **Server-side page-set re-resolution:** the page set is re-derived at BOTH
  preview and apply (TOCTOU-safe — a deleted/renamed page is
  `notfound`/`stale`, not clobbered).
- **Per-page stale exclusion:** a stale page is excluded without sinking the
  batch (proven by `TestBulkTagStale`).

## Self-review notes

- Removed dead `addSet` from `computeBulkTags` (the inner closure uses `addTags`
  directly with the `seen` map for dedup; `addSet` served no purpose).
- Simplified the redundant `computeFn` closure wrapper —
  `computeBulkTags(addTags, removeTags)` already returns the correct signature.
- `h.graph` nil guard: consistent with existing rename/merge/delete handlers,
  which assume `h.graph` is set (it always is in production). The bulk handler
  only touches `h.graph` when `filter_tags` is non-empty.
- `TestBulkTagStale`: the bulk handler re-resolves the page set at apply time
  (like rename/merge/delete), so a stale page is one whose revision moved
  DURING the apply (between `buildTagApplyItems` reading it and
  `ApplyTagsBatch` re-checking it). The test deterministically feeds
  `ApplyTagsBatch` a stale `BaseRevision` — the exact condition a mid-apply
  edit produces — proving the per-page stale exclusion without a flaky race.

## Verification

- `go build ./...` — exits 0
- `go test ./internal/server/ -run TestBulkTag -count=1 -v` — all 6 pass
  (TestBulkTagRBAC, TestBulkTagPreview, TestBulkTagGolden, TestBulkTagStale,
  TestBulkTagFilterSelection, TestBulkTagValidation)
- `go vet ./internal/server/ ./internal/audit/` — exits 0
- `cd web && npx tsc --noEmit` — exits 0
- `cd web && npx vite build` — exits 0

## Notes for qa

- The golden test (`TestBulkTagGolden`) is the load-bearing proof: it verifies
  per-page frontmatter tags after apply, exactly 1 commit (batch atomicity),
  and graph rebuild correctness. Read frontmatter via `pageTags` helper (raw
  YAML parse, since `readPageTags` is unexported in `package server`).
- The stale test exercises `ApplyTagsBatch` directly with a stale
  `BaseRevision` (the deterministic form of the mid-apply race), then verifies
  the HTTP handler's response shape via a follow-up apply. The stale exclusion
  is an `ApplyTagsBatch` invariant the bulk handler rides, not a handler-level
  fork.
- The frontend `BulkTagDialog` uses comma-separated tag inputs (no chip-picker
  component existed in the codebase). The page checkbox list flattens
  `getTree()` to `type==="page"` paths. The dialog follows the same
  preview→approve pattern as `RenameDialog`/`MergeDialog`/`DeleteDialog`.