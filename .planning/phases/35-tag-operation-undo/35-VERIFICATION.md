---
author: qa
responsible: qa
phase: 35
phase_type: implementation
status: verified
hard_bar: true
security_relevant: true
gate_result:
  id: 35-verify-01
  verdict: pass
  verdict_word: passed
  remediation_owner: ""
  evidence_tier: HIGH
  commit: 8bcd7ce
  date: 2026-07-02
---

# Phase 35: Tag-Operation Undo — Verification Report

**Verifier:** qa (independent — judge ≠ doer)
**Commit verified:** 8bcd7ce ("Phase 35 (complete): Tag-Operation Undo (TF-05) — redo fix + SUMMARY")
**Verdict:** **PASSED** — all four TF-05 acceptance criteria met against code reality.

## Acceptance handshake

- **Inputs complete:** PLAN (35-01-PLAN.md), SUMMARY (35-SUMMARY.md), CONTEXT (35-CONTEXT.md), REQUIREMENTS §TF-05 (lines 168-187) all present and consistent. SUMMARY claims to have done what the PLAN asked.
- **Goal verifiable:** "Undo the last tag-admin op via inverse ApplyTagsBatch; byte-stable, 1 commit, per-page 409-guarded, forward-commit" — fully verifiable by test + code read.
- **PLAN ↔ SUMMARY consistency:** SUMMARY's claimed symbols (`handleTagUndo`, `journalOp`, `ActionTagUndo`, `undoLastTagOp`, migration 0013) all confirmed present in code with `get_declaration_body`/grep against reality.

## Evidence — code reality (HIGH tier, SMTC + grep + test-run grounded)

### 1. Op-journal table (TF-05-1) — VERIFIED
- `internal/store/migrations/0013_tag_op_journal.sql` exists (read in full, 15 lines).
- `CREATE TABLE IF NOT EXISTS tag_op_journal` with columns `(id, op_type, op_detail, pages, created_at)` — matches PLAN §Task 1 verbatim.
- Auto-registered via `//go:embed migrations/*.sql` in `internal/store/migrations.go:13-14`; `loadMigrations()` (line 70) reads all `NNNN_*.sql` from the embedded FS, so 0013 is picked up with no code change. Idempotent via `schema_migrations` (lines 40-48).
- **Journaling types** `journalPage` + `journalRecord` present; **`journalOp`** helper at `handlers_tags_admin.go:1037` — non-fatal (logs warn on insert failure, line 1057; the op already committed).
- **Hooked into all 4 handlers** after a successful `ApplyTagsBatch`:
  - `handleTagRename` → `h.journalOp(ctx, "rename", …)` at line 439
  - `handleTagMerge`  → `h.journalOp(ctx, "merge", …)`  at line 524
  - `handleTagDelete` → `h.journalOp(ctx, "delete", …)` at line 593
  - `handleTagBulk`   → `h.journalOp(ctx, "bulk", …)`   at line 720
  - Plus the undo handler itself journals at line 1252 (5 call sites + def).
- Each `journalPage` carries `{page_path, pre_op_tags, body_affected}` — the minimum invertible record per TF-05-1.

### 2. Undo handler + inverse replay (TF-05-2) — VERIFIED
- `func (h *authHandlers) handleTagUndo` at `handlers_tags_admin.go:1068`.
- **Reads the LAST journal row:** `SELECT id, op_type, op_detail, pages FROM tag_op_journal ORDER BY id DESC LIMIT 1` (line 1079-1081). No row → 400 "No tag operation to undo." (line 1083).
- **Builds inverse TagApplyItems per page** (lines 1111-1215):
  - **FRESH revision:** reads `pg, err := h.pages.Get(ctx, jp.PagePath)` (line 1112) and uses `BaseRevision: pg.Revision` (line 1210) — NOT a stored revision. TOCTOU-safe; the 409-guard in `ApplyTagsBatch` is the load-bearing floor underneath.
  - **rename undo:** `bodyOld, bodyNew = d.NewName, d.OldName` (swap, line 1127).
  - **merge undo:** `bodyOld, bodyNew = d.Target, d.Source` (swap, line 1136).
  - **delete undo:** `bodyOld, bodyNew = "", ""` (frontmatter-only, line 1118 default).
  - **bulk undo:** `bodyOld, bodyNew = "", ""` (frontmatter-only).
  - **undo-of-undo (redo):** `case "undo"` (line 1153) unwraps the original op's `{op_type, op_detail}` and applies the ORIGINAL body direction (lines 1172, 1179) — redo = original direction, not a swap-of-swap.
  - **Semantic stale check** (lines 1194-1200): compares the page's CURRENT frontmatter tags against `expectedPostOp(jp.PreOpTags)` via `sameTagSet`; mismatch → `TagApplyStale` + skip (never clobber). The 409-guard in `ApplyTagsBatch` (batch.go:131-134, `current != it.BaseRevision → TagApplyStale`) is the structural floor beneath this.
