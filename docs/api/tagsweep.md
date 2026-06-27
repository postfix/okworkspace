# Tagsweep API

The tagsweep API covers the per-page tag suggestion, bulk sweep, review queue, and approval endpoints. Read-only suggestion is available to any authenticated user; apply endpoints require editor or admin role.

**Base path:** `/api/v1`

**Handler:** [`internal/server/handlers_tagsweep.go`](../../internal/server/handlers_tagsweep.go)
**Routes:** [`internal/server/router.go`](../../internal/server/router.go) (lines 209, 233, 297-299, 307)

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
  "page_path": "docs/getting-started.md",
  "suggestions": [
    {"tag": "release", "existing": true},
    {"tag": "docs", "existing": true},
    {"tag": "onboarding", "existing": false}
  ],
  "base_revision": "abc123def"
}
```

- `page_path` -- the page the tags were suggested for.
- `suggestions` -- array of `{tag, existing}` objects; `existing` is `true` if the tag already exists in workspace vocabulary.
- `base_revision` -- Git revision captured at suggest time (the optimistic-concurrency token for the later apply).

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

**Response `204 No Content`:** Empty body on success. The tags were written and committed.

**Errors:**
- `400` -- Invalid request (or tag charset validation failure).
- `409` -- Conflict (page was edited since suggest -- must re-suggest).
- `500` -- Generic unavailable.

**Implementation:** Re-validates tags via `agent.ValidateTags`, checks charset via `okf.ValidateTagCharset`, writes via `okf.SetTags` through `pages.Service.Save` (single-writer commit). The client's `base_revision` is tested against the current file revision.

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
{"ok": true, "queued": 10, "total": 42}
```

- `queued` -- number of pages enqueued in this batch (capped at `sweep.max_concurrent_jobs`, default 10).
- `total` -- total number of pages targeted. When `queued < total`, the admin can re-run the sweep for the remaining pages (re-staging supersedes previous pending rows).

**Errors:**
- `500` -- Generic unavailable.

**Implementation:** Enumerates targets server-side via `tagsweep.Store.Targets()` (the client cannot inject arbitrary paths). Enqueues up to `sweep.max_concurrent_jobs` (default 10, configurable in `config.yaml`) target jobs fire-and-forget through the shared single-drain jobs worker; remaining pages are left for a subsequent sweep run. Audited with scope + count + total.

**Source:** [`internal/server/handlers_tagsweep.go`](../../internal/server/handlers_tagsweep.go) `handleStartTagSweep()`, [`internal/tagsweep/store.go`](../../internal/tagsweep/store.go) `Targets()`

---

## `GET /api/v1/admin/tags/sweep/status`

Returns the current sweep job progress (how many jobs are queued, running, done, failed) and the number of pages awaiting approval in the review queue.

**Auth:** Admin only.

**Response `200 OK`:**

```json
{
  "progress": {
    "queued": 0,
    "processing": 3,
    "done": 15,
    "failed": 0,
    "total": 18
  },
  "pending_approval": 3
}
```

- `progress.queued` -- jobs waiting to start.
- `progress.processing` -- jobs currently running (always 0 or 1 under the single-drain worker).
- `progress.done` -- completed jobs.
- `progress.failed` -- jobs that failed (LLM error, page not found).
- `progress.total` -- total jobs enqueued for this sweep run.
- `pending_approval` -- number of pages in `tag_suggestions` with `status='pending'` (the review queue count used for badge display).

**Errors:**
- `500` -- "Tag sweep is unavailable."

**Implementation:** Queries the `jobs` table grouped by status, filtered to `kind='tag_suggest'`. Counts pending `tag_suggestions` rows separately. Used by the admin TagReviewView to display a progress bar during active sweeps (polled at 2s interval; auto-stops when no jobs remain).

**Source:** [`internal/server/handlers_tagsweep.go`](../../internal/server/handlers_tagsweep.go) `handleTagSweepStatus()`, [`internal/tagsweep/store.go`](../../internal/tagsweep/store.go) `SweepStatus()`, `PendingApprovalCount()`

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

**Implementation:** For each approval: validates path, re-validates tags via `agent.ValidateTags`, checks charset via `okf.ValidateTagCharset` (rejects HTML metacharacters, Unicode, dots, slashes), reads staged `base_revision` from `tagsweep.GetPending` (server-side, not client-supplied), calls `pages.ApplyTagsBatch` for the atomic batch write, resolves applied pages' pending rows. Audited with non-secret counts.

**Source:** [`internal/server/handlers_tagsweep.go`](../../internal/server/handlers_tagsweep.go) `handleApproveTagSuggestions()`, [`internal/pages/batch.go`](../../internal/pages/batch.go) `ApplyTagsBatch()`
