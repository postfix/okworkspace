---
author: architect
responsible: architect
phase: 33
phase_type: implementation
status: Ready for planning
hard_bar: false
security_relevant: false
design_fork: false
parent_artifacts:
  - .planning/ROADMAP.md
  - .planning/REQUIREMENTS.md
---

# Phase 33: Tag Hierarchy - Context

**Gathered:** 2026-07-02
**Status:** Ready for planning

<domain>
## Phase Boundary

Tags form a cycle-safe parent/child hierarchy in the Phase-32 `Tags/*.md` metadata
store, shown grouped in the admin list. Hierarchy is **display/organization only** in
v1.6 — no auto-apply of parent tags, no descendant-expanding search, no page bytes
changed by a hierarchy edit. This is the narrowest tag-track phase: it adds one
frontmatter field (`parent`) + cycle detection + grouped rendering.

**Requirements:** TF-03

**Non-goals:** hierarchy-aware behavior (auto-apply parent tags, descendant-expanding
search, hierarchy graph grouping — all stated Future items); changing any page bytes
(hierarchy is metadata, not a page operation); multi-level undo of hierarchy edits.

**Depends on:** Phase 32 (the metadata store). The `parent` field extends the
`Tags/{tag}.md` frontmatter schema chosen in Phase 32.
</domain>

<decisions>
## Implementation Decisions

### The parent field (D-01) — one frontmatter key
- **D-01:** The Phase-32 `Tags/{tag}.md` frontmatter gains a `parent` field:
  ```yaml
  ---
  tag: deploy
  color: "#4caf50"
  description: "Deployment & release ops"
  parent: ops          # optional; the parent tag name (normalized); absent = root
  ---
  ```
  `parent` is the normalized parent tag name, or empty/absent for a root tag. The
  field is metadata-only — setting/changing it changes zero page bytes (no
  frontmatter or body on any wiki page is touched).

### Cycle detection (D-02) — server-side, total
- **D-02:** The `tagmeta.SetMeta` (or a new `SetParent` method) rejects cycles and
  self-parenting server-side. A tag cannot be its own ancestor. The check walks the
  parent chain from the target tag up to the root (or until the target is revisited →
  cycle). The check is O(depth) — the hierarchy is shallow for a ~5-user wiki, so a
  simple walk suffices (no need for a topological sort or SCC). Self-parenting
  (`parent: deploy` on `Tags/deploy.md`) is rejected directly.
- **D-02a:** The cycle check reads the parent chain from the on-disk store (or the
  cache, which is in sync after a rebuild). The check happens BEFORE the write — a
  cycle-producing `parent` value is rejected with a 400, not written to disk.
- **D-02b:** Orphan/root tags are valid (no parent = root). A tag whose parent doesn't
  exist in the vocabulary is also valid (it's a dangling parent reference — displayed
  as a root with a note, or simply treated as root; the planner picks — recommend
  treating a non-existent parent as root with no error, since tags can be renamed/
  deleted out from under a hierarchy).

### Endpoint (D-03) — reuse the Phase-32 metadata endpoint
- **D-03:** Hierarchy edits go through the SAME `POST /admin/tags/meta` endpoint as
  TF-02 (per the acceptance criterion: "hierarchy edits use the same admin-only +
  CSRF + validated endpoint as TF-02 metadata"). The `tagMetaRequest` gains a
  `parent` field. The handler validates the parent (exists in vocabulary OR is empty
  OR is a valid tag name that will exist — the cycle check runs after the parent is
  resolved). The `tagmeta.Store.SetMeta` method gains a `parent` parameter.
- **D-03a:** The `Meta` struct gains `Parent string`. The `tag_meta` cache table gains
  a `parent TEXT NOT NULL DEFAULT ''` column. A new migration `0012_tag_meta_parent.sql`
  adds the column (additive — `ALTER TABLE tag_meta ADD COLUMN parent TEXT NOT NULL DEFAULT ''`).
  The rebuild harness populates it from the frontmatter.

### Grouped admin rendering (D-04) — parent → children grouping
- **D-04:** The admin tag list (`TagManagement.tsx`) displays tags grouped by
  hierarchy: root tags as section headers, their children indented beneath. Tags
  with no parent are top-level. Tags with a parent are shown under their parent. A
  tag whose parent doesn't exist is shown at top-level (orphaned, per D-02b). The
  grouping is display-only — the data model is flat (each tag has one optional
  parent); the grouping is computed client-side from the flat list.

### The agent's Discretion
- The exact grouped rendering shape (indented list vs tree vs collapsible sections —
  keep it simple: indented list under parent headers).
- Whether a non-existent parent is an error or treated as root (recommend: root, no
  error — tags can be deleted out from under a hierarchy).
- Whether `SetParent` is a separate method or folded into `SetMeta` (recommend: fold
  into `SetMeta` — one endpoint, one write).
- Commit slicing.
</decisions>

<disciplines>
## Lenses surfaced for this phase

No disciplines surfaced — phase scope didn't call for any (after applying the
override-signals test). The cycle-detection constraint is a must-have (belongs in
PLAN `must_haves`), not a lens. The display-only scope is a constraint, not a lens.

