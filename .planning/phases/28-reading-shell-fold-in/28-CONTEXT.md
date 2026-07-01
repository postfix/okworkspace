---
author: architect
responsible: architect
phase: 28
phase_type: implementation
status: Ready for planning
hard_bar: true
security_relevant: false
design_fork: false
parent_artifacts:
  - .planning/ROADMAP.md
  - .planning/REQUIREMENTS.md
---

# Phase 28: Reading-Shell Fold-In - Context

**Gathered:** 2026-07-01
**Status:** Ready for planning

<domain>
## Phase Boundary

Commit the large uncommitted Obsidian-parity reading-shell refactor (~43 tracked
+438/−2387, plus new untracked components) as a coherent, **verified-before-committed**
baseline; close the one material test-coverage gap (the inline nav-search redesign);
document the two intentional scope reductions (in-place tree-filter removal; per-page
local-graph removal); and confirm the manual-QA-only CM6 selection/coordinate change in
a real browser. After this phase, subsequent work builds on committed HEAD, not a
working-tree blob.

**Fold, don't re-design.** The Obsidian-parity direction, local-graph removal, and
search redesign are accepted as-is. This phase stabilizes, tests, documents, and commits
them — it does NOT re-litigate the design. New capabilities belong in other phases.

**Non-goals:** re-designing the refactor; adding new reading-shell/search features;
touching the Phase-29 tag write surface (`handlers_tags_admin.go`, `internal/pages/batch.go`,
`internal/okf/*`, `web/src/lib/tag-reconcile.ts`).
</domain>

<decisions>
## Implementation Decisions

### Commit slicing (RS-01, criterion 2)
- **D-01:** The refactor lands as **4 commits**, in this order, each a coherent
  independently-revertable unit:
  1. **reading-shell** — `Breadcrumb` + `PropertiesPanel` + `DocStatusBar` (+ their CSS/tests)
     and the `TagBadgeBar` removal; the reading column becomes a document.
  2. **inline nav-search** — `NavSearchResults` + `stores/navSearch.ts` (+ CSS/tests) and the
     `SearchPalette` + `store/searchStore.ts` + `stores/treeFilter.ts` removal.
  3. **local-graph removal (end-to-end)** — Go (`internal/graph/query.go`,
     `handlers_graph.go`, `router.go`, tests) + TS (`api/client.ts`, `GraphView`,
     `LocalGraphPanel`/`DepthControl`, `stores/localGraphPanel.ts`, tests) in ONE commit.
  4. **CM6 coordinate fix** — `web/src/lib/cm/theme.ts` (margin→padding) +
     `LivePreviewEditor.tsx` (`drawSelection` + `lineWrapping`) as its OWN
     independently-revertable commit (hard requirement — this is the manual-QA-only change).
- **D-02:** **Verify BEFORE committing (enforced order):** (a) confirm suites green,
  (b) backfill the nav-search tests, (c) run the RS-04 browser smoke — THEN commit. No
  unverified code enters the baseline. This is the exact "committed-then-breaks" pattern
  the milestone exists to fix (v1.3 did precisely that).

### Working-tree hygiene (contamination outside the refactor)
- **D-03:** The 8 untracked `.smtc/memories/*.md` (prior-phase engineer/qa/security KB
  notes) + the modified `.planning/phases/26-mermaid-diagrams/26-VERIFICATION.md` are
  committed in a **separate housekeeping commit** (e.g. `chore: fold in prior-phase KB
  notes + phase-26 verification`), kept OUT of the four refactor commits so those stay
  pure.
- **D-04:** The deleted `internal/web/dist/.gitkeep` is **restored** — it is the
  build-output directory keeper; its deletion looks accidental, not part of the refactor.

### RS-04 browser smoke execution
- **D-05:** **The agent drives a real Chromium** (via the browser tool) against a running
  `vite dev` + Go backend through the RS-04 scenarios (drag-select + copy across
  bold/heading/link/code; caret placement around headings + on wrapped lines), captures
  screenshots into the phase dir, and records results in the SUMMARY. Any construct
  headless Chromium cannot faithfully exercise (true human-mouse drag-select drift) is
  **flagged explicitly** for the user's spot-check rather than silently claimed.

