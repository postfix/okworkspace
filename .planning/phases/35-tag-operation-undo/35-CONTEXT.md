---
author: architect
responsible: architect
phase: 35
phase_type: implementation
status: Ready for planning
hard_bar: true
security_relevant: true
design_fork: true
parent_artifacts:
  - .planning/ROADMAP.md
  - .planning/REQUIREMENTS.md
---

# Phase 35: Tag-Operation Undo - Context

**Gathered:** 2026-07-02
**Status:** Ready for planning

<domain>
## Phase Boundary

Undo the last tag-admin operation (rename/merge/delete/bulk) by replaying its
inverse through `ApplyTagsBatch` — byte-stable, one commit, per-page 409-guarded,
forward-commit (never a history rewrite) — backed by a dedicated SQLite op-journal
table. This is the hardest tag-track phase: it inverts all four op types (including
body `#tag` rewriting for rename/merge/delete), and the 409-guard is the load-bearing
safety invariant.

**Requirements:** TF-05

**Non-goals:** multi-level undo (v1.6 is last-op undo; a full undo stack is deferred);
undo of non-tag operations (page edits, attachment ops); git history rewrite (undo
is a forward commit, consistent with `pages.RestoreVersion`).

**Depends on:** Phase 31 (bulk is an undoable op + validates the inverse-apply pattern).
The op-journal is a new SQLite table — no Phase 32 dependency (co-driver chose the
new-table option over the metadata-store option).
</domain>

<decisions>
## Implementation Decisions

