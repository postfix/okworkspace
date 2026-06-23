---
phase: 03
slug: search
status: draft
nyquist_compliant: true
wave_0_complete: false
created: 2026-06-21
---

# Phase 03 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.
> Sourced from `03-RESEARCH.md` §Validation Architecture (Phase Requirements → Test Map),
> which is present because `workflow.nyquist_validation: true`.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go stdlib `testing` (table-driven, `t.TempDir()` real-index/real-SQLite harnesses) · vitest + React Testing Library (frontend) |
| **Config file** | none for Go (`go test ./...`); `web/vitest.config.ts` (already present from Phase 1) |
| **Quick run command** | `go test ./internal/search/... -count=1` |
| **Full suite command** | `go test ./... -race && (cd web && npx vitest run)` |
| **Estimated runtime** | ~90 seconds |

---

## Sampling Rate

- **After every task commit:** `go test ./internal/search/... -count=1` (+ `go vet ./internal/search/...`) plus the task's own `<automated>` command.
- **After every plan wave:** `go test ./... -race`.
- **Phase gate (before `/gsd-verify-work`):** `CGO_ENABLED=0 go build ./...` green (single static binary preserved with scorch) + full `go test ./... -race` green + `(cd web && npx tsc --noEmit && npx vitest run)` green.
- **Max feedback latency:** 90 seconds.

---

## Per-Task Verification Map

Maps each Phase-3 requirement (SRCH-01..06) and each required lifecycle/safety behavior to its covering test name, owning plan/task, and automated command.

