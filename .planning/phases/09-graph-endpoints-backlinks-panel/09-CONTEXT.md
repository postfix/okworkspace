# Phase 9: Graph Endpoints & Backlinks Panel - Context

**Gathered:** 2026-06-24
**Status:** Ready for planning
**Mode:** Smart discuss (autonomous) — two grey areas resolved by the user; remainder pinned from v1.0 ARCHITECTURE research.

<domain>
## Phase Boundary

Expose the Phase-8 derived adjacency store over HTTP as typed-edge graph payloads, and ship the first user-visible output: a page-view **"Referenced by" backlinks panel** (LINK-02). This phase delivers the read API the Phase-10 graph UI consumes, plus the backlinks panel.

**In scope:** a global graph endpoint (all pages as nodes, typed edges), a local graph endpoint (a page's neighborhood), shared-tag edges via the bipartite model with a popular-tag cap, and the collapsible backlinks section in the page view.

**Out of scope:** the graph visualization UI / canvas / edge-type toggles / hover (Phase 10 — GRAPH-01..05); any LLM tagging (Phases 11–12). This phase's only UI is the backlinks panel.
</domain>

<decisions>
## Implementation Decisions

### Shared-tag edge model — USER DECISION: bipartite page↔tag nodes
- Model tags as their OWN node type. The payload contains page nodes (`type:"page"`) AND tag nodes (`type:"tag"`), connected by page→tag membership edges (`type:"tag"`). This yields K edges per tag, NOT K² (no clique blow-up).
- Still apply a **popular-tag cap**: exclude/flag tags that appear on more than a threshold share of pages (e.g. a tag on >25% of pages, or an absolute cap) so a ubiquitous tag node doesn't become a hub hairball. Make the threshold a named constant.
- Edge `type` field distinguishes `link` (page→page forward), `backlink` (reverse — may be derived client-side from links, but the endpoint should expose direction), and `tag` (page→tag membership). The Phase-10 UI toggles these; this phase just emits them typed.
- Backlinks for the panel come from the reverse query on `page_links` (Phase 8), independent of the graph payload.

### Backlinks panel — USER DECISION: collapsible section below the page
- An Obsidian-style **"Referenced by (N)"** collapsible block rendered at the BOTTOM of the page view (read mode), below the rendered Markdown body.
- Lists every page that links to the current page; each entry is click-to-navigate (uses the existing page-navigation route). Empty state: hide or show a quiet "No backlinks yet".
- Keep it additive to the existing PageView/LivePreviewEditor read surface — do not disturb the CM6 read rendering.

### Endpoint design (from ARCHITECTURE research — standard pattern)
- `GET /api/v1/graph` — global: all page+tag nodes and all typed edges, in a LEAN JSON payload (ids + minimal labels; no bodies). Read from the Phase-8 `page_links`/`page_tags` cache tables, never re-parsing files. Server-side cache/shape so the payload stays small.
- `GET /api/v1/graph/local?path=<page>&depth=<n>` — neighborhood: the page plus its direct neighbors (depth default 1). Depth-limited to keep payloads small.
- Both live in the authed (read) chi group, mirroring the existing `/tree` and `/search` endpoints (NOT editor/admin — any authenticated user can read the graph).
- Backlinks for the panel: either a dedicated `GET /api/v1/pages/{path}/backlinks` or fold into the existing page GET response — planner's choice, but keep it a lean read consistent with existing page endpoints.

### Claude's Discretion
- Exact JSON schema field names, the precise popular-tag threshold value, whether backlinks ride the page GET or a dedicated endpoint, and the depth default — all at Claude's discretion within the contracts above.
</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- Phase-8 `internal/graph` Store: `page_links` (forward edges, reverse = backlinks) and `page_tags` (membership) — the source for all graph payloads. Read-only queries here; no new maintenance.
- `internal/server/router.go` authed group (where `/tree`, `/search` live) — mount `/graph` + `/graph/local` here; mirror those handlers' shape (`internal/server/handlers_tree.go`, `handlers_search.go`).
- Page view read surface: `web/src/` PageView / LivePreviewEditor read mode (Phase 6/7) + the existing page-navigation route — the backlinks panel attaches here.
- `@tanstack/react-query` for fetching graph/backlinks; existing api client (`web/src/api/client.ts`) for the new endpoints.

### Established Patterns
- Authed read endpoints return lean JSON; handlers in `internal/server/handlers_*.go`; routes in `router.go`.
- Frontend: react-query hooks for server state; components in `web/src/`; vitest + tsc gates.
- No new Go dependency (graph payload built from existing tables); one new frontend area (backlinks panel) — no new frontend dependency needed (the force-graph lib is Phase 10).

### Integration Points
- `internal/graph` Store query methods (may need new read methods: AllNodesEdges, Neighborhood, BacklinksFor) — add read queries to the Phase-8 package.
- `internal/server` new handlers + routes in the authed group.
- `web/src/` page view: add the collapsible "Referenced by" section + a react-query hook hitting the backlinks endpoint.
</code_context>

<specifics>
## Specific Ideas

- The global payload must stay lean — ids + labels only, read from cache tables, no file reads, no bodies. The popular-tag cap is load-bearing for keeping tag nodes from becoming hubs.
- Backlinks panel is the FIRST user-visible v1.0 output — it should feel native to the existing Obsidian-style read surface.
- Typed edges (`link`/`backlink`/`tag`) and tag nodes set up Phase 10's toggles + rendering — get the schema right here so Phase 10 only renders/filters, never recomputes.
</specifics>

<deferred>
## Deferred Ideas

- Graph visualization (canvas/force-directed), edge-type toggles, hover-highlight, local-graph panel UI — Phase 10.
- Backlink context snippets (the linking sentence) — v2 (LINK-F1).
- Right-side dockable panel layout — considered; chose the simpler below-page collapsible for Phase 9 (the right-side panel can come with the Phase-10 local-graph panel if desired).
</deferred>
