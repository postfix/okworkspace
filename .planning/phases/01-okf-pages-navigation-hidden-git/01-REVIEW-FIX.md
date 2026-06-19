---
phase: 01-okf-pages-navigation-hidden-git
fixed_at: 2026-06-19T00:00:00Z
review_path: .planning/phases/01-okf-pages-navigation-hidden-git/01-REVIEW.md
iteration: 1
findings_in_scope: 10
fixed: 10
skipped: 0
status: all_fixed
---

# Phase 1: Code Review Fix Report

**Fixed at:** 2026-06-19T00:00:00Z
**Source review:** .planning/phases/01-okf-pages-navigation-hidden-git/01-REVIEW.md
**Iteration:** 1

**Summary:**
- Findings in scope: 10 (6 Warning + 4 Info)
- Fixed: 10
- Skipped: 0

All four verification gates are GREEN after the fixes:
- `go build ./...`
- `go test ./... -race` (the okf golden-corpus `TestGoldenRoundTrip` exit gate stays green)
- `npm --prefix web run build`
- `npm --prefix web test -- --run` (12 files, 91 tests passed)

Regression tests were added for the behavioral fixes (WR-01, WR-03, WR-04, WR-05, WR-07).
`internal/web/dist/.gitkeep` was deleted by the Vite build and restored before finishing.
The pre-existing untracked `.smtc*` artifacts were left untouched.

## Fixed Issues

### WR-01: Trash DB/Git divergence on async commit failure

**Files modified:** `internal/pages/trash.go`, `cmd/okf-workspace/main.go`, `internal/pages/reviewfix_test.go`
**Commits:** `d17f99e` (fix), `67bf0e9` (test)
**Applied fix:** Added `Service.ReconcileTrash`, which prunes any `trash` row whose
`trash_path` is absent on disk (the divergence a failed async commit leaves behind),
and wired it as a best-effort startup pass in `main.go`. Per the engineering
constraint, this is the startup-reconciliation + prune approach (NOT a full
atomicity refactor), chosen to preserve the single write path (D-04) and the
existing trash tests. **Residual risk is documented on the method and below.**

### WR-03: Autosave overlapping saves self-409 / stale-closure content

**Files modified:** `web/src/routes/PageEditor.tsx`, `web/src/routes/PageEditor.test.tsx`
**Commit:** `46ea602`
**Applied fix:** Added a `saving` ref in-flight guard in `doSave` (overlapping
saves are dropped), removed the always-armed 6s idle timer, and escalate the
version save only after a draft save settles (single timer). `doSave` now reads
`body`/`frontmatter` from refs kept in sync on each edit, fixing the
`useCallback` stale-closure that could save stale content. Added a regression
test (a burst of Save clicks produces exactly one in-flight save) and added
`vi.clearAllMocks()` between tests to stop call-count pollution.

### WR-04: View/restore accept any hex token, not one belonging to THIS page

**Files modified:** `internal/pages/history.go`, `internal/pages/reviewfix_test.go`
**Commits:** `c08d697` (fix), `67bf0e9` (test)
**Applied fix:** Added `Service.tokenInHistory`, which lists the page's own
`--follow` history tokens and requires membership before `ShowAt`. Wired it into
both `ViewVersion` and `RestoreVersion` ahead of any blob read/restore. The token
stays opaque and the existing `ErrInvalidVersion` error shape is reused (no SHA is
leaked), honoring the hidden-Git contract.
**Status: fixed — requires human verification** (see Notes: a `git log --follow`
rename-heuristic subtlety means a cross-page token can occasionally pass the
membership check and instead be rejected by `ShowAt` with a 500 rather than the
clean `ErrInvalidVersion`; no disclosure occurs either way, which the regression
test asserts).

### WR-05: Push failure fails an already-committed job; isNonFastForward over-matches "rejected"

**Files modified:** `internal/pages/commitjob.go`, `internal/gitstore/push.go`, `internal/gitstore/push_test.go`
**Commit:** `657c298`
**Applied fix:** (1) In `CommitHandler`, a `g.Push` error after a durable commit is
now logged via `slog.WarnContext` and swallowed (returns nil) instead of failing
and retrying the whole job — push is best-effort/alert-only per VER-04. (2)
`isNonFastForward` now matches the bracketed `[rejected]` non-fast-forward marker
(plus `non-fast-forward`/`fetch first`) instead of a bare `"rejected"` substring,
so a server-side hook denial is treated as a real failure, not swallowed as
divergence. Added a unit test (`TestIsNonFastForward`) covering both directions.

