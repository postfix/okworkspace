---
phase: 06-live-preview-editor-obsidian-style
plan: 02
subsystem: ui
tags: [codemirror6, lezer, markdown, decorations, viewplugin, live-preview, vitest]

# Dependency graph
requires:
  - phase: 06-live-preview-editor-obsidian-style
    plan: 01
    provides: LivePreviewEditor surface, modeCompartment + liveExtensions seam (mode.ts), GFM-only markdown() config, token-only theme seam, RED-pending livePreview.test.ts stub
provides:
  - "live-preview ViewPlugin (web/src/lib/cm/livePreview.ts): walks syntaxTree over visibleRanges → DecorationSet (mark styling + zero-width replace hides) with active-line marker reveal"
  - "Text-construct inline rendering in Live mode: headings (cm-heading-1..6), bold (cm-strong), italic (cm-em), inline code (cm-inline-code), fenced code (cm-code-block), links (cm-md-link)"
  - "Obsidian-style active-line reveal: hides skipped on lines the selection touches (option (b)), layout-neutral, selection-driven"
  - "theme.ts rendering the decoration classes at UI-SPEC token values, visually matching MarkdownProse.css (read mode)"
affects: [06-03-image-table-widgets, 06-04-unified-read-mode]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "live-preview ViewPlugin.fromClass: rebuild DecorationSet on docChanged||viewportChanged||selectionSet; decorations provided via { decorations: v => v.decorations }"
    - "Active-line reveal via option (b): compute selection-touched line numbers up-front and SKIP emitting hideMark on those lines (cleaner than a post-filter; avoids the cursor-trap footgun)"
    - "Zero-width Decoration.replace({}) hides + class-only Decoration.mark styling → layout-neutral marker reveal (no atomicRanges on hides)"
    - "CM6 EditorView.theme reads only tokens.css vars; focus-ring (literal 2px, no token exists) lives in the per-component CSS per the existing project convention"

key-files:
  created:
    - web/src/lib/cm/livePreview.ts
  modified:
    - web/src/lib/cm/livePreview.test.ts
    - web/src/lib/cm/mode.ts
    - web/src/lib/cm/theme.ts
    - web/src/components/LivePreviewEditor.css

key-decisions:
  - "Active-line reveal uses RESEARCH option (b): skip emitting hides on selection-touched lines rather than emit-then-filter — cleaner, and never traps the cursor (no atomicRanges on hides, Pitfall 1)"
  - "Fenced-code CodeMark fences are KEPT (not hidden) — distinguished from inline-code backticks by checking the CodeMark's parent node name (FencedCode)"
  - "Inline-link URL hidden only when its parent is a Link (so reference-style/autolink URLs are untouched); LinkMark brackets hidden generally"
  - "Heading rendered as a Decoration.line (cm-heading-N) on the line + a hide of the leading '# ' run (HeaderMark + one trailing space), so reveal is layout-neutral (a heading line stays a heading line whether or not its marker shows)"
  - "Focus ring relocated from the JS theme object to LivePreviewEditor.css so theme.ts is provably px/hex-free (the plan's tokens-only verify greps theme.ts); the literal 2px ring matches tokens.css/controls.css/SearchPalette.css convention"
  - "ListMark NOT hidden in v1 (RESEARCH Open Q3) — list bullets stay visible/styled"

patterns-established:
  - "Lezer node-name → decoration mapping isolated in buildDecorations(); the plugin issues no document transactions and never serializes the tree back to text (view-only invariant, T-06-04/T-06-05)"

requirements-completed: [EDIT-01]

# Metrics
duration: 12min
completed: 2026-06-21
---

# Phase 6 Plan 02: Live-Preview Decorations (Text Constructs) Summary

**A CodeMirror 6 live-preview ViewPlugin that walks the Lezer markdown tree over the visible ranges and renders headings/bold/italic/inline-code/fenced-code/links inline as you type — hiding the syntax markers everywhere except the active line (Obsidian-style, layout-neutral reveal), with the decoration classes themed to match read mode byte-for-byte.**

## Performance

- **Duration:** ~12 min
- **Completed:** 2026-06-21
- **Tasks:** 2
- **Files modified:** 5 (1 created, 4 modified)

