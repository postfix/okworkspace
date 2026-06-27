# Milestone v1.1 Requirements — Hardening & Polish

## Overview

This milestone closes v1.0's deferred items: visual UAT, eventual-consistency UX improvement, documentation, and security hardening. The codebase is well-understood from v1.0; no new architecture is needed.

---

## UAT — Visual Verification

### UAT-01: Phase 10 Graph UI Visual Verification

**Requirement:** A human tester verifies that the global and local graph views render correctly with zoom, pan, hover-highlight, and edge toggles functioning as designed.

**Acceptance criteria:**
- [ ] Global graph `/app/graph` renders all pages as nodes with correct degree sizing
- [ ] Orphan pages visually distinguished (no edges, isolated)
- [ ] Zoom (scroll wheel / pinch) works smoothly without clipping issues
- [ ] Pan (drag canvas) works without losing nodes off-screen
- [ ] Hover on node highlights its connected edges and neighbors
- [ ] Click on node navigates to that page
- [ ] Edge-type toggle chips (Links, Backlinks, Shared Tags) filter edges correctly
- [ ] Shared Tags toggle OFF by default; toggling ON shows tag edges with capping
- [ ] Local graph dock (page-view) collapses by default; expanding fetches `/graph/local`
- [ ] 1/2/3-hop depth control updates local graph correctly
- [ ] Graph renders on both desktop and mobile viewports

**Notes:**
- Wiring verified in v1.0; this is visual/manual verification only
- Automated tests can cover node count, edge count, API contract — not pixel-level rendering

---

## UX — Eventual Consistency

### UX-01: Bulk-Approve Eventual-Consistency UX

**Requirement:** Improve the bulk-approve user experience when the commit hasn't physically landed yet, reducing confusion about whether the operation succeeded.

**Acceptance criteria:**
- [ ] Bulk-approve shows progress indication (polling state or job queue view)
- [ ] User sees "applying..." state while commit is in-flight
- [ ] Success state shown only after commit lands (or clear "queued" state if async)
- [ ] Stale revision (409) per-page failures don't sink the batch; user sees which pages succeeded/failed
- [ ] Optional: "View pending jobs" admin panel shows queue status

**Design options (choose one):**
1. Poll until commit lands (simplest, blocks UI)
2. Show "queued" state with progress indicator (non-blocking)
3. Dedicated admin panel for job queue visibility (more work)

**Notes:**
- Current behavior is correct (job is durable, re-drains on restart) but UX is confusing
- Must not introduce new races or break single-writer guarantee

---

## Documentation

### DOC-01: Contributor Architecture Docs

**Requirement:** Create `docs/` directory with contributor-facing architecture documentation for v1.0 graph and tag subsystems.

**Acceptance criteria:**
- [ ] `docs/README.md` — project overview, how to contribute
- [ ] `docs/architecture/overview.md` — system architecture diagram and data flow
- [ ] `docs/architecture/graph-subsystem.md` — graph store, rebuild, query layer
- [ ] `docs/architecture/tag-subsystem.md` — tag suggestion, bulk sweep, apply path
- [ ] `docs/api/` — public API surfaces (endpoint contracts, not implementation)
- [ ] All docs cross-link to relevant code files (e.g., `internal/graph/store.go`)

**Notes:**
- Security review was run at v1.0 close; docs were deferred
- This is documentation, not new code — focus on clarity for contributors

### DOC-02: User-Facing README Update

**Requirement:** Update root `README.md` to reflect v1.0 features and current capabilities.

**Acceptance criteria:**
- [ ] Feature list includes graph, backlinks, LLM tagging
- [ ] Screenshots or ASCII diagrams for key features (optional)
- [ ] Quick start guide updated for current config
- [ ] Link to `docs/` for detailed architecture

---

## Security Hardening

### SEC-01: Admin Sweep Fan-Out Cap

**Requirement:** Add a configurable cap on concurrent tag-suggest jobs per bulk sweep to prevent resource exhaustion.

**Acceptance criteria:**
- [ ] Config setting for max concurrent jobs per sweep (default: 10)
- [ ] Sweep enqueues jobs in batches up to cap; remaining queued until slots free
- [ ] Admin UI shows progress (N/M pages processed)
- [ ] Config documented in `config.yaml` example

