import { useEffect, useRef, type ReactNode } from "react";
import "./Dialog.css";

export interface DialogProps {
  open: boolean;
  title: string;
  children: ReactNode;
  // onCancel is called by Esc, the Cancel button, and a backdrop click. It must
  // NEVER confirm — only the explicit confirm button confirms (UI-SPEC).
  onCancel: () => void;
  // onConfirm runs the primary action. Provided by callers that have one.
  onConfirm?: () => void;
  confirmLabel?: string;
  cancelLabel?: string;
  // destructive renders the confirm button in the destructive style and is the
  // signal that backdrop-click must not confirm (it already never does).
  destructive?: boolean;
  // busy disables the action buttons and shows the confirm button as loading.
  busy?: boolean;
  // hideFooter lets a caller supply its own footer (e.g. a form submit).
  hideFooter?: boolean;
  // className applies extra classes to the .dialog box (e.g. a wider variant).
  className?: string;
}

// Dialog is a focus-trapped modal. Esc and backdrop-click invoke onCancel; the
// confirm button is the ONLY path that invokes onConfirm (so a backdrop click
// can never confirm a destructive action — UI-SPEC interaction contract).
export default function Dialog({
  open,
  title,
  children,
  onCancel,
  onConfirm,
  confirmLabel = "Confirm",
  cancelLabel = "Cancel",
  destructive = false,
  busy = false,
  hideFooter = false,
  className = "",
}: DialogProps) {
  const dialogRef = useRef<HTMLDivElement>(null);
  const previouslyFocused = useRef<Element | null>(null);
  // Keep the latest onCancel in a ref so the focus/keydown effect can call it
  // without listing onCancel as a dependency. Callers pass a fresh onCancel
  // identity on every render (e.g. `onCancel={() => setAddOpen(false)}`); if the
  // effect depended on it, every keystroke-driven re-render would re-run the
  // effect and re-focus the first field, stealing the caret.
  const onCancelRef = useRef(onCancel);
  // Sync the ref in an effect (not during render) so the focus/keydown effect
  // below can call the latest onCancel without depending on its identity.
  useEffect(() => {
    onCancelRef.current = onCancel;
  }, [onCancel]);

  useEffect(() => {
    if (!open) return;
    previouslyFocused.current = document.activeElement;
    const node = dialogRef.current;
    // Move focus into the dialog. Runs only when the dialog opens.
    const focusable = node?.querySelector<HTMLElement>(
      'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])',
    );
    focusable?.focus();

    function onKeyDown(e: KeyboardEvent) {
      if (e.key === "Escape") {
        e.preventDefault();
        onCancelRef.current();
        return;
      }
      if (e.key !== "Tab" || !node) return;
      // Trap focus within the dialog.
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
    // Depend on `open` ONLY: the focus-into-dialog logic must run when the
    // dialog opens/closes, not on every render. onCancel is read via
    // onCancelRef so its changing identity never re-fires this effect.
  }, [open]);

  if (!open) return null;

  return (
    <div
      className="dialog-backdrop"
      // Backdrop click cancels. It never confirms — even for destructive
      // dialogs only the explicit confirm button confirms.
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onCancel();
      }}
    >
      <div
        className={`dialog${className ? ` ${className}` : ""}`}
        role="dialog"
        aria-modal="true"
        aria-label={title}
        ref={dialogRef}
      >
        <h2 className="dialog-title">{title}</h2>
        <div className="dialog-body">{children}</div>
        {!hideFooter && (
          <div className="dialog-footer">
            <button
              type="button"
              className="btn btn-secondary"
              onClick={onCancel}
              disabled={busy}
            >
              {cancelLabel}
            </button>
            {onConfirm && (
              <button
                type="button"
                className={destructive ? "btn btn-destructive" : "btn btn-primary"}
                onClick={onConfirm}
                disabled={busy}
              >
                {busy ? "Working…" : confirmLabel}
              </button>
            )}
          </div>
        )}
      </div>
    </div>
  );
}
