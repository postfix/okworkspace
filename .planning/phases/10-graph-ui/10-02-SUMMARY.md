---
phase: 10-graph-ui
plan: 02
subsystem: frontend-graph
tags: [graph, react-force-graph-2d, lazy-route, zustand, react-query, canvas, bundle-discipline, obsidian-ui]

# Dependency graph
requires:
  - phase: 10-graph-ui (plan 01)
    provides: "useGraph() hook, graphEdges zustand slice, pure helpers (computeDegrees/isOrphan, filterEdges, neighborHighlight/edgeKey), GraphCanvas ForceGraph2D wrapper, GraphNode/GraphEdge/GraphData types, --graph-node-orphan token"
provides:
  - "web/src/components/graph/EdgeToggles.tsx — Links/Backlinks/Shared tags chip cluster bound to the graphEdges slice (GRAPH-04; Shared tags default OFF)"
  - "web/src/components/graph/GraphView.tsx — the /app/graph full-pane global graph (header + EdgeToggles + GraphCanvas; degree sizing, orphan distinction, active accent, tag diamonds, hover-dim, click-to-open, empty/loading/error states)"
  - "lazy /app/graph route in web/src/App.tsx (React.lazy + Suspense — canvas lib code-splits out of the initial bundle)"
  - "'Graph' navrow entry in web/src/routes/AppShell.tsx (active styling on /app/graph)"
affects: [10-03-localgraphpanel]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Heavy canvas route is React.lazy + Suspense so react-force-graph-2d code-splits into its own chunk (verified: dist/assets/GraphView-*.js 192KB separate from index bundle)"
    - "GraphView is a thin orchestrator over the 10-01 core: useGraph (server state) + filterEdges/computeDegrees/neighborHighlight in useMemo (client-only) → draw callbacks for the dumb GraphCanvas; never recomputes adjacency, never refetches on a toggle"
    - "Canvas draw callbacks read tokens via getComputedStyle (matching the GraphCanvas convention) so tokens.css drives node/edge colors; latest hover kept in a ref so draw callbacks stay stable"
    - "Edge-toggle chips clone the .agentpanel-suggestion ghost-chip pattern (ON = --color-accent-soft + --color-accent-border) with aria-pressed reflecting the slice"
    - "Canvas/chrome tests under jsdom mock react-force-graph-2d with a DOM stand-in that exposes the click/hover handlers — chrome + navigation asserted, pixels deferred to human verification"

key-files:
  created:
    - "web/src/components/graph/EdgeToggles.tsx"
    - "web/src/components/graph/EdgeToggles.css"
    - "web/src/components/graph/EdgeToggles.test.tsx"
    - "web/src/components/graph/GraphView.tsx"
    - "web/src/components/graph/GraphView.css"
    - "web/src/components/graph/GraphView.test.tsx"
  modified:
    - "web/src/App.tsx"
    - "web/src/routes/AppShell.tsx"
    - "web/src/routes/AppShell.css"

key-decisions:
  - "GraphView takes an optional activePath prop (undefined for the global view) so the SAME component can accent the current page node when reused later; the global /app/graph view passes none — no node is accented there, matching the UI-SPEC (active accent is for the local-graph context)"
  - "Degrees are computed over the VISIBLE (filtered) edges, not the raw payload, so node sizing reflects exactly what is on screen when toggles change"
  - "Node labels render only above a zoom threshold (globalScale > 1.6) OR when in the hover-highlight set, so the first zoomed-out view is not a wall of text (Obsidian feel); labels are canvas text only (stored-XSS guard)"
  - "The Graph nav entry reuses the existing .navrail-trash-row row styling + a new .navrail-row-active modifier (→ --color-tree-active-bg) rather than inventing a new nav treatment"

requirements-completed: [GRAPH-01, GRAPH-02, GRAPH-04, GRAPH-05]

# Metrics
duration: ~6min
completed: 2026-06-24
status: complete
---

# Phase 10 Plan 02: Global Graph View Summary

