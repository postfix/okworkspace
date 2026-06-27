# Tag Subsystem

The tag subsystem implements an approval-gated LLM tag-suggestion pipeline: the agent suggests tags for a page, the human reviews and approves them, and tags are written byte-stably through the existing single-writer commit path. The bulk sweep extends this to multiple untagged pages at once, staging results into a review queue for batched approval.

**Packages:**
- `internal/agent/` -- `SuggestTags` mode (LLM call, validation, retry)
- `internal/tagsweep/` -- Bulk sweep staging, pending queue, target enumeration
- `internal/pages/` -- `ApplyTagsBatch` (byte-stable tag write, single commit)
- `internal/okf/` -- `SetTags` (structural frontmatter editor)
- `internal/server/` -- `handlers_tagsweep.go` (HTTP endpoints)

**Source files:**
- [`internal/agent/suggesttags.go`](../../internal/agent/suggesttags.go) -- SuggestTags mode, validateTags, parseTagArray
- [`internal/agent/prompts.go`](../../internal/agent/prompts.go) -- suggest-tags system prompt
- [`internal/tagsweep/store.go`](../../internal/tagsweep/store.go) -- Store, SweepStatus, PendingApprovalCount, StagePending, ListPending, Targets
- [`internal/tagsweep/job.go`](../../internal/tagsweep/job.go) -- KindTagSuggest job handler
- [`internal/pages/batch.go`](../../internal/pages/batch.go) -- ApplyTagsBatch, TagApplyItem, TagApplyResult
- [`internal/okf/repair.go`](../../internal/okf/repair.go) -- SetTags (byte-stable frontmatter editor), ValidateTagCharset (charset gate)
- [`internal/config/config.go`](../../internal/config/config.go) -- SweepConfig (max_concurrent_jobs)
- [`internal/okf/charset_test.go`](../../internal/okf/charset_test.go) -- charset validation test suite

---

## Architecture Overview

```
+---+     +---+     +---+     +---+
| 1 |     | 2 |     | 3 |     | 4 |     +----------+
|   |     |   |     |   |     |   |     |  LLM     |
| S  |---->| S |---->| R |---->| A |---->| (OpenAI) |
| u  |     | t |     | e |     | p |     +----------+
| g  |     | a |     | v |     | p |         |
| g  |     | g |     | i |     | r |<--------+
| e  |     | e |     | e |     | o |     +----------+
| s  |     |   |     | w |     | v |     |  Human   |
| t  |     |   |     |   |     | e |     | (Admin)  |
+---+     +---+     +---+     +---+     +----------+
                                              |
                                              v
                                        +----------+
                                        |  Apply   |
                                        | + Commit |
                                        +----------+
```

1. **Suggest** -- LLM generates tag candidates for a page
2. **Stage** -- Result stored as `pending` row in `tag_suggestions` (no file write)
3. **Review** -- Admin views the queue of pending suggestions
4. **Approve** -- Admin selects tags to apply, server re-validates, writes byte-stably

---

## Suggest Phase (`internal/agent/suggesttags.go`)

### `SuggestTags(ctx, path)` Mode

A single-shot agent **mode** (not a tool) that:

1. Reads the page body via the role-scoped pages reader (never `os.ReadFile`)
2. Captures the page's `baseRevision` at suggest time (optimistic concurrency token)
3. Reads the workspace tag vocabulary from `graph.Store.Vocabulary` (best-effort, errors are logged and skipped -- the suggestion still runs)
4. Calls the LLM with a vocab-biased prompt asking for up to `MaxSuggestedTags` (5) lowercase single-token tags
5. Parses the LLM response, tolerating JSON arrays, code fences, JSON objects with tag keys, and prose-wrapped arrays
6. Validates and normalizes the result: lowercase, trim, dedupe, cap to maximum, reject prose/overlong/control-character tokens
7. Retries up to 3 attempts if the response fails validation (never returns garbage)
8. Returns the normalized tags + existing-vs-new flags + base revision

Key constants:
- `MaxSuggestedTags = 5` -- maximum tags per suggestion
- `maxTagLen = 40` -- a single tag token is capped at 40 characters
- `suggestTagsMaxTokens = 256` -- LLM output cap
- `suggestTagsTemperature = 0.2` -- cool temperature for deterministic output
- `tagsMaxRetries = 2` -- 1 initial + 2 retries = 3 total attempts

### `ValidateTags(raw, vocab)`

Exported re-validation gate reused by the server's apply endpoint. Takes a raw list (potentially from an untrusted client), normalizes (lowercase, trim, dedupe, cap), and returns the validated tags + existing flags. This is the server-side defense-in-depth: the client tag list is NEVER trusted (T-12-06).

### `parseTagArray(reply)`

Lenient parser that extracts a JSON array of strings from the LLM response, tolerant of:
1. A bare JSON array (the contract)
2. A code-fenced JSON array (with or without `json` language tag)
3. A JSON object with `tags`, `suggestions`, or `labels` key
4. Prose-wrapped array (extracts the first `[...]` substring)

