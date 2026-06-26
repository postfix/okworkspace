# Roadmap: OKF Workspace

## Overview

OKF Workspace is built dependency-first: a safe foundation (single binary, auth, path resolver, Git repo, single-writer commit service) comes before any file operation; the core wiki loop (OKF pages, navigation, hidden Git history) is built on that foundation with a byte-stable Markdown round-trip as its exit gate; attachments and their text-extraction pipeline follow, gating both search and the agent; full-text search indexes pages plus extracted attachment text; an approval-gated Eino agent reads, summarizes, rewrites, and proposes diffs that humans approve before applying; and soft-lock collaboration hardens the save path with presence and conflict resolution; finally the editor and file tree are reshaped into an Obsidian-style experience. The v1.0 milestone layers a knowledge graph and LLM auto-tagging on top of these shipped v0.9.9 seams: a derived link/tag adjacency store (rebuildable from files, never source of truth) powers backlinks, a global + local force-directed graph UI, and an LLM tag-suggestion chain routed through the existing propose→approve→apply→commit safety model. Each phase delivers an end-to-end user capability and reuses the cross-cutting spines (safe-path resolver, single-writer Git service, async job worker) established early.

## Milestones

- ✅ **v0.9.9 MVP** — Phases 0–7 (shipped 2026-06-23) — full self-hosted OKF wiki: auth/RBAC, pages+hidden Git, attachments+extraction, search, approval-gated Eino agent, soft-lock collaboration, Obsidian-style live-preview editor + file tree. See [`milestones/v0.9.9-ROADMAP.md`](milestones/v0.9.9-ROADMAP.md).
- ✅ **v1.0 Knowledge Graph & LLM Auto-Tagging** — Phases 8–12 (shipped 2026-06-24) — derived link/tag store + backlinks, Obsidian-style global/local graph UI with edge-type toggles, and approval-gated per-page + bulk LLM tag suggestion. See [`milestones/v1.0-ROADMAP.md`](milestones/v1.0-ROADMAP.md).
- 🔵 **v1.1 Hardening & Polish** — Phases 13–16 (in progress) — visual UAT, UX improvement, documentation, and security hardening.

## Phases

<details>
<summary>✅ v0.9.9 MVP (Phases 0–7, 36 plans) — SHIPPED 2026-06-23</summary>

- [x] **Phase 0: Skeleton, Auth & Foundations** (4 plans) — single binary serves the React shell behind a login; RBAC, sessions, safe-path resolver, Git repo init, single-writer commit spine, audit log
- [x] **Phase 1: OKF Pages, Navigation & Hidden Git** (5 plans) — create/edit/organize/version pages with automatic hidden Git history, trash + restore, byte-stable Markdown round-trip
- [x] **Phase 2: Attachments & Text Extraction** (4 plans) — upload/preview/download/replace/orphan-delete original-fidelity attachments with PDF/DOCX/TXT extraction
- [x] **Phase 3: Search** (4 plans) — find pages, headings, and attachments across titles, body, tags, filenames, and extracted text (Bleve, incremental index)
- [x] **Phase 4: Eino Agent** (7 plans) — approval-gated AI help over pages, selections, attachments, and the workspace; real-diff DiffReviewDialog trust gate
- [x] **Phase 5: Collaboration** (4 plans) — presence, soft locks, force-edit, and conflict resolution so concurrent edits never silently lose work
- [x] **Phase 6: Live-Preview Editor (Obsidian-style)** (4 plans) — inline-rendering Markdown editor with source toggle, byte-stable round-trip preserved
- [x] **Phase 7: Obsidian-style File Tree** (4 plans) — folder operations directly in the tree (right-click menus, drag-and-drop, folder rename/move/delete)

Full phase detail, success criteria, and plan breakdown archived in [`milestones/v0.9.9-ROADMAP.md`](milestones/v0.9.9-ROADMAP.md).

</details>

<details>
<summary>✅ v1.0 Knowledge Graph & LLM Auto-Tagging (Phases 8–12, 14 plans) — SHIPPED 2026-06-24</summary>

- [x] **Phase 8: Derived Link/Tag Store & Maintenance** (3 plans) — rebuildable link/backlink/tag adjacency kept fresh on every page mutation, with an admin rebuild backstop (LINK-01, LINK-03)
- [x] **Phase 9: Graph Endpoints & Backlinks Panel** (2 plans) — HTTP graph endpoints (typed edges incl. shared-tag) and a page-view "Referenced by" backlinks panel (LINK-02)
- [x] **Phase 10: Graph UI** (3 plans) — Obsidian-style global graph view + per-page local panel with edge-type toggles, degree sizing, orphans, and hover-highlight (GRAPH-01..05)
- [x] **Phase 11: Per-Page LLM Tag Suggestion** (3 plans) — byte-stable `okf.SetTags` primitive + on-demand suggest→approve tagging through the single-writer commit path (TAG-01..04)
- [x] **Phase 12: Bulk Sweep & Batch Review Queue** (3 plans) — admin bulk untagged-pages sweep that enqueues suggestion jobs into a review queue, approved per page through the same byte-stable apply path (TAG-05, TAG-06)

