import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ChevronRight, ListChecks } from "lucide-react";

import {
  approveTagSuggestions,
  listTagSuggestions,
  suggestTags,
  type TagSuggestion,
  type TagSuggestionEntry,
} from "../api/client";
import { TagSuggestList } from "./TagSuggest";
import "./TagReviewView.css";

const QUEUE_KEY = ["tag-suggestions"];

// TagReviewView is the admin batch review queue (TAG-06). It lists every page
// that currently has pending tag suggestions (the persisted, resumable backlog
// from listTagSuggestions) and reviews them ONE PAGE AT A TIME by REUSING the
// Phase-11 TagSuggestList approval surface — the per-tag rows, new-vs-existing
// badge, new-default-unchecked, select-all/clear, count line, trust-gate focus
// inversion (Apply NEVER auto-focused; Skip is the DOM-first safe default; Esc +
// backdrop dismiss without writing), and the 409 stale state are all inherited,
// NOT re-implemented here.
//
// The ONLY adaptations: the dismiss button reads "Skip for now" (leaves the page
// in the backlog), the primary CTA reads "Apply approved", and on a status="applied"
// result the page leaves the backlog + the progress line decrements. A per-page
// status="stale" switches THAT page into the inherited stale state without sinking
// the rest of the backlog. Titles/paths/tags render as React text children ONLY —
// NEVER dangerouslySetInnerHTML (the locked stored-XSS guard, T-12-11).
//
// The route is admin-gated by RequireAdmin in App.tsx (the server RequireRole(admin)
// on every endpoint is the real boundary; this is convenience). Default export so
// it can be React.lazy-loaded like GraphView (the Phase-10 lazy-route pattern).
export default function TagReviewView() {
  const queryClient = useQueryClient();

  const {
    data: queue = [],
    isLoading,
    isError,
  } = useQuery<TagSuggestionEntry[]>({
    queryKey: QUEUE_KEY,
    queryFn: listTagSuggestions,
  });

  // The currently-open page path (null = no page open, showing the backlog).
  const [openPath, setOpenPath] = useState<string | null>(null);
  // The per-page selection set + a captured base_revision (echoed back on apply),
  // plus the per-page stale flag — the same local state TagSuggest manages.
  const [checked, setChecked] = useState<Record<string, boolean>>({});
  const [stale, setStale] = useState(false);
  const [applyError, setApplyError] = useState(false);

  // The open entry, resolved from the live queue (so a backlog refresh keeps the
  // open page's suggestions in sync). null once the page leaves the backlog.
  const openEntry = useMemo(
    () => queue.find((e) => e.page_path === openPath) ?? null,
    [queue, openPath],
  );

  const suggestions: TagSuggestion[] = openEntry?.suggestions ?? [];
  const selectedCount = useMemo(
    () => suggestions.filter((s) => checked[s.tag]).length,
    [suggestions, checked],
  );

  // openReview opens a backlog row's suggestions in the reused approval surface.
  // New tags default UNCHECKED, existing default checked (the inherited second
  // non-color signal beyond the "new" badge).
  function openReview(entry: TagSuggestionEntry) {
    setOpenPath(entry.page_path);
    setChecked(
      Object.fromEntries(entry.suggestions.map((s) => [s.tag, s.existing])),
    );
    setStale(false);
    setApplyError(false);
  }

  function closeReview() {
    if (approveMut.isPending) return; // don't dismiss mid-write
    setOpenPath(null);
    setStale(false);
    setApplyError(false);
  }

  const approveMut = useMutation({
    mutationFn: (tags: string[]) =>
      approveTagSuggestions([{ page_path: openPath as string, tags }]),
    onSuccess: (results) => {
      // The batched approve returns one result row per requested page. A per-page
      // "stale" switches THIS page into the inherited stale state (no clobber)
      // without affecting any other backlog row; "applied"/"notfound" both leave
      // the backlog (the row is gone server-side), so refetch the queue + close.
      const result = results.find((r) => r.page_path === openPath);
      if (result?.status === "stale") {
        setStale(true);
        return;
      }
      // applied or notfound: the page is no longer pending — drop it from the
      // backlog (the progress line decrements) and return to the list.
      queryClient.invalidateQueries({ queryKey: QUEUE_KEY });
      setOpenPath(null);
    },
    onError: () => {
      // A transport/server failure (not a per-page stale) — show the inherited
      // apply-error line and keep the review surface open for a retry.
      setApplyError(true);
    },
  });

  // Re-run from the stale state re-fetches fresh suggestions for the open page
  // and re-opens its review surface (or simply re-fetches the queue if the page
  // is gone). It never writes — the user must Apply again explicitly.
  const rerunMut = useMutation({
    mutationFn: () => suggestTags(openPath as string),
    onSuccess: (res) => {
      setStale(false);
      setApplyError(false);
      setChecked(
        Object.fromEntries(res.suggestions.map((s) => [s.tag, s.existing])),
      );
      // Refresh the backlog so the open entry carries the fresh suggestions +
      // base_revision (the queue is the source of truth the surface reads).
      queryClient.invalidateQueries({ queryKey: QUEUE_KEY });
    },
  });

  function onApply() {
    setApplyError(false);
    const tags = suggestions.filter((s) => checked[s.tag]).map((s) => s.tag);
    approveMut.mutate(tags);
  }

  return (
    <div className="tagreview">
      <header className="tagreview-header">
        <h1 className="tagreview-title">
          <ListChecks size={20} aria-hidden="true" className="tagreview-title-icon" />
          Tag review
        </h1>
        <p className="tagreview-subtitle">
          Pages with tag suggestions waiting for your approval.
        </p>
      </header>

      <div className="tagreview-body">
        {isLoading ? (
          <p className="tagreview-loading">Loading pages to review…</p>
        ) : isError ? (
          <p className="tagreview-status" role="status">
            Couldn&rsquo;t load the review queue. Refresh to try again.
          </p>
        ) : queue.length === 0 ? (
          <div className="tagreview-empty">
            <h2 className="tagreview-empty-heading">No pending suggestions</h2>
            <p className="tagreview-empty-body">
              Run a tag-suggestion sweep from the admin Settings page, then approved
              suggestions show up here.
            </p>
          </div>
        ) : (
          <>
            <p className="tagreview-progress" aria-live="polite">
              {queue.length} pages left to review
            </p>
            <ul className="tagreview-list">
              {queue.map((entry) => {
                const title = entry.page_path; // path is the stable label (no title in the queue payload)
                return (
                  <li key={entry.page_path}>
                    <button
                      type="button"
                      className={`tagreview-row${
                        openPath === entry.page_path ? " tagreview-row-active" : ""
                      }`}
                      onClick={() => openReview(entry)}
                      aria-label={`Review suggestions for ${title}`}
                    >
                      <span className="tagreview-row-main">
                        {/* Title/path render as React text children — NEVER
                            dangerouslySetInnerHTML (stored-XSS guard, T-12-11). */}
                        <span className="tagreview-row-title">{title}</span>
                        <span className="tagreview-row-path">{entry.page_path}</span>
                      </span>
                      <span className="role-badge tagreview-pending-badge">
                        {entry.suggestions.length} pending
                      </span>
                      <ChevronRight
                        size={16}
                        aria-hidden="true"
                        className="tagreview-row-chevron"
                      />
                    </button>
                  </li>
                );
              })}
            </ul>
          </>
        )}
      </div>

      {openEntry && (
        <TagSuggestList
          suggestions={suggestions}
          checked={checked}
          onToggle={(tag) => setChecked((c) => ({ ...c, [tag]: !c[tag] }))}
          onSelectAll={() =>
            setChecked(Object.fromEntries(suggestions.map((s) => [s.tag, true])))
          }
          onClearAll={() =>
            setChecked(Object.fromEntries(suggestions.map((s) => [s.tag, false])))
          }
          selectedCount={selectedCount}
          stale={stale}
          applyError={applyError}
          applying={approveMut.isPending}
          rerunning={rerunMut.isPending}
          onApply={onApply}
          onCancel={closeReview}
          onRerun={() => rerunMut.mutate()}
          cancelLabel="Skip for now"
          applyLabel="Apply approved"
        />
      )}
    </div>
  );
}
