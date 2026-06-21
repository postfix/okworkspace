# Roadmap: OKF Workspace

## Overview

OKF Workspace is built dependency-first: a safe foundation (single binary, auth, path resolver, Git repo, single-writer commit service) comes before any file operation; the core wiki loop (OKF pages, navigation, hidden Git history) is built on that foundation with a byte-stable Markdown round-trip as its exit gate; attachments and their text-extraction pipeline follow, gating both search and the agent; full-text search indexes pages plus extracted attachment text; an approval-gated Eino agent reads, summarizes, rewrites, and proposes diffs that humans approve before applying; and finally soft-lock collaboration hardens the save path with presence and conflict resolution. Each phase delivers an end-to-end user capability and reuses the cross-cutting spines (safe-path resolver, single-writer Git service, async job worker) established early.

## Phases

**Phase Numbering:**

- Integer phases (0, 1, 2, 3, 4, 5): Planned milestone work
- Decimal phases (2.1, 2.2): Urgent insertions (marked with INSERTED)

Decimal phases appear between their surrounding integers in numeric order.

- [x] **Phase 0: Skeleton, Auth & Foundations** - Single binary serves the React shell behind a login; RBAC, sessions, safe-path resolver, and Git repo init are in place
- [x] **Phase 1: OKF Pages, Navigation & Hidden Git** - Users create, edit, organize, and version pages with automatic hidden Git history and restore (completed 2026-06-18)
- [x] **Phase 2: Attachments & Text Extraction** - Users upload, preview, download, and manage original-fidelity attachments with searchable extracted text (completed 2026-06-21; 11/11 verified, code review resolved incl. stored-XSS + worker-stall fixes; 4 browser-only checks deferred)
- [x] **Phase 3: Search** - Users find pages, headings, and attachments across titles, body, tags, filenames, and extracted text (completed 2026-06-21; 13/13 verified, code review resolved incl. drift-detection + heading-deep-link fixes; 13 browser checks deferred)
- [ ] **Phase 4: Eino Agent** - Users get approval-gated AI help over pages, selections, attachments, and the workspace
- [ ] **Phase 5: Collaboration** - Users see presence, soft locks, and conflict resolution so concurrent edits never silently lose work
- [x] **Phase 6: Live-Preview Editor (Obsidian-style)** - Editors get an Obsidian-style live-preview Markdown editor (inline rendering as you type, source toggle) while keeping the byte-stable Markdown round-trip (completed 2026-06-21)
- [x] **Phase 7: Obsidian-style File Tree (folder operations & tree UX)** - Users manage folders and files directly in the tree (right-click menus, drag-and-drop, folder rename/move/delete) the way they would in Obsidian (completed 2026-06-21)

## Phase Details

### Phase 0: Skeleton, Auth & Foundations

**Goal**: A non-technical user can log into a running single-binary app, with all load-bearing security and storage foundations (safe-path resolver, RBAC, sessions, Git repo, single-writer commit spine) in place for later phases.
**Mode:** mvp
**Depends on**: Nothing (first phase)
**Requirements**: AUTH-01, AUTH-02, AUTH-03, AUTH-04, AUTH-05, AUTH-06, SEC-01, SEC-03, SEC-04, SEC-05
**Success Criteria** (what must be TRUE):

  1. A user can log in with a username and password, see their display name in the UI, and log out from any page
  2. A user's session persists across a browser refresh via an HTTPOnly, SameSite cookie, and mutating requests are CSRF-protected
  3. On first startup the system creates an admin user, initializes the data directory and Git repo, and self-heals any stale Git lock
  4. A user's available actions reflect their role (admin / editor / reader), and key actions (login, config changes) appear in an audit log
  5. Every file-path access is forced through a safe resolver that rejects `../`, absolute paths, and symlink escape (fuzz-tested)**Plans**: 4 plans

**Wave 1**

- [x] 00-01-PLAN.md — Walking Skeleton: scaffold + Argon2id/SCS/CSRF auth spine + admin bootstrap + login -> AppShell (wave 1)

**Wave 2** *(blocked on Wave 1 completion)*

