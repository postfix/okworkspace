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
|  |  | app.db (SQLite)          |                                  | |
|  |  | users, sessions,         |                                  | |
|  |  | audit_log,               |                                  | |
|  |  | page_links, page_tags,   |                                  | |
|  |  | tag_suggestions, jobs    |                                  | |
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
1. Browser requests `/api/v1/graph` (global graph) or `/api/v1/graph/local?path=X&depth=2`
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

## Component Map

| Component | Package | Purpose |
|-----------|---------|---------|
| Repo | `internal/repo` | Safe-path resolver, filesystem chokepoint |
| Config | `internal/config` | YAML-based config with server/auth/agent/sweep sections |
| Pages | `internal/pages` | Page CRUD, byte-stable Markdown round-trip, tag apply |
| Graph | `internal/graph` | Derived link/tag adjacency cache, rebuild, query layer |
| Tagsweep | `internal/tagsweep` | Bulk tag sweep staging, pending queue, target enumeration, sweep progress tracking |
| Agent | `internal/agent` | Eino orchestration: ask, summarize, rewrite, draft, suggest-tags, generate-description |
| Editor | `web/src/lib/cm` | CM6 live-preview surface (edit + read); decoration pipeline, syntax highlight, mermaid, hashtags (see [editor-subsystem.md](editor-subsystem.md)) |
| Search | `internal/search` | Bleve full-text index |
| Auth | `internal/auth` | Session management, RBAC, Argon2id |
| Server | `internal/server` | HTTP handlers, chi router, DI wiring |
| OKF | `internal/okf` | Markdown/frontmatter parsing, repair, emit, link extraction |
| Jobs | `internal/jobs` | Single-drain async job worker |
| Store | `internal/store` | SQLite connection management, migrations |

## Key Architectural Decisions

- **SQLite is never the source of truth.** All wiki content is plain files. The graph cache (`page_links`, `page_tags`) and tag staging (`tag_suggestions`) are derived data: deleting and rebuilding from files reproduces identical rows.
- **Single-writer commit spine.** All mutations serialize through one writer to prevent Git merge conflicts. Optimistic concurrency (base revision check) catches stale edits.
- **Fire-and-forget for derived caches.** Page mutations enqueue graph/index rebuild jobs asynchronously. The jobs worker serializes them through a single drain goroutine.
- **Hidden infrastructure.** Error responses to clients use generic messages ("The graph is unavailable."). Internal details (Git, SQLite, index, jobs) are logged server-side only.
- **Generic error copies on all server-facing failures.** No Git/SQLite/LLM vocabulary reaches the client response body.
