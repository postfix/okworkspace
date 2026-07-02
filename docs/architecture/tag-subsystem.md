# Tag Subsystem

The tag subsystem implements an approval-gated LLM tag-suggestion pipeline: the agent suggests tags for a page, the human reviews and approves them, and tags are written byte-stably through the existing single-writer commit path. The bulk sweep extends this to multiple untagged pages at once, staging results into a review queue for batched approval.

**Packages:**
- `internal/agent/` -- `SuggestTags` mode (LLM call, validation, retry)
- `internal/tagsweep/` -- Bulk sweep staging, pending queue, target enumeration
- `internal/pages/` -- `ApplyTagsBatch` (byte-stable frontmatter+body tag write, single commit), `PagesWithBodyTag` / `DetectStaleBodyTags` (body-inclusive enumeration)
- `internal/okf/` -- `SetTags` (structural frontmatter editor), `FindHashtags` / `RewriteHashtags` (region-aware body `#tag` rewrite), `ValidateInlineTag` (body-splice gate)
- `internal/server/` -- `handlers_tagsweep.go` (suggest/sweep/approve), `handlers_tags_admin.go` (rename/merge/delete admin ops)

**Source files:**
- [`internal/agent/suggesttags.go`](../../internal/agent/suggesttags.go) -- SuggestTags mode, validateTags, parseTagArray
- [`internal/agent/prompts.go`](../../internal/agent/prompts.go) -- suggest-tags system prompt
- [`internal/tagsweep/store.go`](../../internal/tagsweep/store.go) -- Store, SweepStatus, PendingApprovalCount, StagePending, ListPending, Targets
- [`internal/tagsweep/job.go`](../../internal/tagsweep/job.go) -- KindTagSuggest job handler
- [`internal/pages/batch.go`](../../internal/pages/batch.go) -- ApplyTagsBatch, TagApplyItem (BodyOld/BodyNew/SkipFrontmatter), TagApplyResult
- [`internal/pages/tagscan.go`](../../internal/pages/tagscan.go) -- PagesWithBodyTag, DetectStaleBodyTags (FindHashtags-based body scan through the SEC-01 resolver)
- [`internal/okf/hashtag.go`](../../internal/okf/hashtag.go) -- FindHashtags (the single body-`#tag` recognizer), RewriteHashtags (byte-stable region-aware body rewrite)
- [`internal/okf/validate.go`](../../internal/okf/validate.go) -- ValidateInlineTag (body-splice grammar gate)
- [`internal/okf/repair.go`](../../internal/okf/repair.go) -- SetTags (byte-stable frontmatter editor), ValidateTagCharset (frontmatter charset gate)
- [`internal/server/handlers_tags_admin.go`](../../internal/server/handlers_tags_admin.go) -- rename/merge/delete/list handlers, preview `body_affected`, the SEC-v1.5-F01 intersection gate
- [`internal/config/config.go`](../../internal/config/config.go) -- SweepConfig (max_concurrent_jobs)
- [`internal/okf/charset_test.go`](../../internal/okf/charset_test.go) -- charset validation test suite
- [`internal/okf/hashtag_test.go`](../../internal/okf/hashtag_test.go) + [`internal/okf/testdata/hashtag_corpus.json`](../../internal/okf/testdata/hashtag_corpus.json) -- Lezer↔Go offset-parity corpus (TR-02, CI-enforced)
- [`internal/okf/validate_test.go`](../../internal/okf/validate_test.go) -- ValidateInlineTag grammar-subset corpus + charset-non-loosening guard

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

Batched apply for BOTH the review queue AND the admin rename/merge/delete ops (Pitfall 6: one commit for N pages, not one-per-page). It does NOT call `pages.Save` per page -- it stages every applied page into one `commitPayload` and enqueues a SINGLE commit through the same single-writer worker `Save` uses (`EnqueueCommit`), under `lockMutation` (serializes against namespace mutations so a concurrent rename cannot move a page mid-batch):

