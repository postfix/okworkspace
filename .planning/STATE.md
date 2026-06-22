---
gsd_state_version: 1.0
milestone: v0.9.9
milestone_name: milestone
current_phase: 05
current_phase_name: collaboration
status: executing
stopped_at: Completed 05-04-PLAN.md (conflict resolution + force-edit safety proof, COLL-03/COLL-04)
last_updated: "2026-06-22T12:50:58.701Z"
last_activity: 2026-06-22
last_activity_desc: Completed 05-04-PLAN.md (conflict resolution + force-edit safety proof, COLL-03/COLL-04)
progress:
  total_phases: 8
  completed_phases: 8
  total_plans: 36
  completed_plans: 36
  percent: 100
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-06-17)

**Core value:** A non-technical teammate can create, edit, and find knowledge — and get useful AI help on it — while every byte stays as plain Markdown + original files on disk, versioned in Git, with no proprietary store to escape.
**Current focus:** Phase 05 — collaboration

## Current Position

Phase: 05 (collaboration) — ALL 4 PLANS COMPLETE
Plan: 4 of 4 complete (wave 4 / 05-04 done)
Status: Phase 5 implementation complete — ready for phase verification
Last activity: 2026-06-22 — Completed 05-04-PLAN.md (conflict resolution + force-edit safety proof, COLL-03/COLL-04)

Progress: [██████████] 100% (4 of 4 plans)

### Key decisions (05-04 — the load-bearing slice)

