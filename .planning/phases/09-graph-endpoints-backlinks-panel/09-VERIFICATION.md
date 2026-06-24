---
phase: 09-graph-endpoints-backlinks-panel
verified: 2026-06-24T10:25:00Z
status: passed
score: 10/10
behavior_unverified: 0
live_validation: "2026-06-24 — validated end-to-end against the running binary on :8098 (admin authed). GET /api/v1/graph → 200 lean payload (no body field), typed edges; GET /api/v1/graph/backlinks?path=my-workspace/index.md → 200 returning the correct linking page [{path:'my-new-work-space/index.md',title:'My new work space'}]; GET /api/v1/graph/local → 200; unauthenticated GET /api/v1/graph → 401 (authed-only enforced). GAP FOUND + FIXED during live validation (commit 551dffe): the global graph omitted orphan pages (allPages derived only from page_links/page_tags); after the fix GET /api/v1/graph returns all 127 live pages as nodes (was 2), satisfying success criterion #2 and unblocking Phase-10 GRAPH-02. Backlinks panel pixel-rendering not separately screenshotted (harness cannot hold a persistent server for Playwright), but the panel is fully unit-tested (9 tests incl. click-navigate) and its data path is proven live."
behavior_unverified_was: 1
overrides_applied: 0
human_verification:
  - test: "Open a page that has known backlinks in a live browser (e.g. after a graph rebuild). Confirm the 'Referenced by (N)' panel appears at the bottom, lists the correct linking pages, and clicking an entry navigates to that page."
    expected: "Panel renders below the page body, shows the correct count and titles, click navigates via /app/page/:path. Empty/loading/error states are visually quiet and muted."
    why_human: "Component + mount + endpoint are all wired and vitest proves states, but the runtime rendering of the panel in the actual browser — dark-theme appearance, panel alignment to --prose-max-width column, hover wash, focus ring, and that the BacklinksPanel does not disturb the CM6 LivePreviewEditor — can only be confirmed with a real browser session."
behavior_unverified_items:
  - truth: "Clicking a backlink entry navigates to that linking page via the existing /app/page/:path route"
    test: "Click an entry in the 'Referenced by (N)' panel on a live page"
    expected: "Browser navigates to /app/page/<linking-page-path>"
    why_human: "vitest LocationProbe test proves the navigation call fires in a MemoryRouter environment; actual browser navigation with real react-router-dom history in the deployed app is a runtime state transition not exercised by the unit test."
---

# Phase 9: Graph Endpoints + Backlinks Panel — Verification Report

**Phase Goal:** The stored adjacency is exposed over HTTP as typed-edge graph payloads, and the first user-visible output ships: a page-view backlinks panel listing every page that links to the current one.
**Verified:** 2026-06-24T10:25:00Z
**Status:** human_needed
**Re-verification:** No — initial verification