1. For each `TagApplyItem`: `Exists` check (missing → `notfound`), then compares the item's `BaseRevision` against the current committed `Revision` (moved → `stale`, excluded, batch continues). NOTE: `BaseRevision` is supplied BY THE CALLER, already server-sourced -- the sweep-approve handler stages it from `tag_suggestions` (T-12-07), the admin handlers read it fresh via `pages.Get`. The batch trusts the field, it does not re-read the queue.
2. Reads + parses the current page bytes (TOCTOU-safe: the body it rewrites is its OWN fresh read, not the handler's).
3. **Body channel (v1.5, TR-03):** when `BodyOld != ""`, rewrites the body `#tags` via `okf.RewriteHashtags(doc.Body, BodyOld, BodyNew)` -- region-aware, byte-stable. `BodyOld == ""` disables it (the pre-v1.5 behavior: body emitted verbatim).
4. **Frontmatter region:** unless `SkipFrontmatter` is set, rewrites the tags region byte-stably via `SetTagsFrontmatter` → `okf.SetTags`. For a body-only affected page (`SkipFrontmatter`) the tags region stays byte-identical -- only the body changes, so a page whose tag lives only in prose is not given a spurious `tags:` key.
5. **No-op guard:** if the emitted bytes equal the read bytes (neither frontmatter nor body changed), the write is skipped entirely but the page is still reported `applied`.
6. Stages the write; after the ONE commit lands, marks every staged page `applied` and enqueues fire-and-forget index + graph upserts.
7. Per-page outcomes (`applied` / `stale` / `notfound`) are returned in input order; a `stale`/`notfound` page never sinks the batch. A non-nil error is returned ONLY for an infrastructure failure (read/emit/commit-enqueue) that prevents committing at all.

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

## Workspace-Wide Tag Management (v1.3 admin ops + v1.5 server-authoritative body rewrite)

The admin tag-management endpoints rename, merge, and delete a tag across the WHOLE
workspace. They ride the same `SetTagsFrontmatter` builder + `ApplyTagsBatch` single-commit
path as the review queue. Each op follows a **preview → approve** contract: `preview=true`
returns the affected-page set without writing; `preview=false` applies through the batch.

**The v1.5 regression fix (Phase 29, TR-01..06) made these ops server-authoritative for the
page BODY, not just the frontmatter `tags:` key.** Before v1.5 a rename/merge/delete rewrote
only the frontmatter, leaving the inline `#tag` in the page body stale; because the editor
re-materializes body `#tags` into frontmatter on the next save (ED-02b), the "renamed" tag
silently came back. The fix rewrites the body `#tags` AND the frontmatter in the SAME
byte-stable batched commit, and widens the affected-set enumeration to include pages that
carry the tag ONLY in their body.

### `okf.FindHashtags(body)` -- the single body-`#tag` recognizer (`internal/okf/hashtag.go`)

A conservative, code-region-aware BYTE scanner (NOT a Markdown parser) that returns every
inline `#tag` occurrence with byte offsets, the server-side counterpart to `links.go`'s
`FindLinks`. It is the ONE recognizer reused by BOTH `RewriteHashtags` and the pages-service
enumeration (`tagscan.go`), so there is never a "second `#tag` parser". It recognizes a
`#tag` exactly where the canonical TS Lezer grammar (`web/src/lib/cm/hashtag.ts`) emits a
`Hashtag` inline node, and SKIPS every region the Lezer parse excludes:

- fenced code blocks (```` ``` ````/`~~~`) and inline code spans (backtick runs)
- indented (4-space/tab) code blocks and HTML blocks (CommonMark type-1/type-6)
- `\#` escapes (the backslash consumes the next byte)
- the ATX-heading leading `#` run (a HeaderMark, not inline content)

A `#` abutting a blockquote marker (`>#tag`) is a valid start boundary exactly as the Lezer
`cx.offset` rule makes it. **Discipline: skip-when-uncertain.** When a region cannot be
confidently classified the `#` is left UNCHANGED -- a missed rewrite leaves a stale tag that
self-heals on the next correct op; an over-eager rewrite corrupts a code region. The bias is
hard toward under-rewriting.

**Offset-parity contract (TR-02, CI-enforced).** `FindHashtags` is pinned offset-for-offset
to the Lezer grammar by a single-sourced corpus: `web/src/lib/cm/hashtag.parity.test.ts` runs
the REAL Lezer parser (`inlineTagSpansFromBody`, sharing `tagParserExtensions` with the render
+ materializer) over the shared fixtures and regenerates `internal/okf/testdata/hashtag_corpus.json`;
`internal/okf/hashtag_test.go` loads that JSON and asserts BYTE `from`/`to` spans (not merely
tag-name sets). A grammar change not committed to the JSON fails the TS side; a Go scanner that
finds a different span fails the Go side. Drift fails CI on either side.

### `okf.RewriteHashtags(body, old, new)` -- byte-stable body rewrite

Rewrites body `#tag` occurrences whose (case-insensitive) name equals `old`: a rename/merge
splices `#`+`new`; a delete (`new == ""`) removes the `#`+token. Returns the edited body and a
changed flag; on no match the ORIGINAL slice is returned byte-identical. Only matched `#tag`
spans are touched -- prose, code regions, and other tags are preserved exactly. Two locked rules:

- **D-03a (delete whitespace):** removing a token collapses exactly ONE adjacent space, and
  ONLY when the token was space-bounded on BOTH sides (` #ops ` → ` `); otherwise whitespace is
  untouched and a newline is NEVER merged across.
- **D-03b (merge adjacent-duplicate):** a merge yielding `#b #b` leaves both tokens as-is (no
  dedupe -- the frontmatter reconcile dedupes the set).

### `okf.ValidateInlineTag(tag)` -- the body-splice gate (`internal/okf/validate.go`)

Validates a BARE tag name (no leading `#`) against the inline-`#tag` grammar SUBSET the Lezer
body grammar can represent, so a rename/merge TARGET can be spliced into a body as `#`+name and
round-trip as the SAME tag the editor would badge. Rules mirror `hashtag.ts`: non-empty; body
alphabet only `[A-Za-z0-9_/-]`; ≥1 non-digit `[A-Za-z_-]` (so `1984` and `12/34` are rejected --
`/` is a separator, not a qualifying non-digit); no leading/trailing/doubled slash.

**It is STRICTER than and DISTINCT from `ValidateTagCharset`.** `ValidateTagCharset`
(`^[a-zA-Z0-9_ -]+$`) permits a space and an all-digit token but FORBIDS `/` -- the exact inverse
of what body injection needs. `ValidateTagCharset` keeps its frontmatter-charset / SEC-02
HTML-metachar role unchanged; `ValidateInlineTag` is an ADDITIONAL gate for a token entering PROSE.

**SEC-v1.5-F01 intersection gate.** A rename/merge target is written to BOTH the body (via
`RewriteHashtags`, `/`-safe) AND the frontmatter `tags:` key (via `SetTags`, `/`-forbidden). A
nested target like `a/b` passes `ValidateInlineTag` but would 500 at the frontmatter write, so
`handleTagRename` / `handleTagMerge` require the INTERSECTION -- both `ValidateInlineTag` AND
`ValidateTagCharset` -- making such a target a clean **400 at the gate, not a 500 at apply**.

### Body-inclusive affected-set enumeration (`internal/pages/tagscan.go`)

- **`PagesWithBodyTag(ctx, tag)`** returns every live page whose BODY carries `#tag` (region-aware,
  via `FindHashtags`), reading each body through the SEC-01 resolver (`repo.Tree()` + `repo.Read`).
  It is the TR-04 blind-spot closer: a page carrying `#tag` ONLY in its body is absent from the
  frontmatter-only `page_tags` cache (invisible to `graph.PagesByTag`), so the rename/merge/delete
  affected set unions `graph.PagesByTag(tag)` with `PagesWithBodyTag(tag)`. It deliberately lives in
  the PAGES service, NOT `internal/graph/query.go`, whose "reads NO bodies -- only path enumeration"
  invariant must hold.
- **`DetectStaleBodyTags(ctx)`** (TR-06) is a bounded one-shot DETECT+REPORT scan flagging every live
  page whose body `#tag` disagrees with its frontmatter (a body `#tag` whose frontmatter counterpart
  is absent -- the signature of a pre-fix rename/delete that rewrote frontmatter but left the body
  stale). It NEVER writes and has **no HTTP/CLI caller** -- it is a read-only report method (the
  ROADMAP "for all pages including never-reopened" over-claim was honestly softened to "for all FUTURE
  operations" + this surfaced historical set). Covered by `TestDetectStaleBodyTags`.

### Preview `body_affected` scope (TR-05)

`buildTagApplyItems` includes a page when its frontmatter tag set changes OR its body `#tags` will
change (the widened predicate). For a body-only page it sets `SkipFrontmatter` (leave the tags region
byte-identical) and marks `BodyAffected` in the preview. The preview response carries a per-page
`body_affected` boolean; the admin UI (`TagManagement.tsx`) surfaces "N page(s) also have body #tags
that will be rewritten" + a `prose` badge, so the admin sees prose will change, not just frontmatter.
See [`docs/api/tag-admin.md`](../api/tag-admin.md) for the full contract.

---

## HTTP Endpoints

See [`docs/api/tagsweep.md`](../api/tagsweep.md) (suggest/sweep/approve) and [`docs/api/tag-admin.md`](../api/tag-admin.md) (rename/merge/delete/bulk, meta, analytics, undo) for full endpoint contracts. The v1.6 tag-metadata store, hierarchy, analytics, and undo design lives in [`docs/architecture/tag-metadata-subsystem.md`](tag-metadata-subsystem.md).

| Endpoint | Method | Auth | Purpose |
|----------|--------|------|---------|
| `/api/v1/agent/suggest-tags` | POST | Any authenticated | Per-page tag suggestion |
| `/api/v1/agent/apply-tags` | POST | Editor | Approve per-page tags |
| `/api/v1/admin/tags/sweep` | POST | Admin | Start bulk sweep (capped at `max_concurrent_jobs`) |
| `/api/v1/admin/tags/sweep/status` | GET | Admin | Sweep job progress + pending approval count |
| `/api/v1/admin/tags/suggestions` | GET | Admin | List pending suggestions |
| `/api/v1/admin/tags/approve` | POST | Admin | Batched approval |
| `/api/v1/admin/tags` | GET | Admin | Workspace tag vocabulary + per-tag page counts |
| `/api/v1/admin/tags/rename` | POST | Admin | Rename a tag workspace-wide (frontmatter + body), preview/apply |
| `/api/v1/admin/tags/merge` | POST | Admin | Merge a source tag into a target, preview/apply |
| `/api/v1/admin/tags/delete` | POST | Admin | Delete a tag workspace-wide (frontmatter + body), preview/apply |
| `/api/v1/admin/tags/bulk` | POST | Admin | Bulk add/remove multiple tags across a page set (v1.6, frontmatter-only), preview/apply |
| `/api/v1/admin/tags/meta` | POST | Admin | Set/clear a tag's color/description/parent (v1.6, files-as-truth store) |
| `/api/v1/admin/tags/analytics` | GET | Admin | Read-only per-tag usage summary (v1.6, over `page_tags` counts) |
| `/api/v1/admin/tags/undo` | POST | Admin | Undo the last tag-admin op via inverse `ApplyTagsBatch` (v1.6) |

---

## Safety Guarantees

1. **Sweep writes nothing.** The bulk sweep only stages pending rows -- NO file is written, NO commit is made, until a human explicitly approves.
2. **Client tags NEVER trusted.** All tag input is re-validated and normalized server-side via `ValidateTags`, then checked against the allowed charset via `ValidateTagCharset` (alphabetic, numeric, underscores, dashes, spaces only -- no HTML metacharacters, Unicode, dots, or slashes). The raw client list is never spliced into the YAML/body (T-12-06).
3. **Base revision from the queue.** The approve handler reads the server-staged `base_revision` from `tag_suggestions` -- NEVER from the client (T-12-07).
4. **Stale pages don't sink the batch.** A page that was edited between suggest and apply gets status `stale` but does not fail the whole batch. The admin can re-approve after re-suggesting.
5. **Retry budget.** The LLM suggest mode has a hard retry cap (3 attempts). Garbage/prose output never reaches the queue.
6. **Serial rate-limiter.** The single-drain jobs worker naturally serializes LLM calls (one at a time). No parallel LLM caller.
7. **Audit trail.** Every sweep start and batch approval is recorded in the audit log with non-secret counts (scope, applied, stale, notfound, skipped).
8. **Body rewrite is region-aware + byte-stable (v1.5).** Rename/merge/delete rewrite body `#tags` only where `okf.FindHashtags` recognizes them (fenced/inline/indented/HTML/escaped `#` excluded), offset-pinned to the Lezer grammar by a CI-enforced parity corpus. Only matched `#tag` byte spans change; a no-match body is returned byte-identical. The rename/merge target passes the INTERSECTION of `ValidateInlineTag` (body-splice-safe) and `ValidateTagCharset` (frontmatter-storable) -- a frontmatter-incompatible target (`a/b`) is a clean 400, not a 500 (SEC-v1.5-F01).
