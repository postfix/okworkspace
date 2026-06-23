import { useEffect, useRef, useState } from "react";
import ReactDiffViewer from "react-diff-viewer-continued";
import { AlertTriangle, Check } from "lucide-react";

import "./DiffReviewDialog.css";

// DiffReviewDialog is the load-bearing TRUST GATE (AGNT-10; AI-SPEC §1 #1/#3,
// §6) and is REUSED in Phase 5 (conflict resolution) — so its props stay general
// and it never hard-codes agent-specific copy beyond what the caller passes in.
//
// It is built on the same focus-trap/Esc/backdrop-cancel contract as Dialog.tsx
// but does NOT delegate to Dialog, because the trust contract requires a
// DELIBERATE inversion of Dialog's "focus the first focusable" behavior:
//
//   *** Approve is the primary (accent) action but is NEVER auto-focused. ***
//
// Initial focus lands on Reject (the safe default), so a user cannot approve a
// consequential write by reflexively hitting Enter when the dialog opens. This is
// intentional — do NOT "fix" it by focusing Approve in a future refactor.
// ConflictBusy marks which destructive/copy action is mid-flight in conflict mode
// so the footer can disable and label exactly the clicked button (Overwrite →
// "Saving…", Save-as-copy → "Saving copy…"). null = no action in flight.
export type ConflictBusy = "overwrite" | "copy" | null;

export interface DiffReviewDialogProps {
  open: boolean;
  // mode selects the footer contract. "review" (default) is the Phase 4
  // Approve/Reject trust gate — UNCHANGED. "conflict" is the Phase 5 save-collision
  // surface: a 3-button risk-ranked footer (Overwrite / Manual merge / Save as
  // copy) with initial focus on a SAFE choice, never Overwrite.
  mode?: "review" | "conflict";
  // title is caller-supplied ("Review this change" / "Review the rewrite" / a
  // Phase-5 conflict title) — never assume agent copy here.
  title: string;
  oldText: string;
  newText: string;
  // summary is an OPTIONAL one-line caption ABOVE the diff. It is never a
  // replacement for the diff — the real diff is always rendered.
  summary?: string;
  // columnCaption is an OPTIONAL muted Label under the summary clarifying which
  // side is which ("Left: the saved version · Right: your unsaved version") so
  // "old/new" is never ambiguous in a conflict context.
  columnCaption?: string;
  // --- review mode (mode="review") ---
  onApprove?: () => void;
  // onReject is invoked by the Reject button, Esc, and a backdrop click. It must
  // NEVER apply — dismissing the dialog discards the proposal (Dialog contract).
  // In conflict mode it is the cancel handler (Esc/backdrop = apply nothing).
  onReject: () => void;
  // stale === true means the page moved between proposal and approval (a 409 from
  // /apply-patch). The Approve path is REMOVED — there is no way to apply a stale
  // proposal; the user must re-run. onRerun, when provided, re-issues the proposal.
  stale?: boolean;
  onRerun?: () => void;
  // busy (review mode) disables the footer and shows the approve button as
  // "Saving…".
  busy?: boolean;
  // --- conflict mode (mode="conflict") ---
  // The three risk-ranked resolution handlers. Overwrite is the ONLY data-losing
  // choice; Manual merge and Save as copy are the SAFE choices.
  onOverwrite?: () => void;
  onManualMerge?: () => void;
  onSaveAsCopy?: () => void;
  // conflictBusy disables the conflict footer and labels the in-flight button.
  conflictBusy?: ConflictBusy;
}

