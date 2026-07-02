---
author: codebase-archaeologist
responsible: codebase-archaeologist
phase: 32, 33, 34, 35
milestone: v1.6
status: verified
confidence: direct_read
grounding: direct file read (SMTC daemon offline this session — no semantic/blast-radius certs; caller/consumer claims from grep + direct read)
head: 1653933
---

# Codebase Map: Tag-Metadata Subsystem (tagmeta) + tag-op journal

## Subsystem: a committed, files-as-truth store for tag ADJUNCT data (color / description / parent), plus the undo op-journal

The v1.6 milestone's central new subsystem (Phases 32–35). Before v1.6 a tag had NO
on-disk identity of its own: the `page_tags` cache (migration 0009) is derived from page
*frontmatter*, and a tag existed only as a string carried by some page. v1.6 gives a tag
its own durable record — **`internal/tagmeta`** — carrying color, a one-line description,
and an optional parent (a cycle-safe display hierarchy). Two adjacent read/undo capabilities
ride the same tag-admin surface: **usage analytics** (Phase 34, a read-only view over the
existing `page_tags` counts, sourcing orphans from this store) and **tag-operation undo**
(Phase 35, a dedicated `tag_op_journal` that replays the inverse of the last tag-admin op
through `ApplyTagsBatch`).

The design mirrors the existing files-as-truth discipline exactly: tag metadata is CONTENT
(one `Tags/{tag}.md` Markdown file per tag), and the `tag_meta` SQLite table is a
**rebuildable derived cache** — `RebuildTagMeta` reproduces identical rows from disk, the same
contract migration 0009 (`page_tags`) holds. The write path reuses the page-save single-writer
commit spine (`pages.CommitEnqueuer` / `EnqueueCommit`) — **never a second commit kind**. The
`tag_op_journal` (Phase 35), by contrast, is operational/derived state (machine-generated undo
records, not user-authored content) and lives ONLY in SQLite — mirroring `tag_suggestions`
(migration 0010), not the files-as-truth store.

> **Relationship to `MAP-tag-rewrite-subsystem.md`.** That map owns the v1.5 body+frontmatter
> rewrite engine (`FindHashtags`/`RewriteHashtags`/`ApplyTagsBatch`). THIS map owns the v1.6
> tag-metadata store and the undo journal. They meet at exactly one seam: undo builds an
> **inverse `ApplyTagsBatch`** — see "The undo op-journal" below and MAP-tag-rewrite §
> "The v1.6 undo integration".

### Files

