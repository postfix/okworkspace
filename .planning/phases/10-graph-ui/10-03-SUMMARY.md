---
phase: 10-graph-ui
plan: 03
subsystem: frontend-graph
tags: [graph, local-graph, react-force-graph-2d, react-query, zustand, dock, depth-control, obsidian-ui, canvas]

# Dependency graph
requires:
  - phase: 10-graph-ui (plan 01)
    provides: "useLocalGraph(path,depth) hook, graphEdges zustand slice, pure helpers (computeDegrees/isOrphan, filterEdges, neighborHighlight/edgeKey), GraphCanvas ForceGraph2D wrapper, GraphNode/GraphEdge/GraphData types, --graph-node-orphan token"
  - phase: 10-graph-ui (plan 02)
    provides: "EdgeToggles chip cluster (imported + reused), the GraphView draw-callback / activePath-accent / hover-dim reference applied locally"
provides:
  - "web/src/stores/localGraphPanel.ts — persisted open/collapse + depth zustand slice (open=false, depth=1, clamp [1,3], key okf.graph.localPanel) + clampDepth helper"
  - "web/src/components/graph/DepthControl.tsx — the Depth 1/2/3 (default 1) select bound to the slice"
  - "web/src/components/graph/LocalGraphPanel.tsx — the right-side collapsible 'Local graph' dock (canvas + EdgeToggles + DepthControl + states), seed-page accent, hover-highlight, click-to-open"
  - "PageView mount: <LocalGraphPanel path={path}/> additively in the success branch of web/src/routes/PageView.tsx"
affects: []

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "LocalGraphPanel is a thin orchestrator over the 10-01 core (useLocalGraph + filterEdges/computeDegrees/neighborHighlight in useMemo → draw callbacks for the dumb GraphCanvas) — the SAME wiring as GraphView, with the seed (route) path accented; never recomputes adjacency, never refetches on a toggle"
    - "Collapsed-by-default dock gates the fetch: while open=false the hook is called with an empty seed path (useLocalGraph's enabled:path!=='' keeps it idle) so a reader who never opens the panel makes no /graph/local request and pays no canvas cost — and PageView.test stays green with no new api mock"
    - "react-query auto-update via the [path,depth] queryKey: the active-page route param + the DepthControl slice value both flow into useLocalGraph, so the neighborhood reloads on page change OR depth change with no manual invalidation"
    - "Dock chrome is a token-for-token mirror of .agentpanel (fixed --agentpanel-width column, left border, surface header, collapse icon button, max-width:1280px floating-overlay) + a slim fixed reopen tab when collapsed"

key-files:
  created:
    - "web/src/stores/localGraphPanel.ts"
    - "web/src/stores/localGraphPanel.test.ts"
    - "web/src/components/graph/DepthControl.tsx"
    - "web/src/components/graph/DepthControl.css"
    - "web/src/components/graph/DepthControl.test.tsx"
    - "web/src/components/graph/LocalGraphPanel.tsx"
    - "web/src/components/graph/LocalGraphPanel.css"
    - "web/src/components/graph/LocalGraphPanel.test.tsx"
  modified:
    - "web/src/routes/PageView.tsx"

key-decisions:
  - "The local-graph dock is collapsed by default (open=false) — unlike the AgentPanel (which IS the chat surface and defaults open). The local graph is an optional companion, so it stays out of the way until revealed (CONTEXT: collapsible so it doesn't crowd the editor)."
  - "While collapsed the useLocalGraph hook is passed an empty path (queryPath = open ? path : '') so the query stays idle — no /graph/local fetch and no canvas mount until the reader opens the panel. This also keeps PageView.test.tsx green without adding a getLocalGraph mock."
  - "EdgeToggles is imported from 10-02 (it now exists) rather than re-rendered inline — the shared graphEdges slice keeps the global and local views in lock-step (dependency_note honored: import-if-present path taken)."
  - "Empty state fires when the seed payload has zero edges (or zero nodes): a single isolated page node = 'This page has no links yet'."
  - "When collapsed, a slim fixed reopen tab (PanelRightOpen, aria 'Show local graph') is the way back in — PageView has no topbar entry for this dock (the AppShell Assistant toggle is AgentPanel's), so the tab replaces the topbar affordance while preserving the AgentPanel `if (!open) return null` collapse discipline."

