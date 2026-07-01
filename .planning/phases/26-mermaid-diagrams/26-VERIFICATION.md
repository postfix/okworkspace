---
author: qa
responsible: qa
phase: "26"
---

# Phase 26: Mermaid Diagrams (ED-05a) — VERIFICATION

**Plan file:** `.planning/phases/26-mermaid-diagrams/26-01-PLAN.md`
**Context file:** `.planning/phases/26-mermaid-diagrams/26-CONTEXT.md`
**SUMMARY file:** `.planning/phases/26-mermaid-diagrams/26-01-PLAN-SUMMARY.md` (claims checked against code reality, not trusted)
**Requirements:** ED-05a
**Commits verified (on `main`):** `194128d` `cc3752c` `c44416f` `b3acc17` `ecc6ff8`
**Substrate note:** SMTC MCP disconnected for this run — evidence gathered via Bash/Read/grep + live execution (tsc/vitest/build) + `npm ls` dep-provenance. Adequate for a frontend/TS phase. mermaid's real renderer cannot run under jsdom (no `getBBox`); the suite MOCKS `import("mermaid")`, so this verification judges OUR DOM-handling + lazy-import + reveal contract, not mermaid's renderer.

## Verdict

```yaml
gate_result:
  record_author: qa
  id: GR-26-verification
  phase: "26"
  verdict: pass
  basis: >
    All nine must-have truths hold against the real code (read, not trusted from the SUMMARY)
    and full re-run evidence (tsc 0, vitest 762/762 across 61 files, build exit 0 with the
    extended assertLazyChunks active, entry 1,182,740 B < 1,420,000 budget). A ```mermaid fence
    renders as inline SVG on BOTH CM6 surfaces: read mode (PageView wires
    modeCompartment.of(liveExtensions) + cleanRender.of(true), LivePreviewEditor.tsx:156,159) and
    the live editor (liveExtensions, mode.ts:32) — both run codeBlockField, whose mermaid branch
    (livePreview.ts:522-533) emits a MermaidWidget when !rangeActive and NOTHING when the
    selection intersects (active-line reveal); the field recomputes on tr.docChanged || tr.selection
    (livePreview.ts:558). mermaid loads ONLY via a dynamic import("mermaid") (mermaid.ts:55) — grep
    confirms zero static `from "mermaid"` reachable from src; the build assertion forbids mermaid +
    9 mermaid-UNIQUE transitive deps and is proven to have teeth (unit tests + the build's prior
    stylis/d3 leak failures), with mermaid emitted into its own lazy chunks (mermaid.core,
    chunk-KEIR6QF5 662 kB, cytoscape.esm 435 kB, katex 258 kB, per-diagram renderers). initialize
    runs once-guarded under {startOnLoad:false, securityLevel:'strict', theme:'dark',
    suppressErrorRendering:true} (mermaid.ts:24-29,56-59); the strict-sanitized SVG is injected via
    DOMParser('image/svg+xml') + parsererror check + non-<svg>-root check + importNode + appendChild
    (widgets.ts:291-304) — never innerHTML, bindFunctions never called. Any failure is swallowed by
    the whole-pass try/catch leaving the synchronous CodeBlockWidget raw-code fallback; teardown is
    guarded by a destroyed flag + wrapper.isConnected, re-checked post-await. The concurrent-render
    serialization (code-reviewer MAJOR fix) is present: a module-level promise queue chains every
    parse+render pair (mermaid.ts:45,53-69). The change is additive — Phase-25 highlighting
    (highlight.ts) and the GFM parser (markdown.ts) are NOT in the diff; frontend-only (Go/internal/
    cmd diff empty); entry budget unchanged. The bundle-assertion LOOSENING (dropping stylis + d3-*)
    is SOUND — see ruling: both are genuinely pre-existing entry deps via static features
    (react-diff-viewer→@emotion/css→stylis; react-force-graph-2d→force-graph→d3-*), not mermaid
    sneaking in, and every mermaid-unique signal is retained so a mermaid leak cannot hide.
  remediation_owner: null
  evidence:
    - "tsc -b → exit 0"
    - "vitest run → 762 passed / 61 files, 0 fail (mermaid 6, widgets +MermaidWidget 10, livePreview read+live 12, assertLazyChunks +6 mermaid, adversarial 49)"
    - "npm run build → exit 0 with extended assertLazyChunks active; entry index-Cejyk_0L.js = 1,182,740 B < 1,420,000 budget; mermaid in lazy chunks only"
    - "grep: zero static `from \"mermaid\"` in src/ (only dynamic import(\"mermaid\") at mermaid.ts:55)"
    - "npm ls: stylis + d3-selection shared with react-diff-viewer/react-force-graph (pre-existing); dagre-d3-es/cytoscape/dompurify/katex/@mermaid-js/parser/roughjs all mermaid-sole-root"
    - "git diff --stat 194128d^..ecc6ff8 -- '*.go' internal/ cmd/ → empty"
