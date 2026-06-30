import { useEffect, useRef, useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { AlertTriangle, Check, Loader2, Sparkles } from "lucide-react";

import {
  applyDescription,
  generateDescription,
  me,
  type Me,
} from "../api/client";
import "./DescriptionGenerate.css";

// DescriptionGenerate is the per-page description TRUST SURFACE (ED-01b). It owns
// the same locked agent-apply interaction contract as TagSuggest (DiffReviewDialog):
//
//   *** Apply is the accent primary action but is NEVER auto-focused. ***
//
// Cancel is the DOM-first deliberate initial-focus target so a reflexive Enter on
// open cannot write; Esc and a backdrop click invoke Cancel (never Apply); a 409
// stale revision REMOVES the apply path and offers Regenerate rather than clobbering
// a concurrent edit. The generated draft renders into an EDITABLE textarea (the user
// may edit it before approving) — the review surface ED-01b requires.
//
// NEVER auto-runs: generate fires ONLY on the explicit button click, apply ONLY on
// the explicit Apply (D-07). Mounting issues no generate/apply call.
//
// The trigger is editor-gated (renders only for editor/admin — the server gate is
// the real boundary; this is convenience). `onApplied(text)` hands the approved text
// back to PageEditor, which owns the autosave-safe reconciliation (it alone holds the
// revision/frontmatter refs) — this REPLACES a bare query invalidate, which cannot
// re-seed the editor past its `seeded` guard and would race the autosave.
export default function DescriptionGenerate({
  pagePath,
  onApplied,
}: {
  pagePath: string;
  onApplied?: (text: string) => void;
}) {
  // The role lives in the cached ["me"] session query (App.tsx seeds it). We read it
  // here for the editor gate; the apply endpoint is independently editor+CSRF gated
  // server-side, so this client gate is convenience, not the boundary.
  const { data: meData } = useQuery<Me>({ queryKey: ["me"], queryFn: me });
  const canEdit = meData?.role === "editor" || meData?.role === "admin";

  // The review surface is open once a generate succeeds. draft is the EDITABLE text
  // (the user may revise it before approving); baseRevision is the generate-time
  // optimistic-concurrency token echoed back to apply. stale flips on a 409.
  const [open, setOpen] = useState(false);
  const [draft, setDraft] = useState("");
  const [baseRevision, setBaseRevision] = useState("");
  const [stale, setStale] = useState(false);
  const [applyError, setApplyError] = useState(false);

  const generateMutation = useMutation({
    mutationFn: () => generateDescription(pagePath),
    onSuccess: (res) => {
      setDraft(res.description);
      setBaseRevision(res.base_revision);
      setStale(false);
      setApplyError(false);
      setOpen(true);
    },
  });

  const applyMutation = useMutation({
    // The mutation variable is the EDITED text the user approved — send exactly that,
    // never a stale closure of the original draft. The server re-validates it.
    mutationFn: (text: string) =>
      applyDescription({
        page_path: pagePath,
        description: text,
        base_revision: baseRevision,
      }),
    onSuccess: (_data, text) => {
      // Hand the approved text to PageEditor's reconcile (it owns advancing
      // baseRevision + merging into local frontmatter, autosave-safe).
      onApplied?.(text);
      setOpen(false);
    },
    onError: (err: Error & { status?: number }) => {
      // A 409 means the page moved since the draft was generated — switch to the
      // stale state (no clobbering retry). Any other error shows the apply-error line.
      if (err.status === 409) setStale(true);
      else setApplyError(true);
    },
  });

  function onApply() {
    setApplyError(false);
    applyMutation.mutate(draft);
  }

  function onRegenerate() {
    setStale(false);
    setApplyError(false);
    generateMutation.mutate();
  }

  function closeSurface() {
    if (applyMutation.isPending) return; // don't dismiss mid-write
    setOpen(false);
  }

  // Editor-gated: a reader never sees the write trigger.
  if (!canEdit) return null;

  return (
    <div className="descgen">
      <button
        type="button"
        className="btn btn-ghost descgen-trigger"
        onClick={() => generateMutation.mutate()}
        disabled={generateMutation.isPending}
      >
        {generateMutation.isPending ? (
          <>
            <Loader2 size={16} className="spinner" aria-hidden="true" />
            Generating description…
          </>
        ) : (
          <>
            <Sparkles size={16} aria-hidden="true" />
            Generate description
          </>
        )}
      </button>
      {generateMutation.isError && !open && (
        <p className="descgen-status" role="status">
          Couldn&rsquo;t generate a description. Try again.
        </p>
      )}

      {open && (
        <DescriptionReview
          draft={draft}
          onDraftChange={setDraft}
          stale={stale}
          applyError={applyError}
          applying={applyMutation.isPending}
          regenerating={generateMutation.isPending}
          onApply={onApply}
          onCancel={closeSurface}
          onRegenerate={onRegenerate}
        />
      )}
    </div>
  );
}

export interface DescriptionReviewProps {
  draft: string;
  onDraftChange: (text: string) => void;
  stale: boolean;
  applyError: boolean;
  applying: boolean;
  regenerating: boolean;
  onApply: () => void;
  onCancel: () => void;
  onRegenerate: () => void;
}

// DescriptionReview is the modal review surface. It clones the Dialog modal shell and
// REPRODUCES the trust-gate focus inversion verbatim: Cancel is the DOM-first
// deliberate initial-focus target; Apply is accent + NEVER auto-focused; Esc +
// backdrop = Cancel (never Apply); full focus trap + restore-focus-on-close. The
// editable textarea is the ED-01b review surface (the user may revise the draft).
export function DescriptionReview({
  draft,
  onDraftChange,
  stale,
  applyError,
  applying,
  regenerating,
  onApply,
  onCancel,
  onRegenerate,
}: DescriptionReviewProps) {
  const dialogRef = useRef<HTMLDivElement>(null);
  // Cancel is the deliberate initial-focus target (NEVER Apply) — a reflexive Enter
  // on open must not write. *** Do NOT "fix" this by focusing Apply. ***
  const cancelRef = useRef<HTMLButtonElement>(null);
  const previouslyFocused = useRef<Element | null>(null);
  // Keep the latest onCancel in a ref so the focus/keydown effect can call it without
  // re-running on every parent re-render (mirrors TagSuggestList / DiffReviewDialog).
  const onCancelRef = useRef(onCancel);
  useEffect(() => {
    onCancelRef.current = onCancel;
  }, [onCancel]);

  useEffect(() => {
    previouslyFocused.current = document.activeElement;
    // Trust contract: focus the SAFE default (Cancel), NEVER Apply or the textarea.
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

  const canApply = draft.trim() !== "" && !applying;

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
        className="dialog descgen-dialog"
        role="dialog"
        aria-modal="true"
        aria-label="Generated description"
        ref={dialogRef}
      >
        <h2 className="dialog-title">Generated description</h2>

        {stale ? (
          // Stale-revision state — the Apply path is REMOVED. A warning (never accent)
          // marks the blocking condition; Close / Regenerate are the only paths.
          <>
            <div className="diff-stale" role="alert">
              <AlertTriangle
                size={16}
                className="diff-stale-icon"
                aria-hidden="true"
              />
              <div className="diff-stale-text">
                <strong className="diff-stale-heading">
                  This page changed since the description was generated.
                </strong>
                <span className="diff-stale-body">
                  Someone edited the page while you were reviewing. Regenerate to
                  describe the latest version.
                </span>
              </div>
            </div>
            <div className="dialog-footer descgen-footer">
              <button
                ref={cancelRef}
                type="button"
                className="btn btn-secondary"
                onClick={onCancel}
                disabled={regenerating}
              >
                Close
              </button>
              <button
                type="button"
                className="btn btn-primary"
                onClick={onRegenerate}
                disabled={regenerating}
              >
                {regenerating ? "Generating…" : "Regenerate"}
              </button>
            </div>
          </>
        ) : (
          <>
            <p className="descgen-subline">
              Review and edit the description, then apply it.
            </p>
            {/* The editable review surface: the generated draft, which the user may
                revise before approving (ED-01b). Plain text only — rendered as a
                textarea value, never dangerouslySetInnerHTML. */}
            <textarea
              className="descgen-textarea"
              aria-label="Description draft"
              value={draft}
              onChange={(e) => onDraftChange(e.target.value)}
              rows={3}
            />

            {applyError && (
              <p className="descgen-status" role="status">
                Couldn&rsquo;t apply the description. Try again.
              </p>
            )}

            <div className="dialog-footer descgen-footer">
              {/* Cancel is FIRST in the DOM so the focus trap's first focusable is
                  Cancel, and it is the deliberate initial-focus target. */}
              <button
                ref={cancelRef}
                type="button"
                className="btn btn-secondary"
                onClick={onCancel}
                disabled={applying}
              >
                Cancel
              </button>
              <button
                type="button"
                className="btn btn-ghost"
                onClick={onRegenerate}
                disabled={applying || regenerating}
              >
                {regenerating ? "Generating…" : "Regenerate"}
              </button>
              <button
                type="button"
                className="btn btn-primary descgen-apply"
                onClick={onApply}
                disabled={!canApply}
              >
                {applying ? (
                  "Applying…"
                ) : (
                  <>
                    <Check size={16} aria-hidden="true" />
                    Apply
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
