---
phase: 08-derived-link-tag-store-maintenance
plan: 03
subsystem: server
tags: [graph, admin, reindex, rbac, csrf, audit, hidden-git, link-03, okf]

# Dependency graph
requires:
  - phase: 08-derived-link-tag-store-maintenance
    provides: "graph.KindGraph, graph.RebuildPayload, graph.OpenStore/Store, graph.GraphHandler (08-01); graphStore + single jobs worker constructed in main.go (08-02)"
  - phase: 03-search
    provides: "handleReindex + searchEnqueuer + the admin /admin/search/reindex affordance + Admin.tsx Rebuild-search button cloned here"
provides:
  - "POST /api/v1/admin/graph/reindex — admin-only fire-and-forget from-files graph rebuild (202)"
  - "internal/server: graphEnqueuer interface + (*authHandlers).handleGraphReindex + authHandlers.graphJobs + server.Deps.GraphJobs"
  - "audit.ActionGraphReindex (\"graph_reindex\")"
  - "web: reindexGraph() api fn + Admin.tsx Rebuild graph index button (notice/error states)"
  - "internal/server/handlers_graph_test.go: TestGraphReindexAdminOnly (202 admin / 403 editor, no hidden-Git vocab)"
affects: [phase-09-graph-ui]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Admin graph rebuild is a 1:1 structural clone of the search rebuild: handleGraphReindex mirrors handleReindex (nil-guard -> 500 generic copy, Enqueue fire-and-forget, audited non-fatally, 202), registered in the SAME RequireRole(admin) subgroup beside /admin/search/reindex"
    - "Separate named graphEnqueuer interface (identical shape to searchEnqueuer) so the graph handler does not couple to the search enqueuer field; main.go passes the SAME single worker as both SearchJobs and GraphJobs (no second store/worker)"
    - "Hidden-Git copy: generic graphUnavailable const + audit Detail \"rebuild graph index\" + button label \"Rebuild graph index\" name no Bleve/Git/index/Reindex term; assertNoGitVocab guards the response body in the handler test"

key-files:
  created:
    - "internal/server/handlers_graph_test.go"
  modified:
    - "internal/audit/audit.go"
    - "internal/server/handlers_search.go"
    - "internal/server/handlers_auth.go"
    - "internal/server/router.go"
    - "cmd/okf-workspace/main.go"
    - "web/src/api/client.ts"
    - "web/src/routes/Admin.tsx"
    - "web/src/api/client.test.ts"

key-decisions:
  - "Defined a distinct graphEnqueuer interface (not reuse of searchEnqueuer) per the plan, so the graph handler/field is decoupled from the search enqueuer — both interfaces are the same shape and main.go satisfies both with the one jobs.Worker"
  - "graphUnavailable copy references the link affordance (\"The link index is unavailable...\") rather than reusing searchUnavailable, keeping the user-facing copy about the graph/link rebuild while still leaking no Git/Bleve/index vocabulary"
  - "The handler test stands up a real graph.Store + jobs.Worker with graph.GraphHandler registered (no search.Index needed), so the 202 path exercises the real enqueue end to end rather than a stub"

patterns-established:
  - "A derived-store admin rebuild affordance = clone the search-rebuild quartet (handler+interface, audit action, admin-subgroup route, Admin.tsx section), reuse the existing single worker as the enqueuer dep, and gate it by assertNoGitVocab"

requirements-completed: [LINK-03]

# Metrics
duration: 9min
completed: 2026-06-24
status: complete
---

# Phase 8 Plan 03: Admin Rebuild Graph Index (LINK-03) Summary

**Cloned the shipped "Rebuild search index" affordance for the derived link/tag graph store (08-01/08-02): a POST `/api/v1/admin/graph/reindex` endpoint that enqueues `graph.RebuildPayload()` on the SAME single jobs worker fire-and-forget (returning 202, never blocking), an `ActionGraphReindex` audit record, the route registered inside the existing `RequireRole(admin)` subgroup (RBAC from the session role + nosurf CSRF, never client input), and an Admin.tsx "Rebuild graph index" button with notice/error states — proven admin-only (202/403) and hidden-Git-clean by a server test exercising the real enqueue path. No second graph store/worker is constructed; no new dependency is added.**

## Performance
- **Duration:** ~9 min
- **Tasks:** 3
- **Files:** 1 created (the handler test), 8 modified

## Accomplishments
- **Task 1 (backend wiring):**
  - `audit.ActionGraphReindex = "graph_reindex"` added beside `ActionSearchReindex`.
  - `internal/server/handlers_search.go`: added a `graphEnqueuer` interface (same shape as `searchEnqueuer`), a `graphUnavailable` generic copy const, and `handleGraphReindex` — a structural clone of `handleReindex`: nil-`graphJobs` -> 500 generic copy; `h.graphJobs.Enqueue(r.Context(), graph.KindGraph, graph.RebuildPayload())` fire-and-forget with slog-only error detail -> 500 on enqueue error; audited non-fatally (`Action: audit.ActionGraphReindex`, actor from `auth.CurrentUser`, `Detail: "rebuild graph index"`, `Source: "web-ui"`); `writeJSON(w, 202, {"ok": true})` on success.
  - `internal/server/handlers_auth.go`: `graphJobs graphEnqueuer` field added to `authHandlers`.
  - `internal/server/router.go`: `GraphJobs graphEnqueuer` added to `Deps`, `graphJobs: deps.GraphJobs` wired into the handler literal, and `admin.Post("/admin/graph/reindex", h.handleGraphReindex)` registered immediately after the search reindex route in the `RequireRole(auth.RoleAdmin)` subgroup (inherits the same RBAC + global nosurf CSRF).
  - `cmd/okf-workspace/main.go`: `GraphJobs: worker` added beside `SearchJobs: worker` — the same single worker already has `graph.KindGraph` registered (08-02), so no second store/worker is built.
