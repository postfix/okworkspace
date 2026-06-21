---
phase: 03-search
verified: 2026-06-21T05:20:00Z
status: human_needed
score: 13/13
overrides_applied: 0
human_verification:
  - test: "Open the running app, press ⌘K (or Ctrl K), type a word that appears in a page title"
    expected: "Palette opens with input focused; page result appears under 'Pages' group with matched term bold (weight-only, no highlight fill); clicking or pressing Enter navigates to that page"
    why_human: "Live browser — focus management, visual weight-only rendering, in-app navigation cannot be verified by grep"
  - test: "With the palette open, type a word from a page body (not the title)"
    expected: "Page result appears with a body snippet; matched term is bold; result navigates to the page"
    why_human: "Live browser — body-match snippet rendering and navigation"
  - test: "Type a tag value (e.g. 'spec') in the palette"
    expected: "Pages carrying that tag appear in results even if the tag word is absent from title and body"
    why_human: "Live browser — tag-exact-match path requires a real indexed page with that tag"
  - test: "Upload an attachment to a page; wait for extraction to complete; type the original filename in the palette"
    expected: "Attachment result appears under 'Attachments' group; title is the original filename; navigating opens the owning page"
    why_human: "Live browser + attachment lifecycle — attachment upload and post-extraction search result (SRCH-04)"
  - test: "After extraction completes, search for a word that only appears in the attachment's extracted text"
    expected: "Attachment result appears; navigating to it opens the OWNING page, not a direct file URL (SRCH-05)"
    why_human: "Live browser — extracted-text search result requires a real .txt sidecar on disk"
  - test: "Navigate to a page that has ATX headings. Press ⌘K and search a word from one heading"
    expected: "Heading result appears under 'Headings' group; clicking it navigates to the page AND scrolls to the matching heading section (deep-link anchor)"
    why_human: "Live browser — heading deep-link scroll behavior and anchor resolution"
  - test: "Type ↑/↓ arrow keys in an open palette with results; press Enter on the active row"
    expected: "Active row highlight moves; Enter navigates in-app; Esc closes and restores focus to the trigger"
    why_human: "Live browser — keyboard navigation and focus-restore behavior"
  - test: "Type a query that matches nothing"
    expected: "No-matches state renders 'No matches' and echoes the query in curly quotes"
    why_human: "Live browser — empty-state copy rendering"
  - test: "Edit a page body and save; within seconds search for a word from the new body (without triggering a manual rebuild)"
    expected: "Updated page body content appears in search results (incremental indexing after save)"
    why_human: "Live browser — fire-and-forget incremental hook timing; cannot verify end-to-end without a live server and save cycle"
  - test: "Delete a page to trash; search for a word unique to that page"
    expected: "No results (deleted page is excluded); page absent from palette results"
    why_human: "Live browser — trash-exclusion verified programmatically but needs live round-trip confirmation"
  - test: "In Admin panel, click 'Rebuild search index'; observe the confirmation message"
    expected: "Button shows 'Starting…' while pending; confirmation 'Search index rebuild started.' appears; no Git/Bleve vocabulary anywhere on the page"
    why_human: "Live browser — async 202 confirmation rendering and hidden-Git language check"
  - test: "Verify no Git/Bleve/index vocabulary appears anywhere in the search UI (palette, results, errors, admin button)"
    expected: "No mention of 'index', 'Bleve', 'commit', 'HEAD', 'repo', 'Git' in any user-visible text in the search feature"
    why_human: "Live browser — visual sweep of copy; regexp scan of component text confirmed clean but rendered copy needs human eyes"
  - test: "Force the server error path: with no index configured (or stop the server mid-request). Type in the palette and observe the error state"
    expected: "'Search is unavailable' / 'Something went wrong while searching. Try again in a moment.' — no internal error details"
    why_human: "Live browser — error-state rendering under server failure conditions"
---

# Phase 3: Search Verification Report

