---
author: qa
responsible: qa
phase: "28"
---

# Phase 28: Reading-Shell Fold-In — VERIFICATION

**Plan file:** `.planning/phases/28-reading-shell-fold-in/28-01-PLAN.md`
**Context file:** `.planning/phases/28-reading-shell-fold-in/28-CONTEXT.md`
**SUMMARY file:** `.planning/phases/28-reading-shell-fold-in/28-SUMMARY.md` (claims checked against code reality, not trusted)
**Requirements:** RS-01, RS-02, RS-03, RS-04
**Verified at HEAD:** `d0af48d` (phase commit range `037fc92..d0af48d`)
**Prior-phase baseline:** v1.4 milestone-close `95ca67d` (present in history; regression compared against it).
**Substrate note:** SMTC daemon OFFLINE this run — all code claims are grep/read + live re-execution confidence (**Structural/Honest tier, never Decision-tier**). Adequate for a fold-in phase (commit slicing, test backfill, docs, one browser-only CM6 change). The RS-04 CM6 selection/coordinate behavior is not unit-testable under jsdom; it was verified via a real-Chromium `posAtCoords↔coordsAtPos` round-trip + screenshots — see RS-04 for my judgment on soundness and the honest limit of my independent re-execution.

## Verdict

```yaml
gate_result:
  record_author: qa
  id: GR-28-verification
  phase: "28"
  verdict: pass
  basis: >
    All four RS requirements hold against the real code (read on disk, not trusted from the
    SUMMARY) and full re-run evidence at HEAD d0af48d: backend go build/vet/test = 18 pkgs ok
    (exit 0); from web/ npx tsc -b exit 0, npx vitest run 800 passed / 64 files, npm run build
    exit 0 with assertLazyChunks wired in vite.config.ts and active. The refactor lands as the
    exact 6-commit slicing in plan order; the CM6 commit (ccff505) is isolated to theme.ts +
    LivePreviewEditor.tsx ALONE (2 files, independently revertable); commit 3 (561d28c) carries
    BOTH the Go and TS local-graph paths in one commit; commits 5+6 contain no refactor code;
    internal/web/dist/.gitkeep is present at HEAD and appears in NO phase-28 commit. The canonical
    tag union (readTags(frontmatter) ∪ inlineTagsFromBody(body), ci-deduped, inline-origin no ✕,
    only non-inline removable) survived the TagBadgeBar→PropertiesPanel move (13 tests pass; the
    three union assertions are literally present and the effectiveTags impl at PropertiesPanel.tsx:
    57-73 matches). The nav-search coverage gap is closed: NavSearchResults.test.tsx (7 cases),
    navSearch.test.ts (3), and the ⌘K case in AppShell.test.tsx (1) = +11 over the 789 baseline
    → 800; the tests were grounded against the real component and cover the claimed behavior. Both
    scope reductions are documented as revisitable decisions with REQUIREMENTS Future pointers
    (overview.md, graph-subsystem.md, PROJECT.md 2 rows); both stale refs are corrected as
    replacements (not deletions); the tag-badge CSS class names remain; the local-graph removal is
    grep-clean end-to-end in source (only a gitignored vendored dist bundle matched). RS-04's 3
    screenshots exist and show the claimed drawSelection span + wrapped line; the CM6 changes are
    in committed code; the deterministic round-trip is a SOUND substitute for a visual smoke.
    Regression: every deleted test maps to a removed feature (TagBadgeBar, SearchPalette, local
    graph, tree-filter); all surviving global-graph tests remain; the Phase-29 write surface is
    untouched. Fold-not-redesign held — the only edits to folded code are the two RS-03 stale-ref
    string fixes.
  evidence:
    - "go build ./... && go vet ./... && go test ./... → 18 packages ok, 2 no-test (exit 0)"
    - "web/: npx tsc -b exit 0; npx vitest run → 800 passed / 64 files (exit 0); npm run build exit 0 (assertLazyChunks in vite.config.ts:3,69)"
    - "git log --oneline 037fc92..d0af48d = 6 refactor/housekeeping/docs commits + the execute-phase docs(28) SUMMARY commit d0af48d, in plan order"
    - "git show --stat ccff505 = web/src/lib/cm/theme.ts + web/src/components/LivePreviewEditor.tsx ONLY (2 files, 28+/7-)"
    - "git show --stat 561d28c = 6 Go files + 13 TS files (local-graph, one commit)"
    - "git log 037fc92..d0af48d -- internal/web/dist/.gitkeep = empty; git ls-files = tracked at HEAD; git status --porcelain = clean (only untracked 29-tag-regression-fix/)"
    - "npx vitest run src/components/PropertiesPanel.test.tsx = 13 passed (RS-01 crit 3 union assertions present)"
    - "git diff --diff-filter=D 037fc92 d0af48d = TagBadgeBar/DepthControl/LocalGraphPanel/SearchPalette/searchStore/localGraphPanel/treeFilter (all removed features)"
    - "git diff --name-only 037fc92 d0af48d -- handlers_tags_admin.go internal/pages/batch.go internal/okf/* web/src/lib/tag-reconcile.ts = empty (Phase-29 surface untouched)"
    - "grep -rn source-only (excl dist) 'Neighborhood|/graph/local|LocalGraphPanel|DepthControl' internal/ web/src = no matches"
```

