---
phase: 03-search
plan: 03
subsystem: api
tags: [search, bleve, atx-headings, github-slugger, rehype-slug, attachments, extracted-text, deep-link, stale-cleanup, typed-results]

# Dependency graph
requires:
  - phase: 03 (plan 01 — backend search foundation)
    provides: typed Bleve index (TypePage/TypeHeading/TypeAttachment mappings declared empty-but-registered), Result DTO + Query/mapHit, KindIndex job handler + indexPayload, page_headings table (migration 0007), idempotent atomic rebuild, harness_test.go real-index harness
  - phase: 02 (attachments + text extraction)
    provides: attachments/<id>.json meta sidecar carrying PagePath, attachments/<id>.txt extracted text, three-part on-disk model
  - phase: 01 (pages)
    provides: okf.Parse/Field opaque Doc.Body, links.go fence-skipping helpers (fenceAt/skipFencedBlock), repo safe-path resolver (Read/Exists/Tree)
provides:
  - okf.ScanHeadings — ATX heading scanner over opaque Doc.Body (skips fenced code) producing GitHub-style #anchors that mirror github-slugger byte-for-byte (incl. -1/-2 de-dup)
  - attachment document indexing — filename (SRCH-04) + extracted text → OWNING page (SRCH-05) via AttachmentMeta.PagePath, tolerating a missing .txt
  - heading document indexing — deep-link sub-docs (SRCH-06 heading) with #anchor + owning page title
  - stale-heading cleanup — page_headings prior-id-set delete on re-index/rename + page-delete clearing (Bleve has no delete-by-prefix)
  - heading/attachment typed query-result mapping; trash filter now drops every kind
  - rehype-slug on the read view with a sanitize schema that keeps heading ids verbatim (no user-content- clobber) so deep-links resolve
  - cross-plan helpers: search.UpsertAttachmentPayload / DeleteAttachmentPayload (for 03-04 attachment mutation hooks)
affects: [03-04 mutation-hook wiring (enqueues page+attachment upsert/delete on save/rename/remove), admin reindex UI]

# Tech tracking
tech-stack:
  added:
    - rehype-slug 6.0.0 (rehypejs plugin wrapping github-slugger; read-view heading ids)
  patterns:
    - "Go slug() mirrors github-slugger EXACTLY: lowercase, strip non letter/number/-/_, each space→'-' (NO collapsing, NO hyphen trimming), -1/-2 de-dup — cross-tier anchor equality is the deep-link contract (T-03-15)"
    - "Heading body scanning is byte-only (never a Markdown AST, never re-emitted) — reuses links.go fenceAt/skipFencedBlock so a '#' inside a code fence is opaque"
    - "Bleve has no delete-by-prefix: per-page heading ids are tracked in page_headings; an upsert deletes the prior set before re-indexing; rebuild rewrites the whole table to mirror a from-scratch index (the backstop)"
    - "Attachment owning-page link comes from AttachmentMeta.PagePath (stored at upload) — no tree scan; search reads the <id>.json sidecar directly via repo.Read (no import of the attachments package)"
    - "rehype-slug runs BEFORE rehype-sanitize and 'id' is removed from the sanitize clobber list so heading ids survive verbatim (default schema otherwise prefixes user-content-)"

key-files:
  created:
    - internal/okf/headings.go
    - internal/okf/headings_test.go
    - internal/search/headings.go
    - internal/search/attachments.go
  modified:
    - internal/search/doc.go
    - internal/search/indexjob.go
    - internal/search/rebuild.go
    - internal/search/query.go
    - internal/search/query_test.go
    - internal/search/rebuild_test.go
    - internal/search/harness_test.go
    - web/src/components/MarkdownProse.tsx
    - web/src/routes/PageView.test.tsx
    - web/package.json
    - web/package-lock.json

key-decisions:
  - "slug() does NOT collapse whitespace nor trim resulting hyphens (the plan's prose said 'collapse spaces to single -'); github-slugger does neither, so collapsing would DIVERGE from rehype-slug and break deep-links — mirrored github-slugger exactly instead (verified against node_modules/github-slugger/regex.js + live output)."
  - "Fixed deep-link breakage at the renderer: rehype-sanitize's default schema clobbers id to user-content-<slug>; removed 'id' from the clobber list (id is already in the global allow-list) so the rendered id equals okf.ScanHeadings's #anchor. No new attribute permitted; rehype-raw stays OFF."
  - "search reads attachment meta/txt via repo.Read with a local 3-field attachmentMeta struct + local path helpers rather than importing internal/attachments (avoids a heavy cross-package dependency for two path shapes)."
  - "rebuild rewrites the ENTIRE page_headings table after a successful swap (mirrors the fresh index); the incremental upsert/delete path uses the per-page prior-id set — both verified against the real SQLite schema."

patterns-established:
  - "Cross-tier slug parity: a Go scanner and a JS renderer plugin agree on anchors because both implement github-slugger; a duplicate-slug test asserts the -1/-2 de-dup matches."
  - "Tracking-table-backed stale cleanup for an engine without delete-by-prefix (page_headings)."