- COLL-03 PROVEN: force-edit is lock-only; a stale forced save still 409s (`TestForceEditStillRejectsStaleSave`). `pages.Service.Save` is UNCHANGED — the 409 floor (service.go:200) is reused, not modified.
- Save-as-copy = `Create(deduped path)` + `Save(newPath, fresh rev)`; the original is never written and never carries the conflicted base revision (`TestSaveAsCopyLeavesOriginal`).
- A 409 (explicit Save OR autosave) opens `DiffReviewDialog` conflict mode (old=server, new=mine) with three risk-ranked choices (Overwrite / Manual merge / Save as copy); autosave is gated while the dialog is open; the safe choice owns initial focus, never Overwrite. Conflict UI is additive over the 05-02 lock + 05-03 presence wiring.
- Self-check: backend build/vet/test green (incl. both new tests); frontend vitest 289/289; tsc clean; no user-facing Git vocabulary in conflict copy.

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
| Phase 06 P04 | 6min | 3 tasks | 8 files |
| Phase 07 P01 | 18min | 3 tasks | 7 files |
| Phase 07 P02 | 7min | 3 tasks | 8 files |
| Phase 07 P03 | 13min | 3 tasks | 7 files |
| Phase 04 P01 | 9min | 3 tasks | 7 files |
| Phase 04 P02 | 14 | 3 tasks | 11 files |
| Phase 04 P03 | 18 | 2 tasks | 6 files |
| Phase 04 P04 | 7 | 4 tasks | 7 files |
| Phase 04 P05 | 5min | 3 tasks | 7 files |
| Phase 04 P06 | 9min | 3 tasks | 17 files |
| Phase 04 P07 | 22min | 3 tasks | 9 files |
| Phase 05 P01 | 20min | 4 tasks | 6 files |
| Phase 05 P05-03 | ~10min | 3 tasks | 7 files |
| Phase 05 P04 | 25min | 4 tasks | 9 files |

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
- [Phase ?]: 06-04: read mode unified onto a read-only LivePreviewEditor (EditorState.readOnly + EditorView.editable.of(false), compartment forced to liveExtensions) — pixel-identical to edit Live, single decoration pipeline (no second weaker read renderer); @uiw/react-md-editor removed
- [Phase ?]: 06-04: heading anchors via a whole-document StateField (Decoration.line id == github-slugger slug, deduped matching okf.ScanHeadings, never user-content-prefixed) + scrollToHash-on-mount preserves SRCH-06; location.hash used only as a lookup key (T-06-11/T-06-12)
- [Phase ?]: 06-04: MarkdownProse retained on disk (not deleted) — HistoryPanel still uses it for old-version preview; retired only from the PageView read path
- [Phase 7 P01]: Folder rename/move is ONE atomic commit — relocateFolder relocates index.md + every dir/ descendant + rewrites all inbound links together (TREE-02), lifting Phase-1 single-page relocate to a folder batch
- [Phase 7 P01]: Folder collision REJECTS (ErrFolderExists → HTTP 409, UI-SPEC copy) before any disk write — folders never auto-suffix/merge (TREE-06); unlike pages which auto-suffix via uniqueExactPath
- [Phase 7 P01]: Moved-page bytes are written VERBATIM (never re-emitted through okf.Emit) — byte-stability preserved; only genuine inbound links re-emitted
- [Phase 7 P01]: New okf.RewriteLinksMoved (resolveDir for matching, emitDir for recomputation) + unified single-pass rewriteFolderInboundLinks fixes cross-linked moving siblings and eliminates Pitfall 1 double-staging by construction (each page keyed once by final path)
- [Phase 7 P01]: Folder rename/move share the /pages/* POST catch-all by suffix (/rename-folder, /move-folder) — same sibling-wildcard avoidance as Phase 1 /rename; editor-gated from session role; new_parent re-validated via cleanPathString (WR-08)
- [Phase ?]: 07-02 grouped folder delete/restore loops the existing per-page Delete/Restore over descendantPages under one crypto/rand delete_group_id (nullable migration 0008); per-page-looped per the RESOLVED atomicity decision, partial progress recoverable; solo delete stays NULL; all group-id SQL parameterized
- [Phase ?]: 07-03: Clean tree-UX rebuild guarded by a regression-net-first contract — treeBehaviors.test.tsx pins every shipped LeftTree/TreeContextMenu/RenameModal/MoveDialog behavior GREEN against the un-rebuilt code, then the rebuild keeps it GREEN (no user-visible regression)
- [Phase ?]: 07-03: RenameModal/MoveDialog parameterized by NodeKind with a per-kind copy map; the folder branch (renameFolder/moveFolder + 409 collision copy) is implemented but UNREACHED from LeftTree until Plan 04; MoveDialog folder kind excludes self+subtree from destinations
- [Phase 7 P04]: Optimistic ["tree"] updates via useTreeMutations — onMutate cancel+snapshot+applyMove → onError rollback → onSettled invalidate; pure applyMove does the literal prefix swap identical to server relocateFolder (Pitfall 6) so the reconcile refetch never jumps the node
- [Phase 7 P04]: Folder DnD drop-validity guard runs DURING dragover by reading the dragged path from a module-level activeDragPath ref (HTML5 dataTransfer.getData() returns '' mid-drag); invalid drops (self/descendant/same-parent) skip preventDefault → native cursor:not-allowed, no highlight (TREE-06 client)
- [Phase 7 P04]: Grouped TrashView folds ["trash"] by delete_group_id into one "Folder '{name}' · N pages" row per group (folder name from common-ancestor index.md path) with a Restore folder action calling restoreFolderGroup; batched (restored)-suffix notice on collision (TREE-05 UI)
- [Phase ?]: Pinned eino-ext openai as real semver v0.1.13 (not pseudo-version); anchored go-udiff via blank import so the pin survives go mod tidy.
- [Phase ?]: Agent read-only tool boundary is build-gated by a set-equality test (D5/AGNT-11); a 6th/mutating tool fails the build. Apply is a non-tool HTTP endpoint.
- [Phase ?]: Propose-patch returns the FULL new body + base revision captured at proposal time; the server diffs old↔new (go-udiff for the churn metric only) — never a prose summary (AGNT-09).
- [Phase ?]: Apply is a NON-tool editor-gated CSRF HTTP endpoint reusing pages.Save(baseRevision) → single-writer gitstore.Commit(approved_agent_patch); a moved revision 409s, never a silent overwrite (AGNT-10/11).
- [Phase ?]: agentContext store is ephemeral (not persisted) — carries transient selection/attachment, unlike persisted UI-preference stores
- [Phase ?]: Rewrite apply reuses applyPatch (selection span spliced into cached body); no new write endpoint; missing-span/stale revision 409s into the dialog stale state
- [Phase ?]: Lock paths use two-layer guard: repo.Resolve guards the repo root; lockPath guards the .okf-workspace/locks/ subtree (ErrUnsafePagePath)
- [Phase ?]: Force is lock-file-only, decoupled from save authority (pages.Save); lockExpiry=2m, GC interval=60s
- [Phase 05]: COLL-03 proven — force-edit is lock-only; a stale forced save still 409s (TestForceEditStillRejectsStaleSave). pages.Service.Save is UNCHANGED — the 409 floor is reused, not modified.
- [Phase 05]: Save-as-copy = Create(deduped path) + Save(newPath, fresh rev); the original is never written and never carries the conflicted base revision (TestSaveAsCopyLeavesOriginal).
- [Phase 05]: A 409 (explicit OR autosave) opens DiffReviewDialog conflict mode (old=server, new=mine); autosave is gated while open; safe choice owns initial focus, never Overwrite; conflict UI is additive over 05-02 lock + 05-03 presence.

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

Last session: 2026-06-22T12:50:07.879Z
Stopped at: Completed 05-04-PLAN.md (conflict resolution + force-edit safety proof, COLL-03/COLL-04)
Resume file: None