## Accomplishments
- **EDIT-01 for text constructs lands.** `web/src/lib/cm/livePreview.ts` is the core ViewPlugin: it iterates `syntaxTree(view.state)` over each `view.visibleRanges` segment and pushes `Decoration.mark` (styling) + zero-width `Decoration.replace({})` (marker hides) + `Decoration.line` (per-level heading style), then is wired into `liveExtensions` so Live mode now renders inline while Source mode stays decoration-free.
- **Obsidian-style active-line reveal.** Implemented via RESEARCH option (b): the build computes the set of line numbers the selection touches and simply skips emitting hides on those lines, so the raw markers reveal for editing on the cursor's line while styling marks stay. No `atomicRanges` on hides (Pitfall 1 cursor trap avoided); reveal is selection-driven via `update.selectionSet`, never timed.
- **Layout-neutral by construction.** Hides are zero-width and the mark/line classes never change line-height (bold = weight only), so entering/leaving a line never reflows neighbours (Pitfall 2 — a hard UI-SPEC constraint).
- **Visual parity with read mode.** `theme.ts` styles `cm-strong`/`cm-em`/`cm-inline-code`/`cm-code-block`/`cm-heading-1..6`/`cm-md-link` at the exact UI-SPEC token values mirroring `MarkdownProse.css`, referencing only `var(--…)` tokens.
- **Test gate flipped RED→GREEN.** The Wave-0 `livePreview.test.ts` stub (10 `it.todo`) is now 9 real assertions GREEN (bold/heading/link/inline-code/fenced-code decoration emission + active-line reveal + inactive-line hides retained + view-only/no-mutation sanity), with the image/table WIDGET assertions kept as `it.todo` for 06-03.

## Task Commits

1. **Task 1: live-preview ViewPlugin (tree walk → decorations + active-line reveal)** — `31280a1` (feat)
2. **Task 2: theme + LivePreviewEditor.css visual parity (layout-neutral)** — `e5aeff5` (feat)

## Files Created/Modified
- `web/src/lib/cm/livePreview.ts` — **created.** The ViewPlugin: `buildDecorations()` walks the Lezer tree over visibleRanges, mapping node names (`StrongEmphasis`→cm-strong, `Emphasis`→cm-em, `InlineCode`→cm-inline-code, `FencedCode`→cm-code-block, `Link`→cm-md-link, `ATXHeading1..6`→cm-heading-N line + HeaderMark hide; `EmphasisMark`/`LinkMark`/inline `CodeMark`/inline `URL` hidden), with active-line reveal. View-only — no doc transactions.
- `web/src/lib/cm/livePreview.test.ts` — **modified.** Fleshed the RED stub to GREEN: mounts headless EditorViews, flattens the plugin's DecorationSet, and asserts the expected kinds/classes per construct + that hides drop on the active line while inactive lines keep theirs. Image/table stay `it.todo`.
- `web/src/lib/cm/mode.ts` — **modified.** `liveExtensions` now includes `livePreview` (between `markdownExtension` and `theme`). `setMode`'s effects-only contract unchanged; `sourceExtensions` stays decoration-free.
- `web/src/lib/cm/theme.ts` — **modified.** Replaced the obsolete `.cm-md-code/.cm-md-codeblock` rules with the real decoration classes themed at UI-SPEC token values; moved the focus ring out so theme.ts is px/hex-free.
- `web/src/components/LivePreviewEditor.css` — **modified.** Centred the prose content column; added the accent focus ring (project convention, literal 2px) replacing the prior `outline: none`.