| Task ID | Plan | Wave | Requirement | Behavior | Threat Ref | Test Name | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|----------|------------|-----------|-----------|-------------------|-------------|--------|
| 03-01-02 | 03-01 | 1 | SRCH-01 | Title search ranks title matches above body-only (title boost) | T-03-05 | `TestQuery_TitleBoost` | unit | `go test ./internal/search/ -run TestQuery_TitleBoost -count=1` | ⬜ W0 scaffold (03-01 Task 1) → GREEN (03-01 Task 2) | ⬜ pending |
| 03-01-02 | 03-01 | 1 | SRCH-02 | Body full-text search finds a term present only in body | — | `TestQuery_Body` | unit | `go test ./internal/search/ -run TestQuery_Body -count=1` | ⬜ W0 scaffold (03-01 Task 1) → GREEN (03-01 Task 2) | ⬜ pending |
| 03-01-02 | 03-01 | 1 | SRCH-03 | Tag (keyword) search finds a page by tag (sequence-aware read, not okf.Field) | — | `TestQuery_Tag` | unit | `go test ./internal/search/ -run TestQuery_Tag -count=1` | ⬜ W0 scaffold (03-01 Task 1) → GREEN (03-01 Task 2) | ⬜ pending |
| 03-01-02 | 03-01 | 1 | SRCH-06 (page) + **typed-results** | Every page hit carries type "page"; response includes a `type` facet with a page count | — | `TestQuery_TypedResultsAndFacet` | unit | `go test ./internal/search/ -run TestQuery_TypedResultsAndFacet -count=1` | ⬜ W0 scaffold (03-01 Task 1) → GREEN (03-01 Task 2); EXTENDED to heading/attachment by 03-03 Task 2 | ⬜ pending |
| 03-01-02 | 03-01 | 1 | **rebuild-idempotency** | RebuildIndex run twice yields identical doc count + same hits | — | `TestRebuild_Idempotent` | unit | `go test ./internal/search/ -run TestRebuild_Idempotent -count=1` | ⬜ W0 scaffold (03-01 Task 1) → GREEN (03-01 Task 2); EXTENDED (attachments+headings) by 03-03 Task 2 | ⬜ pending |
| 03-01-02 | 03-01 | 1 | **trashed-pages-excluded** | A page under `.okf-workspace/trash` is NOT indexed / never returned | T-03-02 | `TestRebuild_ExcludesTrash` | unit | `go test ./internal/search/ -run TestRebuild_ExcludesTrash -count=1` | ⬜ W0 scaffold (03-01 Task 1) → GREEN (03-01 Task 2) | ⬜ pending |
| 03-01-02 | 03-01 | 1 | (drift) | Stored last_indexed_head != gitstore.HeadSHA → DriftCheck true (startup rebuild trigger) | — | `TestDrift_HeadMismatchRebuilds` | unit | `go test ./internal/search/ -run TestDrift_HeadMismatchRebuilds -count=1` | ⬜ W0 scaffold (03-01 Task 1) → GREEN (03-01 Task 2) | ⬜ pending |
| 03-01-02 | 03-01 | 1 | (lifecycle) | KindIndex "delete" op removes the page document | — | `TestIndex_DeleteRemovesPage` | unit | `go test ./internal/search/ -run TestIndex_DeleteRemovesPage -count=1` | ⬜ W0 scaffold (03-01 Task 1) → GREEN (03-01 Task 2) | ⬜ pending |
| 03-01-02 | 03-01 | 1 | (XSS) | Highlight fragments are weight-only `<strong>`/safe; never raw `<mark>`/`<script>` | T-03-01 | `TestHighlight_WeightOnlySafe` | unit | `go test ./internal/search/ -run TestHighlight_WeightOnlySafe -count=1` | ⬜ W0 scaffold (03-01 Task 1) → GREEN (03-01 Task 2) | ⬜ pending |
| 03-01-01 | 03-01 | 1 | (drift accessor) | `gitstore.HeadSHA` returns HEAD sha (empty on no-HEAD) | — | `TestHeadSHA` | unit | `go test ./internal/gitstore/ -run TestHeadSHA -count=1` | ✅ created in 03-01 Task 1 (compiles standalone) | ⬜ pending |
| 03-01-03 | 03-01 | 1 | (HTTP) | `GET /api/v1/search` authed-only; returns typed JSON array; empty q → `[]`; no Git vocabulary in errors | T-03-06 | `TestSearchEndpoint` | integration | `go test ./internal/server/ -run TestSearchEndpoint -count=1` | ⬜ W0 scaffold (03-01 Task 1) → GREEN (03-01 Task 3) | ⬜ pending |
| 03-01-03 | 03-01 | 1 | (HTTP/authz) | `POST /api/v1/admin/search/reindex` 202 for admin, 403 for editor | T-03-07 | `TestReindexAdminOnly` | integration | `go test ./internal/server/ -run TestReindexAdminOnly -count=1` | ⬜ W0 scaffold (03-01 Task 1) → GREEN (03-01 Task 3) | ⬜ pending |
| 03-03-01 | 03-03 | 2 | (heading parse) | ATX heading scan: levels, fence-skip, GitHub slug, de-dup, trim | T-03-15 | `TestScanHeadings_Levels` / `_SkipsFence` / `_Slug` / `_DuplicateSlug` / `_Trim` | unit | `go test ./internal/okf/ -run TestScanHeadings -count=1` | ⬜ created in 03-03 Task 1 | ⬜ pending |
| 03-03-02 | 03-03 | 2 | SRCH-04 | Filename search finds an attachment by original name | T-03-11 | `TestQuery_Filename` | unit | `go test ./internal/search/ -run TestQuery_Filename -count=1` | ⬜ W0 scaffold (03-01 Task 1, t.Skip) → GREEN (03-03 Task 2) | ⬜ pending |
| 03-03-02 | 03-03 | 2 | SRCH-05 | Extracted-text hit returns the OWNING page path | T-03-12 | `TestQuery_AttachmentOwningPage` | unit | `go test ./internal/search/ -run TestQuery_AttachmentOwningPage -count=1` | ⬜ W0 scaffold (03-01 Task 1, t.Skip) → GREEN (03-03 Task 2) | ⬜ pending |
| 03-03-02 | 03-03 | 2 | SRCH-06 (heading) | Heading hit returns `kind:"heading"` with right `anchor` + `page_title` (deep-link) | T-03-15 | `TestQuery_HeadingDeepLink` | unit | `go test ./internal/search/ -run TestQuery_HeadingDeepLink -count=1` | ⬜ W0 scaffold (03-01 Task 1, t.Skip) → GREEN (03-03 Task 2) | ⬜ pending |
| 03-03-02 | 03-03 | 2 | (lifecycle) | Delete-to-trash / heading rename removes page + its heading docs (real `page_headings` schema, migration 0007) | T-03-12 | `TestIndex_DeleteRemovesHeadings` | unit | `go test ./internal/search/ -run TestIndex_DeleteRemovesHeadings -count=1` | ⬜ created in 03-03 Task 2 | ⬜ pending |
| 03-03-03 | 03-03 | 2 | (cross-tier anchor) | Rendered `## My Section` → `<h2 id="my-section">` equals `okf.ScanHeadings` slug; rehype-raw stays off | T-03-13 / T-03-14 | `PageView.test.tsx` (renderer id + XSS regression) | unit (vitest) | `cd web && npx vitest run src/routes/PageView.test.tsx` | ✅ exists (Phase 1) — extended assertion in 03-03 Task 3 | ⬜ pending |
| 03-02-03 | 03-02 | 2 | SRCH-01/02/03/06 (UI) | ⌘K palette: grouped typed rows, weight-only `<strong>` highlight (no raw HTML), no-results + error states, ↑/↓ + Enter nav | T-03-08 / T-03-09 | `SearchPalette.test.tsx` | unit (vitest) | `cd web && npx vitest run src/components/search/SearchPalette.test.tsx` | ⬜ created in 03-02 Task 3 | ⬜ pending |
| 03-04-01 | 03-04 | 3 | (CR-01 safety) | Extraction-done re-index uses fire-and-forget `Enqueue`, NEVER `EnqueueAndWait` (drain-goroutine deadlock guard) | T-03-16 | `TestExtractJob_UsesFireAndForgetEnqueue` | unit | `go test ./internal/attachments/ -run TestExtractJob_UsesFireAndForgetEnqueue -count=1` | ⬜ created in 03-04 Task 1 | ⬜ pending |
| 03-04-01 | 03-04 | 3 | (incremental) | Every page/attachment mutation enqueues the correct `KindIndex` job (correct Op/Kind/Path/ID) | T-03-17 | page/attachment `service_test.go` enqueue assertions | unit | `go test ./internal/pages/ ./internal/attachments/ -count=1` | ⬜ extended in 03-04 Task 1 | ⬜ pending |
| 03-04-02 | 03-04 | 3 | (concurrency) | Concurrent `Query` reads + `Index`/`Delete` writes on the shared bleve.Index do not race | T-03-19 | `TestIndex_ConcurrentReadWrite` | unit | `go test ./internal/search/ -race -run TestIndex_ConcurrentReadWrite -count=1` | ⬜ created in 03-04 Task 2 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