- **Task 2 (handler test):** `internal/server/handlers_graph_test.go` (`package server_test`) stands up a temp migrated store, a real content repo + gitstore, a real `graph.Store` (`OpenStore`+`SetRepo`+`SetGit`), a real `jobs.Worker` with `graph.GraphHandler` registered for `graph.KindGraph`, and `server.New(Deps{..., GraphJobs: w})`. `TestGraphReindexAdminOnly` proves editor -> 403, bootstrap admin -> 202, and runs `assertNoGitVocab` on the admin body. Reuses the shared `loginAs`/`doMutate`/`loginAsAdmin`/`assertNoGitVocab` helpers (no redefinitions).
- **Task 3 (frontend):** `reindexGraph(): Promise<void>` added to `web/src/api/client.ts` (clone of `reindexSearch`, POSTing `/api/v1/admin/graph/reindex` via the shared CSRF-protected `mutate`). `Admin.tsx` imports it, adds `graphReindexNotice`/`graphReindexError` state + `reindexGraphMut`, and renders a "Graph" maintenance `<section>` cloning the Search one — label `"Rebuild graph index"` (never "Reindex"/"Bleve"/Git), reusing the existing CSS classes. `client.test.ts` asserts `reindexGraph` hits the graph route and `reindexSearch` the search route (both resolve on 202).

## Endpoint / symbol reference (for Phase 9)
- Route: **POST `/api/v1/admin/graph/reindex`** (admin subgroup; `RequireRole(admin)` + nosurf CSRF).
- Handler: `(*authHandlers).handleGraphReindex` (`internal/server/handlers_search.go`).
- Interface/dep: `graphEnqueuer` + `authHandlers.graphJobs` + `server.Deps.GraphJobs` (the single jobs worker).
- Audit action: `audit.ActionGraphReindex` = `"graph_reindex"`.
- Frontend: `reindexGraph()` (`web/src/api/client.ts`) + the "Rebuild graph index" button (`web/src/routes/Admin.tsx`).

## Deviations from Plan
None. Every literal symbol assumption in the plan matched the real code (handleReindex/searchEnqueuer/searchUnavailable shape; authHandlers.searchJobs field; Deps.SearchJobs + handler-literal wiring; the admin-subgroup `/admin/search/reindex` registration; the audit action const block; the main.go `server.Deps{}` literal passing `SearchJobs: worker`; graph.OpenStore/SetRepo/SetGit/KindGraph/RebuildPayload/GraphHandler; the Admin.tsx reindex section + reindexSearch api fn). The graph store constructor is `OpenStore`+`SetRepo`+`SetGit` (no `SetDB` — the DB is passed to `OpenStore`), exactly as 08-02's main.go wiring records.

## Threat surface
No new security surface beyond the threat register: the new POST is gated by the same session-role RBAC + nosurf CSRF as the search reindex (T-08-08/09), the enqueue is fire-and-forget so it cannot block the request (T-08-10), the response/audit copy is hidden-Git-clean and guarded by `assertNoGitVocab` (T-08-11), and the action is audited with the acting admin's id (T-08-12). No new dependency was installed (T-08-SC).

## Issues Encountered
None.

## Self-Check Verification (actual command output)

Backend:
```
=== CGO_ENABLED=0 go build ./... ===
build exit=0
=== go vet ./internal/server/ ./internal/audit/ ===
vet exit=0
=== go test ./internal/server/ ./internal/audit/ -count=1 ===
ok  	github.com/postfix/okworkspace/internal/server	3.865s
ok  	github.com/postfix/okworkspace/internal/audit	0.015s
```

New handler test (verbose):
```
=== RUN   TestGraphReindexAdminOnly
audit action=graph_reindex actor=1 detail="rebuild graph index"
--- PASS: TestGraphReindexAdminOnly (0.10s)
PASS
ok  	github.com/postfix/okworkspace/internal/server	0.140s
```

Frontend (web/):
```
=== npx tsc -b ===
tsc exit=0
=== npx vitest run src/api/client.test.ts ===
 ✓ src/api/client.test.ts (7 tests) 38ms
 Test Files  1 passed (1)
      Tests  7 passed (7)
```

## Task Commits
1. **Task 1: admin graph reindex endpoint + ActionGraphReindex + GraphJobs wiring** — `229ee58` (feat)
2. **Task 2: admin-only graph reindex handler test (202/403, no hidden-Git vocab)** — `5762987` (test)
3. **Task 3: reindexGraph api fn + Rebuild graph index Admin button** — `20c912c` (feat)

## Next Phase Readiness
- **LINK-03 is fully covered:** an admin can rebuild the link/graph index from files via an affordance consistent with the existing "Rebuild search index" button. The endpoint reuses the 08-02 graph Store + single worker (no second store) and adds no new dependency. Phase 09 (graph UI) can rely on the derived store being admin-rebuildable on demand in addition to the per-mutation freshness + startup-drift self-heal already live.
- **This is the final plan of phase 08** — all three LINK requirements (LINK-01 08-02, LINK-02 graph store 08-01, LINK-03 here) are now delivered.

## Self-Check: PASSED

All 9 modified/created files exist on disk; all 3 task commits (`229ee58`, `5762987`, `20c912c`) are present in `git log`. `CGO_ENABLED=0 go build ./...`, `go vet`, `go test ./internal/server/ ./internal/audit/`, `npx tsc -b`, and `npx vitest run src/api/client.test.ts` all passed in this run.

---
*Phase: 08-derived-link-tag-store-maintenance*
*Completed: 2026-06-24*
