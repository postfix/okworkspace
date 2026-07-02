---
gsd_state_version: 1.0
status: milestone closed. Working tree clean; Go 19 pkgs green, web 815 tests green at close.
last_updated: "2026-07-02T14:57:29.489Z"
last_activity: 2026-07-02
progress:
  total_phases: 0
  completed_phases: 0
  total_plans: 0
  completed_plans: 0
  percent: 0
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-07-02)

**Core value:** A non-technical teammate can create, edit, and find knowledge — and get useful AI help
on it — while every byte stays as plain Markdown + original files on disk, versioned in Git, with no
proprietary store to escape.
**Current focus:** Milestone v1.6 **shipped 2026-07-02** (tag-system completion + authoring polish). No
active milestone — the next is **v1.7 Rich-Block Editor (TipTap)**, requirements locked in
`REQUIREMENTS.md` §R, awaiting co-driver kickoff (`/anvil-new-milestone`).

## Current Position

Milestone: v1.6 Tag System Completion & Authoring Polish — **✅ COMPLETE & CLOSED** (built HEAD `4b5c71f`,
closed `248e401`; MILESTONE-AUDIT verdict COMPLETE; security review SHIP; docs + maps refreshed).
Phases: 31–37 all built, qa-verified (7 VERIFICATION.md), and audited against code reality. All 9
requirements (TF-01…05, EP-01…04) met.
Status: milestone closed. Working tree clean; Go 19 pkgs green, web 815 tests green at close.
Next: co-driver decision — start **v1.7 (TipTap)** via `/anvil-new-milestone`, or pick up a deferred
hardening item (global JSON body cap F-02, pin mermaid deps + CI `npm audit` F-01).
Last activity: 2026-07-02

## Accumulated Context

- **Substrate (v1.6 run):** `anvil-cc` CLI live (`state`/`roadmap`/`debt` OK, debt=0). SMTC MCP daemon
  NOT bound in this harness (no `mcp__smtc__*`) — code claims capped at read/grep + test-execution
  (Structural/Empirical), never Decision-tier. Native agent-teams spawn unavailable → all spans
  delegated via the `task` tool (engineer/qa/security-engineer/archaeologist). **For v1.7: warm the
  LSP/SMTC layer at build time and re-probe; cap cert tiers honestly if still cold.**

- **Tag subsystem seam (durable — do NOT fork):** `pages.ApplyTagsBatch` (N pages → 1 byte-stable
  commit, TOCTOU-safe body channel `BodyOld`/`BodyNew`), `SetTagsFrontmatter`, the single-recognizer
  `okf.FindHashtags`/`RewriteHashtags`, the two complementary validators (`ValidateTagCharset` frontmatter
  / `ValidateInlineTag` body-splice — never collapse them). Per-tag counts: `graph.Store.TagPageCounts`
  (`query.go:449`). v1.6's bulk/undo all reuse this seam. See `MAP-tag-rewrite-subsystem.md`.

- **Tag metadata store (v1.6 Phase 32, SHIPPED):** the design fork resolved — store shape is
  **`Tags/{tag}.md` frontmatter** (co-driver-chosen over `.okf-workspace/tags.json`), files-as-truth,
  with a rebuildable `tag_meta` SQLite cache (migrations `0011`–`0013` incl. hierarchy `parent` + the
  op-journal). New package `internal/tagmeta/`. See `MAP-tagmeta-subsystem.md`. `Tags/` is skipped by
  page-tree + graph/search rebuilds.

- **XSS invariant (load-bearing, held across v1.6):** all badge/widget DOM is `createElement` +
  `textContent` — NO `innerHTML` of page content. Tag color is applied via `span.style.color`; the
  EP-01 react-markdown `#tag` badge is a sanitized hast element, NOT raw HTML (`rehype-raw` OFF /
  `rehype-sanitize` ON). Any future editor work (v1.7 TipTap) MUST preserve this.

- **Lesson banked (v1.6 close):** per-phase engineers ran only their OWN new tests; three regressions
  escaped into shared components (`widgets.ts`, `TagManagement.tsx`) and were caught ONLY by the
  architect's close-time full-suite smoke. **A phase touching a shared component must run that
  component's FULL existing suite, not just its new tests.**

- **v1.7 starts warm:** TipTap (rich-block editor, track R, phases 38–41) was **split out** by the
  independent roadmap red-team (SPLIT-REQUIRED, co-driver-accepted). R requirements + all R-track
  red-team findings (BLK-01/02/03 incl. the shared-content Tier-1-normalization fidelity decision,
  MAJ-03/04/05, MIN-02/04) are preserved in `REQUIREMENTS.md` §R. `PageEditor.tsx` (the ~893-line host
  TipTap would refactor) is at `web/src/routes/`, NOT `components/`.

## Previous Milestones

- **v1.6 Tag System Completion & Authoring Polish** — shipped 2026-07-02 (Phases 31–37)
- **v1.5 Tag Regression Fix, Reading-Shell Fold-In & v1.3 Closure** — shipped 2026-07-01 (Phases 28–30)
- **v1.4 Editor Enhancements & Authoring Polish** — shipped 2026-07-01 (Phases 21–27)
- **v1.3 Tag Management UX** — built `9de5077` 2026-06-28; closed retroactively in v1.5
- **v1.2 Graph Schema Extension** — shipped 2026-06-28 (Phase 18)
- **v1.1 Hardening & Polish** — shipped 2026-06-27 (Phases 13–17)
- **v1.0 Knowledge Graph & LLM Auto-Tagging** — shipped 2026-06-24 (Phases 8–12)
- **v0.9.9 MVP** — shipped 2026-06-23 (Phases 0–7)