- [x] 00-02-PLAN.md — Safe-path resolver (SEC-01, fuzz-tested) + single-writer Git commit spine + job worker + first-run repo seed (wave 2)

**Wave 3** *(blocked on Wave 2 completion)*

- [x] 00-03-PLAN.md — RBAC RequireRole + /admin user management + self-service profile + forced password change + logout + CLI reset (wave 3)

**Wave 4** *(blocked on Wave 3 completion)*

- [x] 00-04-PLAN.md — Audit log (SEC-05, SQLite mirror + slog) + full config.yaml schema + systemd/Docker packaging (wave 4)

**UI hint**: yes
**Notes**: SEC-* land here as the cross-cutting security floor (path resolver, Argon2id hashing, HTTPOnly/SameSite cookies, CSRF, audit scaffolding). The single-writer Git service and async job worker are introduced here as spines reused by every later phase. Remote-push divergence behavior (fast-forward-only pull with alert) and `pull_on_startup` semantics should be defined during Phase 0/1 planning. No phase research needed — chi middleware, Argon2id, SCS sessions, and nosurf CSRF are standard, well-documented patterns.

### Phase 1: OKF Pages, Navigation & Hidden Git

**Goal**: A non-technical user can create, edit, organize, link, and version Markdown pages through a file tree, with Git history kept entirely hidden behind the UI.
**Mode:** mvp
**Depends on**: Phase 0
**Requirements**: PAGE-01, PAGE-02, PAGE-03, PAGE-04, PAGE-05, PAGE-06, PAGE-07, PAGE-08, PAGE-09, NAV-01, NAV-02, NAV-03, NAV-04, NAV-05, VER-01, VER-02, VER-03, VER-04
**Success Criteria** (what must be TRUE):

  1. A user can create a page in a folder, edit its title/tags/description/body, save, and view it rendered as Markdown
  2. A user can rename, move, delete-to-trash, and restore pages, and link from one page to another with links staying valid after rename/move
  3. A user can browse pages in an expand/collapse left tree, create folders, see the current page highlighted, and see recently visited pages
  4. Saving a page automatically creates a Git commit recording the user's identity, with no Git knowledge required from the user
  5. A user can view a page's version history, restore a previous version, and (when configured) have commits pushed to a remote
  6. Saving a page with missing required frontmatter fills in the required fields without corrupting the file's Markdown bytes

**Plans**: 5 plans

**Wave 1**

- [x] 01-01-PLAN.md — okf byte-stable round-trip (golden-corpus exit gate) + CommitJob single-writer spine + drafts migration (wave 1)

**Wave 2** *(blocked on Wave 1)*

- [x] 01-02-PLAN.md — create/read/edit/save slice: page service + 409 floor + live tree (replaces PLACEHOLDER_TREE) + folder create + recents (wave 2)

**Wave 3** *(blocked on Wave 2)*

- [x] 01-03-PLAN.md — rename/move with eager link rewrite (round-trip-safe) + link picker + page action menu (wave 3)

**Wave 4** *(blocked on Wave 3)*

- [x] 01-04-PLAN.md — delete-to-trash + restore with provenance and collision handling (wave 4)

**Wave 5** *(blocked on Wave 4)*

- [x] 01-05-PLAN.md — version history (no SHAs) + restore-version forward commit + config-gated remote push (wave 5)

**UI hint**: yes
**Notes**: Spike recommended (not full phase research): prototype the single-writer Git batching + stale-lock recovery and the `internal/okf` byte-stable round-trip early — both have subtle failure modes. A golden-corpus byte-stable round-trip test is the Phase 1 exit gate (blocks Markdown round-trip rot). Per-file optimistic-concurrency floor (revision = content hash, 409 on mismatch) is scaffolded here and hardened in Phase 5. Confirm rename/move link-integrity strategy (eager rewrite vs. alias-redirect) during planning. The `internal/jobs` async worker is introduced here (CommitJob) and reused in Phases 2 and 3.

### Phase 2: Attachments & Text Extraction