| File | Role |
|------|------|
| `internal/tagmeta/meta.go` | Package doc (files-as-truth invariant) + the `Meta{Tag, Color, Description, Parent}` record — one parsed tag-metadata row. `Parent==""` = root. |
| `internal/tagmeta/validate.go` | The three input gates. `ValidateColor` — hex `#RGB`/`#RRGGBB` OR a fixed palette token (`red/blue/green/amber/purple/gray`); `""` = clear; everything else (`url()`, `var()`, expressions, arbitrary CSS) rejected (the color flows into a `style.color` on badge DOM — this is the trust-boundary gate). `ValidateDescription` — single-line, ≤200 chars, rejects HTML metacharacters `<>&"'` (defense-in-depth; rendered as textContent, never HTML). `ValidateParent` — a valid tag name (`okf.ValidateTagCharset`) or empty; does NOT check existence (non-existent parent = root, D-02b). |
| `internal/tagmeta/store.go` | The `Store` (holds `*repo.Repo` = SEC-01 resolver, `*sql.DB` = cache, a `commitEnqueuer` = the shared page-save commit path, `commitUsr` = threaded actor). Public surface: `GetMeta`/`AllMeta` (cache reads), `SetMeta`/`ClearMeta` (write path), `RebuildTagMeta` (cache rebuild from disk), `CheckCycle` (exported so the server handler can gate before the write), `SetUser`/`SetRepo`/`NewStore`. Internal: `metaFile` (on-disk shape), `tagPath`, `writeAndCommit`/`removeAndCommit`/`clearFileAndCache`, `upsertCache`/`deleteCache`, `parseMetaFile`, `isTagMetaFile`. Const `tagsDir="Tags"`, `maxCycleDepth=100`. |
| `internal/store/migrations/0011_tag_meta.sql` | `tag_meta(tag PK, color, description)` — the derived cache (Phase 32). Rebuildable from `Tags/*.md`; SQLite is NEVER the source of truth (mirrors 0009_graph tone). |
| `internal/store/migrations/0012_tag_meta_parent.sql` | Additive `ALTER TABLE tag_meta ADD COLUMN parent TEXT NOT NULL DEFAULT ''` (Phase 33). Auto-discovered by version number — no registration. |
| `internal/store/migrations/0013_tag_op_journal.sql` | `tag_op_journal(id PK AUTOINCREMENT, op_type, op_detail JSON, pages JSON, created_at)` — the append-only undo journal (Phase 35). Operational/derived state, NOT content (mirrors 0010_tag_suggestions tone). |
| `internal/server/handlers_tags_admin.go` | The HTTP surface for all four v1.6 tag-admin additions: `handleTagMeta` (POST /admin/tags/meta), `handleTagAnalytics` (GET /admin/tags/analytics), `handleTagBulk` (POST /admin/tags/bulk), `handleTagUndo` (POST /admin/tags/undo) + `journalOp` (the journaling helper hooked into rename/merge/delete/bulk). `handleTagList` extended to merge color/description/parent from `tagmeta.AllMeta`. Types: `tagMetaRequest`/`Response`, `tagBulkRequest`, `tagAnalyticsResponse`/`tagCountPair`, `journalPage`/`journalRecord`. |
| `internal/graph/query.go` | `TaggedPageCount` (new Phase-34 method: `SELECT COUNT(DISTINCT page_path) FROM page_tags`, nil-db-safe) — the analytics tagged-page total. `TagPageCounts` (pre-existing) is the analytics per-tag count source (no new heavy scan). |
| `internal/pages/tree.go` + `internal/graph/rebuild.go` + `internal/graph/query.go` + `internal/search/rebuild.go` | The `Tags/` **reserved-folder skip**: `Tags/` is added to `isSkippedDir`/rebuild skips so the metadata folder is not a navigable page tree entry, not graph-linked, and not search-indexed (it is reserved store content, not wiki pages). |
| `cmd/okf-workspace/main.go` | Wires `tagmetaStore := tagmeta.NewStore(contentRepo, db, worker, push)` after `tagsweepStore` (**reusing the SAME jobs worker** for the single-writer commit path) and passes it as `server.Deps.Tagmeta`. |
| `internal/tagmeta/store_test.go` | 15 tests: SetGet, RebuildTagMeta, RebuildParity, EmptyStore, ValidateColor, ValidateDescription, SetMetaWritesFile, ClearMetaDeletesFile, RebuildSortsByTag (Phase 32) + SetMetaWithParent, CycleDetection_SelfParent, CycleDetection_Indirect, CycleDetection_DeepChain, NonExistentParent, RebuildParityWithParent (Phase 33). |
| `internal/server/handlers_tags_admin_test.go` | The HTTP-layer tests: `TestTagMeta*` (RBAC/SetGet/Validation/Clear/Absent), `TestTagHierarchy*` (SetParent/Cycle/SelfParent/RootTag), `TestTagAnalytics*` (RBAC/full/Empty), `TestBulkTag*` (RBAC/Preview/Golden/Stale/FilterSelection/Validation), `TestJournal*` + `TestUndo*` (9: Rename/Bulk journal rows; RBAC/Nothing/RenameGolden/Stale/Delete/Bulk/OfUndo undo). |

### The files-as-truth store (Phase 32, TF-02)

```
Tags/{tag}.md                       ← SOURCE OF TRUTH (committed content)
   │  YAML frontmatter: tag, color, description, parent   +  a `# {tag}` heading body
   │  (metaFile renders it; the body is free-form human docs, ignored by the cache)
   ▼