### Search test-backfill depth (RS-02)
- **D-06:** `SearchResultRow` is **covered transitively** through `NavSearchResults` tests
  (rows render, click fires `openResult`) — no standalone `SearchResultRow.test.tsx`. Its
  logic-heavy part (`highlight.ts`) is already unit-tested; a dedicated file would restate
  markup.
- **D-07:** The `⌘K focuses the navrail search input` assertion lives in a **new
  AppShell-focused test** (the handler is in `AppShell.tsx:558`, not in NavSearchResults) —
  test it where it lives. NavSearchResults tests stay about results rendering/navigation.
- **D-08:** `NavSearchResults` tests cover: Pages/Headings/Attachments grouping (empty
  groups omitted), heading navigate-and-scroll (rehype-slug id, leading-`#` stripped) with
  query-clear, and loading / error / empty states. `stores/navSearch.ts` tests cover
  query set/clear (tree-returns-when-empty). Follow the existing store-test pattern
  (`web/src/stores/*.test.ts`).

### Scope-reduction docs placement (RS-03)
- **D-09:** The two removals fold into **existing** architecture docs (matches the current
  per-subsystem layout): the tree-filter→search change → `docs/architecture/overview.md`
  (nav/reading-shell section); the per-page local-graph removal →
  `docs/architecture/graph-subsystem.md` (retained global `/app/graph`, removed
  `/graph/local` + `Neighborhood` + `LocalGraphPanel`/`DepthControl`). Plus a Key Decisions
  entry each in `.planning/PROJECT.md`.
- **D-10:** Each documented removal **records the removal + rationale AND points to the
  Future re-introduction path** (REQUIREMENTS.md Future section) — reads as an intentional,
  revisitable decision, exactly RS-03's intent ("decisions, not accidental loss").

### Stale references to correct (RS-03)
- **D-11:** `web/src/routes/AppShell.tsx:752` comment "Search now lives in the top bar" is
  stale (search is in the navrail) — correct it. `web/src/routes/PageEditor.test.tsx:508,536`
  test descriptions still name "TagBadgeBar" — update to reflect `PropertiesPanel`. (The
  `TagBadgeBar`/`tag-badge` occurrences inside `PropertiesPanel.tsx/.css` are **legit CSS
  class names**, deliberately reused for visual parity — do NOT touch them.)

### The agent's Discretion
- The exact commit *messages* (feature-scoped, conventional) and the precise ordering of
  file groups within a commit.
- Test file structure/naming within the established vitest + testing-library conventions.
- Screenshot naming/layout under the phase dir for the smoke evidence.
</decisions>

<disciplines>
## Lenses surfaced for this phase

> **Lenses, not rules.** Each entry below is a perspective the architect surfaced for this phase. Downstream agents engage with these lenses: adopting, refining, or reasoning past them. Divergence-with-reasoning is a successful interaction.

~~~yaml
disciplines:
  - section: "inline"
    name: "fold-not-redesign"
    scope: ["all refactor files", "web/src/components/**", "internal/graph/**"]
    guidance: "Stabilize, test, document, and commit the refactor AS-IS. Do not
      re-architect, rename, or 'improve' the folded code while committing it — the
      design direction (Obsidian-parity, local-graph removal, search redesign) is
      accepted and out of scope for re-litigation."
    rationale: "The milestone's value here is a clean, trustworthy baseline for the
      Phase-29 surgical commits. Design churn mixed into the fold-in would defeat the
      isolation the whole milestone ordering exists to create."
    override_signals:
      - "Stabilization uncovers a genuine defect in the folded code (a broken tag
        union, a failing/flaky suite, a dangling ref the grep-clean claim missed) —
        fixing it minimally to make the baseline green is correct, and is not a
        re-design."
      - "A stale reference or comment actively misleads (RS-03) — correcting it is in
        scope even though it edits folded code."
~~~

</disciplines>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Phase spec & requirements
- `.planning/ROADMAP.md` §"Phase 28: Reading-Shell Fold-In" - goal, success criteria, key files.
- `.planning/REQUIREMENTS.md` §RS-01…RS-04 - acceptance criteria (the authoritative bar).
- `.planning/STATE.md` §"Accumulated Context" - the uncommitted-working-tree summary + substrate note (SMTC offline this session).