**The headline Obsidian-style global graph ships: a lazy-loaded `/app/graph` route (the `react-force-graph-2d` canvas code-splits into its own 192KB chunk, out of the initial editor bundle) reachable from a 'Graph' nav entry, rendering all pages as degree-sized nodes with orphan distinction, tag-diamond nodes, client-side edge-type toggle chips (Shared tags OFF by default), hover-neighbor highlighting that dims the rest, and click-a-page-node → open its page — all built thinly on the 10-01 testable core, with 11 new unit tests over the chrome/states/navigation/toggle wiring (full suite 321 → 332 green).**

## Performance
- **Duration:** ~6 min
- **Tasks:** 2 (both `type=auto`)
- **Files created:** 6; modified: 3
- **Tests:** +11 new (5 EdgeToggles + 6 GraphView); full suite 321 → 332, all green
- **No new dependency** (consumes the 10-01 `react-force-graph-2d`)

## Accomplishments
- **EdgeToggles (GRAPH-04):** a three-chip cluster (`Links` / `Backlinks` / `Shared tags`) bound to the `graphEdges` zustand slice. Each chip is an accessible `<button type="button">` with `aria-pressed` reflecting the boolean; clicking calls `toggle(kind)`. Shared tags reflects the slice default **OFF** (success criterion — first view is not a hairball); Links/Backlinks ON. Styling clones `.agentpanel-suggestion` (ON = `--color-accent-soft` + `--color-accent-border`), token-only (grep-verified no hex).
- **GraphView (GRAPH-01/02/05):** full-pane `/app/graph` component — header (title `Graph` + EdgeToggles) over `GraphCanvas`. Fetches the global payload with `useGraph()`; filters edges with `filterEdges` + `useMemo` over the slice (client-only, never refetches); computes `computeDegrees`/`isOrphan` and `neighborHighlight` on hover. Draw callbacks: degree-scaled node radius (min/max bound), orphan = `--graph-node-orphan` fill + `--color-border-strong` outline at min radius, active page (when `activePath` supplied) = `--color-accent`, tag nodes = `--color-faint` diamond (distinct shape, not a rainbow); hover dims non-neighbor nodes/edges to low alpha and brightens connecting edges to `--color-accent-2`. Click a PAGE node → `navigate('/app/page/<id>')`; tag nodes are non-navigable.
- **States (exact UI-SPEC copy, hidden-Git-safe):** loading `Building the graph…` (with `.spinner`), error `Couldn't load the graph. Refresh to try again.` (no infra vocabulary), empty `No pages to graph yet` + the body copy.
- **Lazy route (GRAPH-01 / T-10-05):** `const GraphView = lazy(() => import("./components/graph/GraphView"))` + a `/app/graph` `<Route>` cloning the `/trash` `RequireAuth`→`AppShell`→`Suspense` pattern. Production build confirms a separate `GraphView-*.js` (192KB) chunk — the canvas lib never enters the initial `index` bundle.
- **Nav entry:** a `Graph` navrow in `.navrail-body` beside the Trash row (lucide `Network` icon), `onClick`→`navigate('/app/graph')`, with `.navrail-row-active` (→ `--color-tree-active-bg`) + `aria-current="page"` when on the route.

## Task Commits
1. **Task 1: EdgeToggles chip cluster** — `b2702c8` (feat)
2. **Task 2: GraphView + lazy /app/graph route + Graph nav entry** — `72788fd` (feat)

Plan metadata (STATE.md + ROADMAP.md) committed separately.

## Deviations from Plan

### Auto-fixed Issues
**1. [Rule 3 - Blocking] EdgeToggles test dropped a Node-FS-based CSS read**
- **Found during:** Task 1 verification.
- **Issue:** The first EdgeToggles test asserted token-only CSS by reading `EdgeToggles.css` via `node:fs` + `import.meta.url` → `fileURLToPath`. Under the vitest/jsdom config `@types/node` is not in the test `types` (tsc errors) and `import.meta.url` is not a `file:` scheme (runtime `TypeError`).
- **Fix:** Replaced that case with a DOM accessibility assertion (every chip is a `<button type=button>` carrying `aria-pressed`). Token-only CSS is still guaranteed by construction and grep-verified in the self-check (`grep -cE '#[0-9a-fA-F]{3,6}' EdgeToggles.css` → 0).
- **Files:** `web/src/components/graph/EdgeToggles.test.tsx`.
- **Commit:** `b2702c8`.