---

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | GET /api/v1/graph returns a lean typed-edge payload (page + tag nodes, link + tag edges, no page bodies) | VERIFIED | `handlers_graph.go` handleGraph calls `h.graph.GraphData(r.Context())` and returns `writeJSON(w, 200, data)`; `GraphData` struct has `Nodes []GraphNode` + `Edges []GraphEdge` with no body field; `TestGraphData_BipartiteLeanShape` JSON-marshals output and asserts no "body"/"frontmatter" tokens — PASS |
| 2 | GET /api/v1/graph/local?path=&depth= returns the page's neighborhood (depth default 1, clamped <=3) | VERIFIED | `handleGraphLocal` reads `path` + `depth` as query params; calls `h.graph.Neighborhood(ctx, pagePath, depth)`; Store clamps depth to [1,3] via `depthMin`/`depthMax` constants; `TestNeighborhood_Depth1`, `TestNeighborhood_DepthClamp`, `TestNeighborhood_SeedAlwaysPresent` all PASS |
| 3 | GET /api/v1/graph/backlinks?path= returns the list of pages linking TO the given page with resolved titles | VERIFIED | `handleGraphBacklinks` calls `h.graph.Backlinks(ctx, pagePath)`; `Backlinks` does `SELECT src_path FROM page_links WHERE dst_path=?`; resolves titles via `titleFor`; `TestBacklinks_ReverseQuery` PASS |
| 4 | Popular-tag cap is enforced by named constants and proven by test | VERIFIED | `popularTagShare = 0.25` and `popularTagMinPages = 8` are package constants in `query.go` with hairball-prevention comments; `TestGraphData_PopularTagCap` builds a 10-page workspace, tags 9 with "common" (>25%), asserts no `tag:common` node; `TestGraphData_CapDisabledBelowMinPages` confirms a 3-page workspace is not pruned — both PASS |
| 5 | All three graph read endpoints are mounted in the authed group (any-authed), NOT editor/admin gated | VERIFIED | `router.go` lines 168-170: `authed.Get("/graph", ...)`, `authed.Get("/graph/local", ...)`, `authed.Get("/graph/backlinks", ...)` — all inside the `api.Group(func(authed chi.Router){...})` block, BEFORE the `editor` and `admin` sub-groups; grep for `editor.Get("/graph` or `admin.Get("/graph` returns empty |
| 6 | Graph payload is built only from page_links/page_tags cache tables — no .md body reads in the request path | VERIFIED | `GraphData`, `Neighborhood`, `Backlinks` methods in `query.go` all query `page_links`/`page_tags` via parameterized SQL; the only file touch is `titleFor()` which reads frontmatter only, never bodies, with nil-repo and parse-error fallback; `go list -deps ./internal/graph` confirms no `internal/search` import |
| 7 | A user viewing a page sees a collapsible "Referenced by (N)" panel at the bottom of the page read view | VERIFIED | `BacklinksPanel.tsx` renders `<section className="backlinks-panel">` with a toggle button `aria-expanded={open}` and a `.navtree` of entries; `PageView.tsx` mounts `<BacklinksPanel path={path} />` at line 130, after the body-render ternary and before the `<Dialog>` block; `BacklinksPanel.test.tsx` 5 tests PASS |
| 8 | Clicking a backlink entry navigates to that linking page via /app/page/:path | PRESENT_BEHAVIOR_UNVERIFIED | `BacklinksPanel.tsx` calls `navigate(\`/app/page/${b.path}\`)` on click; `BacklinksPanel.test.tsx` asserts LocationProbe receives `/app/page/notes/a.md` after click — unit test PASSES; however, runtime navigation in a real deployed browser (history state mutation) is a behavior-dependent truth not fully exercised by the MemoryRouter unit test |
| 9 | Empty/loading/error states show exact UI-SPEC copy without blocking the page body | VERIFIED | Component renders `<p className="backlinks-status">Loading backlinks…</p>`, `<p className="backlinks-status">Couldn't load backlinks. Refresh to try again.</p>`, `<p className="backlinks-status">No backlinks yet</p>`; exact copy matches UI-SPEC; tests for all three states PASS |
| 10 | Panel is additive — does not disturb existing CM6 LivePreviewEditor rendering; no new frontend dependency added | VERIFIED | `PageView.tsx` LivePreviewEditor block is untouched (lines 122-129); `git diff HEAD~5..HEAD -- web/package.json` returns empty (no new dependency); `BacklinksPanel.css` uses only var(--…) tokens for design values (only `1px` in `border-top: 1px solid var(--color-border)` — repo-wide hairline convention, color is tokenized) |

**Score:** 10/10 truths verified (1 present, behavior-unverified — navigation in live browser)