**Goal**: A user can attach original files to pages, download them byte-for-byte unchanged, manage them safely, and have their text extracted so search and the agent can read them.
**Mode:** mvp
**Depends on**: Phase 1
**Requirements**: ATT-01, ATT-02, ATT-03, ATT-04, ATT-05, ATT-06, ATT-07, ATT-08, ATT-09, ATT-10, SEC-02
**Success Criteria** (what must be TRUE):

  1. A user can upload a file to a page and later download the unmodified original; uploads are validated against a size limit and MIME-sniffed allowed type
  2. A user sees an attachment card with original name, size, uploader, and date, and can preview PNG/JPG/SVG images inline
  3. A user can replace an attachment with a new version and remove its link from a page; when no page references it, the system deletes it from the repo
  4. Uploading or deleting an attachment automatically commits to Git, and risky download types are served with `Content-Disposition: attachment`
  5. The system extracts text from PDF/DOCX/TXT attachments (with a clear "no text extracted" state when extraction yields nothing)

**Plans**: 4 plans

**Wave 1**

- [ ] 02-01-PLAN.md — Upload + byte-exact download slice + `internal/attachments` foundation (deps, fixtures, config, migration, types/id/meta/refs) (ATT-01/02/09/10, SEC-02) (wave 1)

**Wave 2** *(blocked on Wave 1)*

- [ ] 02-02-PLAN.md — Full attachment card (thumbnail/icon, name·size·uploader·date) + inline image preview dialog (ATT-03/04) (wave 2)

**Wave 3** *(blocked on Wave 2)*

- [ ] 02-03-PLAN.md — Text extraction ExtractJob (pure-Go PDF/DOCX/TXT) + SSE status + ExtractionStatus chip, with the empty-but-succeeded "No text extracted" path (ATT-08) (wave 3)

**Wave 4** *(blocked on Wave 3)*

- [ ] 02-04-PLAN.md — Lifecycle: replace (reuse id, re-extract) + remove link + orphan delete in one commit (ATT-05/06/07) (wave 4)

**UI hint**: yes
**Notes**: PHASE RESEARCH COMPLETE (02-RESEARCH.md, HIGH confidence) — both spikes resolved: (1) large-binary-in-Git is workable at this scale with cap + MIME-sniffed allow-list guardrails (LOCKED, do not relitigate); (2) pure-Go extraction (`ledongthuc/pdf` + `fumiama/go-docx` + stdlib) validated against pinned versions, with the empty-but-succeeded "No text extracted" path for scanned/image PDFs. Generated (non-user-controlled) ULID storage names; metadata JSON sidecars + extracted-text sidecars form the three-part attachment model. ExtractJob extends the Phase 1 job worker (new KindExtract on the same single-writer worker; KindCommit reused unchanged); SSE surfaces extraction status. All attachment writes/deletes flow through the existing single-writer CommitJob — no second write path.

### Phase 3: Search

**Goal**: A user can quickly find any knowledge in the workspace — across page titles, body, tags, attachment filenames, and extracted attachment text — with typed results.
**Mode:** mvp
**Depends on**: Phase 2
**Requirements**: SRCH-01, SRCH-02, SRCH-03, SRCH-04, SRCH-05, SRCH-06
**Success Criteria** (what must be TRUE):

  1. A user can search page titles and full body text and get relevant results
  2. A user can search by tag and by attachment filename
  3. A user can search the extracted text of attachments and find the pages they belong to
  4. Search results are typed as page, attachment, or heading so the user knows what they found

**Plans**: 4 plans

**Wave 1**

- [ ] 03-01-PLAN.md — Backend search foundation: Bleve v2.6.0 typed index + mapping + query builder (title/body/tag, title boost, type facet) + idempotent rebuild-from-files + startup drift detection + GET /search (authed) & POST /admin/search/reindex (admin) + Wave 0 test scaffolds (wave 1)

**Wave 2** *(blocked on Wave 1 completion)*

- [ ] 03-02-PLAN.md — ⌘K SearchPalette UI: zustand store + debounced react-query hook + search() client + Obsidian-style palette (focus-trap, keyboard nav, 5 states, weight-only highlight) + top-bar trigger (wave 2)
- [ ] 03-03-PLAN.md — Attachments + extracted-text (owning-page link) + headings deep-link: ATX heading scanner + attachment/heading typed docs + stale-heading cleanup + rehype-slug renderer anchors (wave 2)

