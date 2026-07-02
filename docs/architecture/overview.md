# Architecture Overview

## System Architecture

OKF Workspace follows a **files-are-truth** architecture. All wiki content lives as plain Markdown files with YAML frontmatter on disk, versioned by Git. SQLite holds only operational metadata (users, sessions, jobs, audit log) and derived caches that are rebuildable from the source files at any time. The React SPA is embedded into the Go binary via `//go:embed`.

```
+---------------------------------------------------------------------+
|  Browser (React SPA)                                                |
|  +------------+  +-----------+  +----------+  +---------+          |
|  | Graph UI   |  | Page View |  | Settings |  | File    |          |
|  | (D3 force) |  | + Editor  |  | / Admin  |  | Tree    |          |
|  +------+-----+  +-----+-----+  +----+-----+  +----+----+          |
|         |              |              |             |               |
+---------+--------------+--------------+-------------+---------------+
          |              |              |             |
          v              v              v             v
+---------------------------------------------------------------------+
|  Go Backend (chi HTTP router)                                       |
|  +----------+  +-----------+  +----------+  +---------+            |
|  | Graph    |  | Pages     |  | Admin    |  | Auth/   |            |
|  | Handlers |  | Handlers  |  | Handlers |  | Session |            |
|  +----+-----+  +-----+-----+  +----+-----+  +----+----+            |
|       |              |              |             |                 |
|       v              v              v             v                 |
|  +---------+   +-----------+   +-------+   +----------------+      |
|  | Graph   |   | Pages     |   | Search|   | Auth Service   |      |
|  | Store   |   | Service   |   | Index |   | + Session Mgmt |      |
|  | (SQLite)|   +-----+-----+   |(Bleve)|   +--------+-------+      |
|  +----+----+         |         +-------+            |              |
|       |              |                              |               |
|       v              v                              v               |
|  +----------------------------------------------------------------+ |
|  |  Repo Layer (internal/repo) — SAFE PATH RESOLVER               | |
|  |  Single chokepoint for all filesystem access. Paths are        | |
|  |  resolved, validated, and constrained before touching disk.    | |
|  +----------------------------------------------------------------+ |
|       |              |                              |               |
|       v              v                              v               |
|  +----------------------------------------------------------------+ |
|  |  File System (data_dir/)                                       | |
|  |  +---------------------------+  +---------------------------+   | |
|  |  | content/ (.md + uploads) |  | .okf.git/ (hidden Git)   |   | |
|  |  | README.md                |  | (auto-committed history)  |   | |
|  |  | onboarding/              |  +---------------------------+   | |
|  |  |   index.md               |                                  | |
|  |  +---------------------------+                                  | |
|  |  +---------------------------+                                  | |
|  | app.db (SQLite)          |                                  | |
|  | users, sessions,         |                                  | |
|  | audit_log,               |                                  | |
|  | page_links, page_tags,   |                                  | |
|  | tag_suggestions,         |                                  | |
|  | tag_meta, tag_op_journal,|                                  | |
|  | jobs                     |                                  | |
|  |  +---------------------------+                                  | |
|  +----------------------------------------------------------------+ |
+---------------------------------------------------------------------+
```

## Data Flow

### Read Path (page view)
1. Browser requests `/app/page/docs/getting-started`
2. Go handler (`handleGetPageOrHistory`) reads page from `repo.Repo` (safe-path resolver)
3. The Markdown body is returned along with YAML frontmatter metadata
4. SPA renders the body via the CM6 live-preview surface -- the same component in read-only mode (PageView) or editable mode (PageEditor); see [editor-subsystem.md](editor-subsystem.md)

### Write Path (page save)
1. Browser sends PUT `/api/v1/pages/*` with the full page content
2. Handler validates access (editor role from session), parses the Markdown body + YAML frontmatter, reconstructs the file byte-stably
3. `pages.Service.Save` serializes through a single-writer Git commit spine
4. Post-save hooks enqueue fire-and-forget jobs:
   - `KindIndex` (search): re-index the page text
   - `KindGraph` (graph): re-extract the page's outbound links and tags

### Graph Read Path
1. Browser requests `/api/v1/graph` (global graph, surfaced at the SPA route `/app/graph`). The per-page local graph (`/graph/local`) was **removed in v1.5**; see [graph-subsystem.md](graph-subsystem.md).
2. Handler reads from the derived SQLite cache tables (`page_links`, `page_tags`) -- no Markdown files are touched in the request path except for title resolution
3. Returns a lean JSON payload: nodes (page + tag ids/labels) and typed edges (link + tag)

### Tag Suggestion Flow (suggest -> apply)
1. Agent's `SuggestTags` mode reads the page body via the role-scoped pages service, builds a vocab-biased prompt, and calls the LLM
2. The LLM response is parsed and validated (normalized, deduped, capped)
3. Tags are returned to the UI along with the page's base revision (optimistic concurrency token)
4. On user approval, `/agent/apply-tags` or `/admin/tags/approve` writes the tags byte-stably via `okf.SetTags` through `pages.Save`

### Bulk Sweep Flow
1. Admin sends POST `/api/v1/admin/tags/sweep` with optional `{"all": true}`
2. Handler enumerates target pages server-side (untagged or all live pages)
3. Enqueues one `KindTagSuggest` job per target page fire-and-forget
4. Each job calls `SuggestTags` for one page and stages the result as a pending row in `tag_suggestions`
5. Admin reviews the pending queue at `GET /admin/tags/suggestions`
6. Admin approves selected pages via `POST /admin/tags/approve`
7. The batch apply writes all approved tags in a single Git commit, then resolves the pending rows

