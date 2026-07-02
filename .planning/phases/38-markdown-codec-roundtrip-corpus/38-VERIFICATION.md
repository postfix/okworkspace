---
author: qa
responsible: architect
phase: 38
phase_type: implementation
verdict: PASS
gate: GO-is-honest
requirements: [RE-01]
---

# Phase 38: Markdown Codec + Round-Trip Corpus — Independent Verification

**Verdict: PASS — the GO is HONEST.** The gate genuinely asserts (real tree-equality +
idempotence + cross-engine parity + HTML-inertness), the corpus covers the full bounded slice,
the parity test is a true cross-engine check through `@lezer/markdown`, the security section runs
under a hard-pinned `node` env, and the five TipTap deps are exact-pinned at 3.27.1 with no
`@tiptap/react`. No fixture was skipped, no assertion relaxed, no normalization widened to
manufacture the pass.

Verified against CODE REALITY (files read + tests run), not the SUMMARY. Every line citation below
was re-confirmed with grep.

## Evidence — the gate runs and asserts

- `cd web && npx vitest run --environment node src/lib/tiptap/` → **2 files, 56 tests passed**
  (codec.test.ts 42 + parity.test.ts 14). Re-run confirmed.
- `cd web && npx vitest run src/lib/tiptap/` (DEFAULT env, no `--environment node` flag) →
  **56 passed** — proving the per-file `// @vitest-environment node` docblock overrides the global
  jsdom (codec.test.ts:1, parity.test.ts:1). Under jsdom the no-window guard would fail; it passes.
- `cd web && npx vitest run --environment jsdom src/lib/tiptap/codec.test.ts` → still **42 passed**
  incl. the no-window/no-document guard — the docblock overrides even the CLI `--environment jsdom`
  flag. The node env is hard-pinned per-file, not a default a future config change could silently flip.
- No weakening: grep for `.skip|.todo|.only|xit|toBeTruthy|@ts-ignore|@ts-expect-error|eslint-disable|toBeDefined|.not.toThrow`
  across `web/src/lib/tiptap/` → **No matches**.
- Directory contains exactly the 5 planned files (extensions.ts, codec.ts, codec.corpus.ts,
  codec.test.ts, parity.test.ts) — no hidden/extra test files diluting the count.

## RE-01-1 — Headless MarkdownManager codec, shared buildExtensions, import-isolated — PASS

- codec.ts:26-27,34-35,41-42 — `parse`/`serialize`/`roundTrip` built on
  `new MarkdownManager({ extensions: buildExtensions() })`. `grep 'new Editor'` and
  `grep '@tiptap/react'` in the dir → **No matches**. Headless, no DOM.
- extensions.ts:39-52 — `buildExtensions()` returns the bounded slice:
  `StarterKit.configure({ horizontalRule:false, underline:false, dropcursor:false, gapcursor:false,
  trailingNode:false, undoRedo:false })` (:41-48) + `TaskList, TaskItem` (:49-50). Single source of truth.
- Import-isolation: `grep -r "from '.../lib/tiptap" web/src` (excluding tests/self) →
  **No matches**. Nothing in the app entry statically imports the codec — `marked` stays out of the
  entry chunk by construction (RE-04 guard is Phase 42, per scope).

## RE-01-2 — Tree-equality + idempotence over the FULL bounded slice — PASS

- Gate assertions are REAL, per fixture, looped over EVERY corpus entry (codec.test.ts:79):
  - Tree-equality (codec.test.ts:82): `expect(parse(roundTrip(src))).toEqual(parse(src))` (deep JSON).
  - Idempotence (codec.test.ts:86-87): `const once = roundTrip(src); expect(roundTrip(once)).toBe(once)` (`===`).
- Corpus (codec.corpus.ts:22-68) covers EVERY bounded-slice construct — all 14 present, none missing:
  boldItalicStrikeCode (:24), headings 1-6 (:27), bulletList (:30), orderedList (:33),
  nestedMixed ≥2 depth (:36, MIN-09), taskList `- [ ]`/`- [x]` (:39), blockquote (:42),
  blockquoteMixed = quote+list+heading (:45-46, MIN-09), fencedCode (:49),
  mermaidFence kept LITERAL (:53), link (:56), hardBreak two-space (:59), cjkEmoji (:62),
  kitchenSink (:66-67). Both classic pain points (nested lists, blockquote-mixed) are present and
  pass — the gate was not degraded to dodge them.