**Notes:**
- Current: unbounded fan-out, admin-only, small team (low risk)
- Defense-in-depth, not a vulnerability

### SEC-02: Tag Charset Input Validation

**Requirement:** Add server-side validation on tag input to reject disallowed characters (defense-in-depth alongside existing output encoding).

**Acceptance criteria:**
- [ ] `okf.SetTags` validates tag strings against allowed charset (alphanumeric, dash, underscore, space)
- [ ] Invalid tags rejected with 400 Bad Request
- [ ] Existing sink encoding (Markdown render, graph render) unchanged
- [ ] Test cases for XSS attempts via tag names

**Notes:**
- Current: tags permit HTML metachars, all sinks encode correctly
- Defense-in-depth: prevent HTML metachars from ever entering the system

### SEC-03: `govulncheck` Resolution

**Requirement:** Investigate and resolve the `govulncheck` inconclusive state caused by Go 1.26/x-tools toolchain panic.

**Acceptance criteria:**
- [ ] Root cause identified (Go 1.26 bug, x-tools issue, or configuration)
- [ ] Resolution path chosen (upgrade Go, upgrade x-tools, wait for fix, alternative scanner)
- [ ] `govulncheck ./...` runs cleanly or has documented exceptions
- [ ] CI workflow updated if scanning approach changes

**Notes:**
- `go vet ./...` is clean
- This is hardening, not a blocking vulnerability

---

## Graph Filter Sidebar

### GF-01: Graph Filter Sidebar

**Requirement:** Implement a dark-themed, collapsible Graph Filter Sidebar panel (~313px wide) on the global and local graph views that controls node/edge visibility and display settings with live updates.

**Acceptance criteria:**
- [ ] Sidebar panel with "Filters" header, collapse chevron, reset button, and close button
- [ ] Search input ("Search files...") filters graph nodes live by name, path, aliases, and tags
- [ ] Toggle: Tags (default ON) — show/hide tag nodes and tag-only edges
- [ ] Toggle: Attachments (default OFF) — show/hide attachment nodes (image, pdf, audio, video, archive, binary)
- [ ] Toggle: Existing files only (default OFF) — hide nodes for missing/nonexistent files
- [ ] Toggle: Orphans (default ON) — show/hide nodes with zero visible edges after all other filters
- [ ] Groups section (collapsible) with "New group" button for creating named, colored graph groups with query matching
- [ ] Display section (collapsible) with:
  - Arrows toggle (default OFF) — show/hide link direction arrowheads
  - Text fade threshold slider (0–1, default 0.5) — control label fade based on zoom level
  - Node size slider (0–1, default 0.2) — control node radius (2px–12px range)
  - Link thickness slider (0–1, default 0.2) — control edge stroke width (0.5px–5px range)
- [ ] All controls update the graph immediately (no Apply button)
- [ ] Reset button restores all settings to defaults
- [ ] Settings persisted per workspace (localStorage or backend workspace_settings)
- [ ] Filtering pipeline is deterministic and ordered: node filter → edge filter → orphan filter → display settings
- [ ] Accessible: real `<button>`, `<input type="search">`, `<input type="range">` with aria-labels; keyboard navigable
- [ ] Dark theme: bg #262626, text #d4d4d4, accent purple #8b5cf6

**Notes:**
- Discovered during Phase 13 UAT — the graph needs filtering to be usable at scale
- Builds on Phase 10 (GraphCanvas, useGraph, graphEdges store)
- CSS variables should use existing project token patterns where possible
- Graph groups can be deferred to a follow-up if scope is too large

---

## Out of Scope

The following were considered and explicitly deferred:

- **GraphCanvas bundle lazy-load** — Performance polish, deferred to backlog
- **Graph performance/clustering at scale** — Future optimization when page count warrants
- **Tag rename/merge across workspace** — New feature, separate milestone
- **Real-time collaboration (CRDT)** — Out of MVP scope

---

## Traceability

| REQ-ID | Phase | Status |
|--------|-------|--------|
| UAT-01 | Phase 13 | In progress |
| UX-01 | Phase 15 | Not started |
| DOC-01 | Phase 14 | Not started |
| DOC-02 | Phase 14 | Not started |
| SEC-01 | Phase 16 | Not started |
| SEC-02 | Phase 16 | Not started |
| SEC-03 | Phase 16 | Not started |
| GF-01 | Phase 17 | Not started |