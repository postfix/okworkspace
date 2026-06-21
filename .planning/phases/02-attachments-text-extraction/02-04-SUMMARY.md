---
phase: 02-attachments-text-extraction
plan: 04
subsystem: api
tags: [attachments, lifecycle, replace, remove, orphan-delete, single-writer, ref-scan, hidden-git, rbac]

# Dependency graph
requires:
  - phase: 02 (plan 01)
    provides: internal/attachments service (Upload/List/Meta/ResolveBin), ULID id + BinPath/MetaPath/TxtPath helpers, canonical DownloadRefPath(id) link contract, PageReferences scan skeleton, the local commitPayload mirror reusing the ONE pages.KindCommit handler, attachments operational table, editor-gated upload route, audit ActionAttachReplace/ActionAttachDelete consts
  - phase: 02 (plan 02)
    provides: full AttachmentCard (media square + filename + meta line + Download), DeleteConfirmDialog structure analog
  - phase: 02 (plan 03)
    provides: KindExtract job + enqueueExtract, extract_status column + setExtractStatus, ExtractionStatus chip + SSE
  - phase: 01
    provides: single-writer CommitJob (pages.KindCommit + commitPayload supporting Writes+Removes), jobs.Worker, repo safe-path resolver (SEC-01), editor RBAC subgroup (RequireRole), focus-trapped Dialog primitive, @tanstack/react-query
provides:
  - attachments.Service.Replace (reuse SAME id, new bytes+meta in ONE commit, stale-binary removal on ext change, reset+re-enqueue extraction) — ATT-05
  - attachments.Service.Remove (strip canonical link from the page, ref-count across ALL pages, orphan-delete bin+meta+txt in ONE commit when last ref removed, keep shared files) — ATT-06/ATT-07
  - full PageReferences canonical-match consumption (insert and scan share DownloadRefPath — Pitfall 6)
  - PUT /api/v1/attachments/{id} (replace) + DELETE /api/v1/attachments/{id}?page_path= (remove), editor-gated from the session (T-02-14)
  - removes-only commit payload support on the shared pages CommitHandler (orphan delete is a pure deletion)
  - ReplaceAttachmentDialog (non-destructive) + RemoveAttachmentDialog (destructive) wired as editor-only 44px card actions
affects: [phase complete — full attachment lifecycle ships]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Replace reuses the SAME ULID id so every page link (DownloadRefPath(id)) keeps resolving with NO page edit; new bytes+meta land in ONE commit and the stale binary is added to the same commit's Removes only when the sniffed ext changes"
    - "Orphan delete is a removes-only commitPayload (bin+meta+txt in ONE commit) — the shared pages CommitHandler now accepts a payload with zero Writes provided it has Removes"
    - "The unlink edit and the orphan ref-scan both key off the SINGLE canonical DownloadRefPath(id) string (stripAttachmentLinks + PageReferences) so insert and scan can never drift (Pitfall 6)"
    - "Replace/Remove routes ride the existing /attachments/* catch-all (id is the wildcard) under the editor subgroup, mirroring the /pages/* PUT/DELETE pattern — avoids the chi sibling-wildcard conflict a {id} route hits next to the slash-bearing list wildcard"

