// Pure graph model helpers (GRAPH-02): node degree + orphan classification.
// DOM-free, no React, no canvas — data in, data out. The canvas consumes these
// to size nodes by degree and pick the orphan fill; it never recomputes adjacency.
import type { GraphEdge, GraphNode } from "../../api/client";

// computeDegrees returns a map node-id -> degree, where degree is the number of
// edges touching the node as source OR target. Every node in `nodes` is present
// in the result (degree 0 for nodes no edge references), so a lookup never misses
// a real node. Edges that reference an id not in `nodes` still increment that id
// (defensive — the payload is bipartite-consistent, but a stray id won't crash).
export function computeDegrees(
  nodes: GraphNode[],
  edges: GraphEdge[],
): Map<string, number> {
  const degrees = new Map<string, number>();
  for (const n of nodes) {
    degrees.set(n.id, 0);
  }
  const bump = (id: string) => {
    degrees.set(id, (degrees.get(id) ?? 0) + 1);
  };
  for (const e of edges) {
    bump(e.source);
    bump(e.target);
  }
  return degrees;
}

// isOrphan reports whether a node is an orphan PAGE: a page with no edges at all
// (no links AND no tags — degree 0). Tag nodes are NEVER orphans (a tag with no
// members simply isn't in the payload). A node id missing from the degree map is
// treated as degree 0 (defensive — a page node should always be present).
export function isOrphan(
  nodeId: string,
  degrees: Map<string, number>,
  nodeType: GraphNode["type"],
): boolean {
  if (nodeType !== "page") return false;
  return (degrees.get(nodeId) ?? 0) === 0;
}
