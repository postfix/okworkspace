---
title: Phase 05 Collaboration — concurrency / race-condition stress test
date: 2026-06-23T19:57:39Z
method: 6 real Playwright browser sessions (distinct accounts) + focused API repro
harness: .smtc-cache/uat_stress.mjs, .smtc-cache/uat_stress2.mjs, .smtc-cache/repro_lostupdate.mjs
verdict: 6 TOCTOU races found (save, create, +4 structural) — ALL FIXED & retested green
status: RESOLVED 2026-06-23 (per-page + global structural mutex in pages.Service)
---

## Scope & method

Six distinct accounts (`admin, ed, alice, bob, carol, dave`, all editor) were logged
in through the real `/login` form in **six independent Playwright browser contexts**
(separate cookie jars / sessions). Conflicting operations — edit, save, delete-to-trash,
rename, move-page, move-folder, create, copy — were then fired **concurrently**
(`Promise.all` across sessions) against overlapping targets to provoke interleavings.

Bar for a bug: any `5xx`, a server crash, a corrupted git repo / unparesable tree, or a
**contradictory final state** (silent data loss, two pages at one path). Well-defined
losers (`409` conflict, `404` moved-away, collision) are correct outcomes, not bugs.

## Results

| # | Scenario | Outcome |
|---|----------|---------|
| S1 | 3-way concurrent save, same `base_revision` | **✗ BUG — all 3 returned 204** (expected 1×204 + 2×409) |
| S2 | save vs delete-to-trash | ✓ no 5xx; final state coherent |
| S3 | rename vs save (stale path) | ✓ no 5xx; stale save resolved cleanly |
| S4 | two concurrent renames of same page | ✓ no 5xx; resolved |
| S5 | two concurrent creates of same title | **✗ BUG — both 201 at the identical path** (silent clobber) |
| S6 | folder move vs edit page inside | ✓ no 5xx; resolved cleanly |
| S7 | save-as-copy vs delete source | ✓ no 5xx; copy independent of source delete |
| S8 | 6-user mixed-op storm (30 concurrent ops) | ✓ **zero 5xx**; status histogram `{200,201,204,400}` only |
| INV | post-storm health / tree / search | ✓ all 200; tree parses; git repo intact |

Both bugs reproduce **deterministically — 3/3 runs each** in the focused repro.

## BUG 1 — Lost update on concurrent save (severity: MAJOR)

Two+ saves that read the **same** `base_revision` and submit concurrently **all** succeed
(`204`); the optimistic-concurrency `409` never fires. The last commit wins and the other
writers' edits are **silently lost** — no conflict, no DiffReviewDialog. This directly
violates COLL-04's stated guarantee of *"no silent data loss on any path."* (Note: the
**sequential** case is correct — a stale save after a committed one does return 409, which
is what the original COLL-04 UAT exercised. The defect is specific to the concurrent window.)

**Root cause** — `internal/pages/service.go` `Save` (~L186–217): the revision check
```go
current, err := s.Revision(ctx, path)   // git rev-parse HEAD:<path> (blob SHA)
if current != baseRevision { return ErrStaleRevision }
... parse/repair/emit ...
s.enqueueWrite(ctx, path, out, "edit", user)   // commit happens later, single-writer
```
`s.Revision()` and `enqueueWrite()` are **not** under a common lock. `GitStore.mu`
serializes the individual git calls, and the job worker is single-writer, but two handler
goroutines both read `current == baseRevision` *before* either enqueues. Both then enqueue;
the single writer applies both in turn. The commit job (`commitjob.go`) re-validates nothing.

## BUG 2 — Silent clobber on concurrent create of same title (severity: MAJOR)

Two creates of the same title → same slug → `uniquePath()` calls `repo.Exists()` (a bare
`os.Stat`). Both concurrent calls see "doesn't exist", both return the **same** path, both
write it. Both handlers return `201 {path: "…/foo.md"}` — one page silently overwrites the
other instead of suffixing (`foo-2.md`) or rejecting.

**Root cause** — `internal/pages/service.go` `Create` → `uniquePath` (~L318–345): the
`Exists`-check / write interval is unguarded, same TOCTOU shape as Bug 1.

## Recommended fix (one root cause, two sites)

Make the precondition atomic with the write by moving it **inside the single-writer commit
job** (where `GitStore.mu` already serializes), or add a **per-path mutex** in
`pages.Service` held across check→`EnqueueAndWait`:
- Save: re-validate `base_revision` against the live blob SHA inside the commit job; return
  `ErrStaleRevision` from there so the 2nd concurrent writer 409s.
- Create: re-run `uniquePath()` inside the commit job so the 2nd writer gets `foo-2.md`.

A per-path lock keyed by page path is the smallest change and closes both windows; it also
naturally serializes rename/move/delete against save on the same path.

## What held up well

No 5xx anywhere (incl. a 30-op storm); save-vs-delete, rename-vs-save, double-rename,
folder-move-vs-edit, and copy-vs-delete all resolved to coherent states; git repo, page
tree, and search index were all intact and responsive afterward. The races are confined to
the two TOCTOU windows above and require sub-second concurrent timing (low probability at
~5 users, but a real correctness hole against the COLL-04 no-data-loss promise).

---

## Resolution (2026-06-23)

Root cause for ALL races was one TOCTOU pattern: `pages.Service` ran its precondition
check (revision for Save; uniqueness/existence for Create; source-present for
rename/move/delete/restore) and THEN enqueued the commit, with no lock spanning
check→commit. The git layer is single-writer, but two requests both passed the check
before either committed.

### Fix — `internal/pages/` (`pathlock.go` + service.go/rename.go/trash.go)
- Added a small reference-counted `keyedMutex` (`pathlock.go`).
- **Save**: takes a per-page key `"page:<path>"` across the revision check → commit, so
  independent pages still save concurrently but same-page saves serialize → the 2nd
  writer reads the now-current revision and correctly 409s.
- **All namespace mutations** (Create, CreateFolder, Rename, Move, RenameFolder,
  MoveFolder, Delete, DeleteFolder, Restore, RestoreGroup) take ONE global key
  `"mutation"` across check → commit. This is effectively free: commits were already
  globally serialized by the single-writer git mutex; we just extend that to the check.
- Reentrancy: `RestoreGroup` (which loops `Restore`) and `Restore` were split so both
  call an unlocked `restoreInner`; only the public entry points take the lock → no
  self-deadlock.

### Expanded coverage added (structural races R1–R7)
Beyond the original save/create, a second suite (`uat_stress2.mjs`) covers: double
rename of one page (R1), two pages → one title (R2), double delete (R3), rename-vs-delete
(R4), move-into-deleting-folder (R5), folder-rename-vs-folder-move (R6), double restore
(R7) — each asserting tree integrity (no dup/ghost/clobber, no 5xx).

### Retest — all green
| Suite | Result |
|-------|--------|
| `repro_lostupdate.mjs` (save + create) | 1 winner / 2×409; distinct create paths — 3/3 runs |
| `uat_stress2.mjs` (R1–R7 structural) | 23/23 invariants, 0 × 5xx |
| `uat_stress.mjs` (S1–S8 mixed storm) | 18/18 invariants, 0 × 5xx |
| Go regression tests (`-race`) | `TestSave_ConcurrentSameBaseRevision_OneWinner`, `TestCreate_ConcurrentSameTitle_DistinctPaths`, `TestRename_ConcurrentSamePage_NoDuplicate` — PASS |

Durable Go tests live in `internal/pages/concurrency_test.go` (real single-writer worker
drain via `CommitHandler`, start-barrier goroutines) so the fix can't silently regress.
