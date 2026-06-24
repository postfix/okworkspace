import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { ChevronDown, ChevronRight, FileText } from "lucide-react";

import { useBacklinks } from "../hooks/useBacklinks";
import "./BacklinksPanel.css";

// BacklinksPanel renders the collapsible "Referenced by (N)" section at the bottom
// of the page read view (LINK-02). It consumes the 09-01 backlinks endpoint via
// useBacklinks and reuses the existing nav-row classes so each entry is visually
// identical to the left tree / recent rows. Titles render as React text children
// (NEVER dangerouslySetInnerHTML) — the locked stored-XSS guard (T-09-06). The
// empty/loading/error states are quiet muted single lines per the UI-SPEC copy
// contract; the panel never blocks the page body (additive only).
export default function BacklinksPanel({ path }: { path: string }) {
  const navigate = useNavigate();
  const { data, isLoading, isError } = useBacklinks(path);
  // Default expanded — the backlinks are the value. The user can collapse; toggle
  // persistence is not required this phase (local useState only).
  const [open, setOpen] = useState(true);

  const count = data?.length ?? 0;

  return (
    <section className="backlinks-panel">
      <button
        type="button"
        className="backlinks-toggle"
        aria-expanded={open}
        onClick={() => setOpen((o) => !o)}
      >
        {open ? (
          <ChevronDown size={16} aria-hidden="true" className="tree-caret" />
        ) : (
          <ChevronRight size={16} aria-hidden="true" className="tree-caret" />
        )}
        Referenced by ({count})
      </button>
      {open && (
        <>
          {isLoading ? (
            <p className="backlinks-status">Loading backlinks…</p>
          ) : isError ? (
            <p className="backlinks-status">
              Couldn't load backlinks. Refresh to try again.
            </p>
          ) : count === 0 ? (
            <p className="backlinks-status">No backlinks yet</p>
          ) : (
            <ul className="navtree">
              {data!.map((b) => (
                <li key={b.path}>
                  <button
                    type="button"
                    className="navrow navrow-page"
                    onClick={() => navigate(`/app/page/${b.path}`)}
                  >
                    <FileText size={16} aria-hidden="true" className="tree-icon" />
                    <span className="tree-label">{b.title}</span>
                  </button>
                </li>
              ))}
            </ul>
          )}
        </>
      )}
    </section>
  );
}
