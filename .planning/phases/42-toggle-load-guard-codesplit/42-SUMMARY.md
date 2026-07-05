---
author: engineer
phase: 42
requirements: [RE-04]
status: Executed (verification pending — do not mark complete)
design_fork_resolution: "informed-consent (tree-equality guard + one-time notice) — AS BUILT"
---

# Phase 42: Toggle + Load Guard + Code-Split — EXECUTE SUMMARY

Turned the Phase-41 TipTap surface (built + lazy-mounted, not user-reachable) into a shipped,
guarded, opt-in feature. The DECIDED design fork (co-driver: **informed consent**) is implemented
exactly as chosen: a **tree-equality** load guard + a **one-time notice**. NO byte-delta fallback
(rejected option ii) was added — the guard stays tree-equality.

## Files
- **NEW** `web/src/lib/tiptap/guard.ts` — `isLosslessRoundTrip(md)` + a structural `deepEqual`.
- **NEW** `web/src/lib/tiptap/guard.test.ts` — node env, 33 cases.
- **NEW** `web/src/components/TipTapConsentNotice.tsx` — pure one-time notice component.
- **NEW** `web/src/components/TipTapConsentNotice.css` — banner styling (token-only).
- **NEW** `web/src/stores/tiptapConsent.ts` — zustand+persist dismissal store (mirrors `editorMode.ts`).
- **MOD** `web/src/routes/PageEditor.tsx` — 3-way toggle, guarded entry (both open + switch), notice wiring.
- **MOD** `web/src/routes/PageEditor.test.tsx` — TipTap-mode 409 confirmation + TipTap/guard mocks + store reset.
- **MOD** `web/src/routes/PageEditor.css` — 3-way segmented-control layout.
- **MOD** `web/vite.config.ts` — `TIPTAP_FORBIDDEN` + `SHARED_FORBIDDEN` (marked dual-root); comment fixed.

> Store split note: the consent dismissal lives in `stores/tiptapConsent.ts` (not colocated in the
> notice component). This is the DIRECT mirror of `editorMode.ts` the CONTEXT names as the pattern,
> and it eliminates the only new eslint diagnostic (a `react-refresh/only-export-components` warning
> from co-exporting a hook beside a component). Zero lint regressions; the plan grants this discretion
> ("where things live", "commit slicing").

## must_haves — evidence

### RE-04-1 — user-selectable 3-way toggle; CM6 default; tiptap routes through the guard
- `PageEditor.tsx` toolbar: a `role="group"` segmented control with Live (Eye) / Source (Code) /
  Rich (Sparkles). "live"/"source" call `selectMode` → `setMode` directly (CM6, byte-identical).
  CM6 `"live"` stays the store default (`editorMode.ts` unchanged). "Rich" routes through the guard.
- The Rich segment reads `aria-pressed` only when `mode==="tiptap" && tiptapAllowed` — a blocked
  attempt never falsely lights it.

### RE-04-2 — isLosslessRoundTrip = tree-equality; guard on BOTH entries; false → warn + fall back
- `guard.ts`: `isLosslessRoundTrip(md) = deepEqual(parse(roundTrip(md)), parse(md))`, `try/catch → false`.
  Composes the exported headless codec. `deepEqual` is a key-order-independent structural compare
  (not JSON.stringify).
- **Both entries, current live buffer:** `selectMode("tiptap")` runs the guard on `bodyRef.current`
  (switch); an **open-guard effect** runs it once on first data arrival when the persisted mode is
  already `"tiptap"` (open). `tiptapAllowed` gates the mount, so a persisted `"tiptap"` never mounts
  TipTap before the guard clears the buffer.
- **false → warn AND fall back (BLK-03, a guard not a toast):** on rejection, `tiptapAllowed` stays
  false, `tiptapBlocked` shows "This page has content the rich editor can't preserve — staying in the
  Markdown editor.", and the mode is NOT set to tiptap (open-path also `setMode("live")`). TipTap is
  never entered on non-round-trippable content.
- **Code-split safety:** the guard is **dynamically imported** (`await import("../lib/tiptap/guard")`)
  because it transitively pulls `@tiptap/markdown` + `marked` — a static import would violate the new
  entry-chunk forbidden set. A justifying comment marks it a genuine runtime-gated exception.
