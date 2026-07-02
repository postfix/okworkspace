# Tag-Metadata Subsystem (color / description / hierarchy) + Undo & Analytics

The tag-metadata subsystem gives a tag its own durable, on-disk identity — color, a
one-line description, and an optional parent (a cycle-safe display hierarchy) — plus
two adjacent capabilities that ride the same admin surface: **usage analytics** (a
read-only view over the existing tag-count cache) and **tag-operation undo** (replaying
the inverse of the last tag-admin op through the byte-stable batch path). Shipped across
v1.6 Phases 32–35 (TF-02..05).

Before v1.6 a tag had **no on-disk record of its own**: the `page_tags` cache (migration
0009) is derived from page *frontmatter*, so a tag existed only as a string carried by some
page. This subsystem adds a first-class tag record — as **content** (one `Tags/{tag}.md`
file per tag, files-as-truth), with a rebuildable SQLite cache — while keeping tags fully
usable when the store is absent (the metadata is strictly adjunct).

**Package:** `internal/tagmeta/`
**Store cache:** `tag_meta` (migrations `0011_tag_meta.sql`, `0012_tag_meta_parent.sql`)
**Undo journal:** `tag_op_journal` (migration `0013_tag_op_journal.sql`)
**HTTP surface:** `internal/server/handlers_tags_admin.go` (see [tag-admin.md](../api/tag-admin.md))

See the deep map in [`.planning/codebase/MAP-tagmeta-subsystem.md`] for callers, blast radius,
and the full landmine set; this doc is the contributor-facing distillation.

---

## Source files

- [`internal/tagmeta/meta.go`](../../internal/tagmeta/meta.go) — package doc (files-as-truth invariant) + the `Meta{Tag, Color, Description, Parent}` record.
- [`internal/tagmeta/validate.go`](../../internal/tagmeta/validate.go) — `ValidateColor` / `ValidateDescription` / `ValidateParent` (the three server-side gates).
- [`internal/tagmeta/store.go`](../../internal/tagmeta/store.go) — `Store`: `GetMeta` / `AllMeta` / `SetMeta` / `ClearMeta` / `RebuildTagMeta` / `CheckCycle` + the single-writer commit + cache upsert internals.
- [`internal/store/migrations/0011_tag_meta.sql`](../../internal/store/migrations/0011_tag_meta.sql) / [`0012_tag_meta_parent.sql`](../../internal/store/migrations/0012_tag_meta_parent.sql) — the derived cache table + additive parent column.
- [`internal/store/migrations/0013_tag_op_journal.sql`](../../internal/store/migrations/0013_tag_op_journal.sql) — the append-only undo journal.
- [`internal/server/handlers_tags_admin.go`](../../internal/server/handlers_tags_admin.go) — `handleTagMeta` / `handleTagAnalytics` / `handleTagBulk` / `handleTagUndo` + `journalOp`; `handleTagList` extended with color/description/parent.
- [`internal/graph/query.go`](../../internal/graph/query.go) — `TagPageCounts` (analytics per-tag counts) + `TaggedPageCount` (new: distinct tagged pages).
- [`web/src/lib/cm/widgets.ts`](../../web/src/lib/cm/widgets.ts) — `HashtagWidget` color plumbing (`span.style.color`, no innerHTML).
- [`web/src/components/admin/TagManagement.tsx`](../../web/src/components/admin/TagManagement.tsx) — the admin UI (metadata dialog, grouped hierarchy list, analytics panel, bulk dialog, undo button).

---

## Files-as-truth store (Phase 32, TF-02)

```
Tags/{tag}.md                       SOURCE OF TRUTH (committed Markdown content)
   | frontmatter: tag, color, description, parent   +  a `# {tag}` heading body
   |   (metaFile() renders it; the body is free-form human docs, ignored by the cache)
   v