requirements-completed: [GRAPH-03]

# Metrics
duration: ~4min
completed: 2026-06-24
status: complete
---

# Phase 10 Plan 03: Local Graph Panel Summary

**The per-page local graph ships: a right-side collapsible 'Local graph' dock (a token-for-token AgentPanel mirror) that hosts the same `react-force-graph-2d` canvas fed by `useLocalGraph(path, depth)` — the current page (accent-coloured) plus its direct neighbors, auto-updating when the active page route changes, with a 1/2/3-hop Depth control (default 1, clamped), the shared edge-type toggles (GRAPH-04), and hover-neighbor highlighting (GRAPH-05). Collapsed by default so it never crowds the CM6 read surface, with the fetch gated off until revealed; mounted additively in PageView. 21 new unit tests over the slice/depth-clamp, the DepthControl, and the dock chrome/states/collapse/navigation/depth-rekey (full suite 332 → 353 green).**

## Performance
- **Duration:** ~4 min
- **Tasks:** 2 (both `type=auto`)
- **Files created:** 8; modified: 1
- **Tests:** +21 new (8 slice + 3 DepthControl + 10 LocalGraphPanel); full suite 332 → 353, all green
- **No new dependency** (consumes the 10-01 `react-force-graph-2d`; `three` still absent from the lockfile)

## Accomplishments
- **localGraphPanel slice (GRAPH-03):** zustand+persist `{ open, depth, setOpen, toggle, setDepth }` — defaults `open=false` (collapsed) + `depth=1`; `setDepth` clamps to the integer range `[1,3]` via the exported `clampDepth` helper (matching the Phase-9 endpoint clamp); persisted under `okf.graph.localPanel`, mirroring `agentPanel.ts`/`graphEdges.ts` exactly.
- **DepthControl (GRAPH-03):** a labelled `<select className="select">` offering hops 1/2/3 (default 1) bound to the slice; changing it calls `setDepth` (clamped). The value is the single source for the `useLocalGraph` depth arg → changing it re-keys the react-query fetch. Token-only CSS (`.select` primitive + `--hit-min-height`; `Depth` label per the UI-SPEC copy).
- **LocalGraphPanel (GRAPH-03/04/05):** the right-side collapsible dock cloning the `.agentpanel` chrome — header (`Local graph` title + a `PanelRightClose`/`PanelRightOpen` collapse button, aria `Hide local graph`/`Show local graph`), a controls row (EdgeToggles + DepthControl), and a flexing body hosting `GraphCanvas`. Fetches with `useLocalGraph(path, depth)` (keyed `[path, depth]` → auto-updates on page or depth change). Applies the SAME 10-01 wiring as GraphView: `computeDegrees`/`isOrphan`, `filterEdges` over the shared `graphEdges` slice in a `useMemo` (toggles never refetch), `neighborHighlight` on hover (dim the rest). The **seed/current page node is accent-coloured** (`--color-accent`); tag nodes are faint diamonds; orphans dim + outlined — identical to the global view. Page-node click → `navigate('/app/page/<id>')`; tag nodes no-op.
- **States (exact UI-SPEC copy, hidden-Git-safe):** loading `Loading local graph…` (with `.spinner`), error `Couldn't load the local graph. Refresh to try again.` (no infra vocabulary), empty (seed only, no neighbors) `This page has no links yet`.
- **PageView mount:** `<LocalGraphPanel path={path} />` added additively in the success branch immediately after `<BacklinksPanel path={path} />`. The 404/error early-returns and the CM6 read surface are untouched; the dock honors its own persisted collapse state.

## Task Commits
1. **Task 1: localGraphPanel persist slice + DepthControl** — `3a571ef` (feat)
2. **Task 2: LocalGraphPanel dock + PageView mount** — `ed3ac4f` (feat)

Plan metadata (STATE.md + ROADMAP.md + REQUIREMENTS.md) committed separately.

## Deviations from Plan

