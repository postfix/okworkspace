# Phase 31: Bulk Multi-Tag Operations - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md - this log preserves the alternatives considered.

**Date:** 2026-07-02
**Phase:** 31-bulk-multi-tag-operations
**Areas discussed:** Page-set selection, Op shape, Preview/approve contract, Audit, Frontend

---

## Page-set selection

| Option | Description | Selected |
| ------ | ----------- | -------- |
| Explicit selection only | Admin picks pages from a list | |
| Existing-tag filter only | Admin picks "all pages with tag X" | |
| Both (union) | Explicit paths ∪ pages-matching-filter-tags, per TF-01 | ✓ |

**User's choice:** Both (union) — directly mandated by TF-01 acceptance criterion ("by explicit selection and/or by an existing-tag filter"). No gray area; the requirement specifies both.

**Notes:** `graph.Store.PagesByTag` already exists for the filter path. `pages.Service.Tree()` provides the page list for explicit selection. No new query needed.

## Op shape

| Option | Description | Selected |
| ------ | ----------- | -------- |
| Add-only | Admin adds tags to selected pages | |
| Remove-only | Admin removes tags from selected pages | |
| Add + remove in one op | Admin specifies add_tags and remove_tags sets; per-page new = (current ∪ add) \ remove | ✓ |

**User's choice:** Add + remove in one op — TF-01 requires "add and/or remove multiple tags." A single op with both sets is the most useful shape and reuses the `buildTagApplyItems` compute-callback seam cleanly.

**Notes:** Frontmatter-only. `BodyOld=""` disables the body rewrite channel. `ValidateTagCharset` is the sole gate (no `ValidateInlineTag` — no body splicing).

## Preview/approve contract

| Option | Description | Selected |
| ------ | ----------- | -------- |
| Reuse existing pattern | POST /admin/tags/bulk with preview=true → tagPreviewResponse; without → ApplyTagsBatch → tagApplyResponse | ✓ |
| New two-step endpoint | Separate preview and approve endpoints | |

**User's choice:** Reuse existing pattern — mirrors rename/merge/delete exactly. Server re-resolves the page set at apply time (TOCTOU-safe). `base_revision` is server-read, never client-claimed.

## Audit

| Option | Description | Selected |
| ------ | ----------- | -------- |
| New ActionTagBulk constant | audit.ActionTagBulk = "tag_bulk", detail = "N pages, +[a,b] -[c]" | ✓ |
| Reuse ActionTagRename | Overload the existing rename action | |

**User's choice:** New constant — the bulk op is a distinct action type. Pattern matches the existing ActionTagRename/Merge/Delete block.

## Frontend

| Option | Description | Selected |
| ------ | ----------- | -------- |
| New BulkTagDialog | A new dialog in TagManagement.tsx mirroring RenameDialog/MergeDialog/DeleteDialog | ✓ |
| Extend the sweep queue | Reuse the bulk-approve sweep queue UI | |

**User's choice:** New BulkTagDialog — the sweep queue is a different workflow (LLM-suggested tags for untagged pages); bulk add/remove is an admin-direct op with explicit tag sets. A separate dialog keeps the two workflows distinct.

## Claude's Discretion

- The exact page-list API for explicit selection (verify existing endpoint in planning).
- BulkTagDialog UX layout (side-by-side vs stacked add/remove inputs).
- Filter-tag picker component (chip-picker vs dropdown — reuse existing if available).
- Commit slicing.

## Deferred Ideas

None — discussion stayed within phase scope.