---
author: engineer
responsible: architect
phase: 38
phase_type: implementation
status: EXECUTE COMPLETE — GO
verdict: GO
requirements: [RE-01]
---

# Phase 38: Markdown Codec + Round-Trip Corpus (GO/NO-GO gate) - Summary

**Verdict: GO.** The bounded-slice TipTap↔Markdown codec round-trips the full Tier-1 corpus with
tree-equality + idempotence, passes cross-engine parity against the CM6 `@lezer/markdown` GFM config,
and keeps embedded raw HTML inert under a `node` test env. The milestone proceeds to Phase 39.

> **Recovery note (author of record):** the engineer subagent built all artifacts and fixed lint/tsc
> to green, but its task wrapper exited 1 mid-finish (before writing this SUMMARY). The architect
> recovered the receipt after verifying the built state directly on disk (ran every check below). No
> code was authored by the architect — only this SUMMARY + the verification.

## What shipped

- **Deps (exact-pinned, no caret):** `@tiptap/core`, `@tiptap/pm`, `@tiptap/starter-kit`,
  `@tiptap/extension-list`, `@tiptap/markdown` all at **3.27.1**. No `@tiptap/react` (editor UI is
  Phases 40+). `@tiptap/markdown` pulls a second `marked` (17.x) alongside mermaid's 16.x — expected,
  both stay in lazy chunks (Phase 42 wires the guard). `npm audit`: **0 vulnerabilities**.
- **`web/src/lib/tiptap/extensions.ts`** — `buildExtensions()`: StarterKit configured to the bounded
  slice (horizontalRule/underline/dropcursor/gapcursor/trailingNode/undoRedo disabled) + TaskList/TaskItem.
- **`web/src/lib/tiptap/codec.ts`** — headless `parse`/`serialize`/`roundTrip` over `MarkdownManager`
  (no Editor, no DOM). Import-isolated: nothing in the app entry imports it (verified by grep).
- **`web/src/lib/tiptap/codec.corpus.ts`** — `TIER1_CORPUS` (14 fixtures: basic marks, headings 1-6,
  bullet/ordered/nested lists, task lists, blockquote + blockquote-mixed, fenced code + `mermaid`
  literal fence, link, hard break, CJK+emoji, kitchen-sink) + `SECURITY_CORPUS` (4 raw-HTML fixtures).
- **`web/src/lib/tiptap/codec.test.ts`** (`// @vitest-environment node`) — the GATE: per fixture
  tree-equality `parse(roundTrip(src)) ≡ parse(src)` + idempotence; a no-DOM guard
  (`typeof window/document === 'undefined'`); the MAJ-04-b HTML-inertness section (escaped literal
  text, no element node, no executable attr in the parsed JSON).
- **`web/src/lib/tiptap/parity.test.ts`** (`// @vitest-environment node`) — MAJ-02 cross-engine
  parity: re-parses `roundTrip(src)` through `@lezer/markdown` configured to the CM6 GFM set
  `[Table, Strikethrough, TaskList, Autolink]` (mirrors `markdown.ts`) and asserts the Lezer
  node-type sequence matches the source's for every fixture.

## Evidence (all verified on disk by the architect)

- `cd web && npx vitest run --environment node src/lib/tiptap/` → **2 files, 56 tests passed**.
- `cd web && npx vitest run` (full suite) → **68 files, 871 tests passed** (v1.6 close: 66/815;
  delta = exactly the new tiptap tests — zero regression).
- `cd web && npx tsc --noEmit` → exit 0.
- `cd web && npx eslint src/lib/tiptap/` → exit 0.
- `cd web && npm audit --omit=dev` → 0 vulnerabilities.
- Honesty check: no `.skip`/`.todo`/`@ts-ignore`/`toBeTruthy()` weakening; the gate genuinely asserts
  tree-equality + idempotence + parity; the corpus covers every bounded-slice construct incl. the
  MIN-09 pain points (nested lists, blockquote-mixed).

## Observed Tier-1 normalizations (allowed, D-02)

The serializer canonicalizes markers (research-confirmed: `*`/`+`→`-`, `__`→`**`, `_`→`*`, loose
lists tightened). These pass because tree-equality + Lezer-parity both hold — the structure is
identical, only marker bytes change. No supported construct was outright lossy; no NO-GO finding.

## Out of scope / carried forward

- The verbatim preservation tier (tables/math/raw-HTML byte-for-byte) — Phase 39/RE-02.
- `assertLazyChunks` wiring for the TipTap tree + marked dedup — Phase 42/RE-04 (the codec is only
  import-isolated here).
- `@tiptap/react` editor component + host mount — Phases 40-42.

## Independent finding surfaced during this phase (NOT a Phase 38 defect)

`npm run build` (`tsc -b`) is **broken at HEAD** — pre-existing v1.6 code errors in
`remark-hashtag.ts` (Phase 36), `MarkdownProse.tsx`, `PageEditor.tsx` (unused imports), and
`widgets.test.ts`. Confirmed pre-existing: the errors reproduce identically with all Phase 38 changes
removed (tiptap dir moved aside, package.json reverted); the lockfile diff added only TipTap deps (no
`vfile`/`mdast`/`unified` drift). Root cause: v1.6 verified with `tsc --noEmit` (passes), never
`npm run build` (`tsc -b`, stricter tsconfig). This is routed UP to the architect and addressed
separately — it blocks the milestone because Phase 42's `assertLazyChunks` runs inside `vite build`
(gated behind `tsc -b`).
