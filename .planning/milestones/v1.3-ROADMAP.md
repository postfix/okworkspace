# Roadmap: OKF Workspace

## Overview

OKF Workspace is built dependency-first: from auth and the single-writer commit spine through OKF pages, search, Eino agent, collaboration, knowledge graph, and LLM tagging. v1.3 adds workspace-wide tag management — rename, merge, and delete tags across all pages with preview→approve safety.

## Milestones

- ✅ **v0.9.9 MVP** — Phases 0–7 (shipped 2026-06-23) — full self-hosted OKF wiki
- ✅ **v1.0 Knowledge Graph & LLM Auto-Tagging** — Phases 8–12 (shipped 2026-06-24)
- ✅ **v1.1 Hardening & Polish** — Phases 13–17 (shipped 2026-06-27)
- ✅ **v1.2 Graph Schema Extension** — Phase 18 (shipped 2026-06-28)

## Phases

### v1.3 Tag Management UX

- [ ] **Phase 19: Tag Management Backend** — TM-01, TM-02, TM-03, TM-05 — tag rename/merge/delete endpoints riding `SetTagsFrontmatter` + batched single-writer commits; preview→approve flow; incremental graph store update after bulk tag mutations.
- [ ] **Phase 20: Tag Admin UI** — TM-04 — admin panel surface for tag management with tag list, rename/merge/delete dialogs, preview, approve flow, error handling.

---

## Phase 19: Tag Management Backend

**Goal:** Build admin-only tag rename, merge, and delete endpoints that ride the existing byte-stable `SetTagsFrontmatter` + single-writer `EnqueueCommit` path. Each operation follows the preview→approve safety contract. Derived graph store stays consistent after bulk mutations.

**Requirements:** TM-01, TM-02, TM-03, TM-05

### Context

v1.0 delivered per-page LLM tag suggestion and admin bulk sweep. Tags live in YAML frontmatter and in a derived SQLite `page_tags` graph store. There is no way to rename, merge, or delete a tag workspace-wide — changing a tag name requires editing every page manually.

This phase adds three admin endpoints that operate on tags across all pages in one batched commit, using the same `SetTagsFrontmatter` byte-stable builder and the same single-writer commit path that Phase 11 and Phase 12 already rely on.

### Success Criteria

1. `POST /api/v1/admin/tags/rename` — preview + approve tag rename across all pages
2. `POST /api/v1/admin/tags/merge` — preview + approve tag merge (source → target)
3. `POST /api/v1/admin/tags/delete` — preview + approve tag removal from all pages
4. All three endpoints follow the preview→approve pattern (two-step: fetch affected pages, then commit)
5. Single batched commit for all affected pages (N pages → 1 git commit)
6. Tag name normalization: trim, lowercase, reject empty
7. Case-only rename (e.g. "Ops" → "ops") is a no-op
8. Per-page stale revision (409) doesn't sink the batch; response reports per-page status
9. Tag charset validation on input (same as Phase 16 SEC-02)
10. Admin-only RBAC (editor/reader → 403)
11. Audit log entries for all three operations
12. Graph store incrementally updated after each operation (TM-05)
13. Graph store rebuild from disk reproduces identical adjacency after mutations
14. `go test ./internal/graph/... -race` clean after tag mutations

### Key Design Decisions

- **Reuse `SetTagsFrontmatter`** — The byte-stable YAML frontmatter builder from Phase 11. Only the `tags:` region changes. No new write implementation.
- **Reuse batched `EnqueueCommit` with `[]fileWrite` payload** — The Phase 12 pattern. N pages → 1 commit.
- **Preview endpoint returns the set of changes without applying** — Consistent with Phase 12 approve flow.
- **Tag normalization on input** — Trim + lowercase before lookup. Prevents "Ops" and "ops" from being different tags.
- **Conflict: rename to existing tag = merge** — If renaming "deploy" to "ops" and some pages already have "ops", the operation is effectively a merge: "deploy" pages get "ops", "ops" pages keep "ops".
- **Graph store: incremental update** — After the batched commit lands, update `page_tags` in the same request handler (not an async job). The rebuild path is the invariant; incremental update is a performance optimization.

