---
phase: 08-derived-link-tag-store-maintenance
verified: 2026-06-24T09:00:00Z
status: human_needed
score: 12/12 must-haves verified
behavior_unverified: 0
overrides_applied: 0
human_verification:
  - test: "Navigate to Admin route as admin user in a live browser; confirm a 'Rebuild graph index' section appears below the 'Rebuild search index' section with a working button that fires the POST and displays the 'Graph index rebuild started.' notice"
    expected: "Button renders, click triggers the POST /api/v1/admin/graph/reindex, notice appears, no error shown, no Git/Bleve/index vocabulary visible to the user"
    why_human: "Admin.tsx rendering and mutation feedback are wired and unit-tested; visual layout and actual browser HTTP round-trip require a live session"
---

# Phase 8: Derived Link/Tag Store Maintenance Verification Report

**Phase Goal:** A derived, rebuildable-from-files adjacency store (page links, backlinks, tag membership) exists and stays fresh on every page mutation — the foundation both the graph UI and tag-vocabulary prompting depend on.
**Verified:** 2026-06-24T09:00:00Z
**Status:** human_needed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | Page outbound internal .md links stored as forward edges in page_links (src->dst), skipping external/absolute/dangling | VERIFIED | `internal/graph/extract.go` `outboundLinks()` reuses `okf.FindLinks`, resolves with `path.Clean(path.Join(...))`, skips external via `isAbsoluteOrExternal`, drops dangling via `exists()`. `TestDerivedOnly_RebuildReproducesAdjacency` PASS |
| 2 | Backlinks are the reverse query on page_links (no separate table) | VERIFIED | Migration `0009_graph.sql` defines only `page_links` + `page_tags` + `graph_meta`; idx_page_links_dst index exists for O(1) backlink lookups. `TestGraphFreshness_RenameStaleEdge` asserts via SELECT src_path WHERE dst_path=? |
| 3 | Page frontmatter tags stored as raw membership rows in page_tags (same as search.readTags) | VERIFIED | `pageTags()` in `extract.go` re-implements the sequence-aware yaml.Node walk identical to `search.readTags`; `extract_test.go` cross-checks parity. `TestDerivedOnly_RebuildReproducesAdjacency` validates tag rows |
| 4 | Delete both tables + RebuildGraph reproduces byte-identical adjacency (SQLite never source of truth) | VERIFIED | `TestDerivedOnly_RebuildReproducesAdjacency` PASS (explicit: nuke both tables, rebuild, assert `reflect.DeepEqual` on sorted snapshot). `CGO_ENABLED=1 go test ./internal/graph/ -race` PASS |
| 5 | KindGraph upsert atomically replaces only the src_path/page_path rows; delete removes src_path OR dst_path rows | VERIFIED | `upsertPage()` in `graphjob.go`: one tx deletes WHERE src_path=path then re-inserts. `deletePage()`: WHERE src_path=path OR dst_path=path. `TestUpsertReplacesOnlyPageRows` and `TestDeletePageRemovesInboundAndOutbound` PASS |
| 6 | Concurrent KindGraph upsert/delete/rebuild on single drain goroutine is -race clean | VERIFIED | `TestGraph_ConcurrentReadWrite` PASS under `CGO_ENABLED=1 go test ./internal/graph/ -race`. No race, panic, or deadlock |
| 7 | KindGraph handler registered on single worker at startup; startup-drift rebuild self-heals store | VERIFIED | `cmd/okf-workspace/main.go`: `worker.Register(graph.KindGraph, graph.GraphHandler(graphStore, contentRepo))` + `graphStore.DriftCheck(...)` -> fire-and-forget Enqueue confirmed at lines 249, 362-370 |
| 8 | Every mutation kind enqueues graph job beside search enqueue (create/save/folder-create/rename/move/linker-rescan/delete/restore) | VERIFIED | `service.go`: enqueueGraphUpsert at create(168), save(254), createFolder(294). `rename.go`: delete(old)+upsert(new)+upsert(rewritten linkers) at lines 315-316-326 (relocateFolder) and 491-492-503 (relocate). `trash.go`: delete at 131, upsert at 411. `TestGraphFreshness_*` passes for all kinds |
| 9 | Rename/move rewrites inbound edges (no stale src/dst rows) | VERIFIED | `TestGraphFreshness_RenameStaleEdge` PASS — explicitly asserts 0 rows referencing old path after rename; `TestGraphFreshness_Move` PASS. Linker re-scan: rename.go line 503 upserts each rewritten linker page |
| 10 | Admin POST /api/v1/admin/graph/reindex returns 202 (admin) / 403 (non-admin), action audited | VERIFIED | `TestGraphReindexAdminOnly` PASS: editor→403, admin→202, audit row `graph_reindex` logged. `handleGraphReindex` in `handlers_search.go`; route in admin RequireRole(admin) subgroup in `router.go:242` |
| 11 | No user-reachable copy names Bleve/Git/HEAD/reindex in graph rebuild affordance | VERIFIED | `graphUnavailable = "The link index is unavailable. Try again in a moment."`, audit Detail="rebuild graph index". `assertNoGitVocab` on admin 202 response PASS. Admin.tsx uses "Rebuild graph index" / "Graph index rebuild started." — no forbidden terms found by grep |
| 12 | Admin.tsx "Rebuild graph index" button wired to reindexGraph() api fn with notice/error states | VERIFIED | `Admin.tsx` imports `reindexGraph`, has `reindexGraphMut`, renders button at line 277-290. `client.ts` `reindexGraph()` POSTs `/api/v1/admin/graph/reindex`. `client.test.ts` test PASS (7 tests). `npx tsc -b` PASS |

