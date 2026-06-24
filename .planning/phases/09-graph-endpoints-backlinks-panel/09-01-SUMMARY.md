---
phase: 09-graph-endpoints-backlinks-panel
plan: 01
subsystem: backend-api
tags: [graph, backlinks, sqlite, chi, authed-read, lean-payload, hidden-git]

# Dependency graph
requires:
  - phase: 08-derived-link-tag-store-maintenance
    provides: "internal/graph.Store + page_links/page_tags cache tables + SetRepo/SetGit; the read methods query these tables only"
provides:
  - "internal/graph/query.go — read-only Store methods: GraphData / Neighborhood / Backlinks, plus GraphNode/GraphEdge/GraphData/BacklinkEntry types"
  - "popularTagShare=0.25 / popularTagMinPages=8 named cap constants (hairball prevention)"
  - "GET /api/v1/graph, GET /api/v1/graph/local?path=&depth=, GET /api/v1/graph/backlinks?path= (authed read group)"
  - "internal/server/handlers_graph.go — handleGraph / handleGraphLocal / handleGraphBacklinks"
  - "server.Deps.Graph + authHandlers.graph wiring; main.go Graph: graphStore"
affects: [09-02-backlinks-panel, phase-10-graph-ui]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Lean bipartite typed-edge payload: page + tag nodes (ids+labels+type only, no bodies), page->page link edges + page->tag membership edges; direction carries backlink derivability (no separate backlink edge type)"
    - "Popular-tag cap evaluated against the WHOLE-workspace page/tag counts so neighborhood and global views agree on which tags are capped"
    - "Authed-read handler shape cloned from handleSearch: nil-dependency guard => 500 generic copy, slog.Error detail server-side only, writeJSON 200; hidden-Git generic copy (graphReadUnavailable) avoids index/git/sqlite/bleve tokens"
    - "Backlinks/local as query-param reads (?path=) co-located with /graph, avoiding the /pages/* sibling-wildcard conflict in chi"

key-files:
  created:
    - "internal/graph/query.go"
    - "internal/graph/query_test.go"
    - "internal/server/handlers_graph.go"
  modified:
    - "internal/server/handlers_graph_test.go"
    - "internal/server/router.go"
    - "internal/server/handlers_auth.go"
    - "cmd/okf-workspace/main.go"

key-decisions:
  - "titleFor reads ONLY frontmatter title via the SEC-01 resolver (s.repo), basename fallback, never bodies — the sole file touch in the query layer; nil repo or any read/parse error falls back silently"
  - "Read-endpoint generic copy is a NEW peer const graphReadUnavailable ('The graph is unavailable...') deliberately avoiding the word 'index' so the body carries zero infrastructure vocabulary (the existing graphUnavailable contains 'index' which assertNoGitVocab forbids)"
  - "Popular-tag cap uses global workspace counts (not the restricted neighborhood subset) so a tag's hub-ness is judged once and both views agree"
  - "Neighborhood BFS traverses link adjacency in BOTH directions (UNION of src/dst) so a page's inbound and outbound neighbors both appear; depth clamped to [1,3] in the Store"

requirements-completed: [LINK-02]

# Metrics
duration: 18min
completed: 2026-06-24
status: complete
---

# Phase 9 Plan 01: Graph Read Endpoints + Backlinks Query Summary

**A read-only `internal/graph` query layer (`GraphData`/`Neighborhood`/`Backlinks`) over the Phase-8 `page_links`/`page_tags` cache tables, exposed as three authed reads (`GET /api/v1/graph`, `/graph/local`, `/graph/backlinks`) that return a lean bipartite typed-edge payload (page+tag nodes, link+tag edges, ids+labels only — no bodies), with a named popular-tag cap (`popularTagShare`/`popularTagMinPages`) to prevent hairball hubs and hidden-Git-safe generic error copy.**

## Performance
- **Duration:** ~18 min
- **Tasks:** 2 (Task 1 TDD: RED → GREEN)
- **Files created:** 3; modified: 4