### Op-journal shape (D-01) — new SQLite table [CO-DRIVER CHOSEN]
- **D-01:** A new `tag_op_journal` SQLite table holds the invertible op records.
  Operational/derived data (NOT content — unlike tag metadata, an op-journal is
  machine-generated operational state, not user-authored content). Mirrors the
  `tag_suggestions` staging table (migration `0010`). A new migration
  `0013_tag_op_journal.sql` creates it:
  ```sql
  CREATE TABLE IF NOT EXISTS tag_op_journal (
      id         INTEGER PRIMARY KEY AUTOINCREMENT,
      op_type    TEXT NOT NULL,          -- "rename" | "merge" | "delete" | "bulk"
      op_detail  TEXT NOT NULL,          -- JSON: op-specific data (old/new names, add/remove sets)
      pages      TEXT NOT NULL,          -- JSON: [{page_path, pre_op_tags, body_affected}]
      created_at TEXT NOT NULL DEFAULT (datetime('now'))
  );
  ```
  Only the LAST row is undoable (v1.6 = last-op undo). A new row is inserted after
  every successful tag-admin apply (rename/merge/delete/bulk). The journal is
  append-only (no updates, no deletes — history is preserved; a cleanup could trim
  old rows but that's out of scope).

### The invertible record (D-02) — per-page pre-op tags + op intent
- **D-02:** Each journal row carries:
  - `op_type`: the operation type (`"rename"`, `"merge"`, `"delete"`, `"bulk"`).
  - `op_detail`: JSON of the op-specific data:
    - rename: `{old_name, new_name}`
    - merge: `{source, target}`
    - delete: `{tag}`
    - bulk: `{add_tags, remove_tags}`
  - `pages`: JSON array of `{page_path, pre_op_tags, body_affected}`:
    - `pre_op_tags`: the page's frontmatter tags BEFORE the op (captured from
      `buildTagApplyItems`'s `currentTags` — the `tagPreviewPage.CurrentTags` field).
    - `body_affected`: whether the op rewrote the body (`tagPreviewPage.BodyAffected`).
  - This is enough to reconstruct the inverse: for each page, restore the pre-op
    frontmatter tags AND reverse the body rewrite (swap bodyOld/bodyNew).

### Undo mechanism (D-03) — inverse ApplyTagsBatch
- **D-03:** `POST /admin/tags/undo` — admin-only, CSRF-protected:
  1. Read the last journal row (`SELECT ... ORDER BY id DESC LIMIT 1`).
  2. If no row → 404/400 ("nothing to undo").
  3. For each page in the record, build an inverse `TagApplyItem`:
     - `Tags`: the `pre_op_tags` (restore the original frontmatter tags).
     - `BaseRevision`: read FRESH from `pages.Get` at undo time (NOT the stored
       revision — the page may have been edited since the op; the fresh read is the
       TOCTOU-safe source for the 409 check).
     - `SkipFrontmatter`: false (we're restoring frontmatter tags).
     - `BodyOld`/`BodyNew`: SWAPPED from the original op:
       - rename (old→new): undo swaps to (new→old) — `BodyOld=new_name, BodyNew=old_name`.
       - merge (source→target): undo swaps to (target→source) — `BodyOld=target, BodyNew=source`.
       - delete (tag→""): undo restores — `BodyOld="", BodyNew=tag` (re-insert the tag).
         Wait — delete's body rewrite is `BodyOld=tag, BodyNew=""` (remove). Undo is
         `BodyOld="", BodyNew=tag`? No — the body rewrite is `RewriteHashtags(body, old, new)`.
         Delete is `RewriteHashtags(body, tag, "")` — removes `#tag`. Undo is
         `RewriteHashtags(body, "", tag)`? That doesn't make sense (old="" is a no-op).
         **Correction:** undo of a delete body rewrite needs to RE-INSERT the deleted
         `#tag` at its original positions. But `RewriteHashtags` is a splice, not an
         insert — it can't reconstruct deleted spans. So **delete undo can only
         restore frontmatter tags**, NOT body `#tags` that were removed. This is a
         known limitation: body `#tags` removed by a delete are gone (the body bytes
         were rewritten; the original bytes are in git history, not in the page).
         The undo restores the frontmatter `tags:` list (the tag reappears in
         frontmatter) but body `#tags` removed by the delete are NOT re-inserted
         (they self-heal on the next edit via the client re-materializer). State
         this limitation in the undo response + the SUMMARY.
       - bulk: `BodyOld=""`, `BodyNew=""` — bulk is frontmatter-only (no body rewrite);
         undo is also frontmatter-only (restore pre_op_tags).
  4. Call `ApplyTagsBatch` with the inverse items — one commit, per-page 409-guarded.
  5. Audit: `audit.ActionTagUndo = "tag_undo"` (new constant).
  6. Delete or mark the journal row as undone (append a new row for the undo op
     itself? — D-04 below).

- **D-03a (body rewrite inversion for rename/merge):** rename's body rewrite is
  `RewriteHashtags(body, old_name, new_name)`. Undo is `RewriteHashtags(body, new_name, old_name)`
  — swap old/new. This WORKS because `RewriteHashtags` is a byte splice: it finds
  `#new_name` and rewrites to `#old_name`, restoring the original. Merge is the same
  (`#target` → `#source`). Delete's body rewrite (`#tag` → `""`, removal) is NOT
  invertible (the span is gone) — stated limitation.

### Undo-of-undo (D-04) — scoped out in v1.6
- **D-04:** Undo-of-undo is explicitly scoped OUT in v1.6. The undo op itself is
  journaled (so the journal's last row is the undo, which is itself invertible), but
  the UI does not expose a "redo" button. A second undo would invert the undo (which
  is a valid op — the undo is an ApplyTagsBatch like any other). The planner can
  either journal the undo op (making a second undo = redo) or not journal it (making
  undo a one-shot). Recommend: journal the undo op (so the undo IS an undoable op —
  undo-of-undo = redo works for free). The UI shows only "Undo last tag operation"
  (one button); pressing it twice undoes then redoes.

### Journaling integration (D-05) — hook into existing apply paths
- **D-05:** Each tag-admin handler (rename/merge/delete/bulk) inserts a journal row
  AFTER a successful apply (after `ApplyTagsBatch` returns). The row is inserted in
  the same request (not async). The `tagPreviewPage` data (already computed by
  `buildTagApplyItems`) carries the `pre_op_tags` + `body_affected` per page — the
  journal row is built from it. This is an additive change to each handler (a few
  lines: marshal the op detail + page records, INSERT into `tag_op_journal`).

### Frontend (D-06) — undo button
- **D-06:** `TagManagement.tsx` gains an "Undo last tag operation" button (near the
  tag table header). Clicking calls `POST /admin/tags/undo` via a new `undoLastTagOp`
  client function. The response shows the undo results (per-page status:
  applied/stale/notfound) + the limitation note for delete body `#tags`. On success,
  invalidates `TAGS_KEY`. The button is disabled when there's nothing to undo (the
  client can check via a `GET /admin/tags/undo/status` or just let the server return
  404/400 and show the error).

### The agent's Discretion
- Whether the undo journals itself (recommend: yes — undo-of-undo works for free).
- The exact journal row shape (JSON columns vs typed columns — recommend JSON for
  flexibility, matching `tag_suggestions`).
- Whether a `GET /admin/tags/undo/status` endpoint is needed (recommend: no — the
  undo button calls POST directly; a 404 = "nothing to undo" is enough).
- Commit slicing.
</decisions>

<disciplines>
## Lenses surfaced for this phase

~~~yaml
disciplines:
  - section: "TF-05 undo"
    name: "forward-commit-never-rewrite"
    scope: ["the undo handler", "ApplyTagsBatch"]
    guidance: "Undo is a FORWARD COMMIT through ApplyTagsBatch — never a git history
      rewrite. Consistent with pages.RestoreVersion: a restore creates a new commit
      with the old content, it does not git-revert. The op-journal records what
      happened; the undo replays the inverse as a new commit."
    rationale: "Git history rewrite breaks the hidden-Git model (commits are automatic,
      users don't manage history). A forward commit is safe, auditable, and consistent
      with the existing restore pattern."
    override_signals:
      - "None — this is the project's versioning invariant, not a lens. A history
        rewrite is a constraint violation."
  - section: "TF-05 409-guard"
    name: "stale-page-never-clobbered"
    scope: ["the undo handler's inverse ApplyTagsBatch"]
    guidance: "A page edited since the original op is reported stale and SKIPPED from
      the undo — never clobbered. The 409-guard reads the FRESH revision at undo time,
      not the stored one. This is the same invariant the original op relies on."
    rationale: "The server-authoritative model's core safety property: a concurrent
      edit is never silently overwritten. Undo must not weaken this — a page that
      moved on must be skipped, not force-restored."
    override_signals:
      - "None — clobbering a concurrent edit is a data-loss bug, not an override."
~~~

</disciplines>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Phase spec & requirements
- `.planning/ROADMAP.md` §"Phase 35: Tag-Operation Undo" — goal, success criteria, key files, design constraint.
- `.planning/REQUIREMENTS.md` §TF-05 (lines 168-187) — acceptance criteria (the authoritative bar).

### The apply path to reuse (undo replays the inverse through it)
- `internal/pages/batch.go:37-63` (`TagApplyItem` — the inverse items), `:72-226`
  (`ApplyTagsBatch` — the single-writer batched commit + per-page 409).
- `internal/server/handlers_tags_admin.go:271-318` (`buildTagApplyItems` — the
  per-page currentTags/bodyAffected data that becomes the journal record), `:331-418`
  (`handleTagRename` — the apply path to hook journaling into), `:420-501`
  (`handleTagMerge`), `:503-573` (`handleTagDelete`), `:580-697` (`handleTagBulk`).

### The body rewrite (undo swaps old/new for rename/merge)
- `internal/okf/hashtag.go` (`RewriteHashtags(body, old, new)` — swap old/new for undo).

### The op-journal pattern (new SQLite table)
- `internal/store/migrations/0010_tag_suggestions.sql` — the staging-table pattern
  (operational/derived data, JSON columns, append-only). The `tag_op_journal` table
  mirrors this tone.
- `internal/store/migrations.go:13-65` — the embed + Migrate harness.

### Routing & audit
- `internal/server/router.go:321-343` (admin tag route subgroup — register
  `POST /admin/tags/undo` here).
- `internal/audit/audit.go:63-68` (tag action constants — add `ActionTagUndo`).

### Frontend
- `web/src/components/admin/TagManagement.tsx` (add undo button).
- `web/src/api/client.ts` (add `undoLastTagOp`).

### No external specs
- No third-party specs/ADRs.
</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `ApplyTagsBatch` (`batch.go:72`) — the inverse op rides this unchanged.
- `buildTagApplyItems` (`handlers_tags_admin.go:271`) — the `tagPreviewPage` data
  (CurrentTags, BodyAffected) is exactly the invertible record per page.
- `tag_suggestions` migration (`0010`) — the staging-table pattern for the op-journal.
- `RewriteHashtags` (`hashtag.go`) — body rewrite; undo swaps old/new for rename/merge.

### Established Patterns
- Forward-commit (never history rewrite) — `pages.RestoreVersion` does this.
- Per-page 409 exclusion — `ApplyTagsBatch` handles this; the undo rides it.
- Admin-only RBAC + CSRF inherited from the route subgroup.
- Audit logging (non-fatal).

### Integration Points
- Each tag-admin handler (rename/merge/delete/bulk) inserts a journal row after apply.
- New `handleTagUndo` handler — `POST /admin/tags/undo`.
- New migration `0013_tag_op_journal.sql`.
- New `audit.ActionTagUndo` constant.
- Frontend: undo button in `TagManagement.tsx`; `undoLastTagOp` in `client.ts`.

### Landmines
- **Delete body `#tags` are NOT invertible** — `RewriteHashtags(body, tag, "")` removes
  the span; the original bytes are in git history, not the page. Undo restores
  frontmatter tags but NOT body `#tags` removed by a delete. State this limitation.
- **The 409-guard reads the FRESH revision** — do NOT use the stored BaseRevision
  from the journal; read `pages.Get` at undo time for the current revision. A page
  edited since the op is stale and skipped.
- **Undo is a forward commit** — never `git revert` or history rewrite.
- **The journal is operational data** — SQLite is appropriate (unlike tag metadata,
  the op-journal is NOT user-authored content and does NOT need to be portable as
  plain files; it's machine-generated state).
</code_context>

<specifics>
## Specific Ideas

- The golden test (TF-05 crit 4): rename `#ops`→`#devops` on a page with body `#ops`,
  then undo → the page's frontmatter + body `#tags` byte-match the pre-rename state
  (body: `RewriteHashtags(body, "devops", "ops")` restores `#ops`). An intervening
  edit on a second page → that page is skipped (409), the first page is undone.
- The undo button can show a tooltip: "Undo last: rename #ops → #devops (3 pages)"
  (from the last journal row) so the admin knows what they're undoing before clicking.
</specifics>

<deferred>
## Deferred Ideas

- Multi-level undo (a full undo stack) — deferred beyond v1.6.
- Undo of non-tag operations (page edits, attachment ops) — out of scope.
- Journal cleanup (trim old rows) — out of scope; the table is tiny for a 5-user wiki.
</deferred>

---

*Phase: 35-tag-operation-undo*
*Context gathered: 2026-07-02*