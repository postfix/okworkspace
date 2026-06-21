---
phase: 03-search
reviewed: 2026-06-21T00:00:00Z
depth: standard
files_reviewed: 29
files_reviewed_list:
  - internal/search/index.go
  - internal/search/mapping.go
  - internal/search/query.go
  - internal/search/rebuild.go
  - internal/search/indexjob.go
  - internal/search/doc.go
  - internal/search/attachments.go
  - internal/search/headings.go
  - internal/search/highlight.go
  - internal/search/meta.go
  - internal/search/service.go
  - internal/okf/headings.go
  - internal/gitstore/headsha.go
  - internal/server/handlers_search.go
  - internal/server/router.go
  - internal/pages/service.go
  - internal/pages/rename.go
  - internal/pages/trash.go
  - internal/attachments/service.go
  - internal/attachments/lifecycle.go
  - internal/attachments/extractjob.go
  - cmd/okf-workspace/main.go
  - web/src/components/search/SearchPalette.tsx
  - web/src/components/search/SearchResultRow.tsx
  - web/src/components/search/highlight.ts
  - web/src/store/searchStore.ts
  - web/src/hooks/useSearch.ts
  - web/src/api/client.ts
  - web/src/routes/AppShell.tsx
findings:
  critical: 2
  warning: 6
  info: 4
  total: 12
status: resolved
resolved_at: 2026-06-21
resolution:
  fixed: [CR-01, CR-02, WR-01, WR-02, WR-03, WR-04, WR-05, WR-06, IN-02]
  accepted: [IN-01, IN-03, IN-04]
---

# Phase 3: Code Review Report

**Reviewed:** 2026-06-21
**Depth:** standard
**Files Reviewed:** 29
**Status:** resolved (fixed 2026-06-21)

## Resolution Summary (2026-06-21)

All 2 criticals and all 6 warnings were fixed, each in its own atomic commit with
a regression test. `go build ./...` + `go test ./...` and `npm run build` + `tsc
--noEmit` + `vitest run` are green after every fix.

**Fixed:**

- **CR-01** — `RebuildIndex` now persists `last_indexed_head` via an attached
  gitstore handle (`Index.SetGit`, wired in `main.go`), so `DriftCheck` compares
  stored vs current HEAD and only rebuilds on a real mismatch. Regression test:
  `TestRebuild_PersistsHead`.
- **CR-02** — SPA strips the leading `#` from the stored anchor before
  `getElementById` and uses it as the route hash, so heading deep-links land on
  the section. Tests: Go scanner/element-id parity (`TestScanHeadings_AnchorMatchesElementID`)
  + SPA navigation/`stripAnchorHash` tests.
- **WR-01** — `swapDir` moves the live dir to a `.old` backup before renaming and
  rolls back + reopens (or falls back to a fresh empty index) on failure, so a
  failed swap no longer leaves search dead for the process lifetime.
- **WR-02** — `search.OpenOrRecover` wipes + recreates a corrupt index and signals
  a rebuild instead of aborting startup; `main.go` enqueues the recovery rebuild.
  Tests: `TestOpenOrRecover_FreshDir`, `TestOpenOrRecover_CorruptIndex`.
- **WR-03** — `renderHighlight` decodes the server's HTML entities back to
  characters (React re-escapes on render), fixing visible `&lt;`/`&amp;` while
  keeping the weight-only no-raw-HTML XSS guard. Tests: `highlight.test.tsx`.
- **WR-04** — trashing a page now enqueues a search delete for each attachment it
  owns (looked up via the attachments table), so trashed-page attachments leave
  the index. Test: `TestTrashEnqueuesAttachmentDeletes`.
- **WR-05** — `normalizeQuery` caps on runes, never splitting a multibyte rune.
  Tests: `TestNormalizeQuery_RuneBoundary`, `TestNormalizeQuery_ShortUnchanged`.