## Accomplishments
- `internal/graph/query.go`: `GraphData(ctx)` (all page + surviving tag nodes, link + tag edges, popular-tag capped), `Neighborhood(ctx, path, depth)` (BFS over undirected link adjacency, depth clamped [1,3], seed always present), `Backlinks(ctx, path)` (reverse `page_links` query with resolved titles, non-nil empty slice). Exported `GraphNode`/`GraphEdge`/`GraphData`/`BacklinkEntry` types; `Nodes`/`Edges`/backlink slices init non-nil so empty workspaces marshal `[]`.
- Named cap constants `popularTagShare = 0.25` and `popularTagMinPages = 8` with hairball-prevention comments; a tag on >25% of pages is excluded from BOTH tag nodes and tag edges once the workspace has ≥8 pages.
- Tag node ids namespaced `tag:<name>` so they can never collide with a page path; tag node label is the bare tag.
- `internal/server/handlers_graph.go`: three authed read handlers mirroring `handleSearch` (nil-guard → 500 generic, slog detail, writeJSON 200); missing `path` → 400; new `graphReadUnavailable` copy carrying no infra vocabulary.
- Routes mounted in the authed read group beside `/tree` + `/search` (any-authed, NOT editor/admin); wiring threaded through `Deps.Graph` → `authHandlers.graph` → `main.go Graph: graphStore` (reuses the existing store).
- Package still does NOT import `internal/search` (08-01 import-independence invariant preserved, verified via `go list -deps`).

## Task Commits
1. **Task 1 (TDD RED): failing graph query tests** — `d603be1` (test)
2. **Task 1 (TDD GREEN): read-only graph query layer** — `8f54285` (feat)
3. **Task 2: authed handlers + routes + wiring** — `456ac0e` (feat)

Plan metadata (STATE.md + ROADMAP.md) committed separately.

## Files Created/Modified
- `internal/graph/query.go` — GraphData/Neighborhood/Backlinks + types + cap constants + titleFor
- `internal/graph/query_test.go` — lean-shape JSON guard, popular-tag cap (above-share excluded / below-min-pages kept), neighborhood depth + clamp + seed, backlinks reverse query
- `internal/server/handlers_graph.go` — handleGraph/handleGraphLocal/handleGraphBacklinks + graphReadUnavailable/graphPathRequired
- `internal/server/handlers_graph_test.go` — extended: read-endpoint 200 lean shape, missing-path 400, nil-dependency 500 with no Git vocab
- `internal/server/router.go` — Deps.Graph field, struct-literal wiring, three authed.Get mounts
- `internal/server/handlers_auth.go` — authHandlers.graph field + graph import
- `cmd/okf-workspace/main.go` — Graph: graphStore in server.New Deps

## Deviations from Plan

### Adjusted (not literal-plan) decisions

**1. [Rule 2 - Correctness] New read-endpoint generic copy instead of reusing `graphUnavailable`**
- **Found during:** Task 2.
- **Issue:** The plan suggested reusing the existing `graphUnavailable` const ("The link index is unavailable..."). That string contains the word "index", which the existing `assertNoGitVocab` test helper explicitly forbids in a client body. Reusing it would have failed the hidden-Git assertion the plan itself mandates.
- **Fix:** Added a peer const `graphReadUnavailable = "The graph is unavailable. Try again in a moment."` (plus `graphPathRequired` for the 400 case) carrying zero infrastructure vocabulary.
- **Files modified:** internal/server/handlers_graph.go.
- **Commit:** `456ac0e`.

**2. [Adaptation] Extended the existing `handlers_graph_test.go` rather than creating a new file**
- The Phase-8 reindex test already occupied `internal/server/handlers_graph_test.go` with a `graphFixture`/`newGraphServer` harness. The new read tests extend that fixture (added `repo`/`store` fields, `seedAndRebuild`, `authedGet`) instead of duplicating a second harness — matches the plan's "reuse the existing server test harness" instruction.

The plan's symbol assumptions otherwise matched the real code (Store.SetRepo/SetGit, RebuildGraph, okf.Parse/Field/FieldTitle/HasFrontmatter, repo.Read, handleSearch nil-guard pattern, the authed-group mount point).

**Total deviations:** 1 correctness fix (error-copy token) + 1 harness adaptation. No scope creep, no new dependency.

