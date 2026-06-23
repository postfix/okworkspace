---
phase: 03-search
plan: 01
subsystem: api
tags: [search, bleve, scorch, full-text, typed-results, facet, rebuild-from-files, drift-detection, weight-only-highlight, jobs-worker]

# Dependency graph
requires:
  - phase: 01 (pages vertical slices)
    provides: okf.Parse/Field + opaque Doc.Body, repo safe-path resolver (SEC-01) repo.Read/Tree, jobs.Worker (Enqueue/Register), gitstore single-writer git wrapper, pages trash dir convention (.okf-workspace/trash)
  - phase: 02 (attachments + text extraction)
    provides: KindExtract fire-and-forget handler pattern (CR-01 lesson), audit logger, optional-dependency handler wiring pattern, server test harness (loginAs/loginAsAdmin/doMutate/fetchCSRF)
provides:
  - internal/search package — typed Bleve scorch index (mapping/doc/index/meta/query/highlight/rebuild/indexjob/service)
  - end-to-end authed full-text page search (title/body/tag) with title boost, type facet, weight-only-safe highlight (SRCH-01/02/03/06-page)
  - idempotent atomic rebuild-from-files (skips trash + non-.md) + startup HEAD-drift detection → auto-rebuild
  - GET /api/v1/search (authed) + POST /api/v1/admin/search/reindex (admin, audited)
  - gitstore.HeadSHA(ctx) drift-bookkeeping accessor
  - migration 0007_search.sql (search_meta + page_headings tables)
  - SearchConfig.IndexDir (defaults <data_dir>/index, outside the content/Git repo)
  - cross-plan contracts: search.{TypePage,TypeHeading,TypeAttachment,KindIndex}, search.Result DTO, search.{IndexHandler,OpenOrCreate,RebuildPayload,UpsertPagePayload,DeletePagePayload}, indexPayload{Op,Kind,Path,ID}
affects: [03-02 ⌘K SearchPalette (consumes Result DTO + GET /search), 03-03 attachment/heading indexing + mutation-hook wiring (uses KindIndex handler + page_headings table + empty-but-declared heading/attachment mappings), 03-04 admin reindex UI]

# Tech tracking
tech-stack:
  added:
    - github.com/blevesearch/bleve/v2 v2.6.0 (pure-Go scorch full-text index; LOCKED in CLAUDE.md)
  patterns:
    - "Bleve scorch index lives UNDER <data_dir>/index/, NEVER in the content/Git repo (derived, rebuildable artifact)"
    - "Weight-only highlighter registered in the Bleve registry: simple fragmenter + html FragmentFormatter('<strong>','</strong>') — escapes surrounds, never <mark> (XSS guard)"
    - "Rebuild builds into <dir>.tmp then atomic os.Rename swap under a mutex guarding the index pointer (in-flight queries read old index until swap)"
    - "Drift = stored last_indexed_head != gitstore.HeadSHA; startup enqueues rebuild fire-and-forget (Enqueue, NOT EnqueueAndWait — CR-01)"
    - "Tags read sequence-aware from doc.Front yaml.Node (okf.Field returns scalars only — Pitfall 7)"
    - "Search/reindex follow the optional-dependency handler pattern (nil dep → generic 500), like pages/attachments"

key-files:
  created:
    - internal/search/mapping.go
    - internal/search/doc.go
    - internal/search/index.go
    - internal/search/meta.go
    - internal/search/query.go
    - internal/search/highlight.go
    - internal/search/rebuild.go
    - internal/search/indexjob.go
    - internal/search/service.go
    - internal/search/query_test.go
    - internal/search/rebuild_test.go
    - internal/search/indexjob_test.go
    - internal/search/highlight_test.go
    - internal/search/harness_test.go
    - internal/search/testdata/.gitkeep
    - internal/gitstore/headsha.go
    - internal/gitstore/headsha_test.go
    - internal/store/migrations/0007_search.sql
    - internal/server/handlers_search.go
    - internal/server/handlers_search_test.go
  modified:
    - go.mod
    - go.sum
    - internal/config/config.go
    - internal/audit/audit.go
    - internal/server/router.go
    - internal/server/handlers_auth.go
    - cmd/okf-workspace/main.go

