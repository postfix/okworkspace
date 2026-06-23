import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";

import { search, type SearchResult } from "../api/client";

// useDebouncedValue returns `value` only after it has stopped changing for `ms`.
// It keeps the active react-query key from churning on every keystroke (so we
// fire one request per pause, not per character).
export function useDebouncedValue<T>(value: T, ms: number): T {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const id = window.setTimeout(() => setDebounced(value), ms);
    return () => window.clearTimeout(id);
  }, [value, ms]);
  return debounced;
}

// useSearch wraps useQuery with a debounced query string. queryKey ["search", q]
// follows the codebase convention (["tree"], ["me"], ["page", path]). The query
// is gated on a non-empty (trimmed) q so an empty palette makes no request;
// placeholderData keeps the prior results visible while a new query loads, which
// avoids the panel resizing between keystrokes (UI-SPEC: dim-over-replace).
export function useSearch(rawQuery: string) {
  const q = useDebouncedValue(rawQuery, 200);
  return useQuery<SearchResult[]>({
    queryKey: ["search", q],
    queryFn: () => search(q),
    enabled: q.trim().length > 0,
    staleTime: 30_000,
    placeholderData: (prev) => prev,
  });
}
