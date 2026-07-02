---
author: engineer
responsible: engineer
phase: 34
status: complete
---

# Phase 34: Tag Usage Analytics — Summary

## Goal

An admin analytics view over `graph.Store.TagPageCounts` — per-tag page
counts, most/least used, orphans, totals. Read-only, no new scan.

**Requirements:** TF-04

## What Was Built

### Task 1: Backend — analytics handler + route

- **`handleTagAnalytics`** (`internal/server/handlers_tags_admin.go`): GET
  `/admin/tags/analytics` — admin-only, read-only. Sources per-tag counts from
  `graph.Store.TagPageCounts` (the existing `page_tags` cache query — no new
  heavy scan). Computes:
  - `total_tags`: len(counts).
  - `total_pages`: count of live `.md` pages from `pages.Service.Tree()`
    (recursive `countPageNodes` walk of `type=="page"` nodes). Falls back to
    `taggedPages` when the pages service is nil (so untagged is 0, not negative).
  - `tagged_pages`: distinct pages carrying ≥1 tag, from the new
    `graph.Store.TaggedPageCount` (`SELECT COUNT(DISTINCT page_path) FROM page_tags`).
  - `untagged_pages`: total_pages − tagged_pages.
  - `most_used`: top 5 tags by count (descending), tie-broken alphabetically.
  - `least_used`: bottom 5 tags by count (ascending), tie-broken alphabetically.
  - `orphans`: tags that exist as metadata (`tagmeta.AllMeta`) but are carried
    by zero pages (count == 0 in `TagPageCounts`). Non-fatal on metadata read
    error (orphans left empty).
  - `tags`: flat per-tag list (sorted alphabetically) with counts.
  - All arrays are non-nil empty slices (JSON `[]`, not `null`).

- **`graph.Store.TaggedPageCount`** (`internal/graph/query.go`): new exported
  method — `SELECT COUNT(DISTINCT page_path) FROM page_tags`. Nil-db-safe
  (returns 0). Used by the analytics handler for the tagged-page total.

- **Route registration** (`internal/server/router.go`):
  `admin.Get("/admin/tags/analytics", h.handleTagAnalytics)` in the admin
  subgroup, inheriting `RequireRole(admin)`. No CSRF needed (GET, read-only).

- **Tests** (`internal/server/handlers_tags_admin_test.go`):
  - `TestTagAnalyticsRBAC` — editor gets 403; admin gets 200.
  - `TestTagAnalytics` — seeds pages with tags (ops×2, docs×1, one untagged),
    defines a metadata-only orphan tag, verifies total_tags=2, total_pages=3,
    tagged_pages=2, untagged_pages=1, most_used[0]={ops,2}, least_used
    contains docs, orphans contains orphan-tag, tags list has both ops+docs.
  - `TestTagAnalyticsEmpty` — no pages/tags → all zeros, empty arrays.

### Task 2: Frontend — analytics panel

- **`getTagAnalytics`** (`web/src/api/client.ts`): new API function +
  `TagAnalyticsSummary` interface matching the backend JSON shape. GET
  `/api/v1/admin/tags/analytics` with `credentials: "same-origin"`.

- **Analytics tab + panel** (`web/src/components/admin/TagManagement.tsx`):
  - Tab toggle between "Tags" (existing table) and "Analytics" (new view),
    with ARIA `role="tablist"` / `role="tab"` / `aria-selected`.
  - `AnalyticsPanel` component:
    - Summary cards: total tags, tagged pages, untagged pages, total pages.
    - Most-used + least-used sections (top 5 each, from backend).
    - Orphan tags section (metadata-only tags with 0 pages).
    - Per-tag counts table with client-side sort (click "Tag" or "Pages" header
      to toggle sort key/direction).
    - Scope note: "Counts reflect frontmatter tags (the page_tags cache).
      Body-only #tags are counted only after materialization."
  - Data fetched via `useQuery(getTagAnalytics)` with `ANALYTICS_KEY`.

- **CSS** (`web/src/components/admin/TagManagement.css`): styles for the tab
  bar, analytics cards, sections, lists, sortable table headers, and scope note.

## Verification

- `go build ./...` — exits 0.
- `go test ./internal/server/ -run TestTagAnalytics -count=1 -v` — all 3 pass
  (TestTagAnalyticsRBAC, TestTagAnalytics, TestTagAnalyticsEmpty).
- `cd web && npx tsc --noEmit` — exits 0.
- `grep 'handleTagAnalytics' internal/server/handlers_tags_admin.go` — match.
- `grep 'admin.Get("/admin/tags/analytics"' internal/server/router.go` — match.
- `grep 'TagPageCounts' internal/server/handlers_tags_admin.go` — match (reuses existing query).
- `grep 'getTagAnalytics' web/src/api/client.ts` — match.
- `grep 'analytics' web/src/components/admin/TagManagement.tsx` — match.
- `grep 'frontmatter\|page_tags\|body.*materialization' web/src/components/admin/TagManagement.tsx` — match (scope note, TF-04-2).
- Read-only: the analytics handler contains no Write/Post/Delete/Apply/Commit
  calls (TF-04-3).
- No new heavy scan: `TagPageCounts` is the data source (TF-04-1).

## Files Modified

- `internal/graph/query.go` — added `TaggedPageCount` method.
- `internal/server/handlers_tags_admin.go` — added `handleTagAnalytics` handler,
  `tagCountPair` / `tagAnalyticsResponse` types, `countPageNodes` + `topNByCount`
  helpers.
- `internal/server/router.go` — registered `admin.Get("/admin/tags/analytics", …)`.
- `internal/server/handlers_tags_admin_test.go` — 3 analytics tests.
- `web/src/api/client.ts` — `getTagAnalytics` function + `TagAnalyticsSummary` interface.
- `web/src/components/admin/TagManagement.tsx` — tab toggle + `AnalyticsPanel` component.
- `web/src/components/admin/TagManagement.css` — analytics panel styles.

## Notes

- A file collision with Phase 35 (tag-op undo) was resolved: Phase 35's
  `handleTagUndo` declared `var pages []journalPage` which shadowed the `pages`
  package import, breaking `pages.TagApplyItem` references. Renamed the local
  to `jpages` — no logic change, just unshadowed the package import. Phase 35
  engineer confirmed alignment.
- Orphans are sourced from the tag metadata store (`tagmeta.AllMeta`), not the
  graph vocabulary — the vocabulary comes from `page_tags` and always has
  count ≥1, so it can't produce orphans. Metadata-only tags (defined via
  `Tags/{tag}.md` but not yet on any page) are the orphan set.