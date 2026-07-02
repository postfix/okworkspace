---
author: architect
responsible: architect
phase: 31
phase_type: implementation
status: Ready for planning
hard_bar: false
security_relevant: true
design_fork: false
parent_artifacts:
  - .planning/ROADMAP.md
  - .planning/REQUIREMENTS.md
---

# Phase 31: Bulk Multi-Tag Operations - Context

**Gathered:** 2026-07-02
**Status:** Ready for planning

<domain>
## Phase Boundary

An admin adds and/or removes multiple tags across a selected set of pages in ONE
byte-stable preview→approve batch commit, reusing `pages.ApplyTagsBatch`. This is
the v1.3-original deferred bulk-multi-tag op (TF-01), the cheapest tag-track phase:
the batch seam already exists, the write path is proven, and the op is
frontmatter-only (no body `#tag` rewrite → no `RewriteHashtags`, no
`ValidateInlineTag`).

**Requirements:** TF-01

**Non-goals:** body `#tag` rewriting (that is the v1.5 rename/merge/delete channel —
bulk add/remove touches only frontmatter tag sets); tag metadata/color/description
(Phase 32); tag hierarchy (Phase 33); tag-op undo (Phase 35, which depends on this
phase's op being journaled).

**This phase builds on committed HEAD** — v1.5 (Phase 30) closed; the v1.5
security review SHIP; debt=0. The batch seam (`ApplyTagsBatch`, `buildTagApplyItems`)
is the proven, reviewed, byte-stable write path this op rides.
</domain>

<decisions>
## Implementation Decisions

### Page-set selection (D-01) — both mechanisms, per TF-01
- **D-01:** The bulk op accepts a page set built from **both** mechanisms (per the
  TF-01 acceptance criterion "by explicit selection and/or by an existing-tag
  filter"), combined as a union:
  - **Explicit selection:** an explicit list of page paths (checkboxes in the page
    list, or a multi-select). The admin picks pages directly.
  - **Existing-tag filter:** "all pages carrying tag X" — resolved server-side via
    `graph.Store.PagesByTag` (`query.go:428`), the existing derived-cache query. The
    admin may filter by one or more tags (union of the resulting page sets).
  - The two are UNIONed: explicit paths ∪ pages-matching-any-filter-tag. The
    server re-derives the set at approve time (the client never sends a trusted page
    list without re-validation — the server reads the live page set at apply).
  - **No new query needed** — `PagesByTag` already exists. A flat page list for
    explicit selection is available via `pages.Service.Tree()` (the existing
    SEC-01-resolver enumeration, `tree.go:36`).

### The op shape (D-02) — add/remove-from-set, frontmatter-only
- **D-02:** The bulk op takes two tag sets: `add_tags` and `remove_tags`. For each
  page in the resolved set, the new frontmatter tag list is
  `(current ∪ add_tags) \ remove_tags`, normalized + deduped. This is a NEW compute
  function (`computeBulkTags`) fed to the existing `buildTagApplyItems` helper
  (`handlers_tags_admin.go:205`) — the same seam rename/merge/delete use. The
  `bodyOld`/`bodyNew` fields are `""` (no body rewrite → `BodyOld==""` disables the
  body channel per `batch.go:56-60`). `SkipFrontmatter` is `false` for every page.
  A page whose computed set equals its current set (no-op) is excluded from the
  batch (the existing `changed` predicate in `buildTagApplyItems`).
- **D-02a:** Tags are re-validated server-side via `okf.ValidateTagCharset`
  (`repair.go:159`) — the frontmatter charset gate. `ValidateInlineTag` is NOT
  needed (no body splicing). This matches the TF-01 acceptance criterion exactly.

### Preview→approve contract (D-03) — same shape as rename/merge/delete
- **D-03:** `POST /admin/tags/bulk` with `preview=true` returns the per-page
  before/after tag sets + affected count (the `tagPreviewResponse` shape,
  `handlers_tags_admin.go:58-67`). Without preview, it applies via `ApplyTagsBatch`
  and returns `tagApplyResponse` (`:72`). The request carries the page-set spec
  (explicit paths + filter tags) AND the add/remove tag sets; the server re-resolves
  the page set at apply time from the live store (TOCTOU-safe — a page deleted
  between preview and apply is `notfound`, a page edited is `stale`, both excluded
  per-page without sinking the batch, exactly like the existing approve path).
- **D-03a:** The `base_revision` for each page is read server-side at preview time
  and re-checked at apply time — the same optimistic-concurrency floor the existing
  tag-admin ops use (`batch.go:42-43`). The client does NOT send `base_revision`
  (it is never a client-claimed value).

### Audit (D-04) — new action constant
- **D-04:** A new `audit.ActionTagBulk = "tag_bulk"` constant (`audit.go:65-67`
  pattern) records the bulk op: actor + detail `"N pages, +[a,b] -[c]"`. The
  `Detail` carries only non-secret provenance (counts + tag names, never a page
  body). Recorded non-fatally per the existing audit discipline.

### Frontend (D-05) — a BulkTagDialog beside the existing dialogs
- **D-05:** A new `BulkTagDialog` component in `TagManagement.tsx` (alongside
  `RenameDialog`/`MergeDialog`/`DeleteDialog`, `:101-301`) with: (1) a page-set
  selector (explicit checkboxes over the page list + a "filter by tag" multi-select
  using the existing `listTags` vocabulary), (2) add-tags + remove-tags inputs (tag
  multi-selects using the `listTags` vocabulary for autocomplete), (3) a
  preview-results table (per-page before→after, reusing the `TagPreviewPage` shape),
  (4) an approve button. The dialog calls a new `bulkTag` client function in
  `client.ts` mirroring `renameTag`/`mergeTag`/`deleteTag`.
- **D-05a:** The page list for explicit selection comes from the existing
  `listPages`/tree endpoint (verify the exact API in planning). The tag vocabulary
  comes from the existing `listTags` (`client.ts:1371`).

### The agent's Discretion
- The exact page-list API for explicit selection (verify the existing tree/page-list
  endpoint name in planning; reuse, do not invent).
- The BulkTagDialog UX layout (side-by-side add/remove vs stacked) — keep it
  consistent with the existing dialogs' visual density.
- Whether the filter-tag multi-select is a chip-picker or a multi-select dropdown —
  reuse an existing picker component if one lives in the codebase.
- Commit slicing (handler+compute+test as one commit; frontend as a second; or
  together) — the engineer's call.
</decisions>

<disciplines>
## Lenses surfaced for this phase

> **Lenses, not rules.** Each entry below is a perspective the architect surfaced
> for this phase. Downstream agents adopt, refine, or reason past them; divergence
> with reasoning is a successful interaction.

~~~yaml
disciplines:
  - section: "TF-01 write path"
    name: "no-new-write-path"
    scope: ["internal/server/handlers_tags_admin.go", "internal/pages/batch.go"]
    guidance: "The bulk op MUST route through the existing buildTagApplyItems +
      ApplyTagsBatch seam. Do NOT add a second batched-write path, a parallel
      commit, or a per-page loop. The one byte-stable single-writer path is the
      invariant the v1.5 security review audited."
    rationale: "A second write path forks the audited surface and can drift on
      byte-stability, 409-handling, or commit-atomicity. The seam is already
      factored for reuse (compute-callback shape)."
    override_signals:
      - "A provable defect in ApplyTagsBatch itself that forces a new path —
        surface it to the architect; do not silently fork."
  - section: "TF-01 page set"
    name: "server-re-resolves-page-set"
    scope: ["the bulk handler preview + apply"]
    guidance: "The page set is re-derived server-side from the live store at BOTH
      preview and apply. The client's explicit path list is a request, never a
      trusted set — a path may be deleted or renamed between preview and apply.
      Filter-tags are re-resolved via PagesByTag at apply time."
    rationale: "TOCTOU safety: the existing tag-admin ops rely on server-side
      re-read + base_revision 409. The bulk op's larger page set makes the
      preview→apply gap wider, so the re-derive floor matters more, not less."
    override_signals:
      - "None — this is the existing pattern; no reason to trust the client set."
~~~

</disciplines>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Phase spec & requirements
- `.planning/ROADMAP.md` §"Phase 31: Bulk Multi-Tag Operations" — goal, success
  criteria, key files, design decisions.
- `.planning/REQUIREMENTS.md` §TF-01 (lines 108-124) — acceptance criteria (the
  authoritative bar).

### The write seam to reuse (non-negotiable — do not fork)
- `internal/pages/batch.go:37-63` (`TagApplyItem` — set `BodyOld=""`, `BodyNew=""`,
  `SkipFrontmatter=false` for a frontmatter-only bulk op), `:72-226`
  (`ApplyTagsBatch` — the single-writer batched commit, per-page 409 exclusion).
- `internal/server/handlers_tags_admin.go:194-252` (`buildTagApplyItems` — the
  compute-callback seam; pass a new `computeBulkTags` func), `:58-72`
  (`tagPreviewResponse`/`tagApplyResponse` — reuse the preview/apply response
  shapes), `:265-505` (rename/merge/delete handlers — the preview→approve pattern
  to mirror for the bulk handler).
- `internal/okf/repair.go:159` (`ValidateTagCharset` — the frontmatter charset gate
  for added tags).

### Page-set resolution
- `internal/graph/query.go:428-447` (`PagesByTag` — the existing-tag-filter query;
  returns page paths for a tag from the derived cache).
- `internal/pages/tree.go:36` (`Service.Tree()` — the SEC-01-resolver page
  enumeration for the explicit-selection list).

### Routing & audit
- `internal/server/router.go:321-343` (the admin tag route subgroup —
  `RequireRole(admin)` + nosurf CSRF; register `POST /admin/tags/bulk` here).
- `internal/audit/audit.go:63-67` (`ActionTagRename/Merge/Delete` — add
  `ActionTagBulk` in this block).

### Frontend
- `web/src/components/admin/TagManagement.tsx:20-301` (the admin tag surface +
  existing dialogs to mirror for `BulkTagDialog`).
- `web/src/api/client.ts:1348-1417` (`renameTag`/`mergeTag`/`deleteTag` +
  `TagPreview`/`TagApplyResponse`/`TagPreviewPage` types — mirror for `bulkTag`).
- `web/src/api/client.ts:1371` (`listTags` — the vocabulary for tag autocomplete
  inputs).

### No external specs
- No third-party specs/ADRs — requirements fully captured above.
</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `buildTagApplyItems` (`handlers_tags_admin.go:205`) — the compute-callback seam;
  a new `computeBulkTags(current []string) ([]string, bool)` plugs in directly. The
  `bodyOld/bodyNew` parameters are `""` for a frontmatter-only op.
- `ApplyTagsBatch` (`batch.go:72`) — the proven single-writer batched commit
  (N pages → 1 commit, per-page 409, byte-stable tags-region write). No changes
  needed to it.
- `tagPreviewResponse`/`tagApplyResponse` (`handlers_tags_admin.go:58-72`) — the
  preview/apply response shapes; reuse for the bulk op.
- `PagesByTag` (`graph/query.go:428`) — the existing-tag-filter query; no new
  query.
- `ValidateTagCharset` (`repair.go:159`) — the frontmatter charset gate for added
  tags.
- The admin route subgroup (`router.go:321-343`) — `RequireRole(admin)` + nosurf
  CSRF already wrap every tag-admin endpoint; the bulk endpoint inherits both.

### Established Patterns
- Preview→approve: `preview=true` returns affected pages without writing; approve
  applies via `ApplyTagsBatch` (the exact pattern of rename/merge/delete).
- Per-page 409 exclusion: a stale/missing page is reported in its
  `TagApplyResult` and excluded without sinking the batch (`batch.go:75-79`).
- Server-side re-validation: the client tag list is never trusted; the handler
  re-validates each added tag via `ValidateTagCharset`.
- Frontend dialogs: `RenameDialog`/`MergeDialog`/`DeleteDialog` share a
  preview-table + approve-button pattern; `BulkTagDialog` mirrors it.

### Integration Points
- Router: add `admin.Post("/admin/tags/bulk", h.handleTagBulk)` in the admin tag
  subgroup (`router.go:343` after `delete`).
- Audit: add `ActionTagBulk = "tag_bulk"` in the tag-management action block
  (`audit.go:67`).
- Frontend: add `bulkTag` to `client.ts` (mirror `renameTag`); add
  `BulkTagDialog` to `TagManagement.tsx`; add a "Bulk add/remove" button beside
  the existing rename/merge/delete actions.

### Landmines
- **Do NOT send `base_revision` from the client** — it is server-read at preview,
  re-checked at apply (the existing pattern, `batch.go:42-43`). A client-claimed
  revision is a trust violation.
- **Do NOT rewrite bodies** — bulk add/remove is frontmatter-only. `BodyOld=""`
  disables the body channel. Setting `BodyOld`/`BodyNew` would invoke
  `RewriteHashtags` (the v1.5 body channel) — that is NOT this op.
- **The page set must be re-resolved at apply time** — a page deleted/renamed
  between preview and apply must be `notfound`/`stale`, not clobbered. The client's
  explicit path list is advisory; the server re-derives the live set.
- **A no-op page (set unchanged) is excluded** — the existing `changed` predicate
  in `buildTagApplyItems` handles this; do not add a no-op page to the batch.
</code_context>

<specifics>
## Specific Ideas

- The golden test (TF-01 crit 4): add `[a, b]` + remove `[c]` across 3 pages →
  correct per-page frontmatter in 1 commit; a stale page reported `stale` and
  excluded. Model it on the existing rename/merge golden tests
  (`handlers_tags_admin_test.go`).
- The preview table should show per-page `current_tags → new_tags` (the
  `TagPreviewPage` shape, `client.ts:1351-1358`) so the admin sees exactly what
  changes before approving — same visual contract as rename/merge/delete.
</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope. The tag-op undo (Phase 35) will
build on this phase's op being a journaled, invertible operation, but that is
Phase 35's scope, not this one's.
</deferred>

---

*Phase: 31-bulk-multi-tag-operations*
*Context gathered: 2026-07-02*