### Prompt

The system prompt (`buildSuggestTagsMessages` in `internal/agent/prompts.go`):

```
You are a tagging assistant... Given a page's content and the workspace's
existing tag vocabulary, suggest up to N short topical tags...
Return ONLY a JSON array of lowercase, single-token tags...
Prefer reusing tags from the provided existing vocabulary over inventing
near-synonyms...
```

The page body is supplied as untrusted DATA, delimited between BEGIN/END markers -- never spliced into the system prompt (prompt-injection discipline).

---

## Sweep & Staging Phase (`internal/tagsweep/`)

### `tagsweep.Store`

Staging store over the shared `*sql.DB`. Mirrors `graph.Store`'s construction:
1. `OpenStore(db)` -- bare constructor
2. `SetRepo(r)` -- attach content repo (for live-page enumeration)

### `tag_suggestions` Table (migration 0010)

```
page_path     TEXT NOT NULL    -- the page to tag
suggestions   TEXT NOT NULL    -- JSON array of {tag, existing} pairs
base_revision TEXT NOT NULL    -- Git revision captured at suggest time
status        TEXT NOT NULL    -- 'pending' | 'resolved'
```

Partial-unique index: `(page_path) WHERE status='pending'` enforces one pending row per page.

This table holds operational/derived STAGING data -- NOT page content and NEVER the source of truth. Re-running a sweep reproduces the rows (last-write-wins on re-suggest for the same page).

### `Targets(ctx, allPages)`

Enumerates pages the sweep should target:
- `allPages=false` (default): live pages MINUS pages already in `page_tags` (untagged backfill)
- `allPages=true`: ALL live pages

Uses the same live-page enumeration as `graph.livePages()` (skip dirs, non-.md, trash).

### `StagePending(ctx, pagePath, sugg, baseRevision)`

Stages a pending suggestion row, superseding any prior pending row for the same page (DELETE existing, INSERT new) in one transaction. Called by the KindTagSuggest job handler after the LLM successfully produces tags.

### `ListPending(ctx)` / `GetPending(ctx, pagePath)`

- `ListPending`: returns all pages with `status='pending'`, ordered by `page_path` (the review queue read)
- `GetPending`: returns the single pending row for a specific page, used by the approve handler to read the server-staged `base_revision` (T-12-07: NEVER trust a client-supplied revision)

### `ResolvePending(ctx, pagePaths)`

Flipped `pending` rows to `resolved` for the named pages after a successful apply.

### Sweep Fan-Out Cap (SEC-01)

The admin sweep enforces a configurable concurrency cap to prevent resource exhaustion:

- `sweep.max_concurrent_jobs` in `config.yaml` (default: 10) controls how many tag-suggest jobs are enqueued in a single sweep run.
- The handler stops enqueuing once `queued >= maxConcurrent`; remaining targets are left for a subsequent run (re-staging supersedes).
- The response reports both `queued` and `total` so the UI can show `N/M` progress.
- Safety floor of 10 if the config value is < 1.

**Config type:** `SweepConfig` in `internal/config/config.go`.

### Sweep Progress Polling (UX-01)

The admin TagReviewView polls a dedicated status endpoint to provide live progress feedback during sweeps:

- **`SweepProgress`** struct: `{Queued, Processing, Done, Failed, Total}` -- queried from the `jobs` table grouped by status, filtered to `kind='tag_suggest'`.
- **`PendingApprovalCount()`** -- counts rows in `tag_suggestions` with `status='pending'` for the review queue badge.
- The frontend polls `GET /admin/tags/sweep/status` every 2s while jobs are in-flight.
- Auto-stops polling when no queued/processing jobs remain. Shows a progress bar with "Processing sweep... N/M pages" during active sweeps, and a completion state with "N pages applied (M failed)" when done.
- Non-blocking: user can navigate away and back; polling resumes on re-mount if jobs remain.

### Tag Charset Validation (SEC-02)

Defense-in-depth input validation for tag strings:

- **`okf.ValidateTagCharset(tag) error`** -- rejects tags containing characters outside `^[a-zA-Z0-9_ -]+$` (letters, numbers, underscores, dashes, spaces).
- Called at two layers: in `handleApplyTags` and `handleApproveTagSuggestions` (after `agent.ValidateTags` normalization), and in `okf.SetTags` as a last-resort gate.
- Client-supplied tag lists are NEVER written raw -- they pass through `agent.ValidateTags` (normalize/cap/dedupe), then `okf.ValidateTagCharset` (charset gate), then `okf.SetTags` (redundant charset gate).
- Tests in `internal/okf/charset_test.go` cover valid tags, 17 invalid cases (XSS vectors, Unicode, HTML metacharacters, dots, slashes, quotes), and empty tags.

### `KindTagSuggest` Job

