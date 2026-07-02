---
gsd_state_version: 1.0
milestone: v1.7
milestone_name: Rich-Block Editor (TipTap)
status: "v1.7 planning complete — RE-01…05 requirements + risk-first roadmap (Phases 38–42) authored, red-team gate run (GO-WITH-REVISIONS, folded). Ready to build; next is discuss/plan Phase 38 (RE-01 codec GO/NO-GO gate)."
last_updated: "2026-07-02T17:30:00.000Z"
last_activity: 2026-07-02
progress:
  total_phases: 5
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
**Current focus:** Milestone **v1.7 Rich-Block Editor (TipTap)** — opt-in TipTap (ProseMirror) editor
beside CM6, behind a two-tier Markdown-fidelity gate; CM6 stays the byte-exact default. Requirements
RE-01…05 + risk-first roadmap (Phases 38–42) authored; independent red-team gate run. Ready to build.

## Current Position

Milestone: v1.7 Rich-Block Editor (TipTap) — **planning complete, ready to build**.
Phases: 38 (RE-01 codec GO/NO-GO gate), 39 (RE-02 verbatim preserved tier), 40 (RE-05 editor-host
abstraction — no-behavior-change refactor), 41 (RE-03 TipTap #tag/suggest/slash + real save),
42 (RE-04 toggle + load guard + code-split; shared-content design fork). None started.
Status: v1.7 planning complete; v1.6 shipped + closed 2026-07-02.
Next: `/anvil-discuss-phase 38` then `/anvil-plan-phase 38`. **Phase 38 is a GO/NO-GO spike** — if the
round-trip corpus can't pass, the milestone halts and returns to the co-driver.
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

- **v1.7 kickoff (this session):** roadmap authored as **5 phases (38–42)** — RE-05 (editor-host
  abstraction) added as its own phase to resolve BLK-01 ordering + isolate the host refactor (MAJ-03).
  Code-reality VERIFIED: `PageEditor.tsx` is **949 lines** at `web/src/routes/` (red-team said ~893 —
  grown); `web/src/stores/editorMode.ts` is a 2-way zustand+persist store (`live|source`, key
  `okf.editor.mode`) to EXTEND to 3-way (BLK-02, do NOT fork); `assertLazyChunks` at
  `web/vite-plugins/`; the `#tag` corpus + `inlineTagsFromBody` in `web/src/lib/cm/hashtag.ts` (RE-03
  must match display-side — no third semantics). **Independent red-team gate (RoadmapRedTeam3) run:
  GO-WITH-REVISIONS**; all findings (MAJ-01…04, MED-05…07, MIN-08…10) FOLDED into `REQUIREMENTS.md`
  (active) + `ROADMAP.md`. **Key correction:** the ED-02b materialization seam ALREADY EXISTS and is
  wired — `reconcileFrontmatterTags` (`web/src/lib/tag-reconcile.ts:51` ← `PageEditor.tsx:21,278`) runs
  `inlineTagsFromBody` (Lezer). RE-03 routes TipTap's serialized body through the SAME `runSaver` — do
  NOT build a second materializer (my earlier grep missed it in `lib/`). **Sharpened by the gate:**
  RE-01's round-trip must ALSO assert cross-engine parity vs Goldmark/Lezer (MAJ-02 — MarkedJS is a 4th
  parser); the `#`/`/` suggestion POPUP is a 2nd untrusted render surface (MAJ-03, textContent-only);
  `marked` is now dual-rooted in the forbidden set (MIN-08). **Design fork deferred to Phase 42
  discuss:** shared-content Tier-1 normalization → `anvil-design-options` + co-driver pause.

## Previous Milestones

- **v1.6 Tag System Completion & Authoring Polish** — shipped 2026-07-02 (Phases 31–37)
- **v1.5 Tag Regression Fix, Reading-Shell Fold-In & v1.3 Closure** — shipped 2026-07-01 (Phases 28–30)
- **v1.4 Editor Enhancements & Authoring Polish** — shipped 2026-07-01 (Phases 21–27)
- **v1.3 Tag Management UX** — built `9de5077` 2026-06-28; closed retroactively in v1.5
- **v1.2 Graph Schema Extension** — shipped 2026-06-28 (Phase 18)
- **v1.1 Hardening & Polish** — shipped 2026-06-27 (Phases 13–17)
- **v1.0 Knowledge Graph & LLM Auto-Tagging** — shipped 2026-06-24 (Phases 8–12)
- **v0.9.9 MVP** — shipped 2026-06-23 (Phases 0–7)