---

## RS-01 — Commit the reading-shell refactor as a clean, verified baseline — **PASSED**

**(a) Full suites green at HEAD (re-run, not trusted):**
- Backend: `go build ./... && go vet ./... && go test ./...` → build exit 0, vet exit 0, `18 packages ok, 2 no tests` (exit 0). The two no-test packages are the `internal/web` embed pkg + a vendored `node_modules/.../flatted` Go pkg — expected.
- Frontend (from `web/`): `npx tsc -b` exit 0; `npx vitest run` → **800 passed / 64 files** (exit 0); `npm run build` → exit 0. The `assertLazyChunks` guard is imported and invoked in `web/vite.config.ts:3,69` (`maxEntryBytes: 1_420_000`) and ran during the build. The `act(...)` lines in vitest stderr are pre-existing warnings (present in the v1.4 baseline), not failures.

**(b) 6-commit slicing in plan order** (`git log --oneline 037fc92..d0af48d`):
| # | commit | message |
|---|--------|---------|
| 1 | `2ad00f0` | feat(28): reading-shell document view — Breadcrumb + PropertiesPanel + DocStatusBar (retire TagBadgeBar) |
| 2 | `8c90494` | feat(28): inline navrail full-text search (NavSearchResults + navSearch) replacing the ⌘K palette |
| 3 | `561d28c` | refactor(28): remove per-page local graph end-to-end (retain global /app/graph) |
| 4 | `ccff505` | fix(28): CM6 selection/coordinate accuracy — heading margin→padding + drawSelection + lineWrapping |
| 5 | `869d91f` | chore(28): fold in prior-phase KB notes + phase-26 verification |
| 6 | `4916a9d` | docs(28): document tree-filter + per-page-local-graph removals (with future re-intro paths) |
| (7) | `d0af48d` | docs(28): phase-28 plan + summary + RS-04 smoke evidence |

Commit 7 (`d0af48d`, current HEAD) is the execute-phase SUMMARY step's `docs(28)` commit (plan + SUMMARY + screenshots), committed AFTER the plan's six per the plan's `<commit_policy>`. **This is expected, not a discrepancy** — the SUMMARY was authored at `4916a9d` (before this 7th commit existed), so its "HEAD 037fc92 → 4916a9d, 6 commits" text is accurate for the moment it was written. (Flagged below so the architect isn't confused by the HEAD mismatch.)