decisions:
  - "Index attaches repo/db via SetRepo/SetDB after OpenOrCreate(dir) so the startup constructor keeps the simple single-arg signature; DriftCheck/StoreHead take the gitstore via a headProvider interface."
  - "Weight-only highlight implemented by registering a named highlighter ('okf-weight') and referencing it via NewHighlightWithStyle — Bleve resolves the request Style from the registry; reuses the html formatter's built-in html.EscapeString of surrounding text as the XSS guard rather than hand-rolling escaping."
  - "deletePage deletes only the page doc this plan; heading-doc cleanup (page_headings) is deferred to 03-03 (pages-only scope). page_headings + heading/attachment mappings are declared now so 03-03 adds field population, not a schema/mapping migration that would force a rebuild."
  - "Malformed page during rebuild is skipped (not fatal) — a single bad file must not abort the whole index rebuild."
  - "Search is always wired (cfg.Search.Enabled left for a future flag); a failed index open returns a startup error rather than silently disabling search."

metrics:
  duration: ~35m
  completed: 2026-06-21
  tasks: 3
  files_changed: 27
  commits: 3
---

# Phase 3 Plan 01: Backend Search Foundation Summary

Stood up the backend full-text search vertical slice on the LOCKED Bleve v2.6.0 pure-Go scorch engine: an authenticated user can `GET /api/v1/search?q=...` and get typed, ranked PAGE results across title (boosted), body, and tags, with a type facet and weight-only-safe highlight fragments — backed by an idempotent atomic rebuild-from-files and startup HEAD-drift auto-rebuild, plus an admin reindex endpoint. The index is a derived on-disk artifact under `<data_dir>/index/`, never inside the content/Git repo; `CGO_ENABLED=0 go build ./...` still produces one static binary (scorch is pure-Go).

## What Was Built

- **`internal/search` package** — typed index mapping (`type` field + per-type page/heading/attachment document mappings; heading/attachment declared empty-but-registered for 03-03), scorch lifecycle (`OpenOrCreate` + atomic dir-swap), disjunction query builder (match + prefix + fuzzy + phrase, title boost 3 / prefix boost 2, fuzziness 1), `type` facet, registered weight-only `<strong>` highlighter, idempotent rebuild-from-files (skips trash + non-`.md`, reads via `repo.Read`), `last_indexed_head` meta + `DriftCheck`/`StoreHead`, and the `KindIndex` job handler (upsert/delete/rebuild) with a `defer recover()` guard.
- **`gitstore.HeadSHA(ctx)`** — read-only `git rev-parse HEAD`, empty on no-HEAD, mirroring `BlobRevision` and serialized under the single-writer mutex.
- **Migration 0007** — `search_meta` (drift bookkeeping) + `page_headings` (heading-doc tracking for 03-03), both idempotent.
- **HTTP + wiring** — `GET /search` (authed group), `POST /admin/search/reindex` (admin subgroup, 202, audited "rebuild search index"), `SearchConfig.IndexDir` default, and `main.go` startup: open index → register handler → best-effort drift check → fire-and-forget rebuild enqueue.

## Tasks Completed