export default function DiffReviewDialog({
  open,
  mode = "review",
  title,
  oldText,
  newText,
  summary,
  columnCaption,
  onApprove,
  onReject,
  stale = false,
  onRerun,
  busy = false,
  onOverwrite,
  onManualMerge,
  onSaveAsCopy,
  conflictBusy = null,
}: DiffReviewDialogProps) {
  const isConflict = mode === "conflict";
  const conflictBusyActive = conflictBusy !== null;
  const dialogRef = useRef<HTMLDivElement>(null);
  // The Reject button is the deliberate initial-focus target (NOT Approve) in
  // review mode.
  const rejectRef = useRef<HTMLButtonElement>(null);
  // In conflict mode the SAFE control ("Save as copy") is the deliberate
  // initial-focus target — NEVER Overwrite (the data-losing action). A reflexive
  // Enter on open must not discard the server's change. *** Do NOT "fix" this by
  // focusing Overwrite in a future refactor. ***
  const safeFocusRef = useRef<HTMLButtonElement>(null);
  const previouslyFocused = useRef<Element | null>(null);
  // Keep the latest onReject in a ref so the focus/keydown effect can call it
  // without re-running on every parent re-render (mirrors Dialog.tsx).
  const onRejectRef = useRef(onReject);
  useEffect(() => {
    onRejectRef.current = onReject;
  }, [onReject]);

  // Side-by-side by default; an inline toggle keeps long single-column patches
  // readable (UI-SPEC). View choice is local ephemeral state.
  const [splitView, setSplitView] = useState(true);

  // A no-op proposal (old === new) must NEVER fabricate a diff — Approve is
  // disabled and a plain message is shown, but the diff component is STILL the
  // rendered surface (we just gate Approve), so the dialog is never "prose-only".
  const noChange = oldText === newText;

  useEffect(() => {
    if (!open) return;
    previouslyFocused.current = document.activeElement;
    // *** Trust contract: focus the SAFE default, NEVER the consequential action.
    // review mode → Reject; conflict mode → Save as copy (NEVER Overwrite). ***
    (safeFocusRef.current ?? rejectRef.current)?.focus();

    function onKeyDown(e: KeyboardEvent) {
      if (e.key === "Escape") {
        e.preventDefault();
        onRejectRef.current(); // Esc = cancel = reject-without-applying
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
  }, [open]);

  if (!open) return null;

  return (
    <div
      className="dialog-backdrop"
      // Backdrop click cancels (= reject). It NEVER approves — only the explicit
      // Approve button approves (the trust contract).
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onReject();
      }}
    >
      <div
        className="dialog diff-dialog"
        role="dialog"
        aria-modal="true"
        aria-label={title}
        ref={dialogRef}
      >
        <div className="diff-dialog-head">
          <h2 className="dialog-title">{title}</h2>
          {!noChange && (
            <div
              className="diff-view-toggle"
              role="group"
              aria-label="Diff view"
            >
              <button
                type="button"
                className={`btn ${splitView ? "btn-secondary" : "btn-ghost"}`}
                aria-pressed={splitView}
                onClick={() => setSplitView(true)}
              >
                Side by side
              </button>
              <button
                type="button"
                className={`btn ${!splitView ? "btn-secondary" : "btn-ghost"}`}
                aria-pressed={!splitView}
                onClick={() => setSplitView(false)}
              >
                Inline
              </button>
            </div>
          )}
        </div>

        {summary && <p className="diff-summary">{summary}</p>}
        {columnCaption && !noChange && (
          <p className="diff-column-caption">{columnCaption}</p>
        )}

        {/* The REAL diff — always rendered (never a prose-only summary). For a
            no-op we still render the diff surface and show a message + disabled
            Approve, rather than fabricating change. */}
        <div className="diff-region" tabIndex={0} aria-label="Proposed changes">
          {noChange ? (
            <p className="diff-empty">
              {isConflict
                ? "These versions are identical — your save will go through."
                : "No changes were proposed."}
            </p>
          ) : (
            <ReactDiffViewer
              oldValue={oldText}
              newValue={newText}
              splitView={splitView}
              // disableWorker keeps the diff synchronous so it renders in jsdom
              // (tests) and in bundler setups where the worker bundle fails.
              disableWorker
              styles={diffStyles}
            />
          )}
        </div>

        <div className="dialog-footer diff-footer">
          {isConflict ? (
            noChange ? (
              // Identical versions — the "conflict" resolved itself (both saved the
              // same bytes). NEVER fabricate a diff; offer a single SAFE Save (it
              // routes through the normal revision-checked save, here = Overwrite at
              // the current revision, which will simply succeed).
              <div className="diff-footer-actions">
                <button
                  ref={safeFocusRef}
                  type="button"
                  className="btn btn-primary"
                  onClick={onOverwrite}
                  disabled={conflictBusyActive}
                >
                  {conflictBusy === "overwrite" ? "Saving…" : "Save"}
                </button>
              </div>
            ) : (
              // The 3-button risk-ranked conflict footer. Overwrite (the ONLY
              // destructive control) is POSITIONALLY ISOLATED from the safe pair:
              // the safe buttons are DOM-first so the focus trap's first focusable
              // is a safe choice, and Overwrite sits in its own trailing group with
              // an explicit risk sub-line (color is never the sole risk signal).
              <>
                <div className="diff-conflict-safe">
                  {/* Save as copy is DOM-first → the focus trap's first focusable,
                      and carries the deliberate initial focus (NEVER Overwrite). */}
                  <button
                    ref={safeFocusRef}
                    type="button"
                    className="btn btn-secondary"
                    onClick={onSaveAsCopy}
                    disabled={conflictBusyActive}
                    aria-label="Save my version as a new page, leaving the original unchanged"
                  >
                    {conflictBusy === "copy" ? "Saving copy…" : "Save as copy"}
                  </button>
                  <button
                    type="button"
                    className="btn btn-secondary"
                    onClick={onManualMerge}
                    disabled={conflictBusyActive}
                    aria-label="Merge manually — open my version with theirs shown for reference"
                  >
                    Manual merge
                  </button>
                </div>
                <div className="diff-conflict-overwrite">
                  <button
                    type="button"
                    className="btn btn-ghost-destructive"
                    onClick={onOverwrite}
                    disabled={conflictBusyActive}
                    aria-label="Overwrite with my version, replacing their changes"
                  >
                    {conflictBusy === "overwrite" ? "Saving…" : "Overwrite"}
                  </button>
                  <span
                    className="diff-conflict-risk"
                    id="diff-conflict-risk-note"
                  >
                    This replaces the other person&rsquo;s changes.
                  </span>
                </div>
              </>
            )
          ) : stale ? (
            // Stale-revision state — Approve is REMOVED. A warning (never accent)
            // marks the blocking condition; Re-run/Close are the only paths.
            <>
              <div className="diff-stale" role="alert">
                <AlertTriangle
                  size={16}
                  className="diff-stale-icon"
                  aria-hidden="true"
                />
                <div className="diff-stale-text">
                  <strong className="diff-stale-heading">
                    This page changed since the assistant read it.
                  </strong>
                  <span className="diff-stale-body">
                    Someone edited the page while you were reviewing. Re-run the
                    assistant to get a fresh proposal.
                  </span>
                </div>
              </div>
              <div className="diff-footer-actions">
                <button
                  type="button"
                  className="btn btn-secondary"
                  onClick={onReject}
                  disabled={busy}
                >
                  Close
                </button>
                {onRerun && (
                  <button
                    type="button"
                    className="btn btn-primary"
                    onClick={onRerun}
                    disabled={busy}
                  >
                    Re-run
                  </button>
                )}
              </div>
            </>
          ) : (
            <>
              {/* Reject is FIRST in the DOM so the focus trap's first focusable
                  is Reject, and it is the deliberate initial-focus target. */}
              <button
                ref={rejectRef}
                type="button"
                className="btn btn-ghost-destructive"
                onClick={onReject}
                disabled={busy}
              >
                Reject
              </button>
              <button
                type="button"
                className="btn btn-primary diff-approve"
                onClick={onApprove}
                disabled={busy || noChange}
              >
                {busy ? (
                  "Saving…"
                ) : (
                  <>
                    <Check size={16} aria-hidden="true" />
                    Approve &amp; save
                  </>
                )}
              </button>
            </>
          )}
        </div>
      </div>
    </div>
  );
}

