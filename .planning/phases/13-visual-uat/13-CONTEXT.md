---
author: architect
---

# Phase 13: Visual UAT - Context

**Gathered:** 2026-06-27
**Status:** Ready for planning

<domain>
## Phase Boundary

Manual browser testing of Phase 10 graph UI — verifying 11 visual/interactive criteria (UAT-01) that automated tests cannot cover: canvas rendering, zoom/pan, hover-highlight, edge toggles, node sizing, orphan distinction, click-to-navigate, and local graph dock behavior. **No new code.** This phase catches visual issues that wiring tests missed.

**In scope:** Human verification of all 11 UAT-01 acceptance criteria in a Chromium desktop browser against the existing workspace, with bugs filed immediately as `.planning/todos/pending/` entries.

**Out of scope:** Any new features, code changes, API changes, bug fixes (those are separate phases). Mobile device testing, cross-browser testing, performance profiling, accessibility auditing.
</domain>

<decisions>
## Implementation Decisions

### Test Environment
- **D-01:** Run via `./scripts/dev.sh` — local dev server on `localhost:8098`. Matches existing Phase 10 UAT pattern. Admin password: `AdminPass123!`.

### Browser Coverage
- **D-02:** Chromium desktop only (Chrome/Edge/Brave). Canvas rendering is consistent across Chromium variants; this covers the primary desktop use case.

### Test Data
- **D-03:** Use existing workspace data. No dedicated test workspace needed — if the workspace lacks certain scenarios (orphan pages, high-degree nodes, tag-heavy pages), note that as a data limitation rather than creating test data.

### Bug Workflow
- **D-04:** File each visual/interactive bug immediately as a `.planning/todos/pending/` UAT todo with severity classification:
  - **blocker:** Renders graph unusable (crash, blank canvas, all nodes missing)
  - **major:** Breaks a success criterion (e.g., hover-highlight broken, edge toggle doesn't filter)
  - **minor:** Cosmetic (e.g., slight clipping, label overlap, orphan distinction too subtle)
  Blockers and majors must be resolved before phase close. Minors are noted but non-blocking.

### Claude's Discretion
- Specific page selection during testing (explore naturally; prioritize pages with diverse link/tag counts).
- Exact visual threshold for "smooth" zoom/pan (subjective but should feel responsive, no frame-drops on existing workspace data).
</decisions>

<disciplines>
## Lenses surfaced for this phase

No disciplines surfaced — phase scope didn't call for any (manual testing is inherently guided by the checklist; override signals don't meaningfully apply).
</disciplines>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Phase 10 Graph UI (what's being tested)
- `.planning/phases/10-graph-ui/10-CONTEXT.md` — Phase 10 implementation decisions (graph location, local panel, node sizing/coloring, rendering library, edge toggles)
- `.planning/phases/10-graph-ui/10-UAT.md` — Deferred Phase 10 UAT (two tests: global graph + local graph panel; deferred awaiting browser session)
- `web/src/components/graph/GraphCanvas.tsx` — The imperative Canvas wrapper (force engine, auto-sizing, XSS guard)

### Requirements
- `.planning/REQUIREMENTS.md` §UAT-01 — 11 acceptance criteria for visual verification

### Key source files
- `web/src/components/graph/GraphCanvas.tsx` — Canvas wrapper component
- `web/src/routes/GraphView.tsx` — Global graph route (`/app/graph`)
- `web/src/routes/PageView.tsx` — Local graph dock (right-side collapsible panel)
- `internal/server/handlers_graph.go` — Graph HTTP endpoints
- `internal/graph/query.go` — Graph query layer
</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- Phase 10 UAT pattern: `./scripts/dev.sh` → browser → criteria checklist → deferred items in `.planning/todos/pending/`
- Dev script (`./scripts/dev.sh`) already exists and starts the Go binary + embedded SPA
- Admin credentials: `AdminPass123!` (from `config.yaml` default / admin bootstrap)

### Integration Points
- Global graph: `http://localhost:8098/app/graph`
- Local graph: any page view (e.g., `http://localhost:8098/app/page/index`) — expand right-side "Local graph" dock
- Backlinks panel: page view shows "Referenced by" below the page body

### Established Patterns
- Prior UATs (Phases 8, 9, 10, 11) all used the "deferred" pattern — automated tests passed, visual/manual verification deferred
- Bug tracking uses `.planning/todos/pending/*.md` with severity classification
</code_context>

<specifics>
## Specific Ideas

- Phase 10 UAT already has a starter checklist (2 tests: global graph + local graph panel). Phase 13 expands that to the full 11 UAT-01 criteria.
- The existing workspace should have enough pages and tags to exercise all graph features — if not, note the missing scenarios as data limitations (not bugs).
- Canvas rendering is pixel-level — screenshot comparison is unnecessary; visual judgment is sufficient for this pass.
</specifics>

<deferred>
## Deferred Ideas

- Mobile viewport testing — out of scope for this phase; Chromium desktop only per D-02.
- Cross-browser testing (Firefox, Safari) — out of scope per D-02.
- Accessibility audit of graph canvas — out of scope (canvas is inherently screen-reader-unfriendly; would need separate alternative presentation, deferred to future milestone).
- GraphCanvas bundle lazy-load — already deferred to backlog from v1.0 close.

None — discussion stayed within phase scope.
</deferred>

---
*Phase: 13-visual-uat*
*Context gathered: 2026-06-27*
