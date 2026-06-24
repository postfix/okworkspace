# Phase 10: Graph UI - Context

**Gathered:** 2026-06-24
**Status:** Ready for planning
**Mode:** Smart discuss (autonomous) — three UX decisions resolved by the user; remainder pinned from v1.0 STACK/FEATURES research.

<domain>
## Phase Boundary

Ship the headline visual feature: an Obsidian-style **global graph view** and a docked **per-page local graph panel**, both interactive, with **configurable edge types** and hover-highlight. Consumes the Phase-9 graph endpoints (`/api/v1/graph`, `/api/v1/graph/local`). Delivers GRAPH-01..05.

**In scope:** global graph route + view; local graph panel; node sizing by degree; orphan distinction; click-to-open; pan/zoom; edge-type toggles (page links / backlinks / shared tags); hover-highlight of a node + its neighbors; depth control for the local panel.

**Out of scope:** any LLM tagging (Phases 11–12); tag/group coloring, in-graph search/filter, saved filters (v2 GRAPH-F1..F3); changing the Phase-9 endpoints (read them as-is — if a payload tweak is genuinely needed, keep it additive and lean).
</domain>

<decisions>
## Implementation Decisions

### Global graph location — USER DECISION: dedicated route + nav entry
- A full-page **`/app/graph`** route (react-router-dom), reachable from a nav/sidebar entry (mirror how other top-level destinations are linked in the AppShell/left nav).
- The graph route component is **lazy-loaded** (`React.lazy` + Suspense) so `react-force-graph-2d` does NOT bloat the initial editor bundle (research pitfall 6 / bundle-bloat guard).

### Local graph panel — USER DECISION: right-side collapsible panel
- A dockable **right-side collapsible panel** showing the current page + its direct neighbors (the local/neighborhood endpoint), interactive, auto-updating when the active page changes, with a **depth control (default 1 hop)**.
- Collapsible/toggleable so it doesn't crowd the editor when not wanted. Reuse existing panel/toggle patterns (e.g. the AgentPanel right-side pattern) + tokens.

### Node coloring & sizing — USER DECISION: uniform + accent for active
- Page nodes: one muted token color; the **current/active page** highlighted with the **accent** token. **Orphan** (unlinked, untagged) pages visually distinct (e.g. dimmer fill / outline). Node **size reflects degree** (connection count).
- Tag nodes (bipartite, from Phase 9): visually distinct from page nodes (e.g. a different muted shape/shade) but NOT a rainbow palette. Tag-group/folder coloring is deferred (v2 GRAPH-F1).

### Rendering library & edge toggles (from STACK research — LOCKED)
- **`react-force-graph-2d` (~v1.29.x)** — the 2D Canvas variant ONLY. Do NOT install the umbrella `react-force-graph` (pulls three.js → bundle bloat / breaks single-binary embed). Verify `three` never enters the lockfile.
- Edge-type toggles are **client state**, not a backend concern: three booleans (page links / backlinks / shared tags) in the existing **zustand** store; filter the edges array with `useMemo` before handing to the graph. **Shared-tag edges default OFF** (success criterion); page-links default ON.
- The Phase-9 payload already carries typed edges + tag nodes — the UI filters/renders, never recomputes.
- Canvas renderer (not SVG) for pan/zoom performance; let the force simulation settle (cooldown) then idle; click node → navigate to its page route; hover node → highlight it + immediate neighbors + connecting edges.

### Claude's Discretion
- Exact force params (charge, link distance, cooldown ticks), label-on-zoom threshold, the precise orphan visual treatment, node/tag shapes, and panel toggle affordance are at Claude's discretion (a short tuning spike is reasonable) — provided the success criteria hold and it matches the existing dark, muted, Obsidian-like aesthetic.
</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- Phase-9 endpoints + api client: `GET /api/v1/graph` (global, bipartite page+tag nodes, typed edges) and `GET /api/v1/graph/local?path=&depth=` (neighborhood). Add api client fns + react-query hooks (mirror `getBacklinks`/`useBacklinks`).
- `web/src/` AppShell + react-router routes (where `/app/page/:path` etc. live) — add the `/app/graph` route + a nav entry. The existing right-side AgentPanel is the analog for the local-graph dock.
- zustand store (existing UI state) — add the edge-toggle booleans. `@tanstack/react-query` for graph fetches. tokens.css for all colors/spacing (muted palette, accent for active).
- Existing page-navigation helper (used by the Phase-9 backlinks panel click-to-navigate) — reuse for node click.

### Established Patterns
- React 19 + Vite + react-router-dom 7; react-query for server state; zustand for ephemeral UI state; token-only CSS; vitest + tsc gates; lazy-load heavy routes.
- Canvas/CM6-style components manage their own imperative view in a useEffect/ref (LivePreviewEditor is the precedent for wrapping a non-React rendering engine — apply the same ref-lifecycle discipline to the ForceGraph2D instance).
- No `dangerouslyInnerHTML`; build CGO-free single binary; SPA embedded (rebuild SPA+binary to see changes live).

### Integration Points
- New `/app/graph` route + nav entry in AppShell/router.
- Right-side local-graph panel mounted in the page/editor view (alongside the AgentPanel area).
- `web/package.json`: ADD `react-force-graph-2d` (the ONLY new dep this phase) — `-2d` subpackage, verify no `three` in the lockfile.
- api client + react-query hooks for `/graph` and `/graph/local`; zustand edge-toggle slice.
</code_context>

<specifics>
## Specific Ideas

- Bundle discipline is load-bearing: lazy-load the graph route, install `react-force-graph-2d` (not the umbrella), and add a check that `three` is absent from the lockfile (research pitfall 6).
- The team are ex-Obsidian users — the graph should FEEL like Obsidian: force layout that settles, degree-sized nodes, hover-focus dimming the rest, click-to-open, smooth pan/zoom, depth slider on the local graph.
- Shared-tag edges OFF by default (toggle reveals them) so the first view isn't a hairball — this is why Phase 9 used the bipartite cap.
- Wrap the ForceGraph2D imperative instance with the same ref-lifecycle care as the CM6 editor; clean up on unmount.
</specifics>

<deferred>
## Deferred Ideas

- Tag-group / folder node coloring (GRAPH-F1), in-graph search/filter (GRAPH-F2), neighbors-of-neighbors local toggle (GRAPH-F3) — v2.
- 3D/WebGL graph — explicitly out (2D Canvas only).
- Saved graph filters / named color presets — v2.
</deferred>
