---
author: qa
responsible: qa
phase: 34
phase_type: implementation
gate_result:
  id: qa-phase-34-001
  verdict: pass
  status: final
  provenance:
    reviewer: qa
    method: goal-backward, code-reality-grounded (SMTC-free manual walk + test runs)
    commit: 358bbc3
---

# Phase 34: Tag Usage Analytics — Verification

## Goal (from PLAN)

An admin analytics view over `graph.Store.TagPageCounts` — per-tag page
counts, most/least used, orphans, totals. Read-only, no new heavy scan.

**Requirement:** TF-04 (`.planning/REQUIREMENTS.md` §TF-04, lines 156-167).

## Acceptance handshake

- **Inputs complete** — PLAN + SUMMARY + REQUIREMENTS §TF-04 all present and
  consistent. SUMMARY claims match PLAN tasks 1 & 2.
- **Goal verifiable** — "per-tag counts, most/least used, orphans, totals,
  read-only, admin-only" is fully verifiable via test + code walk.
- **Lenses engageable** — wp/read-only-check via grep over handler body;
  behavior via the three `TestTagAnalytics*` tests.

Verdict on inputs: **verifiable** — proceeding to goal-backward check.

## Evidence — verified against code reality (commit 358bbc3)

### TF-04-1: Admin analytics view surfaces per-tag counts, most/least used, orphans, totals — sourced from TagPageCounts, no new scan

**Backend handler** — `internal/server/handlers_tags_admin.go:798-872`
(`handleTagAnalytics`, GET `/admin/tags/analytics`):
- Calls `h.graph.TagPageCounts(ctx)` (line 804) → `map[string]int`.
- Calls `h.graph.TaggedPageCount(ctx)` (line 809) for distinct tagged-page total.
- Computes: `total_tags` (len counts), `total_pages` (from `pages.Tree()` page
  walk via `countPageNodes`, fallback to `taggedPages` when pages nil),
  `tagged_pages`, `untagged_pages`, `most_used` (top 5 desc via `topNByCount`),
  `least_used` (bottom 5 asc), `orphans` (metadata-only tags with count 0, from
  `tagmeta.AllMeta`), `tags` (flat alphabetical list).
- All arrays are non-nil empty slices (JSON `[]`, not `null`) — lines 852-860.

**Source query** — `internal/graph/query.go:403-421` (`tagPageCounts`):
`SELECT tag, COUNT(DISTINCT page_path) FROM page_tags GROUP BY tag` — the
EXISTING `page_tags` cache query (TM-04). `TagPageCounts` (line 453) is a
thin wrapper over it. `TaggedPageCount` (line 460) is a new lightweight
`SELECT COUNT(DISTINCT page_path) FROM page_tags` — same derived cache, no new
heavy scan. Both are read-only SELECTs on the DERIVED `page_tags` cache.

**Route** — `internal/server/router.go:352`:
`admin.Get("/admin/tags/analytics", h.handleTagAnalytics)` — registered inside
the admin group (`router.go:308-309`: `admin.Use(auth.RequireRole(auth.RoleAdmin))`),
inheriting the SAME RequireRole(admin) gate as every other admin tag route.

**Test coverage** — `internal/server/handlers_tags_admin_test.go`:
- `TestTagAnalyticsRBAC` (1137) — editor → 403; admin → 200.
- `TestTagAnalytics` (1155) — seeds ops×2, docs×1, one untagged page, one
  metadata-only orphan tag; asserts total_tags=2, total_pages=3,
  tagged_pages=2, untagged_pages=1, most_used[0]={ops,2}, least_used contains
  {docs,1}, orphans contains {orphan-tag,0}, tags has ops=2 + docs=1.
- `TestTagAnalyticsEmpty` (1251) — no pages/tags → all zeros, empty arrays
  (len(MostUsed)=0, len(LeastUsed)=0, len(Orphans)=0, len(Tags)=0).

