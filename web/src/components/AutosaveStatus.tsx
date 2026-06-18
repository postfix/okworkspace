import { Check, Loader2 } from "lucide-react";

import "./AutosaveStatus.css";

// SaveState drives the autosave indicator copy. No Git vocabulary ever surfaces
// (hidden-Git rule) — only "Saving…", "Draft saved", and "Saved".
export type SaveState = "idle" | "saving" | "draft-saved" | "saved";

export default function AutosaveStatus({ state }: { state: SaveState }) {
  if (state === "idle") {
    return <span className="autosave-status" aria-live="polite" />;
  }
  if (state === "saving") {
    return (
      <span className="autosave-status autosave-muted" aria-live="polite">
        <Loader2 size={14} aria-hidden="true" className="autosave-spinner" />
        Saving…
      </span>
    );
  }
  if (state === "draft-saved") {
    return (
      <span className="autosave-status autosave-muted" aria-live="polite">
        Draft saved
      </span>
    );
  }
  return (
    <span className="autosave-status autosave-saved" aria-live="polite">
      <Check size={14} aria-hidden="true" />
      Saved
    </span>
  );
}