```

## Per-truth verdicts

| # | Must-have truth | Verdict | Key evidence (file:line) |
|---|---|---|---|
| 1 | ```mermaid renders inline SVG in READ + LIVE (a fence → diagram, not CodeBlockWidget/plain) | **PASS** | Branch `lang.toLowerCase()==="mermaid"` → `MermaidWidget` (livePreview.ts:522-531); emitted in read (`clean`→activeLines empty→always) AND live (`!rangeActive`). Read surface wires `cleanRender.of(true)` + `liveExtensions` (LivePreviewEditor.tsx:156,159; PageView.tsx:125). Live via `liveExtensions` (mode.ts:32). Tests: read `livePreview.test.ts:439`; live `:700,734` |
| 2 | mermaid loads ONLY via dynamic `import()`; no static import; assertion has teeth | **PASS** | Dynamic `(await import("mermaid")).default` (mermaid.ts:55); grep finds NO `from "mermaid"` in src. `assertLazyChunks` forbids mermaid + 9 unique deps (vite.config.ts:41-58); teeth: unit tests (assertLazyChunks.test.ts:200-275) + build's prior stylis/d3 leak failures. Build green, mermaid in lazy chunks. See ruling below |
| 3 | `initialize({startOnLoad:false,strict,dark,suppressErrorRendering:true})` once-guarded; SVG via DOMParser + parsererror check + appendChild, never innerHTML; bindFunctions not called | **PASS** | Config `mermaid.ts:24-29`; once-guard `:31,56-59`. Inject `new DOMParser().parseFromString(svg,"image/svg+xml")` + parsererror check + non-`<svg>`-root check + `importNode` + `appendChild` (widgets.ts:291-304); no innerHTML; bindFunctions absent. Tests `mermaid.test.ts:41-57`, `widgets.test.ts:352-375` |
| 4 | Parse/render error → CodeBlockWidget raw-code fallback; never blank/thrown; no leftover temp `<div id="d…">` | **PASS** (residual) | Whole-pass try/catch keeps the sync fallback (widgets.ts:285,309-312); `renderMermaid` rejects on parse/render failure (mermaid.ts:53-69, propagation tested `mermaid.test.ts:79-91`). Fallback tests `widgets.test.ts:377-421`. `suppressErrorRendering:true` (the temp-`<div>` removal) is set + correct, but mermaid-internal so not jsdom-observable — honest residual, prescribed path |
| 5 | Live-edit reveals raw source on selection-intersect, renders otherwise; field recomputes on selection; non-mermaid unchanged; stale guard discards stale svg | **PASS** | Reveal via `rangeActive` (livePreview.ts:505-512,526); recompute on `tr.docChanged \|\| tr.selection` (`:558`). Non-mermaid `else if (clean)` read-only, unchanged (`:534`). Stale guard: destroyed flag + `wrapper.isConnected` re-checked post-await (widgets.ts:288,301). Tests `livePreview.test.ts:700-742` (a-e + atomic); teardown `widgets.test.ts:423-453` |
| 6 | Async-widget pattern (sync fallback → async load → atomic detached swap, destroyed+isConnected guard, whole-pass try/catch; eq on (code,theme)); concurrent-render serialization present | **PASS** | Sync fallback first (widgets.ts:272-278); guarded atomic swap (`:288,301-304`); whole-pass try/catch (`:285,309`); `eq` on (code,theme) (`:265-267`). Serialization: module-level promise `queue` chains each parse+render, survives rejection (mermaid.ts:45,53-69). Test `widgets.test.ts:455` |
| 7 | Additive: `buildCodeBlockDecorations` branches mermaid → MermaidWidget else CodeBlockWidget; non-mermaid + Phase-25 highlighting + GFM parser untouched | **PASS** | Branch is additive (livePreview.ts:522-542); non-mermaid path identical to Phase 25. `highlight.ts` and `markdown.ts` NOT in `git diff --stat 194128d^..ecc6ff8`. `widgets.test.ts:281` (```go still CodeBlockWidget), `:451` |
| 8 | Frontend-only; entry budget NOT raised; dark now, light-swap-ready | **PASS** | `git diff --stat … -- '*.go' internal/ cmd/` empty. `maxEntryBytes: 1_420_000` unchanged (vite.config.ts:71); entry 1,182,740 B (+~2 kB widget code vs Phase-25 1,180,610). Theme locked dark via once-guard, light = value swap (mermaid.ts:18-23; `eq` keys theme) |
| 9 | Tests mock `import("mermaid")`; assert DOM handling + lazy-import + assertion's mermaid coverage | **PASS** | `vi.mock("mermaid")` (mermaid.test.ts:23); DOM handling (widgets.test.ts:331-453); no-static-import `?raw` assertion (mermaid.test.ts:93-100); assertion mermaid teeth (assertLazyChunks.test.ts:199-276) |

## Build / test evidence (re-run by qa)

```
# from web/
$ npx tsc -b            → exit 0 (clean)
$ npx vitest run        → 61 files passed, 762 tests passed, 0 fail
                          (mermaid.test 6, widgets +MermaidWidget 10, livePreview read+live 12,
                           assertLazyChunks +6 mermaid teeth, mermaid.adversarial 49)
$ npm run build         → exit 0 WITH extended assertLazyChunks active
                          entry index-Cejyk_0L.js = 1,182.74 kB (1,182,740 B) < 1,420,000 budget
                          mermaid LAZY: mermaid.core (35 kB), chunk-KEIR6QF5 (662 kB),
                          cytoscape.esm (435 kB), katex (258 kB), sequenceDiagram/
                          architectureDiagram/ganttDiagram/… per-diagram renderers — all NON-entry
                          4 INEFFECTIVE_DYNAMIC_IMPORT warnings = the expected Phase-25
                          markdown→html→css/js core chain (warnings, not errors)
$ git diff --stat 194128d^..ecc6ff8 -- '*.go' internal/ cmd/ → empty (frontend-only)
```

The forbidden-regex arm is proven e2e: the SUMMARY records (and the dep-provenance confirms why) the first two build runs failed on `stylis/src/Enum.js` then `d3-selection/src/namespaces.js` entry leaks — direct evidence the plugin executes and catches entry leaks. The mermaid-specific regexes are proven to fire by the unit teeth tests.

## Bundle-loosening ruling (truth #2 — the critical judgment call)

**RULING: the loosening is SOUND. mermaid + its unique heavy deps are proven lazy; the dropped deps (`stylis`, `d3-*`) are genuinely pre-existing entry deps via OTHER static features. No hole. PASS truth #2.**

The engineer dropped `stylis` and `d3-*` from the `assertLazyChunks` `forbidden` list (the research_facts recommended them) and kept only mermaid-UNIQUE signals. I ruled independently against ED-05a's "mermaid never enters the main bundle":

- **(a) `stylis` / `d3-*` are GENUINELY pre-existing entry deps via static features — NOT mermaid sneaking in.** `npm ls stylis` shows two roots: `mermaid@11.16.0 → stylis@4.4.0` AND `react-diff-viewer-continued@4.2.2 → @emotion/css → stylis@4.2.0`. `npm ls d3-selection` shows `mermaid → d3@7.9.0 / @upsetjs/venn.js` AND `react-force-graph-2d@1.29.1 → force-graph → d3-selection`. `react-diff-viewer-continued` and `react-force-graph-2d` are STATICALLY imported app features, so stylis + d3-* ride the entry chunk independently of mermaid. Forbidding them would fail the build on a pre-existing, non-mermaid fact (exactly the Phase-25 lang-css precedent). Dropping them is correct.
- **(b) mermaid CORE + every UNIQUE heavy dep ARE forbidden AND proven absent from entry.** `npm ls` confirms `dagre-d3-es`, `cytoscape`, `dompurify`, `katex`, `@mermaid-js/parser`, `roughjs` are all mermaid-SOLE-ROOT — they exist only because mermaid pulls them. All are in `MERMAID_FORBIDDEN` (vite.config.ts:41-52), as is `node_modules/mermaid/` itself. The build is green with the assertion active and mermaid lands in its own lazy chunks → proven absent from entry. The assertion retains teeth (unit tests assert each leak throws; the build's prior leak failures confirm live execution).
- **(c) No phase-induced entry bloat.** Entry = 1,182,740 B vs Phase-25's 1,180,610 B — the ~2 kB delta is the new mermaid.ts/widget/livePreview code, NOT mermaid (which is wholly in lazy chunks). Budget unchanged.

**Why there is no hole:** a stray static `import "mermaid"` drags `node_modules/mermaid/*` into entry, caught directly by `/node_modules[\\/]mermaid[\\/]/` — the PRIMARY signal, independent of the dropped deps. It would additionally drag 9 unique transitive deps, all still forbidden. The dropped `stylis`/`d3-*` were merely redundant signals that also happen to be shared; removing them weakens nothing about detecting mermaid itself. The loosening narrows two pre-existing core facts out of the forbidden set without weakening the guard on mermaid or any unique dep. Correct engineering, not scope-cheating.

## Gaps routed UP

**None.** All nine truths PASS; the bundle-loosening judgment clears. No code change required.

## UAT items (manual sanity checks — not gaps)

1. **Concurrent multi-diagram render serialization** — the code-reviewer MAJOR fix (the module-level promise queue, mermaid.ts:45,53-69) is present and structurally correct, but its effect (serializing parse+render so two blocks' global per-diagram DB singletons don't interleave) is **not reproducible under jsdom** (no `getBBox` → the real renderer can't run). The mock-based tests prove the queue is wired and survives rejection, but not the real-renderer concurrency. **Manual check:** open a page with ≥2 ```mermaid blocks of different diagram types and confirm both render correctly (no cross-contaminated/garbled diagram). Honest residual flagged by the engineer; confirmed not jsdom-observable.

2. **No-live-script guarantee rests on upstream `securityLevel:'strict'` + DOMPurify** — script *execution* / inline-handler *firing* cannot be observed under jsdom (no `runScripts`). The adversarial suite (49 tests) proves the invariants under the jsdom-observable surface (the `__mermaidPwn` execution sentinel stayed unset). **Confirmed this IS the ED-05a-prescribed path, NOT a gap:** CONTEXT D-04 prescribes `securityLevel:'strict'` ("sanitizes/strips scripts") + DOMParser+appendChild (never innerHTML); research_facts confirms strict ⇒ mermaid pipes output through DOMPurify before returning. The widget adds DOMParser injection hygiene as defense-in-depth (a DOMParser-created `<script>` has its "already started" flag set → would not execute on insertion even if one slipped past DOMPurify), and the comment correctly credits DOMPurify as THE sanitizer rather than overclaiming the widget as a second one. **Manual check (optional):** on a real browser, a diagram authored with an embedded `<script>`/`onclick` renders inert.

---

*Verified 2026-06-30 by qa. Goal-backward against commit range `194128d^..ecc6ff8` + code reality; SMTC unavailable, evidence via Bash/Read + live tsc/vitest/build + npm ls dep-provenance.*

---

## UAT EXECUTION ADDENDUM — real-browser discharge of the two carried UAT items

**Date:** 2026-07-01 · **By:** qa · **Trigger:** v1.4-MILESTONE-AUDIT follow-up #3 ("Phase-26 manual UAT ×2 — confirm in a real browser"). The original verification (2026-06-30) was jsdom-only and explicitly deferred these two checks to a real browser; this addendum discharges them.

**Substrate:** live headless Chromium (real JS engine + real mermaid renderer — the surface jsdom cannot observe) against a freshly-rebuilt binary at HEAD `95ca67d`. Frontend rebuilt from source (`npm run build`, exit 0, `assertLazyChunks` active) → embedded → `go build` → `serve`. Fixtures created via the authenticated API; read-mode render inspected in the SPA (`/app/page/*`, the `LivePreviewEditor` clean surface).

### UAT-1 — concurrent multi-diagram render serialization → **PASS**

- Page with 3 **different** diagram types (`graph`/`sequenceDiagram`/`pie`) rendered as 3 distinct SVGs with the **correct** `aria-roledescription` each (`flowchart-v2`, `sequence`, `pie`), sequential ids `mmd-0/1/2` (the queue signature), **zero cross-contamination**, no garble (visually confirmed). No raw-code fallback.
- Stronger stress (beyond the prescribed bar): 4 **same-type** flowcharts with distinct labels (Alpha/Beta/Gamma/Delta). Every rendered SVG contained **exactly one** label family — `mmd-0`=Alpha-only, `mmd-1`=Beta-only. The module-level promise queue (mermaid.ts:45,53-69) holds: no interleaving of the shared per-diagram-type DB singleton.
- Verdict: the serialization fix works; the prescribed criterion (≥2 different-type blocks render correctly, no cross-contamination) is met.

### UAT-2 — embedded `<script>` / inline-handler inertness → **PASS**

Real-JS-engine confirmation that the `securityLevel:'strict'` + DOMPurify + DOMParser-injection chain is inert. Vectors tried, all with the `window.__mermaidPwn` execution sentinel:
- `<script>window.__mermaidPwn=1</script>` in a node id;
- `<img src=x onerror=window.__mermaidPwn=2>` in a label;
- `<b onmouseover=window.__mermaidPwn=8>` in a label **plus a forced `mouseover` dispatched on every SVG descendant**;
- `click Z "javascript:window.__mermaidPwn=3"` interactivity directive.

Result across all: sentinel **UNSET**, `svg script` = 0, `on*` attrs = 0, `javascript:` hrefs = 0. Malicious content **fails closed** at mermaid's strict parser → the inert textContent-only raw-code fallback (CodeBlockWidget, Phase-25-verified); benign diagrams render to clean handler-free SVG. No embedded script or handler ever executes.

### FINDING (NEW — beyond the prescribed UATs; render *completeness*, not correctness/security)

```yaml
finding:
  record_author: qa
  id: F-26-UAT-render-completeness
  phase: "26"
  severity: medium
  status: open
  remediation_owner: architect
  title: Read-mode mermaid blocks outside the initial CM6 parse/viewport window do not render until a doc/selection change
  summary: >
    In read mode (LivePreviewEditor / clean), a `mermaid` fence that lies outside the initial
    synchronous parse+viewport window does NOT render to SVG on load or on scroll — it shows either
    the literal raw fence source (```mermaid / graph TD / ... visible as plain cm-lines, NO decoration)
    or, when decorated-but-off-initial-viewport, stays stuck on the raw-<pre> code fallback. It renders
    only after ANY doc/selection change forces codeBlockField to recompute.
  evidence:
    - "PROVEN root cause: codeBlockField.update recomputes ONLY on `tr.docChanged || tr.selection` (livePreview.ts:557-559); buildCodeBlockDecorations iterates syntaxTree(state) captured at field-create (livePreview.ts:555-556). CM6's Lezer parser only parses a bounded prefix synchronously; blocks past that prefix are absent from the tree → no Decoration emitted. Background parse-completion + viewport scroll emit transactions with neither docChanged nor selection → the field never recomputes."
    - "Repro A (below parse budget, long page): page = heading + mermaid(TOP) + 60 filler paragraphs + mermaid(BOTTOM). At the true bottom after 5s settle: BOTTOM block shows literal `mermaid`/`graph TD`/`BOT[BottomNode]`/``` as plain cm-lines, 0 .cm-md-mermaid widgets, rawFenceVisible=true."
    - "Fix-by-interaction PROVEN: clicking the raw `BottomNode` line (selection change → recompute over the now-complete syntaxTree) flipped it: raw:false, widgets:1, rendered SVG:1."
    - "Repro B (in-budget but below initial viewport, short page): 4 same-type flowcharts — mmd-0/mmd-1 rendered at load; Gamma/Delta first-materialize on gentle scroll as raw-<pre> fallback and stay unrendered at rest (async SVG-swap not completed on scroll-materialized widgets; note MermaidWidget.destroyed (widgets.ts:318) is never reset in toDOM (widgets.ts:269))."
    - "Scope: shared codeBlockField also governs Phase-25 CodeBlockWidget + (mirrored) tableField. In this UAT a below-fold ```go block and a markdown table on the same 60-paragraph layout DID render (they landed inside the parse budget); a code/table block PAST the budget failing is [INFERENCE] from the shared recompute condition, NOT directly reproduced. Mermaid is worst-hit because a diagram is expected where a below-fold ```go still shows readable code."
  impact: >
    Graceful, never unsafe (raw source shown, never blank/garbled/executable) and self-heals on any
    interaction — but on long or multi-diagram pages a reader sees raw `mermaid` source instead of the
    diagram until they click/scroll-edit. Qualifies truth #1 ("renders inline SVG in READ mode"): holds
    for blocks within the initial parse+viewport window; below-window blocks need an interaction. The
    original jsdom verification could not observe this (mocked import, no viewport/parse virtualization).
  routing: >
    Routed UP to architect (per QA ethos — gaps do not go to the engineer). Disposition (small
    gap-closure phase vs backlog) is the architect's call. Likely fix shape: recompute codeBlockField on
    parser progress (e.g. a ViewPlugin watching syntaxTreeAvailable / ensureSyntaxTree, or an update on
    the language parse effect) and/or reset MermaidWidget.destroyed at toDOM. Fix is the engineer's span.
```

### UAT gate

```yaml
gate_result:
  record_author: qa
  id: GR-26-UAT
  phase: "26"
  verdict: pass_with_debt
  basis: >
    Both prescribed carried UAT items PASS in a real browser: concurrent multi-diagram serialization
    renders correctly with zero cross-contamination (UAT-1), and embedded script/inline-handler content
    is inert across every vector incl. a forced mouseover (UAT-2, sentinel UNSET). One new MEDIUM finding
    (F-26-UAT-render-completeness) surfaced beyond the prescribed bar — a read-mode render-completeness
    gap for blocks outside the initial parse/viewport window; graceful and self-healing on interaction,
    routed UP to the architect. This does not refute the original 9-truth pass (correctness/security/
    isolation all held); it qualifies truth #1's read-mode coverage.
  remediation_owner: architect
  evidence:
    - "Real Chromium at HEAD 95ca67d (frontend rebuilt from source, embedded, go build, serve); read-mode SPA render inspected."
    - "UAT-1: 3 different-type SVGs correct+isolated; 4 same-type flowcharts exactly-one-label-family each."
    - "UAT-2: __mermaidPwn UNSET across <script>/img-onerror/b-onmouseover(+forced hover)/click-javascript:; 0 svg script, 0 on* attrs, 0 javascript: hrefs."
    - "Finding F-26-UAT-render-completeness with proven root cause (livePreview.ts:555-559) + fix-by-interaction repro."
```

*UAT executed 2026-07-01 by qa. Real headless Chromium (live JS + real mermaid renderer) against HEAD `95ca67d`; fixtures via authenticated API; read-mode render inspected in the SPA. The two carried Phase-26 UAT items are discharged PASS; one new MEDIUM render-completeness finding routed UP to the architect.*
