# Phase 1: OKF Pages, Navigation & Hidden Git - Context

**Gathered:** 2026-06-18
**Status:** Ready for planning

<domain>
## Phase Boundary

Phase 1 delivers the **core wiki loop**: a non-technical user can **create, edit, organize, link, and version Markdown pages through a left file tree**, with Git history kept entirely hidden behind the UI. Built directly on the Phase 0 spines (safe-path resolver `internal/repo`, single-writer Git commit service `internal/gitstore`, async job worker `internal/jobs`, shared `internal/store`).

**Requirements covered (18):** PAGE-01..09, NAV-01..05, VER-01..04 (see `.planning/REQUIREMENTS.md`).

**In scope:**
- Page create (in selected folder), read (rendered Markdown), edit (title/tags/description/body), save, delete-to-trash, restore-from-trash, rename, move.
- OKF page format: YAML frontmatter parse + **required-field repair on save** without corrupting Markdown bytes (PAGE-09).
- **Byte-stable Markdown round-trip** (`internal/okf`) — golden-corpus test is the **phase exit gate**.
- Page-to-page linking with link validity preserved across rename/move.
- Navigation: left expand/collapse file tree, create folder, current-page highlight, recent pages.
- Hidden Git: automatic commit on save with user-identity metadata; page history view; restore previous version; optional remote push (config-gated).
- Per-file optimistic-concurrency **floor** (revision = content hash, 409 on stale save) — scaffolded here, hardened in Phase 5.

**Explicitly NOT in this phase (later phases):** attachments + text extraction (Phase 2), search/Bleve (Phase 3), Eino agent + DiffReviewDialog (Phase 4), presence / soft locks / full conflict-resolution UX (Phase 5). The DiffReviewDialog page mode (§18.3) is *not* built here — it arrives with the agent in Phase 4.

</domain>

<decisions>
## Implementation Decisions

