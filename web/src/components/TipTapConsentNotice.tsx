import { Info } from "lucide-react";

import { useTipTapConsent } from "../stores/tiptapConsent";
import "./TipTapConsentNotice.css";

// TipTapConsentNotice — the ONE-TIME informed-consent notice (RE-04-3, D-03). This is the
// ENTIRE resolution of the shared-content fidelity design fork: the co-driver chose
// INFORMED CONSENT (a tree-equality load guard + this notice), NOT the byte-delta guard
// (option ii, rejected). Because the load guard (lib/tiptap/guard.ts) uses TREE-equality,
// it PASSES modeled-syntax normalization (`*`/`+` bullets -> `-`, `__`/`_` emphasis ->
// `**`/`*`, loose lists tightened) — semantically lossless, but those BYTES reflow for
// everyone viewing the page on the next save. The guard alone can't inform the user of
// that, so this notice does — ONCE (the dismissal persists via the tiptapConsent store).
//
// It is a non-blocking, dismiss-persisted banner (NOT a gate, NOT a per-save nag): editing
// works whether or not it is dismissed, and "Got it" persists the dismissal so it is never
// shown again. It composes the shared `.banner` primitive with role="status" (informative,
// not an alert) and an Info glyph so color is never the sole signal. PageEditor renders it
// only while TipTap is actually live and consent has not been dismissed.
export default function TipTapConsentNotice() {
  const dismiss = useTipTapConsent((s) => s.dismiss);
  return (
    <div
      className="banner tiptap-consent-notice"
      role="status"
      aria-live="polite"
    >
      <span className="tiptap-consent-icon" aria-hidden="true">
        <Info size={16} />
      </span>
      <span className="tiptap-consent-text">
        <span className="tiptap-consent-lead">The rich editor normalizes</span>{" "}
        bullet, emphasis, and spacing style. Saved changes reflow those bytes for
        everyone viewing this page.
      </span>
      <span className="tiptap-consent-spacer" />
      <button
        type="button"
        className="btn btn-secondary tiptap-consent-dismiss"
        onClick={dismiss}
      >
        Got it
      </button>
    </div>
  );
}
