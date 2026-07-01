---
gsd_state_version: 1.0
milestone: v1.5
milestone_name: Tag Regression Fix, Reading-Shell Fold-In & v1.3 Closure
status: v1.5 COMPLETE — shipped 2026-07-01 (Phases 28–30)
last_updated: "2026-07-01T20:30:00.000Z"
last_activity: 2026-07-01
progress:
  total_phases: 3
  completed_phases: 3
  total_plans: 3
  completed_plans: 3
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-07-01)

**Core value:** A non-technical teammate can create, edit, and find knowledge — and get useful AI help on it — while every byte stays as plain Markdown + original files on disk, versioned in Git, with no proprietary store to escape.
**Current focus:** Milestone v1.5 — assess-and-fix: fix the confirmed tag rename/merge/delete regression (v1.4's inline-`#tag` materialization re-adds a renamed-away tag on the next client save) server-authoritatively via `okf.RewriteHashtags`; fold in the uncommitted Obsidian-parity reading-shell refactor; formally close the already-shipped-but-unverified v1.3 Tag Management.

## Current Position

Milestone: v1.5 Tag Regression Fix, Reading-Shell Fold-In & v1.3 Closure — **COMPLETE (shipped 2026-07-01)**
Phases: 28 (reading-shell fold-in), 29 (tag-regression fix), 30 (v1.3 closure) — all verified + closed
Status: v1.5 complete; MILESTONE-AUDIT COMPLETE (milestones/v1.5-MILESTONE-AUDIT.md); security review SHIP (v1.5-SECURITY-REVIEW.md); ready for the next milestone
Last activity: 2026-07-01

## Accumulated Context

- **Uncommitted working tree (folds into v1.5 Phase 28):** a large Obsidian-parity reading-shell
  refactor sits on top of the v1.4-complete commit (`95ca67d`) — 44 tracked + ~20 untracked files.
  Coherent + green (`go test`, `tsc -b`, `vitest 789/62` all pass). Adds PropertiesPanel / Breadcrumb /
  DocStatusBar / NavSearchResults; deletes TagBadgeBar, per-page LocalGraphPanel/DepthControl,
  SearchPalette. **Test gap:** NavSearchResults + navSearch store are untested (196 lines of
  SearchPalette tests were dropped). CM6 selection/coordinate change (`drawSelection`+`lineWrapping`+
  margin→padding) is manual-QA-only.
- **Confirmed regression:** server tag rename/merge/delete rewrite frontmatter `tags:` only
  (`ApplyTagsBatch` passes `doc.Body` through verbatim, batch.go:133-137); client
  `reconcileFrontmatterTags` re-unions `inlineTagsFromBody(body)` on save (tag-reconcile.ts:78) →
  a renamed-away `#tag` still in a page body reappears on next save. Delete fully reverts.
- **Fix (ratified):** server-authoritative body rewrite via new `okf.RewriteHashtags`, modeled on the
  existing code-region-aware `okf.RewriteLinks`/`FindLinks` scanner (links.go:28-83,349); pinned to the
  TS Lezer grammar (`web/src/lib/cm/hashtag.ts` + `hashtag.test.ts` corpus) by a shared parity test to
  prevent the Phase-21-warned second-parser drift. Enumeration must be body-inclusive (`PagesByTag` /
  `page_tags` is frontmatter-only — a body-only `#tag` is invisible to it).
- **v1.3 closure gap:** Tag Management shipped in commit `9de5077` (2026-06-28) with no phase 19/20 dirs,
  no independent qa verification, no security review, no milestone audit. v1.5 backfills all three.
- **Substrate:** SMTC MCP daemon OFFLINE this session — code claims are read/grep confidence, cert tiers
  capped at Structural/Honest. anvil-cc sidecar OK (`debt=0`). Native agent-teams spawn unavailable in
  this harness; delegation via the `task` tool.

## Previous Milestones

- **v1.4 Editor Enhancements & Authoring Polish** — shipped 2026-07-01 (Phases 21–27)
- **v1.3 Tag Management UX** — built `9de5077` 2026-06-28 (backend + admin UI); closed retroactively in v1.5
- **v1.2 Graph Schema Extension** — shipped 2026-06-28 (Phase 18)
- **v1.1 Hardening & Polish** — shipped 2026-06-27 (5 phases)
- **v1.0 Knowledge Graph & LLM Auto-Tagging** — shipped 2026-06-24 (5 phases)
- **v0.9.9 MVP** — shipped 2026-06-23 (8 phases)
