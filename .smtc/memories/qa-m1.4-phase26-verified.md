# qa — Phase 26 Mermaid Diagrams (ED-05a) verified — PASS

**Milestone:** m1.4 · **Verdict:** pass (all 9 truths) · **Commits:** 194128d cc3752c c44416f b3acc17 ecc6ff8
**Evidence:** tsc 0; vitest 762/762 (61 files); build exit 0 w/ assertion active, entry 1,182,740 B < 1,420,000; Go diff empty.

## Reusable technique: ruling on a LOOSENED bundle-absence assertion
When an engineer DROPS a recommended forbidden-regex (here `stylis`, `d3-*`) from `assertLazyChunks`,
verify with `npm ls <dep>`: a dep with TWO+ roots (one being a statically-imported app feature) is a
genuine pre-existing entry dep, NOT the lazy lib sneaking in.
- `stylis` ← mermaid AND react-diff-viewer-continued→@emotion/css.
- `d3-*` ← mermaid AND react-force-graph-2d→force-graph.
- mermaid-UNIQUE (sole-root) deps kept forbidden: dagre-d3-es, cytoscape, dompurify, katex,
  @mermaid-js/parser, roughjs, khroma, marked, dayjs.
No hole because the PRIMARY signal `/node_modules[\\/]mermaid[\\/]/` catches any static `import "mermaid"`
directly — dropped shared deps were merely redundant. Same pattern as Phase-25 lang-css exclusion.

## Read+live widget surface (D-02)
Both surfaces run `codeBlockField` (in `livePreviewExtension`). Read = PageView wires
`modeCompartment.of(liveExtensions) + cleanRender.of(true)` (LivePreviewEditor.tsx:156,159). Live =
`liveExtensions` (mode.ts:32). Mermaid branch (livePreview.ts:522) emits when `!rangeActive`, nothing
when selection intersects (active-line reveal); field recomputes on `tr.docChanged || tr.selection`.
Non-mermaid fences stay `else if (clean)` read-only — additive, Phase-25 untouched.

## jsdom limits (honest residuals → UAT, not gaps)
mermaid renderer can't run under jsdom (no getBBox) → suite MOCKS `import("mermaid")`. Two things only
manually verifiable: (1) concurrent multi-diagram render serialization (the promise-queue fix in
mermaid.ts:45,53-69 is wired+tested but real concurrency unobservable); (2) no-live-script rests on
upstream securityLevel:'strict'+DOMPurify (no runScripts in jsdom). Both are the ED-05a-PRESCRIBED path
(CONTEXT D-04) + DOMParser defense-in-depth — confirmed not gaps.