**Score:** 12/12 truths verified (0 present-but-behavior-unverified)

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|-------------|-------------|--------|----------|
| LINK-01 | 08-01, 08-02 | System maintains derived page-link/backlink index, kept fresh on every page mutation | SATISFIED | `internal/graph` package: Store, KindGraph, GraphHandler, RebuildGraph, extract.go. Mutation freshness proven by 7 TestGraphFreshness_* tests. Derived-only invariant test PASS |
| LINK-03 | 08-03 | Admin can rebuild link/graph index from files (recovery backstop) | SATISFIED | POST /api/v1/admin/graph/reindex gated by RequireRole(admin) + nosurf CSRF, audited as ActionGraphReindex, returns 202 fire-and-forget. TestGraphReindexAdminOnly PASS |

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/store/migrations/0009_graph.sql` | page_links, page_tags, graph_meta tables + indexes | VERIFIED | All 3 tables with IF NOT EXISTS; idx_page_links_dst + idx_page_tags_tag indexes present |
| `internal/graph/store.go` | Store type, OpenStore, SetRepo, SetGit, DriftCheck, StoreHead | VERIFIED | All exported symbols present; headProvider interface local (no search import) |
| `internal/graph/graphjob.go` | KindGraph, GraphHandler, UpsertPagePayload, DeletePagePayload, RebuildPayload, upsertPage, deletePage | VERIFIED | All symbols present; defer-recover() in handler; fire-and-forget Enqueue contract |
| `internal/graph/extract.go` | outboundLinks reusing okf.FindLinks; pageTags mirroring search.readTags | VERIFIED | `okf.FindLinks` called at line 30; pageTags re-implements yaml.Node walk; no internal/search import |
| `internal/graph/rebuild.go` | RebuildGraph from-files walk, StoreHead at end | VERIFIED | Walks repo.Tree(), accumulates edges+tags, one tx DELETE+INSERT, StoreHead last |
| `internal/graph/rebuild_test.go` | Derived-only, incremental==rebuild, upsert-replaces-only, delete, handler tests | VERIFIED | TestDerivedOnly_RebuildReproducesAdjacency, TestIncrementalEqualsRebuild, TestUpsertReplacesOnlyPageRows, TestDeletePageRemovesInboundAndOutbound, TestGraphHandler_RecoversPanic, TestGraphHandler_Dispatch PASS |
| `internal/graph/concurrency_test.go` | CR-01 concurrent test under -race | VERIFIED | TestGraph_ConcurrentReadWrite PASS with -race |
| `cmd/okf-workspace/main.go` | graph.KindGraph registered, SetRepo/SetGit wired, startup drift block | VERIFIED | Lines 246-249 (store+register), 362-370 (drift block) confirmed |
| `internal/pages/service.go` | enqueueGraphUpsert/enqueueGraphDelete helpers + create/save/folder-create calls | VERIFIED | Helpers at lines 354-366; call sites at 168, 254, 294 |
| `internal/pages/rename.go` | delete(old)+upsert(new)+upsert(linkers) for relocate + relocateFolder | VERIFIED | 6 enqueueGraph* calls at lines 315-316-326 and 491-492-503 |
| `internal/pages/trash.go` | delete in deleteWithGroup, upsert in restoreInner | VERIFIED | Lines 131 (delete), 411 (upsert) |
| `internal/pages/graphenqueue_test.go` | Per-mutation-kind freshness integration tests | VERIFIED | 7 TestGraphFreshness_* tests all PASS |
| `internal/audit/audit.go` | ActionGraphReindex = "graph_reindex" | VERIFIED | Constant at line 54; confirmed by audit test output in TestGraphReindexAdminOnly |
| `internal/server/handlers_search.go` | handleGraphReindex, graphEnqueuer interface | VERIFIED | handleGraphReindex at line 92; graphEnqueuer interface; graphUnavailable const |
| `internal/server/router.go` | POST /admin/graph/reindex in admin subgroup, GraphJobs dep field | VERIFIED | GraphJobs field at line 56; route at line 242 inside RequireRole(admin) group |
| `internal/server/handlers_graph_test.go` | TestGraphReindexAdminOnly (202/403 + assertNoGitVocab) | VERIFIED | PASS; assertNoGitVocab called on admin 202 response at line 126 |
| `web/src/api/client.ts` | reindexGraph() POSTing /api/v1/admin/graph/reindex | VERIFIED | Function at lines 1032-1034; client.test.ts assertion PASS |
| `web/src/routes/Admin.tsx` | Rebuild graph index button with notice/error states | VERIFIED | Button at lines 277-290; reindexGraphMut + state vars present |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| `internal/graph/extract.go` | `internal/okf/links.go` | `okf.FindLinks(body)` + path.Clean(path.Join) resolution | VERIFIED | `okf.FindLinks` called at extract.go:30 |
| `internal/graph/extract.go` | (search.readTags parity) | `pageTags()` re-implements yaml.Node walk with same FieldTags key | VERIFIED | Same yaml.SequenceNode/ScalarNode walk; no search import |
| `internal/graph/graphjob.go` | `internal/jobs/worker.go` | `jobs.Handler` function type returned by GraphHandler | VERIFIED | GraphHandler returns `jobs.Handler` at graphjob.go:55 |
| `internal/pages/service.go` | `internal/graph/graphjob.go` | `worker.Enqueue(ctx, graph.KindGraph, graph.UpsertPagePayload(path))` | VERIFIED | Lines 354-358 in service.go |
| `cmd/okf-workspace/main.go` | `internal/graph/rebuild.go` | startup DriftCheck -> Enqueue(graph.KindGraph, graph.RebuildPayload()) | VERIFIED | Lines 362-370 in main.go |
| `internal/pages/rename.go` | `internal/graph/graphjob.go` | delete(old)+upsert(new)+upsert(rewritten linkers) in relocate/relocateFolder | VERIFIED | Lines 315-316-326 and 491-492-503 in rename.go |
| `internal/server/router.go` | `internal/server/handlers_search.go` | `admin.Post("/admin/graph/reindex", h.handleGraphReindex)` in RequireRole(admin) group | VERIFIED | router.go line 242 inside RequireRole(admin) subgroup |
| `internal/server/handlers_search.go` | `internal/graph/graphjob.go` | `h.graphJobs.Enqueue(ctx, graph.KindGraph, graph.RebuildPayload())` | VERIFIED | handlers_search.go line 97 |
| `cmd/okf-workspace/main.go` | `internal/server/router.go` | `server.Deps{GraphJobs: worker}` passing the same single worker | VERIFIED | main.go line 387; GraphJobs: worker |
| `web/src/routes/Admin.tsx` | `web/src/api/client.ts` | `reindexGraph()` called from reindexGraphMut useMutation | VERIFIED | Admin.tsx line 160; client.ts function at 1032 |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Derived-only invariant: delete tables + rebuild = identical adjacency | `go test ./internal/graph/ -run TestDerivedOnly_RebuildReproducesAdjacency -v` | PASS (0.01s) | PASS |
| Race-clean concurrent upsert/delete/rebuild | `CGO_ENABLED=1 go test ./internal/graph/ -race -run TestGraph_Concurrent` | ok (3.312s) | PASS |
| Admin 202 / editor 403 on graph reindex | `go test ./internal/server/ -run TestGraphReindexAdminOnly -v` | PASS: editor=403, admin=202, audit action=graph_reindex logged | PASS |
| Per-mutation freshness (all 7 kinds) | `go test ./internal/pages/ -run TestGraph -v` | 7/7 PASS including RenameStaleEdge, RebuildCrossCheck | PASS |
| Full graph+pages+server+audit test suite | `go test ./internal/graph/ ./internal/pages/ ./internal/server/ ./internal/audit/` | all PASS | PASS |
| Frontend typecheck | `cd web && npx tsc -b` | exit 0 | PASS |
| Frontend vitest (290 tests, incl. reindexGraph) | `cd web && npx vitest run` | 33 test files, 290 tests PASS | PASS |
| CGO-free build | `CGO_ENABLED=0 go build ./...` | exit 0, no output | PASS |

### Requirements Coverage

Both requirement IDs from the plan frontmatter are fully covered:

- **LINK-01** (Phase 8, Complete per REQUIREMENTS.md): The derived adjacency store (`page_links`, `page_tags`) exists as a rebuildable SQLite cache, kept fresh by KindGraph job enqueues on every mutation kind. The from-files invariant is proven by automated test.
- **LINK-03** (Phase 8, Complete per REQUIREMENTS.md): Admin can rebuild the link/graph index from files via POST /api/v1/admin/graph/reindex, consistent with the existing "Rebuild search index" affordance. RBAC (admin-only), CSRF (nosurf), audit (ActionGraphReindex), and hidden-Git vocabulary rules all verified by automated test.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| None found | — | No TBD/FIXME/XXX/PLACEHOLDER in any phase-modified file | — | — |

The `grep -rn "TODO"` on graph package and test files returned no results. Warning-level comments in Admin.tsx ("Hidden-Git rule: the label says 'Rebuild graph index'...") are implementation notes in code comments, not code-quality debt.

### Human Verification Required

#### 1. Admin "Rebuild graph index" button — visual render and live browser round-trip

**Test:** Log in as admin in a live browser session, navigate to the Admin page (/admin or the admin route in the SPA). Locate the maintenance section.

**Expected:** A "Graph" maintenance section appears below the "Search" section, containing a "Rebuild graph index" button. Clicking it fires the POST /api/v1/admin/graph/reindex, briefly disables the button with "Starting…", then shows "Graph index rebuild started." notice. No error is shown. No "Reindex", "Bleve", "Git", "HEAD", or other internal vocabulary is visible to the user.

**Why human:** `Admin.tsx` rendering, useMutation feedback, and the live HTTP round-trip require a browser session. The wiring (component, api fn, backend handler, RBAC, CSRF, response) is fully verified by automated tests; only the visual presentation and end-to-end browser UX cannot be verified by grep or unit tests.

---

## Summary

Phase 8 achieves its goal. The derived link/tag adjacency store is fully implemented:

- **Data layer (LINK-01):** Migration `0009_graph.sql` creates `page_links`, `page_tags`, `graph_meta`. The `internal/graph` package exposes `Store`, `KindGraph`, `GraphHandler`, `RebuildGraph`, all payload builders, and `DriftCheck`/`StoreHead`. Extraction reuses `okf.FindLinks` and mirrors `search.readTags` (no separate scanner, no search import). The from-files derived-only invariant is proven by `TestDerivedOnly_RebuildReproducesAdjacency`. The concurrency discipline is proven by `-race` clean `TestGraph_ConcurrentReadWrite`.

- **Mutation freshness (LINK-01):** Graph enqueue siblings exist beside every search enqueue across all mutation kinds (create, save, folder-create, rename, move, linker-rescan, delete-to-trash, restore, folder-delete). The rename/move stale-edge pitfall is explicitly closed and proven by `TestGraphFreshness_RenameStaleEdge`. Startup drift recovery is wired in `main.go`. All 7 `TestGraphFreshness_*` integration tests PASS.

- **Admin rebuild affordance (LINK-03):** POST `/api/v1/admin/graph/reindex` is admin-RBAC-gated, CSRF-protected, audited as `ActionGraphReindex`, returns 202 fire-and-forget, and carries no hidden-Git vocabulary. The "Rebuild graph index" button in `Admin.tsx` is wired to `reindexGraph()` in `client.ts` with notice/error states. `TestGraphReindexAdminOnly` PASS (editor→403, admin→202, assertNoGitVocab PASS). Frontend: `tsc -b` clean, all 290 vitest tests PASS.

One human verification item remains: the Admin page button visual presentation and live browser round-trip.

---

_Verified: 2026-06-24T09:00:00Z_
_Verifier: Claude (gsd-verifier)_
