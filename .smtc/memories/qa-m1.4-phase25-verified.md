# qa — Phase 25 Code Syntax Highlighting (ED-04a) VERIFIED (m1.4)

**Verdict: pass** — all 9 must-have truths hold against code reality. Commits `98eb4ca..ff6b226`, frontend-only.

## What shipped
- ONE shared `codeHighlightStyle` (web/src/lib/cm/highlight.ts:34) drives BOTH CM6 surfaces:
  live via `syntaxHighlighting()` (mode.ts), read via `highlightCode(..., codeHighlightStyle)` (widgets.ts CodeBlockWidget).
- Grammars code-split (`LanguageDescription.of({ load: () => import() })`); read widget renders plain → re-highlights on parser resolve; teardown-safe (destroyed flag + `codeEl.isConnected` + atomic detached-fragment swap).
- Token spans createElement + textContent ONLY (extends T-06-08 no-innerHTML). Unknown lang + ```mermaid stay plain (`matchLanguageName` fuzzy=false).

## Durable technique: prove a build-assertion has TEETH without touching prod
The `assertLazyChunks` vite plugin (web/vite-plugins/assertLazyChunks.ts) is a `generateBundle` Rollup plugin that `this.error`s on a forbidden module in an entry chunk or oversize entry. I proved teeth LIVE via a `vite-node` harness: import the plugin, call `generateBundle.call({error(){throw}}, {}, syntheticBundle)` with the REAL production forbidden-regex. Confirmed: lazy grammar→throw, core-4→pass, oversize→throw, prefix-evasion `lang-cssinjs`→throw. Reusable pattern for Phase 26 (mermaid).

## Bundle-budget ruling (the judgment call ED-04a needed)
Engineer excluded `lang-(markdown|html|css|javascript)` from the forbidden list. JUSTIFIED, not scope-cheating:
- `lang-markdown` is a PRE-PHASE direct dep (`git show 98eb4ca^:web/package.json`) and hard-depends on `lang-html` → which hard-depends on `lang-css` + `lang-javascript` (node_modules dep chains). Verify via the package.json `dependencies` blocks.
- So those 4 are unavoidably in the entry chunk and were there pre-phase; the 4 `INEFFECTIVE_DYNAMIC_IMPORT` build warnings name exactly this chain (expected).
- All NON-pre-existing per-fence grammars (python/go/rust/json/yaml/sql + legacy shell/diff) stay forbidden AND proven absent (build passes with teeth-proven assertion + 0 entry markers).
- ED-04a "main chunk grows by core+style NOT per-language grammars" satisfied in spirit; js/ts riding pre-existing lang-javascript is no-bloat reuse.

## Evidence baseline (web/)
tsc -b exit 0; vitest 685/685 (59 files); npm run build exit 0 with assertion active; entry 1,180,616 B < 1,420,000 budget; Go/internal/cmd diff empty.

## Note
Entry chunk ~1.18 MB triggers Vite 500kB warning — PRE-EXISTING (CM6+app), not phase-induced. Bump `maxEntryBytes` (vite.config.ts) only on legit entry growth.