### Adapted (not literal-plan) decisions
- **GraphView `activePath` is optional and undefined for the global view.** The plan notes "the active/current page node (if a `path` prop is supplied — for the global view it may be none)". Implemented exactly that: the prop exists for the later local-graph reuse, but `/app/graph` passes none, so no node is accent-coloured in the global view (degree sizing + orphan distinction + tag diamonds carry the visual language there).
- **Degrees computed over filtered (visible) edges**, so toggling Links/Backlinks/Shared tags rescales nodes to match what is drawn (a stronger reading of GRAPH-02 + GRAPH-04 than recomputing over the raw payload).

**Total:** 1 auto-fix (test infra) + 2 adaptations. No scope creep; no new dependency; PageView untouched (zero overlap with the sibling 10-03 plan).

## Threat Mitigations Applied
- **T-10-03 (stored XSS):** No `dangerouslySetInnerHTML` in any new file (grep-verified — the two matches are documentary comments). Node labels reach the screen ONLY as canvas text drawn by `nodeCanvasObject`; `GraphCanvas` keeps `nodeLabel` empty (no DOM tooltip).
- **T-10-04 (info disclosure):** The error state renders the generic `Couldn't load the graph. Refresh to try again.` — zero git/sqlite/bleve/index vocabulary (mirrors Phase 9).
- **T-10-05 (bundle bloat / DoS):** GraphView is `React.lazy`-imported → the production build emits a separate `GraphView-*.js` (192KB) chunk; the canvas library stays out of the initial editor bundle. `three` remains absent from the lockfile (0).
- **T-10-SC (supply chain):** No new dependency added in this plan (handled in 10-01).

## Self-Check Verification (actual command output)

```
### npx tsc -b ###                                tsc exit=0
### npx vitest run (full suite) ###               Test Files 41 passed (41) ; Tests 332 passed (332)
### new tests only ###                            EdgeToggles(5) + GraphView(6) = 11 passed
### grep -c 'lazy(' src/App.tsx ###               1  (GraphView)
### grep -c '"three"' package-lock.json ###       0
### grep -c '/app/graph' src/routes/AppShell.tsx # 3
### npm run build ###                             emits dist/assets/GraphView-*.js (192KB) — separate chunk (code-split confirmed)
### dangerouslySetInnerHTML in new files ###      NONE (only in comments)
### grep -cE '#hex' EdgeToggles.css / GraphView.css #  0 / 0 (token-only)
```

## Human Verification (visual — jsdom cannot assert canvas pixels)
1. Open `/app/graph` from the nav entry: all pages appear as nodes with link edges; pan (drag) + zoom (wheel) feel smooth; the force layout settles then idles.
2. Larger-degree nodes are visibly bigger; orphan pages are dimmer/outlined; labels appear above the zoom threshold and on hover.
3. Toggle `Shared tags` ON → tag diamonds + tag edges appear; OFF on first load. Toggle `Links`/`Backlinks` → edges show/hide without a refetch.
4. Hover a node → it + immediate neighbors + connecting edges stay bright while the rest dims; leaving restores.
5. Click a page node → the app navigates to that page; clicking a tag node does nothing.

## Next Plan Readiness (10-03)
10-03 (LocalGraphPanel dock + PageView mount) consumes the same 10-01 core (`useLocalGraph`, `filterEdges`, `neighborHighlight`, `GraphCanvas`) and can reuse `GraphView`'s `activePath`-accent path / draw-callback approach as a reference. Zero file overlap: this plan touched App.tsx routing + AppShell nav + the new `graph/` GraphView+EdgeToggles only; PageView is untouched.

## Self-Check: PASSED

All 6 created files + 3 modified files exist; both task commits (`b2702c8`, `72788fd`) are present in `git log`.

---
*Phase: 10-graph-ui*
*Completed: 2026-06-24*
