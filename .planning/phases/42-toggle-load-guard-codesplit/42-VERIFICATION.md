---
phase: 42
title: Toggle + Load Guard + Code-Split (RE-04)
verifier: Phase42QA
date: 2026-07-05
verdict: PASS
design_fork: informed-consent (tree-equality guard + one-time notice) — AS BUILT, option ii NOT present
---

# Phase 42 (RE-04) — INDEPENDENT VERIFICATION: **PASS**

The DECIDED design fork (co-driver: **informed consent**, REQUIREMENTS.md:233-250, option i)
is implemented **exactly** as chosen — a **tree-equality** load guard + a **one-time notice**,
with **NO byte-delta fallback** (rejected option ii is absent). All six honesty checks hold.

## Evidence (file:line)

**1. Guard is TREE-equality, not byte-equality (RE-04-2, fork core) — PASS**
- `web/src/lib/tiptap/guard.ts:75-83` — `isLosslessRoundTrip(md) = deepEqual(parse(roundTrip(md)), parse(md))`,
  `try/catch → false`. It compares PARSED TREES, never `roundTrip(md) === md`. `deepEqual`
  (`guard.ts:29-63`) is a hand-rolled key-order-independent structural compare (not JSON.stringify).
- `web/src/lib/tiptap/guard.test.ts:63-91` — modeled-syntax normalization (`* `→`- `, `+ `,
  `__`/`_` emphasis) asserts `roundTrip(x) !== x` (bytes DO change) **AND** `isLosslessRoundTrip(x) === true`
  (tree-equal → passes). This is the explicit fork-honesty assertion: a byte-equality guard would
  (wrongly) return false here.
- `web/src/lib/tiptap/guard.test.ts:93-118` — round-trippable corpus → true; markdown image
  (unmodeled, URL dropped, tree diverges) → false; codec throw → false.
- codec (`web/src/lib/tiptap/codec.ts:26-44`) `parse`/`serialize`/`roundTrip` = headless MarkdownManager.

**2. NO byte-delta fallback (RE-04-3) — PASS**
- Grep + read of `PageEditor.tsx` finds exactly ONE fallback trigger: guard `false`
  (`PageEditor.tsx:247-253`, `273-281`). No `=== md`, no byte-delta comparison, no per-save nag anywhere.
- The notice carries normalization awareness, NOT a guard: `TipTapConsentNotice.tsx` +
  `stores/tiptapConsent.ts` — one-time, dismiss-persisted to localStorage `okf.tiptap.consent`
  (`tiptapConsent.ts:28-35`), non-blocking (`role="status"`, does not gate editing),
  rendered only when `mode==="tiptap" && tiptapAllowed && !consentDismissed`
  (`PageEditor.tsx:1081-1083`) — not per save.

**3. Guard wiring — guard, not toast, on BOTH entries against live buffer (BLK-03/MED-07) — PASS**
- switch-to-tiptap: `selectMode` (`PageEditor.tsx:264-285`) runs `verifyBufferForTipTap(bodyRef.current)`;
  pass → `setTiptapAllowed(true); setMode("tiptap")`; fail → `setTiptapBlocked(true)`, mode NOT changed
  (stays CM6), TipTap never mounts.
- open-in-tiptap: open-guard effect (`PageEditor.tsx:243-255`) — persisted `mode==="tiptap"` re-runs the
  guard on `bodyRef.current`; fail → `setTiptapBlocked(true); setMode("live")` (falls back to CM6).
- Both use the CURRENT live buffer `bodyRef.current`. Mount gate `renderEditorSurface`
  (`PageEditor.tsx:837`) requires `mode==="tiptap" && tiptapAllowed`, so TipTap is NEVER mounted on
  non-round-trippable content. Fallback warning banner at `PageEditor.tsx:1010-1014` (visible, `role="status"`).
- 3-way toggle: `PageEditor.tsx:886-917` (Live / Source / Rich). CM6 "live" is default —
  `stores/editorMode.ts:31` (`mode: "live"`); "tiptap" never auto-selected.

**4. Code-split — TipTap tree build-enforced out of entry (RE-04-4, MIN-04/MIN-08) — PASS**
- `web/vite.config.ts:57-60` `TIPTAP_FORBIDDEN = [/node_modules[\/]@tiptap[\/]/,
  /node_modules[\/]prosemirror-[a-z-]+[\/]/]`; `:63-66` `SHARED_FORBIDDEN = [/node_modules[\/]marked[\/]/]`
  (marked dual-root); all in `FORBIDDEN_IN_ENTRY` (`:68-73`).
- Dual-root comment CORRECTED: `vite.config.ts:38-43` — marked declared DUAL-ROOTED (mermaid@16 +
  @tiptap/markdown@17), moved OUT of the mermaid-unique list into SHARED_FORBIDDEN with an honest comment.
- `npm run build` → **exit 0** (assertLazyChunks passed). Entry chunk `dist/assets/index-B6YH3SdQ.js`
  grep `@tiptap/|prosemirror-|MarkdownManager` → **0 matches** (clean). Non-vacuous: those strings DO
  appear in lazy chunk `dist/assets/extensions-*.js`.
- Guard is `await import(...)` in PageEditor (`PageEditor.tsx:227`) — REQUIRED (guard→codec→@tiptap/marked);
  a static import would leak the tree into entry. Known/expected, not a defect.

**5. Save untouched (RE-04-5) — PASS**
- `git diff PageEditor.tsx`: the ONLY line mentioning `runSaver` is a comment reflow
  (`PageEditor.tsx:158`); the `runSaver` body (`:341-` region), save loop, and `savePage` calls are
  byte-unchanged. TipTap adds no save path.
- New test `PageEditor.test.tsx:948-980` — a stale-revision save from (guarded, mocked-safe) TipTap mode
  409s and opens the SAME conflict flow as CM6.

## Tests
- `npx vitest run src/lib/tiptap/guard.test.ts src/routes/PageEditor.test.tsx` → **2 files, 53/53 passed**.
- `npm run build` → **exit 0**, entry chunk clean, TipTap tree lazy.

## Verdict
**PASS.** Tree-equality guard (modeled-syntax normalization passes; genuine loss/throw falls back),
one-time dismiss-persisted non-blocking notice, NO byte-delta fallback (option ii absent), guard-not-toast
on both open + switch against the live buffer, CM6 "live" default, TipTap build-enforced out of the entry
chunk. Fork honored exactly as the co-driver decided (informed consent). No gaps.