### Save & commit model
- **D-01:** Editing uses an **autosave-draft + batched-commit** model (SPEC §14.2's richer shape), NOT one-commit-per-keystroke and NOT a bare explicit-Save=commit.
- **D-02:** The **autosaved draft persists in SQLite** (operational data, via `internal/store`), keyed by page path + user. The canonical `.md` file on disk is **only written/changed when the batched CommitJob fires**. Consequence (load-bearing): the **working tree is always byte-equal to the last Git commit** — this keeps "files are truth" clean and protects the byte-stable round-trip exit gate (no "saved-but-uncommitted" files on disk).
- **D-03:** The batched commit triggers on **explicit Save OR a short idle fallback** (~5–10s of no edits with a dirty draft). Belt-and-suspenders against lost work while still giving the user explicit control over when a version is cut.
- **D-04:** The commit goes through the **existing single-writer Git spine** (`gitstore.CommitSpec`/`Commit` + a `CommitJob` on the `internal/jobs` worker). Do not bypass the single-writer service with raw `git` calls. Commit message carries user identity + action + source per SPEC §14.2.

### Page links & rename/move integrity
- **D-05:** Canonical on-disk link format is a **standard Markdown relative `.md` path** (e.g. `[Deploy](../runbooks/deploy.md)`) — matches SPEC §10's example and keeps content portable/agent-readable off-server. No wiki-style `[[...]]`, no app-only ID links.
- **D-06:** Users insert links via a **"link to page" picker** that emits the correct relative path; typing Markdown links directly is also supported. In **read mode**, clicking a page link **navigates within the app** (does not leave to a raw file).
- **D-07:** On **rename/move**, **eagerly rewrite every inbound link** across the repo to the new path, committed in the **same commit (or a paired commit)** as the move, so links stay valid (success criterion 2). Link rewrites MUST go through the **round-trip-safe edit path** (`internal/okf`) so rewriting a link never corrupts other Markdown bytes. Accept the cost of a repo-wide link scan per rename/move (fine at ~5 users / small repo).

### Delete-to-trash & restore
- **D-08:** Delete = **`git mv` of the page into `.okf-workspace/trash/`** via the single-writer Git service (a real commit) — history is continuous through the move. Restore = move it back (another commit). No `git rm` + history-resurrection.
- **D-09:** **Pages only** are trashable in MVP; **folders are implicit** (a folder is just a directory and disappears from the tree when it has no pages). No explicit "delete folder" action in Phase 1.
- **D-10:** Trash records the **original path + deleted-by + timestamp** (so restore knows where it came from). Restore-target collisions handled (suffix or prompt).
- **D-11 (refinement, not blocking):** Deleting a page that other pages link to leaves those inbound links **dangling** — acceptable for MVP. A "warn before deleting a linked page" affordance is a *nice-to-have* the planner may add but is not required.

### New-page creation flow
- **D-12:** Create opens a **small modal asking for the title**; the backend **slugifies the title into a filename** (e.g. "Deploy Staging" → `deploy-staging.md`) inside the selected folder, with **collision suffixing** (`-2`, `-3`, …). Filenames/paths stay hidden from the user (SPEC §3 "users shouldn't need to understand file paths"). No transient `untitled.md` inline-create flow.
- **D-13:** New pages are **pre-filled with valid required frontmatter**: `type: Page` (default), generated `title`, ISO-8601 `timestamp`, empty `tags`, empty `description`. This guarantees freshly created pages already pass frontmatter validation (no repair churn). No type-picker / folder-inferred type in MVP — user can change `type` later by editing.

### Version history & restore (Claude's discretion — sensible defaults, not asked)
- **VER-02 history view:** list the page's Git commits (timestamp, author/display name, action) via the backend Git service; no raw SHAs surfaced to the user.
- **VER-03 restore:** restore writes the chosen old version's content as a **new forward commit** (preserves history; never rewrites/`reset`s Git history). Restore flows through the same single-writer commit path.
- **VER-04 push:** reuse the Phase-0 config keys (`git.remote_enabled`, `git.push_on_commit`, `git.pull_on_startup`, ff-only pull, branch). When enabled, push happens after commit; on divergence, alert and refuse to auto-merge (per Phase-0 D-12). Push is the deferred-from-Phase-0 piece that lands here.

### Navigation details (Claude's discretion — sensible defaults, not asked)
- **Tree (NAV-01/02):** built from `GET /api/v1/tree` (SPEC §17.2); folders expand/collapse; page rows titled by **frontmatter `title`** (fall back to filename). Current page highlighted (NAV-04).
- **Recent pages (NAV-05):** **client-side** (localStorage / zustand) per the existing frontend state pattern — no server round-trip needed for a 5-user tool. (Revisit if cross-device recents are ever wanted.)
- **Create folder (NAV-03):** `POST /api/v1/folders`; a folder with a seeded/blank `index.md` is acceptable so empty folders are representable.

### Optimistic-concurrency floor (scaffold; hardened in Phase 5)
- Page GET returns `revision` (content hash); PUT carries `base_revision`; backend returns **409** on mismatch rather than silently overwriting. Full conflict UX (overwrite / merge / save-as-copy, presence, soft locks) is **Phase 5** — only the revision check + 409 is built now.

### Claude's Discretion (summary)
History-view rendering, restore mechanics (forward commit), recent-pages storage (client-side), tree titling/ordering, folder-create representation, and the exact idle-debounce interval are left to research/planning within the constraints above. No "you decide" punts were taken on the four discussed areas — all are locked above.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Product & technical spec (source of truth)
- `SPEC.md` §5.3 — Git is hidden from users (drives the "no Git knowledge required" UX constraint).
- `SPEC.md` §5.5 + §22.3 — Markdown round-trip must be protected; the round-trip test list (headings, lists, nested lists, code blocks, tables, links, images, attachment links, frontmatter preservation) — **the Phase 1 exit gate**.
- `SPEC.md` §6.1 / §6.2 / §6.6 — Pages, Navigation, and Versioning MVP feature lists.
- `SPEC.md` §10 — **OKF page format**: required frontmatter (`type`, `title`, `description`, `tags`, `timestamp`), optional fields, "repair missing required fields on save", and the relative-path link example (drives D-05/D-13/PAGE-09).
- `SPEC.md` §14 (§14.1 commit triggers, §14.2 commit batching + message format, §14.3 remote sync) — drives D-01..D-04 and VER-04.
- `SPEC.md` §17.2 — Tree API (`GET /tree`, `POST /folders`, `POST /pages`) + tree response shape.
- `SPEC.md` §17.3 — Pages API (`GET/PUT/DELETE /pages/{path}`, `/rename`, `/history`, `/restore`) + page response (`frontmatter`, `body`, `revision`) and update request (`base_revision`) — drives the optimistic-concurrency floor.
- `SPEC.md` §18.3 — Page modes (Read / Edit / Diff review / History); Phase 1 builds Read/Edit/History (Diff review is Phase 4).
- `SPEC.md` §9 — Repository layout (`index.md`, topic folders, `.okf-workspace/{manifest.json,trash/,locks/}`) — drives D-08 trash location.
- `SPEC.md` §16 / §16.1 — Backend package layout & Git service responsibilities (`internal/okf` is added here).
- `SPEC.md` §20.2–§20.3 — Data dir layout and `config.yaml` git keys (remote/push/pull/branch).

### Architecture, build order & pitfalls
- `.planning/research/ARCHITECTURE.md` — Component responsibilities; single-writer Git pattern (Pattern 3); one-way projection invariant (files truth, SQLite rebuildable); where `internal/okf` and `CommitJob` slot in.
- `.planning/research/PITFALLS.md` — Pitfall 2 (concurrent Git `index.lock` → single-writer + stale-lock self-heal, reused here for CommitJob) and the Markdown-round-trip rot risk that the golden-corpus gate defends against.

### Stack (locked versions & library choices)
- `CLAUDE.md` / `.planning/research/STACK.md` — Locked: Goldmark v1.8.2 (Markdown render) + `gopkg.in/yaml.v3` with `yaml.Node` for **round-trip-safe** frontmatter; `@uiw/react-md-editor` (Markdown-with-preview editor); `react-markdown` + `remark-gfm` + `rehype-sanitize` (read-mode render must match Goldmark's GFM); `react-diff-viewer-continued` (history/diff UI); `@tanstack/react-query` + `zustand` (server/UI state); shell-out `git` CLI; `modernc.org/sqlite` (draft store).

### Project framing, prior context & requirements
- `.planning/PROJECT.md` — Key Decisions (files-as-truth, SQLite operational-only, Git hidden, Markdown-with-preview editor, TipTap deferred).
- `.planning/REQUIREMENTS.md` — PAGE-01..09, NAV-01..05, VER-01..04 definitions + traceability.
- `.planning/ROADMAP.md` — Phase 1 goal, 6 success criteria, and Notes (spike single-writer batching + `internal/okf` round-trip; golden-corpus exit gate; optimistic-concurrency floor; confirm rename/move link-integrity strategy — **resolved here: eager rewrite, D-07**).
- `.planning/phases/00-skeleton-auth-foundations/00-CONTEXT.md` — Phase-0 decisions reused here: single-writer Git spine, job worker, safe-path resolver, `internal/store`, D-12 remote-sync semantics (ff-only pull, alert-on-divergence), seeded starter layout.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets (built in Phase 0, consumed here)
- `internal/repo/path.go` — **safe-path resolver** (fuzz-tested chokepoint). Every page/folder/trash path MUST resolve through it. `internal/repo/files.go` for file read/write primitives.
- `internal/gitstore/commit.go` — `GitStore.Commit(ctx, CommitSpec)` + `buildMessage`/`authorEmail`. Reuse `CommitSpec` for page save, rename (with link rewrites), trash/restore, and restore-version commits. `git.go`/`health.go` for the single-writer git invocation and stale-lock health.
- `internal/jobs/{queue.go,worker.go}` — async job worker spine. **Add `CommitJob`** here (first reuse of the worker introduced in Phase 0).
- `internal/store` — shared `*sql.DB` + migrations. **Add a drafts table** (autosave) here; never store canonical page content in SQLite (operational-only invariant).
- Frontend: `web/src/routes/AppShell.tsx` already has the chrome (top bar, nav rail, main pane) and a **`PLACEHOLDER_TREE`** that this phase replaces with the live `GET /api/v1/tree`. Reuse `components/Dialog.tsx` for the create-page modal; `@tanstack/react-query` is already wired (`me`, `health` queries) for page/tree fetching.

### Established Patterns (to follow)
- **Single-writer Git**: all repo writes serialize through one writer (mutex/worker). New write paths (save, rename+rewrite, trash, restore) must enqueue through it — never write the repo directly.
- **One-way projection**: `.md` files are truth; SQLite (drafts, revisions cache, recents if ever server-side) is a rebuildable/operational cache.
- **Frontmatter round-trip**: use `yaml.Node` so unknown/optional fields survive a save; required-field repair must be additive and byte-safe.

### Integration Points
- **New package `internal/okf`** — parse/emit OKF Markdown (frontmatter + body), required-field repair, byte-stable round-trip. The golden-corpus test lives here and gates the phase.
- **New package(s) for pages/tree handlers** (`internal/okf` service + chi routes under `/api/v1/{tree,folders,pages}`) wired into the existing `internal/server` middleware stack (session/CSRF/RBAC/audit already attach there).
- **RBAC becomes meaningful here**: editor-vs-reader gating (readers can view, editors can create/edit/delete) now exercises the `RequireRole` middleware built in Phase 0.
- **`cmd/okf-workspace/main.go`** startup wiring extends to register `CommitJob` and the new page/tree routes.

</code_context>

<specifics>
## Specific Ideas

- The edit loop should feel "Notion-like" but never lose work: type → autosave draft (SQLite) → Save (or idle) cuts a hidden Git commit. The user never sees a SHA, a branch, or a path — only "Saved" and a human-readable history list.
- Create flow mirrors the non-technical promise: "New page" → type a title → it appears in the tree. Filename slugging and frontmatter scaffolding happen invisibly.
- Trash should feel like a recycle bin: deleted pages are recoverable from `.okf-workspace/trash/` with their original location remembered, and nothing is ever truly destroyed by a delete in MVP.
</specifics>

<deferred>
## Deferred Ideas

- **Full conflict-resolution UX** (overwrite / manual-merge / save-as-copy), **presence indicators**, and **soft locks** — Phase 5. Phase 1 builds only the revision/409 floor.
- **DiffReviewDialog / agent-proposed-patch page mode (§18.3 Diff review)** — Phase 4 (agent).
- **Warn-before-deleting-a-linked-page** affordance (dangling-link guard) — nice-to-have refinement, not required in MVP (D-11).
- **Folder delete/move-to-trash as a unit** — out of MVP scope (D-09); pages-only for now.
- **Server-side / cross-device recent pages** — Phase 1 keeps recents client-side; revisit only if multi-device recents are wanted.
- **Folder-inferred or picker-selected page `type`** — MVP defaults to `type: Page` (D-13); richer type selection deferred.
- **Configurable idle-debounce interval / draft retention policy** — sensible default now; expose later if needed.

None of these were scope creep into other capabilities — they are hardening/UX refinements of Phase 1's own surface or items the roadmap already places in later phases.
</deferred>

---

*Phase: 1-OKF Pages, Navigation & Hidden Git*
*Context gathered: 2026-06-18*