## RE-01-3 — Cross-engine parity through @lezer/markdown (real, not a self-check) — PASS

- parity.test.ts:24 imports the REAL other engine:
  `import { parser, Table, Strikethrough, TaskList, Autolink } from "@lezer/markdown"`.
- parity.test.ts:33 `gfmParser = parser.configure([Table, Strikethrough, TaskList, Autolink])` —
  mirrors EXACTLY the CM6 GFM set. Cross-checked against `markdown.ts:33-37`
  (`base: commonmarkLanguage, extensions: tagParserExtensions`) and `hashtag.ts` (`tagParserExtensions`
  = the same four GFM exts + app-specific `hashtagExtension`, correctly dropped for standard-Markdown
  parity). Same base (commonmark), same four extensions — no invented GFM set.
- parity.test.ts:59-63 — genuine cross-engine: `sourceStructure = lezerStructure(src)` vs
  `roundTripStructure = lezerStructure(roundTrip(src))`, then `expect(roundTripStructure).toEqual(sourceStructure)`.
  It re-parses the codec's **serialized bytes** (marked@17 output) through a **different** engine
  (@lezer/markdown) and asserts node-type structure equality vs the source. NOT a TipTap
  self-comparison — it independently closes the "both sides through the same parse" concern in RE-01-2.
- Node-type-sequence (vs full-tree) comparison is explicit agent discretion under D-03; document-order
  sequence still captures nesting. Within spec, not a gap.

## RE-01-4 — node env + HTML inertness — PASS

- `// @vitest-environment node` at codec.test.ts:1 and parity.test.ts:1 (grep-confirmed).
  Load-bearing and override-proof (see the jsdom-CLI run above).
- No-DOM guard (codec.test.ts:75-76): `expect(typeof globalThis.window).toBe("undefined")` +
  `...document...` — passes, proving node env; guards a future accidental jsdom switch.
- HTML inertness (codec.test.ts:96-129) over SECURITY_CORPUS (codec.corpus.ts:76-79:
  `<img src=x onerror>`, `<script>`, `<div onclick>`, `<em>`):
  - Escaped to literal text: `expect(out).toContain("&lt;")` (:99) and
    `expect(out).not.toMatch(/<[a-zA-Z/]/)` (:102).
  - No element node / no executable attr in parsed JSON: every node type ∈ `ALLOWED_NODE_TYPES`
    (image/div/script excluded, codec.test.ts:51-65) and none of
    `["onerror","onclick","onload","src","href","onmouseover"]` (:68) present as a live attr key
    (:110). Plus the explicit handler test (:127-128): `types.has("image")===false`,
    `attrKeys.has("onerror")===false`.

## RE-01-5 — deps exact-pinned 3.27.1, no @tiptap/react, audit clean — PASS

- `grep -cE '"@tiptap/(core|pm|starter-kit|extension-list|markdown)": "3.27.1"' web/package.json`
  → **5** (package.json:28-32, no carets). `grep -c '@tiptap/react'` → **0**.
- `@lezer/markdown` present (`^1.6.4`, package.json:22) — no new dep needed for parity.
- `cd web && npm audit --omit=dev` → **found 0 vulnerabilities**.

## Honesty check (the hard bar)

The SUMMARY's GO rests on assertions that genuinely fire. Confirmed independently:
no `.skip`/`.todo`/`toBeTruthy`/`@ts-ignore`; every corpus fixture is looped through both gate
tests (codec.test.ts:79) and the parity test (parity.test.ts:54); the marker normalizations claimed
(`*`/`+`→`-`, `__`→`**`, `_`→`*`) pass BOTH tree-equality AND independent Lezer parity, so they are
structure-preserving (D-02 allowed), not lossiness hidden behind a widened normalization. No fixture
was demoted to Tier-2 or silently dropped. The GO is honest.

## Out of scope (NOT Phase 38 defects — confirmed pre-existing, per assignment)

- `npm run build` / `tsc -b` fails on PRE-EXISTING v1.6 errors (remark-hashtag.ts, MarkdownProse.tsx,
  PageEditor.tsx, widgets.test.ts). Phase 38's own `web/src/lib/tiptap/*` are tsc-clean. Not re-reported.

---

*Verified: 2026-07-02 by Phase38QA (independent, goal-backward, against code reality).*
