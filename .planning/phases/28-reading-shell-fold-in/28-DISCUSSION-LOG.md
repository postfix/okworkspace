# Phase 28: Reading-Shell Fold-In - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md - this log preserves the alternatives considered.

**Date:** 2026-07-01
**Phase:** 28-reading-shell-fold-in
**Areas discussed:** Commit slicing & hygiene, RS-04 browser smoke, Search test-backfill depth, Scope-reduction docs placement

---

## Commit slicing & hygiene

### Commit granularity

| Option | Description | Selected |
| ------ | ----------- | -------- |
| 3 feature commits + CM6 | reading-shell / inline nav-search / local-graph removal (end-to-end) / CM6 fix alone; verify before commit | ✓ |
| 1 refactor commit + CM6 | Single blob except CM6; simpler history, hard to bisect | |
| Backend-split (4+CM6) | Local-graph removal split into Go + TS commits (5 total) | |

**User's choice:** 3 feature commits + CM6
**Notes:** CM6 change (theme.ts margin→padding + drawSelection/lineWrapping) is a hard-required own-commit per criterion 2. Verify-before-commit order enforced (D-02).

### Working-tree contamination

| Option | Description | Selected |
| ------ | ----------- | -------- |
| Separate housekeeping commit(s) | Commit .smtc/memories + 26-VERIFICATION.md separately; restore dist/.gitkeep | ✓ |
| Commit KB, drop the rest | Commit memories only; investigate verification/gitkeep first | |
| Leave all out of Phase 28 | Refactor files only; rest lingers for later cleanup | |

**User's choice:** Separate housekeeping commit(s)
**Notes:** Keeps the four refactor commits pure. dist/.gitkeep deletion judged accidental (build-output keeper) → restore. (D-03, D-04)

---

## RS-04 browser smoke

| Option | Description | Selected |
| ------ | ----------- | -------- |
| I drive Chromium + you confirm | Agent drives real Chromium via browser tool, captures screenshots, records in SUMMARY; flags headless-infidelity cases | ✓ |
| You run a manual checklist | Agent produces checklist + run commands; user executes and pastes results | |
| Both: I drive, you backstop | Agent drives reproducible cases; user runs residual human-drag-select checklist | |

**User's choice:** I drive Chromium + you confirm
**Notes:** Self-contained evidence trail. Agent flags any construct headless Chromium can't faithfully exercise (true human-mouse drag-select drift) for the user's spot-check. (D-05)

---

## Search test-backfill depth

### SearchResultRow coverage

| Option | Description | Selected |
| ------ | ----------- | -------- |
| Cover it via NavSearchResults | No standalone SearchResultRow.test.tsx; exercised transitively | ✓ |
| Add standalone SearchResultRow tests | Dedicated file; largely restates NavSearchResults coverage | |

**User's choice:** Cover it via NavSearchResults
**Notes:** highlight.ts (the logic-heavy part) already unit-tested; criteria target NavSearchResults/navSearch. (D-06)

### ⌘K test home

| Option | Description | Selected |
| ------ | ----------- | -------- |
| New AppShell-focused test | Test ⌘K where the handler lives (AppShell.tsx:558) | ✓ |
| Fold into NavSearchResults suite | Single search-surface file; couples test to wrong unit | |
| Skip ⌘K unit, cover in smoke | Lightest; leaves a named RS-02 criterion without a unit test | |

**User's choice:** New AppShell-focused test
**Notes:** ⌘K opens navrail + focuses searchInputRef — AppShell behavior, not NavSearchResults. (D-07)

---

## Scope-reduction docs placement

### Docs home

| Option | Description | Selected |
| ------ | ----------- | -------- |
| Fold into existing architecture docs | overview.md (tree-filter→search) + graph-subsystem.md (local-graph removal) + PROJECT.md Key Decisions | ✓ |
| New dedicated decisions doc | Single reading-shell-v1.5.md; splits subsystem knowledge | |

**User's choice:** Fold into existing architecture docs
**Notes:** Matches the current per-subsystem docs layout. (D-09)

### Future re-introduction

| Option | Description | Selected |
| ------ | ----------- | -------- |
| Record removal + Future pointer | Document removal + rationale AND point to REQUIREMENTS.md Future re-intro path | ✓ |
| Record removal only | Removal + why; re-intro left to REQUIREMENTS.md without cross-ref | |

**User's choice:** Record removal + Future pointer
**Notes:** Reads as intentional, revisitable — exactly RS-03's "decisions, not accidental loss." (D-10)

---

## The agent's Discretion

- Exact commit messages (feature-scoped, conventional) and file-group ordering within a commit.
- Test file structure/naming within vitest + testing-library conventions.
- Screenshot naming/layout under the phase dir for smoke evidence.

## Deferred Ideas

- Re-introduce per-page local graph (already a Future requirement).
- In-place file-tree filter (already a Future requirement).
- Playwright/e2e infra to make the CM6 caret/selection behavior regression-testable.