### Architecture docs (fold the scope-reduction notes into these)
- `docs/architecture/overview.md` - nav/reading-shell subsystem (tree-filter→search change lands here).
- `docs/architecture/graph-subsystem.md` - graph subsystem (per-page local-graph removal lands here).
- `.planning/PROJECT.md` §Key Decisions - the two removal decisions land here.

### Codebase map (CM6 subsystem — relevant to the RS-04 smoke)
- `.planning/codebase/MAP-cm-editor-subsystem.md` - the CodeMirror editor subsystem map.

### Test conventions
- `web/src/stores/*.test.ts` (e.g. `recent.test.ts`, `editorMode.test.ts`) - store-test pattern for `navSearch`.
- `web/src/components/search/highlight.test.tsx` - existing search-util test (SearchResultRow's tested part).

### No external specs
- No third-party specs/ADRs — requirements fully captured in the decisions above.
</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `web/src/components/PropertiesPanel.tsx:57-73` — the tag union
  (`readTags(frontmatter) ∪ inlineTagsFromBody(body)`, inline-origin no ✕, only non-inline
  removable) is ALREADY implemented and confirmed intact; RS-01 criterion 3 is verifying
  this, not building it.
- `web/src/components/search/SearchResultRow.tsx` + `highlight.ts` — render + highlight
  already exist and `highlight.ts` is unit-tested; NavSearchResults composes them.
- `web/src/hooks/useSearch.ts` — the `/api/v1/search` query hook NavSearchResults consumes
  (mock this in tests).
- Store-test pattern: `web/src/stores/{recent,editorMode,agentContext}.test.ts`.

### Established Patterns
- Frontend tests: vitest + `@testing-library/react` + `@testing-library/user-event`
  + jsdom. `npm test` = `vitest run`. No Playwright/e2e infra.
- `npm run build` runs `tsc -b && vite build` with the `assertLazyChunks` guard — part of
  the green bar (criterion 2).
- Backend gate: `go build ./... && go vet ./... && go test ./...`.

### Integration Points
- `web/src/routes/AppShell.tsx` — owns the ⌘K keydown (`:558`, opens navrail + focuses
  `searchInputRef`), the navrail search input (`:693`), and the searching?→NavSearchResults
  vs tree switch (`:716-720`). The ⌘K test renders AppShell (or minimal nav chrome).
- CM6 smoke surface: `web/src/lib/cm/theme.ts` (heading `margin`→`padding`, +16/−6) +
  `web/src/components/LivePreviewEditor.tsx` (`drawSelection` + `lineWrapping`). The theme
  comment (`theme.ts` @@ ~79) documents WHY: CM6's height oracle measures padding, not
  margin, so `posAtCoords`/`coordsAtPos` stay exact.

### Landmines
- The working tree mixes non-refactor changes into `git status` (8 `.smtc/memories/*.md`,
  `26-VERIFICATION.md`, deleted `dist/.gitkeep`) — see D-03/D-04. Do NOT sweep these into a
  refactor commit.
- `TagBadgeBar` still appears as a **CSS class name** in `PropertiesPanel.tsx/.css` by
  design (visual parity) — only the `PageEditor.test.tsx` *descriptions* (D-11) are stale.
- SMTC daemon OFFLINE this session — any code claim is grep/read confidence (Structural/
  Honest tier), never Decision-tier.
</code_context>

<specifics>
## Specific Ideas

- The RS-04 smoke must exercise, at minimum: drag-select + copy across **bold, heading,
  link, code** (correct copied text); caret placement **around headings** and **on wrapped
  lines** (lands where clicked, no drift). These are the exact regressions the
  margin→padding fix targets.
- Restore `internal/web/dist/.gitkeep` verbatim (empty file) — it keeps the embedded
  build-output dir present for the Go embed.
</specifics>

<deferred>
## Deferred Ideas

- **Re-introduce a per-page local graph** if the global-only graph proves insufficient —
  already a Future requirement (REQUIREMENTS.md Future section). Docs point to it (D-10).
- **In-place file-tree filter** (narrow-the-tree-by-title) if full-text-replaces-tree
  proves worse for quick nav — already a Future requirement. Docs point to it (D-10).
- Playwright/e2e infra for the CM6 caret/selection behavior — out of scope this phase
  (smoke is manual-driven per D-05); would make RS-04 regression-testable in future.

None of these are acted on in Phase 28.
</deferred>

---

*Phase: 28-reading-shell-fold-in*
*Context gathered: 2026-07-01*
