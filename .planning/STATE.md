---
gsd_state_version: 1.0
milestone: v1.6
milestone_name: Tag System Completion & Authoring Polish
status: Planning complete — roadmap approved (Phases 31–37); ready to start Phase 31
last_updated: "2026-07-02T07:10:00.000Z"
last_activity: 2026-07-02
progress:
  total_phases: 7
  completed_phases: 0
  total_plans: 0
  completed_plans: 0
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-07-02)

**Core value:** A non-technical teammate can create, edit, and find knowledge — and get useful AI help
on it — while every byte stays as plain Markdown + original files on disk, versioned in Git, with no
proprietary store to escape.
**Current focus:** Milestone v1.6 — complete the v1.3-original deferred tag set on the hardened v1.5
subsystem (bulk multi-tag, on-disk tag metadata + hierarchy, tag-op undo, usage analytics) and land four
small v1.4-deferred authoring wins (`#tag` badge in react-markdown, copy-code button, insert-at-cursor
templates, description in search cards). TipTap (rich editor) split to v1.7.

## Current Position

Milestone: v1.6 Tag System Completion & Authoring Polish — **planning complete, ready to build**
Phases: 31 (bulk multi-tag), 32 (tag metadata store), 33 (tag hierarchy), 34 (usage analytics),
35 (tag-op undo), 36 (rendering polish), 37 (insert-at-cursor templates) — all planned, none started
Status: REQUIREMENTS.md + ROADMAP.md authored; independent roadmap red-team run + folded; TipTap
track split to v1.7 (co-driver-accepted). Next: `/anvil-discuss-phase 31` then `/anvil-plan-phase 31`.
Last activity: 2026-07-02

## Accumulated Context

- **Substrate (this session):** `anvil-cc` CLI live (`state`/`roadmap`/`debt` OK, debt=0). SMTC MCP
  daemon NOT bound in this harness (no `mcp__smtc__*`) — code claims cap at read/grep confidence;
  warm the LSP/SMTC layer at build time and cap cert tiers honestly. Native agent-teams spawn NOT
  available — delegation via the `task` tool (same as v1.4/v1.5). Grounding for v1.6 was done via two
  `explore` agents (`agent://TagAdminSeams`, `agent://PolishSeams`) + an independent `plan` red-team
  (`agent://RoadmapRedTeam-2`).
- **Tag subsystem seam (ready to reuse — do NOT fork):** `pages.ApplyTagsBatch` (N pages → 1 byte-stable
  commit, TOCTOU-safe body channel `BodyOld`/`BodyNew`), `SetTagsFrontmatter`, the single-recognizer
  `okf.FindHashtags`/`RewriteHashtags`, the two complementary validators (`ValidateTagCharset` frontmatter
  / `ValidateInlineTag` body-splice — never collapse them). Per-tag counts already exist
  (`graph.Store.TagPageCounts`, `query.go:449`). See `MAP-tag-rewrite-subsystem.md`.
- **Tag-track design fork (Phase 32, front-loaded):** tag metadata/hierarchy have NO on-disk source
  today (`page_tags` migration 0009 rebuilds from frontmatter only; no `tag_meta`/`tags.json`). Phase 32
  must invent the disk format + a cache table + migration + rebuild harness. Store shape
  (`.okf-workspace/tags.json` vs `Tags/*.md`) decided in Phase-32 discuss-phase. Files-as-truth: metadata
  MUST be on-disk + committed; SQLite only a rebuildable cache.
- **Editor-polish reality (all four premises confirmed TRUE):** `#tag` badges render in CM6 read/edit
  (built) but NOT in the two `MarkdownProse` react-markdown surfaces (`HistoryPanel` preview, `AgentAnswer`)
  — EP-01 must be a remark→sanitized-element transform (rehype-raw OFF / rehype-sanitize ON preserved).
  Copy-code button absent (`CodeBlockWidget`, `widgets.ts`). Insert-at-cursor templates absent (Phase-27
  templates are new-page-seed only; reuse `Templates/` + `listTemplates`). Search cards do NOT show
  description (not indexed, not in `search.Result`, not rendered).
- **Shared-file merge coordination (red-team MAJ-02 — the "fully independent" claim was corrected):**
  Phase 32 (tag color) and Phase 36 (copy-code) BOTH touch `web/src/lib/cm/widgets.ts` + `theme.ts` —
  sequence 32 before 36; color via class/style attribute only (no `innerHTML`, preserve the XSS invariant).
  `PageEditor.tsx` is at `web/src/routes/`, NOT `components/`.
- **Red-team verdict folded:** SPLIT-REQUIRED → TipTap to v1.7 (accepted). All T+E-applicable findings
  (TF-02 phase weight, TF-05 dedicated op-journal + latent dep on 32, MAJ-02 shared files, path fixes,
  per-feature verification of Phase 36) are folded into ROADMAP.md + REQUIREMENTS.md. The R-track findings
  (BLK-01/02/03, shared-content fidelity decision, MAJ-03/04/05, MIN-02/04) are preserved in
  REQUIREMENTS.md §R for v1.7.

## Previous Milestones

- **v1.5 Tag Regression Fix, Reading-Shell Fold-In & v1.3 Closure** — shipped 2026-07-01 (Phases 28–30)
- **v1.4 Editor Enhancements & Authoring Polish** — shipped 2026-07-01 (Phases 21–27)
- **v1.3 Tag Management UX** — built `9de5077` 2026-06-28; closed retroactively in v1.5
- **v1.2 Graph Schema Extension** — shipped 2026-06-28 (Phase 18)
- **v1.1 Hardening & Polish** — shipped 2026-06-27 (Phases 13–17)
- **v1.0 Knowledge Graph & LLM Auto-Tagging** — shipped 2026-06-24 (Phases 8–12)
- **v0.9.9 MVP** — shipped 2026-06-23 (Phases 0–7)
