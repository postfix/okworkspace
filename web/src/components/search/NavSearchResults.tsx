import { useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import { Loader2 } from "lucide-react";

import type { SearchResult, SearchResultKind } from "../../api/client";
import { useSearch } from "../../hooks/useSearch";
import { useNavSearch } from "../../stores/navSearch";
import SearchResultRow from "./SearchResultRow";
import "./NavSearchResults.css";

// Display order + label for the result groups (empty groups omitted).
const GROUP_ORDER: { kind: SearchResultKind; label: string }[] = [
  { kind: "page", label: "Pages" },
  { kind: "heading", label: "Headings" },
  { kind: "attachment", label: "Attachments" },
];

// NavSearchResults renders full-text search matches INLINE in the navrail body
// (the panel that shows the file tree when the query is empty). It replaces the
// old ⌘K modal palette: the same /api/v1/search results, grouped Pages / Headings
// / Attachments, but in the sidebar. Opening a result navigates in-app AND clears
// the query so the tree returns with the opened page in view.
//
// The parent (AppShell) only renders this when the query is non-empty, so a
// non-empty query is a precondition here.
export default function NavSearchResults() {
  const navigate = useNavigate();
  const query = useNavSearch((s) => s.query);
  const setQuery = useNavSearch((s) => s.setQuery);
  const { data, isLoading, isError } = useSearch(query);

  const [showLoading, setShowLoading] = useState(false);

  // Group by kind in display order; skip empty groups.
  const groups = useMemo(() => {
    const rows = data ?? [];
    return GROUP_ORDER.map((g) => ({
      ...g,
      rows: rows.filter((r) => r.kind === g.kind),
    })).filter((g) => g.rows.length > 0);
  }, [data]);

  const total = data?.length ?? 0;

  // Delayed loading indicator (~150ms) to avoid flicker on fast responses.
  useEffect(() => {
    if (!isLoading) {
      setShowLoading(false);
      return;
    }
    const id = window.setTimeout(() => setShowLoading(true), 150);
    return () => window.clearTimeout(id);
  }, [isLoading]);

  function openResult(r: SearchResult) {
    if (r.kind === "heading" && r.anchor) {
      // Strip the server anchor's leading '#' for the DOM id (rehype-slug ids
      // have none); keep it in the route hash so a copied deep link stays valid.
      const id = r.anchor.replace(/^#/, "");
      navigate(`/app/page/${r.path}#${id}`);
      window.requestAnimationFrame(() => {
        document.getElementById(id)?.scrollIntoView();
      });
    } else {
      navigate(`/app/page/${r.path}`);
    }
    // Clear the query so the tree returns with the opened page highlighted.
    setQuery("");
  }

  if (isError) {
    return (
      <div className="navsearch-status" role="alert">
        Search is unavailable. Try again in a moment.
      </div>
    );
  }

  if (showLoading && total === 0) {
    return (
      <div className="navsearch-status" aria-live="polite">
        <Loader2 size={14} className="spinner" aria-hidden="true" />
        <span>Searching…</span>
      </div>
    );
  }

  if (total === 0) {
    return (
      <p className="navsearch-status" aria-live="polite">
        No matches for “{query.trim()}”.
      </p>
    );
  }

  return (
    <div className="navsearch-results" aria-label="Search results">
      {groups.map((g) => (
        <div key={g.kind} className="navsearch-group">
          <div className="navsearch-group-label">{g.label}</div>
          {g.rows.map((r) => (
            <SearchResultRow
              key={`${r.kind}:${r.path}:${r.anchor ?? ""}:${r.title}`}
              result={r}
              id={`navsearch-${r.kind}-${r.path}-${r.anchor ?? ""}`}
              active={false}
              onActivate={() => {}}
              onOpen={() => openResult(r)}
            />
          ))}
        </div>
      ))}
    </div>
  );
}
