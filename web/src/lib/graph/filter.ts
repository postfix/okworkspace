// Pure edge filter (GRAPH-04/GF-01): keep/drop edges by the active toggle booleans,
// plus client-side node filtering (search, visibility toggles, orphan filter).
// DOM-free — the UI useMemos these over the payload before handing data to the
// canvas. Toggling is client-only; this never touches the network.
import type { GraphEdge, GraphNode } from "../../api/client";

// EdgeToggles is the active edge-type toggle set (the GRAPH-04 zustand slice's
// data shape, minus the action). Kept here so both the store and pure consumers
// share one type.
export interface EdgeToggles {
  links: boolean;
  backlinks: boolean;
  sharedTags: boolean;
}

// filterEdges returns the edges to render given the toggle state:
//   - "link" edges survive when EITHER links OR backlinks is on. The Phase-9
//     payload carries direction on link edges; "Links" shows forward-direction
//     links and "Backlinks" reveals the reverse-direction view. Because a single
//     link edge IS the page->page relation in both directions, an edge is kept
//     when at least one of the two link-facing toggles is on, and dropped only
//     when BOTH are off (the GRAPH-04 contract: turning both off hides links).
//   - "tag" edges (shared-tag/membership) survive only when sharedTags is on
//     (OFF by default so the first view is not a hairball).
export function filterEdges(
  edges: GraphEdge[],
  toggles: EdgeToggles,
): GraphEdge[] {
  const showLinks = toggles.links || toggles.backlinks;
  return edges.filter((e) => {
    if (e.type === "link") return showLinks;
    if (e.type === "tag") return toggles.sharedTags;
    return false;
  });
}

// NodeFilters is the set of node-visibility filters the UI exposes. All are
// client-side booleans that run against the already-fetched payload.
export interface NodeFilters {
  searchQuery: string;
  showTags: boolean;
  showAttachments: boolean;
  existingFilesOnly: boolean;
}

// matchesSearch returns true when the node matches the query. Search is
// case-insensitive substring against node label, id, and (when present) the
// server's extended fields aliases and tags. An empty query matches every node.
export function matchesSearch(node: GraphNode, query: string): boolean {
  const q = query.trim().toLowerCase();
  if (!q) return true;
  return (
    node.label?.toLowerCase().includes(q) ||
    node.id?.toLowerCase().includes(q) ||
    (node.aliases?.some((a) => a.toLowerCase().includes(q)) ?? false) ||
    (node.tags?.some((t) => t.toLowerCase().includes(q)) ?? false)
  );
}

// filterNodes applies search + visibility toggles to a node array. Returns the
// filtered nodes. Pure: no side effects, no DOM.
export function filterNodes(
  nodes: GraphNode[],
  filters: NodeFilters,
): GraphNode[] {
  return nodes.filter((n) => {
    if (!filters.showTags && n.type === "tag") return false;
    if (!filters.showAttachments && n.type === "attachment") return false;
    if (filters.existingFilesOnly && n.type === "page" && !n.exists)
      return false;
    if (!matchesSearch(n, filters.searchQuery)) return false;
    return true;
  });
}

// applyOrphanFilter removes nodes with zero visible edges (when showOrphans is
// false), plus any edges that referenced a removed node. Returns {nodes, edges}.
export function applyOrphanFilter(
  nodes: GraphNode[],
  edges: GraphEdge[],
  showOrphans: boolean,
): { nodes: GraphNode[]; edges: GraphEdge[] } {
  if (showOrphans) return { nodes, edges };
  const connected = new Set<string>();
  for (const e of edges) {
    connected.add(e.source);
    connected.add(e.target);
  }
  const filteredNodes = nodes.filter((n) => connected.has(n.id));
  const nodeIds = new Set(filteredNodes.map((n) => n.id));
  const filteredEdges = edges.filter(
    (e) => nodeIds.has(e.source) && nodeIds.has(e.target),
  );
  return { nodes: filteredNodes, edges: filteredEdges };
}