key-files:
  created:
    - internal/attachments/lifecycle.go
    - internal/attachments/refs_test.go
    - internal/server/handlers_attachments_lifecycle_test.go
    - web/src/components/attachments/ReplaceAttachmentDialog.tsx
    - web/src/components/attachments/RemoveAttachmentDialog.tsx
  modified:
    - internal/attachments/service_test.go (TestReplaceKeepsID/TestOrphanDelete/TestRemoveKeepsSharedFile + contains helper + strings import)
    - internal/pages/commitjob.go (allow removes-only commit payload — Rule 1)
    - internal/server/handlers_attachments.go (handleReplaceAttachment + handleDeleteAttachment)
    - internal/server/router.go (PUT/DELETE /attachments/* under the editor subgroup)
    - web/src/api/client.ts (replaceAttachment + removeAttachment)
    - web/src/components/attachments/AttachmentCard.tsx (editor-only Replace/Remove buttons + dialogs)
    - web/src/components/attachments/AttachmentCard.css (.attachment-card-actions / .attachment-card-action 44px)
    - web/src/components/attachments/AttachmentsSection.tsx (pass pagePath + canEdit to the card)
    - internal/web/dist (rebuilt embedded SPA)

key-decisions:
  - "Replace/Remove reuse the /attachments/* catch-all (id via chi.URLParam '*') rather than a {id} route, to avoid the chi sibling-wildcard conflict 02-01 already worked around for GET; consistent with the /pages/* PUT/DELETE pattern"
  - "Relaxed the pages CommitHandler's len(Writes)==0 guard to require at least one write OR remove — orphan delete is a legitimate removes-only commit; without this the delete job errored and timed out, leaving the file undeleted (Rule 1 bug)"
  - "unlinkPage skips an empty commit when the link is already absent on the page (no-op, no phantom commit)"
  - "Replace resets extract_status to pending and fire-and-forget re-enqueues KindExtract so the card chip transitions live again; a non-extractable new type enqueues nothing"

patterns-established:
  - "Pattern: lifecycle mutations (Replace/Remove) flow through the SAME single-writer enqueueCommit as Upload — no os/git calls; Removes carries the deleted paths so git stages the deletion in one commit"
  - "Pattern: hidden-Git copywriting continues — 'kept in history and can be restored' conveys version retention with zero Git vocabulary; commit messages 'Replace attachment'/'Delete attachment'/'Remove attachment link' never surface"

requirements-completed: [ATT-05, ATT-06, ATT-07]

# Metrics
duration: ~40min
completed: 2026-06-21
---

# Phase 2 Plan 04: Attachment Lifecycle — Replace + Remove + Orphan Delete Summary

**Closed the attachment lifecycle loop: an editor can Replace an attachment (ATT-05 — the SAME opaque ULID id is reused, the new bytes + updated meta are written through the existing single-writer CommitJob in one commit so the prior version is retained in history, and text is re-extracted), and Remove an attachment's link from a page (ATT-06 — the canonical `DownloadRefPath(id)` link is stripped from the page Markdown); when the removed link was the LAST reference across ALL pages, the binary + JSON meta + TXT sidecar are deleted in ONE commit (ATT-07), while a file still referenced by another page is never deleted — all behind editor-only, focus-trapped confirm dialogs with the "Replace file"/"Remove file" labels and hidden-Git retention copy.**

## Performance
- **Duration:** ~40 min
- **Completed:** 2026-06-21
- **Tasks:** 3 (Tasks 1 & 2 TDD; Task 3 frontend)
- **Files created/modified:** 14

## Accomplishments
- **ATT-05 Replace:** `Service.Replace` reuses the SAME id, validates the new bytes exactly like Upload (size cap → ErrTooLarge, magic-byte sniff vs allow-list → ErrTypeForbidden), writes the new binary + updated meta in ONE commit, removes the stale binary in the SAME commit when the sniffed ext changes, resets `extract_status` to pending, and fire-and-forget re-enqueues `KindExtract`. Because the id is unchanged, every page link keeps resolving with no page edit; the prior bytes are retained in history.
- **ATT-06 Remove (unlink):** `Service.Remove` loads the target page, strips every `DownloadRefPath(id)` link via `stripAttachmentLinks` (handling `[text](ref)` and `![alt](ref)` plus a bare-URL fallback), and commits the edited page through the single-writer path.
- **ATT-07 orphan delete:** after unlinking, `PageReferences` scans EVERY page for the canonical link; when the count is zero, ONE removes-only commitPayload deletes `[bin, meta, txt]` (txt only if present) and the operational row is deleted. A file still referenced elsewhere is KEPT — only the link on the target page is dropped (`TestRemoveKeepsSharedFile`).
- **Pitfall 6 honored:** the unlink edit and the orphan ref-scan both key off the single `DownloadRefPath(id)` string, so insert and scan can never drift (grep-verified: `DownloadRefPath` used by both `refs.go` and `lifecycle.go`).
- **HTTP + RBAC:** `PUT /attachments/{id}` (MaxBytesReader-before-parse, re-sniff, returns updated meta) and `DELETE /attachments/{id}?page_path=` (returns `{deleted_orphan}`) ride the `/attachments/*` catch-all under the editor subgroup; a reader session is rejected 403 on both (`TestReplaceRemoveEditorOnly`, T-02-14). Audit `attach_replace`/`attach_delete` recorded on success.
- **Editor-only card UI:** `ReplaceAttachmentDialog` (non-destructive, `.btn-primary`, file picker, "Replace file", "kept in history and can be restored") and `RemoveAttachmentDialog` (destructive, "Remove file", "If no other page uses it, the file is deleted") — both Esc/backdrop CANCEL only. The card renders Replace (`Undo2`) and Remove (`Trash2`, `.btn-ghost-destructive`) 44px icon buttons for editors; readers see only Download.

## Task Commits
1. **Task 1: Replace + PageReferences + Remove on the service (TDD)** — `4001e83` (feat)
2. **Task 2: PUT replace + DELETE remove handlers, routes; removes-only commit fix (TDD)** — `9261c74` (feat)
3. **Task 3: Replace/Remove dialogs wired into the editor-only card** — `9da790e` (feat)

## Files Created/Modified
See frontmatter `key-files`. Highlights:
- `internal/attachments/lifecycle.go` — `Replace`, `Remove`, `unlinkPage`, `stripAttachmentLinks`, `indexLinkWithTarget`, `deleteRow`.
- `internal/pages/commitjob.go` — relaxed guard so a removes-only commit (orphan delete) is valid.
- `internal/server/handlers_attachments.go` — `handleReplaceAttachment` + `handleDeleteAttachment`.
- `web/src/components/attachments/{Replace,Remove}AttachmentDialog.tsx` — DeleteConfirmDialog-shaped confirm dialogs.

## Decisions Made
See frontmatter `key-decisions`. One Rule-1 fix (removes-only commit guard) was required; no architectural (Rule 4) changes.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Allowed removes-only commit payloads on the shared pages CommitHandler**
- **Found during:** Task 2 (TestRemoveAttachment orphan-delete branch)
- **Issue:** The orphan delete is a pure deletion — a commitPayload with `Removes: [bin, meta, txt]` and ZERO `Writes`. The shared `pages.CommitHandler` had a `len(p.Writes) == 0 → error "commit requires at least one write"` guard, so the delete job returned an error, retried, and `EnqueueAndWait` timed out (5s). The files were never deleted and the post-delete download still returned 200.
- **Fix:** Relaxed the guard to `len(Writes)==0 && len(Removes)==0 → error` (a commit must touch something, but a removes-only payload is valid). All existing pages tests (writes-only saves, rename/move with Removes) still pass.
- **Files modified:** internal/pages/commitjob.go
- **Committed in:** `9261c74` (Task 2)

**Total deviations:** 1 auto-fixed (Rule 1). No scope creep; no architectural changes.

## Known Stubs
None. Replace/Remove are wired end-to-end (service → handler → dialog → live `["attachments", pagePath]` react-query invalidation). No hardcoded/placeholder content.

## Threat Surface
No new surface beyond the plan's threat_model. T-02-14: both routes editor-gated from the session (reader → 403, tested). T-02-15: orphan ref-count scans ALL pages for the single canonical link; delete only when zero, in ONE commit (Pitfall 6, tested keep-shared + delete-orphan). T-02-16: MaxBytesReader on the replace body before parse + re-sniff vs allow-list. T-02-17: id validated non-empty/NUL/slash-free; all paths via repo.Resolve; attachment paths built only from the server id.

## Verification
- `CGO_ENABLED=0 go test ./internal/attachments/ -run 'TestReplace|TestPageReferences|TestOrphanDelete|TestRemoveKeepsSharedFile' -count=1` → ok.
- `go test ./internal/server/ -run 'TestReplaceAttachment|TestRemoveAttachment|TestReplaceRemoveEditorOnly' -count=1` → ok.
- ATT-07 single-commit delete: `grep "Removes" internal/attachments/lifecycle.go` shows bin+meta+txt in ONE payload; `TestOrphanDelete` asserts `len(Removes)==3` in the last commit.
- Insert/scan share the canonical form: `grep "DownloadRefPath" internal/attachments/` shows it in `refs.go` (scan) and `lifecycle.go` (edit).
- `CGO_ENABLED=0 go test ./... -count=1` → all packages green; `go build ./...` clean.
- `cd web && npm run build && npx tsc --noEmit` → both pass.
- Confirm labels: `grep -q "Replace file"` / `grep -q "Remove file"` in the dialog files → LABELS-OK.
- Hidden-Git: vocab grep on the two dialogs → NO-GIT-VOCAB.
- `go mod tidy`: go.mod + go.sum unchanged (no new deps).

### Deferred (out of scope, SCOPE BOUNDARY)
- A pre-existing `react-hooks/static-components` ESLint finding on `AttachmentCard.tsx`'s `const Icon = typeIconFor(meta)` (from 02-02, unchanged here). The build (`tsc -b && vite build`) and the 02-04 verify command pass; ESLint is not part of the build. Logged in `deferred-items.md`.

## User Setup Required
None — no external service or configuration changes.

## Next Phase Readiness
Phase 2 is complete: the full attachment lifecycle (upload, preview, extract, replace, remove, orphan-delete) ships. The single-writer invariant, canonical-link contract, and editor RBAC gate hold across every operation.

## Self-Check: PASSED
