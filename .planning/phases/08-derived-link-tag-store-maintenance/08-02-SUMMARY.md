---
phase: 08-derived-link-tag-store-maintenance
plan: 02
subsystem: pages
tags: [graph, backlinks, tags, jobs, mutation-wiring, fire-and-forget, drift-rebuild, okf]

# Dependency graph
requires:
  - phase: 08-derived-link-tag-store-maintenance
    provides: "graph.OpenStore/Store, graph.KindGraph, graph.GraphHandler, graph.UpsertPagePayload/DeletePagePayload/RebuildPayload, (*Store).RebuildGraph/StoreHead/DriftCheck/SetRepo/SetGit (08-01)"
  - phase: 03-search
    provides: "the enqueueIndexUpsert/enqueueIndexDelete fire-and-forget pattern + startup search-drift block cloned here"
provides:
  - "cmd/okf-workspace/main.go: graphStore wiring (OpenStore+SetRepo+SetGit) + worker.Register(graph.KindGraph, graph.GraphHandler(...)) + startup graph DriftCheck -> Enqueue(RebuildPayload)"
  - "pages.Service.enqueueGraphUpsert/enqueueGraphDelete (fire-and-forget siblings of the index helpers)"
  - "graph enqueue beside every search enqueue across all mutation methods (Create/Save/CreateFolder/relocate/relocateFolder/deleteWithGroup/restoreInner)"
  - "internal/pages/graphenqueue_test.go: per-mutation-kind freshness integration suite + rename stale-edge case + rebuild cross-check"
affects: [08-03-admin-rebuild-endpoint, phase-09-graph-ui]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Graph enqueue mirrors search enqueue 1:1: every enqueueIndex* call site gains a matching enqueueGraph* sibling with the same op (upsert vs delete)"
    - "Rename/move re-scans every rewritten linker (graph upsert over the SAME writes/byPath set as the index upsert) so inbound edges are rewritten — no stale src/dst rows"
    - "Folder delete + group restore are covered by the per-page deleteWithGroup/restoreInner helpers (single enqueue per page, no double-enqueue in the loop callers)"
    - "Startup graph DriftCheck -> fire-and-forget RebuildPayload mirrors the search drift block; a fresh boot reads as drift and self-heals the empty graph"
    - "Integration test polls page_links/page_tags until the async fire-and-forget job settles (mirrors waitForFile/waitForPayload)"

key-files:
  created:
    - "internal/pages/graphenqueue_test.go"
  modified:
    - "cmd/okf-workspace/main.go"
    - "internal/pages/service.go"
    - "internal/pages/rename.go"
    - "internal/pages/trash.go"

key-decisions:
  - "graphStore is constructed in main.go right after the search index wiring (over st.DB(), SetRepo(contentRepo)+SetGit(gs)); 08-03's admin endpoint can reference this same graphStore + worker for an admin-triggered RebuildPayload"
  - "Save (not Create) is what carries link/tag bodies in the test — Create only scaffolds empty frontmatter — so the test fixture creates then Saves each page with its real body+tags"
  - "Per-page upsert scope means restore re-adds only the restored page's OUTBOUND edges + tags (an inbound edge owned by another page is reconciled by re-saving that page or the rebuild backstop); documented and proven by the DeleteRestore + RebuildCrossCheck tests"

patterns-established:
  - "Every pages mutation enqueues a fire-and-forget graph job beside its search job; a dropped graph enqueue is Warn-logged and swallowed (rebuild backstop reconciles) — a save is never blocked or failed by graph maintenance (T-08-05)"

requirements-completed: [LINK-01]

# Metrics
duration: 14min
completed: 2026-06-24
status: complete
---

# Phase 8 Plan 02: Mutation Wiring + Freshness Tests Summary

**Wired the `KindGraph` job (built in 08-01) into the running system: `main.go` now registers `graph.GraphHandler` on the single jobs worker beside `search.KindIndex` and runs a startup graph-drift rebuild mirroring the search drift block; `pages.Service` gained `enqueueGraphUpsert`/`enqueueGraphDelete` fire-and-forget helpers called beside EVERY existing search enqueue across all eight mutation sites — with rename/move re-scanning every rewritten linker so inbound edges never go stale (pitfall 2 closed) — all proven by a per-mutation-kind freshness integration suite that finishes with a `RebuildGraph` cross-check (incremental adjacency == from-scratch rebuild).**

## Performance
- **Duration:** ~14 min
- **Tasks:** 3
- **Files:** 1 created (the test), 4 modified