| Task | Name | Commit | Key files |
| ---- | ---- | ------ | --------- |
| 1 | Wave 0 scaffolds + bleve module + HeadSHA + migration 0007 + config | b2f9e5c | go.mod/go.sum, gitstore/headsha.go(+test), migrations/0007_search.sql, config.go, search/*_test.go scaffolds |
| 2 | Bleve index lifecycle + typed mapping + query + rebuild + drift + indexjob (TDD) | 2a2dbc5 | search/{mapping,doc,index,meta,query,highlight,rebuild,indexjob,service}.go |
| 3 | Search HTTP endpoints + router/Deps wiring + startup open/register/drift (TDD) | 5de578b | server/handlers_search.go(+test), router.go, handlers_auth.go, audit.go, main.go |

## Tests

All required tests exist and pass:
- `TestQuery_TitleBoost`, `TestQuery_Body`, `TestQuery_Tag`, `TestQuery_TypedResultsAndFacet`, `TestQuery_EmptyFastPath` (SRCH-01/02/03/06-page)
- `TestRebuild_Idempotent`, `TestRebuild_ExcludesTrash`, `TestDrift_HeadMismatchRebuilds` (lifecycle/drift)
- `TestIndex_DeleteRemovesPage`, `TestIndex_RebuildOp` (KindIndex handler)
- `TestHighlight_WeightOnlySafe` (XSS guard: `<strong>` present, no raw `<mark>`/`<script>`)
- `TestHeadSHA` (gitstore accessor)
- `TestSearchEndpoint`, `TestReindexAdminOnly` (HTTP authz + no Git vocabulary in errors)

Gates: `CGO_ENABLED=0 go build ./...` green; `CGO_ENABLED=0 go test ./...` green; `go test ./internal/search/ ./internal/server/ -race` green; `go vet ./internal/search/...` clean; `npm run build` (web) green.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] `go mod tidy` aborts on a transitive test-only dep**
- **Found during:** Task 1 / Task 2 module management.
- **Issue:** `go mod tidy` fails mid-run on `github.com/json-iterator/go.test` → `github.com/google/gofuzz` (a transitive test dependency of a Bleve dep) reporting "module found but does not contain package". This is a known upstream tidy quirk, unrelated to our code; it left the `require` un-written so the search package failed to compile after the first tidy.
- **Fix:** Used `go get github.com/blevesearch/bleve/v2@v2.6.0` (which writes go.mod/go.sum correctly) instead of relying on `go mod tidy`. The build, full test suite, and `CGO_ENABLED=0` static build all pass with the resulting go.mod/go.sum, confirming completeness.
- **Files modified:** go.mod, go.sum
- **Commit:** b2f9e5c / 2a2dbc5

### Minor scope notes (within plan)

- Added `harness_test.go` (a shared `t.TempDir()` index+repo+db+gitstore harness) to back the Wave 0 scaffolds — implied by the plan's "real-index harness" instruction, not separately listed in `files_modified`.
- Added `search.UpsertPagePayload`/`DeletePagePayload` helpers next to `RebuildPayload` so 03-03's mutation hooks have a single payload-shape owner; defined here, unused by this plan.
- Added a `typeFacetCount` package-internal test helper to assert facet presence (SRCH-06) without exposing the raw `*bleve.SearchResult`.

## Threat Model Coverage

| Threat ID | Disposition | How addressed |
|-----------|-------------|---------------|
| T-03-01 (highlight XSS) | mitigated | Registered weight-only `<strong>` highlighter; html formatter escapes surrounds; `TestHighlight_WeightOnlySafe` asserts no raw `<mark>`/`<script>`. |
| T-03-02 (trash disclosure) | mitigated | Rebuild skips `.okf-workspace/trash`; Query drops any page hit under the trash prefix (defense in depth); `TestRebuild_ExcludesTrash`. |
| T-03-03 (path traversal) | mitigated | All file reads via `repo.Read`/`repo.Tree` (SEC-01); no `os.*` on content paths. |
| T-03-04 (index path escape) | mitigated | IndexDir under `<data_dir>/index/`, outside the content repo; default derived from data_dir. |
| T-03-05 (query DoS) | mitigated | `q` capped at 256 chars; result size capped at 50; fixed fuzziness 1. |
| T-03-06 (error disclosure) | mitigated | Generic "Search is unavailable…" to client; details to slog only; `assertNoGitVocab` in tests. |
| T-03-07 (reindex EoP) | mitigated | Admin subgroup `RequireRole(admin)` + nosurf CSRF; `TestReindexAdminOnly` (admin 202 / editor 403). |
| T-03-SC (supply chain) | mitigated | bleve v2.6.0 LOCKED; Go proxy module; go.sum committed. |

## Known Stubs

None that block the plan goal. The following are intentional, documented forward-deferrals to 03-03 (NOT data stubs in the page-search path, which is fully wired end to end):
- `deletePage` removes only the page document; heading-doc cleanup via the `page_headings` table is wired by 03-03.
- Heading/attachment document mappings are declared empty-but-registered; field population (and attachment/heading result kinds) land in 03-03 without changing the index mapping or the `Result` envelope.
- `KindIndex` mutation hooks (enqueue on page save/rename/delete) are not yet wired into `pages.Service`; this plan provides the handler + payload helpers, 03-03 wires the call sites. Correctness today is guaranteed by the idempotent rebuild + startup drift check.

## Self-Check: PASSED
- internal/search/{mapping,query,rebuild,indexjob,index,meta,highlight,doc,service}.go: FOUND
- internal/gitstore/headsha.go: FOUND
- internal/store/migrations/0007_search.sql: FOUND
- internal/server/handlers_search.go: FOUND
- Commits b2f9e5c, 2a2dbc5, 5de578b: FOUND in git log
