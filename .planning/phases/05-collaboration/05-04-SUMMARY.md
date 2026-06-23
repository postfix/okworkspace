---
phase: 05-collaboration
plan: 04
subsystem: collaboration / conflict-resolution
status: complete
tags: [COLL-03, COLL-04, conflict, optimistic-concurrency, force-edit, save-as-copy]
requires:
  - "05-01: locks.Service.Acquire/Force (force-edit is lock-only)"
  - "05-02: PageEditor lock lifecycle + Force edit wiring"
  - "Phase 4: DiffReviewDialog trust-gate shell (focus-inversion, real-diff-always)"
provides:
  - "TestForceEditStillRejectsStaleSave — the COLL-03 load-bearing proof"
  - "TestSaveAsCopyLeavesOriginal — COLL-04 backend proof"
  - "DiffReviewDialog conflict mode (3-button risk-ranked footer, safe-focus)"
  - "PageEditor 409→conflict-dialog + Overwrite/Manual-merge/Save-as-copy handlers + autosave gating"
affects:
  - "web/src/routes/PageEditor.tsx (additive: conflict UI alongside 05-02 lock + 05-03 presence)"
  - "web/src/components/DiffReviewDialog.tsx (additive conflict branch; review mode unchanged)"
tech-stack:
  added: []
  patterns:
    - "Force-edit is LOCK-ONLY: the pages.Save revision check is the sole data-loss authority, never bypassed."
    - "Save-as-copy = Create(deduped path) + Save(newPath, fresh rev) — the original is never written."
    - "Conflict UI extends (never forks) DiffReviewDialog; safe choice owns initial focus, never Overwrite."
key-files:
  created:
    - internal/pages/forceedit_test.go
    - internal/pages/saveascopy_test.go
    - web/src/routes/PageEditor.conflict.test.tsx
  modified:
    - web/src/components/DiffReviewDialog.tsx
    - web/src/components/DiffReviewDialog.css
    - web/src/components/DiffReviewDialog.test.tsx
    - web/src/routes/PageEditor.tsx
    - web/src/routes/PageEditor.css
    - web/src/routes/PageEditor.test.tsx
decisions:
  - "pages.Service.Save is UNCHANGED — the 409 floor (service.go:200) is reused, not modified; the slice proves and consumes it."
  - "Manual merge advances baseRevision to the server's revision and surfaces the server body in a collapsible reference pane, so a normal Save after reconciling does not instantly re-409."
  - "Save-as-copy navigates via useNavigate to /app/edit/{newPath}; the original page is never written and never carries the conflicted base revision."
metrics:
  duration: "~25m"
  completed: "2026-06-22"
  tasks: 4
  files: 9
---

# Phase 5 Plan 4: Conflict Resolution + Force-Edit Safety Proof Summary

Delivered COLL-04 (a real-diff conflict dialog with three risk-ranked, safe-by-default choices — Overwrite / Manual merge / Save as copy — all routed through the existing revision-checked save path) and hardened+proved COLL-03 (force-edit is lock-only and a stale forced save still 409s), with `pages.Service.Save` left untouched.

## What was built

- **Task 1 — `TestForceEditStillRejectsStaleSave` (COLL-03, load-bearing).** A `locks.Service` over the same repo as the `pages.Service`: A acquires the lock, B force-takes it (asserting Force changed only the lock — page revision and body unchanged), a real commit lands at rev0→rev1, then B saves with the now-stale rev0 → `ErrStaleRevision`, and the stale body never lands. Control: B saves at rev1 → succeeds. This proves force-edit never bypasses the per-document revision check, by construction (Save reads the committed revision itself; nothing in the save path is lock-aware).
- **Task 2 — `TestSaveAsCopyLeavesOriginal` (COLL-04 backend).** Create(deduped sibling) + Save(newPath, mine) at its fresh revision; the original's body and committed revision are byte-identical afterward; a repeat copy auto-dedups via `uniquePath`.
- **Task 3 — `DiffReviewDialog` conflict mode.** Added `mode?: "review" | "conflict"` (default review, unchanged). Conflict mode renders a 3-button risk-ranked footer: Overwrite (`.btn-ghost-destructive`, the only destructive control, positionally isolated with a worded risk sub-line) and the safe pair (Manual merge + Save as copy, DOM-first so the focus trap lands on a safe choice). Initial focus targets Save as copy — never Overwrite (the Phase 4 focus-inversion preserved + commented). Real diff always rendered; identical-versions guard shows the calm message + a single Save. Verbatim UI-SPEC copy + aria-labels; no Git vocabulary; token-only CSS.
- **Task 4 — `PageEditor` wiring.** A 409 (explicit Save or autosave) fetches the current server version and opens the conflict dialog (old=server, new=mine), superseding the Phase 1 banner; `baseRevision` is not advanced so edits stay intact. Autosave is gated on an open conflict (`conflictOpenRef`) to prevent debounce thrash. Overwrite saves the original at the freshly-fetched revision and re-opens on a re-409; Save-as-copy creates+saves a new page and navigates; Manual merge keeps my body with the server version visible. The 05-02 lock and 05-03 presence wiring remain intact (conflict UI is purely additive).

## Deviations from Plan

None — plan executed exactly as written. `pages/service.go` was not modified (verified via `git diff`).

## Self-Check: PASSED

All created files exist on disk; all four task commits are present in `git log` (`6c07d9a`, `184b0f0`, `010acbc`, `006fbe9`).

Verification commands and results:

- Backend: `go build ./...` → OK · `go vet ./...` → OK · `go test ./...` → all packages ok (internal/pages 3.5s incl. both new tests).
- Load-bearing: `go test ./internal/pages/ -run 'TestForceEditStillRejectsStaleSave|TestSaveAsCopyLeavesOriginal' -count=1` → both PASS.
- Frontend: `cd web && npm run test` → 33 files, **289 tests passed** (incl. DiffReviewDialog conflict tests + PageEditor.conflict.test.tsx + updated PageEditor 409 test).
- Type-check: `cd web && npx tsc --noEmit` → clean.
- `git diff internal/pages/service.go` (across the slice) → empty (the 409 floor is reused, not modified).
- `grep -niE 'merge conflict|commit|HEAD|branch|SHA|repo|git'` over DiffReviewDialog.tsx conflict copy → no user-facing Git vocabulary (only the CSS class names `dialog-head`/`stale-heading`).

## Threat mitigations verified

- **T-05-15** (force-edit never bypasses the revision check) — `TestForceEditStillRejectsStaleSave`.
- **T-05-16** (Overwrite race) — Overwrite fetches the current rev then saves at it; a re-409 re-opens with the newer server version (`onConflictOverwrite` + conflict test).
- **T-05-17** (Save-as-copy never touches the original) — `TestSaveAsCopyLeavesOriginal` + the conflict test asserting only the initial 409'd save hits the original path.
- **T-05-18** (default focus never on Overwrite) — `DiffReviewDialog.test.tsx` conflict focus assertion.
- **T-05-19** (XSS) — diff/old/new + copy title render via React/react-diff-viewer (escaped); title is plain text fed to `createPage` (path-validated server-side).
