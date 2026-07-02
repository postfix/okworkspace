---
author: engineer
responsible: engineer
phase: 35
phase_type: implementation
status: complete
hard_bar: true
security_relevant: true
---

# Phase 35: Tag-Operation Undo — Engineer Summary

**Goal:** Undo the last tag-admin operation (rename/merge/delete/bulk) by replaying
its inverse through `ApplyTagsBatch` — byte-stable, one commit, per-page 409-guarded,
forward-commit — backed by a dedicated SQLite op-journal.

**Requirements:** TF-05

## What was built

### Task 1 — op-journal table + journaling integration
- **Migration** `internal/store/migrations/0013_tag_op_journal.sql` (already committed
  by the prior engineer): append-only `tag_op_journal` table
  `(id, op_type, op_detail JSON, pages JSON, created_at)`. Operational/derived data
  (machine-generated, not user-authored content) — SQLite is appropriate, mirroring
  the `tag_suggestions` staging-table tone (migration 0010).
- **Journaling types** `journalPage` + `journalRecord` on `authHandlers`
  (`handlers_tags_admin.go`): `{page_path, pre_op_tags, body_affected}` per page.
- **`journalOp` helper** (non-fatal — a DB error is logged, never breaks the
  request; the op already committed): marshals op detail + per-page records and
  `INSERT`s into `tag_op_journal`.
- **Hooked into all four tag-admin handlers** AFTER a successful `ApplyTagsBatch`:
  - `handleTagRename` → `journalOp(ctx, "rename", {old_name, new_name}, preview)`
  - `handleTagMerge`  → `journalOp(ctx, "merge", {source, target}, preview)`
  - `handleTagDelete` → `journalOp(ctx, "delete", {tag}, preview)`
  - `handleTagBulk`   → `journalOp(ctx, "bulk", {add_tags, remove_tags}, preview)`
  - The `preview` (`[]tagPreviewPage` from `buildTagApplyItems`) carries
    `CurrentTags` (= pre-op tags) + `BodyAffected` — exactly the invertible record.
- **DB access:** `authHandlers.store *store.Store` (the shared operational DB)
  with `store.DB()`. The field is optional (nil → undo returns 500), following the
  existing optional-dependency pattern.

### Task 2 — undo handler + audit + route
- **`ActionTagUndo = "tag_undo"`** audit constant (`internal/audit/audit.go`).
- **`handleTagUndo`** (`POST /admin/tags/undo`, admin-only + CSRF inherited from the
  admin route subgroup):
  1. Reads the LAST journal row (`SELECT ... ORDER BY id DESC LIMIT 1`).
  2. No row → 400 ("No tag operation to undo").
  3. For each page, builds an inverse `TagApplyItem`:
     - `Tags = pre_op_tags` (restore the original frontmatter tags).
     - `BaseRevision` read FRESH via `pages.Get` at undo time (NOT a stored one —
       the page may have moved; the fresh read is TOCTOU-safe for the 409-guard).
     - `BodyOld`/`BodyNew` swapped for rename/merge:
       - rename undo: `BodyOld=new_name, BodyNew=old_name`.
       - merge undo:  `BodyOld=target, BodyNew=source`.
       - delete undo: `BodyOld="", BodyNew=""` — body `#tags` removed by a delete
         are NOT invertible (the span is gone; see Limitation below). Undo restores
         frontmatter tags only; body `#tags` self-heal on the next edit.
       - bulk undo: `BodyOld="", BodyNew=""` (bulk is frontmatter-only).
     - A **semantic stale check**: if the page's CURRENT frontmatter tags no longer
       match what the op would have produced from `pre_op_tags`, the page was edited
       since the op → reported `stale` and SKIPPED (never clobbered). The 409-guard
       in `ApplyTagsBatch` is the load-bearing safety net underneath this.
  4. `ApplyTagsBatch(ctx, inverseItems, actor)` — one forward commit, per-page
     409-guarded.
  5. Audit `ActionTagUndo` (non-fatal).
  6. **Journal the undo op itself** so undo-of-undo (redo) works for free. The undo
     row's `pre_op_tags` = the tags BEFORE the undo (the post-original-op state), and
     its `op_detail` carries the ORIGINAL op's `{op_type, op_detail}` so a redo can
     swap the body back in the original direction.
