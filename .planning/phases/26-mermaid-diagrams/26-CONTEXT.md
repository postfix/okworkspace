---
author: architect
---

# Phase 26: Mermaid Diagrams — Context

**Gathered:** 2026-06-30
**Status:** Ready for planning

<domain>
## Phase Boundary

` ```mermaid ` fenced blocks render as inline SVG diagrams on BOTH CM6 surfaces (read mode
AND the live editor), via a lazy-loaded, sandboxed `MermaidWidget`, with a safe raw-code
fallback on any parse/render error. `mermaid` is build-asserted OUT of the main editor chunk
(it is large) by reusing Phase 25's `assertLazyChunks` plugin. **Frontend-only** (`web/`); no
backend change. This is the milestone's **heaviest/riskiest** phase. Requirement: **ED-05a**.
Depends on Phase 25 (shares `widgets.ts` + `livePreview.ts`; reuses the async-widget pattern
and the lazy-chunk assertion).

**In scope (ED-05a):**
- A `MermaidWidget` (block replace decoration from the fenced-code `StateField`, branching on
  the fence info string `=== "mermaid"`) rendering flowchart / sequence / class / state / ER at
  minimum.
- Lazy `import()` of `mermaid` (its own async chunk); an **automated build assertion** (reusing
  `assertLazyChunks` with `/mermaid/` added to `forbidden`) proves `mermaid` is absent from the
  entry chunk — not eyeballed.
- Render with `mermaid.initialize({ startOnLoad:false, securityLevel:'strict', theme:'dark' })`;
  SVG injected via **DOMParser + appendChild** — NEVER `innerHTML` of page content.
- A parse/render error → fall back to the existing `CodeBlockWidget` (raw fenced code) — never
  blank, never a crash.
- Light/dark theme honored (dark now); live-preview re-render **debounced** (not per-keystroke).

**Out of scope / unchanged:**
- Diagram-authoring helpers / toolbar / a live source split-pane; exporting or copying the SVG.
- Non-mermaid diagram languages.
- Raising the entry-chunk budget — `mermaid` MUST stay lazy (the budget set in Phase 25 holds).
- Any SSR / CDN mermaid load (client-only render, per ED-05a security).
- Syntax-highlighting a `mermaid` fence (Phase 25 already routes `mermaid` to plain — now it
  becomes a diagram instead).
</domain>

<decisions>
## Implementation Decisions

### Rendering surfaces (D-01, D-02)
- **D-01 — Branch the fenced-code decoration on `lang === "mermaid"` → `MermaidWidget`.** Where
  `buildCodeBlockDecorations` (livePreview.ts) currently always builds a `CodeBlockWidget` for a
  `FencedCode` node, branch: `lang === "mermaid"` → `MermaidWidget`, else → `CodeBlockWidget`
  (unchanged). It stays a `block: true` replace decoration from the `StateField` (CM6 forbids
  block decorations from a ViewPlugin).
- **D-02 — Read + live (Obsidian-style) — DECIDED with co-driver (resolves the ED-05a internal
  inconsistency).** The diagram renders in read mode AND in the live editor. Today
  `buildCodeBlockDecorations` is gated entirely on the `cleanRender` facet (read-mode only); this
  phase must ALSO emit the `MermaidWidget` in edit mode, EXCEPT when the cursor/selection
  intersects the block — then reveal the raw ```mermaid source for editing (active-line reveal,
  mirroring how the editor already reveals raw markup on the active construct). The live re-render
  is debounced (D-08). NOTE: this is the load-bearing new complexity vs Phase 25's read-only
  `CodeBlockWidget`; the planner must work out the live-mode emission + reveal gate + `atomicRanges`
  interaction carefully (this is why the phase is the riskiest).

