---
gsd_state_version: 1.0
milestone: v0.9.9
milestone_name: milestone
status: executing
stopped_at: Phase 6 planned (4 plans verified); executing
last_updated: "2026-06-21T11:07:06.793Z"
last_activity: 2026-06-21 -- Phase 06 execution started
progress:
  total_phases: 8
  completed_phases: 4
  total_plans: 21
  completed_plans: 20
  percent: 50
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-06-17)

**Core value:** A non-technical teammate can create, edit, and find knowledge — and get useful AI help on it — while every byte stays as plain Markdown + original files on disk, versioned in Git, with no proprietary store to escape.
**Current focus:** Phase 06 — live-preview-editor-obsidian-style

## Current Position

Phase: 06 (live-preview-editor-obsidian-style) — EXECUTING
Plan: 4 of 4
Status: Ready to execute
Last activity: 2026-06-21 -- Phase 06 execution started

Progress: [████████░░] 80%

## Quick Tasks Completed

| Date | Slug | Summary | Commit |
|------|------|---------|--------|
| 2026-06-21 | admin-change-role | Wire admin "change user role" UI to existing `setRole` endpoint (Phase 0 UAT gap) | 436f9d7 |
| 2026-06-21 | autosave-trailing-write | Fix autosave silently dropping/clobbering a trailing edit (Phase 1 UAT gap); serialized coalescing saver | 7985857 |

## Performance Metrics

**Velocity:**

- Total plans completed: 0
- Average duration: — min
- Total execution time: 0.0 hours

**By Phase:**

| Phase | Plans | Total | Avg/Plan |
|-------|-------|-------|----------|
| - | - | - | - |

**Recent Trend:**

- Last 5 plans: —
- Trend: —

*Updated after each plan completion*
| Phase 00 P01 | 18 | 3 tasks | 43 files |
| Phase 00 P02 | 12 | 3 tasks | 19 files |
| Phase 00 P03 | 84 | 2 tasks | 20 files |
| Phase 00 P04 | 14 | 2 tasks | 17 files |
| Phase 01 P01 | 25 | 2 tasks | 20 files |
| Phase 01 P02 | 70 | 4 tasks | 42 files |
| Phase 01 P03 | 75 | 2 tasks | 25 files |
| Phase 01 P05 | 13m | 2 tasks | 21 files |
| Phase 06 P01 | 17min | 4 tasks | 20 files |
| Phase 06 P02 | 12min | 2 tasks | 5 files |
| Phase 06 P03 | 11min | 2 tasks | 10 files |

## Accumulated Context

### Roadmap Evolution

- Phase 6 added (2026-06-20): Live-Preview Editor (Obsidian-style) — replace the MVP split-pane editor with a CodeMirror 6 live-preview surface; depends on Phase 1; storage/Git unchanged. Part of the broader "stay web but mimic Obsidian" UI direction (team are ex-Obsidian users). CouchDB-as-Git-replacement was considered and rejected (breaks files-as-truth + hidden-Git history); live co-editing stays Phase 5 via CRDT→Git.
- Phase 7 added (2026-06-21): Obsidian-style File Tree (folder operations & tree UX) — depends on Phase 1. Page-level tree UX (right-click menu, page drag-drop, folder-scoped create, TreeContextMenu, dialog-footer fix) and the commit-wait fix were already shipped ad-hoc on main during Phase 1 UAT (commits 69e4fb6/ee5192c/a1486bd/7e0b098/717cfe7); this phase formalizes them and adds the REMAINING net-new backend folder operations (rename/move/delete-to-trash a folder as a unit + folder drag-drop). Chosen "plan as a phase" over "build now" to stop unplanned scope drift on main while Phase 1 UAT is still unsigned.

### Decisions

Decisions are logged in PROJECT.md Key Decisions table.
Recent decisions affecting current work:

