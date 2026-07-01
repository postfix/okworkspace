import { useQuery } from "@tanstack/react-query";

import { getGraph, type GraphData } from "../api/client";

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