### Adapted (not literal-plan) decisions
**1. [Adaptation] Fetch gated off while collapsed (empty seed path)**
- **Found during:** Task 2 (avoiding a `getLocalGraph` mock in the existing PageView.test).
- **Detail:** Because the dock defaults collapsed and a collapsed dock should pay no canvas/network cost, the hook is called as `useLocalGraph(open ? path : "", depth)`. The 10-01 hook's `enabled: path !== ""` gate keeps the query idle until the panel is opened. This realizes the plan's "a reader who never opens it pays no canvas cost" intent and keeps `PageView.test.tsx` green without adding a new api mock (the plan offered the mock-update path as optional — not needed).
- **Files:** `web/src/components/graph/LocalGraphPanel.tsx`.

**2. [Adaptation] Slim fixed reopen tab as the collapsed affordance**
- **Detail:** The AgentPanel collapse pattern is `if (!open) return null` plus a topbar `Assistant` toggle owned by AppShell. PageView has no topbar entry for this dock, so when collapsed the panel renders a slim fixed reopen tab (`PanelRightOpen`, aria `Show local graph`) on the right edge — the way back in — rather than rendering nothing (which would strand the reader). This preserves the collapse discipline while keeping the dock reachable from the page itself.
- **Files:** `web/src/components/graph/LocalGraphPanel.tsx` + `.css`.

**3. [Adaptation] Empty = zero-edge payload (not only zero-node)**
- **Detail:** A local neighborhood at depth 1 with no links returns the seed page node and no edges. The empty state therefore fires when `data.edges.length === 0` (or `nodes.length === 0`), so a linkless page reads `This page has no links yet` rather than drawing a single lonely node.

**Total:** 0 auto-fixes + 3 adaptations. No scope creep; no new dependency; App.tsx/AppShell.tsx untouched (zero overlap with 10-02).

## Threat Mitigations Applied
- **T-10-06 (stored XSS):** No `dangerouslySetInnerHTML` in any new file (the single match is a documentary comment in LocalGraphPanel.tsx). Node labels reach the screen ONLY as canvas text drawn by `nodeCanvasObject`; `GraphCanvas` keeps `nodeLabel` empty (no DOM tooltip).
- **T-10-07 (info disclosure):** The local-graph error state renders the generic `Couldn't load the local graph. Refresh to try again.` — zero git/sqlite/bleve/index vocabulary (mirrors Phase 9 + GraphView).
- **T-10-08 (injection — path/depth args):** `path` is URL-encoded by the 10-01 `getLocalGraph` fn; `depth` is an integer clamped client-side to `[1,3]` by `setDepth`/`clampDepth` and re-clamped server-side (Phase 9).
- **T-10-SC (supply chain):** No new dependency added in this plan (handled in 10-01); `three` remains absent from `package-lock.json` (verified 0).

## Self-Check Verification (actual command output)

```
### npx tsc -b ###                                       tsc exit=0
### npx vitest run (full suite) ###                      Test Files 44 passed (44) ; Tests 353 passed (353)
### new tests only ###                                   slice(8) + DepthControl(3) + LocalGraphPanel(10) = 21 passed
### grep -c '"three"' package-lock.json ###              0
### grep -c 'LocalGraphPanel' src/routes/PageView.tsx #  2 (import + mount)
### grep -c 'useLocalGraph' .../LocalGraphPanel.tsx ###  8
### grep -cE '#hex' DepthControl.css / LocalGraphPanel.css #  0 / 0 (token-only)
### dangerouslySetInnerHTML in new files ###             NONE (only a documentary comment)
### CGO_ENABLED=0 go build ./... (repo root) ###         go build exit=0 (embed unaffected)
```

## Human Verification (visual — jsdom cannot assert canvas pixels)
1. Open a page that has links; reveal the 'Local graph' panel (collapsed by default, via the right-edge reopen tab). It shows the current page (accent) + its direct neighbors.
2. Navigate to a different page → the local graph auto-updates to the new page's neighborhood without a manual refresh.
3. Change `Depth` to 2 then 3 → more hops appear; clamps at 3, defaults to 1.
4. Toggle `Shared tags` / `Links` / `Backlinks` → edges show/hide locally, matching the global view (shared `graphEdges` slice).
5. Hover a node → it + neighbors + connecting edges stay bright, the rest dims; click a node → navigates to that page.
6. Collapse the panel → it reclaims editor width; reopen after reload → collapse state persisted.

## Self-Check: PASSED

All 8 created files + 1 modified file exist; both task commits (`3a571ef`, `ed3ac4f`) are present in `git log`.

---
*Phase: 10-graph-ui*
*Completed: 2026-06-24*