**Wave 3** *(blocked on Wave 2 completion)*

- [ ] 03-04-PLAN.md — Lifecycle hardening: incremental KindIndex enqueues on all page/attachment mutations + extraction-done + admin Rebuild UI + concurrency race test + end-of-phase human verification (wave 3)

**UI hint**: yes
**Notes**: No phase research needed — standard Bleve patterns. Bleve indexes both page content (Phase 1) and extracted attachment text (Phase 2), so it follows both. The rebuild-from-files reindex job (admin action + startup HEAD-mismatch trigger) is this phase's primary engineering concern alongside the index itself, and is the defense against SQLite/Bleve drift. Incremental IndexJob wires to page-save and extraction-done events, reusing the Phase 1 job worker.

### Phase 4: Eino Agent

**Goal**: A user can ask an AI agent to read, summarize, rewrite, draft, and propose edits over a page, selection, attachment, or the whole workspace — and every write requires explicit human approval of a concrete diff.
**Mode:** mvp
**Depends on**: Phase 3
**Requirements**: AGNT-01, AGNT-02, AGNT-03, AGNT-04, AGNT-05, AGNT-06, AGNT-07, AGNT-08, AGNT-09, AGNT-10, AGNT-11
**Success Criteria** (what must be TRUE):

  1. A user can ask the agent a question scoped to the current page, selected text, a selected attachment, or the whole workspace
  2. A user can ask the agent to summarize a page or an attachment, rewrite selected text, or draft a new page, and see the result streamed
  3. A user can ask the agent to propose a patch to the current page and see it rendered as a concrete diff
  4. No agent change is applied or committed until the user explicitly approves the diff
  5. The agent cannot write files directly, read secrets, run shell, escape the repository, or push to Git — enforced in the Go tool layer, not by prompt

**Plans**: TBD
**UI hint**: yes
**Notes**: NEEDS PHASE RESEARCH — Eino is pre-1.0 (v0.9.9, fast-moving). Before Phase 4 planning, re-verify `react.NewAgent` / `AgentConfig` / `utils.InferTool` / `openai.NewChatModel` signatures against current eino + eino-ext source, confirm the interrupt/resume pattern for the approval gate, and test the chosen provider with `utils.InferTool`-generated schemas before building the full loop. Pin both `eino` and `eino-ext` via `go.sum` immediately after `go get` and commit the lockfile. The approval gate is the load-bearing defense against indirect prompt injection: the DiffReviewDialog must show a real diff (never a prose summary), and the read/write boundary is structural (write tools are NOT in the Eino graph). Every agent file read goes through `repo.Resolve`. The DiffReviewDialog built here is reused in Phase 5. Audit logs capture prompt + approval.

### Phase 5: Collaboration

**Goal**: A small team can edit concurrently without silently overwriting each other — seeing who is editing, getting soft-lock warnings, and resolving conflicts through a clear diff with safe choices.
**Mode:** mvp
**Depends on**: Phase 4
**Requirements**: COLL-01, COLL-02, COLL-03, COLL-04
**Success Criteria** (what must be TRUE):

  1. A user can see when another user is currently editing a page (presence indicator)
  2. The system applies a soft lock while a page is being edited, and a user can still choose to force-edit
  3. Saves use optimistic concurrency with a per-document revision, and a stale save is rejected rather than silently overwriting
  4. On a save conflict, the user is shown a diff and can choose overwrite, manual merge, or save-as-copy (which creates a new page)

**Plans**: TBD
**UI hint**: yes
**Notes**: No phase research needed — conflict UX is well-specified (SPEC §13.1) and the soft-lock file format with TTL/heartbeat is straightforward. This phase hardens and completes the optimistic-concurrency floor scaffolded in Phase 1: the revision check must still run when a user force-edits past a soft lock, and stale locks (session end/crash) must never cause silent data loss. The conflict-resolution UI reuses the DiffReviewDialog built in Phase 4. Soft lock files live in `.okf-workspace/locks/` with user + heartbeat TTL; presence is delivered over SSE.