### Workspace-Wide Tag Management (v1.3 admin ops + v1.5 body rewrite)
1. Admin previews/applies a rename, merge, or delete via `POST /api/v1/admin/tags/{rename,merge,delete}`
2. The affected-page set unions the frontmatter-tag pages (`graph.PagesByTag`) with pages carrying the tag in their BODY (`pages.PagesWithBodyTag`) -- so a body-only page is not missed (v1.5, TR-04)
3. Each affected page's frontmatter `tags:` AND body `#tags` are rewritten byte-stably (`okf.SetTags` + `okf.RewriteHashtags`) in ONE batched commit (`pages.ApplyTagsBatch`) -- the fix that made these ops server-authoritative for the body, not just the frontmatter (v1.5); see [tag-subsystem.md](tag-subsystem.md)

### Tag Metadata, Bulk, Analytics & Undo (v1.6)
1. Admin sets a tag's color/description/parent via `POST /api/v1/admin/tags/meta` → written as `Tags/{tag}.md` (files-as-truth) through the single-writer commit path + a rebuildable `tag_meta` cache. Color/description surface on the CM6 `#tag` badge and the admin list; the parent forms a display-only hierarchy (cycle-safe). See [tag-metadata-subsystem.md](tag-metadata-subsystem.md)
2. Admin bulk-adds/removes multiple tags across a page set via `POST /api/v1/admin/tags/bulk` — the same byte-stable `ApplyTagsBatch` seam (frontmatter-only), no new write path
3. Admin views usage analytics via `GET /api/v1/admin/tags/analytics` — a read-only summary over the existing `page_tags` counts (no new scan), with orphans sourced from the metadata store
4. Admin undoes the last tag-admin op via `POST /api/v1/admin/tags/undo` — the inverse replayed through `ApplyTagsBatch`, backed by the append-only `tag_op_journal`

## Component Map

| Component | Package | Purpose |
|-----------|---------|---------|
| Repo | `internal/repo` | Safe-path resolver, filesystem chokepoint |
| Config | `internal/config` | YAML-based config with server/auth/agent/sweep sections |
| Pages | `internal/pages` | Page CRUD, byte-stable Markdown round-trip, tag apply |
| Graph | `internal/graph` | Derived link/tag adjacency cache, rebuild, query layer |
| Tagsweep | `internal/tagsweep` | Bulk tag sweep staging, pending queue, target enumeration, sweep progress tracking |
| Tagmeta | `internal/tagmeta` | Tag-metadata store (color/description/parent) — files-as-truth `Tags/*.md` + rebuildable `tag_meta` cache (v1.6, see [tag-metadata-subsystem.md](tag-metadata-subsystem.md)) |
| Agent | `internal/agent` | Eino orchestration: ask, summarize, rewrite, draft, suggest-tags, generate-description |
| Editor | `web/src/lib/cm` | CM6 live-preview surface (edit + read); decoration pipeline, syntax highlight, mermaid, hashtags (see [editor-subsystem.md](editor-subsystem.md)) |
| Search | `internal/search` | Bleve full-text index (v1.6: page frontmatter `description` stored + surfaced in result cards) |
| Auth | `internal/auth` | Session management, RBAC, Argon2id |
| Server | `internal/server` | HTTP handlers, chi router, DI wiring |
| OKF | `internal/okf` | Markdown/frontmatter parsing, repair, emit, link extraction |
| Jobs | `internal/jobs` | Single-drain async job worker |
| Store | `internal/store` | SQLite connection management, migrations |

**Navrail search (v1.5 reading-shell fold-in).** The in-place file-tree filter (narrow-the-tree-by-title) has been removed. The navrail's search field now REPLACES the tree with full-text results (`web/src/components/search/NavSearchResults.tsx`, grouped Pages / Headings / Attachments) whenever the query is non-empty, and returns to the tree when the query is cleared. ⌘K focuses the field. See [Key Architectural Decisions](#key-architectural-decisions).

## Key Architectural Decisions

- **SQLite is never the source of truth.** All wiki content is plain files. The graph cache (`page_links`, `page_tags`), tag staging (`tag_suggestions`), and the tag-metadata cache (`tag_meta`, v1.6) are derived data: deleting and rebuilding from files reproduces identical rows. (The `tag_op_journal` undo record, v1.6, is operational-only state — machine-generated, not content.)
- **Single-writer commit spine.** All mutations serialize through one writer to prevent Git merge conflicts. Optimistic concurrency (base revision check) catches stale edits.
- **Fire-and-forget for derived caches.** Page mutations enqueue graph/index rebuild jobs asynchronously. The jobs worker serializes them through a single drain goroutine.
- **Hidden infrastructure.** Error responses to clients use generic messages ("The graph is unavailable."). Internal details (Git, SQLite, index, jobs) are logged server-side only.
- **Generic error copies on all server-facing failures.** No Git/SQLite/LLM vocabulary reaches the client response body.
- **Navrail full-text search replaces the in-place tree filter (v1.5).** One find surface: full-text search beats a title-prefix tree filter for a growing wiki. The narrow-the-tree-by-title control was removed in the reading-shell fold-in; re-introduction is tracked as a Future requirement ("In-place file-tree filter") in `.planning/REQUIREMENTS.md`.
- **Tag metadata is content, not operational data (v1.6).** Tag color/description/parent are durable data a team must be able to copy off the server, so they live as `Tags/*.md` files (files-as-truth) with only a rebuildable `tag_meta` cache — the same discipline as page content. The write path reuses the page-save single-writer commit spine (no second commit kind). The hierarchy is **display-only** in v1.6 (no page rewrites / search expansion); tag-op undo is **last-op only** (a full stack is a Future item). See [tag-metadata-subsystem.md](tag-metadata-subsystem.md).
