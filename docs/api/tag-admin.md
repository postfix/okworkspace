# Tag Admin API

Workspace-wide tag management: rename, merge, delete, and **bulk add/remove** a tag across pages; a tag-vocabulary read; **tag metadata** (color/description/parent); **usage analytics**; and **last-op undo**. All endpoints are admin-only (`RequireRole(admin)` + nosurf CSRF; the analytics read is a GET). The rename/merge/delete/bulk ops follow a **preview → approve** contract and, since v1.5, rename/merge/delete rewrite BOTH the frontmatter `tags:` key AND the inline body `#tags` in one byte-stable batched commit; bulk is frontmatter-only.

**Base path:** `/api/v1`

**Handler:** [`internal/server/handlers_tags_admin.go`](../../internal/server/handlers_tags_admin.go)
**Routes:** [`internal/server/router.go`](../../internal/server/router.go) (`/admin/tags`, `/admin/tags/rename`, `/admin/tags/merge`, `/admin/tags/delete`, `/admin/tags/bulk`, `/admin/tags/meta`, `/admin/tags/analytics`, `/admin/tags/undo`)

See [tag-subsystem.md](../architecture/tag-subsystem.md) for the suggest/sweep/apply + v1.5 body-rewrite design, and [tag-metadata-subsystem.md](../architecture/tag-metadata-subsystem.md) for the v1.6 metadata store, hierarchy, analytics, and undo design.

---

## Preview → apply contract

Each of rename / merge / delete accepts a `preview` boolean:

- `preview=true` → returns a `tagPreviewResponse` listing the affected pages and their tag changes; **nothing is written**.
- `preview=false` (or omitted) → applies the mutation through `pages.ApplyTagsBatch` (N pages → ONE commit) and returns a `tagApplyResponse` with per-page results.

The affected-page set is the UNION of:
- pages carrying the tag in **frontmatter** (`graph.PagesByTag`), and
- pages carrying the tag **only in the body** (`pages.PagesWithBodyTag`, v1.5 / TR-04).

A page is included when its frontmatter tag set changes OR its body `#tags` will change. For a body-only page the frontmatter tags region is left byte-identical (`SkipFrontmatter`); only the body is rewritten.

### `tagPreviewResponse`

```json
{
  "tag": "ops",
  "affected_pages": [
    {
      "page_path": "runbooks/deploy.md",
      "current_tags": ["ops", "release"],
      "new_tags": ["devops", "release"],
      "body_affected": true
    }
  ],
  "total_affected": 1
}
```

- `tag` — the operated tag (rename/merge use `"old→new"` / `"source→target"`).
- `affected_pages[].current_tags` / `new_tags` — the page's frontmatter tag list before/after.
- `affected_pages[].body_affected` — **v1.5 (TR-05):** `true` when the page's BODY `#tags` will also be rewritten (not just the frontmatter `tags:`), so the admin sees prose will change. Surfaced in `TagManagement.tsx` as a `prose` badge + "N page(s) also have body #tags that will be rewritten".
- `total_affected` — count of affected pages.

### `tagApplyResponse`

```json
{
  "action": "rename",
  "results": [
    {"page_path": "runbooks/deploy.md", "status": "applied"}
  ]
}
```

Per-page `status` is one of `applied`, `stale` (base revision moved between read and commit — excluded, does not sink the batch), or `notfound` (page gone). `results` is `[]` when the op is a no-op (nothing affected).

---

## `POST /api/v1/admin/tags/rename`

Renames a tag across all pages (TM-01).

**Auth:** Admin only.

**Request body:**

```json
{"old_name": "ops", "new_name": "devops", "preview": false}
```

**Response `200 OK`:** `tagPreviewResponse` (when `preview=true`) or `tagApplyResponse`.

**Errors:**
- `400` — Invalid request; empty `old_name`/`new_name`; **`new_name` fails `okf.ValidateInlineTag`** (the body-splice grammar gate — rejects spaces, HTML metacharacters, all-digit tokens, leading/trailing/doubled `/`); **or `new_name` fails `okf.ValidateTagCharset`** (the frontmatter charset gate — rejects `/`). The rename target must satisfy the INTERSECTION of both (SEC-v1.5-F01): a nested target like `a/b` is body-splice-safe but not frontmatter-storable, so it is a clean 400 here, not a 500 at the frontmatter write.
- `500` — Generic unavailable (enumeration or apply failure; details logged).

A case-only rename (`ops` → `Ops`, equal after normalization) is a no-op returning an empty `results`.

