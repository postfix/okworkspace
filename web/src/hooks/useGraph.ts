import { useQuery } from "@tanstack/react-query";

import { getGraph, getLocalGraph, type GraphData } from "../api/client";

// useGraph fetches the whole-workspace graph (GRAPH-01). queryKey ["graph",
// "global"] follows the codebase convention (["tree"], ["backlinks", path]).
// staleTime keeps it cached briefly so a quick back/forward to /app/graph does
// not refetch. Mirrors useBacklinks.
export function useGraph() {
  return useQuery<GraphData>({
    queryKey: ["graph", "global"],
    queryFn: () => getGraph(),
    staleTime: 30_000,
  });
}

// useLocalGraph fetches the neighborhood around `path` to `depth` hops (GRAPH-03).
// The queryKey carries BOTH path and depth so react-query auto-refetches when the
// active page route changes OR the depth control changes — no manual invalidation.
// Gated on a non-empty path so an unresolved route makes no request (mirrors
// useBacklinks' enabled gate).
export function useLocalGraph(path: string, depth: number) {
  return useQuery<GraphData>({
    queryKey: ["graph", "local", path, depth],
    queryFn: () => getLocalGraph(path, depth),
    enabled: path !== "",
    staleTime: 30_000,
  });
}
