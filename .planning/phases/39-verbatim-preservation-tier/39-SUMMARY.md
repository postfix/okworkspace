---
author: engineer
responsible: architect
phase: 39
phase_type: implementation
status: EXECUTE COMPLETE — GO
verdict: GO
requirements: [RE-02]
security_relevant: true
---

# Phase 39: Verbatim Preservation Tier (RE-02, MAJ-04) - Summary

**Verdict: GO.** The Tier-2 verbatim-preservation mechanism (`PreservedBlock`/`PreservedInline`/
`BlockMath` atom nodes + block/inline math + block/inline HTML tokenizers) round-trips every Tier-2
construct **byte-for-byte (modulo outer trim)** through the Phase-38 codec, keeps raw HTML **inert**
(MAJ-04: exact source bytes AND no live node/attribute, no DOM path), composes with the Tier-1 slice
in mixed documents, and does **not regress** the Phase-38 Tier-1 corpus or cross-engine parity.

**No documented byte-parity limitation was needed** — inline HTML (the research-flagged fragile
construct) reached byte-exactness via the custom inline-tokenizer route. No byte assertion was
loosened.

## What shipped

- **`web/src/lib/tiptap/preserved.ts`** (NEW, import-isolated) — the Tier-2 nodes, using ONLY the real
  `@tiptap/markdown` 3.27.1 config fields (verified natively typed on `@tiptap/core`'s `NodeConfig`
  at `index.d.ts:756-770` — no module augmentation needed):
  - **`PreservedBlock`** — block atom (`atom:true`, `code:true`, `raw` attr). `markdownTokenName:"table"`
    intercepts the native GFM `table` token; `parseMarkdown` stores `token.raw`; `renderMarkdown`
    returns `node.attrs.raw` verbatim (custom-node output bypasses the escaper entirely,
    `index.js:1076-1080`). It is the shared RENDER node for tables AND block HTML.
  - **`PreservedBlockHtml`** — a thin parse-only `Extension` (no schema node) with
    `markdownTokenName:"html"` routing the block `html` token → `preservedBlock` (1:1 token→handler,
    so the second registration is a sibling; render is owned by `PreservedBlock` via the manager's
    node-name registry). Registered handlers run BEFORE the built-in `parseHTMLToken` fallback
    (`index.js:571-590`) — this replaces the Phase-38 lossy-escaped block-HTML path with byte-exact.
  - **`BlockMath`** — block atom + a custom BLOCK `markdownTokenizer` for `$$…$$` (`.raw` = the exact
    slice). Its `start` reports only a **complete, line-anchored** `$$…$$` position, so a stray `$$` in
    prose never cuts a paragraph (the over-match that would inject a spurious newline).
  - **`PreservedInline`** — inline atom (`inline:true`, `atom:true`, `code:true`, `raw` attr), the
    RENDER node for inline math + inline HTML. An inline atom (not a mark) avoids the mark open/close
    placeholder dance, so arbitrary bytes round-trip as-is.
  - **`InlineMath`** — parse-only + tokenizer `Extension` for `$…$` (guards: `(?!\$)` never eats a
    `$$` block delimiter; `[^$\n]+?` keeps it single-token; falls through to plain text on a lone `$`).
  - **`InlineHtml`** — parse-only + tokenizer `Extension` for inline HTML. Critically it emits a token
    type `"inlineHtml"` (≠ `'html'`) so it LANDS IN THE GENERIC HANDLER PATH (`index.js:822-826`),
    bypassing marked's pre-emptive inline-`html` merge branch (`index.js:773-812`) that would
    otherwise force the lossy DOM path. Regex matches paired `<tag …>…</tag>`, self-closing/void
    single tags, and bare closing tags; requires `[a-zA-Z]` after `<` so a spaced `<` in prose
    (`a < b`) never matches.
  - **`preservedExtensions`** — the exported array, appended by `buildExtensions()`.
- **`web/src/lib/tiptap/extensions.ts`** — `buildExtensions()` now spreads `...preservedExtensions`
  after the Tier-1 slice. Purely additive; each preserved extension intercepts a marked token type the
  Tier-1 slice does not model, so Tier-1 fixtures are untouched.
- **`web/src/lib/tiptap/codec.corpus.ts`** — `TIER2_CORPUS` (8 fixtures: `table`, `tableAligned`,
  `blockMath`, `blockMathMultiline`, `inlineMath`, `inlineMathTwice`, `blockHtml`, `inlineHtml`) +
  `MIXED_CORPUS` (3 interleaved Tier-1+Tier-2 docs). `SECURITY_CORPUS` retained; its comment updated to
  the sharpened byte-exact-inert contract.
- **`web/src/lib/tiptap/codec.test.ts`** — a **Tier-2 byte-equality** describe
  (`roundTrip(src).trim() === src.trim()` + idempotence, over `TIER2_CORPUS` ∪ `MIXED_CORPUS`), and a
  **rewritten MAJ-04 section** (the Phase-38 escaped-output assertions were REPLACED, not loosened):
  raw HTML now asserts EXACT source bytes AND inertness (no forbidden element node, no executable attr
  key, HTML carried only in a `raw` string) under the node env. `ALLOWED_NODE_TYPES` extended with the
  preserved atoms (converted to a `Record<string,true>` per the repo lookup-table rule).

## Per-construct byte-parity results (the RE-02 gate — every one PASSES, byte-for-byte modulo outer trim)

