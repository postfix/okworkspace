# Roadmap: OKF Workspace

## Overview

OKF Workspace is built dependency-first: a safe foundation (single binary, auth, path resolver, Git repo, single-writer commit service) comes before any file operation; the core wiki loop (OKF pages, navigation, hidden Git history) is built on that foundation with a byte-stable Markdown round-trip as its exit gate; attachments and their text-extraction pipeline follow, gating both search and the agent; full-text search indexes pages plus extracted attachment text; an approval-gated Eino agent reads, summarizes, rewrites, and proposes diffs that humans approve before applying; and soft-lock collaboration hardens the save path with presence and conflict resolution; finally the editor and file tree are reshaped into an Obsidian-style experience. Each phase delivers an end-to-end user capability and reuses the cross-cutting spines (safe-path resolver, single-writer Git service, async job worker) established early.

## Milestones

- ✅ **v0.9.9 MVP** — Phases 0–7 (shipped 2026-06-23) — full self-hosted OKF wiki: auth/RBAC, pages+hidden Git, attachments+extraction, search, approval-gated Eino agent, soft-lock collaboration, Obsidian-style live-preview editor + file tree. See [`milestones/v0.9.9-ROADMAP.md`](milestones/v0.9.9-ROADMAP.md).

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

## Progress

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

**v0.9.9 MVP: 8/8 phases, 36/36 plans complete — shipped 2026-06-23.** All phase verifications closed (live browser UAT 2026-06-24).
