import { useEffect, useMemo, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import { Loader2, Search } from "lucide-react";

import type { SearchResult, SearchResultKind } from "../../api/client";
import { useSearch } from "../../hooks/useSearch";
import { useSearchStore } from "../../store/searchStore";
import SearchResultRow from "./SearchResultRow";
import "./SearchPalette.css";

// Display order + label for the result groups (UI-SPEC: empty groups omitted).
const GROUP_ORDER: { kind: SearchResultKind; label: string }[] = [
  { kind: "page", label: "Pages" },
  { kind: "heading", label: "Headings" },
  { kind: "attachment", label: "Attachments" },
];

const LIST_ID = "search-palette-listbox";
const optionId = (i: number) => `search-option-${i}`;

// SearchPalette gates on the zustand open state and mounts a fresh PaletteInner
// each time it opens (the `key` forces a remount), so all transient state —
// query text, active row, loading flag — resets cleanly without a reset effect.
export default function SearchPalette() {
  const open = useSearchStore((s) => s.open);
  if (!open) return null;
  return <PaletteInner />;
}

// PaletteInner is the ⌘K quick-switcher overlay. It replicates Dialog.tsx's
// focus-management contract (focus-in on mount, Esc closes, Tab trap, focus
// restore on unmount) but renders bespoke chrome: input-on-top, no title/footer
// buttons. It is only ever mounted while the palette is open.
function PaletteInner() {
  const setOpen = useSearchStore((s) => s.setOpen);
  const navigate = useNavigate();

  const [query, setQuery] = useState("");
  const [activeIndex, setActiveIndex] = useState(0);
  // Show the loading state only after a short delay to avoid flicker on fast
  // responses (UI-SPEC: spinner only if in flight > ~150ms).
  const [showLoading, setShowLoading] = useState(false);

  const panelRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const previouslyFocused = useRef<Element | null>(null);

  const { data, isLoading, isError } = useSearch(query);

  // Flat, in-display-order result list — keyboard nav flows across groups as one
  // continuous list, but rendering still groups them.
  const flat: SearchResult[] = useMemo(() => {
    const results = data ?? [];
    return GROUP_ORDER.flatMap(({ kind }) =>
      results.filter((r) => r.kind === kind),
    );
  }, [data]);

  const hasQuery = query.trim().length > 0;
  // Clamp the active row into range at read time (results shrink as queries
  // change). Derived during render — no clamp effect, no cascading setState.
  const active = flat.length === 0 ? 0 : Math.min(activeIndex, flat.length - 1);

  // Delayed loading indicator (≈150ms). Reset to false via cleanup, and the
  // setState only ever runs inside the timer callback (allowed) — never
  // synchronously in the effect body.
  useEffect(() => {
    if (!isLoading || !hasQuery) return;
    const id = window.setTimeout(() => setShowLoading(true), 150);
    return () => {
      window.clearTimeout(id);
      setShowLoading(false);
    };
  }, [isLoading, hasQuery]);

  // Auto-scroll the active row into view as the selection moves.
  useEffect(() => {
    const el = document.getElementById(optionId(active));
    el?.scrollIntoView({ block: "nearest" });
  }, [active]);

  // Focus-management contract, replicated from Dialog.tsx. Runs once on mount:
  // store the previously-focused element (the trigger), focus the input, trap
  // Tab, close on Esc, and restore focus to the trigger on unmount (close).
  useEffect(() => {
    previouslyFocused.current = document.activeElement;
    inputRef.current?.focus();

    const node = panelRef.current;
    function onKeyDown(e: KeyboardEvent) {
      if (e.key === "Escape") {
        e.preventDefault();
        setOpen(false);
        return;
      }
      if (e.key !== "Tab" || !node) return;
      const items = node.querySelectorAll<HTMLElement>(
        'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])',
      );
      if (items.length === 0) return;
      const first = items[0];
      const last = items[items.length - 1];
      if (e.shiftKey && document.activeElement === first) {
        e.preventDefault();
        last.focus();
      } else if (!e.shiftKey && document.activeElement === last) {
        e.preventDefault();
        first.focus();
      }
    }

    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("keydown", onKeyDown);
      (previouslyFocused.current as HTMLElement | null)?.focus?.();
    };
  }, [setOpen]);

  // openResult navigates in-app and closes the palette (Enter / click). Page →
  // its route; heading → page route then scroll to its anchor; attachment → its
  // owning page (SRCH-05).
  function openResult(r: SearchResult) {
    setOpen(false);
    navigate(`/app/page/${r.path}`);
    if (r.kind === "heading" && r.anchor) {
      // The heading id lands once 03-03 adds anchors to the renderer; until then
      // this is a no-op deep-link, the page still opens correctly.
      const anchor = r.anchor;
      window.requestAnimationFrame(() => {
        document.getElementById(anchor)?.scrollIntoView();
      });
    }
  }

  // Keyboard nav within the input: ↑/↓ move the active row, Enter opens it.
  function onInputKeyDown(e: React.KeyboardEvent<HTMLInputElement>) {
    if (e.key === "ArrowDown") {
      e.preventDefault();
      setActiveIndex(flat.length === 0 ? 0 : (active + 1) % flat.length);
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      setActiveIndex(
        flat.length === 0 ? 0 : (active - 1 + flat.length) % flat.length,
      );
    } else if (e.key === "Enter") {
      e.preventDefault();
      const r = flat[active];
      if (r) openResult(r);
    }
  }

  return (
    <div
      className="dialog-backdrop palette-backdrop"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) setOpen(false);
      }}
    >
      <div
        className="palette-panel"
        role="dialog"
        aria-modal="true"
        aria-label="Search"
        ref={panelRef}
      >
        <div className="palette-input-row">
          <Search size={16} aria-hidden="true" className="palette-input-icon" />
          <input
            ref={inputRef}
            type="text"
            className="palette-input"
            placeholder="Search pages, headings, and attachments…"
            value={query}
            onChange={(e) => {
              setQuery(e.target.value);
              setActiveIndex(0);
            }}
            onKeyDown={onInputKeyDown}
            role="combobox"
            aria-expanded={flat.length > 0}
            aria-controls={LIST_ID}
            aria-activedescendant={
              flat.length > 0 ? optionId(active) : undefined
            }
            aria-autocomplete="list"
          />
        </div>

        <div className="palette-results">
          {!hasQuery ? (
            <div className="palette-empty" aria-live="polite">
              <h2 className="palette-empty-heading">Search your workspace</h2>
              <p className="palette-empty-body">
                Find any page, heading, or attachment. Start typing to see
                results.
              </p>
            </div>
          ) : isError ? (
            <div
              className="banner banner-warning palette-error"
              role="alert"
              aria-live="polite"
            >
              <span className="palette-error-heading">
                Search is unavailable
              </span>
              <span className="palette-error-body">
                Something went wrong while searching. Try again in a moment.
              </span>
            </div>
          ) : showLoading && flat.length === 0 ? (
            <div className="palette-loading" aria-live="polite">
              <Loader2 size={16} className="spinner" aria-hidden="true" />
              <span>Searching…</span>
            </div>
          ) : flat.length === 0 ? (
            <div className="palette-empty" aria-live="polite">
              <h2 className="palette-empty-heading">No matches</h2>
              <p className="palette-empty-body">
                Nothing matched &ldquo;{query}&rdquo;. Try a different word or
                check the spelling.
              </p>
            </div>
          ) : (
            <div role="listbox" id={LIST_ID} aria-label="Search results">
              {GROUP_ORDER.map(({ kind, label }) => {
                const group = (data ?? []).filter((r) => r.kind === kind);
                if (group.length === 0) return null;
                return (
                  <div key={kind} className="palette-group">
                    <div className="palette-group-label">{label}</div>
                    {group.map((r) => {
                      const idx = flat.indexOf(r);
                      return (
                        <SearchResultRow
                          key={`${r.kind}:${r.path}:${r.anchor ?? ""}:${r.title}`}
                          result={r}
                          id={optionId(idx)}
                          active={idx === active}
                          onActivate={() => setActiveIndex(idx)}
                          onOpen={() => openResult(r)}
                        />
                      );
                    })}
                  </div>
                );
              })}
            </div>
          )}
        </div>

        <div className="palette-footer">
          ↑↓ to navigate · ↵ to open · esc to close
        </div>
      </div>
    </div>
  );
}
