# Tagsweep API

The tagsweep API covers the per-page tag suggestion, bulk sweep, review queue, and approval endpoints. Read-only suggestion is available to any authenticated user; apply endpoints require editor or admin role.

**Base path:** `/api/v1`

**Handler:** [`internal/server/handlers_tagsweep.go`](../../internal/server/handlers_tagsweep.go)
**Routes:** [`internal/server/router.go`](../../internal/server/router.go) (lines 209, 233, 297-298, 306)

---

## `POST /api/v1/agent/suggest-tags`

Suggests up to 5 normalized tags for a single page. Read-only -- no files are written. Returns the tags with existing-vs-new flags and the page's base revision for later optimistic-concurrency checks.

**Auth:** Any authenticated user.

**Request body:**

```json
{"path": "docs/getting-started.md"}
```

**Response `200 OK`:**

```json
{
  "tags": ["release", "docs", "onboarding"],
  "existing": [true, true, false],
  "base_revision": "abc123def"
}
```

- `tags` -- normalized, deduped, capped list (max 5)
- `existing` -- parallel array: `true` if tag already exists in workspace vocabulary
- `base_revision` -- Git revision captured at suggest time (the optimistic-concurrency token for the later apply)

**Errors:**
- `400` -- Invalid request (missing path, malformed body).
- `422` -- "Could not produce a clean tag set." (LLM output failed validation after 3 retries).
- `500` -- Generic unavailable. Agent not configured.

**Implementation:** Calls `agent.Service.SuggestTags(ctx, path)`. The page body is read through the role-scoped pages reader, the LLM is called with vocab-biased prompt, output is parsed and validated (3-attempt retry loop). Never writes to disk.

**Source:** [`internal/agent/suggesttags.go`](../../internal/agent/suggesttags.go) `SuggestTags()`

---

## `POST /api/v1/agent/apply-tags`

Applies approved tags to a single page byte-stably. The client-supplied tag list is re-validated and normalized server-side. The base revision is checked for optimistic concurrency (409 on stale).

**Auth:** Editor or admin.

**Request body:**

```json
{
  "path": "docs/getting-started.md",
  "tags": ["release", "docs"],
  "base_revision": "abc123def"
}
```

**Response `200 OK`:**

```json
{"ok": true}
```

**Errors:**
- `400` -- Invalid request.
- `409` -- Conflict (page was edited since suggest -- must re-suggest).
- `500` -- Generic unavailable.

**Implementation:** Re-validates tags via `agent.ValidateTags`, writes via `okf.SetTags` through `pages.Service.Save` (single-writer commit). The client's `base_revision` is tested against the current file revision.

**Source:** [`internal/server/handlers_agent.go`](../../internal/server/handlers_agent.go), [`internal/okf/repair.go`](../../internal/okf/repair.go) `SetTags()`

---

## `POST /api/v1/admin/tags/sweep`

Starts a bulk sweep that enqueues one tag-suggestion job per target page (fire-and-forget). Writes nothing to disk -- each job stages its result as a pending row.

**Auth:** Admin only.

**Request body** (optional):

```json
{"all": false}
```

- `all=false` (default): only untagged pages (those not in `page_tags`)
- `all=true`: ALL live pages

An empty body defaults to `all=false`.

**Response `202 Accepted`:**

```json
{"ok": true, "queued": 42}
```

- `queued` -- number of pages enqueued (0 when every page already has tags).

**Errors:**
- `500` -- Generic unavailable.

**Implementation:** Enumerates targets server-side via `tagsweep.Store.Targets()` (the client cannot inject arbitrary paths). Enqueues one `KindTagSuggest` job per target fire-and-forget through the shared single-drain jobs worker. Audited with scope + count.

**Source:** [`internal/server/handlers_tagsweep.go`](../../internal/server/handlers_tagsweep.go) `handleStartTagSweep()`, [`internal/tagsweep/store.go`](../../internal/tagsweep/store.go) `Targets()`

---

## `GET /api/v1/admin/tags/suggestions`

Lists the pages with a pending staged suggestion (the review queue read).

**Auth:** Admin only.

**Response `200 OK`:**

```json
[
  {
    "page_path": "docs/getting-started.md",
    "suggestions": [
      {"tag": "release", "existing": true},
      {"tag": "docs", "existing": true},
      {"tag": "onboarding", "existing": false}
    ],
    "base_revision": "abc123def"
  }
]
```

Always returns a JSON array (empty `[]` when no pending suggestions exist).

**Errors:**
- `500` -- Generic unavailable.

**Implementation:** Calls `tagsweep.Store.ListPending(ctx)`. Returns all rows with `status='pending'`, ordered by `page_path`.

**Source:** [`internal/tagsweep/store.go`](../../internal/tagsweep/store.go) `ListPending()`

---

## `POST /api/v1/admin/tags/approve`

Approves one or many pages' staged suggestions and writes the approved tags byte-stably in a single Git commit. The server re-validates each page's tags and reads the staged `base_revision` from the queue -- the client tag list and any claimed revision are NEVER trusted.

**Auth:** Admin only.

**Request body:**

```json
{
  "approvals": [
    {"page_path": "docs/getting-started.md", "tags": ["release", "docs"]},
    {"page_path": "onboarding/index.md", "tags": ["onboarding"]}
  ]
}
```

Maximum 1000 pages per batch request.

**Response `200 OK`:**

```json
[
  {"page_path": "docs/getting-started.md", "status": "applied"},
  {"page_path": "onboarding/index.md", "status": "applied"}
]
```

Per-page statuses:
- `applied` -- tags written successfully and committed.
- `stale` -- page was edited since suggest (concurrent edit). Does not fail the batch; the page stays pending for re-suggest.
- `notfound` -- page no longer exists on disk.
- `skipped` -- page was dropped during server pre-checks (empty tag list after normalization, or no pending row).

**Errors:**
- `400` -- "There are no approvals to apply." (empty body), "Too many pages in one approval." (over 1000), or "Invalid request." (bad identifiers or NUL bytes).
- `500` -- Generic unavailable.

**Implementation:** For each approval: validates path, re-validates tags via `agent.ValidateTags`, reads staged `base_revision` from `tagsweep.GetPending` (server-side, not client-supplied), calls `pages.ApplyTagsBatch` for the atomic batch write, resolves applied pages' pending rows. Audited with non-secret counts.

**Source:** [`internal/server/handlers_tagsweep.go`](../../internal/server/handlers_tagsweep.go) `handleApproveTagSuggestions()`, [`internal/pages/batch.go`](../../internal/pages/batch.go) `ApplyTagsBatch()`
