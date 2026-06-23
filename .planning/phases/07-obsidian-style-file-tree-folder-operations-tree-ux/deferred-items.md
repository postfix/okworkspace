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

## UI-audit follow-ups deferred (advisory, not applied this phase)

- **Empty-tree state message** (UI-REVIEW IN/priority-2): when the tree is empty,
  `LeftTree` renders an empty `<ul>` with no hint. Adding a `.lefttree-status`
  "No pages yet…" hint is a nice UX win, BUT two pinned tests assert the Pages list
  is `toBeEmptyDOMElement()` on a null/empty tree — one explicitly a `(UAT blocker)`
  / "no white-screen" guarantee, the other in the clean-rebuild regression net.
  Adding the hint requires deliberately updating those pinned tests, which is a
  conscious decision better made on its own (not as a drive-by during an advisory UI
  pass). The empty state is rare in practice (workspaces have pages). Deferred so the
  no-regression override stays intact. When done: add the hint + update both tests to
  assert the hint renders (and still no crash).
- **Shared `Dialog.css` overlay color token** (UI-REVIEW minor): `Dialog.css`
  hardcodes `background: rgba(16, 24, 40, 0.45)` for the backdrop. Extracting it to a
  `--color-overlay` token is correct, but `Dialog.css` is a SHARED, pre-existing
  component NOT created/modified by Phase 7 — changing it is out of this phase's
  scope. Deferred to a tokens-tidy pass.

## Optimistic-update vs slow/queued commit race (observed during browser validation)

- **Found during:** Phase 7 browser validation (folder delete).
- **Symptom:** When the single-writer git worker is SATURATED (cold start — the
  startup search-index rebuild hogs the worker; server logs `WARN pages: commit wait
  timed out; returning success, job stays queued timeout=5s`), an optimistic
  folder-delete's `onSettled` tree refetch can beat the still-queued commit and
  briefly RE-SHOW the deleted folder in the tree. The delete is correct on disk
  (verified: the trash group + restore worked) and the tree self-corrects on the
  next refetch.
- **Assessment:** Not a correctness bug — it only manifests under worker saturation;
  in steady state (fast commits) the refetch lands after the commit. It IS a visible
  flicker-back of the optimistic update.
- **Disposition:** Deferred. Future guard options: hold/skip the `["tree"]`
  `onSettled` invalidate while a commit job for that path is still queued, or have the
  tree endpoint read pending-but-uncommitted state, or lengthen the commit-wait under
  load. Do NOT block Phase 7.
