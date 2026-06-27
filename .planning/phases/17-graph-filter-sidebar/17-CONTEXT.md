---
author: architect
---

# Phase 17: Graph Filter Sidebar - Context

**Gathered:** 2026-06-27
**Status:** Ready for planning

<domain>
## Phase Boundary

Add filtering and display controls to the global graph page (`/app/graph`) — living flat in the same panel as the existing Links/Backlinks/Shared Tags edge-type toggle chips. No collapsing, no separate sections, no sidebar.

**In scope:** Search input, 4 visibility toggles (Tags, Attachments, Existing files only, Orphans), "New group" button, 4 display controls (Arrows toggle, Text fade threshold slider, Node size slider, Link thickness slider), reset button, localStorage persistence, live-updating filtering pipeline.

**Out of scope:** Backend changes (filtering is client-side on already-fetched graph data), graph group CRUD beyond the "New group" button, local graph dock changes, mobile-specific layout.
</domain>

<decisions>
## Implementation Decisions

### Layout
- **D-01:** All new controls live in the same panel as the existing Links/Backlinks/Shared Tags edge-type chips on the GraphView page. One flat area, no collapsing sections.
- **D-02:** Search input goes above or alongside the existing toggle chips. Display sliders go below the toggles.

### State Management
- **D-03:** New Zustand store `web/src/stores/graphFilters.ts` for filter state (search query, visibility toggles, display settings, groups). Separate from `graphEdges.ts` (edge-type toggles remain separate concern).
- **D-04:** Default state: `searchQuery: ""`, `showTags: true`, `showAttachments: false`, `existingFilesOnly: false`, `showOrphans: true`, `showArrows: false`, `textFadeThreshold: 0.5`, `nodeSize: 0.2`, `linkThickness: 0.2`, `groups: []`.
- **D-05:** Persist filter state to localStorage key `"graph-filter-state"`. Hydrate on mount.

### Filtering Pipeline
- **D-06:** Filtering runs client-side on the already-fetched graph data (from `useGraph` hook). Deterministic order: node visibility filter → edge filter (only edges where both endpoints visible) → orphan filter (if orphans hidden) → display settings applied to remaining nodes/edges.
- **D-07:** Search matches against node `label`, `path`, `aliases`, and `tags` (case-insensitive substring).

### Display Controls
- **D-08:** Node size slider maps 0–1 to 2px–12px radius. Link thickness maps 0–1 to 0.5px–5px stroke. Text fade threshold maps 0–1 to zoom-level opacity cutoff.
- **D-09:** Arrows toggle controls `linkDirectionalArrowLength` on the force-graph instance.

### Groups
- **D-10:** "New group" button creates a group with default name "New group", random color from a preset palette, empty query. Full group CRUD (edit name/color/query, delete) is stretch — group creation alone is MVP.

### Styling
- **D-11:** Match existing GraphView panel styling. No new CSS variables needed unless the existing panel doesn't cover the new controls.
</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Existing Graph UI (what we're extending)
- `web/src/components/graph/GraphView.tsx` — Global graph route, renders edge toggles + GraphCanvas
- `web/src/components/graph/GraphCanvas.tsx` — Imperative Canvas wrapper (react-force-graph-2d)
- `web/src/stores/graphEdges.ts` — Zustand store for edge-type toggle state
- `web/src/hooks/useGraph.ts` — Data-fetching hook for `/api/v1/graph`

### Requirements
- `.planning/REQUIREMENTS.md` §GF-01 — 15 acceptance criteria for graph filter sidebar
</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `graphEdges.ts` store pattern — copy for `graphFilters.ts`
- Edge toggle chips in `GraphView.tsx` — new toggles follow same pill-button pattern
- `useGraph.ts` — fetches graph data; filtering pipeline runs on the returned data
- `GraphCanvas.tsx` — exposes force-graph instance ref for display settings (node size, link width, arrows)

### Integration Points
- `GraphView.tsx` main panel — mount point for all new controls
- `GraphCanvas.tsx` — receives filtered data + display settings as props
- `useGraph.ts` — hook into returned data for client-side filtering
</code_context>

<deferred>
## Deferred Ideas

- Graph group CRUD beyond "New group" button (edit, delete, reorder)
- Mobile-specific filter panel layout
- Server-side filtering (add search/filter params to `/api/v1/graph`)
- Filter controls on local graph dock panel
</deferred>

---
*Phase: 17-graph-filter-sidebar*
*Context gathered: 2026-06-27*
