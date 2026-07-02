---
author: architect
responsible: architect
phase: 34
phase_type: implementation
status: Ready for planning
hard_bar: false
security_relevant: false
design_fork: false
parent_artifacts:
  - .planning/ROADMAP.md
  - .planning/REQUIREMENTS.md
---

# Phase 34: Tag Usage Analytics - Context

**Gathered:** 2026-07-02
**Status:** Ready for planning

<domain>
## Phase Boundary

An admin analytics view over the existing `graph.Store.TagPageCounts` query —
per-tag page counts (sortable), most/least used, orphan tags, totals. Read-only,
no new heavy scan, no mutation. The cheapest tag-track phase.

**Requirements:** TF-04

**Non-goals:** mutation of any kind (analytics is read-only); a new index or scan
(reuses the existing `GROUP BY` query); body-`#tag` counting (scope is frontmatter
`page_tags` cache only, stated in the view).

**Depends on:** — (the `TagPageCounts` query already exists at `query.go:453`).
</domain>

<decisions>
## Implementation Decisions

### Data source (D-01) — reuse TagPageCounts
- **D-01:** The analytics view sources from `graph.Store.TagPageCounts(ctx)`
  (`query.go:453`), which returns `map[string]int` (tag → page count). No new
  query, no new scan. The handler computes:
  - Per-tag page counts (sortable by count or name).
  - Most-used tags (top N by count).
  - Unused/orphan tags (count == 0 — tags in the vocabulary with no pages, or tags
    that exist as frontmatter on zero pages; the vocabulary comes from `handleTagList`).
  - Totals: total tags, total tagged pages, total untagged pages (untagged = total
    pages minus tagged pages; total pages from `pages.Service.Tree()` or the graph's
    live-page set).
- **D-01a (scope statement):** the view states its scope: "Counts reflect frontmatter
  tags (the page_tags derived cache). Body-only #tags not yet in frontmatter are
  counted only after materialization." This is a static text note in the UI.

### Endpoint (D-02) — extend handleTagList or new GET
- **D-02:** Extend `handleTagList` (`handlers_tags_admin.go:507`) to include the
  analytics summary (totals, most/least used, orphans) alongside the existing
  per-tag list. OR add a separate `GET /admin/tags/analytics` endpoint. Recommend
  extending `handleTagList` — it already returns the tag list with counts; adding
  summary fields to the response is simpler than a new endpoint. The response shape:
  ```json
  {
    "tags": [{tag, count, color, description, parent}, ...],
    "summary": {
      "total_tags": N,
      "total_tagged_pages": M,
      "total_untagged_pages": P,
      "most_used": [{tag, count}, ...],
      "least_used": [{tag, count}, ...],
      "orphans": [{tag, count: 0}, ...]
    }
  }
  ```
  The planner decides the exact shape; recommend extending `handleTagList` with an
  optional `?analytics=true` query param, or a separate endpoint if the response
  gets too heavy. A separate `GET /admin/tags/analytics` is cleaner — the planner
  picks (recommend separate, for clean separation of the list vs analytics views).

### Frontend (D-03) — analytics panel/tab in TagManagement
- **D-03:** `TagManagement.tsx` gains an "Analytics" tab or panel showing:
  - Totals (total tags, tagged pages, untagged pages) as summary cards/numbers.
  - Per-tag page counts table (sortable by count or name — client-side sort).
  - Most-used + least-used sections (top 5 each).
  - Orphan tags section (tags with 0 pages).
  - The scope note (static text).
  The analytics data comes from `getTagAnalytics()` (new client function) or
  extends `listTags()` response.

### The agent's Discretion
- Whether to extend `handleTagList` or add a separate `GET /admin/tags/analytics`
  endpoint (recommend separate).
- The exact analytics UI layout (tabs vs panels vs cards — keep it simple).
- Client-side sort implementation (use existing table sort patterns if any).
- Commit slicing.
</decisions>

<disciplines>
## Lenses surfaced for this phase

No disciplines surfaced — phase scope didn't call for any. The read-only constraint
is a must-have (belongs in PLAN `must_haves`), not a lens.

</disciplines>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Phase spec & requirements
- `.planning/ROADMAP.md` §"Phase 34: Tag Usage Analytics" — goal, success criteria.
- `.planning/REQUIREMENTS.md` §TF-04 (lines 156-167) — acceptance criteria.

### The query to reuse
- `internal/graph/query.go:449-455` (`TagPageCounts` — returns `map[string]int`).
- `internal/server/handlers_tags_admin.go:507-529` (`handleTagList` — the existing
  tag list endpoint to extend or sit beside).

### Frontend
- `web/src/components/admin/TagManagement.tsx` — add analytics tab/panel.
- `web/src/api/client.ts` — add `getTagAnalytics` (or extend `listTags`).

### No external specs
- No third-party specs/ADRs.
</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `graph.Store.TagPageCounts` (`query.go:453`) — the existing per-tag page count
  query. No new query needed.
- `handleTagList` (`handlers_tags_admin.go:507`) — the existing admin tag list;
  extend or sit beside.
- `TagManagement.tsx` — the admin tag surface; add an analytics view.

### Integration Points
- Router: `admin.Get("/admin/tags/analytics", h.handleTagAnalytics)` (if separate
  endpoint) in the admin tag subgroup (`router.go:343`).
- Frontend: new `getTagAnalytics` client function; analytics panel in
  `TagManagement.tsx`.

### Landmines
- **Do NOT mutate** — analytics is strictly read-only. No write path.
- **State the scope** — the view must state that counts reflect frontmatter tags
  only (body-only #tags are not counted until materialized).
</code_context>

<specifics>
## Specific Ideas

- The untagged-pages count: total live pages (from `pages.Service.Tree()` or the
  graph's live-page set) minus tagged pages (from `TagPageCounts`). A page is
  "tagged" if it appears in any `page_tags` row.
</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope.
</deferred>

---

*Phase: 34-tag-usage-analytics*
*Context gathered: 2026-07-02*