Registered on the shared single-drain jobs worker (the same one carrying search index and graph jobs). One job per target page, enqueued fire-and-forget (never `EnqueueAndWait`). The serial single drain is the natural LLM rate-limiter -- there is NO parallel LLM caller.

The handler (`SuggestHandler`):
1. Calls `agent.Service.SuggestTags` for the named page
2. On success: stages the (tag, existing) pairs + base_revision into `tag_suggestions`
3. On error: returns the error (worker retries with backoff) -- NEVER stages a partial/garbage row
4. Runs under `defer recover()` so a model panic never wedges the drain goroutine

---

## Apply Phase

### Byte-Stable Apply

Tags are written through the same byte-stable pipeline as any page edit:

1. **`okf.SetTags(doc, tags)`** -- structural YAML frontmatter editor. Sets the `tags` key to a block-style sequence (`- tag` per line), creating the key if absent. Mutates ONLY the tags value node in the parsed `yaml.Node` tree; the body and every other frontmatter key remain byte-identical.
2. **`pages.Service.Save`** -- the single-writer commit spine. Re-renders the frontmatter via `okf.Emit`, constructs the full file, and commits through Git.
3. **Base revision check** -- `pages.Save` compares the caller's `BaseRevision` against the current file revision. A mismatch returns 409 Conflict (the page was edited between suggest and apply -- the user must re-suggest).

### `ApplyTagsBatch(ctx, items, actor)`

Batched approval for the review queue (Pitfall 6: one commit for N pages, not one-per-page):

1. For each item, reads the server-staged `base_revision` from `tag_suggestions` (T-12-07: NOT from the client)
2. Reads the current file revision, checks against staged base revision
3. Calls `okf.SetTags` to update the frontmatter in memory
4. Runs all pages through `pages.Save` in one batch commit
5. Returns per-page outcomes:
   - `applied` -- tags written successfully
   - `stale` -- base revision moved (concurrent edit); page skipped but does not sink the batch
   - `notfound` -- page no longer exists on disk
6. The handler resolves only the applied pages' pending rows; stale/notfound rows stay pending for re-run

### `handleApproveTagSuggestions`

The server endpoint for batched approval (TAG-06):

1. Validates page_path (non-empty, no NUL bytes, valid identifier)
2. Re-validates and normalizes ALL client-supplied tags via `agent.ValidateTags` (the client tag list is NEVER written raw -- T-12-06)
3. Reads the STAGED base_revision from the queue for each page (T-12-07: never trusts client-supplied revision)
4. Calls `pages.ApplyTagsBatch` for the atomic batch write
5. Resolves pending rows for applied pages; leaves stale/notfound rows pending
6. Audits the batch with non-secret counts only
7. Returns per-page results so the UI can decrement the backlog

### `handleApplyTags` (per-page approve, TAG-03)

The single-page approve endpoint (used during per-page editing, not the bulk queue):
- Same validation + base_revision re-check + byte-stable write path
- Single page through `pages.Save`

---

## HTTP Endpoints

See [`docs/api/tagsweep.md`](../api/tagsweep.md) for full endpoint contracts.

| Endpoint | Method | Auth | Purpose |
|----------|--------|------|---------|
| `/api/v1/agent/suggest-tags` | POST | Any authenticated | Per-page tag suggestion |
| `/api/v1/agent/apply-tags` | POST | Editor | Approve per-page tags |
| `/api/v1/admin/tags/sweep` | POST | Admin | Start bulk sweep (capped at `max_concurrent_jobs`) |
| `/api/v1/admin/tags/sweep/status` | GET | Admin | Sweep job progress + pending approval count |
| `/api/v1/admin/tags/suggestions` | GET | Admin | List pending suggestions |
| `/api/v1/admin/tags/approve` | POST | Admin | Batched approval |

---

## Safety Guarantees

1. **Sweep writes nothing.** The bulk sweep only stages pending rows -- NO file is written, NO commit is made, until a human explicitly approves.
2. **Client tags NEVER trusted.** All tag input is re-validated and normalized server-side via `ValidateTags`, then checked against the allowed charset via `ValidateTagCharset` (alphabetic, numeric, underscores, dashes, spaces only -- no HTML metacharacters, Unicode, dots, or slashes). The raw client list is never spliced into the YAML/body (T-12-06).
3. **Base revision from the queue.** The approve handler reads the server-staged `base_revision` from `tag_suggestions` -- NEVER from the client (T-12-07).
4. **Stale pages don't sink the batch.** A page that was edited between suggest and apply gets status `stale` but does not fail the whole batch. The admin can re-approve after re-suggesting.
5. **Retry budget.** The LLM suggest mode has a hard retry cap (3 attempts). Garbage/prose output never reaches the queue.
6. **Serial rate-limiter.** The single-drain jobs worker naturally serializes LLM calls (one at a time). No parallel LLM caller.
7. **Audit trail.** Every sweep start and batch approval is recorded in the audit log with non-secret counts (scope, applied, stale, notfound, skipped).