**Phase Goal:** A user can quickly find any knowledge — page titles, body, tags, attachment filenames, extracted attachment text — with typed results (page/heading/attachment). Requirements SRCH-01..06.
**Verified:** 2026-06-21T05:20:00Z
**Status:** human_needed
**Re-verification:** No — initial verification

## Goal Achievement

All six SRCH requirements are provably implemented in shipped code with passing tests. The phase goal is architecturally achieved. Human verification is required only for live-browser behaviors (palette UX, deep-link scroll, incremental freshness timing, error-state rendering).

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | User can search page titles (SRCH-01) | VERIFIED | `TestQuery_TitleBoost` passes: title-match page ranks above body-match page; `buildQuery` applies title boost 3.0 in `query.go` |
| 2 | User can search page body full text (SRCH-02) | VERIFIED | `TestQuery_Body` passes: term only in body (`xyzzy`) returns the page; `buildQuery` includes body field with fuzziness |
| 3 | User can search by tag (SRCH-03) | VERIFIED | `TestQuery_Tag` passes: tag-exact-term query on a `tags` keyword field; `readTags` in `doc.go` reads sequence-aware from `yaml.Node` |
| 4 | User can search attachment filenames (SRCH-04) | VERIFIED | `TestQuery_Filename` passes: query on original filename returns `kind:"attachment"` result with `title == original_name`; `indexAttachment` reads meta sidecar for `OriginalName` |
| 5 | Attachment extracted text search returns owning page (SRCH-05) | VERIFIED | `TestQuery_AttachmentOwningPage` passes: extracted-text match returns `path == owning page`; `readExtractedText` + `readAttachmentMeta.PagePath` wired in `attachments.go`; `extractjob.go` re-indexes after extraction via fire-and-forget `Enqueue(search.KindIndex, UpsertAttachmentPayload)` |
| 6 | Typed results page/attachment/heading with type facet (SRCH-06) | VERIFIED | `TestQuery_TypedResultsAndFacet_AllKinds` passes: facet counts for page/heading/attachment all ≥ 1; `TestQuery_HeadingDeepLink` verifies `kind:"heading"` with `anchor == "#rollback-procedure"` and `page_title`; mapping.go declares all three document types |
| 7 | Index lives outside Git repo (no git-tracked index) | VERIFIED | `SearchConfig.IndexDir` defaults to `filepath.Join(data_dir, "index")` (config.go line 168); migration 0007 comment explicitly states "NEVER inside the content/Git repo"; all data flows through `OpenOrCreate(indexDir)` in `main.go` |
| 8 | Weight-only highlight (no `<mark>`, no `dangerouslySetInnerHTML` of raw server HTML) | VERIFIED | `TestHighlight_WeightOnlySafe` asserts no `<mark>` or `<script>` survives; `highlight.go` registers `okf-weight` highlighter using `<strong>` formatter; `highlight.ts` `renderHighlight()` maps only known markers to React elements — zero `dangerouslySetInnerHTML` calls in search UI |
| 9 | Trashed pages excluded from results | VERIFIED | `TestRebuild_ExcludesTrash` passes: trashed page absent from results; `isTrashed()` gates both rebuild walk (non-`.md` + `trashPrefix`) and query result mapping (defense-in-depth in `query.go`) |
| 10 | Incremental index — all page mutations enqueue KindIndex fire-and-forget | VERIFIED | `TestCreateEnqueuesIndexUpsert`, `TestSaveEnqueuesIndexUpsert`, `TestRenameEnqueuesIndexMove`, `TestMoveEnqueuesIndexMove`, `TestDeleteEnqueuesIndexDelete`, `TestRestoreEnqueuesIndexUpsert` all pass; enqueue helpers use `worker.Enqueue` never `EnqueueAndWait` |
| 11 | Incremental index — attachment mutations enqueue KindIndex fire-and-forget | VERIFIED | `TestUploadEnqueuesIndex`, `TestReplaceEnqueuesIndex`, `TestRemoveEnqueuesIndexDelete` all pass; `TestExtractJob_UsesFireAndForgetEnqueue` asserts the extraction-done path uses `Enqueue` and `EnqueueAndWait` call count == 0 (CR-01) |
| 12 | Idempotent rebuild + startup HEAD-drift detection | VERIFIED | `TestRebuild_Idempotent` passes; `TestDrift_HeadMismatchRebuilds` passes; `DriftCheck`/`StoreHead`/`HeadSHA` wired in `main.go`; `gitstore.HeadSHA` tested by `TestHeadSHA` |
| 13 | Concurrency safety — no data race under concurrent readers, writer, and rebuild-swap | VERIFIED | `TestIndex_ConcurrentReadWrite` passes (CGO_ENABLED=0; race-clean under CGO_ENABLED=1 -race per SUMMARY); `withIndex` holds RLock for entire operation, not snapshot-then-release |