### Phase 6: Live-Preview Editor (Obsidian-style)

**Goal**: As an editor accustomed to Obsidian, I want a live-preview Markdown editor that renders formatting inline as I type (with a source/raw toggle), so that editing in the web app feels like Obsidian rather than a split source+preview pane.
**Mode:** mvp
**Depends on**: Phase 1
**Requirements**: EDIT-01..EDIT-04 (new — to be formalized in REQUIREMENTS.md at spec/plan time)
**Success Criteria** (what must be TRUE):

  1. While editing, Markdown formatting (headings, bold/italic, lists, links, inline code, code blocks) renders inline in the editor as the user types — not only in a separate preview pane
  2. The user can toggle between live-preview and raw-source modes, and switching modes never alters the underlying Markdown bytes
  3. Saving from the live-preview editor produces byte-identical Markdown to the source-mode round-trip — the okf golden-corpus exit gate still holds (no lossy block model)
  4. Existing editor guarantees are preserved: autosave drafts, optimistic-concurrency save, and sanitized rendering (rehype-sanitize on / raw HTML off)

**Plans**: 4/4 plans complete

**Wave 1**

- [x] 06-01-PLAN.md — CM6 editor swap slice: add CM6/Lezer deps + all Wave-0 test stubs + persisted Live/Source mode store + minimal LivePreviewEditor swapped into PageEditor (verbatim save, Compartment toggle, Cmd/Ctrl-E) (EDIT-02/03/04) (wave 1)

**Wave 2** *(blocked on Wave 1)*

- [x] 06-02-PLAN.md — Live-preview ViewPlugin: Lezer tree-walk decorations (headings/bold/italic/links/inline-code/code-blocks) + active-line marker reveal + theme parity with MarkdownProse (EDIT-01 text constructs) (wave 2)

**Wave 3** *(blocked on Wave 2)*

- [x] 06-03-PLAN.md — Inline image + GFM table widgets (sanitized, no-innerHTML) + internal `.md` link SPA navigation; completes the EDIT-01 render set + the image-src V5 control (EDIT-01/04) (wave 3)

**Wave 4** *(blocked on Wave 3)*

- [x] 06-04-PLAN.md — Unified read-only surface: heading-anchor ids (==okf slug, SRCH-06 preserved) + scroll-to-#hash + PageView swap + retire MarkdownProse + remove @uiw/react-md-editor + CLAUDE.md update + end-of-phase human verify (EDIT-01/04) (wave 4) — implementation + automated coverage complete; perceptual human-verify deferred to phase-level verification

**UI hint**: yes
**Notes**: Part of the Obsidian-alignment UI direction (team are ex-Obsidian users; the web app stays the client). Replace the MVP `@uiw/react-md-editor` split-pane with a CodeMirror 6 live-preview surface (inline-rendered Markdown decorations — the same engine family Obsidian uses). Storage/sync are OUT of scope and unchanged: Git remains the system of record; live multi-user co-editing stays in Phase 5 (CRDT→Git), NOT a store swap. Must preserve the okf byte-stable round-trip (raw Markdown in/out) and keep rehype-sanitize / raw-HTML-off. Sibling Obsidian-feel items (quick switcher Ctrl-O, command palette Ctrl-P, `[[wikilink]]` autocomplete, backlinks panel, denser file tree, dark theme) are related but tracked separately. Depends on Phase 1's editor; position can be moved earlier than 2–5 (via `/gsd-phase --edit`/`--insert`) if the team wants the editor first.

### Phase 7: Obsidian-style File Tree (folder operations & tree UX)

