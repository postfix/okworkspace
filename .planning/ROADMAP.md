# Roadmap: OKF Workspace

## Overview

OKF Workspace is built dependency-first: a safe foundation (single binary, auth, path resolver, Git repo, single-writer commit service) comes before any file operation; the core wiki loop (OKF pages, navigation, hidden Git history) is built on that foundation with a byte-stable Markdown round-trip as its exit gate; attachments and their text-extraction pipeline follow, gating both search and the agent; full-text search indexes pages plus extracted attachment text; an approval-gated Eino agent reads, summarizes, rewrites, and proposes diffs that humans approve before applying; and soft-lock collaboration hardens the save path with presence and conflict resolution; finally the editor and file tree are reshaped into an Obsidian-style experience. The v1.0 milestone layers a knowledge graph and LLM auto-tagging on top of these shipped v0.9.9 seams: a derived link/tag adjacency store (rebuildable from files, never source of truth) powers backlinks, a global + local force-directed graph UI, and an LLM tag-suggestion chain routed through the existing propose→approve→apply→commit safety model. Each phase delivers an end-to-end user capability and reuses the cross-cutting spines (safe-path resolver, single-writer Git service, async job worker) established early.

## Milestones

- ✅ **v0.9.9 MVP** — Phases 0–7 (shipped 2026-06-23) — full self-hosted OKF wiki: auth/RBAC, pages+hidden Git, attachments+extraction, search, approval-gated Eino agent, soft-lock collaboration, Obsidian-style live-preview editor + file tree. See [`milestones/v0.9.9-ROADMAP.md`](milestones/v0.9.9-ROADMAP.md).
- 🚧 **v1.0 Knowledge Graph & LLM Auto-Tagging** — Phases 8–12 (in progress) — derived link/tag store + backlinks, Obsidian-style global/local graph UI with edge-type toggles, and approval-gated per-page + bulk LLM tag suggestion.

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

### 🚧 v1.0 Knowledge Graph & LLM Auto-Tagging (In Progress)

**Milestone Goal:** Add Obsidian-style graph visualization and LLM-assisted tagging so the team can *see* how knowledge connects and keep it organized with minimal effort — all over existing v0.9.9 seams, with zero new backend dependency and exactly one new frontend package (`react-force-graph-2d`), preserving the single CGO-free binary and files-as-truth constraints.

- [ ] **Phase 8: Derived Link/Tag Store & Maintenance** - Rebuildable link/backlink/tag adjacency kept fresh on every page mutation, with an admin rebuild backstop
- [ ] **Phase 9: Graph Endpoints & Backlinks Panel** - HTTP graph endpoints (typed edges incl. shared-tag) and a page-view "Referenced by" backlinks panel
- [ ] **Phase 10: Graph UI** - Obsidian-style global graph view + per-page local panel with edge-type toggles, degree sizing, orphans, and hover-highlight
- [ ] **Phase 11: Per-Page LLM Tag Suggestion** - Byte-stable `okf.SetTags` primitive + on-demand suggest→approve tagging through the single-writer commit path
- [ ] **Phase 12: Bulk Sweep & Batch Review Queue** - Admin bulk untagged-pages sweep that enqueues suggestion jobs into a review queue, approved per page through the same byte-stable apply path

## Phase Details

### Phase 8: Derived Link/Tag Store & Maintenance

**Goal**: A derived, rebuildable-from-files adjacency store (page links, backlinks, tag membership) exists and stays fresh on every page mutation — the foundation both the graph UI and tag-vocabulary prompting depend on.
**Depends on**: Phase 1 (page mutation seams), Phase 3 (KindIndex/RebuildIndex pattern to mirror) — both shipped in v0.9.9
**Requirements**: LINK-01, LINK-03
**Success Criteria** (what must be TRUE):

  1. After any page mutation (create / save / rename / move / delete-to-trash / restore), the stored link and backlink adjacency reflects the change without an app restart
  2. The link/tag store is derived only — deleting it and rebuilding from the Markdown files on disk reproduces identical adjacency (SQLite is never the source of truth)
  3. An admin can rebuild the link/graph index from files via an affordance consistent with the existing "Rebuild search index" button
  4. Tag membership per page is queryable, giving the workspace's existing tag vocabulary for downstream prompting

**Plans**: 2/3 plans executed

- [x] 08-01-PLAN.md — `internal/graph` derived store (page_links/page_tags + migration 0009), KindGraph job + RebuildGraph data layer (LINK-01)
- [x] 08-02-PLAN.md — startup handler registration + drift rebuild + per-mutation fire-and-forget graph enqueue across all mutation kinds (LINK-01)
- [ ] 08-03-PLAN.md — admin "Rebuild graph index" affordance: POST /admin/graph/reindex + ActionGraphReindex + Admin.tsx button (LINK-03)

### Phase 9: Graph Endpoints & Backlinks Panel

**Goal**: The stored adjacency is exposed over HTTP as typed-edge graph payloads, and the first user-visible output ships: a page-view backlinks panel listing every page that links to the current one.
**Depends on**: Phase 8
**Requirements**: LINK-02
**Success Criteria** (what must be TRUE):

  1. A user viewing a page sees a "Referenced by" panel listing every page that links to it, and clicking an entry navigates to that page
  2. A global graph endpoint returns all pages as nodes and their links as typed edges (page-links / backlinks / shared-tags) in a lean payload
  3. A local graph endpoint returns a given page's neighborhood (the page plus its direct neighbors)
  4. Shared-tag edges are computed with a popular-tag cap / threshold so the payload never explodes into a hairball on common tags