### WR-07: CreateFolder has no empty-slug fallback (punctuation-only name → 500)

**Files modified:** `internal/pages/service.go`, `internal/pages/reviewfix_test.go`
**Commits:** `d180301` (fix), `67bf0e9` (test)
**Applied fix:** `CreateFolder` now returns `ErrTitleRequired` (a clean 400) when
the slug of a non-empty name is empty, mirroring the empty-title contract and
`uniquePath`'s fallback, instead of building an absolute `/index.md` the resolver
rejects as a 500. Added a regression test over several punctuation-only names plus
a happy-path guard.

### WR-08: Move trusts a raw new_parent path (traversal-shaped targets bypass slug safety)

**Files modified:** `internal/server/handlers_pages.go`
**Commit:** `369b2dd`
**Applied fix:** `handleRenamePage` now runs `cleanPathString` on `new_parent`
(rejecting absolute / NUL / `..` segments) before calling `Move`, so a
traversal-shaped destination fails with a clean 400 rather than relying solely on
the resolver to 500. This applies the same validation guard page paths already get
(SEC-01 defense-in-depth).

### IN-01: Folder-create audit target malformed for root-level folders

**Files modified:** `internal/server/handlers_pages.go`
**Commit:** `369b2dd`
**Applied fix:** The folder-create audit `Target` is now
`strings.Trim(req.Parent+"/"+req.Name, "/")`, so a root-level folder no longer
records a leading-slash `"/name"` target.

### IN-02: readField/setField interpolate the field name into a RegExp without escaping

**Files modified:** `web/src/lib/frontmatter.ts`, `web/src/lib/frontmatter.test.ts`
**Commit:** `b20d795`
**Applied fix:** Added `escapeRegExp` and apply it to the field name in both
`readField` and `setField` before building the RegExp, closing the latent
metacharacter footgun. Added regression tests for metacharacter field names.

### IN-03: Dead protocol-relative branch in isAbsoluteOrExternal

**Files modified:** `internal/okf/links.go`
**Commit:** `62790d7`
**Applied fix:** Removed the unreachable `len(dest) >= 2 && dest[0] == '/' &&
dest[1] == '/'` branch and folded its intent into a comment on the existing
`dest[0] == '/'` check (which already covers both absolute and protocol-relative).
Behavior is unchanged; the okf golden round-trip stays green.

### IN-04: ViewVersion returns LIVE HEAD revision for a historical version

**Files modified:** `internal/pages/history.go`
**Commit:** `c08d697`
**Applied fix:** Per the constraint ("at minimum document; only change behavior if
clearly correct"), strengthened the `ViewVersion` doc comment into an explicit
CONTRACT: the `Revision` on a version-view response is ALWAYS the live HEAD (never
the viewed version's identity), the view is read-only, and callers must not treat
it as an editable base. Behavior was intentionally NOT changed (the History panel
relies on the read-only view today; zeroing `revision` risked a behavioral
regression with no current bug).

## Notes / Residual Risk

- **WR-01 residual risk:** `ReconcileTrash` converges the two stores AFTER the
  fact, not atomically. A phantom trash row remains visible in `ListTrash` until
  the next reconcile pass (currently startup), and a Restore whose commit fails
  *after* its row was deleted is not re-created here (the page is still on disk but
  no longer listed). A full fix would record/delete the trash row from inside the
  commit handler; that refactor was deferred to keep the single write path and the
  existing trash tests intact, as the constraint permitted. This is documented in
  the method's doc comment.

- **WR-04 human verification:** the membership check uses `git log --follow`, whose
  rename-detection heuristic can pull an unrelated page's commit into a page's
  history on a small repo with near-identical content. In that case a cross-page
  token passes `tokenInHistory` but then fails at `ShowAt` (the path did not exist
  under that name at that commit) — surfacing as a 500 rather than the clean
  `ErrInvalidVersion`. The security property (no cross-page disclosure or restore)
  still holds and is asserted by the regression test
  (`TestVersionTokenMustBelongToPageHistory`), which verifies neither
  `ViewVersion` nor `RestoreVersion` ever returns another page's bytes. A developer
  should confirm whether tightening to an exact-blob/exact-path history check (or
  mapping the `ShowAt` not-found error to `ErrInvalidVersion`) is desired for a
  cleaner 4xx.

---

_Fixed: 2026-06-19T00:00:00Z_
_Fixer: Claude (gsd-code-fixer)_
_Iteration: 1_
