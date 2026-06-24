// Pure edge filter (GRAPH-04): keep/drop edges by the active toggle booleans.
// DOM-free — the UI useMemos this over the payload edges before handing them to
// the canvas. Toggling is client-only; this never touches the network.
import type { GraphEdge } from "../../api/client";

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
