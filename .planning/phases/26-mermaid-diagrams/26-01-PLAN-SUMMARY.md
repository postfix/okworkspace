---
author: engineer
responsible: engineer
phase: 26-mermaid-diagrams
plan: 01
type: implement
status: complete
requirements: [ED-05a]
---

# Phase 26 — Mermaid Diagrams (ED-05a) — Engineer SUMMARY

**PLAN ACCEPTED** after reviewing all three tasks against the real codebase. Feasibility:
the Phase-25 `CodeBlockWidget` async-widget pattern + `assertLazyChunks` are directly
reusable; the research_facts mermaid@11.16.0 API (dynamic `import().default`, `initialize`
once, `parse`→`render`, `suppressErrorRendering:true`, DOMParser injection) was confirmed
against the installed package. Task-3's reveal (the named refusal point) is a line-for-line
mirror of the already-proven `tableField`/`buildTableDecorations` (recompute on
`docChanged || selection`, `rangeActive` drop) — feasible, not a hack. Proceeded.

## What was built, per task

### Task 1 — lazy loader + MermaidWidget + read-mode wiring + fallback (commit `194128d`)
- `web/src/lib/cm/mermaid.ts` (NEW): `renderMermaid(code)` lazy-loads mermaid via a dynamic
  `import("mermaid").default` ONLY (no static import), `initialize` once (module guard) under
  `{startOnLoad:false, securityLevel:'strict', theme:'dark', suppressErrorRendering:true}`,
  `await parse(code)` then `render("mmd-"+counter++, code)`, returns the svg string, throws on
  failure. (Hardened in `ecc6ff8` — see gates.)
- `web/src/lib/cm/widgets.ts` `MermaidWidget`: synchronous raw-code fallback FIRST (reuses
  `CodeBlockWidget(code,"mermaid").toDOM()`), then async render → `DOMParser`
  `parseFromString(svg,"image/svg+xml")` + parsererror check + non-`<svg>`-root check +
  `importNode` + `setAttribute("style","max-width:100%;height:auto")` + atomic swap. Guarded
  by `destroyed` flag + `wrapper.isConnected` (re-checked post-await); whole pass in try/catch;
  `view?.requestMeasure?.()` after swap; `eq()` on `(code,theme)`. NEVER innerHTML; bindFunctions skipped.
- `web/src/lib/cm/livePreview.ts` `buildCodeBlockDecorations`: branch `lang==="mermaid"` →
  `MermaidWidget` else `CodeBlockWidget` (read mode only here).
- `web/src/components/LivePreviewEditor.css`: `.cm-md-mermaid` dark surface + `min-height` +
  fade-in on `--rendered`.
- Tests (mermaid MOCKED — jsdom has no `getBBox`): `mermaid.test.ts` (6) + `widgets.test.ts`
  (+10 MermaidWidget) + `livePreview.test.ts` (read-mode mermaid→MermaidWidget, go→CodeBlockWidget).

### Task 2 — bundle assertion: mermaid + heavy deps out of every entry chunk (commit `cc3752c`)
- `web/vite.config.ts`: added `MERMAID_FORBIDDEN` (mermaid + mermaid-UNIQUE deps) to the
  existing `assertLazyChunks` `forbidden`; `maxEntryBytes` UNCHANGED.
- `web/vite-plugins/assertLazyChunks.test.ts`: +4 teeth tests (mermaid leak / each unique-dep
  family leak → errors; shared stylis+d3-* NOT flagged; clean lazy placement passes).

**Structural finding (handled, not scope-invented):** research_facts listed `stylis` and
`d3-*` as forbidden, but `npm ls` shows BOTH are SHARED with other statically-imported app
deps — `stylis` via `react-diff-viewer-continued → @emotion/css`, `d3-*` via
`react-force-graph-2d → force-graph`. They legitimately ride the entry chunk INDEPENDENT of
mermaid (the Phase-25 lang-css situation), so forbidding them wrongly failed the build (the
first two build runs reproduced exactly that: leak on `stylis/src/Enum.js`, then
`d3-selection/src/namespaces.js`). I excluded both and kept only the mermaid-UNIQUE deps
(`mermaid`, `dagre-d3-es`, `cytoscape`, `khroma`, `dompurify`, `katex`, `marked`, `dayjs`,
`roughjs`, `@mermaid-js/parser`) — each is a reliable leak signal (a stray static mermaid
import would drag all of them into entry).

### Task 3 — live-mode rendering + active-line reveal + stale-guard (commit `c44416f`)
- `buildCodeBlockDecorations`: emits `MermaidWidget` when read mode (`cleanRender`) OR (edit
  mode AND selection does NOT intersect the block via `rangeActive`); emits NOTHING when the
  selection intersects → reveals raw ```mermaid source. Non-mermaid fences keep their EXACT
  Phase-25 behavior (`else if (clean)` — read-mode `<pre>` only; ```go in edit mode unchanged).
- `codeBlockField.update`: now recomputes on `tr.docChanged || tr.selection` (was docChanged
  only) so cursor in/out toggles reveal⇄render. atomicRanges over the field → a rendered
  diagram is one unit; a revealed block contributes none.
