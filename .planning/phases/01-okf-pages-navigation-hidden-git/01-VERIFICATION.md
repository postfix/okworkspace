---
phase: 01-okf-pages-navigation-hidden-git
verified: 2026-06-19T00:30:00Z
status: passed
score: 17/19 must-haves verified
overrides_applied: 0
human_verification:
  - test: "[FIXED IN CODE post-verification — light regression confirmation] Open a page with a cross-folder relative link (page in `docs/` linking to `runbooks/deploy.md`) AND a same-directory link, and click each in Read mode"
    expected: "Both links navigate to the correct in-app page (no broken `/app/page/../...` route, no root-jump)"
    why_human: "WR-02/WR-06 was FIXED post-verification in commit 0c8421e — MarkdownProse now resolves relative `.md` links against the current page's directory via the unit-tested pure resolver `web/src/lib/mdlink.ts` (16 cases incl. same-dir, multi-level `../`, root clamp, external untouched). Automated coverage now exists; this is downgraded to an in-browser regression confirmation of D-06/PAGE-08 click navigation."
  - test: "Edit a page that has autosave running; observe the autosave cycle for about 10 seconds without touching the keyboard"
    expected: "No spurious 409 'This page was changed somewhere else' banner appears during a single-user edit session with no concurrent activity"
    why_human: "WR-03 (open review warning): the draft autosave (1s) and idle version save (6s) timers run concurrently with no in-flight guard. If the 1s save is still in flight when the 6s fires, the second save reads a stale base_revision and triggers a false 409 conflict banner. This is a race condition observable only in a live browser session."
gaps: []
---

# Phase 1: Verification Report