> **Required-test coverage (revision_context fixes_required #1):** the three explicitly-required tests —
> **rebuild-idempotency** (`TestRebuild_Idempotent`, 03-01 Task 2), **typed-results**
> (`TestQuery_TypedResultsAndFacet`, 03-01 Task 2 page + 03-03 Task 2 heading/attachment), and
> **trashed-pages-excluded** (`TestRebuild_ExcludesTrash`, 03-01 Task 2) — each map to a named test and a
> concrete owning plan/task above. All six SRCH requirements (SRCH-01..06) are covered: SRCH-01/02/03/06-page in
> 03-01 Task 2, SRCH-04/05/06-heading in 03-03 Task 2.

---

## Wave 0 Requirements

Wave 0 is satisfied inside **Plan 03-01 Task 1**, whose outputs are the RED-first Go test scaffolds for the
whole phase's `internal/search` surface, plus the `bleve v2.6.0` module add, the `gitstore.HeadSHA` accessor
(+ its standalone-compiling test), and migration `0007_search.sql` (creating `search_meta` + `page_headings`).

> NOTE: 03-01 Task 1's `<verify>` deliberately runs ONLY `go test ./internal/gitstore/ -run TestHeadSHA`
> (not `go build ./...`): the Wave 0 `internal/search/*_test.go` scaffolds reference the `internal/search`
> API that Task 2 implements, so a full-tree compile would false-fail by design at that point. The
> `CGO_ENABLED=0 go build ./...` single-binary gate is owned by 03-01 Task 2 (after `internal/search` exists)
> and re-run in 03-01 Task 3 and 03-04 Task 1. (revision_context fixes_required #2.)

- [ ] `internal/search/query_test.go` — TestQuery_TitleBoost / _Body / _Tag / _TypedResultsAndFacet (SRCH-01/02/03/06-page); SRCH-04/05/heading cases added by 03-03 (t.Skip placeholders until then) — created by 03-01 Task 1.
- [ ] `internal/search/rebuild_test.go` — TestRebuild_Idempotent, TestRebuild_ExcludesTrash, TestDrift_HeadMismatchRebuilds — created by 03-01 Task 1.
- [ ] `internal/search/indexjob_test.go` — TestIndex_DeleteRemovesPage (fakeEnqueuer pattern from extractjob_test.go) — created by 03-01 Task 1.
- [ ] `internal/search/highlight_test.go` — TestHighlight_WeightOnlySafe — created by 03-01 Task 1.
- [ ] `internal/server/handlers_search_test.go` — TestSearchEndpoint, TestReindexAdminOnly (authz + JSON shape + no Git vocabulary) — created by 03-01 Task 1 (scaffold) → GREEN in 03-01 Task 3.
- [ ] `internal/search/testdata/.gitkeep` — fixtures dir placeholder — created by 03-01 Task 1.
- [ ] `internal/gitstore/headsha_test.go` — TestHeadSHA (compiles + passes standalone in Task 1) — created by 03-01 Task 1.
- [ ] Module add: `go get github.com/blevesearch/bleve/v2@v2.6.0 && go mod tidy` (commit go.sum) — 03-01 Task 1.

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Full ⌘K search UX feels correct end-to-end (palette open/focus, weight-only highlight not yellow, heading deep-link jumps to section, attachment-by-extracted-text opens owning page, incremental freshness without restart, admin rebuild, NO Git/index/Bleve vocabulary anywhere) | SRCH-01..06 | Subjective UX + hidden-Git polish + cross-tier deep-link scroll are checked at the end-of-phase human-verify checkpoint (per `human_verify_mode: end-of-phase`), not per task | The 13-step browser checklist in **03-04 Task 3** (`checkpoint:human-verify`): build + run the binary against a scratch data dir and walk steps 1–13. |

*All automatable phase behaviors have automated verification (see Per-Task Verification Map). Only the subjective ⌘K UX, deep-link scroll feel, and hidden-Git polish are deferred to the end-of-phase human-verify checkpoint.*

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies
- [x] Sampling continuity: no 3 consecutive tasks without automated verify (every task has an `<automated>` command)
- [x] Wave 0 covers all MISSING references (all `internal/search` + `handlers_search` scaffolds created by 03-01 Task 1 before any consumer)
- [x] All three required tests mapped to named test + plan/task: rebuild-idempotency, typed-results, trashed-pages-excluded
- [x] All six SRCH requirements (SRCH-01..06) mapped to a named covering test
- [x] No watch-mode flags (frontend runs use `npx vitest run`; Go runs are one-shot `-count=1`)
- [x] Feedback latency < 90s
- [x] `nyquist_compliant: true` set in frontmatter

**Approval:** approved 2026-06-21