**Implementation:** Normalizes both names, validates the target (both gates), unions `graph.PagesByTag(old)` with `pages.PagesWithBodyTag(old)`, builds items (frontmatter via `computeRenameTags`, body via `BodyOld=old`/`BodyNew=new`), and applies via `pages.ApplyTagsBatch`. Audited (`ActionTagRename`, `old→new`, page count).

---

## `POST /api/v1/admin/tags/merge`

Merges a source tag into a target tag (TM-02): every page with `#source` gets `#source` rewritten to `#target` (body + frontmatter), deduped in frontmatter.

**Auth:** Admin only.

**Request body:**

```json
{"source": "k8s", "target": "kubernetes", "preview": false}
```

**Response `200 OK`:** `tagPreviewResponse` or `tagApplyResponse`.

**Errors:**
- `400` — Invalid request; empty `source`/`target`; `source == target`; or `target` fails `okf.ValidateInlineTag` **or** `okf.ValidateTagCharset` (the same intersection gate as rename, SEC-v1.5-F01).
- `500` — Generic unavailable.

**Implementation:** Unions frontmatter source pages + frontmatter target pages (which may need a frontmatter dedupe) + body-`#source` pages (`pages.PagesWithBodyTag(source)`). **Body pages for `#target` are NOT enumerated** — the merge only rewrites `#source`; adding `#target` to a page that merely mentions it would be wrong. Body rewrite intent is `source → target`. Audited (`ActionTagMerge`).

---

## `POST /api/v1/admin/tags/delete`

Removes a tag from all pages (TM-03): `#tag` is removed from the body (D-03a whitespace rule) and dropped from frontmatter.

**Auth:** Admin only.

**Request body:**

```json
{"tag": "deprecated", "preview": false}
```

**Response `200 OK`:** `tagPreviewResponse` or `tagApplyResponse`.

**Errors:**
- `400` — Invalid request; empty `tag`.
- `500` — Generic unavailable.

Delete needs no new-token gate (the body op is a removal, `BodyNew=""`), so `ValidateInlineTag`/`ValidateTagCharset` are not applied to the deleted tag.

**Implementation:** Unions `graph.PagesByTag(tag)` with `pages.PagesWithBodyTag(tag)`, body rewrite intent `tag → ""`. Audited (`ActionTagDelete`).

---

## `GET /api/v1/admin/tags`

Returns the workspace tag vocabulary with per-tag page counts from the derived graph cache (TM-04), merged with each tag's metadata (color / description / parent) from the tag-metadata store (v1.6 / TF-02, TF-03).

**Auth:** Admin only.

**Response `200 OK`:**

```json
[
  {"tag": "release", "count": 12, "color": "#4caf50", "description": "Shipped work", "parent": "ops"},
  {"tag": "ops", "count": 5, "color": "blue", "description": "", "parent": ""}
]
```

- `count` — pages carrying the tag in **frontmatter** (`page_tags` cache).
- `color` / `description` / `parent` — **v1.6:** merged from `tagmeta.AllMeta`. A tag with no metadata row gets empty strings (`""`). `parent` is the display-hierarchy parent (`""` = root).

**Errors:**
- `500` — Generic unavailable.

**Implementation:** Reads per-tag counts from the `page_tags` cache and merges per-tag metadata from `tagmeta.AllMeta` (one call). (A separate any-authed autocomplete read, `GET /api/v1/tags/vocabulary`, returns the sorted distinct tag list.)

---

## `POST /api/v1/admin/tags/bulk`

Adds and/or removes multiple tags across a selected set of pages in ONE byte-stable preview→apply batch (Phase 31 / TF-01). Frontmatter-only — the body channel is disabled (`BodyOld=""`).

**Auth:** Admin only.

**Request body:**

```json
{
  "page_paths": ["runbooks/deploy.md"],
  "filter_tags": ["ops"],
  "add_tags": ["devops", "release"],
  "remove_tags": ["legacy"],
  "preview": false
}
```

- The page set is the UNION of explicit `page_paths` and pages carrying any of `filter_tags` (via `graph.PagesByTag`), re-resolved **server-side** at both preview and apply (TOCTOU-safe).
- The new tag set per page is `(current ∪ add_tags) \ remove_tags`, normalized + deduped, original order preserved then new tags appended.

**Response `200 OK`:** `tagPreviewResponse` (when `preview=true`) or `tagApplyResponse` (`action: "bulk"`).

**Errors:**
- `400` — Invalid request; at least one of `page_paths`/`filter_tags` AND at least one of `add_tags`/`remove_tags` are required; an added tag fails `okf.ValidateTagCharset` (removed/filter tags are matched, not written, so they are not charset-gated).
- `500` — Generic unavailable.