Full phase detail, success criteria, and plan breakdown archived in [`milestones/v1.0-ROADMAP.md`](milestones/v1.0-ROADMAP.md). Close gates: audit passed (14/14 requirements), integration `integration_ok`, security review passed.

</details>

<details>
<summary>🔵 v1.1 Hardening & Polish (Phases 13–16) — IN PROGRESS</summary>

- [ ] **Phase 13: Visual UAT** (1 plan) — manual browser testing of Phase 10 graph UI (zoom, pan, hover-highlight, edge toggles, global + local views) (UAT-01)
- [ ] **Phase 14: Documentation** (2 plans) — contributor architecture docs for graph/tag subsystems, user-facing README update (DOC-01, DOC-02)
- [ ] **Phase 15: UX Improvement** (1 plan) — bulk-approve eventual-consistency UX improvement with progress indication (UX-01)
- [ ] **Phase 16: Security Hardening** (3 plans) — admin sweep fan-out cap, tag charset input validation, govulncheck resolution (SEC-01, SEC-02, SEC-03)

Phase 13 (UAT) catches visual issues early. Phase 14 (docs) can run in parallel. Phases 15–16 follow.

</details>

## Progress

**Execution Order:**
- Phases 13 and 14 can run in parallel
- Phase 15 depends on Phase 13 (visual issues caught before UX work)
- Phase 16 can start after Phase 13

| Phase | Milestone | Plans | Status | Completed |
| ----- | --------- | ----- | ------ | --------- |
| 0. Skeleton, Auth & Foundations | v0.9.9 | 4/4 | Complete | 2026-06-18 |
| 1. OKF Pages, Navigation & Hidden Git | v0.9.9 | 5/5 | Complete | 2026-06-19 |
| 2. Attachments & Text Extraction | v0.9.9 | 4/4 | Complete | 2026-06-20 |
| 3. Search | v0.9.9 | 4/4 | Complete | 2026-06-20 |
| 4. Eino Agent | v0.9.9 | 7/7 | Complete | 2026-06-21 |
| 5. Collaboration | v0.9.9 | 4/4 | Complete | 2026-06-21 |
| 6. Live-Preview Editor | v0.9.9 | 4/4 | Complete | 2026-06-22 |
| 7. Obsidian-style File Tree | v0.9.9 | 4/4 | Complete | 2026-06-22 |
| 8. Derived Link/Tag Store | v1.0 | 3/3 | Complete | 2026-06-23 |
| 9. Graph Endpoints & Backlinks | v1.0 | 2/2 | Complete | 2026-06-24 |
| 10. Graph UI | v1.0 | 3/3 | Complete | 2026-06-24 |
| 11. Per-Page LLM Tag Suggestion | v1.0 | 3/3 | Complete | 2026-06-24 |
| 12. Bulk Sweep & Batch Review | v1.0 | 3/3 | Complete | 2026-06-24 |
| **13. Visual UAT** | **v1.1** | **0/1** | **Not started** | — |
| **14. Documentation** | **v1.1** | **0/2** | **Not started** | — |
| **15. UX Improvement** | **v1.1** | **0/1** | **Not started** | — |
| **16. Security Hardening** | **v1.1** | **0/3** | **Not started** | — |

---

## Phase 13: Visual UAT

**Goal:** Verify that Phase 10 graph UI renders correctly with zoom, pan, hover-highlight, and edge toggles functioning as designed.

**Requirements:** UAT-01

**Context:** Phase 10 wiring was verified in v1.0, but visual/manual testing was deferred. This phase performs the human verification that automated tests cannot cover.

**Success Criteria:**
1. Global graph `/app/graph` renders all pages as nodes with correct degree sizing
2. Orphan pages visually distinguished (no edges, isolated)
3. Zoom (scroll wheel / pinch) works smoothly without clipping issues
4. Pan (drag canvas) works without losing nodes off-screen
5. Hover on node highlights its connected edges and neighbors
6. Click on node navigates to that page
7. Edge-type toggle chips (Links, Backlinks, Shared Tags) filter edges correctly
8. Shared Tags toggle OFF by default; toggling ON shows tag edges with capping
9. Local graph dock (page-view) collapses by default; expanding fetches `/graph/local`
10. 1/2/3-hop depth control updates local graph correctly
11. Graph renders on both desktop and mobile viewports