tag_meta (SQLite, migration 0011+0012)   ← DERIVED CACHE (rebuildable)
   │  RebuildTagMeta: DELETE all + re-INSERT from Tags/*.md in ONE txn
   │  parity harness: delete-cache → rebuild = identical rows (TestRebuildParity*)
   ▼
GetMeta / AllMeta                    ← the read path (cache-backed; nil db → empty, graceful)
```

- **`SetMeta(ctx, tag, color, description, parent)`** — validates all three fields, runs
  `CheckCycle` BEFORE the write (D-02a), then: when color+description+parent are ALL empty →
  DELETE the file (`clearFileAndCache`); otherwise `metaFile()` → `writeAndCommit` (single-writer
  `EnqueueCommit`) → `upsertCache`. An immediate read sees the new metadata (the cache is upserted
  synchronously after the commit enqueue).
- **`RebuildTagMeta(ctx)`** — walks `Tags/*.md` via `repo.Tree()` + the `Tags/` prefix filter +
  `repo.Read` + `okf.Parse`, parses each frontmatter into a `Meta`, and repopulates the cache in
  ONE transaction. A repo with no `Tags/` folder → empty cache (graceful, not an error). Mirrors
  `graph.RebuildGraph`.
- **Graceful degradation:** a nil `db` returns empty reads (the on-disk files stay truth). A tag
  with NO metadata renders exactly as pre-v1.6 (empty color/description → CSS default badge). The
  whole subsystem is ADJUNCT — tags are fully usable with the store absent.

### The cycle-safe hierarchy (Phase 33, TF-03 — display-only)

- `Meta.Parent` is a normalized parent tag name (`""` = root). It is stored in
  `Tags/{tag}.md` frontmatter (`parent:` always written, even empty, so rebuild reproduces it)
  and the `tag_meta.parent` cache column.
- **`CheckCycle(ctx, tag, parent)`** rejects a cycle-producing parent BEFORE the write (D-02a —
  a cycle is never persisted to disk, which would be un-rebuildable). It walks the parent chain
  upward via the cache: `parent==tag` → self-parent (reject); revisiting `tag` → cycle (reject);
  chain > `maxCycleDepth` (100) → corrupted store (reject); reaching root or a non-existent
  parent → valid. O(depth) — the hierarchy is shallow for a ~5-user wiki.
- **Display-only:** the hierarchy is grouped-list presentation in the admin UI. There is NO
  auto-apply of parent tags to pages, NO descendant-expanding search, NO hierarchy graph
  grouping — those are explicitly deferred to a Future "hierarchy-aware tag behavior" item.
  The server handler (`handleTagMeta`) runs `CheckCycle` as the authoritative gate (400 on cycle);
  the UI's self-exclusion in the parent `<select>` is a convenience, not the enforcement.

### The color trust boundary (why `ValidateColor` is the load-bearing gate)

The color is the ONE metadata field that reaches a DOM sink other than textContent: the badge
widget applies it as `span.style.color = color` (`web/src/lib/cm/widgets.ts` `HashtagWidget`,
Phase 32) — **NO innerHTML** (the no-innerHTML XSS invariant holds; color flows through
`style.color` only). `ValidateColor` is therefore restricted to a hex color or a fixed palette
token at the SERVER gate — `url()`/`var()`/expressions/arbitrary CSS are a clean 400. Extending
the palette set is a deliberate decision, never an open door to arbitrary CSS. (Confirmed:
`grep 'innerHTML' web/src/lib/cm/widgets.ts` in the HashtagWidget section returns no match;
the color enters only via `style.color`.)

### Usage analytics (Phase 34, TF-04 — read-only)

`handleTagAnalytics` (GET /admin/tags/analytics, admin-only, read-only) computes a
`tagAnalyticsResponse{total_tags, total_pages, tagged_pages, untagged_pages, most_used,
least_used, orphans, tags}` with NO new heavy scan:

- per-tag counts from `graph.Store.TagPageCounts` (the existing `page_tags` cache query),
- `tagged_pages` from the new `graph.Store.TaggedPageCount` (`COUNT(DISTINCT page_path)`),
- `total_pages` from `pages.Service.Tree()` (recursive live-`.md` count; falls back to
  `taggedPages` when the pages service is nil so untagged is never negative),
- `most_used`/`least_used` = top/bottom 5 by count, tie-broken alphabetically,
- **`orphans`** = tags defined in `tagmeta.AllMeta` but carried by ZERO pages (count 0 in
  `TagPageCounts`). This is the ONE place analytics reads the tagmeta store: the graph vocabulary
  always has count ≥1 (it comes FROM `page_tags`), so a metadata-only tag (defined via
  `Tags/{tag}.md`, not yet on any page) is the only source of orphans. Non-fatal on a metadata
  read error (orphans left empty).
- All arrays are non-nil empty slices (JSON `[]`, not `null`). Scope note surfaced in the UI:
  counts reflect FRONTMATTER `page_tags`; body-only `#tags` are counted only after materialization.

### The undo op-journal (Phase 35, TF-05 — inverse ApplyTagsBatch)

```
tag_op_journal (SQLite, migration 0013)      ← append-only; ONLY the LAST row is undoable (v1.6 = last-op undo)
   id, op_type ∈ {rename,merge,delete,bulk,undo}, op_detail JSON, pages JSON [{page_path, pre_op_tags, body_affected}], created_at
```

- **`journalOp(ctx, opType, opDetail, preview)`** — hooked into all four mutating tag-admin
  handlers (`handleTagRename`/`Merge`/`Delete`/`Bulk`) AFTER a successful `ApplyTagsBatch`. It
  marshals the op detail + per-page pre-op state (`CurrentTags` from the preview = the pre-op
  frontmatter tags; `BodyAffected`) and INSERTs one row. **Non-fatal** — a DB error is logged,
  never breaks the request (the op already committed). Requires `authHandlers.store` (optional;
  nil → undo unavailable).
- **`handleTagUndo`** (POST /admin/tags/undo, admin-only + CSRF) reads the LAST journal row, then
  for each page builds an INVERSE `pages.TagApplyItem`:
  - `Tags = pre_op_tags` (restore the original frontmatter tags),
  - `BaseRevision` read **FRESH** via `pages.Get` at undo time (NOT a stored revision — the page
    may have moved; the fresh read is TOCTOU-safe for the 409-guard),
  - `BodyOld`/`BodyNew` **swapped** for rename (`new→old`) / merge (`target→source`); delete and
    bulk carry no body inverse (`BodyOld=BodyNew=""`).
  - A **semantic stale check**: recompute what the op WOULD have produced from `pre_op_tags`
    (`computeRenameTags`/`computeMergeTags`/`computeDeleteTags`/`computeBulkTags`) and compare to
    the page's CURRENT frontmatter tags (`sameTagSet`). A mismatch = the page was edited since the
    op → report `stale`, SKIP (never clobber). `ApplyTagsBatch`'s per-page 409-guard is the
    load-bearing net underneath.
  - One forward `ApplyTagsBatch` commit (never a `git revert`/history rewrite — consistent with
    `pages.RestoreVersion`). Audited `ActionTagUndo` (non-fatal).
  - **Undo journals itself** so undo-of-undo (redo) works for free: the undo row's `pre_op_tags` =
    the tags BEFORE the undo (= post-original-op state), and its `op_detail` carries the ORIGINAL
    op's `{op_type, op_detail}` so a redo replays the ORIGINAL body direction (the `case "undo"`
    branch unwraps it — redo = original direction, NOT a swap-of-swap).

### Bulk multi-tag (Phase 31, TF-01 — no new write path)

`handleTagBulk` (POST /admin/tags/bulk) adds and/or removes multiple tags across a selected page
set (explicit `page_paths` ∪ pages matching any `filter_tags`, resolved server-side at BOTH
preview and apply — TOCTOU-safe) in ONE byte-stable batch. It reuses `computeBulkTags(add, remove)`
→ `(current ∪ add) \ remove` (normalized, deduped, order-preserving) through the EXISTING
`buildTagApplyItems` → `ApplyTagsBatch` seam with `BodyOld=""` (frontmatter-only, no body channel).
Added tags are re-validated via `okf.ValidateTagCharset`; removed/filter tags are matched, not
written. It is journaled (`journalOp("bulk", …)`) so it is undoable like rename/merge/delete.

## Undocumented invariants & landmines (per-claim confidence)

1. **`Tags/` is a RESERVED folder — skipped by tree, graph, and search.** (HIGH — `pages/tree.go`
   `isSkippedDir`, `graph/rebuild.go` + `graph/query.go` + `search/rebuild.go` skips per
   32-SUMMARY.) A future feature that walks pages MUST honor the `Tags/` skip, or the metadata
   store leaks into the navigable page tree / graph nodes / search hits. `isTagMetaFile`
   (`store.go:326`) is the canonical predicate — reuse it, don't re-derive the prefix check.

2. **The `tag_meta` cache is rebuildable; SQLite is NEVER source of truth.** (HIGH — `meta.go:1-11`,
   `0011_tag_meta.sql`, `TestRebuildParity`/`TestRebuildParityWithParent`.) Deleting the table and
   running `RebuildTagMeta` reproduces identical rows from `Tags/*.md`. Any new write MUST go
   through `SetMeta`/`ClearMeta` (which write the file THEN upsert the cache) — a cache-only write
   would be lost on the next rebuild and violate files-as-truth.

3. **The tagmeta write path reuses the page-save commit spine — no second commit kind.** (HIGH —
   `store.go:36-47`, `commitEnqueuer = pages.CommitEnqueuer`; `main.go` reuses the SAME worker.)
   Introducing a parallel commit path here reopens the single-writer-spine invariant the whole
   project rests on. `SetUser` must be called before `SetMeta` so the git commit records the real
   actor (the admin handler does this).

4. **`CheckCycle` runs BEFORE the write (D-02a) and reads the CACHE.** (HIGH — `store.go:134-138,
   345-368`.) It walks the parent chain via `GetMeta` (cache), which is in sync after a rebuild. A
   cycle-producing parent is rejected at the gate — never persisted (an on-disk cycle would be
   un-rebuildable). The server handler calls the exported `CheckCycle` too (400) — do NOT remove
   either gate; the store-level check is the last line, the handler check is the HTTP contract.

5. **A non-existent parent is treated as ROOT, not an error (D-02b).** (HIGH — `validate.go:75-85`,
   `store.go:356-357`, `TestNonExistentParent`.) `ValidateParent` does NOT check existence, and
   `CheckCycle` treats a parent with no cache row as root. A tag can reference a parent that was
   later deleted/renamed; the UI treats a dangling parent as root. Do not "fix" this into a
   referential-integrity error — it is the intended dangling-parent tolerance.

6. **The hierarchy is DISPLAY-ONLY in v1.6.** (HIGH — 33-SUMMARY verification: `grep 'parent'
   internal/pages/ internal/search/` finds only filesystem-path parents, no tag-hierarchy
   auto-apply/search-expansion.) No page rewrite, no search expansion, no graph grouping keys off
   `parent`. Wiring parent into a page-mutation or search path is a NEW feature (Future
   "hierarchy-aware tag behavior"), not a bugfix — route the decision UP.

7. **`ValidateColor` is the color trust boundary; the badge sink is `style.color`, never innerHTML.**
   (HIGH — `validate.go:22-42`; `widgets.ts` HashtagWidget uses `span.style.color`, no innerHTML.)
   Loosening `ValidateColor` to accept arbitrary CSS is a style-injection regression (route to
   security-engineer). The palette set is a fixed allow-list — extend by deliberate decision only.

8. **`tag_op_journal` is operational state (SQLite-only), NOT files-as-truth.** (HIGH —
   `0013_tag_op_journal.sql:1-8`.) Unlike `tagmeta`, the journal is machine-generated undo
   records — correctly SQLite-only (mirrors `tag_suggestions`). Do NOT "promote" it to a
   committed on-disk store; it is transient operational data, and only the LAST row is undoable.

9. **Undo reads a FRESH `BaseRevision` + runs a SEMANTIC stale check — it never clobbers a
   concurrent edit.** (HIGH — `handlers_tags_admin.go:1101-1214`, `TestUndoStale`.) The journal
   stores `pre_op_tags`, NOT revisions; undo reads the current revision fresh AND compares current
   tags to the op's expected output. A page edited since the op is reported `stale` and skipped.
   Removing either guard reopens a lost-update on undo.

10. **Delete's body `#tags` are NOT invertible by undo.** (HIGH — 35-SUMMARY "Limitation",
    `TestUndoDelete`.) `RewriteHashtags(body, tag, "")` SPLICES OUT the `#tag` span — the original
    bytes live in git history, not in the page, so undo of a delete restores the frontmatter
    `tags:` list but does NOT re-insert removed body `#tags` (they self-heal on the next edit via
    the client re-materializer). Surfaced in the undo response `note`. This is a stated limitation,
    not a bug — do not attempt to "fix" it by reconstructing spans without a deliberate decision.

11. **Bulk is journaled + undoable, and rides the EXISTING batch seam (no new write path).**
    (HIGH — `handlers_tags_admin.go:685-687`, `journalOp("bulk",…)`, `TestUndoBulk`.) `handleTagBulk`
    calls `buildTagApplyItems` + `ApplyTagsBatch` with `BodyOld=""` (frontmatter-only). A change to
    the batch seam affects bulk, the sweep-approve path, rename/merge/delete, AND undo at once.

## Blast radius (direct-read estimate; SMTC unavailable this session)

- **`tagmeta.Store`** → consumed only by `handleTagMeta`/`handleTagList`/`handleTagAnalytics`
  (orphans) via `authHandlers.tagmeta` (`server.Deps.Tagmeta`), constructed once in `main.go`.
  A signature change to `SetMeta`/`GetMeta`/`AllMeta`/`CheckCycle` touches exactly those handlers +
  the tests. `RebuildTagMeta` has no HTTP caller yet (candidate for an admin reindex backstop like
  graph/search — verify before wiring).
- **`graph.Store.TaggedPageCount`** → single caller `handleTagAnalytics`. `TagPageCounts` is shared
  with the tag-list count column and analytics.
- **`journalOp`** → 5 call sites (rename/merge/delete/bulk + undo-journals-itself). `handleTagUndo`
  reads the journal. A journal-schema change (columns / JSON shape) touches `journalOp` + the undo
  unmarshalling switch together — they are a matched pair.
- **`ApplyTagsBatch`** (owned by MAP-tag-rewrite) is the shared sink for bulk AND undo — the inverse
  items ride the same body-channel + 409-guard + no-op-guard the forward ops use. See
  MAP-tag-rewrite-subsystem.md for the batch internals.
- **`HashtagWidget` color** (`web/src/lib/cm/widgets.ts`, `livePreview.ts` `tagColors` Facet) → the
  CM6 badge render. A color-plumbing change is client-only; the server gate (`ValidateColor`) is the
  authoritative trust boundary regardless.

### Test corpus (where the invariants are pinned)

`internal/tagmeta/store_test.go` (15: set/get, rebuild parity with+without parent, all three
validators, cycle detection self/indirect/deep-chain, non-existent parent), and
`internal/server/handlers_tags_admin_test.go` (`TestTagMeta*`, `TestTagHierarchy*`,
`TestTagAnalytics*`, `TestBulkTag*`, `TestJournal*`/`TestUndo*`). Treat a `RebuildParity*` failure
as a files↔cache drift (or an un-rebuilt cache), and a `TestUndoStale`/`TestUndoOfUndo` failure as
an undo-correctness regression, not a flaky test.