**Implementation:** Resolves the page set, builds items via `computeBulkTags(add, remove)` through the existing `buildTagApplyItems` → `pages.ApplyTagsBatch` seam (frontmatter-only), and journals the op (`ActionTagBulk`) so it is undoable.

---

## `POST /api/v1/admin/tags/meta`

Sets or clears a tag's metadata — color, description, and parent (Phase 32/33 / TF-02, TF-03). Writes `Tags/{tag}.md` (files-as-truth) through the single-writer commit path and upserts the rebuildable cache. See [tag-metadata-subsystem.md](../architecture/tag-metadata-subsystem.md).

**Auth:** Admin only.

**Request body:**

```json
{"tag": "deploy", "color": "#4caf50", "description": "Deploy ops", "parent": "ops"}
```

- `color` — a hex color (`#RGB` / `#RRGGBB`) OR a fixed palette token (`red`, `blue`, `green`, `amber`, `purple`, `gray`). `""` clears the color.
- `description` — single-line, ≤200 chars, no HTML metacharacters. `""` clears it.
- `parent` — a valid tag name (display-hierarchy parent) or `""` (root). A non-existent parent is treated as root.
- When `color`, `description`, and `parent` are **all** empty, the tag's metadata file is **deleted** (absent = graceful default).

**Response `200 OK`:** the stored `{tag, color, description, parent}`.

**Errors:**
- `400` — Invalid/empty `tag`; `color` fails `ValidateColor` (rejects `url()`, `var()`, expressions, arbitrary CSS); `description` fails `ValidateDescription` (multi-line, >200 chars, or HTML metacharacters); `parent` fails `ValidateParent`; **or `parent` would create a cycle** (`CheckCycle` — self-parenting, or the tag is already an ancestor of the parent).
- `500` — Tag metadata store not available; write/commit failure.

**Implementation:** Validates color/description/parent, runs `CheckCycle` before the write, calls `tagmeta.SetMeta`, records `ActionTagMeta`.

---

## `GET /api/v1/admin/tags/analytics`

Returns a read-only per-tag usage summary (Phase 34 / TF-04). Sourced from the existing `page_tags` count cache — **no new heavy scan**.

**Auth:** Admin only. Read-only GET (no CSRF token needed).

**Response `200 OK`:**

```json
{
  "total_tags": 2,
  "total_pages": 3,
  "tagged_pages": 2,
  "untagged_pages": 1,
  "most_used": [{"tag": "ops", "count": 2}],
  "least_used": [{"tag": "docs", "count": 1}],
  "orphans": [{"tag": "unused-meta", "count": 0}],
  "tags": [{"tag": "docs", "count": 1}, {"tag": "ops", "count": 2}]
}
```

- `total_pages` — live `.md` pages (from `pages.Service.Tree()`); `tagged_pages` — distinct pages with ≥1 frontmatter tag (`graph.TaggedPageCount`); `untagged_pages` — the difference.
- `most_used` / `least_used` — top / bottom 5 by count, tie-broken alphabetically.
- `orphans` — tags defined in the metadata store (`Tags/{tag}.md`) but carried by **zero** pages.
- `tags` — the full per-tag count list, alphabetical. All arrays are `[]` (never `null`).
- **Scope:** counts reflect **frontmatter** `page_tags`; body-only `#tags` are counted only after materialization.

**Errors:**
- `500` — Graph store not available.

---

## `POST /api/v1/admin/tags/undo`

Undoes the LAST tag-admin operation (rename / merge / delete / bulk) by replaying its inverse through `pages.ApplyTagsBatch` — one forward, byte-stable, 409-guarded commit (Phase 35 / TF-05). See [tag-metadata-subsystem.md](../architecture/tag-metadata-subsystem.md).

**Auth:** Admin only.

**Request body:** none.

**Response `200 OK`:** `tagApplyResponse` (`action: "undo"`) with per-page `status` (`applied` / `stale` / `notfound`), plus an optional `note` (the delete body-`#tag` limitation).

**Errors:**
- `400` — No tag operation to undo (empty journal).
- `500` — Tag undo unavailable (journal store not wired); read/apply failure.

**Behavior notes:**
- `BaseRevision` is read **fresh** per page at undo time; a page edited since the op is reported `stale` and skipped (never clobbered) via a semantic stale check + the batch 409-guard.
- Body `#tags` are restored for rename/merge undo (the inverse body rewrite) but **not** for delete undo — a deleted body span cannot be reconstructed; undo restores the frontmatter `tags:` list only, and body `#tags` self-heal on the next edit. Surfaced in the `note` field.
- Undo journals itself, so undo-of-undo (redo) restores the undone op.
- **Last-op only** in v1.6 (a full undo stack is a Future item).