### Key Files

- `internal/server/handlers_tags.go` — New tag management endpoints
- `internal/okf/emit.go` — `SetTagsFrontmatter` (existing, reuse)
- `internal/gitstore/commit.go` — Batched `EnqueueCommit` (existing, reuse)
- `internal/graph/rebuild.go` — Graph rebuild (existing, verify consistency)
- `internal/graph/query.go` — Graph store incremental update after tag mutation
- `internal/graph/extract.go` — `pageTags` extractor (existing, reuse for verification)

### Dependencies

- Phase 11 (Per-Page LLM Tag Suggestion) — `SetTagsFrontmatter` byte-stable builder
- Phase 12 (Bulk Sweep) — batched commit pattern
- Phase 16 (Security Hardening) — tag charset validation (SEC-02)

---

## Phase 20: Tag Admin UI

**Goal:** Admin-only tag management surface in the existing admin panel with tag list, rename/merge/delete dialogs, preview, approve flow, and error handling.

**Requirements:** TM-04

### Context

Phase 19 provides the backend endpoints. This phase builds the React UI that exposes them to admins. The existing admin panel (`/app/admin`) already hosts user management and tag sweep controls — tag management is a natural third section.

### Success Criteria

1. "Tag Management" section in `/app/admin` with tag list (name + page count)
2. Tag list is searchable/filterable
3. "Rename" action per tag → dialog with new name input + preview of affected pages
4. "Merge" action per tag → dialog with target tag picker + preview of affected pages
5. "Delete" action per tag → confirmation dialog showing affected page count
6. Preview shows paginated affected-page list with current tags
7. Approve button with loading/progress state (mirrors Phase 15 UX improvement)
8. Success/error feedback after operation completes
9. Error states: network failure toast, per-page 409 listing, server validation error
10. Tag list refreshes after successful operation
11. Admin-only: non-admin users don't see the Tag Management section
12. `npx tsc --noEmit` clean, `npx vitest run` green

### Key Design Decisions

- **Inline in existing admin panel** — Not a separate route. Consistent with user management and tag sweep sections.
- **Optimistic UI with server confirmation** — Show pending state during preview fetch and during approve. On success, refresh tag list. On failure, show error.
- **Per-page 409 display** — If some pages fail with stale revision, list them explicitly. Don't silently swallow failures.
- **No tag picker in delete** — Delete operates on a single tag. The dialog shows page count and asks for confirmation.

### Key Files

- `web/src/routes/Admin.tsx` — Add Tag Management section
- `web/src/components/admin/TagManagement.tsx` — New component
- `web/src/api/client.ts` — New API client methods for tag endpoints
- `web/src/components/admin/TagRenameDialog.tsx` — Rename dialog with preview
- `web/src/components/admin/TagMergeDialog.tsx` — Merge dialog with preview
- `web/src/components/admin/TagDeleteDialog.tsx` — Delete confirmation dialog

### Dependencies

- Phase 19 (Tag Management Backend) — all three endpoints must exist
- Phase 15 (UX Improvement) — progress/polling pattern

---

## Progress

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
| 13. Visual UAT | v1.1 | 1/1 | Complete | 2026-06-27 |
| 14. Documentation | v1.1 | 2/2 | Complete | 2026-06-27 |
| 15. UX Improvement | v1.1 | 1/1 | Complete | 2026-06-27 |
| 16. Security Hardening | v1.1 | 3/3 | Complete | 2026-06-27 |
| 17. Graph Filter Sidebar | v1.1 | 2/2 | Complete | 2026-06-27 |
| 18. Graph Schema Extension | v1.2 | 1/1 | Complete | 2026-06-28 |
| **19. Tag Management Backend** | **v1.3** | **0** | **Planned** | — |
| **20. Tag Admin UI** | **v1.3** | **0** | **Planned** | — |

---

## Execution Order

- Phase 19 (Backend) must complete before Phase 20 (UI)
- Phase 19 can be decomposed into sub-plans: TM-01 (rename) → TM-02 (merge) → TM-03 (delete) → TM-05 (graph store)
