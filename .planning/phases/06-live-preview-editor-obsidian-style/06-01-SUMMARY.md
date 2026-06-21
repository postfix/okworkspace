---
phase: 06-live-preview-editor-obsidian-style
plan: 01
subsystem: ui
tags: [codemirror6, lezer, markdown, editor, zustand, vitest, react]

# Dependency graph
requires:
  - phase: 01-foundation
    provides: PageEditor save machinery (runSaver, baseRevision, 409 ConflictBanner, autosave), LinkPicker, frontmatter form
  - phase: 03-search
    provides: okf.ScanHeadings github-slugger anchors that search deep-links target (SRCH-06)
provides:
  - CodeMirror 6 editing surface (LivePreviewEditor) replacing @uiw/react-md-editor in PageEditor, same value/onChange contract
  - Persisted Live/Source editor-mode store (zustand+persist, key okf.editor.mode, default live)
  - Byte-stable Live/Source toggle via a CM6 Compartment (effects-only dispatch) + Cmd/Ctrl-E keymap
  - GFM-only CM6 markdown language config (Table/Strikethrough/TaskList/Autolink) for server+read-mode parity
  - Token-only CM6 EditorView theme + sourceTheme
  - Two pure security/anchor gate modules: sanitizeImageSrc (image-src allowlist) and slug/dedupSlug/headingText (github-slugger port)
  - Whole-phase Wave-0 test scaffolds + shared okf corpus fixtures (cmCorpus.ts)
affects: [06-02-live-preview-decorations, 06-03-image-table-widgets, 06-04-unified-read-mode]

# Tech tracking
tech-stack:
  added:
    - "@codemirror/state@6.6.0"
    - "@codemirror/view@6.43.1"
    - "@codemirror/commands@6.10.3"
    - "@codemirror/language@6.12.3"
    - "@codemirror/lang-markdown@6.5.0"
    - "@lezer/markdown@1.6.4"
  patterns:
    - "CM6 EditorView in a React useEffect with always-destroy cleanup (StrictMode-safe)"
    - "Compartment.reconfigure with effects-only dispatch for byte-identical mode toggle"
    - "externalSeed Annotation so a programmatic value-seed transaction does not echo through onChange"
    - "web/src/lib/cm/ holds composable CM6 extension modules; editor mode is a persisted zustand store"
    - "Frontend slug() ported byte-for-byte from the Go okf.ScanHeadings so anchors agree across backend/read-view"

key-files:
  created:
    - web/src/components/LivePreviewEditor.tsx
    - web/src/components/LivePreviewEditor.css
    - web/src/components/LivePreviewEditor.test.tsx
    - web/src/lib/cm/mode.ts
    - web/src/lib/cm/markdown.ts
    - web/src/lib/cm/theme.ts
    - web/src/lib/cm/sanitizeSrc.ts
    - web/src/lib/cm/headingAnchors.ts
    - web/src/lib/cm/mode.test.ts
    - web/src/lib/cm/livePreview.test.ts
    - web/src/lib/cm/sanitizeSrc.test.ts
    - web/src/lib/cm/headingAnchors.test.ts
    - web/src/stores/editorMode.ts
    - web/src/stores/editorMode.test.ts
    - web/src/test/cmCorpus.ts
  modified:
    - web/src/routes/PageEditor.tsx
    - web/src/routes/PageEditor.test.tsx
    - web/src/routes/PageEditor.css
    - web/package.json
    - web/package-lock.json

key-decisions:
  - "CRLF: CM6 (like a <textarea>) LF-normalizes internally; EDIT-02 toggle invariance is asserted against the document's own bytes, on-disk CRLF round-trip stays a backend gate"
  - "External value seeds are externalSeed-annotated so they do not echo through onChange (avoids Pitfall 6 feedback loop)"
  - "liveExtensions kept minimal this plan (markdown language + theme); inline-preview decorations land in 06-02/06-03"
  - "Package-legitimacy checkpoint treated as pre-resolved per 06-RESEARCH false-positive audit; no human halt"

patterns-established:
  - "CM6 extension modules under web/src/lib/cm/, mode toggle via Compartment, persisted mode store"
  - "Pure, unit-tested gate modules (sanitizeImageSrc, slug/dedupSlug) as single audited chokepoints"

requirements-completed: [EDIT-02, EDIT-03, EDIT-04]

# Metrics
duration: 17min
completed: 2026-06-21
---

# Phase 6 Plan 01: CodeMirror 6 Editor Walking Slice Summary