**Frontend API** — `web/src/api/client.ts:1385-1400`:
`TagAnalyticsSummary` interface + `getTagAnalytics()` GET
`/api/v1/admin/tags/analytics` with `credentials: "same-origin"`.

**Frontend panel** — `web/src/components/admin/TagManagement.tsx:232-360`
(`AnalyticsPanel`):
- Summary cards: total tags, tagged pages, untagged pages, total pages.
- Most-used + least-used sections (top 5 each from backend).
- Orphan tags section (metadata-only, 0 pages).
- Per-tag counts table with client-side sort (click "Tag" or "Pages" header →
  toggles sortKey/sortDir, lines 250-267, 329-352).
- Tab toggle (lines 42-58) between "Tags" and "Analytics" with ARIA
  `role="tablist"` / `role="tab"` / `aria-selected`.
- Data via `useQuery(getTagAnalytics)` with `ANALYTICS_KEY` (line 235-237).
- CSS: `web/src/components/admin/TagManagement.css` (16 `tagmgmt-analytics*`
  rules, lines 317-421).

**TF-04-1: CONFIRMED.**

### TF-04-2: View states scope (frontmatter page_tags cache; body-only #tags after materialization)

- Backend handler doc comment (line 796-797): "The scope is frontmatter
  page_tags only; body-only #tags are counted only after materialization."
- Backend type doc (line 779-780): same scope statement.
- Frontend scope note — `TagManagement.tsx:354-357`:
  > "Counts reflect frontmatter tags (the page_tags cache). Body-only #tags
  > are counted only after materialization."
- Client.ts interface doc (line 1383-1384): same scope statement.

**TF-04-2: CONFIRMED.**

### TF-04-3: Admin-only, read-only (no mutation), efficient

**Admin-only:** route is inside the `admin.Group` with
`admin.Use(auth.RequireRole(auth.RoleAdmin))` (router.go:308-309, 352).
`TestTagAnalyticsRBAC` confirms editor → 403, admin → 200.

**Read-only:** handler body (lines 798-872) contains only:
- `h.graph.TagPageCounts(ctx)` (read)
- `h.graph.TaggedPageCount(ctx)` (read)
- `h.pages.Tree(ctx)` (read)
- `h.tagmeta.AllMeta(ctx)` (read)
- `writeError` / `writeJSON` (response helpers, not mutations)
- No `Write`/`Post`/`Delete`/`Apply`/`Commit`/`SetTagsFrontmatter`/`ApplyTagsBatch`
  calls — verified by grep over the handler body range (exit 0, no mutation
  matches).

**Efficient:** both source queries are single-statement SELECTs against the
derived `page_tags` SQLite cache (GROUP BY / COUNT DISTINCT). No page-body
scan, no file walk for counts. `total_pages` uses the already-flat `Tree()`
leaf walk. At ~5-user, modest-page scale this is constant-overhead per request.

**TF-04-3: CONFIRMED.**

## Test runs (executed against committed code, 358bbc3)

```
$ go test ./internal/server/ -run TestTagAnalytics -count=1 -v
=== RUN   TestTagAnalyticsRBAC
--- PASS: TestTagAnalyticsRBAC (0.10s)
=== RUN   TestTagAnalytics
--- PASS: TestTagAnalytics (0.09s)
=== RUN   TestTagAnalyticsEmpty
--- PASS: TestTagAnalyticsEmpty (0.05s)
PASS
ok  	github.com/postfix/okworkspace/internal/server	0.278s
```

## Build runs

```
$ go build ./...
go build exit:0

$ cd web && npx tsc --noEmit
tsc exit:0
```

## Verdict

**passed** — Phase 34 delivers the TF-04 goal: an admin-only, read-only
analytics view surfacing per-tag page counts (sortable), most/least used,
orphans, and totals, sourced from the existing `graph.Store.TagPageCounts`
query (no new heavy scan). The view states its scope (frontmatter `page_tags`
cache; body-only #tags counted after materialization). All three tests pass;
go build + tsc --noEmit both clean. No gaps found.

## Gaps

None.