- **Calls `ApplyTagsBatch(ctx, items, actor)`** (line 1223) — one forward commit, per-page 409-guarded. Confirmed `ApplyTagsBatch` signature at `internal/pages/batch.go:93`; the 409-guard is at lines 124-134 (stale revision → `TagApplyStale`, excluded from commit, no clobber, batch continues).
- **Semantic helpers present:** `computeRenameTags` (line 156), `computeMergeTags` (line 175), `computeDeleteTags` (line 199), `computeBulkTags` (line 248), `sameTagSet` (line 226) — all verified by grep.

### 3. Admin-only + CSRF + audited; forward-commit; undo-of-undo (TF-05-3) — VERIFIED
- **Route:** `admin.Post("/admin/tags/undo", h.handleTagUndo)` at `router.go:369`, inside the `admin` subgroup (line 308) which applies `admin.Use(auth.RequireRole(auth.RoleAdmin))` (line 309) + inherits the global nosurf CSRF on mutating methods (comment at lines 306-307).
- **RBAC test:** `TestUndoRBAC` — editor gets 403 (line 1488-1498). PASS confirmed in test run.
- **Audited:** `audit.ActionTagUndo = "tag_undo"` at `internal/audit/audit.go:75`; recorded at `handlers_tags_admin.go:1231-1237` with actor, target, detail, source. Non-fatal (assigned to `_`).
- **Forward-commit:** the undo handler calls `h.pages.ApplyTagsBatch(ctx, items, actor)` (line 1223) — a forward write through the single-writer commit path. Grep for `git revert|RestoreVersion` in `handlers_tags_admin.go` returns NO matches in the undo path. Consistent with `pages.RestoreVersion` (history.go:152-163: "writes the page's chosen old version back as a NEW forward commit… NEVER rewinds or rewrites history").
- **Undo-of-undo:** explicitly supported (not scoped out). The undo op is itself journaled (line 1252) with `pre_op_tags = currentTags` (the state BEFORE the undo = post-original-op state, line 1206/1248) and `op_detail` carrying the ORIGINAL op's `{op_type, op_detail}` (line 1252). `TestUndoOfUndo` (line 1685) proves redo restores the rename. PASS confirmed.

### 4. Golden test (TF-05-4) — VERIFIED
- `TestUndoRenameGolden` (line 1513): seeds `a.md` with tags `[ops]` + body `"See #ops for details."`; captures `preTags` + `preBody`; renames `ops→devops` (asserts body now has `#devops`, no `#ops`); undoes; asserts:
  - Frontmatter tags restored to pre-rename (line 1541-1543).
  - **Body byte-match:** `if got := pageBody(t, f, "a.md"); got != preBody` (line 1545) — strict byte equality, not substring. The undo's body rewrite (`RewriteHashtags(body, "devops", "ops")`) restores `#ops` byte-for-byte.
- **Intervening-edit page skipped:** `TestUndoStale` (line 1552): renames two pages, manually edits `a.md` (new revision + commit), undoes; asserts `statuses["stale"] == 1` and `statuses["applied"] == 1` (lines 1594-1599), `b.md` restored to `[ops]`, `a.md` keeps `[manual]` (lines 1601-1606). PASS confirmed.

