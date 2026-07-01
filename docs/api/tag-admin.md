# Tag Admin API

Workspace-wide tag management: rename, merge, and delete a tag across **all** pages, plus a tag-vocabulary read. All endpoints are admin-only (`RequireRole(admin)` + nosurf CSRF). The rename/merge/delete ops follow a **preview → approve** contract and, since v1.5, rewrite BOTH the frontmatter `tags:` key AND the inline body `#tags` in one byte-stable batched commit.

**Base path:** `/api/v1`

**Handler:** [`internal/server/handlers_tags_admin.go`](../../internal/server/handlers_tags_admin.go)
**Routes:** [`internal/server/router.go`](../../internal/server/router.go) (`/admin/tags`, `/admin/tags/rename`, `/admin/tags/merge`, `/admin/tags/delete`)

See [tag-subsystem.md](../architecture/tag-subsystem.md) for the subsystem architecture and the v1.5 server-authoritative body-rewrite design.

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

Returns the workspace tag vocabulary with per-tag page counts from the derived graph cache (TM-04).

**Auth:** Admin only.

**Response `200 OK`:**

```json
[
  {"tag": "release", "count": 12},
  {"tag": "ops", "count": 5}
]
```

**Errors:**
- `500` — Generic unavailable.

**Implementation:** Reads per-tag counts from the `page_tags` cache. (A separate any-authed autocomplete read, `GET /api/v1/tags/vocabulary`, returns the sorted distinct tag list.)