**Score:** 13/13 truths verified

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/search/mapping.go` | Bleve typed mapping TypePage/TypeHeading/TypeAttachment | VERIFIED | All three document types declared; heading/attachment field mappings populated in 03-03 |
| `internal/search/query.go` | Disjunction query builder with title boost + type facet + trash filter | VERIFIED | `buildQuery` boosts title 3.0/prefix 2.0; facet on `type` field; `Query()` drops trashed paths |
| `internal/search/rebuild.go` | Idempotent atomic rebuild + heading/attachment indexing + page_headings rewrite | VERIFIED | tmp dir build → atomic `os.Rename` swap under mutex; headings + attachments indexed in walk |
| `internal/search/highlight.go` | `okf-weight` highlighter registered with `<strong>` wrapping, not `<mark>` | VERIFIED | `init()` registers via `registry.RegisterHighlighter`; html formatter uses `<strong>` |
| `internal/search/indexjob.go` | KindIndex handler dispatching upsert/delete/rebuild ops | VERIFIED | All ops implemented; `defer recover()` guard; payload helpers exported for mutation sites |
| `internal/search/index.go` | Single shared `bleve.Index` with RLock-held `withIndex` | VERIFIED | `withIndex` holds RLock for full operation duration (hardened in 03-04) |
| `internal/search/headings.go` | Stale-heading cleanup via page_headings prior-id-set | VERIFIED | `indexHeadings` deletes stale docs before re-indexing; `deleteHeadings` clears on page delete |
| `internal/search/attachments.go` | Attachment indexing: filename + extracted text → owning page via PagePath | VERIFIED | `indexAttachment` reads meta sidecar + extracted text; `allAttachmentIDs` enumerates via repo.Tree |
| `internal/search/concurrency_test.go` | Race test: 8 readers + 1 writer + rebuild-swap concurrent | VERIFIED | `TestIndex_ConcurrentReadWrite` passes |
| `internal/okf/headings.go` | ATX heading scanner with github-slugger-identical slug/dedup | VERIFIED | `ScanHeadings` + `slug` + `dedupSlug`; fence-aware; no whitespace collapsing (mirrors github-slugger exactly) |
| `internal/gitstore/headsha.go` | HeadSHA accessor for drift bookkeeping | VERIFIED | `HeadSHA` uses `git rev-parse --verify --quiet HEAD`; serialized under single-writer mutex |
| `internal/store/migrations/0007_search.sql` | `search_meta` + `page_headings` tables | VERIFIED | Both tables present with `CREATE TABLE IF NOT EXISTS` |
| `internal/server/handlers_search.go` | GET /search (authed) + POST /admin/search/reindex (admin, CSRF, 202, audited) | VERIFIED | Both handlers present; generic error copy (`searchUnavailable`); audit action `ActionSearchReindex` |
| `web/src/components/search/highlight.ts` | XSS chokepoint: only `<strong>` mapped to React elements, all else escaped | VERIFIED | `renderHighlight` splits on MARKER regex; plain text falls through as string (React-escaped); no `dangerouslySetInnerHTML` |
| `web/src/components/search/SearchPalette.tsx` | ⌘K overlay with 5 states, grouped results, keyboard nav, focus-trap | VERIFIED | All 5 states rendered; GROUP_ORDER groups; ↑/↓/Enter/Esc handlers; focus-trap contract |
| `web/src/store/searchStore.ts` | Zustand store for open/setOpen | VERIFIED | Minimal `create<SearchStore>` with `open` + `setOpen` |
| `web/src/hooks/useSearch.ts` | Debounced react-query hook gated on non-empty q | VERIFIED | `useDebouncedValue(rawQuery, 200)` feeds `useQuery(["search", q])` with `enabled: q.trim().length > 0` |
| `web/src/routes/Admin.tsx` | "Rebuild search index" button wired to reindexSearch mutation | VERIFIED | `reindexMut` calls `reindexSearch()`; async 202 confirmation; disabled while pending; no Bleve vocabulary |
| `web/src/components/MarkdownProse.tsx` | `rehypeSlug` before `rehypeSanitize` with `headingIdSchema` removing id from clobber | VERIFIED | `rehypeSlug` imported and applied; `headingIdSchema` removes `id` from `clobber` array |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| `AppShell.tsx` | `SearchPalette` | `<SearchPalette />` mount + `useSearchStore` global listener | WIRED | Lines 10-11,37-59 in AppShell.tsx; `⌘K`/`Ctrl K` listener calls `setOpen(true)` |
| `SearchPalette` | `GET /api/v1/search` | `useSearch` hook → `search(q)` in `client.ts` | WIRED | `useSearch` feeds debounced query to `search()` which fetches `/api/v1/search?q=...` |
| `router.go` | `handleSearch` | `authed.Get("/search", h.handleSearch)` | WIRED | Line 132 in router.go; inside authenticated group |
| `handleSearch` | `search.Index.Query` | `h.search.Query(r.Context(), q)` | WIRED | handlers_search.go line 37; returns typed `[]Result` marshalled as JSON |
| `pages/service.go` | `search.KindIndex` | `enqueueIndexUpsert/Delete` via `worker.Enqueue` | WIRED | Create/Save/CreateFolder/Rename/Move/Delete/Restore all call helpers confirmed by 6 passing tests |
| `attachments/service.go` + `extractjob.go` | `search.KindIndex` | `worker.Enqueue(search.KindIndex, Upsert/DeleteAttachmentPayload)` | WIRED | Upload/Replace/Remove + extraction-done confirmed by 4 passing tests including CR-01 proof |
| `main.go` | `search.Index` startup | `OpenOrCreate` → `SetRepo` → `SetDB` → `Register(KindIndex)` → `DriftCheck` → `Enqueue` rebuild if drifted | WIRED | Lines 204-256 in main.go confirm full startup wiring |
| `highlight.ts` | React `<strong>` elements | `renderHighlight` splits MARKER regex, maps depth > 0 segments to `createElement("strong")` | WIRED | No `dangerouslySetInnerHTML`; plain text segments passed as strings to React (auto-escaped) |
| `MarkdownProse.tsx` | heading `id` attributes matching `okf.ScanHeadings` | `rehypeSlug` → `rehypeSanitize(headingIdSchema)` with `id` removed from clobber | WIRED | `PageView.test.tsx` asserts `<h2 id="rollback-procedure">` not prefixed |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|--------------|--------|-------------------|--------|
| `SearchPalette.tsx` | `data` (SearchResult[]) | `useSearch(query)` → `search(q)` → `GET /api/v1/search` → `Index.Query()` → Bleve scorch index | Yes — Bleve index built from repo file walk in `RebuildIndex`; `indexPage` reads actual file content via `repo.Read` | FLOWING |
| `handleSearch` (backend) | `results []Result` | `Index.Query(ctx, q)` which calls `withIndex(idx.SearchInContext)` against the real Bleve scorch index | Yes — Bleve returns real ranked hits from indexed file content | FLOWING |
| `RebuildIndex` | page documents | `repo.Tree()` + `repo.Read(path)` + `okf.Parse` | Yes — walks real on-disk files through SEC-01 resolver | FLOWING |
| `indexAttachment` | attachment docs | `readAttachmentMeta(id)` reads `<id>.json`; `readExtractedText(id)` reads `<id>.txt` | Yes — reads real sidecar files; tolerates missing `.txt` gracefully | FLOWING |
| `Admin.tsx` reindex button | mutation result | `reindexSearch()` → `POST /api/v1/admin/search/reindex` → `worker.Enqueue(KindIndex, RebuildPayload())` | Yes — enqueues a real rebuild job; 202 response confirms enqueue succeeded | FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| `CGO_ENABLED=0 go build ./...` succeeds (single static binary) | `CGO_ENABLED=0 go build ./...` | exit 0, no output | PASS |
| All Go tests pass | `CGO_ENABLED=0 go test ./... -count=1` | 14 packages ok, 0 failures | PASS |
| `TestRebuild_Idempotent` passes | part of `go test ./internal/search/` | PASS | PASS |
| `TestQuery_TypedResultsAndFacet` passes | part of `go test ./internal/search/` | PASS | PASS |
| `TestRebuild_ExcludesTrash` passes | part of `go test ./internal/search/` | PASS | PASS |
| `TestExtractJob_UsesFireAndForgetEnqueue` passes (CR-01 deadlock proof) | `go test ./internal/attachments/ -run TestExtractJob` | PASS | PASS |
| `TestIndex_ConcurrentReadWrite` passes | `go test ./internal/search/ -run TestIndex_ConcurrentReadWrite` | PASS (2.04s) | PASS |
| Frontend build succeeds | `cd web && npm run build` | 2446 modules, 0 errors | PASS |
| TypeScript strict check clean | `npx tsc --noEmit` | exit 0, no output | PASS |
| Vitest suite green | `npx vitest run` | 116/116 tests passed, 15 files | PASS |
| `SearchPalette.test.tsx` XSS guard: `<img onerror>` not in DOM | vitest (5 tests) | `document.querySelector("img[onerror]")` returns null while `textContent` contains "onerror" | PASS |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|-------------|-------------|--------|----------|
| SRCH-01 | 03-01 | User can search page titles | SATISFIED | `TestQuery_TitleBoost` passes; title field boost 3.0 in `buildQuery` |
| SRCH-02 | 03-01 | User can search page body full text | SATISFIED | `TestQuery_Body` passes; body field analyzed with `en` analyzer |
| SRCH-03 | 03-01 | User can search by tag | SATISFIED | `TestQuery_Tag` passes; `tags` keyword field exact-matched |
| SRCH-04 | 03-03 | User can search attachment filenames | SATISFIED | `TestQuery_Filename` passes; `OriginalName` stored in `filename` field |
| SRCH-05 | 03-03 | User can search extracted attachment text | SATISFIED | `TestQuery_AttachmentOwningPage` passes; `path` = owning page via `meta.PagePath` |
| SRCH-06 | 03-01,03-03 | Search returns typed results page/attachment/heading | SATISFIED | `TestQuery_TypedResultsAndFacet_AllKinds` passes; `TestQuery_HeadingDeepLink` verifies anchor |

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| `web/src/components/search/highlight.ts` | 9 | `dangerouslySetInnerHTML` in comment only | Info | Not an actual usage — comment documents what is deliberately avoided (T-03-08) |
| `web/src/components/search/SearchPalette.tsx` | 26 | `return null` | Info | Intentional render gate: palette wrapper returns null when closed; `PaletteInner` mounts only when open (documented design decision) |
| `web/src/components/search/SearchPalette.tsx` | 228 | `return null` | Info | Intentional: empty group skipped in render loop — correct behavior |

No TBD, FIXME, or XXX markers found in any search phase files. No unresolved debt markers.

### Human Verification Required

13 browser checks are deferred per `human_verify_mode: end-of-phase` (declared in 03-04-PLAN.md Task 3). All automated gates pass. These checks require a running server and browser session:

#### 1. ⌘K Palette Opens with Focus and Title Results

**Test:** Press ⌘K (or Ctrl K) anywhere in the app. Type a word from a known page title.
**Expected:** Palette opens with input focused; page result appears under "Pages" group with matched term bold (weight-only `<strong>`, no background fill); clicking or Enter navigates to that page in-app.
**Why human:** Live browser — focus management, visual weight-only rendering, in-app navigation.

#### 2. Body Full-Text Result

**Test:** Type a word from a page body (not the title) into the palette.
**Expected:** Page result with body snippet; matched term bold; navigates to page.
**Why human:** Live browser — body snippet rendering.

#### 3. Tag Search

**Test:** Type a tag value (e.g. "spec") into the palette.
**Expected:** Pages with that tag appear even if the tag word is absent from title and body.
**Why human:** Live browser — tag-exact-match with real indexed data.

#### 4. Attachment Filename Search (SRCH-04)

**Test:** Upload an attachment to a page; wait for extraction; type the original filename in the palette.
**Expected:** Attachment result under "Attachments" group; title is the original filename; navigating opens the owning page.
**Why human:** Live browser + attachment lifecycle.

#### 5. Extracted Attachment Text Search (SRCH-05)

**Test:** After extraction, search a word only in the attachment's extracted text.
**Expected:** Attachment result; navigating opens the OWNING page, not a file URL.
**Why human:** Live browser — requires real .txt sidecar on disk.

#### 6. Heading Deep-Link (SRCH-06)

**Test:** Search a word from a page heading. Click the heading result.
**Expected:** Navigates to the page AND scrolls to the matching heading section (anchor deep-link).
**Why human:** Live browser — scroll-into-view behavior after navigation.

#### 7. Keyboard Navigation + Esc

**Test:** With results visible, press ↑/↓ to move selection; press Enter on active row; open palette again and press Esc.
**Expected:** Selection moves with aria-selected; Enter navigates; Esc closes and restores focus to the trigger button.
**Why human:** Live browser — keyboard interaction and focus restore.

#### 8. No-Results State

**Test:** Type a query matching nothing.
**Expected:** "No matches" heading with the query echoed in curly quotes ("Nothing matched "xyz"…").
**Why human:** Live browser — empty-state copy.

#### 9. Incremental Index After Save

**Test:** Edit a page body and save. Within a few seconds, search for a word from the new body (no manual rebuild).
**Expected:** Updated content appears in search results (incremental upsert via enqueue hook).
**Why human:** Live browser + timing — fire-and-forget hook latency.

#### 10. Trash Exclusion

**Test:** Delete a page to trash; search for a word unique to that page.
**Expected:** No results — deleted page absent from palette.
**Why human:** Live browser — round-trip trash exclusion.

#### 11. Admin Rebuild Button

**Test:** In the Admin panel, click "Rebuild search index".
**Expected:** Button shows "Starting…" while pending; "Search index rebuild started." confirmation appears; no Git/Bleve vocabulary anywhere.
**Why human:** Live browser — async 202 confirmation and hidden-Git language sweep.

#### 12. No Git/Index Vocabulary in UI

**Test:** Visual sweep of all search UI: palette, results, errors, admin button copy.
**Expected:** No mention of "index", "Bleve", "commit", "HEAD", "repo", "Git" anywhere visible.
**Why human:** Live browser visual inspection.

#### 13. Error State Rendering

**Test:** Trigger a server error (e.g., stop the search index or force a 500); type in the palette.
**Expected:** "Search is unavailable" / "Something went wrong while searching. Try again in a moment." — no internal error details exposed.
**Why human:** Live browser under controlled failure condition.

### Gaps Summary

No gaps. All 13 observable truths are VERIFIED by code inspection and test execution. The 13 human verification items above are browser-only behaviors that cannot be confirmed by grep or unit tests. Status is `human_needed` because browser checks exist — not because automated verification found any failures.

---

_Verified: 2026-06-21T05:20:00Z_
_Verifier: Claude (gsd-verifier)_