### Lazy load + bundle assertion (D-03)
- **D-03 — `mermaid` lazy via `import()`; reuse `assertLazyChunks`; do NOT grow the entry budget.**
  Load `mermaid` only when a mermaid block is first displayed (dynamic `import()` → its own chunk).
  Reuse `web/vite-plugins/assertLazyChunks.ts` (Phase 25): add `/node_modules\/mermaid\//` to
  `forbidden`, and verify mermaid's heavy transitive deps (d3 / dagre / cytoscape etc.) do not leak
  into the entry chunk either (extend `forbidden` or confirm they ride mermaid's lazy chunk). The
  build assertion (run in `npm run build`) FAILS if `mermaid` reaches entry. The entry
  `maxEntryBytes` budget from Phase 25 stays — mermaid must not inflate it.

### Render, security, fallback (D-04, D-05)
- **D-04 — Strict sandbox + DOMParser SVG injection.** After the lazy import:
  `mermaid.initialize({ startOnLoad:false, securityLevel:'strict', theme:'dark' })` (once / guarded).
  Validate with `mermaid.parse(src)` (or catch `render`'s throw), then `mermaid.render(uniqueId, src)`
  → SVG string → parse with `new DOMParser().parseFromString(svg, "image/svg+xml")` and `appendChild`
  the `<svg>` node. NEVER assign `innerHTML` of page-derived content. `securityLevel:'strict'`
  sanitizes/strips scripts. Diagram source is untrusted page content. Use a collision-free render id.
- **D-05 — Error → raw-code fallback (the existing `CodeBlockWidget`).** A parse or render failure
  (mermaid throws / invalid diagram) → render the raw fenced code via the existing `CodeBlockWidget`
  DOM (textContent) — never a blank widget, never a thrown error escaping `toDOM`. Wrap parse+render
  in try/catch; on failure fall back.

### Patterns reused from Phase 25 (D-06, D-07, D-08)
- **D-06 — Async-widget pattern (Phase 25).** `MermaidWidget` mirrors the proven pattern: `toDOM`
  builds a synchronous fallback DOM FIRST (the raw fenced code, so a slow/failed load shows the
  source — never blank), then async-loads + renders mermaid and swaps via an **atomic
  detached-fragment** build, guarded by a per-instance `destroyed` flag (set in `destroy()`) +
  `dom.isConnected`, inside a whole-pass `try/catch`. `eq()` keyed on `(code, theme)` so unchanged
  diagrams are not re-rendered.
- **D-07 — Theme honored; dark now, light-swap-ready.** App is dark-only → `theme:'dark'`. Structure
  the theme choice so a future light theme passes a light mermaid theme (consistent with Phase 25's
  light-swap-ready `--syntax-*` decision). The diagram should sit on the page/code background tokens.
- **D-08 — Debounced live re-render.** In the live editor, re-render the diagram only after edits
  settle (debounce), NOT per keystroke; `eq()` + the debounce prevent mermaid.render thrash. Read
  mode renders once per widget build.

### Coverage + coordination (D-09, D-10)
- **D-09 — Diagram types.** flowchart, sequence, class, state, ER at minimum (ED-05a / ROADMAP).
- **D-10 — Shared-file coordination.** Extends `web/src/lib/cm/widgets.ts` (NEW `MermaidWidget`;
  `CodeBlockWidget` reused as the fallback) and `web/src/lib/cm/livePreview.ts` (the fenced-code
  decoration branch + the new live-mode emission/reveal). Same files Phase 25 touched; additive.

### Claude's Discretion
- The debounce interval; the exact live-mode reveal gate (selection-intersects-block test) and how
  it composes with `atomicRanges`; the render-id scheme; whether to pre-validate with `mermaid.parse`
  vs rely on `render`'s throw; the precise dark-theme variables passed to mermaid.
</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Requirements / roadmap
- `.planning/REQUIREMENTS.md` §ED-05a — the locked acceptance criteria + security.
- `.planning/ROADMAP.md` §"Phase 26" — goal + the four observable success criteria; §"Phase 25"
  for the shared-pattern lineage.

### Code to extend / reuse
- `web/src/lib/cm/livePreview.ts` — `buildCodeBlockDecorations` (≈480-492: the `FencedCode` →
  `CodeBlockWidget` mount to branch), the `codeBlockField` `StateField`, the `cleanRender` facet
  + `atomicRanges` provide (≈460-475), and `parseFenced`. The live-mode emission + active-line
  reveal is the new work here.
- `web/src/lib/cm/widgets.ts` — `CodeBlockWidget` (line 101: the fallback + the widget shape to
  mirror); add `MermaidWidget`. The no-`innerHTML`/`textContent` invariant (T-06-08).
- `web/vite-plugins/assertLazyChunks.ts` + `web/vite.config.ts` — the Phase-25 build assertion to
  reuse (add `/mermaid/` to `forbidden`).
- `web/src/lib/cm/highlight.ts` / `web/src/lib/cm/widgets.ts` (Phase 25) — the async-widget pattern
  (plain-first → async load → atomic swap → destroyed/isConnected guard) to copy.
- `web/src/lib/cm/theme.ts`, `web/src/styles/tokens.css` — dark theme tokens; the diagram surface.

### Prior-phase invariants (context)
- `.smtc/memories/qa-m1.4-phase25-verified.md` — the `assertLazyChunks` params + the async-widget
  pattern + the entry-budget, all reused here.
</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `assertLazyChunks` vite plugin (Phase 25) — add `/mermaid/` to `forbidden`; this is the whole
  build-assertion criterion, already built.
- The async-widget pattern (Phase 25 `CodeBlockWidget`/`highlight.ts`) — directly reusable for
  `MermaidWidget`.
- `CodeBlockWidget` — IS the raw-code fallback (D-05); no new fallback DOM needed.
- `parseFenced` + the `codeBlockField`/`cleanRender`/`atomicRanges` machinery in livePreview.ts.

### Established Patterns
- Block replace decorations come from a `StateField` (CM6 forbids them in a ViewPlugin).
- No-`innerHTML` of page content (use DOMParser + appendChild for the SVG; textContent for fallback).
- Active-line reveal (reveal raw source on the edited construct) — extend to the mermaid block.
- Lazy chunk + automated absence assertion (Phase 25).
- Tokens-only theming; dark now, structured for a future light swap.

### Integration Points
- `livePreview.ts` `buildCodeBlockDecorations` — the branch point.
- `vite.config.ts` `assertLazyChunks({ forbidden, maxEntryBytes })` — add the mermaid rule.
- `package.json` — add `mermaid` as a dependency (loaded only via dynamic import).

### Risk Notes (heaviest/riskiest phase)
- Live-mode block widget + `atomicRanges` + active-line reveal + async render + debounce all
  interacting — the new complexity beyond Phase 25's read-only widget.
- mermaid's large transitive dep graph (d3/dagre/…) leaking into entry — the assertion must cover it.
- mermaid global `initialize` state / render-id collisions / SVG sizing causing layout shift.
</code_context>

<specifics>
## Specific Ideas
- Obsidian-parity: a mermaid block shows the diagram inline while editing, and flips to the raw
  ```mermaid source only when you put the cursor in it.
- A broken diagram should quietly show the code you typed (the existing code-block render), not an
  error box or a blank.
</specifics>

<deferred>
## Deferred Ideas
- Diagram export / copy-as-SVG, a diagram source editor/toolbar, and non-mermaid diagram languages.
- A real light theme (only the dark theme + the light-swap structure are in scope).
</deferred>

---

*Phase: 26-mermaid-diagrams*
*Context gathered: 2026-06-30*