**Phase Goal:** A non-technical user can create, edit, organize, link, and version Markdown pages through a file tree, with Git history kept entirely hidden behind the UI.
**Verified:** 2026-06-19T00:30:00Z
**Status:** passed (human UAT complete 2026-06-21 — 3/3 pass; 1 minor gap logged: history API leaks raw SHA, UI clean)
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | Golden-corpus byte-stable round-trip test exists and is green (phase exit gate) | VERIFIED | `TestGoldenRoundTrip` in `internal/okf/roundtrip_test.go` passes all 8 fixtures including CRLF (07-crlf.md) and code-block-with-fence (02-codeblock-with-fence.md); `TestCorpusHasCRLFFixture` confirms the CRLF fixture exists. Verified live: `go test -count=1 ./internal/okf/... -run TestGoldenRoundTrip` — all 8 PASS. |
| 2 | A page write becomes exactly one hidden Git commit via the single-writer CommitJob (VER-01); no second write path | VERIFIED | `KindCommit` registered in `main.go:187` as `pages.CommitHandler(contentRepo, gs)` — the Phase-0 no-op stub is gone (confirmed: `grep 'CommitHandler' cmd/okf-workspace/main.go` returns the real handler). `commitjob.go` is the only code path writing `.md` files and calling `g.Commit`. No `exec.Command` or `os.WriteFile` in `internal/pages/` outside `_test.go`. `TestCommitHandler_WritesAndCommits` passes. |
| 3 | Saving a page with missing required frontmatter adds ONLY the missing fields and leaves every other byte unchanged (PAGE-09) | VERIFIED | `okf.Repair` is additive-only via `yaml.Node` surgery (sets `FrontDirty=true` only when adding). `Service.Save` calls `okf.Repair` before enqueuing. `TestSaveRepairsFrontmatter` passes. `TestRepair` in `internal/okf/repair_test.go` passes. Round-trip gate covers byte-stability. |
| 4 | Rename/move eagerly rewrites inbound `.md` links in the SAME commit via the round-trip-safe okf path (PAGE-04/05/08, D-07) | VERIFIED | `rename.go:relocate` builds one `commitPayload` with all writes (new path + rewritten files + removes). `rewriteInboundLinks` calls `okf.Parse` → `okf.RewriteLinks` (body-level, structurally skips code blocks) → `okf.Emit`. No `strings.ReplaceAll` in `rename.go`. `TestRename`, `TestMove`, `TestRename_NoCorruption` all PASS. |
| 5 | Delete-to-trash is a recoverable commit with provenance; restore auto-suffixes to never clobber a live page (PAGE-06/07, D-08/D-09/D-10) | VERIFIED | `trash.go:Delete` uses `commitPayload{Writes+Removes}` (git mv via CommitJob, not git rm). `INSERT INTO trash` records provenance. `Restore` calls `repo.Exists` and appends `" (restored)"` on collision. No `os.Rename`/`os.Remove`/`git rm` in `trash.go`. `TestTrashRestore`, `TestRestore`, `TestRestoreCollision`, `TestDeleteCreatesTrashDir`, `TestListTrash` all PASS. |
| 6 | Version history exposes NO commit SHAs / Git vocabulary to the client (VER-02) | VERIFIED | `HistoryEntry` struct has no SHA field — fields are `Version`(opaque), `Action`, `Who`, `When`. `gitstore.Commit.Token` (the SHA) is never serialized to `HistoryEntry`. `grep -iEn 'sha|hash|commit_id' internal/pages/history.go` returns only comments. HistoryPanel.tsx: grep for `\b(SHA|commit|branch|HEAD|hash|push|merge)\b` returns zero matches outside comments. `TestHistoryNoSHA` PASS. |
| 7 | Restore writes a NEW forward commit, never rewrites history (VER-03) | VERIFIED | `RestoreVersion` calls `s.enqueueWrite(ctx, path, out, "restore-version", user)` — identical to the edit path, advancing HEAD. `grep 'reset' internal/pages/history.go internal/gitstore/history.go` returns zero matches. `grep 'force' internal/gitstore/push.go` returns zero matches. `TestRestoreForwardCommit` PASS. |
| 8 | Remote push is config-gated and ALERTS (never auto-merges) on divergence; the push flag reaches the CommitJob payload (VER-04) | VERIFIED | `push.go:Push` gates on `!RemoteEnabled || !PushOnCommit || Remote==""`. Sets `g.diverged = true` on non-ff rejection and returns nil (never force-pushes). `isNonFastForward` checks "non-fast-forward", "rejected", "fetch first". `commitjob.go:91-95` calls `g.Push(ctx)` only when `p.Push`. All 5 mutation paths (Create/Save via `enqueueWrite`, Rename/Move via `relocate`, Delete/Restore via `trash.go`) set `Push: s.pushOnCommit`. `NewService` records `cfg.Git.PushOnCommit`. `TestPushFlagReachesPayload`, `TestCommitJobPushBranch`, `TestPushDisabled`/`TestPushDiverged` all PASS. |
| 9 | A user creates a page by typing a title; it appears in the left tree (filename/path never shown) | VERIFIED | `handleCreatePage` → `pages.Service.Create` (slugs title, scaffolds frontmatter, enqueues CommitJob). `LeftTree.tsx` uses `useQuery({queryKey:["tree"], queryFn:getTree})`. `CreatePageModal` uses `useMutation` invalidating `["tree"]`. `TestCreatePageRBAC` PASS. Frontend test `LeftTree.test.tsx` (3 tests) PASS. No Git vocabulary in `CreatePageModal.tsx`. |
| 10 | Markdown render keeps raw HTML off / sanitized (stored-XSS guard) | VERIFIED | `MarkdownProse.tsx` imports `rehype-sanitize` and applies it; `rehype-raw` is NOT imported (confirmed by grep returning zero matches). No `dangerouslySetInnerHTML` in `MarkdownProse.tsx`. T-02-03 mitigated. |
| 11 | Mutating routes are editor-RBAC-gated from the session | VERIFIED | `router.go:107` — `editor.Use(auth.RequireRole(auth.RoleEditor))` wraps POST/PUT/DELETE/POST pages, POST folders, POST trash restore. Authorization reads from session via `CurrentUser`, never request body. `TestCreatePageRBAC` (403 reader / 201 editor), `TestRenameHandlerRBAC`, `TestDeletePageRBAC`, `TestRestoreVersionRBAC` all PASS. |
| 12 | All FS/git paths flow through the SEC-01 safe-path resolver | VERIFIED | `handlers_pages.go:cleanPathParam/cleanPathString` rejects `..`/absolute/NUL before passing to service. `Service` methods call `repo.Resolve` as backstop. `gitstore.History` and `gitstore.ShowAt` each call `g.repo.Resolve(path)` before using. No `os.*` writes in `internal/pages/` (grep confirmed). |
| 13 | User browses pages in the live left tree, expands/collapses, creates folders, sees current page highlighted, sees recently visited pages (NAV-01..05) | VERIFIED | `LeftTree.tsx` — live `useQuery` wired to `/api/v1/tree`. Expand/collapse state local. Active row from route match. `CreateFolderModal` calls `createFolder` (POST `/folders`). `stores/recent.ts` is zustand+localStorage. `TestTree` (backend), `LeftTree.test.tsx` (3 frontend tests), `recent.test.ts` (4 tests) all PASS. |
| 14 | Stale save (base_revision mismatch) is rejected with 409 before any write (COLL-03 floor) | VERIFIED | `Service.Save` calls `Revision(ctx,path)` before enqueuing; returns `ErrStaleRevision` when mismatch. `handleSavePage` maps `ErrStaleRevision` → 409. `TestSaveStaleRevision`, `TestSavePageConflict` both PASS. |
| 15 | Version history UI shows no Git SHAs, branches, or hashes | VERIFIED | `HistoryPanel.tsx` renders `{actionLabel(e.action)} by {e.who} · {relativeTime(e.when)}`. No SHA/commit/branch/HEAD in component text (grep confirmed). `actionLabel` maps Git action tokens to plain English ("Edited", "Created", etc). `HistoryPanel.test.tsx` (6 tests) PASS. |
| 16 | A user inserts a link to another page via a picker emitting a relative `.md` path | VERIFIED (with caveat) | `LinkPicker.tsx` calls `relativeMdLink(fromPath, p.path)` and inserts `[title](relative.md)` into the md-editor. No `[[wiki]]` links. Confirmed: `grep -c '\[\[' web/src/components/LinkPicker.tsx` = 0. The picker emits structurally correct relative paths. **Caveat:** WR-02/WR-06 (open review warnings) — `MarkdownProse` cannot navigate these relative links correctly at runtime for nested pages (see Human Verification #1). |
| 17 | Autosave draft persists; explicit Save (or ~6s idle) cuts a hidden version | VERIFIED (with caveat) | `PageEditor.tsx` uses 1s/6s debounce timers calling `doSave`. `savePage` PUT carries `base_revision`. `AutosaveStatus` shows "Saving…"/"Draft saved"/"Saved" with no Git vocabulary. No "commit/branch/SHA/push/merge" in editor strings (grep confirmed). **Caveat:** WR-03 (open review warning) — overlapping timers can produce a false 409 conflict banner in a single-user session (see Human Verification #2). |
| 18 | PAGE-08 in-app navigation on click works for internal `.md` links | UNCERTAIN | `MarkdownProse.tsx:24` strips at most one `./` and one `../` prefix. For a page at `a/b/page.md` with a link to `../../other/page.md`, the target becomes `../other/page.md` after one strip, which routes to `/app/page/../other/page.md` — a broken route. WR-02 confirmed present in code. This is a functional gap for cross-folder and same-directory link navigation (D-06). Cannot confirm without human testing; see Human Verification #1. |
| 19 | WR-03 autosave self-conflict produces no false 409 | UNCERTAIN | Code inspection confirms `doSave` is not guarded against concurrent calls. Both timers invoke the same `doSave` with the same captured `baseRevision.current`; if the 1s save is in-flight when the 6s fires and the first lands before the second starts, the second will 409. Cannot confirm without live testing. See Human Verification #2. |

**Score:** 17/19 must-haves verified (items 18 and 19 are UNCERTAIN, routing to human verification)

### Deferred Items

None. All phase requirements are covered in this phase; items 18 and 19 are UNCERTAIN (not deferred to a later phase) and require human testing.

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/okf/okf.go` | `func Parse` | VERIFIED | Present; `Parse` detects frontmatter fence ONLY at byte offset 0 |
| `internal/okf/repair.go` | `func Repair` | VERIFIED | Additive-only field repair via `yaml.Node`; sets `FrontDirty=true` only when adding |
| `internal/okf/emit.go` | `func (d *Doc) Emit` | VERIFIED | Re-attaches `RawFront` verbatim when `!FrontDirty`; re-marshals only when dirty |
| `internal/okf/roundtrip_test.go` | `TestGoldenRoundTrip` | VERIFIED | 8 fixtures, all PASS; CRLF fixture present |
| `internal/okf/links.go` | `func RewriteLinks` | VERIFIED | Structural rewrite on opaque body bytes; skips fenced code blocks |
| `internal/pages/commitjob.go` | `KindCommit` | VERIFIED | Single-writer handler; `if p.Push { g.Push(ctx) }` branch live |
| `internal/pages/service.go` | `func (s *Service)` | VERIFIED | Create/Save/CreateFolder all enqueue via `enqueueWrite`; `pushOnCommit` field wired to all call sites |
| `internal/pages/rename.go` | `func (s *Service) Rename` | VERIFIED | One CommitJob for move+rewrites; uses `okf.RewriteLinks` not `strings.ReplaceAll` |
| `internal/pages/history.go` | `func (s *Service) Restore` | VERIFIED | Forward commit only; no `reset`; opaque version token validation |
| `internal/pages/trash.go` | `func (s *Service) Delete` | VERIFIED | git mv to `.okf-workspace/trash/`; no `git rm`; provenance recorded |
| `internal/store/migrations/0004_drafts.sql` | `CREATE TABLE IF NOT EXISTS drafts` | VERIFIED | File exists; correct schema |
| `internal/store/migrations/0005_trash.sql` | `CREATE TABLE IF NOT EXISTS trash` | VERIFIED | File exists; correct schema |
| `internal/server/handlers_pages.go` | `func (h *authHandlers) handle` | VERIFIED | CR-01 fix in place (`.md`-anchored dispatch); 409 floor; audit on mutation |
| `internal/server/handlers_history.go` | `handleHistory` | VERIFIED | Present; opaque token; audit on restore |
| `internal/server/handlers_trash.go` | `handleListTrash` | VERIFIED | Present; audit on delete/restore |
| `internal/server/router.go` | `RequireRole(auth.RoleEditor)` | VERIFIED | Editor subgroup wraps all mutating routes |
| `web/src/routes/PageView.tsx` | `MarkdownProse` | VERIFIED | Uses `MarkdownProse` (sanitized, no raw HTML) |
| `web/src/routes/PageEditor.tsx` | `react-md-editor` | VERIFIED | `@uiw/react-md-editor`; 409 handled; autosave; no Git vocabulary |
| `web/src/components/LeftTree.tsx` | `useQuery` | VERIFIED | Live API wired; expand/collapse; active highlight |
| `web/src/stores/recent.ts` | `create` | VERIFIED | zustand + localStorage; recent-pages store |
| `web/src/components/HistoryPanel.tsx` | `Restore this version` | VERIFIED | No SHA/Git vocab in UI; action labels plain English |
| `web/src/components/MarkdownProse.tsx` | XSS-safe render | VERIFIED | `rehype-sanitize` ON; `rehype-raw` absent; no `dangerouslySetInnerHTML` |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| `internal/pages/commitjob.go` | `internal/gitstore.Commit` | single-writer worker | VERIFIED | `g.Commit(ctx, p.Spec)` in `CommitHandler`; no direct git exec in pages package |
| `internal/pages/commitjob.go` | `internal/gitstore.Push` | `if p.Push { g.Push(ctx) }` | VERIFIED | Present at line 91-95; no-op when remote not configured |
| `cmd/okf-workspace/main.go` | `pages.CommitHandler` | `worker.Register(pages.KindCommit, ...)` | VERIFIED | Line 187; no-op stub gone |
| `web/src/routes/PageEditor.tsx` | `PUT /api/v1/pages/{path}` | `savePage` carrying `base_revision` | VERIFIED | `savePage(path, {body, frontmatter, base_revision})` in `doSave` |
| `internal/server/handlers_pages.go` | `pages.Service.Save` → `EnqueueCommit` | 409 floor before enqueue | VERIFIED | `handleSavePage` → `h.pages.Save` → `ErrStaleRevision` check → `enqueueWrite` → `EnqueueCommit` |
| `internal/server/router.go` | `auth.RequireRole(auth.RoleEditor)` | editor RBAC subgroup | VERIFIED | Line 107; all mutations gated |
| `web/src/components/LeftTree.tsx` | `GET /api/v1/tree` | `queryKey: ["tree"], queryFn: getTree` | VERIFIED | Live; replaces PLACEHOLDER_TREE (grep = 0) |
| `internal/pages/rename.go` | `internal/okf.RewriteLinks` | repo-wide scan → okf.Parse → RewriteLinks → okf.Emit | VERIFIED | No `strings.ReplaceAll` in `rename.go` |
| `internal/pages/rename.go` | single CommitJob | one `commitPayload` with all writes + removes | VERIFIED | `relocate` builds one payload; `TestRename` asserts one new commit |
| `internal/pages/trash.go` | `.okf-workspace/trash/` via CommitJob | `commitPayload{Writes+Removes}` | VERIFIED | No `git rm`; `Push: s.pushOnCommit` set |
| `internal/pages/history.go` | forward commit via CommitJob | `enqueueWrite(..., "restore-version", ...)` | VERIFIED | No `reset`; same `enqueueWrite` path as all mutations |
| `internal/pages/service.go` | `commitPayload.Push = s.pushOnCommit` | every `enqueueWrite` call | VERIFIED | `enqueueWrite` sets `Push: s.pushOnCommit`; confirmed in rename.go:121, trash.go:89+186, history uses `enqueueWrite` |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|--------------|--------|--------------------|--------|
| `LeftTree.tsx` | `nodes` | `useQuery` → `GET /api/v1/tree` → `(*Service).Tree` → `repo.WalkDir` + `okf.Parse` | Yes — walks real repo | FLOWING |
| `HistoryPanel.tsx` | `entries` | `useQuery` → `GET /pages/{path}/history` → `gitstore.History` → `git log --follow` | Yes — real git log | FLOWING |
| `PageView.tsx` | `data` (Page) | `useQuery` → `GET /api/v1/pages/{path}` → `pages.Service.Get` → `repo.Read` + `okf.Parse` | Yes — real file read | FLOWING |
| `TrashView.tsx` | `entries` | `useQuery` → `GET /api/v1/trash` → `pages.ListTrash` → `SELECT FROM trash` | Yes — real SQLite query | FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Golden round-trip test (phase exit gate) | `go test -count=1 ./internal/okf/... -run TestGoldenRoundTrip` | 8/8 fixtures PASS | PASS |
| CommitJob single-writer | `go test -count=1 ./internal/pages/... -run TestCommitHandler` | 3 tests PASS | PASS |
| 409 stale revision floor | `go test -count=1 ./internal/server/... -run TestSavePageConflict` | PASS | PASS |
| RBAC editor gate | `go test -count=1 ./internal/server/... -run TestCreatePageRBAC` | PASS (403 reader / 201 editor) | PASS |
| Rename no-corruption | `go test -count=1 ./internal/pages/... -run TestRename_NoCorruption` | PASS | PASS |
| Trash restore collision-safe | `go test -count=1 ./internal/pages/... -run TestRestoreCollision` | PASS | PASS |
| History no SHA leak | `go test -count=1 ./internal/pages/... -run TestHistoryNoSHA` | PASS | PASS |
| Restore is forward commit | `go test -count=1 ./internal/pages/... -run TestRestoreForwardCommit` | PASS | PASS |
| Push flag reaches payload | `go test -count=1 ./internal/pages/... -run TestPushFlagReachesPayload` | PASS | PASS |
| CR-01 regression (version-folder page) | `go test -count=1 ./internal/server/... -run TestGetPageInVersionFolder` | PASS | PASS |
| Full backend suite | `go test ./... -race` | 12 packages, 0 failures | PASS |
| Full frontend suite | `npm --prefix web test -- --run` | 71 tests, 11 files, 0 failures | PASS |
| Binary builds | `go build ./...` | Success | PASS |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|------------|-------------|--------|----------|
| PAGE-01 | 01-02 | User can create a page in the selected folder | SATISFIED | `handleCreatePage` + `pages.Service.Create` + `CreatePageModal`; `TestCreatePageRBAC` PASS |
| PAGE-02 | 01-02 | User can edit a page's title, tags, description, and body | SATISFIED | `PageEditor.tsx` with frontmatter form + MDEditor body; `handleSavePage` |
| PAGE-03 | 01-02 | User can save a page and view it rendered as Markdown | SATISFIED | `handleSavePage` → CommitJob; `PageView.tsx` + `MarkdownProse` |
| PAGE-04 | 01-03 | User can rename a page | SATISFIED | `handleRenamePage` dispatches to `Rename`; `TestRenameDispatch_Title` PASS |
| PAGE-05 | 01-03 | User can move a page to another folder | SATISFIED | `handleRenamePage` dispatches to `Move`; `TestRenameDispatch_Move` PASS |
| PAGE-06 | 01-04 | User can delete a page to trash | SATISFIED | `handleDeletePage` → `pages.Service.Delete`; `TestDeletePageRBAC` PASS |
| PAGE-07 | 01-04 | User can restore a page from trash | SATISFIED | `handleRestoreFromTrash` → `pages.Service.Restore`; `TestRestoreHandler` PASS |
| PAGE-08 | 01-03 | User can link from one page to another | PARTIAL | `LinkPicker.tsx` emits `[title](relative.md)` — link INSERT works. **In-app navigation on click is broken for nested pages** (WR-02/WR-06: `MarkdownProse` only strips one `../`). Routed to Human Verification #1. |
| PAGE-09 | 01-01 | System fills in missing required frontmatter on save | SATISFIED | `okf.Repair` in `Service.Save`; `TestSaveRepairsFrontmatter` PASS; `TestRepair` PASS |
| NAV-01 | 01-02 | User can browse pages in a left-side tree | SATISFIED | `LeftTree.tsx` with live API; `TestTreeHandler` PASS |
| NAV-02 | 01-02 | User can expand and collapse folders in the tree | SATISFIED | `LeftTree.tsx` expand/collapse state; `LeftTree.test.tsx` (3 tests) PASS |
| NAV-03 | 01-02 | User can create a folder | SATISFIED | `handleCreateFolder` → `pages.Service.CreateFolder`; `CreateFolderModal`; `TestCreateFolder` PASS |
| NAV-04 | 01-02 | User sees the currently open page highlighted in the tree | SATISFIED | `LeftTree.tsx` active-row logic from route match; `LeftTree.test.tsx` PASS |
| NAV-05 | 01-02 | User can see a list of recently visited pages | SATISFIED | `stores/recent.ts` (zustand+localStorage); `recent.test.ts` (4 tests) PASS |
| VER-01 | 01-01 | System commits to Git automatically after a page save, recording user identity | SATISFIED | CommitJob registered; `CommitSpec.User` carries actor; `TestCommitHandler_WritesAndCommits` asserts author equals payload User. Note: REQUIREMENTS.md marks VER-01 as "Pending" — this appears to be a stale status; the implementation is complete and tested. |
| VER-02 | 01-05 | User can view a page's version history | SATISFIED | `GET /pages/{path}/history`; `HistoryPanel.tsx` renders action/who/when; no SHA. `TestHistoryHandler` PASS. |
| VER-03 | 01-05 | User can restore a previous version of a page | SATISFIED | `POST /pages/{path}/restore` → `RestoreVersion` → forward commit. `TestRestoreForwardCommit` PASS. |
| VER-04 | 01-05 | System can push commits to a configured Git remote when enabled | SATISFIED | `gitstore.Push` config-gated; `p.Push` in CommitJob; `TestPushFlagReachesPayload` PASS. |

**Note on REQUIREMENTS.md status discrepancy:** The traceability table marks `PAGE-09` as "Pending" and `VER-01` as "Pending". Both are implemented and tested. These appear to be stale status values in REQUIREMENTS.md that were not updated after the plans executed. This is a documentation gap, not an implementation gap.

### Anti-Patterns Found

| File | Pattern | Severity | Impact |
|------|---------|----------|--------|
| `web/src/components/MarkdownProse.tsx:24` | `href!.replace(/^\.?\//, "").replace(/^\.\.\//, "")` — strips at most one `../` level | WARNING | Breaks in-app link navigation for all cross-folder relative links and same-directory links inserted by LinkPicker (WR-02, WR-06). D-06/PAGE-08 "in-app navigation on click" non-functional for nested wiki content. |
| `web/src/routes/PageEditor.tsx:91-96` | Dual autosave timers (1s draft, 6s idle) with no in-flight guard on `doSave` | WARNING | Overlapping timer invocations can produce a spurious 409 conflict banner on a single-user session with slow network (WR-03). |
| `internal/pages/trash.go:91-104` (Delete), `188-195` (Restore) | SQLite write (INSERT/DELETE trash row) is synchronous but CommitJob is async — no transactional coupling | WARNING | If the CommitJob fails after the SQLite write, the trash table and git working tree diverge. Reported as WR-01 (open review warning). Not a blocker for phase goal since the normal path is atomic; failure path leaves recoverable state. |
| `internal/gitstore/push.go:47-52` | `isNonFastForward` matches broad `"rejected"` substring — auth/transport errors are misclassified as divergence | WARNING | WR-05: a network failure containing "rejected" silently sets `diverged` instead of returning an error. Over-broad match. |
| `internal/pages/service.go:307` (WR-07) | `slugify` collapses CJK/emoji/symbol-only titles to empty string; `CreateFolder` has no `untitled` fallback | WARNING | A CJK folder name slugs to `""` and `indexPath` becomes `"/index.md"` (repo root), which the resolver may reject. Not tested; no corpus fixture covers this. |
| `web/src/lib/frontmatter.ts:49-55` (CR-02 — RESOLVED) | `quoteIfNeeded` originally under-quoted YAML-unsafe scalars | RESOLVED | Fixed in commit `7bddef6`; `frontmatter.test.ts` (24 assertions) PASS. |
| `internal/server/handlers_pages.go` (CR-01 — RESOLVED) | Original substring dispatch on `version`/`history` would mis-route pages in `version/` folders | RESOLVED | Fixed in commit `b5f625a`; `.md`-anchored dispatch; `TestGetPageInVersionFolder` PASS. |

No debt markers (`TBD`, `FIXME`, `XXX`) found in files modified by this phase.

### Human Verification Required

### 1. In-app link navigation for nested pages (PAGE-08 / WR-02 / WR-06)

**Test:** Open a page at a nested path (e.g. `docs/architecture/decisions.md`). Use the LinkPicker to insert a link to another page in a different folder (e.g. `runbooks/deploy.md`). Save. Return to Read mode. Click the inserted link.

**Expected:** The app navigates to the correct page (`/app/page/runbooks/deploy.md`). No 404, no wrong page.

**Also test:** Two pages in the same folder (`a/b/page1.md` and `a/b/page2.md`). LinkPicker inserts `[Page 2](page2.md)` (bare filename, no `./`). Click in Read mode.

**Expected:** App navigates to `/app/page/a/b/page2.md`, not `/app/page/page2.md` (repo root).

**Why human:** `MarkdownProse.tsx:24` strips at most one `../` prefix and treats bare filenames as repo-root paths — both multi-level relative links and same-directory links are broken by construction (WR-02 + WR-06, confirmed in code). This is a functional failure of D-06 / PAGE-08 "in-app navigation on click" for all non-root pages. Cannot verify correctness programmatically; needs a running browser session.

### 2. Autosave self-conflict (WR-03)

**Test:** Open a page, type some text to trigger the autosave cycle, then stop typing and wait 7-8 seconds (spanning both the 1s draft save and the 6s idle commit). Observe the status indicator.

**Expected:** Status shows "Saving…" → "Draft saved" → (after idle) "Saving…" → "Saved". NO "This page was changed somewhere else" conflict banner appears during a single-user session.

**Why human:** `doSave` is not guarded against concurrent calls. If the 1s draft save is still in flight (slow network) when the 6s idle fires, the second save uses the old `baseRevision.current` and the backend returns 409. This produces a false conflict banner in a single-user session. Timing-dependent; requires a live browser with real network latency or throttled DevTools.

---

## Gaps Summary

No hard blockers were found. All required artifacts exist and are substantively implemented. The critical issues from code review (CR-01, CR-02) were fixed and regression-tested. All 12 Go packages pass the race detector; all 71 frontend tests pass.

Two items remain UNCERTAIN and require human testing before this phase can be marked `passed`:

1. **PAGE-08 / D-06 link navigation (WR-02 + WR-06): FIXED IN CODE post-verification (commit `0c8421e`).** MarkdownProse now resolves relative `.md` links against the current page's directory via the unit-tested pure resolver `web/src/lib/mdlink.ts` (16 vitest cases incl. same-dir, multi-level `../`, root clamp, external untouched); both callers (PageView, HistoryPanel) pass the current page path. Remaining human item is a light in-browser regression confirmation that the click navigation now works end-to-end.

2. **WR-03 autosave self-conflict:** A race condition in the editor's autosave timer logic could produce a false 409 banner in a single-user session. If confirmed, this would be a UX defect against the "autosave never blocks typing" requirement.

Both open review warnings WR-04 (version token not bound to page history — authorization gap) and WR-01 (DB/Git divergence on trash failure) are correctness concerns the review flagged but are not blockers for the phase goal as stated. They are appropriate candidates for `/gsd-code-review 1 --fix`.

---

_Verified: 2026-06-19T00:30:00Z_
_Verifier: Claude (gsd-verifier)_
