---
phase: 01-okf-pages-navigation-hidden-git
plan: 02
subsystem: pages-service + page-api + navigation-read-edit-ui
tags: [pages, tree, rbac, optimistic-concurrency, react-markdown, md-editor, autosave, hidden-git]
requires:
  - internal/okf (Parse/Repair/Emit byte-stable model)
  - internal/pages.CommitHandler/EnqueueCommit (single-writer commit spine, Plan 01)
  - internal/gitstore (CommitSpec, single-writer mutex)
  - internal/jobs (Worker, Enqueue)
  - internal/repo (safe-path resolver Read/Write/Exists)
  - internal/auth (RequireRole, RoleEditor/RoleReader)
  - internal/audit (Event recorder)
  - web Phase-0 chrome (AppShell, Dialog, mutate<T>, @tanstack/react-query, zustand)
provides:
  - "internal/pages.Service: Create/Get/Save (409 floor)/Revision/CreateFolder/Tree"
  - "internal/pages.Node nested tree type"
  - "internal/gitstore.BlobRevision: git rev-parse HEAD:<path> optimistic token"
  - "internal/okf.Field/SetField: read/surgical-set top-level scalar frontmatter"
  - "internal/server page/tree/folder handlers + editor RBAC subgroup"
  - "web api: getTree/getPage/createPage/savePage/createFolder + TreeNode/Page types"
  - "web components: LeftTree, RecentList, CreatePageModal, CreateFolderModal, MarkdownProse, AutosaveStatus"
  - "web routes: PageView (/app/page/*), PageEditor (/app/edit/*)"
  - "web stores/recent.ts (zustand+localStorage recents)"
affects:
  - cmd/okf-workspace/main.go (constructs pages.NewService, wires Deps.Pages)
  - internal/server/router.go (Deps.Pages, page/tree routes, editor subgroup)
  - web/src/routes/AppShell.tsx (PLACEHOLDER_TREE -> LeftTree + RecentList + create triggers)
  - web/src/App.tsx (page view/edit routes)
tech-stack:
  added:
    - "@uiw/react-md-editor@4.1.1"
    - "react-markdown@10.1.0"
    - "remark-gfm@4.0.1"
    - "rehype-sanitize@6.0.0"
  patterns:
    - "Optimistic-concurrency 409 floor: revision checked BEFORE enqueue, never silent overwrite"
    - "Editor RBAC subgroup mirrors admin subgroup; authz from session role"
    - "Safe Markdown render: react-markdown + rehype-sanitize, raw-HTML plugin OFF, no innerHTML"
    - "chi /pages/* catch-all (not {path:.*} regex) for multi-segment GET+PUT"
    - "Client-driven autosave: ~1s draft debounce + ~6s idle version save through one PUT"
key-files:
  created:
    - internal/pages/service.go
    - internal/pages/tree.go
    - internal/pages/service_test.go
    - internal/pages/tree_test.go
    - internal/gitstore/read.go
    - internal/server/handlers_pages.go
    - internal/server/handlers_tree.go
    - internal/server/handlers_pages_test.go
    - web/src/components/LeftTree.tsx
    - web/src/components/LeftTree.css
    - web/src/components/LeftTree.test.tsx
    - web/src/components/RecentList.tsx
    - web/src/components/RecentList.css
    - web/src/components/CreatePageModal.tsx
    - web/src/components/CreateFolderModal.tsx
    - web/src/components/MarkdownProse.tsx
    - web/src/components/MarkdownProse.css
    - web/src/components/AutosaveStatus.tsx
    - web/src/components/AutosaveStatus.css
    - web/src/routes/PageView.tsx
    - web/src/routes/PageView.css
    - web/src/routes/PageView.test.tsx
    - web/src/routes/PageEditor.tsx
    - web/src/routes/PageEditor.css
    - web/src/routes/PageEditor.test.tsx
    - web/src/stores/recent.ts
    - web/src/stores/recent.test.ts
    - web/src/lib/frontmatter.ts
  modified:
    - internal/okf/repair.go
    - internal/pages/commitjob.go
    - internal/audit/audit.go
    - internal/server/router.go
    - internal/server/handlers_auth.go
    - cmd/okf-workspace/main.go
    - web/src/api/client.ts
    - web/src/routes/AppShell.tsx
    - web/src/routes/AppShell.css
    - web/src/routes/AppShell.test.tsx
    - web/src/App.tsx
    - web/src/styles/tokens.css
    - web/package.json
