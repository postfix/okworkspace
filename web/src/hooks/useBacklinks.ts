import { useQuery } from "@tanstack/react-query";

import { getBacklinks, type Backlink } from "../api/client";

// useBacklinks wraps useQuery to fetch the pages that reference `path`. queryKey
// ["backlinks", path] follows the codebase convention (["tree"], ["page", path],
// ["search", q]). Unlike useSearch there is NO debounce — backlinks fetch once on
// page load, not per keystroke. The query is gated on a non-empty path so an
// unresolved route makes no request; staleTime keeps it cached briefly so a quick
// back/forward doesn't refetch.
export function useBacklinks(path: string) {
  return useQuery<Backlink[]>({
    queryKey: ["backlinks", path],
    queryFn: () => getBacklinks(path),
    enabled: path !== "",
    staleTime: 30_000,
  });
}