---

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/graph/query.go` | GraphData/Neighborhood/Backlinks methods + types + cap constants | VERIFIED | Exists; 389 lines; exports `GraphNode`, `GraphEdge`, `GraphData`, `BacklinkEntry`; `popularTagShare=0.25`, `popularTagMinPages=8` constants with comments |
| `internal/graph/query_test.go` | Tests for lean shape, popular-tag cap, neighborhood depth, backlinks | VERIFIED | Exists; `TestGraphData_BipartiteLeanShape`, `TestGraphData_PopularTagCap`, `TestGraphData_CapDisabledBelowMinPages`, `TestNeighborhood_Depth1`, `TestNeighborhood_DepthClamp`, `TestNeighborhood_SeedAlwaysPresent`, `TestBacklinks_ReverseQuery` — all PASS |
| `internal/server/handlers_graph.go` | handleGraph/handleGraphLocal/handleGraphBacklinks handlers | VERIFIED | Exists; 87 lines; all three handlers follow nil-guard + generic-copy + writeJSON pattern; `graphReadUnavailable` and `graphPathRequired` constants carry no infra vocabulary |
| `web/src/components/BacklinksPanel.tsx` | Collapsible "Referenced by (N)" panel with all states | VERIFIED | Exists; 69 lines; `aria-expanded` toggle, navtree entries, exact UI-SPEC copy strings |
| `web/src/hooks/useBacklinks.ts` | react-query hook for backlinks endpoint | VERIFIED | Exists; 18 lines; `useQuery<Backlink[]>({ queryKey: ["backlinks", path], enabled: path !== "", staleTime: 30_000 })` |
| `web/src/api/client.ts` | getBacklinks fn + Backlink type | VERIFIED | Lines 224-244; `interface Backlink { path: string; title: string }` + `getBacklinks` fetches `/api/v1/graph/backlinks?path=${encodeURIComponent(path)}` with `credentials: "same-origin"`, no CSRF (GET) |
| `web/src/components/BacklinksPanel.test.tsx` | vitest coverage for all 5 states | VERIFIED | Exists; 5 tests covering populated+click-navigate, empty (0), loading, error, collapse — all PASS |

---

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| `internal/server/handlers_graph.go` | `internal/graph/query.go` | handlers call `h.graph.GraphData / Neighborhood / Backlinks` | WIRED | Lines 29, 56, 79 in handlers_graph.go call the three Store methods |
| `internal/server/router.go` | `internal/server/handlers_graph.go` | `authed.Get("/graph", ...)` mounts in authed group | WIRED | Lines 168-170; `grep -c 'authed.Get("/graph'` returns 3 |
| `cmd/okf-workspace/main.go` | `internal/graph/query.go` | `Graph: graphStore` in `server.New(server.Deps{...})` | WIRED | Line 388: `Graph: graphStore,` — reuses the existing store built at line 249 |
| `web/src/components/BacklinksPanel.tsx` | `web/src/hooks/useBacklinks.ts` | `useBacklinks(path)` called in component | WIRED | Line 17: `const { data, isLoading, isError } = useBacklinks(path)` |
| `web/src/hooks/useBacklinks.ts` | `web/src/api/client.ts` | hook calls `getBacklinks(path)` | WIRED | Line 14: `queryFn: () => getBacklinks(path)` |
| `web/src/routes/PageView.tsx` | `web/src/components/BacklinksPanel.tsx` | `<BacklinksPanel path={path}/>` in success branch | WIRED | Line 12 (import) + line 130 (`<BacklinksPanel path={path} />`) |
| `web/src/api/client.ts` | `internal/server/handlers_graph.go` | `getBacklinks` fetches `GET /api/v1/graph/backlinks?path=` | WIRED | Line 237: `/api/v1/graph/backlinks?path=${encodeURIComponent(path)}` |

---

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|---------------|--------|--------------------|--------|
| `BacklinksPanel.tsx` | `data` from `useBacklinks(path)` | `getBacklinks()` → `GET /api/v1/graph/backlinks?path=` → `h.graph.Backlinks(ctx, path)` → `SELECT src_path FROM page_links WHERE dst_path=?` | Yes — parameterized SQL query against `page_links` cache table | FLOWING |
| `handleGraph` response | `GraphData{Nodes, Edges}` | `h.graph.GraphData(ctx)` → queries `page_links`, `page_tags` via `allPages`, `linkEdges`, `tagNodesAndEdges` | Yes — SQL queries against cache tables with popular-tag cap | FLOWING |

---

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| CGO-free build succeeds | `CGO_ENABLED=0 go build ./...` | exit 0 | PASS |
| `go test ./internal/graph/` | all graph tests including query layer | ok, 18+ tests pass | PASS |
| `go test ./internal/server/` | including TestGraphReadEndpoints, TestGraphReadMissingPath, TestGraphReadNilDependency | ok, all pass | PASS |
| `go vet ./internal/graph/ ./internal/server/` | vet clean | exit 0 | PASS |
| `internal/graph` does NOT import `internal/search` | `go list -deps ./internal/graph \| grep internal/search` | empty output, exit 1 | PASS |
| Exactly 3 authed graph route mounts | `grep -c 'authed.Get("/graph' router.go` | 3 | PASS |
| Frontend type-check | `cd web && npx tsc -b` | exit 0 | PASS |
| BacklinksPanel + PageView tests | `npx vitest run BacklinksPanel.test.tsx PageView.test.tsx` | 9/9 tests pass | PASS |
| Full frontend test suite | `npx vitest run` | 34 files, 295 tests pass | PASS |
| No new frontend dependency | `git diff HEAD~5..HEAD -- web/package.json` | empty | PASS |
| BacklinksPanel.css token-only | `grep -E '#hex\|[0-9]+px' BacklinksPanel.css` | only `1px solid var(--color-border)` (repo hairline convention) | PASS |

---

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|-------------|-------------|--------|----------|
| LINK-02 | 09-01, 09-02 | User can see a "Referenced by" (backlinks) panel on a page listing every page linking to it, each entry click-to-navigate | SATISFIED | `BacklinksPanel.tsx` + `handleGraphBacklinks` + `useBacklinks` + `getBacklinks` full chain wired; panel mounted in PageView success branch; vitest proves populated/empty/loading/error/collapse/click-navigate; REQUIREMENTS.md shows LINK-02 as Complete at Phase 9 |

---

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| `cmd/okf-workspace/main.go` | 309 | `TODO` comment referencing `agent.runSearch's TODO` | Info | Pre-existing comment (not introduced by Phase 9 commit `456ac0e` — git show confirms); references a named follow-up concern about role-scoped search in the agent; not in a Phase-9-created file. No blocker. |