- Guard test (node env) — 33 pass: whole Tier-1/Tier-2/Mixed corpus + `#tag` + empty → true;
  `*`/`+` bullets and `__`/`_` emphasis normalize (bytes change, verified) but stay tree-equal → true
  (informed consent); a markdown **image** (unmodeled — StarterKit omits Image; URL dropped to bare
  alt, tree diverges doc>text vs doc>paragraph>text) → false; the throw backstop → false.
  `grep -n isLosslessRoundTrip guard.ts PageEditor.tsx` → both match.

### RE-04-3 — one-time informed-consent notice; shown on first TipTap entry; not a per-save nag
- `TipTapConsentNotice.tsx`: "The rich editor normalizes bullet, emphasis, and spacing style. Saved
  changes reflow those bytes for everyone viewing this page." + "Got it" → `dismiss()`.
- `stores/tiptapConsent.ts`: zustand+persist, key `okf.tiptap.consent` (mirrors `okf.editor.mode`),
  `dismissed`/`dismiss`. `grep -nE 'consent|localStorage' TipTapConsentNotice.tsx tiptapConsent.ts` → match.
- PageEditor renders it only when `mode==="tiptap" && tiptapAllowed && !consentDismissed`. Does NOT
  block editing; dismiss persists → never re-shown; NOT re-shown per save. **No byte-delta fallback**
  — the guard remains pure tree-equality (fork honored as chosen).

### RE-04-4 — forbidden set covers the whole TipTap tree; marked dual-root comment fixed; build green
- `vite.config.ts`: `TIPTAP_FORBIDDEN = [/node_modules[\\/]@tiptap[\\/]/, /node_modules[\\/]prosemirror-[a-z-]+[\\/]/]`;
  `marked` moved out of the mermaid-unique list into `SHARED_FORBIDDEN` with an honest dual-root
  comment (mermaid@16 + @tiptap/markdown@17); all three spread into `FORBIDDEN_IN_ENTRY`. The stale
  "mermaid-UNIQUE" `marked` line is corrected.
- `grep -nE '@tiptap|prosemirror|TIPTAP_FORBIDDEN' vite.config.ts` → match.
- `npm run build` → **exit 0** with the extended `assertLazyChunks`. Verified NON-vacuous: entry
  chunk `index-*.js` has **0** occurrences of `@tiptap` / `prosemirror-` / `MarkdownManager` /
  `ProseMirror`, while those modules DO exist in **lazy** chunks (`extensions-*.js`, `chunk-*.js`,
  and TipTapEditor's own chunk) — a future stray static import now fails the build.

### RE-04-5 — TipTap-mode save is revision-checked (409 confirmed); runSaver untouched
- `runSaver` NOT touched. New `PageEditor.test.tsx` case: switch to Rich (mocked-safe guard mounts the
  mocked TipTap surface), edit, Save; a stale-revision `savePage` 409 opens the SAME conflict dialog
  ("This page was changed somewhere else" + "Overwrite with my version") as CM6 — TipTap uses the same
  runSaver. Confirmation only, no new save path.

## Verification run (engineer-side; independent QA pending)
- `npx vitest run` (full suite) → **70 files, 971 tests pass** (guard.test.ts 33 + the tiptap-409 test
  included). The `act(...)` stderr lines are pre-existing warnings, not failures.
- `npx tsc --noEmit` → **exit 0**.
- `npx eslint src/` → 9 errors + 3 warnings, ALL pre-existing in untouched files (PresenceIndicator,
  GraphView, AppShell, TagManagement, AttachmentCard, NavSearchResults, TrashView, hashtag.parity.test,
  and the pre-existing `connId.current` in PageEditor). Measured baseline (my changes stashed) = same 9
  errors; my 5 new/edited-scope files produce **zero** diagnostics — zero lint regressions. CI (`ci.yml`)
  gates on `npm run test` + `npm run build`, not eslint.
- `npm run build` → **exit 0**, assertLazyChunks green, entry chunk tiptap/prosemirror/marked-free.

## Fork resolution AS BUILT
Informed consent, exactly: **tree-equality** load guard (passes modeled-syntax normalization,
fails genuine loss/throw) + a **one-time dismiss-persisted notice**. No byte-delta behavior was
added anywhere — the guard is not byte-equality, and the notice is the whole fidelity-fork resolution.
