---
phase: 10-graph-ui
plan: 01
subsystem: frontend-graph
tags: [graph, react-force-graph-2d, react-query, zustand, pure-helpers, canvas, hidden-git, bundle-discipline]

# Dependency graph
requires:
  - phase: 09-graph-endpoints-backlinks-panel
    provides: "GET /api/v1/graph + /graph/local lean bipartite typed-edge payload ({nodes:[{id,label,type:page|tag}], edges:[{source,target,type:link|tag}]}); tag node ids namespaced tag:<name>; depth clamped [1,3]"
provides:
  - "web/src/api/client.ts — GraphNode/GraphEdge/GraphData types + getGraph()/getLocalGraph(path,depth) GET fns (hidden-Git-safe generic error copy)"
  - "web/src/hooks/useGraph.ts — useGraph() (queryKey [graph,global]) + useLocalGraph(path,depth) (queryKey [graph,local,path,depth], enabled gate on non-empty path)"
  - "web/src/stores/graphEdges.ts — zustand+persist edge-toggle slice (links/backlinks ON, sharedTags OFF default), EdgeKind type, persisted okf.graph.edges"
  - "web/src/lib/graph/model.ts — computeDegrees + isOrphan (GRAPH-02, DOM-free)"
  - "web/src/lib/graph/filter.ts — filterEdges + EdgeToggles type (GRAPH-04, client-only)"
  - "web/src/lib/graph/highlight.ts — neighborHighlight + edgeKey + HighlightSet (GRAPH-05)"
  - "web/src/components/graph/GraphCanvas.tsx — thin prop-driven ForceGraph2D ref-lifecycle wrapper (GraphCanvasData/Node/Link types)"
  - "react-force-graph-2d@1.29.1 (Canvas-only) in web/package.json; three absent from lockfile"
  - "--graph-node-orphan token in web/src/styles/tokens.css"
affects: [10-02-graphview-route, 10-03-localgraphpanel]

# Tech tracking
tech-stack:
  added:
    - "react-force-graph-2d@1.29.1 (the -2d Canvas subpackage ONLY; deps force-graph/prop-types/react-kapsule, peer react:*; NO three.js)"
  patterns:
    - "Graph data plumbing clones the getBacklinks/useBacklinks read seam exactly (GET credentials same-origin, no CSRF, generic non-infra error copy, react-query staleTime 30_000 + enabled gate)"
    - "Edge filtering is client-only zustand+useMemo over the payload edges array — never refetch on a toggle (GRAPH-04)"
    - "Pure DOM-free helper modules (model/filter/highlight) hold all classification/filter/highlight logic so the canvas stays a dumb renderer and the logic is fully unit-tested under jsdom (which cannot render canvas)"
    - "ForceGraph2D imperative instance wrapped with the LivePreviewEditor ref-lifecycle discipline: own the handle in a ref, pauseAnimation + null the ref on unmount (React 19 StrictMode double-mount leak guard)"
    - "Bundle discipline: install the -2d subpackage only; assert three is absent from the lockfile (research pitfall 6)"

key-files:
  created:
    - "web/src/hooks/useGraph.ts"
    - "web/src/hooks/useGraph.test.ts"
    - "web/src/stores/graphEdges.ts"
    - "web/src/stores/graphEdges.test.ts"
    - "web/src/lib/graph/model.ts"
    - "web/src/lib/graph/model.test.ts"
    - "web/src/lib/graph/filter.ts"
    - "web/src/lib/graph/filter.test.ts"
    - "web/src/lib/graph/highlight.ts"
    - "web/src/lib/graph/highlight.test.ts"
    - "web/src/components/graph/GraphCanvas.tsx"
  modified:
    - "web/package.json"
    - "web/package-lock.json"
    - "web/src/styles/tokens.css"
    - "web/src/api/client.ts"

key-decisions:
  - "filterEdges keeps a link edge when EITHER links OR backlinks is on (a single link edge IS the bidirectional page->page relation; both off hides links) — matches the GRAPH-04 contract where Backlinks reveals the reverse-direction view of the same direction-carrying payload, with no separate backlink edge type"
  - "GraphCanvas defaults nodeLabel to '' (no DOM tooltip) so untrusted server labels are only ever drawn as canvas text by the parent's nodeCanvasObject — closes the stored-XSS sink (T-10-01) at the wrapper level rather than relying on each parent"
  - "GraphCanvas auto-sizes via a ResizeObserver guarded for jsdom (ForceGraph2D needs explicit width/height; the lib does not flex) and reads the backdrop from --color-bg via getComputedStyle so tokens drive the canvas"
  - "Single new orphan token only (--graph-node-orphan = --color-text-muted at 45% alpha); tag nodes reuse --color-faint + shape per UI-SPEC (zero-or-one new token target met at one)"

requirements-completed: []

# Metrics
duration: 5min
completed: 2026-06-24
status: complete
---

# Phase 10 Plan 01: Graph UI Testable Core Summary

