# Phase 1: OKF Pages, Navigation & Hidden Git - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-06-18
**Phase:** 1-OKF Pages, Navigation & Hidden Git
**Areas discussed:** Save & commit model, Links + rename/move integrity, Delete-to-trash & restore, New-page creation flow

---

## Save & commit model

### Q1 — How should saving a page map to Git commits?

| Option | Description | Selected |
|--------|-------------|----------|
| Explicit Save = 1 commit | One PUT → one commit via existing CommitJob; no autosave; simplest/most predictable | |
| Autosave draft + batched commit | Body autosaves to working tree; commit batched after Save or idle (SPEC §14.2 richer shape) | ✓ |
| Explicit Save now, batch later | Ship explicit-Save=1-commit but design queue for later batching | |

**User's choice:** Autosave draft + batched commit
**Notes:** Adopts SPEC §14.2's richer model up front rather than deferring.

### Q2a — Where should the autosaved draft live before commit?

| Option | Description | Selected |
|--------|-------------|----------|
| Draft in SQLite, .md written on commit | Draft in SQLite keyed by page+user; .md only changes when batched CommitJob fires; working tree always = last commit | ✓ |
| Draft written to working tree, commit batched | SPEC §14.2 literal: autosave writes .md continuously; commit batches; on-disk file can be saved-but-uncommitted | |
| You decide | — | |

**User's choice:** Draft in SQLite, .md written on commit
**Notes:** Chosen specifically to keep the working tree byte-equal to last commit, protecting the round-trip exit gate and files-as-truth.

### Q2b — What should trigger the batched commit?

| Option | Description | Selected |
|--------|-------------|----------|
| Explicit Save + idle fallback | Commit on Save click OR after ~5–10s idle with dirty draft | ✓ |
| Explicit Save only | Commit only on Save; autosave still protects draft | |
| Idle/debounce only | No Save trigger; commits purely on debounce | |

**User's choice:** Explicit Save + idle fallback
**Notes:** Belt-and-suspenders against lost work while keeping user control of when a version is cut.

---

## Links + rename/move integrity

### Q1 — Canonical link format + how users create links

| Option | Description | Selected |
|--------|-------------|----------|
| Relative .md path + picker | Standard Markdown relative paths (SPEC §10); picker inserts path; in-app navigation on click | ✓ |
| Wiki-style [[Page]] links | Obsidian-style, app-resolved; non-standard, less portable | |
| Stable page ID links | Immutable ID in frontmatter; rename-proof but not human-readable | |

**User's choice:** Relative .md path + picker
**Notes:** Keeps content portable/agent-readable off-server, matches SPEC §10 example.

### Q2 — Inbound links on rename/move

| Option | Description | Selected |
|--------|-------------|----------|
| Eager rewrite inbound links | Rewrite every inbound link to new path in the same commit | ✓ |
| Alias/redirect stub | Leave old path resolvable via alias; don't touch inbound links | |
| Eager rewrite, alias as safety net | Rewrite eagerly AND record alias fallback | |

**User's choice:** Eager rewrite inbound links
**Notes:** Directly satisfies success criterion 2 ("links stay valid after rename/move"). Rewrites must go through the round-trip-safe edit path so other bytes aren't corrupted. Resolves the ROADMAP-flagged open decision.

---

## Delete-to-trash & restore

### Q1 — How delete-to-trash and restore work on disk and in Git

| Option | Description | Selected |
|--------|-------------|----------|
| git mv to .okf-workspace/trash/ | Delete moves file into trash dir via single-writer (commit); restore moves back; history continuous | ✓ |
| git rm + restore from history | Delete is git rm; trash is a view over history; restore re-creates from last commit | |
| You decide | — | |

**User's choice:** git mv to .okf-workspace/trash/
**Notes:** Matches SPEC §9 `.okf-workspace/trash/` layout; preserves history through the move.

### Q2 — Folder deletion + trash metadata

| Option | Description | Selected |
|--------|-------------|----------|
| Pages only in MVP; folders implicit | Trash/restore for pages; folders disappear when empty; trash records original path + deleted-by + timestamp | ✓ |
| Pages and folders both trashable | Recursive folder delete/restore as a unit; more edge cases | |
| You decide | — | |

**User's choice:** Pages only in MVP; folders implicit
**Notes:** Smallest correct surface. Dangling inbound links after a page delete are acceptable in MVP (warn-before-delete is an optional refinement).

---

## New-page creation flow

### Q1 — Filename derivation + creation UX

| Option | Description | Selected |
|--------|-------------|----------|
| Modal: title → slug filename | Modal asks for title; backend slugifies to filename in selected folder; collision suffixing; frontmatter pre-filled | ✓ |
| Inline 'Untitled' + rename | Immediately create untitled.md and drop into edit mode; filename follows from title later | |
| You decide | — | |

**User's choice:** Modal: title → slug filename
**Notes:** Hides paths/filenames from users per SPEC §3.

### Q2 — Default frontmatter `type` for new pages

| Option | Description | Selected |
|--------|-------------|----------|
| type: Page | Generic default (SPEC §10 required-fields example); changeable later | ✓ |
| Infer from folder | runbooks/ → Runbook, etc.; couples type to folder names | |
| Ask in create modal | Type dropdown in modal; adds a field + controlled vocabulary | |

**User's choice:** type: Page
**Notes:** Simplest; no type-picker needed in MVP; user can change `type` by editing later.

---

## Claude's Discretion

- Version-history view rendering (commit list, no SHAs surfaced).
- Restore-version mechanics: write old content as a new forward commit (never rewrite history).
- Remote push (VER-04): reuse Phase-0 config keys + ff-only-pull / alert-on-divergence semantics (D-12).
- Recent pages (NAV-05): client-side (localStorage/zustand), not server-side.
- Tree titling (frontmatter `title`, fallback filename) and ordering.
- Exact idle-debounce interval for the autosave→commit fallback.
- Optimistic-concurrency floor: revision = content hash, 409 on stale PUT (full conflict UX deferred to Phase 5).

## Deferred Ideas

- Full conflict-resolution UX, presence, soft locks → Phase 5.
- DiffReviewDialog / agent-proposed-patch page mode (§18.3 Diff review) → Phase 4.
- Warn-before-deleting-a-linked-page (dangling-link guard) — nice-to-have, not MVP.
- Folder delete/move-to-trash as a unit — out of MVP scope.
- Server-side / cross-device recent pages.
- Folder-inferred or picker-selected page `type`.
- Configurable idle-debounce interval / draft retention policy.