- **WR-06** — rebuild logs-and-skips a single unreadable file or failed
  `batch.Index` instead of aborting the whole rebuild. Test:
  `TestRebuild_TolerantOfUnreadableFile`.
- **IN-02** — moved the test-only `typeFacetCount` helper into a `_test.go` file.

**Accepted (not changed):**

- **IN-01** — resolved as part of CR-01 (`StoreHead` is now wired and covered by
  `TestRebuild_PersistsHead`); no separate change needed.
- **IN-03** — magic-number tunables are fine for MVP; deferred per the review.
- **IN-04** — empty-text headings are internally consistent and slug-aligned with
  github-slugger; skipping them would risk de-dup parity, so left as-is.

## Summary

Phase 3 wires Bleve full-text search into the existing single-writer job spine. The
core invariants the phase set out to protect are genuinely upheld and verified:

- **CR-01 fire-and-forget is honored at every new enqueue site.** All `KindIndex`
  enqueues from inside worker handlers (`extractjob.go:147`) and from HTTP-handler
  goroutines (`pages/*`, `attachments/*`, `handlers_search.go`) use `worker.Enqueue`,
  never `EnqueueAndWait`. `IndexHandler` never re-enqueues. Confirmed by grepping all
  call sites.
- **Concurrent read/write safety** in `withIndex` holds the RLock across the whole op,
  and the rebuild swap takes the write lock — no torn-handle race.
- **XSS guard** is real: the server registers a custom `<strong>`-only highlighter that
  HTML-escapes surrounding text (`highlight.go`), and the SPA `renderHighlight` maps only
  the two known weight markers, treating any other tag as escaped plain text — no
  `dangerouslySetInnerHTML` anywhere.
- **Path safety** for all index file reads routes through `repo.Read`/`repo.Tree`/
  `repo.Resolve`; the index dir lives under `<data_dir>/index`, outside the content repo.
- **Trash exclusion** is enforced at both the rebuild/upsert path and defensively at
  query time.

Two BLOCKERs were found. The most important: **the drift-detection mechanism — the
documented "primary correctness backstop" of the phase — is non-functional.**
`last_indexed_head` is never written, so the startup HEAD-drift check is permanently
mis-firing. A heading deep-link bug (`#`-prefixed `getElementById`) makes every heading
result land at the page top. Several WARNINGs cover degraded recovery states and a
visible double-escaping defect in snippets.

## Critical Issues

### CR-01: Drift detection never works — `last_indexed_head` is never persisted

**File:** `internal/search/meta.go:49-57`, `internal/search/rebuild.go:31-139`, `internal/search/index.go:23-31`, `cmd/okf-workspace/main.go:248-256`

**Issue:** `StoreHead` (the only function that writes `last_indexed_head`) is **dead
code — never called anywhere** in the codebase (verified by grep). `RebuildIndex`'s own
doc comment claims "After a successful swap it records the current HEAD as
last_indexed_head" (`rebuild.go:30`), but `RebuildIndex` never calls `StoreHead`, and it
**cannot** — the `Index` struct holds only `repo` and `db`, no `gitstore`/`headProvider`
handle, so the rebuild path (whether triggered by startup drift, admin reindex, or the
KindIndex `rebuild` op) has no way to fetch HEAD.

Consequences:
1. `last_indexed_head` stays empty forever. `DriftCheck` (`meta.go:65-74`) compares the
   stored value (`""`) to the current HEAD. In any non-empty repo `"" != <sha>` is always
   true, so **every server startup reports drift and enqueues a full rebuild-from-files**
   (`main.go:248-256`) — wasteful, and it masks real drift because drift is always
   "detected" regardless of actual state.
2. The mechanism that is supposed to catch genuine out-of-band changes (a `git pull`, a
   restore, a crash between commit and index) provides no signal at all — it is
   indistinguishable from the always-true case. The phase's stated primary correctness
   backstop is inoperative.

