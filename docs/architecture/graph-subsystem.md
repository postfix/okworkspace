# Graph Subsystem

The graph subsystem maintains a **derived link/tag adjacency cache** for the workspace -- a rebuildable SQLite store (`page_links`, `page_tags`, `graph_meta`) that powers the global graph view and backlinks panel (the per-page local/neighborhood graph was removed in v1.5 -- see below). It is NEVER the source of truth: deleting both tables and running `RebuildGraph` reproduces byte-identical rows from the on-disk `.md` files.

**Package:** `internal/graph/`

**Source files:**
- [`internal/graph/store.go`](../../internal/graph/store.go) -- Store, DriftCheck, SQLite table access
- [`internal/graph/extract.go`](../../internal/graph/extract.go) -- link extraction, tag reading from parsed Markdown
- [`internal/graph/rebuild.go`](../../internal/graph/rebuild.go) -- from-files rebuild, skip rules
- [`internal/graph/query.go`](../../internal/graph/query.go) -- GraphData, Backlinks, Vocabulary queries
- [`internal/graph/graphjob.go`](../../internal/graph/graphjob.go) -- async job handler (KindGraph), upsert/delete per page

---

## Data Model

### `page_links` table
```
src_path TEXT NOT NULL  -- page that contains the link
dst_path TEXT NOT NULL  -- page being linked to
```
A forward edge from `src_path` to `dst_path`. Both paths are workspace-relative `.md` paths. Links are directed (page A links to page B), but the graph UI treats them as undirected for neighborhood traversal. Backlinks are a simple reverse query (`WHERE dst_path=?`).

### `page_tags` table
```
page_path TEXT NOT NULL  -- page carrying the tag
tag TEXT NOT NULL        -- tag name (lowercase, normalized)
```
A membership edge: the page has this tag in its frontmatter.

### `graph_meta` table
```
key TEXT PRIMARY KEY     -- metadata key
value TEXT               -- metadata value
```
Stores `last_graphed_head` -- the Git HEAD SHA the graph was last built against. Used by `DriftCheck` to detect when the graph is stale.

Both `page_links` and `page_tags` use `INSERT OR IGNORE` so upserts are idempotent. A partial-unique index on `tag_suggestions(page_path) WHERE status='pending'` (in the tagsweep subsystem) enforces the one-pending-row-per-page invariant.

---

## Store (`internal/graph/store.go`)

`graph.Store` wraps the shared `*sql.DB` and the content repo:

```go
type Store struct {
    db   *sql.DB    // shared SQLite connection
    repo *repo.Repo // safe-path resolver for file reads
    gs   headProvider // Git HEAD accessor
}
```

Construction follows the setter pattern (mirroring `search.Index`):
1. `OpenStore(db)` -- bare constructor
2. `SetRepo(r)` -- attach content repo (for rebuild/upsert file reads)
3. `SetGit(gs)` -- attach Git HEAD provider (for drift detection)

### Drift Detection

`DriftCheck` compares the stored `last_graphed_head` against the current Git HEAD. If they differ (an out-of-band pull/restore, or a crash between commit and graph update), the server enqueues a from-files rebuild at startup.

---

## Link Extraction (`internal/graph/extract.go`)

### `outboundLinks(srcPath, body, exists)`

Reuses `okf.FindLinks` (the byte scanner that skips fenced/inline code spans) to find all Markdown links in a page body. Each link destination is:
1. Resolved relative to the source page's directory (`path.Join(path.Dir(srcPath), dest)`)
2. Kept only if it:
   - Is a repo-relative `.md` link (not external/absolute, not a fragment-only link)
   - Is not a self-edge (page linking to itself)
   - Has not been seen before (deduped)
   - Points to a page that actually exists on disk (dangling links are silently dropped)
3. The `#fragment` is stripped before comparison

### `pageTags(doc)`

Reads the `tags` key from the YAML frontmatter's `yaml.Node` tree, the same way the search index reads it (parity requirement). Supports both:
- `tags: [a, b, c]` (flow sequence)
- `tags: [a]` (single-element flow)
- Block-style `tags:\n  - a\n  - b`

Returns the list of trimmed, non-empty tag strings, or nil if there are no tags.

---

## Rebuild (`internal/graph/rebuild.go`)

### `RebuildGraph(ctx)`

A from-files correctness backstop that reproduces the cache tables from on-disk `.md` files. Walk:

1. **First pass:** enumerate every live page via `repo.Tree()`, applying skip rules:
   - Skip directories
   - Skip non-`.md` files
   - Skip anything under `.okf-workspace/trash` (deleted pages)
   - Build the live-page set for link existence checks
2. **Second pass:** for each live page, read through the safe-path resolver (`repo.Read`, never `os.*`), parse via `okf.Parse`, extract outbound links and tags
3. **Transaction:** DELETE both tables, bulk-insert all accumulated rows, commit
4. **Record HEAD:** store the current Git SHA as `last_graphed_head` (done LAST so a partial rebuild never claims a clean state)