**Goal**: As an editor used to Obsidian, I want to manage folders and files directly in the file tree — right-click context menus, drag-and-drop, and folder rename/move/delete — so that organizing the workspace feels like Obsidian instead of needing per-page actions.
**Mode:** mvp
**Depends on**: Phase 1
**Requirements**: TREE-01..TREE-06 (new — to be formalized in REQUIREMENTS.md at spec/plan time)
**Success Criteria** (what must be TRUE):

  1. Right-clicking a folder or page in the tree opens a context menu with the relevant actions (folder: New page here, New folder here, Rename, Move, Delete; page: Rename, Move, Delete, Version history)
  2. A folder can be renamed, moved into another folder, and deleted as a unit — every page inside it relocates, all inbound links are rewritten in the SAME commit, and nothing is corrupted (the okf byte-stable round-trip still holds)
  3. Folders and pages can be reorganized by drag-and-drop (drop onto a folder or root to move), and the tree updates immediately with no manual refresh
  4. Deleting a folder is recoverable — its pages go to trash and can be restored; there is no permanent delete in this phase

**Plans**: 4/4 plans executed

**Wave 1**

- [x] 07-01-PLAN.md — Backend folder relocate (rename/move as a unit + ErrFolderExists collision reject) + folder rename/move routes + Go tests (TREE-02, TREE-06 backend) (wave 1) — COMPLETE 2026-06-21

**Wave 2** *(blocked on Wave 1)*

- [x] 07-02-PLAN.md — Backend folder delete-to-trash + grouped restore + migration 0008 (delete_group_id) + grouped-restore route + Go tests (TREE-04, TREE-05) (wave 2)

**Wave 3** *(blocked on Wave 2)*

- [x] 07-03-PLAN.md — Frontend regression-net pin (Clean-Rebuild Behavior Inventory) + clean rebuild of LeftTree/TreeContextMenu/RenameModal/MoveDialog, no new features, no regression (TREE-01 page side) (wave 3)

**Wave 4** *(blocked on Wave 3)*

- [x] 07-04-PLAN.md — Frontend folder ops: 5-action folder menu + folder DnD (self/descendant guard) + optimistic ["tree"] updates + DeleteFolderDialog + grouped TrashView restore + human-verify checkpoint (TREE-01 folder, TREE-03, TREE-05 UI, TREE-06 client) (wave 4) — COMPLETE 2026-06-21 (240 frontend tests green; perceptual DnD verification deferred to phase-level human verify)

**UI hint**: yes
**Notes**: Formalizes the Obsidian-file-tree direction. ALREADY SHIPPED ad-hoc on `main` during Phase 1 UAT (fold in / do NOT re-do): the page-level right-click context menu, page drag-and-drop move, folder-scoped create ("New page/folder here"), the reusable `TreeContextMenu` component, the dialog-footer fix, and the commit-wait fix that makes tree updates appear on the fly — commits `69e4fb6`, `ee5192c`, `a1486bd`, `7e0b098`, `717cfe7`. REMAINING net-new work this phase covers: **backend folder operations** — rename/move/delete-to-trash a folder as a unit, recursively relocating all contained pages and rewriting inbound links in one commit via the okf round-trip-safe path (reuse Phase 1's `relocate` + trash machinery) — plus making folders draggable/droppable and wiring the folder context menu to those ops. Folder delete trashes the contained pages (restorable); folder-restore semantics (per-page vs grouped) to be decided at planning. Excluded (Obsidian-only / other phases): canvas/base doc types, search-in-folder (Phase 3), bookmarks, copy-path / show-in-system-explorer (paths are hidden by design). Depends on Phase 1; independent of Phases 2–6 — can be reprioritized earlier via `/gsd-phase --edit`/`--insert`.

## Progress

**Execution Order:**
Phases execute in numeric order: 0 → 1 → 2 → 3 → 4 → 5 → 6 → 7

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 0. Skeleton, Auth & Foundations | 4/4 | Complete | 2026-06-18 |
| 1. OKF Pages, Navigation & Hidden Git | 5/5 | Complete   | 2026-06-18 |
| 2. Attachments & Text Extraction | 0/4 | Planned | - |
| 3. Search | 0/TBD | Not started | - |
| 4. Eino Agent | 0/TBD | Not started | - |
| 5. Collaboration | 0/TBD | Not started | - |
| 6. Live-Preview Editor (Obsidian-style) | 4/4 | Complete   | 2026-06-21 |
| 7. Obsidian-style File Tree (folder operations & tree UX) | 4/4 | Complete   | 2026-06-21 |
