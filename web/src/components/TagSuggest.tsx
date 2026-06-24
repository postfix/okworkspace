import { useEffect, useMemo, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, Check, Loader2, Tags } from "lucide-react";

import {
  applyTags,
  me,
  suggestTags,
  type Me,
  type TagSuggestion,
} from "../api/client";
import "./TagSuggest.css";

// TagSuggest is the per-page tag-suggestion TRUST SURFACE (TAG-01 trigger +
// TAG-02 per-tag approval). It owns the locked interaction contract from the
// agent trust gate (DiffReviewDialog):
//
//   *** Apply tags is the accent primary action but is NEVER auto-focused. ***
//
// Cancel is the DOM-first deliberate initial-focus target so a reflexive Enter on
// open cannot write; Esc and a backdrop click invoke Cancel (never Apply); a 409
// stale revision REMOVES the apply path and offers Re-run rather than clobbering a
// concurrent edit. Tag names render as React text children only — NEVER
// dangerouslySetInnerHTML (the locked stored-XSS guard, identical to
// BacklinksPanel). New (invented) tags carry the "new" badge AND default UNCHECKED.
//
// The trigger is editor-gated (renders only for editor/admin — the server gate is
// the real boundary; this is convenience). Nothing is written until Apply.
export default function TagSuggest({ pagePath }: { pagePath: string }) {
  const queryClient = useQueryClient();

  // The role lives in the cached ["me"] session query (App.tsx seeds it). We read
  // it here for the editor gate; the apply endpoint is independently editor+CSRF
  // gated server-side, so this client gate is convenience, not the boundary.
  const { data: meData } = useQuery<Me>({ queryKey: ["me"], queryFn: me });
  const canEdit = meData?.role === "editor" || meData?.role === "admin";

  // The approval surface is open once a suggest succeeds. checked is the per-tag
  // selection set (new tags start UNCHECKED); baseRevision is the suggest-time
  // optimistic-concurrency token echoed back to apply. stale flips on a 409.
  const [open, setOpen] = useState(false);
  const [suggestions, setSuggestions] = useState<TagSuggestion[]>([]);
  const [checked, setChecked] = useState<Record<string, boolean>>({});
  const [baseRevision, setBaseRevision] = useState("");
  const [stale, setStale] = useState(false);
  const [applyError, setApplyError] = useState(false);

  const suggestMutation = useMutation({
    mutationFn: () => suggestTags(pagePath),
    onSuccess: (res) => {
      setSuggestions(res.suggestions);
      setBaseRevision(res.base_revision);
      // New tags default UNCHECKED (the user opts IN to new vocabulary); existing
      // tags default checked. This is the second non-color signal beyond the badge.
      setChecked(
        Object.fromEntries(res.suggestions.map((s) => [s.tag, s.existing])),
      );
      setStale(false);
      setApplyError(false);
      setOpen(true);
    },
  });

  const applyMutation = useMutation({
    mutationFn: (tags: string[]) =>
      applyTags({ page_path: pagePath, tags, base_revision: baseRevision }),
    onSuccess: () => {
      // The new tags are written server-side; reload the page so the editor shows
      // the freshly written frontmatter rather than racing the autosave.
      queryClient.invalidateQueries({ queryKey: ["page", pagePath] });
      setOpen(false);
    },
    onError: (err: Error & { status?: number }) => {
      // A 409 means the page moved since the suggestion — switch to the stale
      // state (no clobbering retry). Any other error shows the apply-error line.
      if (err.status === 409) setStale(true);
      else setApplyError(true);
    },
  });

  const selectedCount = useMemo(
    () => suggestions.filter((s) => checked[s.tag]).length,
    [suggestions, checked],
  );

  function closeSurface() {
    if (applyMutation.isPending) return; // don't dismiss mid-write
    setOpen(false);
  }

  function onApply() {
    setApplyError(false);
    const tags = suggestions.filter((s) => checked[s.tag]).map((s) => s.tag);
    applyMutation.mutate(tags);
  }

  function onRerun() {
    setStale(false);
    setApplyError(false);
    suggestMutation.mutate();
  }

  // Editor-gated: a reader never sees the write trigger.
  if (!canEdit) return null;

  return (
    <div className="tagsuggest">
      <button
        type="button"
        className="btn btn-ghost tagsuggest-trigger"
        onClick={() => suggestMutation.mutate()}
        disabled={suggestMutation.isPending}
      >
        {suggestMutation.isPending ? (
          <>
            <Loader2 size={16} className="spinner" aria-hidden="true" />
            Suggesting tags…
          </>
        ) : (
          <>
            <Tags size={16} aria-hidden="true" />
            Suggest tags
          </>
        )}
      </button>
      {suggestMutation.isError && !open && (
        <p className="tagsuggest-status" role="status">
          Couldn&rsquo;t suggest tags. Try again.
        </p>
      )}

      {open && (
        <TagSuggestList
          suggestions={suggestions}
          checked={checked}
          onToggle={(tag) =>
            setChecked((c) => ({ ...c, [tag]: !c[tag] }))
          }
          onSelectAll={() =>
            setChecked(Object.fromEntries(suggestions.map((s) => [s.tag, true])))
          }
          onClearAll={() =>
            setChecked(Object.fromEntries(suggestions.map((s) => [s.tag, false])))
          }
          selectedCount={selectedCount}
          stale={stale}
          applyError={applyError}
          applying={applyMutation.isPending}
          rerunning={suggestMutation.isPending}
          onApply={onApply}
          onCancel={closeSurface}
          onRerun={onRerun}
        />
      )}
    </div>
  );
}