- **Route** registered in `router.go`:
  `admin.Post("/admin/tags/undo", h.handleTagUndo)`.
- **Forward-commit:** undo is a FORWARD write through `ApplyTagsBatch`, never a
  `git revert` or history rewrite (consistent with `pages.RestoreVersion`).

### Task 3 — frontend
- **`undoLastTagOp`** API client function (`web/src/api/client.ts`): POST
  `/api/v1/admin/tags/undo`.
- **"Undo last tag operation" button** in `TagManagement.tsx` (beside the tag table
  header, `Undo2` icon). On click: calls `undoLastTagOp()`, shows per-page results
  (applied/stale/notfound), invalidates `TAGS_KEY`. A 400 ("nothing to undo") is
  shown as a subtle "Nothing to undo" message. The delete body-`#tag` limitation
  note is displayed when present in the response.
- **CSS** for `.tagmgmt-undo-btn` + `.tagmgmt-undo-note` in `TagManagement.css`.

## Limitation — delete body `#tags` are NOT invertible

`RewriteHashtags(body, tag, "")` (delete's body rewrite) REMOVES the `#tag` span.
The original bytes are in git history, not in the page — `RewriteHashtags` is a
splice, not an insert, so it cannot reconstruct deleted spans. Undo of a delete
restores the frontmatter `tags:` list (the tag reappears in frontmatter) but does
NOT re-insert body `#tags` that were removed by the delete. They self-heal on the
next edit via the client re-materializer. This is surfaced in the undo response
(`note` field) and stated here.

## Tests (`handlers_tags_admin_test.go`)

`go test ./internal/server/ -run 'TestJournal|TestUndo' -count=1 -v` — all 9 pass:

| Test | Verifies |
|---|---|
| `TestJournalRename` | rename inserts a journal row with correct op_type + op_detail + pages |
| `TestJournalBulk` | bulk inserts a journal row |
| `TestUndoRBAC` | editor gets 403 (admin-only) |
| `TestUndoNothing` | no journal rows → 400 |
| `TestUndoRenameGolden` (TF-05-4) | rename `#ops`→`#devops` on a page with body `#ops`, undo → frontmatter + body `#tags` byte-match pre-rename |
| `TestUndoStale` | rename, then edit one page, then undo → edited page stale+skipped, other page undone |
| `TestUndoDelete` | delete then undo → frontmatter tags restored (body #tags NOT restored — limitation) |
| `TestUndoBulk` | bulk add/remove then undo → frontmatter tags restored |
| `TestUndoOfUndo` | rename, undo, undo again → redo restores the rename (undo-of-undo works) |

## Verification

- `go build ./...` exits 0.
- `go test ./internal/server/ -run 'TestJournal|TestUndo' -count=1 -v` — all 9 pass.
- `cd web && npx tsc --noEmit` exits 0.
- `grep 'ActionTagUndo' internal/audit/audit.go` — match.
- `grep 'handleTagUndo' internal/server/handlers_tags_admin.go` — match.
- `grep 'admin.Post("/admin/tags/undo"' internal/server/router.go` — match.
- `grep 'journalOp' internal/server/handlers_tags_admin.go` — match (def + 5 calls).
- `grep 'undoLastTagOp' web/src/api/client.ts` — match.

## Notes

- The prior engineer wrote the migration, the handler skeleton, journaling, audit
  constant, route, tests, and frontend but ran out of budget before fixing the
  undo-of-undo (redo) path. Two bugs in the redo journaling were fixed in this span:
  1. The undo journal row stored `pre_op_tags = it.Tags` (the tags being WRITTEN by
     the undo = the pre-original-op state), so a redo restored the pre-original-op
     state instead of the post-original-op state. Fixed to store
     `preUndoTags[i]` (the current tags BEFORE the undo = the post-original-op
     state), so a redo correctly restores the post-original-op state.
  2. The undo row's `op_detail` was `{"undid": opType}` — missing the body-swap info,
     so a redo could not rewrite body `#tags`. Fixed to carry the ORIGINAL op's
     `{op_type, op_detail}`; the `case "undo"` branch unwraps it and applies the
     ORIGINAL body direction (redo = original direction, not a swap-of-swap).