tag_meta (SQLite, migrations 0011 + 0012)   DERIVED CACHE (rebuildable)
   | RebuildTagMeta: DELETE all + re-INSERT from Tags/*.md in ONE transaction
   | parity: delete-cache -> rebuild = identical rows
   v
GetMeta / AllMeta                   the read path (cache-backed; nil db -> empty, graceful)
```

- **`SetMeta(ctx, tag, color, description, parent)`** validates all three fields, runs
  `CheckCycle` **before** the write, then:
  - color + description + parent all empty → the file is **deleted** (absent = graceful; a tag
    with no metadata renders exactly as pre-v1.6),
  - otherwise `metaFile()` → `writeAndCommit` (the **single-writer** `EnqueueCommit`, the same
    path a page save uses — never a second commit kind) → `upsertCache` (so an immediate read sees
    the new value).
- **`RebuildTagMeta(ctx)`** walks `Tags/*.md` via `repo.Tree()` + the `Tags/` prefix + `repo.Read`
  + `okf.Parse`, parses each frontmatter, and repopulates the cache in one transaction. No `Tags/`
  folder → empty cache (graceful). This mirrors `graph.RebuildGraph`; the parity is proven by
  `TestRebuildParity` / `TestRebuildParityWithParent`.

**`Tags/` is a reserved folder.** It is skipped by the page tree (`isSkippedDir`), the graph
rebuild, and the search index — the metadata store is store content, not navigable wiki pages.

**SQLite is never the source of truth.** Deleting the `tag_meta` table and rebuilding reproduces
identical rows from disk — the same discipline the `page_tags` graph cache holds.

---

## Color: the one trust boundary

Color is the only metadata field that reaches a DOM sink other than `textContent`: the CM6
`HashtagWidget` applies it as `span.style.color = color` (**no `innerHTML`** — the no-innerHTML
XSS invariant of the editor subsystem holds). `ValidateColor` is therefore the load-bearing gate:

- **Accepted:** a hex color (`#RGB` or `#RRGGBB`) or one of a fixed palette of tokens
  (`red`, `blue`, `green`, `amber`, `purple`, `gray`). Empty = clear (use the CSS default).
- **Rejected (400):** `url(...)`, `var(...)`, expressions, or any other arbitrary CSS.

Extending the palette is a deliberate decision; the gate is never opened to free-form CSS.
`ValidateDescription` is defense-in-depth on the description (single-line, ≤200 chars, no HTML
metacharacters `<>&"'`) even though it renders as `textContent`.

---

## Cycle-safe hierarchy (Phase 33, TF-03 — display-only)

`Meta.Parent` (a normalized parent tag name; `""` = root) forms a parent/child hierarchy stored
in `Tags/{tag}.md` frontmatter (`parent:` is always written, empty = root, so rebuild reproduces
it) and the `tag_meta.parent` cache column.

- **`CheckCycle(ctx, tag, parent)`** rejects a cycle-producing parent **before** the write: an
  on-disk cycle would be un-rebuildable. It walks the parent chain upward via the cache —
  self-parenting, revisiting the tag, or exceeding `maxCycleDepth` (100) are all rejected;
  reaching root or a non-existent parent is valid. It is O(depth); the hierarchy is shallow.
- **A non-existent parent is treated as root** (not a referential error) — a tag may reference a
  parent that was later deleted/renamed; the UI treats a dangling parent as root.
- **Display-only.** The hierarchy is a grouped admin list — child rows indented beneath roots with
  a `↳` marker. There is **no** auto-apply of parent tags to pages, **no** descendant-expanding
  search, **no** hierarchy graph grouping in v1.6. Those are deferred to a Future
  "hierarchy-aware tag behavior" item. The server `CheckCycle` (400) is the authoritative gate; the
  UI's self-exclusion in the parent picker is a convenience only.

---

## Usage analytics (Phase 34, TF-04 — read-only, no new scan)

`GET /admin/tags/analytics` returns a per-tag usage summary with **no new heavy scan**:

- per-tag counts from `graph.Store.TagPageCounts` (the existing `page_tags` cache),
- `tagged_pages` from the new `graph.Store.TaggedPageCount` (`COUNT(DISTINCT page_path)`),
- `total_pages` from the live page tree (`pages.Service.Tree()`), `untagged_pages` = the difference,
- `most_used` / `least_used` = top / bottom 5 by count, tie-broken alphabetically,
- **`orphans`** = tags defined in the metadata store (`tagmeta.AllMeta`) but carried by **zero**
  pages. This is the only place analytics reads the tagmeta store: the graph vocabulary always has
  count ≥1 (it comes *from* `page_tags`), so a metadata-only tag (a `Tags/{tag}.md` not yet on any
  page) is the sole source of orphans.

Scope caveat surfaced in the UI: counts reflect **frontmatter** `page_tags`; body-only `#tags` are
counted only after materialization. See the response shape in [tag-admin.md](../api/tag-admin.md).

---

## Tag-operation undo (Phase 35, TF-05 — inverse ApplyTagsBatch)

Undo replays the **inverse** of the *last* tag-admin op (rename / merge / delete / bulk) as one
forward, byte-stable, 409-guarded `ApplyTagsBatch` commit — never a `git revert` or history
rewrite (consistent with `pages.RestoreVersion`). It is backed by a dedicated append-only journal.

### `tag_op_journal` (migration 0013)

```
id INTEGER PK AUTOINCREMENT
op_type    TEXT   -- "rename" | "merge" | "delete" | "bulk" | "undo"
op_detail  TEXT   -- JSON, op-specific (e.g. {old_name, new_name})
pages      TEXT   -- JSON: [{page_path, pre_op_tags, body_affected}]
created_at TEXT
```

This is **operational/derived state** — machine-generated undo records, not user-authored content
— so it lives in SQLite only (mirroring `tag_suggestions`, migration 0010), **not** in the
files-as-truth `Tags/` store.

### How it works

1. **Journaling.** `journalOp` is hooked into `handleTagRename` / `Merge` / `Delete` / `Bulk`
   **after** a successful `ApplyTagsBatch`. It records each page's `pre_op_tags` (the pre-op
   frontmatter tags, straight from the op preview) — the invertible record. It is **non-fatal**:
   a journal write error is logged, never breaks the op (which already committed).
2. **Undo** (`POST /admin/tags/undo`) reads the **last** journal row and builds an inverse
   `TagApplyItem` per page:
   - `Tags = pre_op_tags` (restore the original frontmatter tags),
   - `BaseRevision` read **fresh** via `pages.Get` at undo time (the journal stores tags, not
     revisions) — so `ApplyTagsBatch`'s per-page 409-guard is the authoritative net,
   - the body channel runs in reverse: rename undo swaps `new→old`, merge undo swaps
     `target→source` (delete/bulk carry no body inverse),
   - a **semantic stale check** recomputes what the op would have produced from `pre_op_tags` and
     compares it to the page's current tags; a mismatch = the page was edited since the op → the
     page is reported `stale` and skipped (never clobbered).
3. **Redo for free.** Undo journals *itself*, storing the state before the undo and the *original*
   op's detail, so undo-of-undo replays the original op's direction.

### Limitation — delete's body `#tags` are not invertible

`RewriteHashtags(body, tag, "")` (delete's body rewrite) **splices the `#tag` span out** — the
original bytes are in git history, not the page. Undo of a delete restores the frontmatter `tags:`
list (the tag reappears in frontmatter) but does **not** re-insert removed body `#tags`; they
self-heal on the next edit via the client re-materializer. This is surfaced in the undo response
`note` field. v1.6 ships **last-op** undo (a full undo stack is a Future item).

---

## Bulk multi-tag operations (Phase 31, TF-01 — no new write path)

`POST /admin/tags/bulk` adds and/or removes multiple tags across a selected page set (explicit
`page_paths` ∪ pages matching any `filter_tags`, resolved server-side at both preview and apply)
in one byte-stable batch. It reuses `computeBulkTags(add, remove)` = `(current ∪ add) \ remove`
through the **existing** `buildTagApplyItems` → `ApplyTagsBatch` seam with the body channel
disabled (`BodyOld=""`, frontmatter-only). Added tags are re-validated with `okf.ValidateTagCharset`;
removed/filter tags are matched, not written. It is journaled, so it is undoable like the other ops.
The byte-stable batch itself is documented in [tag-subsystem.md](tag-subsystem.md) and mapped in
`.planning/codebase/MAP-tag-rewrite-subsystem.md`.

---

## Safety guarantees

1. **Admin-only + CSRF.** Every endpoint (`meta` / `analytics` / `bulk` / `undo`) is wired inside
   the admin route subgroup — `RequireRole(admin)` from the session + global nosurf CSRF on the
   mutating POSTs. Analytics is a read-only GET.
2. **Files-as-truth.** Tag metadata is committed content; the SQLite cache is rebuildable. A
   cache-only write would be lost on rebuild — all writes go through `SetMeta`/`ClearMeta`.
3. **One commit spine.** The metadata write path reuses the page-save single-writer commit worker
   — no parallel commit path.
4. **Color trust boundary.** `ValidateColor` restricts color to hex/palette at the server gate; the
   badge sink is `style.color`, never `innerHTML`.
5. **Cycle-safe.** `CheckCycle` rejects a cycle before it can be persisted, at both the store and
   the handler layer.
6. **Undo never clobbers.** A fresh base-revision read + the semantic stale check + the batch
   409-guard mean a concurrently-edited page is reported `stale`, never overwritten.
7. **Journal is operational-only.** `tag_op_journal` is SQLite-only, append-only, last-op undoable.