**Fix:** Give the index a way to record HEAD after every successful rebuild. Either give
`RebuildIndex` access to a `headProvider` and call `StoreHead` at the end, or have the
rebuild caller do it. Concretely, attach gitstore to the index (mirroring `SetRepo`/
`SetDB`) and call it at the tail of `RebuildIndex`:

```go
// index.go
func (s *Index) SetGit(gs headProvider) { s.gs = gs }

// rebuild.go — after the successful swap + heading-row rewrite:
if s.gs != nil {
    if err := s.StoreHead(ctx, s.gs); err != nil {
        return fmt.Errorf("search: record last-indexed head: %w", err)
    }
}
```

Wire `searchIdx.SetGit(gs)` in `main.go` alongside `SetRepo`/`SetDB`. Add a test that
asserts `last_indexed_head` is non-empty after a rebuild and that a second startup with an
unchanged HEAD reports `DriftCheck == false`.

### CR-02: Heading deep-links never scroll — `getElementById` is called with a `#` prefix

**File:** `web/src/components/search/SearchPalette.tsx:125-132`, `internal/okf/headings.go:64`

**Issue:** `okf.ScanHeadings` stores the anchor WITH a leading `#`:
`Anchor: "#" + anchor` (`headings.go:64`), and `headingDoc` indexes that verbatim. The SPA
then does:

```ts
const anchor = r.anchor;                 // e.g. "#deploy-runbook"
document.getElementById(anchor)?.scrollIntoView();
```

`document.getElementById("#deploy-runbook")` matches an element whose id is literally
`#deploy-runbook`. But `rehype-slug` (wired in `MarkdownProse.tsx:79`, and which the Go
scanner is explicitly designed to mirror) assigns ids WITHOUT the `#` —
`id="deploy-runbook"`. So the lookup always returns `null` and **every heading result
lands at the top of the page instead of the target section** — defeating SRCH-06's
deep-link value. (The inline comment claims "no-op deep-link until 03-03," but
`ScanHeadings`/heading indexing/heading querying are all live in this phase, so heading
results are returned and the deep-link is broken now, not deferred.)

**Fix:** Strip the leading `#` before the DOM lookup (it is needed for a URL hash but not
for `getElementById`):

```ts
if (r.kind === "heading" && r.anchor) {
  const id = r.anchor.replace(/^#/, "");
  window.requestAnimationFrame(() => {
    document.getElementById(id)?.scrollIntoView();
  });
}
```

Add a test fixture page with a heading and assert the slug-without-`#` matches the
rendered element id.

## Warnings

### WR-01: Failed `swapDir` leaves the index permanently closed until restart

**File:** `internal/search/index.go:94-123`

**Issue:** `swapDir` removes the live dir, then renames tmp into place:

```go
if err := os.RemoveAll(s.dir); err != nil { ... }
if err := os.Rename(tmp, s.dir); err != nil {
    return fmt.Errorf("search: swap index dir: %w", err)
}
```

If `os.Rename` fails (cross-device tmp, permission, race), `s.idx` has already been set to
`nil` and the old dir has already been `RemoveAll`'d. The function returns an error, but
`s.idx` is left `nil`, so **every subsequent `Query`/`indexPage`/`deletePage` returns
"search: index is closed" until the process restarts** — search is dead for the lifetime
of the process, with no in-process recovery. The on-disk index is also gone (RemoveAll
ran), so the only recovery is restart + rebuild.

**Fix:** Reopen the old dir (or re-create an empty index) on the failure path so the
service degrades to a usable state, and/or rename old→backup before removing so a failed
rename can roll back:

```go
backup := s.dir + ".old"
_ = os.RemoveAll(backup)
if err := os.Rename(s.dir, backup); err != nil { return ... } // old now safe
if err := os.Rename(tmp, s.dir); err != nil {
    _ = os.Rename(backup, s.dir) // roll back
    if rb, oerr := bleve.Open(s.dir); oerr == nil { s.idx = rb }
    return fmt.Errorf("search: swap index dir: %w", err)
}
_ = os.RemoveAll(backup)
```