</disciplines>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Phase spec & requirements
- `.planning/ROADMAP.md` §"Phase 33: Tag Hierarchy" — goal, success criteria, key files.
- `.planning/REQUIREMENTS.md` §TF-03 (lines 143-155) — acceptance criteria.

### The store to extend (built in Phase 32)
- `internal/tagmeta/store.go` — `SetMeta`, `GetMeta`, `AllMeta`, `RebuildTagMeta`
  (add `parent` parameter + field).
- `internal/tagmeta/meta.go` — `Meta{Tag, Color, Description}` (add `Parent string`).
- `internal/store/migrations/0011_tag_meta.sql` — the cache table (extended by
  `0012_tag_meta_parent.sql`).
- `internal/server/handlers_tags_admin.go` — `handleTagMeta` (add `parent` to the
  request + cycle check), `handleTagList` (include `parent` in the response).

### Frontend
- `web/src/components/admin/TagManagement.tsx` — add grouped rendering.
- `web/src/api/client.ts` — `TagEntry` (add `parent`), `setTagMeta` (add `parent` param).

### No external specs
- No third-party specs/ADRs — requirements fully captured above.
</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `tagmeta.Store` (from Phase 32) — `SetMeta`/`GetMeta`/`AllMeta`/`RebuildTagMeta`.
  The `parent` field extends the frontmatter schema + cache table + Meta struct.
- `handleTagMeta` (from Phase 32) — the admin metadata endpoint; add `parent` field
  + cycle check.
- `TagManagement.tsx` (from Phase 32) — the admin tag table; add grouped rendering.

### Established Patterns
- The Phase-32 store is files-as-truth + rebuildable cache. The `parent` field
  follows the same discipline.
- Cycle detection is a simple parent-chain walk — standard, no special algorithm.
- The admin endpoint is admin-only + CSRF + validated (inherited from the route
  subgroup).

### Integration Points
- `tagmeta.Store.SetMeta` gains a `parent` parameter.
- `tag_meta` cache table gains a `parent` column (migration `0012`).
- `handleTagMeta` validates `parent` (exists or empty) + runs the cycle check.
- `handleTagList` response includes `parent` per tag.
- `TagManagement.tsx` computes the grouped view from the flat tag list.

### Landmines
- **Do NOT auto-apply parent tags to pages** — hierarchy is display-only in v1.6.
  Auto-apply is a stated Future item.
- **Do NOT expand search to descendants** — same, display-only.
- **A non-existent parent is not an error** — a tag can reference a parent that was
  deleted/renamed. Treat as root (or display an "orphaned" note), but don't reject
  the write.
- **The cycle check must run BEFORE the write** — a cycle-producing `parent` is
  rejected with a 400, never written to disk (which would create an un-rebuildable
  cycle in the on-disk store).
</code_context>

<specifics>
## Specific Ideas

- The cycle check: walk the parent chain from the target tag. Start at
  `target.parent`, then `parent.parent`, etc. If we revisit `target` → cycle. If we
  reach an empty parent (root) → valid. Limit the walk depth to a reasonable cap
  (e.g. 100 — far beyond any real hierarchy) as a safety valve against a corrupted
  store with a pre-existing cycle.
</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope. Hierarchy-aware behavior (auto-apply
parent tags, descendant-expanding search, hierarchy graph grouping) is a stated
Future item, deferred beyond v1.6.
</deferred>

---

*Phase: 33-tag-hierarchy*
*Context gathered: 2026-07-02*