## Threat Mitigations Applied
- **T-09-01 (info disclosure):** lean payload — nodes carry id+label+type only; both a Store-level JSON guard test and an HTTP-level body check assert no `body`/`frontmatter`.
- **T-09-02 (hidden-Git):** all handler errors return `graphReadUnavailable` (no git/sqlite/bleve/index tokens); nil-dependency test asserts via `assertNoGitVocab`.
- **T-09-03 (injection):** all SQL is parameterized (`?` binds); depth is `strconv.Atoi`-parsed and clamped [1,3]; titles read through the SEC-01 resolver, never `os.*`.
- **T-09-04 (DoS/hairball):** popular-tag cap (named constants) proven by test; local endpoint depth-clamped ≤3.
- **T-09-SC:** no new Go dependency added.

## Self-Check Verification (actual command output)

```
### CGO-free build ###    CGO_ENABLED=0 go build ./...               → build exit=0
### go vet ###            go vet ./internal/graph/ ./internal/server/ → vet exit=0
### go test ###           ok internal/graph ; ok internal/server
### import guard ###      go list -deps ./internal/graph | grep internal/search → OK no search import
### route count ###       grep -c 'authed.Get("/graph' router.go      → 3
### graph read tests ###  TestGraphReadEndpoints/MissingPath/NilDependency → PASS
### query tests ###       TestGraphData_BipartiteLeanShape/PopularTagCap/CapDisabledBelowMinPages,
                          TestNeighborhood_Depth1/DepthClamp/SeedAlwaysPresent,
                          TestBacklinks_ReverseQuery → all PASS
```

## Next Plan Readiness (09-02 + Phase 10)
Exact exported names the frontend backlinks panel (09-02) and the Phase-10 graph UI wire against:
- `GET /api/v1/graph` → `{ "nodes": [{id,label,type}], "edges": [{source,target,type}] }`, type ∈ {page,tag}/{link,tag}.
- `GET /api/v1/graph/local?path=<page>&depth=<1..3>` → same shape (depth default 1, clamped ≤3).
- `GET /api/v1/graph/backlinks?path=<page>` → `[{ "path": string, "title": string }]` (empty array for none).
All are any-authed reads; tag node ids are namespaced `tag:<name>`.

## Self-Check: PASSED

All 3 created files + 4 modified files exist; all 3 task commits (`d603be1`, `8f54285`, `456ac0e`) are present in `git log`.

## Gap Closure (2026-06-24)

**Gap (live verification):** `GET /api/v1/graph` omitted ORPHAN pages. `GraphData`'s
node set was built only from `page_links` (src+dst) and `page_tags`, so a page with
no links and no tags never appeared as a node. Live proof: 127 `.md` files on disk
but the endpoint returned only 2 nodes (the single linked pair). This broke the
Phase-9 "returns ALL pages as nodes" criterion and Phase-10 GRAPH-02 orphan visibility.

**Fix (`internal/graph/query.go`):** `GraphData`'s page-node set is now the UNION of
(a) every live page on disk — enumerated from the repo Tree via the new `livePages()`
helper using the SAME skip rules as `RebuildGraph` (skip dirs, non-`.md`, trashed) —
and (b) every page referenced by the cache tables (belt-and-suspenders). `livePages()`
reads NO bodies (path enumeration only), preserving the lean request-path invariant
(titles remain the sole file touch). A nil repo or `Tree()` error degrades gracefully
to the cache-derived set; the server path wires the repo (`main.go` `SetRepo`), so
production returns all live pages. `Neighborhood` (anchor traversal) and `Backlinks`
are unchanged; the popular-tag cap and lean payload are intact.

**Test:** added `TestGraphData_OrphanPageIsNode` (`internal/graph/query_test.go`) —
seeds a linked pair plus a true orphan (no links, no tags) and asserts the orphan
appears as a `type:"page"` node with its title label and zero edges.

**Verification (actually run):**
- `CGO_ENABLED=0 go build ./...` → exit 0
- `go test ./internal/graph/ ./internal/server/` → both `ok` (incl. new orphan test)

---
*Phase: 09-graph-endpoints-backlinks-panel*
*Completed: 2026-06-24*