requirements-completed: [SRCH-04, SRCH-05, SRCH-06]

# Metrics
duration: ~25m
completed: 2026-06-21
---

# Phase 3 Plan 03: Attachments, Extracted-Text, and Heading Deep-Link Search Summary

**Completed search coverage: attachments findable by original filename and by extracted text (linking to the OWNING page), page headings findable and deep-linking to the rendered section — via a new ATX heading scanner whose GitHub-style anchors match rehype-slug byte-for-byte, with page_headings-backed stale-heading cleanup verified against the real SQLite schema.**

## Performance

- **Duration:** ~25 min
- **Completed:** 2026-06-21
- **Tasks:** 3 (plus a pre-authorized package-legitimacy checkpoint)
- **Files modified:** 15 (4 created, 11 modified)

## Accomplishments
- `okf.ScanHeadings` — the one net-new parser in the codebase: an ATX heading scanner over the opaque body that skips fenced code blocks and emits GitHub-style anchors identical to github-slugger (including `-1`/`-2` de-dup), so heading search anchors equal rendered heading ids.
- Attachment indexing: filename search (SRCH-04) and extracted-text search whose result navigates to the OWNING page (SRCH-05), using the meta's stored `PagePath` — no tree scan; a missing `.txt` is tolerated (filename-only).
- Heading indexing with deep-link `#anchor` + owning page title (SRCH-06 heading); typed results page/heading/attachment all carried by the unchanged `Result` envelope and the type facet.
- Stale-heading cleanup via the `page_headings` prior-id-set delete (Bleve has no delete-by-prefix), with page-delete clearing both heading docs and rows; the rebuild rewrites the whole table as the backstop. Cleanup is tested against the real migration-0007 SQLite schema.
- `rehype-slug` wired into the read view so rendered headings carry the matching ids; the sanitize clobber adjustment keeps those ids verbatim while rehype-raw stays OFF (XSS guard intact).

## Task Commits

1. **Task 1: ATX heading scanner in internal/okf** — `9d625b1` (feat, TDD test+impl)
2. **Task 2: Index attachment + heading docs, typed result mapping, stale-heading cleanup** — `056cde5` (feat, TDD)
3. **Task 3: rehype-slug heading ids matching the Go scanner** — `642c60f` (feat)

_Package-legitimacy checkpoint (rehype-slug): pre-authorized by the orchestrator (core rehypejs/github-slugger plugin); installed `rehype-slug@6.0.0` and proceeded._

## Files Created/Modified
- `internal/okf/headings.go` — `ScanHeadings` + `slug`/`dedupSlug` (github-slugger mirror), fence-aware ATX scanning.
- `internal/okf/headings_test.go` — levels, fence-skip, slug parity, duplicate-slug de-dup, trim.
- `internal/search/headings.go` — `indexHeadings` (prior-set delete + re-index + page_headings rewrite), `deleteHeadings`, prior-id helpers.
- `internal/search/attachments.go` — `indexAttachment`/`deleteAttachment`, meta/txt reads via `repo.Read`, owning-page title resolver, `allAttachmentIDs` enumerator.
- `internal/search/doc.go` — `headingDoc` + `attachmentDoc` builders (heading id = pagePath+anchor).
- `internal/search/indexjob.go` — attachment upsert/delete branches; `indexPage`/`deletePage` now take ctx and maintain heading docs; attachment payload helpers.
- `internal/search/rebuild.go` — index headings per page + attachments after the page walk; rewrite the whole `page_headings` table post-swap.
- `internal/search/query.go` — trash filter now drops heading/attachment hits whose owning page is trashed (T-03-12).
- `internal/search/{query,rebuild,harness}_test.go` — filename/owning-page/heading deep-link/all-kinds facet/stale-cleanup tests + `writeAttachment` harness helper.
- `web/src/components/MarkdownProse.tsx` — `rehypeSlug` before `rehypeSanitize`; `headingIdSchema` removes `id` from clobber.
- `web/src/routes/PageView.test.tsx` — asserts `<h2 id="rollback-procedure">`; XSS regression retained.
- `web/package.json`, `web/package-lock.json` — `rehype-slug@^6.0.0` pinned + locked.