## Decisions Made
- **Reveal strategy = option (b) (skip-on-active-line), not emit-then-filter.** RESEARCH flagged the post-filter as a cursor-trap footgun; computing active lines up-front and not emitting their hides is cleaner and provably trap-free.
- **Fenced vs inline code disambiguation by parent node.** A `CodeMark` under a `FencedCode` parent is a fence and is KEPT (styled); a `CodeMark` elsewhere is an inline-code backtick and is hidden. Same parent-check guards inline-link `URL` hiding (only under a `Link`), so reference/autolink URLs are untouched.
- **Heading as line decoration + leading-run hide.** The heading line gets `cm-heading-N` (sizes the whole line) and the `# ` run is hidden; the line is a heading line regardless of reveal state, so neighbours never reflow.
- **Focus-ring relocation for the tokens-only gate.** The plan's verify greps `theme.ts` for any `px`/hex. The accent focus ring is, by project convention (tokens.css `:focus-visible`, controls.css, SearchPalette.css), a literal `2px` outline in CSS with no width token. Moving it from the JS theme object to `LivePreviewEditor.css` keeps theme.ts a pure token reference while preserving the convention — see Deviations.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] theme.ts tokens-only verify failed on the pre-existing focus-ring `2px` literals**
- **Found during:** Task 2 (the plan's `node -e` grep on theme.ts for `#hex`/`\d+px`)
- **Issue:** theme.ts carried `outline: "2px solid var(--color-accent)"` + `outlineOffset: "2px"` (shipped in 06-01). The plan's tokens-only gate flags any `px`, so the verify failed even though `2px` focus rings are the established project convention (tokens.css `:focus-visible`, controls.css, SearchPalette.css all use the literal — there is no ring-width token).
- **Fix:** Moved the focus ring out of the JS theme object into `LivePreviewEditor.css` (`.cm-editor.cm-focused { outline: 2px solid var(--color-accent); outline-offset: 2px }`), replacing the prior `outline: none`. theme.ts is now provably px/hex-free; the ring still renders, now in the file where the project's focus-ring convention already lives.
- **Files modified:** web/src/lib/cm/theme.ts, web/src/components/LivePreviewEditor.css
- **Verification:** theme.ts grep "tokens-only ok"; tsc clean; full `npm test` GREEN.
- **Committed in:** e5aeff5 (Task 2 commit)

**2. [Rule 1 - Bug] theme.ts grep still matched `2px` inside a code comment**
- **Found during:** Task 2 (re-running the verify after fix 1)
- **Issue:** A comment in theme.ts explaining the focus-ring relocation contained the literal "2px", which the (comment-blind) regex still flagged.
- **Fix:** Reworded the comment to drop the literal ("a literal accent outline" instead of "a literal 2px outline").
- **Files modified:** web/src/lib/cm/theme.ts
- **Verification:** theme.ts grep "tokens-only ok".
- **Committed in:** e5aeff5 (Task 2 commit)

---

**Total deviations:** 2 auto-fixed (1 blocking, 1 bug). Both are gate-compliance fixes within Task 2's scope; no scope creep, no architectural change, no behavior change.

## Threat Model Compliance
- **T-06-04 (Tampering — decorations over untrusted content):** Mitigated. The plugin emits only `Decoration.mark`/`Decoration.replace({})`/`Decoration.line` — no `innerHTML`, no DOM built from page bytes (widgets, the only content→DOM path, land in 06-03 with the sanitizer). The doc is never re-emitted from the tree.
- **T-06-05 (Tampering — byte integrity under decoration recompute):** Mitigated. `buildDecorations` only reads `syntaxTree(view.state)` and returns a DecorationSet; it issues no document transactions. `mode.test.ts` continues to assert toggle byte-stability (GREEN). The "view-only" test asserts `view.state.doc.toString()` is unchanged.
- **T-06-SC (npm installs):** Accepted — no new packages this plan (all added in 06-01).

## Known Stubs
- `livePreview.test.ts` retains 2 `it.todo` (image → ImageWidget; GFM table → block widget grid) — these are 06-03's WIDGET scope by design, not unmet 06-02 goals. The 9 text-construct assertions are GREEN.
- `liveExtensions` does not yet include image/table widgets or link-click navigation — those land in 06-03 (CONTEXT/PLAN-scoped to this wave's text constructs only).

## Issues Encountered
- None beyond the two gate-compliance deviations above.

## Next Phase Readiness
- **06-03 (image/table widgets):** extends `liveExtensions` / `livePreview.ts` with `Decoration.replace({ widget })` for `Image` and a block widget for `Table`; the `sanitizeImageSrc` gate (06-01) is ready to call before mounting `<img>`. The 2 `it.todo` in livePreview.test.ts are the waiting RED gates. Active-line reveal already accounts for widgets if they reuse the same skip-on-active-line path.
- **06-04 (unified read mode):** the themed decoration classes are the shared visual surface; a read-only LivePreviewEditor config can reuse `liveExtensions` + `theme`.

## Self-Check: PASSED

- `web/src/lib/cm/livePreview.ts` exists on disk (created).
- Both task commits present in git history: `31280a1`, `e5aeff5`.
- Full `cd web && npm test` GREEN (166 passed, 2 todo across 22 files); `npx tsc --noEmit` clean (exit 0); eslint clean on touched .ts files; theme.ts tokens-only grep passes.

---
*Phase: 06-live-preview-editor-obsidian-style*
*Completed: 2026-06-21*