export interface TagSuggestListProps {
  suggestions: TagSuggestion[];
  checked: Record<string, boolean>;
  onToggle: (tag: string) => void;
  onSelectAll: () => void;
  onClearAll: () => void;
  selectedCount: number;
  stale: boolean;
  applyError: boolean;
  applying: boolean;
  rerunning: boolean;
  onApply: () => void;
  onCancel: () => void;
  onRerun: () => void;
  // The cancel/apply labels are overridable so the batch review route (Phase 12)
  // can read "Skip for now" / "Apply approved" while the per-page PageEditor
  // surface keeps "Cancel" / "Apply tags". The INTERACTION contract is unchanged
  // by a label — cancel stays the DOM-first focused safe default; apply stays
  // accent + never auto-focused. Defaults preserve the Phase-11 copy verbatim.
  cancelLabel?: string;
  applyLabel?: string;
}

// TagSuggestList is the modal approval surface. It clones the Dialog/DiffReviewDialog
// modal shell and REPRODUCES the trust-gate focus inversion verbatim: Cancel is the
// DOM-first deliberate initial-focus target; Apply is accent + NEVER auto-focused;
// Esc + backdrop = Cancel (never Apply); full focus trap + restore-focus-on-close.
//
// Exported so BOTH the per-page TagSuggest (above) and the batch review route
// (TagReviewView, Phase 12) consume the SAME approval surface — the row internals,
// new-default-unchecked, select-all/clear, count line, trust-gate focus inversion,
// and stale state are inherited verbatim, never re-implemented.
export function TagSuggestList({
  suggestions,
  checked,
  onToggle,
  onSelectAll,
  onClearAll,
  selectedCount,
  stale,
  applyError,
  applying,
  rerunning,
  onApply,
  onCancel,
  onRerun,
  cancelLabel = "Cancel",
  applyLabel = "Apply tags",
}: TagSuggestListProps) {
  const dialogRef = useRef<HTMLDivElement>(null);
  // Cancel is the deliberate initial-focus target (NEVER Apply) — a reflexive
  // Enter on open must not write. *** Do NOT "fix" this by focusing Apply. ***
  const cancelRef = useRef<HTMLButtonElement>(null);
  const previouslyFocused = useRef<Element | null>(null);
  // Keep the latest onCancel in a ref so the focus/keydown effect can call it
  // without re-running on every parent re-render (mirrors DiffReviewDialog).
  const onCancelRef = useRef(onCancel);
  useEffect(() => {
    onCancelRef.current = onCancel;
  }, [onCancel]);

  const empty = suggestions.length === 0;

  useEffect(() => {
    previouslyFocused.current = document.activeElement;
    // Trust contract: focus the SAFE default (Cancel), NEVER Apply.
    cancelRef.current?.focus();

    function onKeyDown(e: KeyboardEvent) {
      if (e.key === "Escape") {
        e.preventDefault();
        onCancelRef.current(); // Esc = cancel = dismiss-without-applying
        return;
      }
      if (e.key !== "Tab" || !dialogRef.current) return;
      const items = dialogRef.current.querySelectorAll<HTMLElement>(
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
  }, []);

  return (
    <div
      className="dialog-backdrop"
      // Backdrop click cancels (= dismiss). It NEVER applies — only the explicit
      // Apply button writes (the trust contract).
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onCancel();
      }}
    >
      <div
        className="dialog tagsuggest-dialog"
        role="dialog"
        aria-modal="true"
        aria-label="Suggested tags"
        ref={dialogRef}
      >
        <h2 className="dialog-title">Suggested tags</h2>

        {stale ? (
          // Stale-revision state — the Apply path is REMOVED. A warning (never
          // accent) marks the blocking condition; Close / Re-run are the only paths.
          <>
            <div className="diff-stale" role="alert">
              <AlertTriangle
                size={16}
                className="diff-stale-icon"
                aria-hidden="true"
              />
              <div className="diff-stale-text">
                <strong className="diff-stale-heading">
                  This page changed since the tags were suggested.
                </strong>
                <span className="diff-stale-body">
                  Someone edited the page while you were reviewing. Re-run to get
                  fresh suggestions.
                </span>
              </div>
            </div>
            <div className="dialog-footer tagsuggest-footer">
              <button
                ref={cancelRef}
                type="button"
                className="btn btn-secondary"
                onClick={onCancel}
                disabled={rerunning}
              >
                {cancelLabel === "Cancel" ? "Close" : cancelLabel}
              </button>
              <button
                type="button"
                className="btn btn-primary"
                onClick={onRerun}
                disabled={rerunning}
              >
                {rerunning ? "Suggesting…" : "Re-run"}
              </button>
            </div>
          </>
        ) : empty ? (
          // The model returned no tags — a quiet muted line (mirrors .diff-empty),
          // hidden-Git-safe. Cancel is still the DOM-first focused safe default.
          <>
            <p className="tagsuggest-empty">No tag suggestions for this page.</p>
            <div className="dialog-footer tagsuggest-footer">
              <button
                ref={cancelRef}
                type="button"
                className="btn btn-secondary"
                onClick={onCancel}
              >
                {cancelLabel}
              </button>
            </div>
          </>
        ) : (
          <>
            <p className="tagsuggest-subline">
              Pick the tags to add. New tags are off by default.
            </p>
            <div className="tagsuggest-header">
              <div className="tagsuggest-selectall">
                <button
                  type="button"
                  className="btn btn-ghost"
                  onClick={onSelectAll}
                >
                  Select all
                </button>
                <button
                  type="button"
                  className="btn btn-ghost"
                  onClick={onClearAll}
                >
                  Clear all
                </button>
              </div>
              <span className="tagsuggest-count" aria-live="polite">
                {selectedCount} selected
              </span>
            </div>

            <div className="tagsuggest-list">
              {suggestions.map((s) => (
                <label key={s.tag} className="tagsuggest-row">
                  <input
                    type="checkbox"
                    className="tagsuggest-checkbox"
                    checked={Boolean(checked[s.tag])}
                    onChange={() => onToggle(s.tag)}
                  />
                  {/* Tag name as a React text child — NEVER dangerouslySetInnerHTML. */}
                  <span className="tagsuggest-tagname">{s.tag}</span>
                  {!s.existing && (
                    <span className="tagsuggest-newbadge">new</span>
                  )}
                </label>
              ))}
            </div>

            {applyError && (
              <p className="tagsuggest-status" role="status">
                Couldn&rsquo;t apply the tags. Try again.
              </p>
            )}

            <div className="dialog-footer tagsuggest-footer">
              {/* Cancel is FIRST in the DOM so the focus trap's first focusable is
                  Cancel, and it is the deliberate initial-focus target. */}
              <button
                ref={cancelRef}
                type="button"
                className="btn btn-secondary"
                onClick={onCancel}
                disabled={applying}
              >
                {cancelLabel}
              </button>
              <button
                type="button"
                className="btn btn-primary tagsuggest-apply"
                onClick={onApply}
                disabled={applying}
              >
                {applying ? (
                  "Applying…"
                ) : (
                  <>
                    <Check size={16} aria-hidden="true" />
                    {applyLabel}
                  </>
                )}
              </button>
            </div>
          </>
        )}
      </div>
    </div>
  );
}