**(b-CM6) Commit 4 isolation** (`git show --stat ccff505`): exactly `web/src/components/LivePreviewEditor.tsx` (+13/−1) + `web/src/lib/cm/theme.ts` (+22/−6) — 2 files, independently revertable. **HARD requirement met.**

**(c) Commit 3 Go+TS in one commit** (`git show --stat 561d28c`): 6 Go files (`internal/graph/query.go`+`_test.go`, `internal/server/handlers_graph.go`+`_test.go`, `router.go`, `handlers_auth.go`) + 13 TS files (`api/client.ts`, `graph/GraphView.tsx`, `GraphCanvas.tsx`, `useGraph.ts`+`.test.ts`, deleted `DepthControl.*`/`LocalGraphPanel.*`/`stores/localGraphPanel.*`). **Both paths, one commit.**

**(d) Commits 5+6 contain no refactor code:** commit 5 = 8 `.smtc/memories/*.md` + `26-VERIFICATION.md`; commit 6 = `PROJECT.md` + `overview.md` + `graph-subsystem.md`. No source files. **Confirmed.**

**(e) `internal/web/dist/.gitkeep`:** present at HEAD (`git ls-files` shows it tracked); `git log 037fc92..d0af48d -- internal/web/dist/.gitkeep` = empty (never in any phase-28 commit). After I ran `npm run build` (emptyOutDir wiped it), I ran `git checkout -- internal/web/dist/.gitkeep` and confirmed `test -f` succeeds and `git status --porcelain` is clean. Tree left clean (only untracked `29-tag-regression-fix/`, the next phase). **Confirmed.**

**(f) RS-01 crit 3 — canonical tag union intact:** `npx vitest run src/components/PropertiesPanel.test.tsx` = **13 passed**. The three union assertions are literally present in the test file: non-inline `release` → ✕ → `onRemoveTag("release")`; inline-only `#cooking`/`deploy` → no ✕; ci-dedup `#Cooking`∪`#cooking` → one chip, no remove control. The implementation at `PropertiesPanel.tsx:57-73` rebuilds `readTags(frontmatter)` (frontmatter order) then inline-only body tags via `inlineTagsFromBody(body)`, ci-deduped, with the `inlineOrigin` flag driving the no-✕ behavior. **Union survived the TagBadgeBar→PropertiesPanel move.**

---

## RS-02 — Close the search-redesign test-coverage gap — **PASSED**

**`NavSearchResults.test.tsx` (NEW, 7 cases)** — read in full and grounded against `NavSearchResults.tsx`:
- **Grouping (2):** three group labels render in order `["Pages","Headings","Attachments"]` (asserted via `.navsearch-group-label` textContent); a zero-result kind omits its group (no attachments → no "Attachments"). Matches `GROUP_ORDER` + `.filter(g => g.rows.length > 0)` at `NavSearchResults.tsx:12-16,37-40`.
- **Heading navigate + query-clear (1):** clicking a heading result with `anchor:"#my-section"` calls `navigate("/app/page/notes.md#my-section")` (**leading `#` stripped for the DOM id, kept in the route hash**) AND `useNavSearch.getState().query === ""`. Matches `openResult` at `:55-69` (`r.anchor.replace(/^#/, "")` → `navigate(...#${id})` → `setQuery("")`).
- **Plain page navigate + clear (1):** `navigate("/app/page/welcome.md")` (no hash) + query clear.
- **States (3):** `isError` → `role="alert"` + /unavailable/ (`:71-77`); empty `data:[]` → /No matches/ (`:88-94`); `isLoading` w/ no data → fake-timers advance 150ms → "Searching…" (`:79-86`, delayed via the `setTimeout(...,150)` effect at `:46-53`). Deterministic (fake timers).

**`navSearch.test.ts` (NEW, 3 cases):** initial `query===""`; `setQuery("hello")` sets; `setQuery("")` clears back (the tree-returns-when-empty contract). Pure store test, uses `useNavSearch.getState()`.