## Accomplishments
- **Task 1 (`main.go`):** Constructed `graphStore := graph.OpenStore(st.DB())` + `SetRepo(contentRepo)` + `SetGit(gs)`, registered `worker.Register(graph.KindGraph, graph.GraphHandler(graphStore, contentRepo))` beside the search registration, and added a startup `graphStore.DriftCheck -> worker.Enqueue(graph.KindGraph, graph.RebuildPayload())` block (fire-and-forget, Warn-on-error, never blocks startup) mirroring the search drift block. A fresh install reads as drift (empty `graph_meta` over a populated repo) and self-heals the initial build.
- **Task 2 (enqueue siblings):** Added `enqueueGraphUpsert`/`enqueueGraphDelete` as exact structural clones of the index helpers (fire-and-forget `worker.Enqueue`, Warn-and-swallow), then a graph enqueue beside every existing index enqueue, matching the op:
  - Create / Save / CreateFolder -> `enqueueGraphUpsert`
  - relocate (rename+move) -> `enqueueGraphDelete(old)` + `enqueueGraphUpsert(new)` + `enqueueGraphUpsert(each rewritten linker)` over the same `writes` loop
  - relocateFolder -> per-move `enqueueGraphDelete(old)`+`enqueueGraphUpsert(new)` + `enqueueGraphUpsert(each rewritten byPath page)`
  - deleteWithGroup -> `enqueueGraphDelete` (covers folder delete via the per-page loop)
  - restoreInner -> `enqueueGraphUpsert` (covers group restore via the per-member loop)
  No double-enqueue: `DeleteFolder`/`RestoreGroup` go through the per-page helper exactly once each.
- **Task 3 (integration suite):** `internal/pages/graphenqueue_test.go` stands up a real `pages.Service` over a temp git repo + migrated SQLite + a real `jobs.Worker` with BOTH `KindCommit` and the real `graph.GraphHandler` registered (exactly main.go's wiring). For each mutation kind it drives the mutation, polls `page_links`/`page_tags` until the fire-and-forget job settles, and asserts the adjacency. The rename stale-edge case is explicit (B->A becomes B->A', no `src='a.md'`/`dst='a.md'` rows survive); backlinks are verified as the reverse query on `page_links`; the suite finishes with a `RebuildGraph` cross-check asserting incremental state == from-scratch state.

## Enqueue-site table — fully applied
Confirmed by `grep` (see verification): every `enqueueIndex*` call site has a matching `enqueueGraph*` sibling with the same op. The eight sites: Create, Save, CreateFolder (service.go); relocate delete+upsert+linker-loop, relocateFolder per-move + byPath-loop (rename.go); deleteWithGroup, restoreInner (trash.go).

## Where the graph Store lives (for 08-03)
`graphStore` is constructed in `cmd/okf-workspace/main.go` immediately after the search index wiring block (after `worker.Register(search.KindIndex, ...)`), as `graph.OpenStore(st.DB())` with `SetRepo(contentRepo)`+`SetGit(gs)`. 08-03's admin rebuild endpoint can reference this same `graphStore` + `worker` to enqueue `graph.RebuildPayload()` on demand.

## Deviations from Plan
None — the plan's literal symbol assumptions all matched the real code (`enqueueIndexUpsert`/`enqueueIndexDelete` helper shape, the `relocate` `writes` loop, the `relocateFolder` `byPath`/`movedDestinations` set, `deleteWithGroup`/`restoreInner` single-enqueue points, `graph.OpenStore`/`GraphHandler`/`UpsertPagePayload`/`DeletePagePayload`/`RebuildPayload`/`DriftCheck`). The integration test lives in `package pages` (internal) — no import cycle because `graph` does not import `pages` — so it reuses the existing harness helpers (`newTestRepoAndGit`, `KindCommit`, `CommitHandler`, `waitForFile`, `waitForGone`, `waitForRevisionNonEmpty`) directly; the external `package pages_test` fallback was not needed.

## Issues Encountered
None.

## Self-Check Verification (actual command output)

```
=== CGO_ENABLED=0 go build ./... ===
build exit=0
=== go vet ./cmd/... ./internal/pages/ ===
vet exit=0
=== go test ./internal/pages/ -count=1 ===
ok  	github.com/postfix/okworkspace/internal/pages	4.730s
test exit=0
```

New freshness suite (verbose):
```
--- PASS: TestGraphFreshness_Create
--- PASS: TestGraphFreshness_Save
--- PASS: TestGraphFreshness_RenameStaleEdge
--- PASS: TestGraphFreshness_Move
--- PASS: TestGraphFreshness_DeleteRestore
--- PASS: TestGraphFreshness_Folder
--- PASS: TestGraphFreshness_RebuildCrossCheck
PASS
ok  	github.com/postfix/okworkspace/internal/pages	1.192s
```

## Task Commits
1. **Task 1: Register KindGraph handler + startup-drift rebuild in main.go** — `3348356` (feat)
2. **Task 2: enqueueGraph upsert/delete beside every search enqueue** — `16c8e86` (feat)
3. **Task 3: per-mutation-kind graph freshness integration tests** — `0b4d9d5` (test)

## Next Phase Readiness
- **08-03 (admin rebuild endpoint)** can wire an admin route that enqueues `graph.RebuildPayload()` on the same `worker`, using the `graphStore` already constructed in `main.go`. Drift recovery + the per-mutation freshness path are now live, so the admin endpoint is a manual re-trigger of the same idempotent rebuild.

## Self-Check: PASSED

All 4 modified/created files exist on disk; all 3 task commits (`3348356`, `16c8e86`, `0b4d9d5`) are present in `git log`. `CGO_ENABLED=0 go build ./...`, `go vet ./cmd/... ./internal/pages/`, and `go test ./internal/pages/ -count=1` all passed in this run.

---
*Phase: 08-derived-link-tag-store-maintenance*
*Completed: 2026-06-24*