### WR-02: Corrupt-index open path does not auto-rebuild as designed

**File:** `internal/search/index.go:48-57`, `cmd/okf-workspace/main.go:208-211`

**Issue:** `openOrCreateBleve` returns the error on any non-`ErrorIndexPathDoesNotExist`
open failure, and the doc comment + Pitfall 3 say the caller should "log and trigger a
full rebuild into a fresh dir rather than repairing in place." But `main.go:208-211`
treats that error as fatal:

```go
searchIdx, err := search.OpenOrCreate(indexDir)
if err != nil {
    return fmt.Errorf("open search index: %w", err)   // server refuses to start
}
```

So a corrupt scorch segment after an unclean shutdown — exactly the case Pitfall 3 says to
recover from — **takes the whole server down** instead of rebuilding from files. The
designed self-healing path is not implemented.

**Fix:** On a non-"does-not-exist" open error, log it, `os.RemoveAll(indexDir)`, recreate
a fresh empty index, and enqueue a rebuild (the rebuild path already exists and is
idempotent). Do not abort startup.

### WR-03: Snippet entities render double-escaped (visible `&lt;` / `&amp;` in results)

**File:** `web/src/components/search/highlight.ts:34-51`, `internal/search/highlight.go:28-38`

**Issue:** The server formatter HTML-escapes the surrounding fragment text
(`html.EscapeString`), so a snippet arrives as e.g.
`config &lt;value&gt; <strong>match</strong>`. `renderHighlight` splits on the `<strong>`
markers and pushes the remaining text as React text nodes. React renders text nodes
verbatim, so `&lt;value&gt;` displays to the user as the literal characters
`&lt;value&gt;` rather than `<value>`. Any page body containing `<`, `>`, `&`, or quotes
will show raw HTML entity codes in search snippets — a user-visible correctness defect
(not a security issue; the escaping is what keeps it safe).

**Fix:** Have the server return plain (unescaped) fragment text and rely on React's own
text-node escaping for safety (React escapes on render, so raw `<value>` text is rendered
as literal text safely), OR decode the known entities in `renderHighlight` before pushing
the text node. The cleanest fix is a fragment formatter that does NOT pre-escape, paired
with the existing marker-only client parser — React then handles escaping and the snippet
reads correctly.

### WR-04: Attachment results whose owning page is trashed are not filtered

**File:** `internal/search/query.go:117-126`, `internal/pages/trash.go:43-110`

**Issue:** When a page is moved to trash, `enqueueIndexDelete(pagePath)` removes the page +
heading docs, but attachment docs are NOT touched — their `page_path` still points at the
(now-trashed) original page path. The query-time trash filter checks
`isTrashed(r.Path)` where `r.Path` is the attachment's `page_path`. Since the attachment's
stored `page_path` is the original live path (e.g. `notes/foo.md`, not
`.okf-workspace/trash/...`), `isTrashed` returns false and the **attachment of a trashed
page remains searchable**, with a result that deep-links to a page that no longer exists at
that path. Area 4 requires trashed-page content be excluded from results.

**Fix:** When trashing a page, also enqueue deletes for that page's attachment docs (or
re-point/remove them). Alternatively, at query time, drop attachment hits whose owning
page no longer resolves (`repo.Exists(page_path)` false) — but the enqueue-on-trash
approach is cleaner and matches the incremental discipline used elsewhere.

### WR-05: `normalizeQuery` truncates on byte boundary, can split a multibyte rune

**File:** `internal/search/query.go:82-91`

**Issue:** `normalizeQuery` caps length with a byte slice: `q = q[:maxQueryLen]`. For a
query containing multibyte UTF-8 (CJK, accented Latin, emoji) at the 256-byte boundary,
this can slice mid-rune, producing an invalid UTF-8 string that is then fed to the Bleve
analyzer. At best it drops the partial term; at worst the analyzer behaves unexpectedly on
the malformed tail. The cap is a DoS guard (good), but should respect rune boundaries.