**Key Files:**
- `web/src/components/GraphCanvas.tsx`
- `web/src/routes/PageView.tsx` (local graph dock)
- `internal/server/handlers_graph.go`
- `internal/graph/query.go`

**Dependencies:** None (v1.0 codebase is stable)

---

## Phase 14: Documentation

**Goal:** Create contributor-facing architecture documentation for v1.0 graph and tag subsystems.

**Requirements:** DOC-01, DOC-02

**Context:** Security review was run at v1.0 close, but `docs/` was deferred. Contributors need architecture docs to understand the graph/tag subsystems.

**Success Criteria:**
1. `docs/README.md` — project overview, how to contribute
2. `docs/architecture/overview.md` — system architecture diagram and data flow
3. `docs/architecture/graph-subsystem.md` — graph store, rebuild, query layer
4. `docs/architecture/tag-subsystem.md` — tag suggestion, bulk sweep, apply path
5. `docs/api/` — public API surfaces (endpoint contracts)
6. Root `README.md` updated with v1.0 features
7. All docs cross-link to relevant code files

**Key Files:**
- `internal/graph/*.go` (graph subsystem)
- `internal/tagsweep/*.go` (tag subsystem)
- `internal/agent/suggesttags.go` (LLM tagging)
- `internal/server/handlers_graph.go`
- `internal/server/handlers_tagsweep.go`

**Dependencies:** None (documentation work is independent)

---

## Phase 15: UX Improvement

**Goal:** Improve the bulk-approve user experience when the commit hasn't physically landed yet, reducing confusion about whether the operation succeeded.

**Requirements:** UX-01

**Context:** Bulk-approve reports success before the commit lands. The job is durable and re-drains on restart, but the UX is confusing. Users may think the operation failed or succeeded when it's still pending.

**Success Criteria:**
1. Bulk-approve shows progress indication (polling state or job queue view)
2. User sees "applying..." state while commit is in-flight
3. Success state shown only after commit lands (or clear "queued" state if async)
4. Stale revision (409) per-page failures don't sink the batch; user sees which pages succeeded/failed
5. Optional: "View pending jobs" admin panel shows queue status

**Design Decision Required:** Choose one:
- Option A: Poll until commit lands (simplest, blocks UI)
- Option B: Show "queued" state with progress indicator (non-blocking)
- Option C: Dedicated admin panel for job queue visibility (more work)

**Key Files:**
- `internal/server/handlers_tagsweep.go` (bulk-approve endpoint)
- `internal/tagsweep/job.go` (job execution)
- `web/src/routes/Admin.tsx` (admin panel)
- `web/src/api/client.ts` (API client)

**Dependencies:** Phase 13 (visual issues caught early)

---

## Phase 16: Security Hardening

**Goal:** Add defense-in-depth protections for admin sweep, tag input, and vulnerability scanning.

**Requirements:** SEC-01, SEC-02, SEC-03

**Context:** Three informational security notes from v1.0 close:
1. Admin sweep fan-out is unbounded (admin-only, low risk, but could exhaust resources)
2. Tag charset permits HTML metachars (all sinks encode correctly, but defense-in-depth)
3. `govulncheck` is inconclusive due to Go 1.26/x-tools toolchain panic

**Success Criteria:**

**SEC-01: Admin Sweep Fan-Out Cap**
1. Config setting for max concurrent jobs per sweep (default: 10)
2. Sweep enqueues jobs in batches up to cap; remaining queued until slots free
3. Admin UI shows progress (N/M pages processed)
4. Config documented in `config.yaml` example

**SEC-02: Tag Charset Input Validation**
1. `okf.SetTags` validates tag strings against allowed charset (alphanumeric, dash, underscore, space)
2. Invalid tags rejected with 400 Bad Request
3. Existing sink encoding unchanged
4. Test cases for XSS attempts via tag names

**SEC-03: `govulncheck` Resolution**
1. Root cause identified (Go 1.26 bug, x-tools issue, or configuration)
2. Resolution path chosen (upgrade Go, upgrade x-tools, wait for fix, alternative scanner)
3. `govulncheck ./...` runs cleanly or has documented exceptions
4. CI workflow updated if scanning approach changes

**Key Files:**
- `internal/tagsweep/job.go` (SEC-01)
- `internal/okf/emit.go` (SEC-02 - `SetTagsFrontmatter`)
- `go.mod`, CI workflow (SEC-03)

**Dependencies:** Phase 13 (visual issues caught early)

---

## Out of Scope

The following were considered and explicitly deferred:

- **GraphCanvas bundle lazy-load** — Performance polish, deferred to backlog
- **Graph performance/clustering at scale** — Future optimization when page count warrants
- **Tag rename/merge across workspace** — New feature, separate milestone
- **Real-time collaboration (CRDT)** — Out of MVP scope