| Construct | Fixture(s) | Byte-exact | Idempotent | Notes |
|---|---|---|---|---|
| GFM table | `table`, `tableAligned` | ✓ | ✓ | Native `table` token; trailing block-separator `\n\n` (folded by marked into `.raw` when a block follows) stripped — it belongs to the serializer, rows are exact. |
| Block math `$$…$$` | `blockMath`, `blockMathMultiline` | ✓ | ✓ | Custom block tokenizer; multi-line LaTeX (`\begin{aligned}…\\…`) exact. |
| Inline math `$…$` | `inlineMath`, `inlineMathTwice` | ✓ | ✓ | Custom inline tokenizer; two spans in one paragraph re-locate correctly. |
| Block HTML | `blockHtml` | ✓ | ✓ | `markdownTokenName:'html'` handler stores merged `.raw`; same block-separator strip. |
| **Inline HTML** | `inlineHtml` | **✓** | ✓ | Custom inline tokenizer (name ≠ `'html'`) — **byte-exact, NO limitation needed**. |
| Mixed Tier-1+Tier-2 | `headingTableMathCode`, `inlineMixedThenList`, `blockHtmlBetweenParas` | ✓ | ✓ | Preserved atoms compose with the bounded slice; interior blocks keep single blank-line separation. |

## MAJ-04 (SECURITY) result

Raw HTML is **exact-byte preserved AND inert**, verified under the `node` env (no `window.DOMParser`):
- `imgOnerror` (`<img src=x onerror="alert(1)">`), `scriptTag`, `divOnclick`, `inlineEm` all round-trip
  to their EXACT source bytes (byte-equality, NOT escaped).
- The parsed JSON has **no forbidden element node** (no `image`/`div`/`script`/`span`/`em`/`iframe`)
  and **no executable attribute key** (`onerror`/`onclick`/`onload`/`src`/`href`/`onmouseover`). The
  HTML is held solely in a `preservedBlock`/`preservedInline` **`raw` string**; `renderMarkdown`
  re-emits it as literal source.
- **No DOM path:** `grep -n 'generateJSON' web/src/lib/tiptap/preserved.ts` → **0 matches**. The `html`
  handler intercepts the token before `parseHTMLToken` can reach the DOM branch; the node-env guard is
  re-asserted inside the MAJ-04 describe so a future accidental jsdom switch fails loudly.

## Tier-1 no-regression

The Phase-38 Tier-1 corpus (tree-equality + idempotence) and `parity.test.ts` (cross-engine
`@lezer/markdown` GFM parity) pass **unchanged** — the preserved extensions are additive. Verified that
the tokenizers do NOT over-match in ways that break the gate:
- An escaped `\$` and a stray/spaced `<` (`a < b`) stay plain text (the HTML tokenizer requires
  `[a-zA-Z]` after `<`; the `^`-anchored regexes fire only at the current lex position).
- **CORRECTION (qa P3, `preserved.ts:198-208`):** the inline-math tokenizer DOES fire on two-dollar
  prose — `It costs $5 and $10` captures `$5 and $` as a `preservedInline` atom (NOT plain text, as an
  earlier draft of this SUMMARY wrongly stated). This does **not** fail Phase 39's gate: it still
  round-trips **byte-exact**, and `TIER1_CORPUS` contains no `$`, so neither Tier-2 byte-parity nor
  Tier-1 tree-equality/parity regress. It IS a latent live-editor semantic (prose currency would
  surface as a math atom in the Phase-41 TipTap surface) — **routed to Phase 41** to tighten the
  inline-math delimiter guard (pandoc no-space-adjacent-to-`$` rule) + add prose-currency fixtures.
- `$`/`<` **inside inline code** (`` `$var` ``, `` `<tag>` ``) round-trip exactly as code (the codespan
  built-in consumes the span before the anchored math/HTML tokenizers can match).

## Verification (all run; all green)

- `cd web && npx vitest run --environment node src/lib/tiptap/` → **83 passed** (was 56 at Phase-38
  baseline; +27 Tier-2 + sharpened-MAJ-04 tests). No skips/todos.
- `cd web && npx tsc --noEmit` → exit **0**.
- `cd web && npx eslint src/lib/tiptap/` → exit **0**.
- `cd web && npm run build` (`tsc -b && vite build`) → exit **0** (stays green; only pre-existing
  CM6 chunk-size/lazy-import warnings, unrelated to the import-isolated codec).
- Acceptance greps: `PreservedBlock|PreservedInline|markdownTokenName|markdownTokenizer` matches (26
  lines); `generateJSON` = 0; `TIER2_CORPUS`/`MIXED_CORPUS` exported; `preserved` registered in
  `extensions.ts`.

## Scope / files touched

ONLY `web/src/lib/tiptap/*`: `preserved.ts` (new), `extensions.ts`, `codec.corpus.ts`, `codec.test.ts`.
No file outside that directory was modified — confirmed zero collision with Phase 40 (editor-host
abstraction) both by `git diff --stat` and directly with Phase40Engineer over IRC.

## Notes for the independent verifier (qa)

- Hard-bar honesty: byte assertions are `roundTrip(src).trim() === src.trim()` — the outer `.trim()` is
  the sanctioned "modulo outer trim" allowance (serializer owns inter-block blank-line joins). No
  fixture was skipped; no assertion was widened to "close enough". Inline HTML is byte-exact, not a
  documented partial.
- The block-separator strip (`.replace(/\n+$/, "")` in the table + block-HTML `parseMarkdown`) removes
  ONLY trailing block-boundary newlines that marked greedily folds into `.raw`; internal newlines
  (multi-line HTML/tables) are preserved. Without it, interior tables/HTML double the blank line and
  break idempotence — proven by the mixed fixtures.
- MAJ-04 stays node-env; the DOM path is never reachable (handler intercept + no-window guard).

**VERIFICATION.md is intentionally NOT written by this engineer** — the independent qa pass owns it.
