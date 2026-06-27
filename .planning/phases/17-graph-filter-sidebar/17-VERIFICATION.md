---
author: qa
responsible: qa
phase: "17"
plan: "01"
artifact_kind: VERIFICATION
requirements: [GF-01]
---

# Phase 17-01 VERIFICATION: Graph Filter Sidebar

## 1. Acceptance Handshake

| Check | Result | Basis |
|-------|--------|-------|
| Inputs complete | PASS | PLAN, CONTEXT, SUMMARY all present. All file paths referenced in the PLAN exist. |
| PLAN-SUMMARY consistency | PASS | SUMMARY correctly describes what was built: graphFilters store, FilterControls component, filter pipeline in GraphView, filter.ts extensions. Claims match the code reality verified below. |
| Goal verifiability | PASS | All GF-01 criteria are concrete and testable via code review + tsc. |
| Lenses engageable | PASS | Code structure, type checking, and runtime behavior all verifiable. |

**PLAN-SUMMARY consistency check via SMTC:**

- `filterNodes`, `matchesSearch`, `applyOrphanFilter` confirmed present in `web/src/lib/graph/filter.ts` with expected signatures
- `useGraphFilters` confirmed exported from `web/src/stores/graphFilters.ts` with `persist({ name: "okf.graph.filters" })`
- `FilterControls` confirmed as default export from `web/src/components/graph/FilterControls.tsx`
- All declared commits confirmed in git log (`8bbaae4`, `994178d`, `9c5b299`, `6668a20`, `d8ba734`)

**TypeScript compilation:** `npx tsc --noEmit -p web/tsconfig.json` exits 0 (no output).

## 2. Verification Against Must-Haves

### Data model

The backend GraphNode struct (`internal/graph/query.go:37`) returns exactly three fields per node:

```go
type GraphNode struct {
    ID    string `json:"id"`
    Label string `json:"label"`
    Type  string `json:"type"` // "page" | "tag"
}
```

The TypeScript client type matches (`web/src/api/client.ts:258`):
```ts
export interface GraphNode {
  id: string;
  label: string;
  type: "page" | "tag";
}
```

This is a critical constraint: the graph has **no attachment-type nodes**, **no exists field**, and **no aliases or tags fields on individual nodes**. Several UI toggles that depend on these fields are dead code (see Gaps below).

### Must-have-by-must-have

| # | Must-have | Status | Evidence |
|---|-----------|--------|----------|
| 1 | Search filters nodes live by name, path, aliases, and tags | PARTIAL | `matchesSearch` checks `label` (name) and `id` (path). Type-asserted `aliases` and `tags` branches are dead code — backend doesn't supply these fields. Search by name and path works correctly. |
| 2 | Four visibility toggles live flat alongside edge-type chips | PASS | `FilterControls` renders inline in `<header className="graphview-header">` after `<EdgeToggles />` in `GraphView.tsx:361`. No sidebar, no collapsing sections. |
| 3 | Tags toggle (ON) shows/hides tag nodes | PASS | Default `showTags: true`. `filterNodes` checks `n.type === "tag"`. Toggle works on current data. |
| 4 | Attachments toggle (OFF) shows/hides attachment nodes | FAIL | Toggle exists in UI but is dead code. `filterNodes` checks `n.type === "attachment"` — the backend never returns nodes of type `"attachment"`. Toggle has no visible effect. |
| 5 | Existing files only (OFF) hides missing-file nodes | FAIL | Toggle exists in UI but is dead code. `filterNodes` checks `(n as { exists?: boolean }).exists === false` — the backend never sends an `exists` field on graph nodes. Toggle has no visible effect. |
| 6 | Orphans toggle (ON) hides orphan nodes when OFF | PASS | Default `showOrphans: true`. `applyOrphanFilter` correctly drops nodes with zero visible edges when `showOrphans` is false. |
| 7 | Arrows toggle (OFF) controls link direction arrowheads | PASS | `linkDirectionalArrowLength={showArrows ? 4 : 0}` passed to `GraphCanvas`. Works correctly. |
| 8 | Text fade threshold slider (0–1, default 0.5) | PASS | Default stored, slider renders. Mapped to `labelZoomThreshold(v) = 0.5 + v * 4` in `nodeCanvasObject`. |
| 9 | Node size slider (0–1, default 0.2), range 2–px | PASS* | Formula `2 + filterNodeSize * 8` gives 2–10px range. Plan's must_have says 2–12px but the prescriptive action in the plan itself specifies `2 + filterNodeSize * 8` (2–10px). Engineer followed the action. Plan-internal inconsistency. |
| 10 | Link thickness slider (0–1, default 0.2), range 0.5–5px | PASS | Formula `0.5 + filterLinkThickness * 4.5` gives 0.5–5.0px. Default 0.2 maps to 1.4px. |
| 11 | Every control updates immediately — no Apply button | PASS | All filter state is reactive through `useMemo` chain: `nodeFilters` -> `visibleNodes` -> `visibleNodeIds` -> `visibleEdges` -> `orphanFiltered` -> `canvasData`. No imperative update step. |
| 12 | Reset button restores all defaults | PASS | `reset()` action calls `set({ ...DEFAULTS, groups: [] })`. Correctly restores all defaults including clearing groups. |
| 13 | Filter state persisted to localStorage | PASS | `persist({ name: "okf.graph.filters" })` middleware configured on the Zustand store. Key follows the project convention (`okf.graph.edges` precedent). |
| 14 | Filtering pipeline deterministic: node -> edge -> orphan -> display | PASS | Code order: `visibleNodes` (node filter) -> `visibleEdges` (edge filter + visible node ID intersection) -> `orphanFiltered` (orphan filter) -> display settings via `GraphCanvas` props. All pure functions, no side effects. |
| 15 | New group button creates a group with default name and color | PASS | `addGroup()` creates group with `{ id: crypto.randomUUID(), name: "New group", query: "", color: "#8b5cf6", enabled: true }`. Button renders full-width purple. |