**⌘K in `AppShell.test.tsx` (NEW, 1 case, RS-02/D-07):** `fireEvent.keyDown(document, { key:"k", metaKey:true })` (correctly targets `document`, since the handler is a `document` listener — a `window` event wouldn't reach it in jsdom) → `waitFor` the `placeholder="Search…"` input `toHaveFocus()`. Tests the handler where it lives (AppShell), starting with the navrail collapsed to prove open+focus.

**Count:** 789 baseline + 11 (7+3+1) = **800**, +2 test files (62→64). Internally consistent and re-run green. **All three RS-02 acceptance criteria met.**

---

## RS-03 — Document the intentional scope reductions — **PASSED**

- **overview.md** (`:113` narrative + `:122` Key-Architectural-Decisions bullet): documents that the in-place file-tree filter (narrow-by-title) is removed and the navrail search now REPLACES the tree with full-text `NavSearchResults` when the query is non-empty, returning to the tree when cleared; the decision bullet points to REQUIREMENTS.md Future "In-place file-tree filter". ✓
- **graph-subsystem.md** (`:3` overview note, `:137-144` removal section, `:182` HTTP-endpoints table): records the per-page local graph removed end-to-end — `Neighborhood(ctx, pagePath, depth)` method, `GET /graph/local` route+handler, client `LocalGraphPanel`/`DepthControl` — with the **global `/app/graph` (GraphData) RETAINED**, and a Future pointer "Re-introduce a per-page local graph". ✓
- **PROJECT.md** Key Decisions table: **2 new rows** (`:116`, `:117`), each with its REQUIREMENTS.md Future re-intro pointer (plus reinforcing bullets at `:59-60`). ✓
- **Stale refs corrected (as replacements, not deletions):**
  - `grep "Search now lives in the top bar" web/src/routes/AppShell.tsx` → **none** (exit 1). Replaced at `AppShell.tsx:752` with `{/* Fixed footer — Settings/Admin. Search lives inline in the navrail (⌘K focuses it). */}`.
  - `grep "TagBadgeBar" web/src/routes/PageEditor.test.tsx` → **none** (exit 1). The `:508` `it()` description + `:536` comment now name "PropertiesPanel"; `git show 2ad00f0` confirms the diff is EXACTLY those two strings — **assertion bodies unchanged**.
- **`tag-badge` CSS class names PRESERVED (must NOT be renamed):** `grep "tag-badge" web/src/components/PropertiesPanel.{tsx,css}` → 8 matches (`.tag-badge-bar`, `.tag-badge`, `.tag-badge-inline`, `.tag-badge-x` in CSS; `className="tag-badge-bar"`, `"tag-badge tag-badge-inline"`, `"tag-badge-x"` in TSX). ✓
- **Local-graph removal real end-to-end (grep-clean):** source-only `grep -rn -E 'Neighborhood|/graph/local|LocalGraphPanel|DepthControl' internal/ web/src/ --include={go,ts,tsx,css}` (excluding `/dist/`) → **no matches** (exit 1). The one apparent hit — `internal/web/dist/assets/cytoscape.esm-*.js` `neighborhood()` — is the vendored cytoscape library's own graph-theory API in a **gitignored build artifact** (`git check-ignore` confirms), produced by the `npm run build` I ran; unrelated to the removed feature. Global `/app/graph` retained in source (`App.tsx:124` route + `AppShell.tsx:681-686` navrail + `GraphView.tsx`). ✓

---

## RS-04 — Smoke the manual-QA-only editor coordinate change — **PASSED (with an honest limit on my re-execution; see finding F-2)**

**(a) Screenshots exist & show the claim** (`.planning/phases/28-reading-shell-fold-in/`):
- `28-smoke-selection-across-constructs.png` — the CM6 `drawSelection` lavender highlight visibly spans `# Heading One` + `**bold**` + `[link](https://example.com)` + `` `code` `` (raw markers revealed on the active/selected lines — correct CM6 live-preview behavior under selection). ✓
- `28-smoke-caret-around-heading.png` — rendered heading + body (markers hidden, no selection), caret at the heading boundary. ✓
- `28-smoke-caret-wrapped-line.png` — the long line wrapped into 3 visual rows (`lineWrapping` active), caret on a wrapped row. ✓

**(b) CM6 changes in committed code** (`git show ccff505`):
- `theme.ts`: all three heading classes `.cm-heading-1/2/3-6` changed `margin`→`padding`, with the CRITICAL height-oracle comment ("CM6's height oracle measures each line's offsetHeight — which INCLUDES padding but EXCLUDES margin... Padding is measured, so coordinates stay exact"). `.cm-content` also gains vertical `padding`. ✓
- `LivePreviewEditor.tsx`: adds `drawSelection` to the `@codemirror/view` import and inserts `drawSelection()` + `EditorView.lineWrapping` in **both** the read-only and editable extension branches (`:146-`, `:172-`). ✓

**(c) My judgment on whether the deterministic evidence soundly satisfies crit 5:**

**It does.** The acceptance criterion is that drag-select+copy across bold/heading/link/code yields correct text, and caret placement around headings + on wrapped lines lands where clicked (no drift). Both properties are functions of CM6's `posAtCoords`/`coordsAtPos` accuracy — the exact mechanism the `margin`→`padding` height-oracle fix targets. The architect's smoke verified this mechanism directly and numerically:
- Copied text was byte-verified from **two independent sources** (CM6 `sliceDoc` AND the browser's rendered `getSelection().toString()`) with no dropped/duplicated bytes across construct boundaries — a stronger check than eyeballing a highlight.
- Caret round-trip at the heading↔body boundary (the most margin/padding-sensitive spot) = **err 0**.
- Wrapped-line round-trip = 21 samples, 20 exact, **maxErr=1 char** at the `#`-marker toggle boundary. I judge this a ±1-char horizontal **rounding at a decoration transition, not drift**: the bug this fix targets produces a **full line-height vertical** error ("selection landing a row off" per the theme.ts comment), qualitatively different from a sub-character marker-boundary rounding within normal CM6 coordinate quantization.

A visual smoke would be *weaker* evidence than this deterministic round-trip. **Residual human-mouse-drag spot-check: an acceptable OPTIONAL follow-up, NOT a blocking gap** — a physical mousedown-drag-mouseup ultimately calls the same `posAtCoords` proven exact here; headless Chromium simply can't reproduce sub-pixel pointer motion, which is the only thing left unexercised, and it rides on already-proven-exact coordinates. This matches the SUMMARY's own honest D-05 flag. Recommend (do not require) a co-driver optionally confirm a real drag feels drift-free.

**Honest limit (finding F-2):** the RS-04 smoke was architect-driven; I did **not** independently re-execute the browser round-trip (it requires standing up an authenticated Go backend + vite dev — the exact environmental blocker the engineer hit, resolved in-session with a dev-only admin credential reset). I independently confirmed the three verifiable substrates — screenshots exist & depict the claim, the CM6 code is committed (ccff505), and the deterministic argument is logically sound — and judge that sufficient for a pass. This is a limitation of my pass, not a defect in the work.

---

## Regression check (vs v1.4 baseline `95ca67d`) — **NO COVERAGE LOSS**

Every deleted file maps to a **removed feature**, not lost coverage for a surviving one (`git diff --diff-filter=D 037fc92 d0af48d`):
- `TagBadgeBar.{tsx,css,test.tsx}` → feature **folded into** `PropertiesPanel` (219-line test + 250-line component ADDED; the same tag-union assertions now live in `PropertiesPanel.test.tsx`, verified passing).
- `search/SearchPalette.{tsx,css,test.tsx}` + `store/searchStore.ts` → ⌘K modal **replaced by** the navrail surface (`NavSearchResults.test.tsx` 186 lines ADDED).
- `graph/LocalGraphPanel.{tsx,css,test.tsx}` + `graph/DepthControl.{tsx,css,test.tsx}` + `stores/localGraphPanel.{ts,test.ts}` → the per-page local-graph **feature removed end-to-end** (RS-03 documented + grep-clean).
- `stores/treeFilter.ts` → the in-place tree-filter **feature removed** (no dedicated test file existed).

Surviving global-graph coverage is **retained**: `internal/graph/query_test.go`, `internal/server/handlers_graph_test.go`, `web/src/components/graph/GraphView.test.tsx`, `web/src/hooks/useGraph.test.ts` all still exist. The deletions inside the modified test files were surgical: `useGraph.test.ts` dropped only the `useLocalGraph` hook cases (global `useGraph` kept); `query_test.go` dropped only `TestNeighborhood_*` (global rebuild/GraphData tests kept). Full suite green at HEAD (800/64, 18 Go pkgs).

## Fold-not-redesign — **HELD**

The refactor commits (1–3) slice a pre-existing working-tree blob; the only NEW source edits Tasks 1–9 introduced into folded code are the two RS-03 stale-ref string fixes (`AppShell.tsx:752` comment in commit 2; `PageEditor.test.tsx:508/536` descriptions in commit 1, verified diff-scoped to those strings with assertions unchanged). Commit stats are coherent replace-feature-X-with-feature-Y shapes confined to the reading-shell / nav-search / graph domains — no scattered churn. The **Phase-29 write surface is untouched**: `git diff --name-only 037fc92 d0af48d -- internal/server/handlers_tags_admin.go internal/pages/batch.go internal/okf/* web/src/lib/tag-reconcile.ts` = empty.

---

## Findings routed UP to the architect

- **F-1 (INFO, not a gap):** HEAD is `d0af48d` — one commit beyond the SUMMARY's stated final `4916a9d`. This 7th commit is the execute-phase SUMMARY step's `docs(28)` commit (plan + SUMMARY + screenshots), which by the plan's `<commit_policy>` lands AFTER the six. The SUMMARY's "6 commits, HEAD 4916a9d" is accurate as-authored (pre-7th-commit). No action needed; noted so the HEAD delta isn't mistaken for scope creep.
- **F-2 (LOW, honest verification limit):** RS-04's browser smoke was architect-driven and I did not independently re-run the `posAtCoords↔coordsAtPos` round-trip (requires an authenticated backend + vite dev; the documented environmental blocker). I confirmed the committed CM6 code, the 3 screenshots, and the soundness of the deterministic argument, and judge RS-04 passed on that basis. Optional co-driver spot-check of a real human drag remains available but is not expected to reveal anything the exact round-trip (maxErr=1 char, all wrapped rows exact) missed. This is a limit of my pass, not a defect.
- **F-3 (INFO, dev-instance side effect from RS-04):** the running `:8098` dev server's `admin` password was reset for the smoke (`okfsmoke-rs04-2026`; documented `adminadmin12` no longer works) and the global git identity was set. These touch only gitignored `data/` + local git config — **not the repo/commits** (tree confirmed clean). Noted for the co-driver.

## Overall verdict: **PASSED**

All four RS requirements are satisfied against code reality with full re-run evidence; the regression and fold-not-redesign gates hold. No blocking gaps. Three non-blocking findings (F-1/F-3 informational, F-2 a stated limit of my independent RS-04 re-execution) are routed up for the architect's awareness.

---

*Verified 2026-07-01 by qa (Phase28QA). Evidence gathered via live re-execution (go build/vet/test, tsc -b, vitest run, npm run build), git log/show/diff, grep/read on disk, and direct inspection of the 3 RS-04 screenshots at HEAD `d0af48d`. SMTC daemon OFFLINE — Structural/Honest tier.*