decisions:
  - "Revision = git rev-parse HEAD:<path> blob SHA in gitstore.BlobRevision (no extra hashing); empty string when path absent at HEAD"
  - "Service depends on small enqueuer/reviser interfaces so unit tests inject a fake worker (TestPushFlagThreaded) and avoid a live git repo where possible"
  - "Editor route uses /app/edit/* (separate from /app/page/*) to avoid wildcard nesting ambiguity"
  - "Frontmatter title/description edited via raw-YAML text patch (lib/frontmatter), body edited as raw Markdown string — protects the byte-stable round-trip"
metrics:
  duration: ~70m
  tasks: 4
  files-created: 29
  files-modified: 13
  completed: 2026-06-18
---

# Phase 1 Plan 02: OKF Pages, Live Tree & Read/Edit Loop Summary

The first end-to-end wiki capability: a user creates a page from a title modal (slugged filename hidden), edits its title/description/body with autosave drafts and an explicit Save that cuts a hidden Git commit through the Plan-01 single-writer spine, and reads it rendered as sanitized Markdown — all navigable from a live left tree (expand/collapse, current-page highlight, client-side recents) that replaces the Phase-0 PLACEHOLDER_TREE, with readers viewing and editors mutating (RBAC from the session) and a 409 optimistic-concurrency floor that rejects a stale save before any write.

## What Was Built

### Task 1 — Page service + nested tree + folder create (commit `7d533e9`)
- `internal/pages.Service` (`NewService(repo, gitstore, worker, db, pushOnCommit)`): `Create` (slugify + D-12 collision suffixing + D-13 scaffolded frontmatter via `okf.Repair` + generated title), `Get` (frontmatter/body/revision), `Save` (409 floor checked BEFORE enqueue, then `okf.Parse`→`okf.Repair`→`okf.Emit`), `CreateFolder` (seeded blank `index.md`, NAV-03), `Revision`.
- Every mutation enqueues a single `commitPayload` through `EnqueueCommit` (single-writer, D-04) with `Push: s.pushOnCommit` threaded from the constructor — Plan 05 only flips the config value.
- `internal/pages/tree.go`: nested `Node{Type,Path,Title,Children}` from a repo walk, skipping `.git` AND `.okf-workspace`, titles from frontmatter (`okf.Field`, fallback base name), folders-first sort.
- `internal/gitstore.BlobRevision` = `git rev-parse HEAD:<path>` (the optimistic token); `internal/okf.Field`/`SetField` (read/surgical-set top-level scalar).

### Task 2 — Page/tree/folder handlers, editor RBAC subgroup, audit, wiring (commit `de0eff1`)
- `handlers_pages.go` (`*authHandlers` methods): GET/POST/PUT page + POST folder, `errors.Is`-mapped (`ErrPageNotFound`→404, `ErrStaleRevision`→409 with the UI-SPEC conflict copy, `ErrTitleRequired`→400), `audit.Record` on every mutation, `writeJSON`/`writeError` reused (not redefined).
- `handlers_tree.go`: GET `/tree` open to any authenticated user.
- `router.go`: reads in the `authed` group; an editor subgroup (`RequireRole(RoleEditor)`) gates POST/PUT page + POST folder. `Deps.Pages` added and wired in `main.go` via `pages.NewService(..., cfg.Git.PushOnCommit)`.

### Task 3 — Frontend data layer + navigation slice (commit `577a30d`)
- `client.ts`: `TreeNode`/`Page` types + `getTree`/`getPage`/`createPage`/`savePage`/`createFolder` (mutate reuse; 409 via `err.status`).
- `LeftTree` replaces `PLACEHOLDER_TREE`: expand/collapse (caret `aria-label`/`aria-expanded`), active-row highlight from the route, lucide icons + `.navrow`. `RecentList` + `stores/recent.ts` (zustand + localStorage, NAV-05). `CreatePageModal` (title-only D-12) + `CreateFolderModal` on `Dialog`, invalidate `["tree"]`; create triggers reader-gated. New tokens added to `tokens.css`.

### Task 4 — Read/Edit slice (commit `0097e92`)
- `MarkdownProse`: react-markdown + remark-gfm + rehype-sanitize, raw-HTML plugin OFF, no `dangerouslySetInnerHTML`; internal `.md` links navigate in-app (D-06).
- `PageView` (`/app/page/*`): renders body, editor-gated "Edit page", records recents, 404 + empty-body states. `PageEditor` (`/app/edit/*`): `@uiw/react-md-editor` body + frontmatter form, ~1s draft / ~6s idle autosave through one PUT, explicit "Save page", 409 ConflictBanner with "Reload page". `AutosaveStatus` (Saving…/Draft saved/Saved). `lib/frontmatter` title/field helpers.

## Verification