Per-page read/parse errors are logged and skipped (best-effort over files -- a corrupt file never aborts the whole rebuild).

### `upsertPage(ctx, pagePath)`

Incremental update for one page after a save. Re-scans the page and atomically replaces ONLY that page's rows in a transaction:
- DELETE `page_links WHERE src_path=?`
- DELETE `page_tags WHERE page_path=?`
- INSERT new rows from the re-scanned page

If the page no longer exists on disk, it degrades to a delete.

### `deletePage(ctx, pagePath)`

Removes a page from the graph completely:
- DELETE `page_links WHERE src_path=? OR dst_path=?` (outbound and inbound)
- DELETE `page_tags WHERE page_path=?`

---

## Query Layer (`internal/graph/query.go`)

### `GraphData(ctx)` -- Global Graph

Returns the whole derived graph as a lean JSON payload:

- **Page nodes:** UNION of every page referenced by the cache tables + every live page on disk (so orphans appear as nodes)
- **Tag nodes:** every tag from `page_tags`, minus popular-tag-capped tags (a tag on >25% of pages in a workspace with >=8 pages is excluded)
- **Link edges:** all page-to-page links from `page_links`
- **Tag edges:** page-to-tag membership from `page_tags` (popular-tag-capped)

No page bodies are read -- the only file touch is title resolution (frontmatter title, basename fallback).

### `Neighborhood(ctx, pagePath, depth)` -- Local Graph *(REMOVED in v1.5)*

The per-page local graph was removed end-to-end in the v1.5 reading-shell fold-in:
the `Neighborhood(ctx, pagePath, depth)` query method, the `GET /graph/local`
route + handler, and the client `LocalGraphPanel` / `DepthControl` components are
all gone. The global graph (`GraphData` / `GET /graph`, surfaced at the SPA route
`/app/graph`) is RETAINED. Re-introduction is tracked as a Future requirement
("Re-introduce a per-page local graph") in `.planning/REQUIREMENTS.md`.

### `Backlinks(ctx, pagePath)` -- Backlinks Panel

Reverse page_links query: returns every page linking TO `pagePath`, each with its resolved title (best-effort, basename fallback).

### `Vocabulary(ctx)` -- Tag Vocabulary

Returns the DISTINCT sorted set of all tags currently in the cache. Used by the agent's tag suggestion to bias toward existing tags (TAG-04).

### `PagesByTag(ctx, tag)` -- Frontmatter-Tag Membership

Returns the paths of every page whose FRONTMATTER `tags:` includes the (normalized) tag, from the `page_tags` cache. Used by the workspace-wide tag admin ops (rename/merge/delete) to enumerate frontmatter-affected pages.

**Body-only tags are invisible here.** `page_tags` is a frontmatter-only cache, so a page carrying `#tag` ONLY in its body is absent from `PagesByTag`. The v1.5 tag-rewrite ops therefore UNION this with `pages.PagesWithBodyTag(tag)` -- which DOES read bodies. That body scan deliberately lives in the **pages service** (`internal/pages/tagscan.go`), NOT here, to preserve this layer's "reads NO bodies -- only path enumeration" invariant (`query.go:256`). See [tag-subsystem.md](tag-subsystem.md#body-inclusive-affected-set-enumeration-internalpagestagscango).

---

## Async Job (`internal/graph/graphjob.go`)

### `KindGraph`

Registered on the shared single-drain jobs worker. Payload shape:

```json
{"op": "upsert"|"delete"|"rebuild", "path": "page.md"}
```

The handler dispatches by `Op`:
- `rebuild` -- calls `RebuildGraph`
- `upsert` -- calls `upsertPage`
- `delete` -- calls `deletePage`

The handler runs under `defer recover()` so a parser/SQLite panic on one corrupt page never wedges the single drain goroutine. Jobs are enqueued fire-and-forget (`worker.Enqueue`, never `EnqueueAndWait`) to prevent deadlock with the single drain.

---

## HTTP Endpoints

See [`docs/api/graph.md`](../api/graph.md) for full endpoint contracts.

| Endpoint | Method | Auth | Purpose |
|----------|--------|------|---------|
| `/api/v1/graph` | GET | Any authenticated | Global graph data |
| `/api/v1/graph/local` | — | — | *REMOVED in v1.5 (per-page local graph); see the Neighborhood note above* |
| `/api/v1/graph/backlinks?path=X` | GET | Any authenticated | Pages linking TO a page |
| `/api/v1/admin/graph/reindex` | POST | Admin | Rebuild graph from files |

---

## Popular Tag Cap

A tag that appears on more than `popularTagShare` (25%) of all pages in a workspace with at least `popularTagMinPages` (8) pages is excluded from BOTH tag nodes and tag membership edges. This prevents ubiquitous tags (like "documentation" or "note") from becoming hub nodes that dominate the graph layout.

The cap is evaluated against the WHOLE workspace's page/tag counts, not a restricted subset. This ensures that a local neighborhood view agrees with the global graph on which tags are capped.