## Decisions Made
See `key-decisions` frontmatter. The two load-bearing ones: (1) the Go slug mirrors github-slugger with NO whitespace collapsing/trimming (the plan's prose suggested collapsing, which would have broken deep-link parity); (2) the deep-link bug was at the sanitize layer (id clobbering to `user-content-…`), fixed by removing `id` from the clobber list rather than only allow-listing it.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] slug must NOT collapse spaces or trim hyphens (cross-tier parity)**
- **Found during:** Task 1 (heading scanner).
- **Issue:** The plan's action text said "collapse spaces to single `-`". github-slugger maps EACH space to a separate `-` and never trims leading/trailing hyphens (verified: `"Hello   World"` → `hello---world`, `"  Trim Me  "` → `--trim-me--`). Collapsing/trimming in Go would diverge from rehype-slug and send heading deep-links to the page top instead of the section (defeats T-03-15).
- **Fix:** Implemented `slug` to mirror github-slugger exactly (lowercase, strip non letter/number/`-`/`_`, each space→`-`, no collapse/trim) and asserted parity in `TestScanHeadings_Slug`/`DuplicateSlug`. The trimming that DOES happen is on the heading TEXT (Markdown trims heading content) before slugging, matching what rehype-slug sees.
- **Files modified:** internal/okf/headings.go, internal/okf/headings_test.go
- **Verification:** Slug outputs cross-checked against live `github-slugger` node output and `node_modules/github-slugger/regex.js`.
- **Committed in:** `9d625b1` (Task 1).

**2. [Rule 1 - Bug] rehype-sanitize clobbers heading ids to `user-content-<slug>` — breaks deep-links**
- **Found during:** Task 3 (renderer anchors).
- **Issue:** The plan said to extend the sanitize schema's allowed attributes to include `id` on h1-h6. But `id` is ALREADY in the default global allow-list; the real failure is that the default schema's `clobber: [...,'id']` + `clobberPrefix: 'user-content-'` rewrites `id="rollback-procedure"` → `id="user-content-rollback-procedure"`, so the rendered id never equals `okf.ScanHeadings`'s `#rollback-procedure` anchor.
- **Fix:** Built `headingIdSchema` = default schema with `id` removed from `clobber` (no new attribute permitted). Verified via a unified pipeline that the rendered heading is `<h2 id="rollback-procedure">`. rehype-raw stays OFF; the XSS regression test still passes.
- **Files modified:** web/src/components/MarkdownProse.tsx, web/src/routes/PageView.test.tsx
- **Verification:** New PageView test asserts `heading.id === "rollback-procedure"`; the existing raw-HTML XSS test still passes.
- **Committed in:** `642c60f` (Task 3).

---

**Total deviations:** 2 auto-fixed (both Rule 1 — correctness). Both were essential to the SRCH-06 deep-link guarantee (cross-tier anchor equality). No scope creep; no architectural change.

## Issues Encountered
- `go test -race` requires `CGO_ENABLED=1` (the race detector needs cgo); the production build/test gates run with `CGO_ENABLED=0` (pure-Go static binary unaffected). Ran the `-race` subset with `CGO_ENABLED=1` per the plan's verify — clean. The race detector is a test-time tool only; the shipped binary is still built `CGO_ENABLED=0`.

## Threat Model Coverage

| Threat ID | Disposition | How addressed |
|-----------|-------------|---------------|
| T-03-11 (tampering, file reads) | mitigated | All attachment/heading reads via `repo.Read`/`repo.Exists`/`repo.Tree`; no `os.*`; missing `.txt` tolerated without erroring. |
| T-03-12 (info disclosure, trash) | mitigated | Query drops heading/attachment hits whose owning `page_path` is under the trash prefix (extended from page-only). |
| T-03-13 (supply chain, npm) | mitigated | rehype-slug legitimacy checkpoint (pre-authorized rehypejs/github-slugger plugin); `package-lock.json` committed; rehype-raw OFF. |
| T-03-14 (sanitize schema EoP) | mitigated | Only the `id` clobber entry removed (id already allow-listed); no tag/attribute relaxed; rehype-raw OFF; PageView XSS test re-run green. |
| T-03-15 (spoofing, anchor mismatch) | mitigated | Go slug mirrors github-slugger incl. de-dup; `TestScanHeadings_DuplicateSlug` + the renderer-id PageView test assert parity. |

## Known Stubs
None. SRCH-04/05/06-heading are fully wired end-to-end (rebuild + incremental indexPage paths). The only forward-deferral is INCREMENTAL mutation-hook wiring into the page/attachment services (03-04); correctness today is guaranteed by the idempotent rebuild + startup drift check, and the `UpsertAttachmentPayload`/`DeleteAttachmentPayload` helpers are provided for 03-04 to call.

## Next Phase Readiness
- 03-04 can wire `search.Upsert/DeletePagePayload` and `search.Upsert/DeleteAttachmentPayload` into the pages/attachments mutation paths (fire-and-forget enqueue) to keep these doc types fresh between rebuilds.
- The ⌘K palette (03-02) already renders heading/attachment rows; the renderer now assigns matching ids, so a heading result deep-links to its section.

## Self-Check: PASSED
- internal/okf/headings.go, internal/okf/headings_test.go: FOUND
- internal/search/headings.go, internal/search/attachments.go: FOUND
- web/src/components/MarkdownProse.tsx: FOUND
- .planning/phases/03-search/03-03-SUMMARY.md: FOUND
- rehype-slug in web/package.json: FOUND
- Commits 9d625b1, 056cde5, 642c60f: FOUND in git log
- Gates: `CGO_ENABLED=0 go build ./...` green; `CGO_ENABLED=0 go test ./...` green; `go test -race` (CGO_ENABLED=1) subset green; `npm run build` + `tsc --noEmit` + `vitest run` (116 tests) green.

---
*Phase: 03-search*
*Completed: 2026-06-21*
