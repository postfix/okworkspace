# Roadmap: OKF Workspace

## Overview

OKF Workspace is built dependency-first: a safe foundation (single binary, auth, path resolver, Git repo, single-writer commit service) comes before any file operation; the core wiki loop (OKF pages, navigation, hidden Git history) is built on that foundation with a byte-stable Markdown round-trip as its exit gate; attachments and their text-extraction pipeline follow, gating both search and the agent; full-text search indexes pages plus extracted attachment text; an approval-gated Eino agent reads, summarizes, rewrites, and proposes diffs that humans approve before applying; and soft-lock collaboration hardens the save path with presence and conflict resolution; finally the editor and file tree are reshaped into an Obsidian-style experience. The v1.0 milestone layers a knowledge graph and LLM auto-tagging on top of these shipped v0.9.9 seams: a derived link/tag adjacency store (rebuildable from files, never source of truth) powers backlinks, a global + local force-directed graph UI, and an LLM tag-suggestion chain routed through the existing propose→approve→apply→commit safety model. Each phase delivers an end-to-end user capability and reuses the cross-cutting spines (safe-path resolver, single-writer Git service, async job worker) established early.

## Milestones

- ✅ **v0.9.9 MVP** — Phases 0–7 (shipped 2026-06-23) — full self-hosted OKF wiki: auth/RBAC, pages+hidden Git, attachments+extraction, search, approval-gated Eino agent, soft-lock collaboration, Obsidian-style live-preview editor + file tree. See [`milestones/v0.9.9-ROADMAP.md`](milestones/v0.9.9-ROADMAP.md).
- ✅ **v1.0 Knowledge Graph & LLM Auto-Tagging** — Phases 8–12 (shipped 2026-06-24) — derived link/tag store + backlinks, Obsidian-style global/local graph UI with edge-type toggles, and approval-gated per-page + bulk LLM tag suggestion. See [`milestones/v1.0-ROADMAP.md`](milestones/v1.0-ROADMAP.md).

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
| 8. Derived Link/Tag Store & Maintenance | v1.0 | 3/3 | Complete    | 2026-06-24 |
| 9. Graph Endpoints & Backlinks Panel | v1.0 | 2/2 | Complete    | 2026-06-24 |
| 10. Graph UI | v1.0 | 3/3 | Complete    | 2026-06-24 |
| 11. Per-Page LLM Tag Suggestion | v1.0 | 3/3 | Complete    | 2026-06-24 |
| 12. Bulk Sweep & Batch Review Queue | v1.0 | 3/3 | Complete   | 2026-06-24 |

**v0.9.9 MVP: 8/8 phases, 36/36 plans complete — shipped 2026-06-23.** All phase verifications closed (live browser UAT 2026-06-24).
**v1.0 Knowledge Graph & LLM Auto-Tagging: 5/5 phases, 14/14 plans complete — shipped 2026-06-24.** Audit passed (14/14 requirements, 5/5 integration seams), security review passed.