### Search verification details

`matchesSearch` (`web/src/lib/graph/filter.ts:49`) checks against:
- `node.label` — page title (works)
- `node.id` — file path like `design.md` (works, effectively covers path search)
- `(node as { aliases?: string[] }).aliases` — **dead code**: GraphNode has no `aliases` field in backend response
- `(node as { tags?: string[] }).tags` — **dead code**: GraphNode has no per-node `tags` field in backend response (tags are modeled as separate tag nodes in the bipartite graph, not as fields on page nodes)

### Pipeline code trace (GraphView.tsx)

```
Line 126-134: nodeFilters useMemo + visibleNodes useMemo   // Node filter
Line 138-149: visibleNodeIds + visibleEdges useMemo         // Edge filter (intersects with visible nodes)
Line 152-155: orphanFiltered useMemo                        // Orphan filter
Line 179-193: canvasData useMemo (maps to lib format)       // Display settings applied via props
Line 392-393: GraphCanvas receives {nodeRelSize, linkDirectionalArrowLength} + linkWidth callback
```

Pipeline order matches the must-have exactly.

## 3. Identified Gaps

### Gap P17-G01: Attachments toggle is non-functional (must_have #4)

**Severity:** Medium

**Description:** The "Attachments" toggle renders in FilterControls but has no effect on the graph. The `filterNodes` function checks `n.type === "attachment"` but the backend GraphNode only ever produces types `"page"` or `"tag"` (`internal/graph/query.go:38`). The TypeScript client type confirms this (`web/src/api/client.ts:261`: `type: "page" | "tag"`).

**In the UI this means:** User sees an "Attachments" toggle chip, toggles it, and nothing changes. This is misleading.

**Root cause:** The graph doesn't model attachment nodes. This was likely always the case — the toggle was added speculatively or from requirements that pre-date the actual graph data model.

**Remediation:** Either (a) remove the toggle from FilterControls since the data model doesn't support it, or (b) extend the backend to include attachment nodes in the graph payload with `type: "attachment"`.

### Gap P17-G02: Existing files only toggle is non-functional (must_have #5)

**Severity:** Medium

**Description:** The "Existing files only" toggle renders in FilterControls but has no effect. The `filterNodes` function checks `(n as { exists?: boolean }).exists === false` but the backend never sends an `exists` field on graph nodes. All nodes in the graph are existing files by construction — the graph rebuild (`internal/graph/graphjob.go:97-101`) skips pages that no longer exist on disk.

**In the UI this means:** User sees "Existing files only" toggle, toggles it, and nothing changes.

**Root cause:** The backend doesn't track missing-file nodes in the graph. Every node is always an existing file.

**Remediation:** Either (a) remove the toggle, or (b) if the intent is to show referenced-but-deleted pages (stale links), extend the graph to include those with an `exists: false` flag.

### Gap P17-G03: Search for aliases and tags is dead code (must_have #1)

**Severity:** Low

**Description:** `matchesSearch` attempts to match `aliases` and `tags` fields via type assertions, but the backend GraphNode struct only serializes `id`, `label`, and `type`. These branches will never match. Search still works correctly for `name` (via `label`) and `path` (via `id`, since node IDs are file paths like `design.md`).

**In the UI this means:** Searching by `aliases` or `tags` has no effect — but these are minor search improvement features that most users won't notice.

**Root cause:** Same as G01/G02 — the graph data model doesn't include these per-node fields.

**Remediation:** Either (a) remove the dead branches, or (b) add these fields to the backend GraphNode when/if the use case arises.

## 4. TypeScript Compilation

```
$ npx tsc --noEmit -p web/tsconfig.json
exit 0, no output
```

Clean compile. The dead-code branches use type assertions (`as { aliases?: string[] }`) rather than actual interface extensions, which is why they don't cause type errors despite the fields not existing on `GraphNode`.

## 5. Gate Result

```yaml
gate_result:
  record_author: qa
  id: GR-P17-01
  phase: "17"
  verdict: fail
  basis: >
    Two toggles (Attachments, Existing files only) render in the UI but have no
    runtime effect because the graph backend data model doesn't include
    attachment-type nodes or an exists field. These are misleading UI elements:
    a user can toggle them repeatedly with no visible change. One low-severity
    dead-code branch in search (aliases/tags matching).
  gaps:
    - P17-G01: Attachments toggle non-functional
    - P17-G02: Existing files only toggle non-functional
    - P17-G03: Search aliases/tags matching is dead code
  remediation_owner: architect
  recommendation: >
    The core filtering pipeline (search, tags visibility, orphans, display
    controls, groups button, reset, localStorage persistence, reset) is correctly
    implemented and the tsc compilation is clean. The three gaps are all the same
    root cause: the graph data model doesn't support these features yet. The
    architect should decide whether to (a) remove the non-functional toggles from
    this phase and add them when backend supports them, or (b) treat them as
    forward-looking UI that is inert but not harmful.
```

## 6. Artifact Map

| Artifact | Path | Status |
|----------|------|--------|
| graphFilters store | `web/src/stores/graphFilters.ts` | Verified |
| filter.ts extensions | `web/src/lib/graph/filter.ts` | Verified |
| FilterControls component | `web/src/components/graph/FilterControls.tsx` | Verified |
| FilterControls CSS | `web/src/components/graph/FilterControls.css` | Verified |
| GraphView integration | `web/src/components/graph/GraphView.tsx` | Verified |
| GraphCanvas props | `web/src/components/graph/GraphCanvas.tsx` | Verified |