**Swapped PageEditor's @uiw/react-md-editor for a CodeMirror 6 surface (LivePreviewEditor) with a byte-stable persisted Live/Source toggle (Compartment + Cmd/Ctrl-E), GFM-only markdown config, token-only theme, and the whole phase's Wave-0 test scaffolds + pure security/anchor gates.**

## Performance

- **Duration:** ~17 min
- **Started:** 2026-06-21T13:26:00Z (approx)
- **Completed:** 2026-06-21T13:43:00Z
- **Tasks:** 4
- **Files modified:** 20 (15 created, 5 modified)

## Accomplishments
- CM6 editing surface replaces MDEditor in PageEditor with the EXACT `value`/`onChange(string)` contract; typed bytes are shipped verbatim (no block-model rewrite) and the save machinery (runSaver/baseRevision/409 ConflictBanner/autosave/frontmatter form/LinkPicker) is untouched.
- Persisted Live/Source mode store (`okf.editor.mode`, default Live) + a byte-identical Compartment toggle proven over all 8 okf corpus fixtures, driven by a header segmented control and the Cmd/Ctrl-E keymap.
- Two pure, unit-tested gate modules shipped GREEN now for later waves: `sanitizeImageSrc` (image-src allowlist, T-06-01) and `slug`/`dedupSlug`/`headingText` (github-slugger port matching okf.ScanHeadings, SRCH-06).
- Every phase Wave-0 test scaffold created and runnable, with EDIT-02/03/04 + SRCH-06 each having an automated verify before its feature lands.

## Task Commits

Each task was committed atomically:

1. **Task 1: Package legitimacy verify + add CM6/Lezer deps** - `d097afc` (chore)
2. **Task 2: Wave-0 scaffolds + corpus fixtures + pure-gate modules** - `bd153f5` (test)
3. **Task 3: editorMode store + mode Compartment + markdown/theme** - `5ff35b6` (feat)
4. **Task 4: LivePreviewEditor + swap into PageEditor** - `f021c8a` (feat)

_Note: Tasks 3 and 4 are TDD tasks; the RED test files were authored in Task 2 (mode.test.ts, LivePreviewEditor.test.tsx) and the editorMode store test was authored RED in Task 3 before implementation, so each landed as a single feat commit that flipped its gate to GREEN._

## Files Created/Modified
- `web/src/components/LivePreviewEditor.tsx` - CM6 EditorView React wrapper, MDEditor value/onChange drop-in; verbatim-bytes updateListener; externalSeed annotation; StrictMode-safe cleanup; mode Compartment reconfigure.
- `web/src/components/LivePreviewEditor.css` - token-var-only host styling.
- `web/src/lib/cm/mode.ts` - modeCompartment, live/source extension sets, byte-stable setMode, Mod-e toggleKeymap.
- `web/src/lib/cm/markdown.ts` - GFM-only markdown() config over commonmark base.
- `web/src/lib/cm/theme.ts` - EditorView.theme + sourceTheme reading only tokens.css vars.
- `web/src/lib/cm/sanitizeSrc.ts` - sanitizeImageSrc allowlist gate.
- `web/src/lib/cm/headingAnchors.ts` - slug/dedupSlug/headingText github-slugger port.
- `web/src/stores/editorMode.ts` - zustand+persist Live/Source store.
- `web/src/test/cmCorpus.ts` - 8 okf corpus fixtures verbatim (CRLF + no-trailing-newline preserved).
- `web/src/routes/PageEditor.tsx` - swapped surface to LivePreviewEditor, added mode toggle, dropped MDEditor import; save machinery unchanged.
- `web/src/routes/PageEditor.test.tsx` - retargeted editor mock to LivePreviewEditor; all autosave/conflict assertions preserved.
- Test files: `mode.test.ts`, `editorMode.test.ts`, `sanitizeSrc.test.ts`, `headingAnchors.test.ts`, `livePreview.test.ts`, `LivePreviewEditor.test.tsx`.