### 5. Delete body #tags limitation — VERIFIED (stated, not invertible)
- The undo handler surfaces a `note` field for delete ops (lines 1255-1263): "Undo restores frontmatter tags only; body #tags removed by the original delete are not re-inserted (they self-heal on the next edit)."
- `TestUndoDelete` (line 1612): asserts frontmatter tags restored to `[ops]` (line 1637-1639), body does NOT contain `#ops` (line 1641-1643 — the limitation), and the response carries the `note` (line 1645-1648). PASS confirmed.
- Stated in SUMMARY §Limitation (lines 86-94) and surfaced in the undo response.

### 6. Frontend — VERIFIED
- `web/src/api/client.ts:1466`: `export async function undoLastTagOp(): Promise<TagApplyResponse & { note?: string }>` — POST `/api/v1/admin/tags/undo`.
- `web/src/components/admin/TagManagement.tsx`: `Undo2` icon imported (line 8); `undoLastTagOp` imported (line 15); `undoLast` handler (lines 95-114) calls the API, shows applied/stale counts + note, invalidates `TAGS_KEY` via `onDone`; button rendered in JSX (lines 164-171) with `tagmgmt-undo-btn` class, busy state, title; `undoMsg` + `undoError` rendered (lines 172-173). 400 → "Nothing to undo." subtle message (line 110).
- CSS for `.tagmgmt-undo-btn` + `.tagmgmt-undo-note` in `TagManagement.css:163-178`.

## Test run evidence (qa's own execution, 2026-07-02)

```
$ go test ./internal/server/ -run 'TestJournal|TestUndo' -count=1 -v
--- PASS: TestJournalRename (0.10s)
--- PASS: TestJournalBulk (0.08s)
--- PASS: TestUndoRBAC (0.06s)
--- PASS: TestUndoNothing (0.05s)
--- PASS: TestUndoRenameGolden (0.12s)
--- PASS: TestUndoStale (0.12s)
--- PASS: TestUndoDelete (0.10s)
--- PASS: TestUndoBulk (0.11s)
--- PASS: TestUndoOfUndo (0.13s)
PASS
ok  	github.com/postfix/okworkspace/internal/server	0.907s
```
All 9 tests pass (2 journal + 7 undo).

```
$ go build ./...
(exit 0, 1.47s)

$ cd web && npx tsc --noEmit
EXIT=0 (0.30s)
```

## TF-05 acceptance criteria checklist

- [x] **Each tag-admin op records a minimum invertible record durably in a dedicated op-journal.** Migration 0013 `tag_op_journal`; `journalOp` hooked into all 4 handlers; record = `{op_type, op_detail, pages: [{page_path, pre_op_tags, body_affected}]}`.
- [x] **Undo replays inverse via ApplyTagsBatch (byte-stable, 1 commit); per-page 409-guarded (stale skipped, never clobbered).** `handleTagUndo:1223` calls `ApplyTagsBatch`; batch.go:131-134 is the 409-guard; semantic stale check at 1194-1200; `TestUndoStale` proves skip.
- [x] **Admin-only + CSRF + audited; forward-commit; undo-of-undo possible or explicitly scoped out.** `RequireRole(admin)` + nosurf CSRF on the admin subgroup; `ActionTagUndo` audited; forward via `ApplyTagsBatch` (no `git revert`); undo-of-undo supported + `TestUndoOfUndo` passes.
- [x] **Golden test: rename #ops→#devops, undo → frontmatter + body #tags byte-match pre-rename; intervening-edit page skipped.** `TestUndoRenameGolden` asserts strict byte equality (`got != preBody`); `TestUndoStale` asserts the edited page is skipped with a clear report.

## Gaps found

None. All four TF-05 criteria are met against code reality, with HIGH-tier evidence (code reads + own test run + build + tsc).

## Notes

- The op-journal is operational/derived data in SQLite (appropriate; not user-authored content) — consistent with the 0010 `tag_suggestions` staging-table tone and the project's "SQLite holds operational data only" rule (PROJECT.md §Storage model).
- The redo fix noted in the SUMMARY (preUndoTags + op_detail carrying the original op) was verified against `TestUndoOfUndo` passing — the second undo correctly restores the rename (tags = devops).