---
author: codebase-archaeologist
responsible: codebase-archaeologist
phase: 17
milestone: v1.1
status: verified
confidence: smtc_honest
---

# Codebase Map: Graph Filter Sidebar

## Subsystem: Client-Side Graph Filtering & Display Controls

**Added in:** Phase 17 (GF-01). Entirely client-side — no backend changes.

### Files

| File | Role |
|------|------|
| `web/src/stores/graphFilters.ts` | Zustand store: search query, 4 visibility toggles, 3 display sliders, arrows toggle, groups array, reset action. Persisted to localStorage under key `okf.graph.filters`. |
| `web/src/components/graph/FilterControls.tsx` | UI component: search input, 4 toggle chips (Tags, Attachments, Existing files only, Orphans), Arrows toggle, 3 range sliders (Text fade threshold, Node size, Link thickness), "New group" button, Reset button. All controls read/write through `useGraphFilters`. |
| `web/src/components/graph/FilterControls.css` | Dark-theme styling using project CSS variables with fallbacks. |
| `web/src/lib/graph/filter.ts` | Pure functions: `matchesSearch` (case-insensitive substring on label/id), `filterNodes` (applies search + visibility toggles), `applyOrphanFilter` (drops nodes with 0 visible edges). |
| `web/src/components/graph/GraphView.tsx` | Wired filtering pipeline: `filterNodes` -> `filterEdges` (from existing graphEdges store) -> `applyOrphanFilter` -> display settings passed as props to `GraphCanvas`. |
| `web/src/components/graph/GraphCanvas.tsx` | Added `linkDirectionalArrowLength` prop, passed to `ForceGraph2D`. |

### Filtering Pipeline (deterministic order)

```
data.nodes + data.edges
  -> filterNodes(search + showTags + showAttachments + existingFilesOnly)
  -> filterEdges(links + backlinks + sharedTags) intersect visibleNodeIds
  -> applyOrphanFilter(showOrphans)
  -> display settings (nodeRelSize, linkWidth, linkDirectionalArrowLength, labelZoom)
  -> GraphCanvas
```

### Display Settings

| Control | Formula | Default | Mapped Value |
|---------|---------|---------|--------------|
| Node size (0-1) | `2 + nodeSize * 8` | 0.2 | 3.6px |
| Link thickness (0-1) | `0.5 + linkThickness * 4.5` | 0.2 | 1.4px |
| Text fade threshold (0-1) | `0.5 + threshold * 4` | 0.5 | ~2.5x zoom cutoff |
| Arrows | `showArrows ? 4 : 0` | false | 0px (hidden) |

### Data Model Constraint

The backend `GraphNode` returns only `{id, label, type}` where `type` is `"page"` or `"tag"`. This means:

- **Attachments toggle is non-functional** — the backend never returns `type: "attachment"`.
- **Existing files only toggle is non-functional** — the backend never sends an `exists` field (all nodes are existing files by construction).
- **Search aliases/tags matching is dead code** — the backend doesn't supply per-node `aliases` or `tags` fields.

These were noted as gaps (medium severity) in the Phase 17 verification. The architect can decide whether to remove the inert toggles or extend the backend to support them.

### Callers

- `GraphView` mounts `<FilterControls />` inline in the header alongside `<EdgeToggles />`.
- `useGraphFilters()` (from `graphFilters.ts`) provides reactive state and `reset()`.
- `filter.ts` functions are pure and called from `GraphView` `useMemo` chains.

### Blast Radius

Changes to `web/src/stores/graphFilters.ts` affect `FilterControls` and `GraphView`. Changes to `filter.ts` affect `GraphView` only. Changes to `GraphCanvas` props affect component instantiation only (force-simulation re-render is internal).