- Target = full MVP (SPEC Phases 0–5); SPEC.md is source of truth
- Files-as-truth; SQLite = operational data only (never content)
- Git hidden behind backend; commits automatic; agent writes require explicit user approval
- MVP editor = Markdown-with-preview; TipTap deferred to v2 (gated on round-trip tests)
- [Phase ?]: Pure-Go SCS SQLite session store (avoids cgo sqlite3store; keeps CGO_ENABLED=0)
- [Phase ?]: auth.UserLookup interface breaks the auth<->users import cycle
- [Phase ?]: SPA built into internal/web/dist (Go //go:embed cannot traverse '..')
- [Phase ?]: Phase 0: SEC-01 resolver
- [Phase ?]: Phase 0: single-writer Git via one mutex + exec.Command arg slices; push deferred to Phase 1
- [Phase ?]: Phase 0: jobs.run_after stored as REAL fractional epoch (SQLite datetime truncates to seconds)
- [Phase ?]: Phase 0: RequireRole RBAC reads role only from the session-bound user (never client input); admin is a superset this phase (D-07)
- [Phase ?]: Phase 0: self-service profile/password paths accept no role parameter — own-role escalation impossible by construction (D-06)
- [Phase 0]: SEC-05 audit log is write-only (dual-write SQLite mirror + structured slog line); audit.Record is non-fatal and never records a secret
- [Phase 0]: Resolved LLM API key is unexported + redacted in Config String()/GoString(); read only via Agent.APIKey()
- [Phase 0]: Runtime Docker base is pinned minimal Alpine (ships git for the single-writer Git CLI), not scratch/distroless; still non-root
- [Phase 1 P02]: Optimistic-concurrency revision = `git rev-parse HEAD:<path>` blob SHA (gitstore.BlobRevision); 409 floor checked before enqueue — stale save never silently overwrites
- [Phase 1 P02]: chi page routes use the plain `/pages/*` catch-all (not `{path:.*}` regex) — the regex wildcard mis-routes multi-segment paths when a GET and a PUT share the node
- [Phase 1 P02]: Read mode renders via react-markdown + remark-gfm + rehype-sanitize with the raw-HTML plugin OFF (no innerHTML) — stored-XSS guard per CLAUDE.md
- [Phase 1 P03]: Link rewrite is structural (byte scanner skips fenced/inline code, escapes, external URLs) on okf.Doc.Body, never an AST — code blocks containing link-like text are provably never corrupted (TestRename_NoCorruption)
- [Phase 1 P03]: Rename/move = delete-old + write-new + inbound rewrites staged in ONE commit (D-07); git rename detection keeps `git log --follow` continuous — no `git mv` plumbing needed
- [Phase 1 P03]: One /rename endpoint dispatches new_title→Rename / new_parent→Move (exactly-one-of); mounted on the /pages/* catch-all (handler strips /rename) to avoid the chi sibling-wildcard 405
- [Phase ?]: VER-03 restore is a forward commit through the single-writer path (RestoreVersion); history is never rewritten
- [Phase ?]: VER-02 history hides the SHA in an opaque version token; no Git-named field is serialized to the UI
- [Phase ?]: VER-04 push is config-gated and ff-aware: non-ff sets diverged and alerts, never force/auto-merge
- [Phase ?]: 06-01: CM6 editor swap — CRLF is LF-normalized by CM6 (as a textarea); EDIT-02 toggle invariance asserted against the doc's own bytes, CRLF on-disk round-trip stays a backend gate
- [Phase ?]: 06-01: external value seeds are externalSeed-annotated so they don't echo through onChange (avoids controlled-input feedback loop)
- [Phase ?]: 06-02: live-preview ViewPlugin walks Lezer tree → mark styling + zero-width replace hides; active-line reveal via option (b) skip-on-active-line (no atomicRanges on hides), selection-driven, layout-neutral
- [Phase ?]: 06-02: fenced-code fences kept vs inline backticks hidden by CodeMark parent check; inline-link URL hidden only under a Link; focus ring moved to CSS to keep theme.ts tokens-only
- [Phase ?]: 06-03: GFM table block widget moved to a StateField — CM6 forbids block replace decorations from a ViewPlugin; inline image widgets stay on the plugin; livePreviewExtension bundles both
- [Phase ?]: 06-03: link marks carry data-href (data not action); linkNav routes internal .md via resolveRelativeMdLink scheme gate — no javascript:/data: href reaches navigate (T-06-07)

### Pending Todos

[From .planning/todos/pending/ — ideas captured during sessions]

None yet.

### Blockers/Concerns

[Issues that affect future work]

- Phase 1: byte-stable Markdown round-trip (golden corpus) is the exit gate — spike single-writer Git batching + stale-lock recovery early
- Phase 2: NEEDS RESEARCH — large-binary-in-Git policy + PDF/DOCX extraction fidelity must be resolved before uploads ship
- Phase 4: NEEDS RESEARCH — Eino is pre-1.0; re-verify constructor/tool-schema/interrupt-resume APIs and pin go.sum before building the agent loop

## Deferred Items

Items acknowledged and carried forward from previous milestone close:

| Category | Item | Status | Deferred At |
|----------|------|--------|-------------|
| *(none)* | | | |

## Session Continuity

Last session: 2026-06-21T11:06:56.293Z
Stopped at: Phase 6 planned (4 plans verified); executing
Resume file: .planning/phases/06-live-preview-editor-obsidian-style/06-01-PLAN.md