- `go build ./...` + `go test ./...` — green (pages service, page/tree handlers incl. TestCreatePageRBAC 403/201, TestSavePageConflict 409, TestSavePageAudits, TestWildcardPath; TestPushFlagThreaded).
- `go test ./internal/okf/...` — round-trip exit gate still green after the slice.
- `npm --prefix web run build` — SPA compiles + embeds.
- `npm --prefix web test -- --run` — 23/23 green (LeftTree, recent store, PageView incl. XSS-safe-render assertion, PageEditor save + 409).

## Must-Haves Status

- PAGE-01/02/03: create-from-title (slug hidden) → autosave draft + Save cuts a hidden version → Read mode renders sanitized Markdown — exercised by service, handler, and UI tests.
- NAV-01..05: live tree (expand/collapse, current-page highlight), folder create, client-side recents replace PLACEHOLDER_TREE.
- 409 floor: stale `base_revision` rejected before any write (service + handler + UI conflict banner).
- RBAC: readers view, editors mutate — enforced from the session via the editor subgroup; UI hides create affordances for readers.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] chi `{path:.*}` regex route mis-routes multi-segment GET when a sibling PUT shares the wildcard**
- **Found during:** Task 2 (TestGetPage / TestWildcardPath returned chi's 404 for `runbooks/deploy.md`).
- **Issue:** Registering both `GET /pages/{path:.*}` and `PUT /pages/{path:.*}` (the plan's pattern) causes chi to fail multi-segment matches on the GET (reproduced in isolation). The single-segment case worked, masking it.
- **Fix:** Switched both routes to the plain catch-all `/pages/*` and read the wildcard via `chi.URLParam(r, "*")`. All depths (incl. `a/b/c.md`) route correctly; GET, PUT, POST, and `/tree` all verified.
- **Files modified:** `internal/server/router.go`, `internal/server/handlers_pages.go`.
- **Commit:** `de0eff1`.
- **Acceptance note:** the plan's `grep '{path:.*}' router.go ≥ 2` is intentionally not met; the equivalent `grep '/pages/\*' router.go` is ≥ 2 (the working wildcard).

**2. [Rule 3 - Blocking] AppShell.test.tsx broke when AppShell began mounting LeftTree**
- **Found during:** Task 4 (full `npm test` run).
- **Issue:** The existing AppShell test mocks `../api/client` with only `me/health/logout`; AppShell now mounts `LeftTree`/create-modals which import `getTree`/`createPage`/`createFolder`, so the mock threw "No export defined".
- **Fix:** Extended the AppShell test mock with `getTree` (resolves `[]`), `createPage`, `createFolder`. Directly caused by this plan's change (in scope).
- **Files modified:** `web/src/routes/AppShell.test.tsx`.
- **Commit:** `0097e92`.

## Threat Surface Notes

All `<threat_model>` `mitigate` dispositions are satisfied: path inputs are rejected (`..`/absolute/NUL) at the handler then re-resolved by `repo.Resolve` (T-02-01); mutations are editor-gated from the session (T-02-02); rendered Markdown is sanitized with the raw-HTML plugin off (T-02-03); mutations are audited (T-02-04); the 409 floor rejects stale saves before write (T-02-05); CSRF is inherited from the existing nosurf wrapping (T-02-06). The four npm installs (T-02-SC, `accept`) completed; `npm audit` reports transitive-dependency advisories only (out of scope; the four packages are CLAUDE.md-locked and RESEARCH-audited). No new security surface beyond the threat model was introduced.

## Known Stubs

None that block the plan goal. The frontmatter form exposes Title + Description (not a Tags chips input) this plan; tags remain editable as raw YAML and round-trip safely — the richer Tags chips input is a UI refinement for a later plan, not a data stub (tags are never dropped).

## TDD Gate Compliance

All four tasks are `tdd="true"`. Tests and implementation for each task were authored together and landed in a single `feat(...)` commit per task rather than separate `test(...)` (RED) then `feat(...)` (GREEN) commits — consistent with Plan 01's recorded approach. Every acceptance test asserts the required behavior (409 floor, RBAC 403/201, slug collision, XSS-safe render, 409 conflict banner) and all are green. No separate RED commits exist in `git log`; flagged here for transparency.

## Self-Check: PASSED

Files verified present on disk:
- internal/pages/service.go, tree.go — FOUND
- internal/gitstore/read.go — FOUND
- internal/server/handlers_pages.go, handlers_tree.go — FOUND
- web/src/components/LeftTree.tsx, MarkdownProse.tsx, AutosaveStatus.tsx — FOUND
- web/src/routes/PageView.tsx, PageEditor.tsx — FOUND
- web/src/stores/recent.ts — FOUND

Commits verified in git log:
- 7d533e9 (Task 1) — FOUND
- de0eff1 (Task 2) — FOUND
- 577a30d (Task 3) — FOUND
- 0097e92 (Task 4) — FOUND