// diffStyles themes react-diff-viewer-continued to the token palette: added lines
// derive from --color-success and removed from --color-destructive at LOW
// saturation (diff semantics, NOT the app's 10% accent — they must not read as
// accent), mono font, surface gutters. Colors reference CSS variables so a future
// dark theme works untouched.
const diffStyles = {
  variables: {
    light: {
      diffViewerBackground: "var(--color-bg)",
      addedBackground: "rgba(22, 163, 74, 0.10)",
      addedColor: "var(--color-text)",
      removedBackground: "rgba(220, 38, 38, 0.10)",
      removedColor: "var(--color-text)",
      wordAddedBackground: "rgba(22, 163, 74, 0.22)",
      wordRemovedBackground: "rgba(220, 38, 38, 0.22)",
      addedGutterBackground: "rgba(22, 163, 74, 0.16)",
      removedGutterBackground: "rgba(220, 38, 38, 0.16)",
      gutterBackground: "var(--color-surface)",
      gutterColor: "var(--color-text-muted)",
      codeFoldBackground: "var(--color-surface)",
      emptyLineBackground: "var(--color-bg)",
    },
  },
  contentText: {
    fontFamily: "var(--font-family-mono)",
    fontSize: "var(--font-size-body)",
  },
  diffContainer: {
    fontFamily: "var(--font-family-mono)",
  },
};