**Fix:** Truncate on runes:

```go
if len(q) > maxQueryLen {
    r := []rune(q)
    if len(r) > maxQueryLen { r = r[:maxQueryLen] }
    q = string(r)
}
```

(Or use `utf8.DecodeLastRuneInString` to back off to a valid boundary.)

### WR-06: Rebuild reads every page/attachment but a `repo.Read` IO error aborts the whole rebuild

**File:** `internal/search/rebuild.go:62-67`

**Issue:** Inside the rebuild walk, a transient `repo.Read` failure on a single file
(`rerr != nil`) tears down the entire tmp index and aborts the rebuild with an error:

```go
raw, rerr := s.repo.Read(it.Path)
if rerr != nil {
    _ = tmpIdx.Close(); _ = os.RemoveAll(tmp)
    return fmt.Errorf("search: read %q: %w", it.Path, rerr)
}
```

A malformed-parse case a few lines down is correctly tolerated (skip, continue), but an
IO/resolver error on one file kills the rebuild — meaning one unreadable file makes search
permanently un-rebuildable (startup drift then re-enqueues a rebuild that fails again).
Given the rebuild is the correctness backstop, a single bad read should be best-effort
like the parse case.

**Fix:** Log and `continue` on a single-file read error rather than aborting the whole
rebuild (or distinguish truly fatal errors, e.g. resolver path-escape, from transient IO).
The same applies to the per-heading and per-attachment `batch.Index` error paths — a single
bad doc should not nuke the whole index build.

## Info

### IN-01: `StoreHead` is dead code (will mask the CR-01 regression once fixed)

**File:** `internal/search/meta.go:49-57`

**Issue:** Beyond CR-01, `StoreHead` being unreferenced is itself a smell — Go does not
flag unused methods, so this silently shipped. Once CR-01 is fixed, add a test that
exercises it, otherwise a future refactor could re-orphan it unnoticed.

**Fix:** Wire it (per CR-01) and cover it with a test.

### IN-02: `typeFacetCount` is test-only helper in production code

**File:** `internal/search/query.go:166-191`

**Issue:** `typeFacetCount`'s own comment says "Used by tests to assert the type facet is
present." A test-only method living in the production file (not `_test.go`) adds surface
and an extra index search path that ships in the binary.

**Fix:** Move it into a `query_test.go` export-test helper, or inline the facet assertion
into the test using the public `Query` + a facet accessor.

### IN-03: `maxResults`/`facetSize`/`fragmentSize` etc. are reasonable but undocumented as tunables

**File:** `internal/search/query.go:23-35`, `internal/search/highlight.go:19-20`

**Issue:** Magic-number caps (`maxResults=50`, `maxQueryLen=256`, `fragmentSize=200`,
`fuzziness=1`) are fine for a 5-user workspace but are compile-time constants with no
config surface. Not a bug; noting for future tuning (Phase deferred "ranking tuning").

**Fix:** None required for MVP; consider moving to `SearchConfig` if tuning is needed.

### IN-04: `Index` op on a heading whose slug is empty yields ID `pagePath + "#"`

**File:** `internal/okf/headings.go:124-141`, `internal/search/doc.go:11-13`

**Issue:** A heading consisting only of dropped characters (e.g. `# !!!`) slugs to `""`, so
the anchor is `"#"` and the heading doc ID is `pagePath + "#"`. This is internally
consistent (de-dup gives `""`, `-1`, ... for repeats) and matches what github-slugger /
rehype-slug produce for the same input, so deep-links still align — but an empty-text
heading is an odd result row (blank title). Behaviorally acceptable; flagged for awareness.

**Fix:** Optionally skip headings whose trimmed text is empty in `ScanHeadings`.

---

_Reviewed: 2026-06-21_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
