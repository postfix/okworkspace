import { AlertTriangle, Loader2, Lock } from "lucide-react";

import "./SoftLockBanner.css";

// SoftLockBanner is the non-blocking WARNING shown when another live session holds
// the soft lock on the page you are editing (COLL-02). It is built on the shared
// `.banner banner-warning` primitive and carries role="status" (NOT role="alert")
// because it is informative and recoverable — the editor stays usable via the
// "Force edit" take-over button. Color is never the sole signal: the Lock /
// AlertTriangle icon and the lead text carry the meaning alongside the warning hue.
//
// The copy is VERBATIM from the Phase 5 UI-SPEC Copywriting Contract and uses NO
// Git vocabulary. Crucially the banner never implies that taking over makes your
// next save safe — "Your changes won't be saved until you take over" is a turn
// statement, not a save-safety promise (the save-time revision check is untouched).
//
// holderName renders as plain React text (auto-escaped) — never via
// dangerouslySetInnerHTML — so a crafted username cannot inject markup (T-05-10).
interface SoftLockBannerProps {
  // holderName is the username of the session currently holding the lock.
  holderName: string;
  // busy is true while a force-edit (take-over) request is in flight — the Lock
  // icon becomes a spinner and the button disables with "Taking over…".
  busy?: boolean;
  // failed is true after a transient force-edit failure — the icon becomes an
  // AlertTriangle and the button re-enables as a retry with the failure copy.
  failed?: boolean;
  // onForceEdit takes over the lock (calls forceLock, never a save).
  onForceEdit: () => void;
}

export default function SoftLockBanner({
  holderName,
  busy = false,
  failed = false,
  onForceEdit,
}: SoftLockBannerProps) {
  return (
    <div
      className="banner banner-warning softlock-banner"
      role="status"
      aria-live="polite"
    >
      <span className="softlock-icon" aria-hidden="true">
        {busy ? (
          <Loader2 size={16} className="spinner" />
        ) : failed ? (
          <AlertTriangle size={16} />
        ) : (
          <Lock size={16} />
        )}
      </span>
      <span className="softlock-text">
        {failed ? (
          "Couldn't take over just now — check your connection and try again."
        ) : (
          <>
            <span className="softlock-lead">{holderName} is editing this page.</span>{" "}
            Your changes won't be saved until you take over.
          </>
        )}
      </span>
      <span className="softlock-spacer" />
      <button
        type="button"
        className="btn btn-secondary softlock-force"
        aria-label="Take over editing this page"
        onClick={onForceEdit}
        disabled={busy}
      >
        {busy ? "Taking over…" : "Force edit"}
      </button>
    </div>
  );
}