**The non-visual, fully unit-tested foundation of the Obsidian-style Graph UI: the single Canvas-only `react-force-graph-2d` dependency (three.js provably absent from the lockfile), `getGraph`/`getLocalGraph` api fns + `useGraph`/`useLocalGraph` react-query hooks over the Phase-9 endpoints, a persisted zustand edge-toggle slice (Links/Backlinks ON, Shared tags OFF), three pure helpers (degree+orphan / edge-filter / hover-neighbor) with 26 new tests, and a thin leak-free `GraphCanvas` ForceGraph2D wrapper — everything vitest can assert, front-loaded so the Wave-2 canvas components stay thin.**

## Performance
- **Duration:** ~5 min
- **Tasks:** 3 (Task 1 checkpoint auto-approved; Task 2 dep+token; Task 3 TDD RED→GREEN)
- **Files created:** 11; modified: 4
- **Tests:** +26 new (full suite 295→321, all green)

## Accomplishments
- **New dependency (Canvas-only):** `react-force-graph-2d@1.29.1` installed in `web/` — deps `force-graph`/`prop-types`/`react-kapsule`, peer `react:*`. HARD-VERIFIED `three` is absent from `package-lock.json` (`grep -c '"three"'` → 0, case-insensitive substring also 0); umbrella `react-force-graph` not present.
- **Orphan token:** `--graph-node-orphan: rgba(141, 150, 168, 0.45)` added to `tokens.css` (the existing `--color-text-muted` #8d96a8 at ~45% alpha — on-palette alpha variant, not a new hue). Only one new token; tag nodes reuse `--color-faint` + shape.
- **api fns + types:** `GraphNode`/`GraphEdge`/`GraphData` + `getGraph()` (GET `/api/v1/graph`) and `getLocalGraph(path, depth)` (GET `/api/v1/graph/local?path=&depth=`) in `client.ts`, cloning the `getBacklinks` GET shape (credentials same-origin, no CSRF) with generic hidden-Git-safe error copy ("Couldn't load the graph." / "Couldn't load the local graph.").
- **hooks:** `useGraph()` (queryKey `["graph","global"]`, staleTime 30_000) + `useLocalGraph(path, depth)` (queryKey `["graph","local",path,depth]` so it auto-refetches when path OR depth changes; `enabled: path !== ""`), mirroring `useBacklinks`.
- **zustand slice:** `useGraphEdges` (`graphEdges.ts`) — `{links:true, backlinks:true, sharedTags:false, toggle(kind)}`, zustand+persist under `okf.graph.edges`, mirroring `agentPanel.ts`.
- **pure helpers:** `model.ts` (`computeDegrees`, `isOrphan`), `filter.ts` (`filterEdges` + `EdgeToggles`), `highlight.ts` (`neighborHighlight` + `edgeKey` + `HighlightSet`) — all DOM-free, exercised by 26 unit tests covering the exact behavior-block cases.
- **canvas wrapper:** `GraphCanvas.tsx` — thin prop-driven `ForceGraph2D` host owning the imperative ref with the LivePreviewEditor discipline (auto-size via guarded ResizeObserver, `getComputedStyle` backdrop, `pauseAnimation()` + null-ref cleanup on unmount). No classification/filter/highlight inside the canvas; labels drawn as canvas text only.

## Task Commits
1. **Task 1 (checkpoint:human-verify):** auto-approved per orchestrator autonomous directive (research-LOCKED dependency) — no commit (verification gate).
2. **Task 2: install dep + orphan token + lockfile guard** — `bb69c16` (feat)
3. **Task 3: api fns/hooks + edge-toggle store + 3 pure helpers + GraphCanvas (TDD)** — `4a2f589` (feat)

Plan metadata (STATE.md + ROADMAP.md) committed separately.

## Files Created/Modified
- `web/package.json` / `web/package-lock.json` — react-force-graph-2d@^1.29.1 (no three)
- `web/src/styles/tokens.css` — `--graph-node-orphan`
- `web/src/api/client.ts` — GraphNode/GraphEdge/GraphData + getGraph/getLocalGraph
- `web/src/hooks/useGraph.ts` (+ `.test.ts`) — useGraph/useLocalGraph
- `web/src/stores/graphEdges.ts` (+ `.test.ts`) — edge-toggle slice
- `web/src/lib/graph/model.ts` (+ `.test.ts`) — degree + orphan
- `web/src/lib/graph/filter.ts` (+ `.test.ts`) — edge filter
- `web/src/lib/graph/highlight.ts` (+ `.test.ts`) — hover-neighbor set
- `web/src/components/graph/GraphCanvas.tsx` — ForceGraph2D wrapper

## Deviations from Plan

### Auto-approved checkpoint (autonomous mode)
**Task 1 (package-legitimacy gate)** — The plan defines Task 1 as a `checkpoint:human-verify` (`gate="blocking-human"`). The orchestrator ran in AUTONOMOUS mode and explicitly directed auto-approval of this gate because `react-force-graph-2d` is the research-LOCKED v1.0 STACK choice. Mitigation preserved: after install the lockfile `three`-absence assertion (`grep -c '"three"' package-lock.json` → 0) was HARD-VERIFIED before proceeding — the supply-chain mitigation (T-10-SC) is intact regardless of the human gate being auto-approved. Installed package verified: `react-force-graph-2d@1.29.1`, deps `force-graph`/`prop-types`/`react-kapsule`, peer `react:*`, no three.js.

### Adapted (not literal-plan) decisions
**1. [Adaptation] `filterEdges` link/backlinks semantics**
- **Found during:** Task 3 (writing the filter behavior).
- **Detail:** The Phase-9 payload carries direction on a single `link` edge type (no separate backlink type). Rather than synthesizing reverse edges, `filterEdges` keeps a `link` edge when EITHER `links` OR `backlinks` is on (both off ⇒ links hidden), which satisfies the GRAPH-04 contract ("Backlinks reveals the reverse-direction view") against the real direction-carrying payload. The Wave-2 canvas can still style forward vs. reverse direction per-edge using the payload direction; the toggle simply gates visibility.
- **Files:** `web/src/lib/graph/filter.ts`.

**2. [Adaptation] Hook tests use `@testing-library/react` `renderHook` + a `QueryClientProvider`**
- The repo had no prior react-query hook test; `@testing-library/react` (already a dep) provides `renderHook`. Tests mock `getGraph`/`getLocalGraph` via `vi.spyOn` and assert the query wiring (queryKey carries path+depth → refetch; empty-path `enabled` gate keeps `fetchStatus` idle) rather than the canvas, matching the behavior block.

**3. [Adaptation] `GraphCanvas` auto-sizes via a guarded ResizeObserver**
- `ForceGraph2D` requires explicit width/height. There was no existing canvas-sizing precedent, so the wrapper measures its host with a ResizeObserver (guarded for jsdom where it is undefined) and feeds measured pixels. Not unit-tested (jsdom cannot render canvas, per the plan); its correctness is deferred to the 10-02/10-03 human verification where the canvas actually mounts.

**Total:** 1 auto-approved checkpoint (mitigation preserved) + 3 adaptations. No scope creep; the ONLY new dependency is `react-force-graph-2d`.

## Threat Mitigations Applied
- **T-10-01 (stored XSS):** No `dangerouslySetInnerHTML` anywhere in the new files (grep-verified — only documentary comments mention it). `GraphCanvas` defaults `nodeLabel` to `''` (no DOM tooltip); labels reach the screen only as canvas text drawn by the parent's `nodeCanvasObject`.
- **T-10-02 (info disclosure / hidden-Git):** `getGraph`/`getLocalGraph` throw generic copy ("Couldn't load the graph." / "Couldn't load the local graph.") with zero git/sqlite/bleve/index vocabulary, mirroring the Phase-9 BacklinksPanel contract.
- **T-10-SC (supply chain):** package-legitimacy gate (auto-approved, research-vetted) + the lockfile `three`-absence assertion proves the umbrella/three.js path was not pulled.

## Self-Check Verification (actual command output)

```
### npx tsc -b ###                          tsc exit=0
### npx vitest run (full suite) ###         Test Files 39 passed (39) ; Tests 321 passed (321)
### new tests only ###                      model(7) filter(5) highlight(5) graphEdges(5) useGraph(4) = 26 passed
### grep -c '"three"' package-lock.json ###  0
### grep -c '"react-force-graph-2d"' package.json ###  1
### grep -c -- '--graph-node-orphan' tokens.css ###    1
### CGO_ENABLED=0 go build ./... ###        go build exit=0 (embed unaffected)
### dangerouslySetInnerHTML in new files ###  NONE (only in comments)
### GraphCanvas import ###                  from "react-force-graph-2d" (not umbrella)
```

## Next Plan Readiness (10-02 + 10-03)
The Wave-2 UI plans consume:
- `useGraph()` → feed `data` (map `{nodes, edges}` → `{nodes, links}`) into `GraphCanvas`; `useLocalGraph(activePath, depth)` for the right-side panel (auto-refetches on path/depth change).
- `useGraphEdges()` + `filterEdges(edges, toggles)` (useMemo) before mapping to lib `links`; the toggle chip cluster drives `toggle(kind)`.
- `computeDegrees` → node radius; `isOrphan` → `--graph-node-orphan` fill; `neighborHighlight(hoverId, edges)` → dim non-members. Build `nodeCanvasObject`/`linkColor`/`linkWidth` from these and pass to `GraphCanvas`.
- `GraphCanvas` props: `data`, `onNodeClick` (→ `navigate('/app/page/<id>')`, skip `tag:` ids), `onNodeHover` (→ set hover focus), the draw callbacks, and force-tuning overrides.
- Lazy-load the GraphView route (`React.lazy` + Suspense) so the canvas lib code-splits out of the initial editor bundle.

## Self-Check: PASSED

All 11 created files + 4 modified files exist; both task commits (`bb69c16`, `4a2f589`) are present in `git log`.

---
*Phase: 10-graph-ui*
*Completed: 2026-06-24*
