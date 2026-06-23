import { FileText, Hash, Paperclip } from "lucide-react";

import type { SearchResult } from "../../api/client";
import { renderHighlight } from "./highlight";
import "./SearchResultRow.css";

// kindIcon renders the muted 16px lucide glyph for a result kind (UI-SPEC
// anatomy). Returning the element (not the component) keeps the icon out of the
// "component created during render" lint and avoids remounting on each render.
function kindIcon(kind: SearchResult["kind"]) {
  const props = {
    size: 16,
    "aria-hidden": true,
    className: "search-result-icon",
  } as const;
  switch (kind) {
    case "heading":
      return <Hash {...props} />;
    case "attachment":
      return <Paperclip {...props} />;
    case "page":
    default:
      return <FileText {...props} />;
  }
}

// kindLabel is the neutral type-badge text (Page / Heading / Attachment).
function kindLabel(kind: SearchResult["kind"]): string {
  return kind.charAt(0).toUpperCase() + kind.slice(1);
}

export interface SearchResultRowProps {
  result: SearchResult;
  // active is the single keyboard/hover-selected row (exactly one at a time).
  active: boolean;
  // id wires aria-activedescendant from the input to this option.
  id: string;
  // onActivate sets this row active (mouse hover syncs with keyboard nav).
  onActivate: () => void;
  // onOpen opens this result and closes the palette (Enter / click).
  onOpen: () => void;
}

// SearchResultRow is a typed result row reusing the .navrow geometry. It is a
// role="option" button. Matched terms in the title and snippet are bold via
// renderHighlight (weight-only — no background, no accent, no raw HTML).
export default function SearchResultRow({
  result,
  active,
  id,
  onActivate,
  onOpen,
}: SearchResultRowProps) {
  // Heading/attachment carry an owning-page sub-line ("in {page title}").
  const subLine =
    result.kind !== "page" && result.page_title ? result.page_title : null;

  return (
    <button
      type="button"
      id={id}
      role="option"
      aria-selected={active}
      className={`navrow search-result-row${active ? " search-result-row--active" : ""}`}
      // Mouse down (not click) keeps the input from losing focus before navigation.
      onMouseMove={onActivate}
      onClick={onOpen}
    >
      {kindIcon(result.kind)}
      <span className="search-result-main">
        <span className="search-result-titlerow">
          <span className="search-result-title">{renderHighlight(result.title)}</span>
          <span className="role-badge search-result-badge">
            {kindLabel(result.kind)}
          </span>
        </span>
        {result.snippet && (
          <span className="search-result-snippet">
            {renderHighlight(result.snippet)}
          </span>
        )}
        {subLine && (
          <span className="search-result-subline">in {subLine}</span>
        )}
      </span>
    </button>
  );
}