- Tests (+6): edit selection outside→widget, inside→none, toggle out→reappears, ```go in edit
  mode→no widget, read mode always renders, rendered block is atomic + doc byte-verbatim.
- **The concern from the handshake is resolved empirically:** the Task-3 tests mount a REAL
  `EditorView` that RENDERS the mermaid widget in edit mode — the co-extensive ViewPlugin
  `cm-code-block` mark + StateField block-replace overlap does NOT throw in CM6.

## Bundle outcome (criterion 2)
- **Entry chunk = 1,182,740 bytes** (`index-Cejyk_0L.js`) — vs Phase-25's 1,180,610 B; the
  ~2 kB delta is the new mermaid.ts/widget/livePreview code, NOT mermaid itself. Budget
  `maxEntryBytes = 1,420,000` UNCHANGED, well under.
- mermaid landed in its own lazy chunks: core+d3 `chunk-KEIR6QF5` (~662 kB), `cytoscape.esm`
  (435 kB), `katex` (258 kB), and per-diagram renderers (`sequenceDiagram`,
  `architectureDiagram`, `ganttDiagram`, …) — all NON-entry.
- `npm run build` GREEN with the extended assertion active = the automated proof mermaid +
  its heavy deps stayed out of every entry chunk (a leak fails the build, as the stylis/d3
  runs demonstrated).

## Gate outcomes + dispositions

**anvil-code-reviewer** — verdict SHIP (1 MAJOR + 2 MINOR). **anvil-adversarial-tester** —
verdict HOLDS, 49 adversarial tests, ZERO reproduced defects; all four invariants held. The
durable suite landed at `web/src/lib/cm/mermaid.adversarial.test.ts` (commit `b3acc17`).

| Finding | Gate | Severity | Disposition |
|---|---|---|---|
| Concurrent `mermaid.render` not serialized (global per-diagram DB singletons not reentrant; N blocks fire concurrent renders, parse→render gap interleaves) | code-reviewer | MAJOR | **FIXED** (`ecc6ff8`): module-level promise queue serializes parse+render; survives a rejection |
| Case-sensitive `lang === "mermaid"` drops ```Mermaid to plain | code-reviewer | MINOR | **FIXED** (`ecc6ff8`): `lang.toLowerCase() === "mermaid"` (matches CodeBlockWidget) |
| Comment overstates eq()-reuse (every cursor exit re-renders a new instance) | code-reviewer | MINOR | **FIXED** (`ecc6ff8`): comment corrected; "not per-keystroke" still holds |
| Widget comment overstates its independent no-script contribution (real boundary is upstream DOMPurify/strict) | adversarial | LOW | **FIXED** (`ecc6ff8`): comment reworded to credit DOMPurify as the sanitizer + frame the widget as injection hygiene; NOT a second sanitizer (footgun avoided) |
| CDATA-script svg → synchronous `importNode` throw | adversarial | OBSERVATION | **correct-by-design**: swallowed by the whole-pass try/catch → fallback survives; now pinned by a test |
| Teardown/stale guard, DOMParser injection, once-guard/id, reveal gate, vite guard | both | — | **checked clean** (reviewer "earned"; adversarial sentinel `__mermaidPwn` stayed unset across all payloads) |

Honest residual: real script *execution* / inline-handler *firing* can't be observed under
jsdom (no `runScripts`), so "no live script" is proven structurally + via the execution
sentinel, with the production guarantee resting on mermaid's `securityLevel:'strict'`
DOMPurify upstream — accurately reflected in the code comment now.

## Verification evidence (from `web/`)
- `npx tsc -b` → clean (exit 0).
- `npx vitest run` → **762 passed / 61 files**, 0 fail (Phase-25 base 685/59; this phase
  +1 file [mermaid.test.ts], +6 mermaid, +10 widgets, +6 livePreview-read, +6 livePreview-live,
  +5 assertLazyChunks, +49 adversarial). No regression in cm/livePreview/PageView/Phase-25 suites.
- `npm run build` → clean WITH the extended assertion active; entry 1,182,740 B (mermaid lazy).
  The forbidden-regex arm was proven e2e (the stylis then d3-selection leaks failed the build
  before the shared-dep exclusion).
- `internal/web/dist/.gitkeep` restored after each `emptyOutDir` build; no dist artifacts committed.
- Only `web/` code committed; the planning artifacts are left for the architect to commit at close.

## Handoff
```yaml
handoff:
  record_author: engineer
  accountable_owner: engineer
  from: engineer
  to: architect
  artifact: .planning/phases/26-mermaid-diagrams/26-01-PLAN-SUMMARY.md
  intent: "ED-05a mermaid diagrams built + gated; ready for verify"
  status: sent
  commits: [194128d, cc3752c, c44416f, b3acc17, ecc6ff8]
  notes: >
    Frontend-only, no backend change. mermaid renders on BOTH CM6 surfaces (read + live with
    active-line reveal), lazy via dynamic import(), strict-sandboxed, raw-code fallback on any
    failure. The build assertion's forbidden list excludes stylis + d3-* (shared with
    emotion/force-graph, legitimately in entry) and keeps mermaid-unique deps — flag for verify
    so it is not read as a missing rule (it is the Phase-25 lang-css pattern). Concurrent-render
    serialization (MAJOR) is structurally fixed but NOT reproducible under jsdom (no getBBox);
    worth a manual sanity check on a real multi-diagram page during UAT. Adversarial suite
    (b3acc17, 49 tests) proves the security invariants under the jsdom-observable surface; the
    no-live-script production guarantee rests on mermaid securityLevel:'strict' DOMPurify upstream.
```

**EXECUTE COMPLETE.**
