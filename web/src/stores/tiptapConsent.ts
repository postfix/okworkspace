// tiptapConsent store (RE-04-3, D-03). The ONE-TIME informed-consent dismissal is a
// per-device UI preference (not server state), persisted to localStorage so the user is
// informed ONCE — never re-nagged — across reloads. Mirrors the editorMode.ts zustand +
// persist idiom; the persisted key is "okf.tiptap.consent". PageEditor reads `dismissed`
// to gate the TipTapConsentNotice banner; the banner's "Got it" calls `dismiss`.
//
// This is the ENTIRE resolution of the shared-content fidelity design fork: the co-driver
// chose INFORMED CONSENT (a tree-equality load guard + this one-time notice), NOT the
// byte-delta guard (option ii, rejected). Because the load guard (lib/tiptap/guard.ts) is
// TREE-equality, it PASSES modeled-syntax normalization (`*`/`+` bullets -> `-`, `__`/`_`
// emphasis -> `**`/`*`, loose lists tightened) — semantically lossless, but those BYTES
// reflow for everyone viewing the page on the next save. The guard alone can't inform the
// user of that; this notice does — once (dismiss-persisted, not a per-save nag).
import { create } from "zustand";
import { persist } from "zustand/middleware";

interface TipTapConsentState {
  // dismissed is true once the user has acknowledged the notice (persisted forever).
  dismissed: boolean;
  // dismiss records the informed consent permanently. Idempotent.
  dismiss: () => void;
}

export const useTipTapConsent = create<TipTapConsentState>()(
  persist(
    (set) => ({
      dismissed: false,
      dismiss: () => set({ dismissed: true }),
    }),
    { name: "okf.tiptap.consent" },
  ),
);