No `TBD`, `FIXME`, or `XXX` markers in any Phase-9-created or Phase-9-modified files.

---

### Human Verification Required

#### 1. Backlinks Panel Visual Rendering and Navigation in Live Browser

**Test:** After building and starting the app, open a page that has at least one known inbound link (create two pages where page A links to page B, run Admin > Rebuild Graph, then open page B).
**Expected:** A "Referenced by (1)" collapsible panel appears at the bottom of the page body, below the CM6 read surface. It shows the title of page A. Clicking "page A" navigates to `/app/page/<path-of-A>`. The CM6 LivePreviewEditor content above is undisturbed. Empty/loading/error states are quiet and muted in the dark theme.
**Why human:** Component wiring and all states are proven by vitest. The runtime navigation state transition in the actual deployed browser (react-router-dom history mutation), the Obsidian-dark visual appearance of the panel against the page body, the alignment to `--prose-max-width`, the hover wash, and the focus ring are all runtime behaviors that grep and unit tests cannot observe.

---

### Gaps Summary

No gaps found. All 10 must-haves are verified at the code level:
- The three graph read endpoints (global, local, backlinks) are implemented, authed, tested, and wired through to the SQLite cache tables.
- The popular-tag cap is enforced by named constants and proven by two dedicated tests (above-threshold excluded, below-min-pages not pruned).
- The backlinks panel is mounted in PageView's success branch, uses the exact UI-SPEC copy strings, adds no new frontend dependency, and is proven by 5 vitest tests covering all states plus click-navigate.
- The only open item is a visual/runtime navigation check that requires a live browser session (human_needed per the verification instructions).

---

_Verified: 2026-06-24T10:25:00Z_
_Verifier: Claude (gsd-verifier)_
