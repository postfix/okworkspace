# Graph API

The graph API serves the derived link/tag adjacency cache as lean JSON payloads. All endpoints are read-only and available to any authenticated user. The admin rebuild endpoint is admin-only.

**Base path:** `/api/v1`

**Handler:** [`internal/server/handlers_graph.go`](../../internal/server/handlers_graph.go)
**Routes:** [`internal/server/router.go`](../../internal/server/router.go) (lines 181-183, 290)

---

## `GET /api/v1/graph`

Returns the whole derived graph: every page node, every surviving tag node, link edges, and tag membership edges.

**Auth:** Any authenticated user.

**Response `200 OK`:**

```json
{
  "nodes": [
    {"id": "docs/getting-started.md", "label": "Getting Started", "type": "page"},
    {"id": "tag:release", "label": "release", "type": "tag"}
  ],
  "edges": [
    {"source": "index.md", "target": "docs/getting-started.md", "type": "link"},
    {"source": "index.md", "target": "tag:release", "type": "tag"}
  ]
}
```

**Errors:**
- `500` -- "The graph is unavailable." (graph dependency not configured, or query error; details logged server-side)

**Implementation:** Calls `graph.Store.GraphData(ctx)`. No file bodies are read -- only titles from frontmatter (basename fallback). Popular tags (>25% of pages in a workspace with >=8 pages) are excluded from both nodes and edges.

**Source:** [`internal/graph/query.go`](../../internal/graph/query.go) `GraphData()`

---

## `GET /api/v1/graph/local`

Returns the neighborhood (local graph) around a seed page, up to `depth` hops over link edges.

**Auth:** Any authenticated user.

**Query Parameters:**

| Param | Type | Default | Description |
|-------|------|---------|-------------|
| `path` | string | (required) | Workspace-relative `.md` path of the seed page |
| `depth` | integer | `1` | BFS traversal depth, clamped to `[1, 3]` |

**Response `200 OK`:**

```json
{
  "nodes": [
    {"id": "index.md", "label": "Home", "type": "page"},
    {"id": "docs/getting-started.md", "label": "Getting Started", "type": "page"}
  ],
  "edges": [
    {"source": "index.md", "target": "docs/getting-started.md", "type": "link"}
  ]
}
```

**Errors:**
- `400` -- "A page is required." (missing path parameter)
- `500` -- "The graph is unavailable."

**Implementation:** Calls `graph.Store.Neighborhood(ctx, pagePath, depth)`. BFS over undirected link adjacency. Tag nodes/edges for reachable pages are included (popular-tag-capped).

**Source:** [`internal/graph/query.go`](../../internal/graph/query.go) `Neighborhood()`

---

## `GET /api/v1/graph/backlinks`

Returns the list of pages that link TO a given page (reverse link lookup).

**Auth:** Any authenticated user.

**Query Parameters:**

| Param | Type | Default | Description |
|-------|------|---------|-------------|
| `path` | string | (required) | Workspace-relative `.md` path of the target page |

**Response `200 OK`:**

```json
[
  {"path": "index.md", "title": "Home"},
  {"path": "docs/architecture.md", "title": "Architecture"}
]
```

Always returns a JSON array (empty `[]` when there are no backlinks).

**Errors:**
- `400` -- "A page is required." (missing path parameter)
- `500` -- "The graph is unavailable."

**Implementation:** Calls `graph.Store.Backlinks(ctx, pagePath)`. Reads resolved titles from frontmatter (best-effort, basename fallback).

**Source:** [`internal/graph/query.go`](../../internal/graph/query.go) `Backlinks()`

---

## `POST /api/v1/admin/graph/reindex`

Triggers a from-files rebuild of the derived link/tag graph. Returns immediately (fire-and-forget, 202 Accepted). The rebuild runs asynchronously on the single-drain jobs worker.

**Auth:** Admin only.

**Request body:** None.

**Response `202 Accepted`:**

```json
{"ok": true}
```

**Errors:**
- `500` -- Generic unavailable message.

**Implementation:** Enqueues a `KindGraph` job with `{"op": "rebuild"}`. The handler calls `graph.Store.RebuildGraph(ctx)` which reads every live `.md` file, re-extracts all links and tags, and replaces the cache tables in one transaction.

**Source:** [`internal/server/handlers_search.go`](../../internal/server/handlers_search.go) `handleGraphReindex()`, [`internal/graph/rebuild.go`](../../internal/graph/rebuild.go) `RebuildGraph()`
