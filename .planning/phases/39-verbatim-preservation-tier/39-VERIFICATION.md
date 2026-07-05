---
author: qa
responsible: architect
phase: 39
phase_type: verification
verdict: PASS
requirements: [RE-02]
security_relevant: true
verifies: 39-SUMMARY.md
---

# Phase 39 (RE-02) — Independent Verification

**VERDICT: PASS.** Tier-2 byte-parity is REAL byte-equality (strict `.toBe`, per fixture, +
idempotence); MAJ-04 raw HTML is byte-exact AND inert (no element node / no executable attr, node
env); Tier-1 is intact (parity + tree-equality unchanged, all green); nothing loosened, no
`.skip`/`.todo`. The engineer's GO holds against code reality. One P3 SUMMARY-wording caveat noted
below — does NOT block GO.

Verified against CODE, not the SUMMARY. Ran the phase suite (+ a transient runtime probe, removed).

## Gate re-run (ground truth)

`cd web && npx vitest run --environment node src/lib/tiptap/` → **2 files, 83 passed (83)**, 0 skipped.
Verbose enumeration confirms every fixture is a real iterated case (no phantom/zero-fixture loop):
- Tier-1 gate: 14 fixtures × (tree-equality + idempotence) + node-env guard — `codec.test.ts`.
- Tier-2 byte-equality gate: 8 `TIER2_CORPUS` + 3 `MIXED_CORPUS` = 11 fixtures × (byte-for-byte +
  idempotence) — `codec.test.ts:110-120`.
- MAJ-04: 4 `SECURITY_CORPUS` × (exact-bytes + inert + raw-only + idempotent) + explicit `<img
  onerror>` case + node-env guard — `codec.test.ts:133-207`.
- Cross-engine parity: 14 Tier-1 fixtures — `parity.test.ts`.

## 1. Byte-parity is REAL (not loosened) — PASS

- `codec.test.ts:113` — `expect(roundTrip(src).trim()).toBe(src.trim())`. Strict `.toBe` string
  equality, per Tier-2 + mixed fixture. The `.trim()` is the sanctioned "modulo outer trim" only
  (strips outer whitespace; interior bytes must match exactly). No `contains`/`toMatch`/`toBeTruthy`
  substitute, no partial-match, no `.skip`/`.todo`, no relaxed comparator anywhere in the file.
- `codec.test.ts:117` — idempotence `expect(roundTrip(once)).toBe(once)` per fixture.

## 2. Corpus coverage complete — PASS

`codec.corpus.ts` — GFM table (`table`, `tableAligned` w/ `:--`/`:-:`/`--:` alignment), `$$` block
math (`blockMath`, `blockMathMultiline` w/ LaTeX backslashes), `$` inline math (`inlineMath`,
`inlineMathTwice`), block HTML (`blockHtml`), inline HTML (`inlineHtml`), + 3 interleaved Tier-1+Tier-2
(`headingTableMathCode`, `inlineMixedThenList`, `blockHtmlBetweenParas`). The SUMMARY's "inline HTML
byte-exact, no limitation" is backed by a REAL byte-equality fixture — `inlineHtml`
(`codec.corpus.ts:108`) runs through the same `.toBe` Tier-2 gate and passes (verbose run confirms) —
not a documented-away limitation.

## 3. `preserved.ts` uses real 3.27.1 hooks, no DOM path — PASS

- `@tiptap/markdown` / `@tiptap/core` / `@tiptap/starter-kit` all **3.27.1** (`package.json`). Hook
  fields are NATIVELY typed on `@tiptap/core`'s NodeConfig: `markdownTokenName` (index.d.ts:756),
  `parseMarkdown` (:760), `renderMarkdown` (:764), `markdownTokenizer` (:770) — no augmentation.
- `parseMarkdown` stores `token.raw` (`preserved.ts:72,105,161,205,222`); `renderMarkdown` returns
  `node.attrs.raw` verbatim (`preserved.ts:74,132,163`); `blockMathTokenizer` registered for `$$`
  (`preserved.ts:143-197`).
- `grep generateJSON web/src/lib/tiptap/{preserved,codec,extensions}.ts` → **0 code invocations** (only
  docblock mentions in corpus/test comments). No `new Editor`, no `DOMParser`, no `innerHTML`. `parse`
  goes through headless `MarkdownManager` only (`codec.ts:28`).

## 4. MAJ-04 inert + exact under node env — PASS

- `codec.test.ts:137,157` — node-env guard `expect(typeof globalThis.window).toBe("undefined")` (+
  document) asserted inside BOTH the Tier-1 and MAJ-04 describes — load-bearing (no `window.DOMParser`
  ⇒ HTML→DOM path closed).
- `codec.test.ts:167` — raw HTML asserts EXACT source bytes `roundTrip(src).trim() === src.trim()`.
- `codec.test.ts:171-179` — inertness: every emitted type ∈ ALLOWED (bounded slice + preserved atoms);
  FORBIDDEN element types (`image`/`div`/`script`/`span`/`em`/`iframe`) absent; EXECUTABLE attr KEYS
  (`onerror`/`onclick`/`onload`/`src`/`href`/`onmouseover`) absent; HTML carried only in `raw` string.
- `codec.test.ts:196-207` — explicit `<img src=x onerror="alert(1)">`: bytes survive (`onerror`/`<img`
  present as literal text) yet no `image` node, no `onerror` attr key.

## 5. Tier-1 no regression — PASS

- `parity.test.ts` 14/14 green (unchanged cross-engine `@lezer/markdown` GFM structural parity);
  Tier-1 tree-equality + idempotence 14/14 green. Preserved extensions are additive.
- Runtime probe (transient spec, created→run→removed) on the SUMMARY's over-match edge:
  `a < b then c > d` → stays a **text** node (serializer escapes to `&lt;`/`&gt;`; NOT over-matched
  into a preserved node); `` `$var` `` in code → byte-exact text. No preserved-node leakage — and
  TIER1_CORPUS contains no `$`/`<`, so no fixture is affected regardless.

## P3 (informational) — SUMMARY over-states "prose $ stays plain text"

Runtime probe: `It costs $5 and $10 today.` — the inline-math tokenizer DOES fire, capturing
`$5 and $` as a `preservedInline` atom (types = doc,paragraph,text,**preservedInline**). It still
round-trips **byte-exact** (`byteExact=true`), and no Tier-1 fixture has `$`, so the byte-parity gate
and Tier-1 no-regression are both intact. But the SUMMARY's Tier-1-no-regression note — "A `$` in prose
(`It costs $5 and $10`) … stay plain text" — is inaccurate for the two-`$` case: it becomes a preserved
inline node, not plain text. Harmless to the headless Phase-39 contract (byte-exact, no corruption, no
test regression); flagged only as a latent semantic behavior for the live-editor integration phase
(a prose "$5 … $10" would surface as a math atom). Not a gate failure; GO stands.

## Bottom line

Every Phase-39 acceptance bar is met against code reality: byte-parity is exact `.toBe` equality,
MAJ-04 is byte-exact + inert under node env with zero DOM path, Tier-1 is untouched and green, and no
assertion was skipped or loosened. **PASS.**