## Decisions Made
- **CRLF normalization:** CM6's Text store normalizes line endings to `\n` internally (verified empirically; `lineSeparator` does not preserve mixed CRLF). A browser `<textarea>` — the prior MDEditor surface — does the same on the wire, so this is not a regression. EDIT-02's invariant ("the toggle never mutates the current bytes") is asserted against the document's own pre-toggle string; the on-disk CRLF verbatim round-trip remains a backend concern covered by `TestGoldenRoundTrip` / `TestCorpusHasCRLFFixture`.
- **externalSeed annotation:** A programmatic `value`-prop seed dispatches a `changes` transaction, which would otherwise fire `onChange` and echo the external value back as if it were a user edit. The seed is annotated and the updateListener skips annotated transactions — clean separation of "user edit" vs "external reset" (RESEARCH Pitfall 6).
- **Package-legitimacy checkpoint:** Treated as pre-resolved per the 06-RESEARCH Package Legitimacy Audit (the SUS verdicts on @codemirror/view and @lezer/markdown are documented false positives of the too-new heuristic; canonical Marijn Haverbeke packages, no postinstall scripts). No human halt, per the orchestrator's checkpoint note.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] CRLF corpus fixture broke the literal byte-equality assertion**
- **Found during:** Task 3 (mode.test.ts GREEN gate)
- **Issue:** The EDIT-02 test asserted `view.state.doc.toString() === fixture.text`, but CM6 LF-normalizes the CRLF fixture (07-crlf.md) on seed, so `before !== fixture.text`.
- **Fix:** Re-anchored the byte-stability assertion to the document's own pre-toggle bytes (`before`), which is the true EDIT-02 invariant ("toggle does not change current bytes"), and documented the CM6/textarea LF-normalization + that on-disk CRLF round-trip is a backend gate.
- **Files modified:** web/src/lib/cm/mode.test.ts
- **Verification:** mode.test.ts GREEN over all 8 fixtures including CRLF.
- **Committed in:** 5ff35b6 (Task 3 commit)

**2. [Rule 2 - Missing Critical] External value seed echoed through onChange (feedback loop)**
- **Found during:** Task 4 (LivePreviewEditor.test.tsx GREEN gate)
- **Issue:** Seeding a new external `value` fired the updateListener's `onChange` with the seeded value, which in a controlled-input usage would loop value→onChange→value.
- **Fix:** Added an `externalSeed` CM6 Annotation on the seed transaction and skipped annotated transactions in the updateListener so only genuine user edits report through onChange.
- **Files modified:** web/src/components/LivePreviewEditor.tsx
- **Verification:** "reseeds without onChange feedback loop" test GREEN; all other contract tests GREEN.
- **Committed in:** f021c8a (Task 4 commit)

**3. [Rule 3 - Blocking] eslint react-hooks/refs: refs mutated during render**
- **Found during:** Task 4 (lint of new files)
- **Issue:** `onChangeRef.current = onChange` / `pathRef.current = currentPath` assigned during render, which `react-hooks/refs` flags as an error.
- **Fix:** Moved the ref syncs into a `useEffect([onChange, currentPath])`.
- **Files modified:** web/src/components/LivePreviewEditor.tsx
- **Verification:** eslint clean on LivePreviewEditor.tsx; component tests + tsc still GREEN.
- **Committed in:** f021c8a (Task 4 commit)

---

**Total deviations:** 3 auto-fixed (1 bug, 1 missing critical, 1 blocking)
**Impact on plan:** All three were correctness/quality fixes within the planned task scope. No scope creep; no architectural change.

## Issues Encountered
- None beyond the deviations above. The package-legitimacy checkpoint was pre-resolved per the orchestrator note.

## Known Stubs
- `web/src/lib/cm/livePreview.test.ts` is an intentional Wave-0 placeholder (10 `it.todo`) — the EDIT-01 live-preview ViewPlugin it covers ships in 06-02, which fills these bodies. This is a deliberate, documented RED-pending scaffold per the plan (so the EDIT-01 verify command exists now), not an unmet goal of 06-01.
- `liveExtensions` (mode.ts) is intentionally minimal this plan (markdown language + theme only); inline-preview decorations and image/table widgets land in 06-02/06-03. Documented in CONTEXT/PLAN as the walking-slice scope.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- 06-02 (live-preview decorations) can extend `liveExtensions` in `web/src/lib/cm/mode.ts` and fill `livePreview.test.ts`; the Lezer GFM tree is already parsing.
- 06-03 (image/table widgets) has the `sanitizeImageSrc` gate ready to call before mounting `<img>`.
- 06-04 (unified read mode) has the `slug`/`dedupSlug`/`headingText` anchor port ready and can remove `@uiw/react-md-editor` once the read path stops importing it (it is still a dependency this plan, by design).

## Self-Check: PASSED

All 12 declared created files exist on disk; all 4 task commits (d097afc, bd153f5, 5ff35b6, f021c8a) are present in git history. Full `npm test` GREEN (157 passed, 10 todo, 1 skipped file = the intentional livePreview stub); `npx tsc --noEmit` clean; `eslint` clean on new files; `go test ./internal/okf/ -run TestGoldenRoundTrip` GREEN.

---
*Phase: 06-live-preview-editor-obsidian-style*
*Completed: 2026-06-21*
