# Deferred Items — Phase 7

Out-of-scope discoveries logged during execution (SCOPE BOUNDARY rule). NOT fixed
in this phase.

## Pre-existing flaky CodeMirror tests (Phase 6, unrelated to Phase 7 files)

- **Found during:** 07-04 final full-suite verification (`npm test`).
- **Files:** `web/src/lib/cm/livePreview.test.ts`, `web/src/lib/cm/headingAnchors.test.ts`
- **Symptom:** Each intermittently fails ONE assertion (italic Emphasis mark hide;
  heading-line id == slug) only under full-suite parallel CPU load. Both pass
  reliably in isolation (`npx vitest run src/lib/cm/livePreview.test.ts
  src/lib/cm/headingAnchors.test.ts` → 30/30) and passed in other full-suite runs
  of this session.
- **Assessment:** CodeMirror layout/decoration timing flake under contention, not a
  correctness regression. These files were NOT touched by Phase 7 (07-04 touches
  only LeftTree/TrashView/useTreeMutations/DeleteFolderDialog + their tests).
- **Disposition:** Deferred — belongs to a Phase 6 test-stability follow-up
  (e.g. mark the timing-sensitive assertions deterministic or serialize the CM
  test files). Do NOT block Phase 7 completion.