**Plans**: TBD
**UI hint**: yes

### Phase 10: Graph UI

**Goal**: The headline visual feature ships — an Obsidian-style global graph view and a docked per-page local graph panel, both interactive, with configurable edge types.
**Depends on**: Phase 9
**Requirements**: GRAPH-01, GRAPH-02, GRAPH-03, GRAPH-04, GRAPH-05
**Success Criteria** (what must be TRUE):

  1. A user can open a global graph view showing all pages as nodes and links as edges, pan and zoom it, and click a node to open that page
  2. Node size visibly reflects connection count (degree), and orphan (unlinked) pages are visible and distinguishable from connected ones
  3. A user can open a per-page local graph panel showing the current page plus its direct neighbors, which auto-updates when the active page changes and offers a depth control (default 1 hop)
  4. A user can toggle edge types (page links / backlinks / shared tags) on and off in the graph UI, with shared-tag edges off by default
  5. Hovering a node highlights that node and its immediate neighbors and edges

**Plans**: TBD
**UI hint**: yes

### Phase 11: Per-Page LLM Tag Suggestion

**Goal**: A user can get LLM tag suggestions for the page they are on and approve them per tag, with approved tags merged byte-stably into the YAML frontmatter through the existing single-writer commit path — establishing trust before any bulk operation.
**Depends on**: Phase 8 (tag vocabulary), Phase 4 (propose→apply→Save safety pattern to mirror — shipped in v0.9.9)
**Requirements**: TAG-01, TAG-02, TAG-03, TAG-04
**Success Criteria** (what must be TRUE):

  1. A user can request LLM tag suggestions on demand for the page they are viewing or editing
  2. Suggested tags are presented for per-tag approve/reject before anything is written, with new (invented) tags distinguished from existing ones and defaulting to unchecked
  3. Approving tags changes only the `tags` lines of the page's YAML frontmatter (body and other frontmatter byte-unchanged), commits through the single-writer Git path, and a stale page revision 409s rather than clobbering
  4. Suggestions are biased toward the existing workspace tag vocabulary, capped at max 5 per page, and normalized on write (lowercase, trimmed, deduped)

**Plans**: TBD
**UI hint**: yes

### Phase 12: Bulk Sweep & Batch Review Queue

**Goal**: An admin can run a bulk tagging sweep over untagged (or all) pages that produces a review queue of pending suggestions — writing nothing automatically — which a user reviews and approves per page through the same byte-stable apply path.
**Depends on**: Phase 11 (per-page suggest + apply primitives), Phase 8 (page_tags for untagged-page enumeration)
**Requirements**: TAG-05, TAG-06
**Success Criteria** (what must be TRUE):

  1. An admin can start a bulk tagging sweep over untagged (or all) pages that enqueues per-page suggestion jobs on the existing async worker and produces a queue of pending suggestions, writing nothing automatically
  2. A user can review the bulk-sweep suggestion queue and approve or reject suggestions per page
  3. Approvals from the queue route through the same byte-stable frontmatter apply path as per-page tagging, committed in batches (not one commit per page)
  4. The sweep is resumable and never bypasses human approval — killing and restarting the worker does not auto-write tags

**Plans**: TBD
**UI hint**: yes

## Progress

**Execution Order:**
Phases execute in numeric order: 8 → 9 → 10 → 11 → 12. Phase 8 is the shared foundation; Phases 9→10 (graph chain) and Phase 11 (tag chain) can proceed in parallel after Phase 8; Phase 12 depends on both Phase 11 and Phase 8.

| Phase | Milestone | Plans | Status | Completed |
| ----- | --------- | ----- | ------ | --------- |
| 0. Skeleton, Auth & Foundations | v0.9.9 | 4/4 | Complete | 2026-06-18 |
| 1. OKF Pages, Navigation & Hidden Git | v0.9.9 | 5/5 | Complete | 2026-06-18 |
| 2. Attachments & Text Extraction | v0.9.9 | 4/4 | Complete | 2026-06-21 |
| 3. Search | v0.9.9 | 4/4 | Complete | 2026-06-21 |
| 4. Eino Agent | v0.9.9 | 7/7 | Complete | 2026-06-21 |
| 5. Collaboration | v0.9.9 | 4/4 | Complete | 2026-06-22 |
| 6. Live-Preview Editor (Obsidian-style) | v0.9.9 | 4/4 | Complete | 2026-06-21 |
| 7. Obsidian-style File Tree | v0.9.9 | 4/4 | Complete | 2026-06-21 |
| 8. Derived Link/Tag Store & Maintenance | v1.0 | 2/3 | In Progress|  |
| 9. Graph Endpoints & Backlinks Panel | v1.0 | 0/TBD | Not started | - |
| 10. Graph UI | v1.0 | 0/TBD | Not started | - |
| 11. Per-Page LLM Tag Suggestion | v1.0 | 0/TBD | Not started | - |
| 12. Bulk Sweep & Batch Review Queue | v1.0 | 0/TBD | Not started | - |

**v0.9.9 MVP: 8/8 phases, 36/36 plans complete — shipped 2026-06-23.** All phase verifications closed (live browser UAT 2026-06-24).
**v1.0 Knowledge Graph & LLM Auto-Tagging: 0/5 phases — planning.**
