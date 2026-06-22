---
phase: 05-collaboration
plan: 01
subsystem: collaboration
tags: [locks, soft-lock, presence, ttl, gc, go, repo-resolve, sec-01, jobs-worker]

# Dependency graph
requires:
  - phase: 02-storage-safety
    provides: "repo.Resolve SEC-01 chokepoint (Read/Write/Remove/MkdirAll/Exists) + os.Root traversal guard"
  - phase: 01-foundation
    provides: "jobs.Worker single-writer drain + Register/Handler/Enqueue; auth session keys"
provides:
  - "internal/locks package: file-backed, path-safe, clock-injected soft-lock store (Acquire/Refresh/Force/Release/Get/List/EditorsFor)"
  - "lock_gc job (KindGC + GCHandler + Service.GC) reaping expired lock files"
  - "auth.SessionConnectionIDKey session const (connection_id) for the client-supplied lock owner"
  - "main.go wiring: lockStore construction + lock_gc registration + ctx-gated GC ticker"
affects: [05-02 soft-lock HTTP slice, 05-03 presence/SSE slice]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Pure clock-injected service (now func() time.Time) mirroring pages.Service for deterministic TTL/expiry tests"
    - "Mirror-the-tree lock files under .okf-workspace/locks/{pagePath}.lock, all I/O via repo.* (never os.* on a lock path)"
    - "Two-layer path safety: repo.Resolve guards the repo ROOT; lockPath guards the locks SUBTREE (ErrUnsafePagePath)"

key-files:
  created:
    - internal/locks/lock.go
    - internal/locks/service.go
    - internal/locks/gc.go
    - internal/locks/service_test.go
  modified:
    - internal/auth/session.go
    - cmd/okf-workspace/main.go

key-decisions:
  - "lockPath rejects page paths whose .. segments cancel the locks/ prefix (repo.Resolve alone would accept them since they stay inside the repo root)"
  - "Force is lock-file-only — decouples 'who may type' from 'is the write safe' (save authority stays in pages.Save)"
  - "Torn/garbage lock = no live lock (self-heals next heartbeat); accepted per T-05-05 at 5 users"
  - "lockExpiry=2m, lockGCInterval=60s (interval < expiry so a crashed session reaps within ~one TTL window)"

patterns-established:
  - "Clock-injected pure service: NewService sets now=time.Now; tests overwrite svc.now with an advanceable fixed clock — zero time.Sleep"
  - "Subtree containment guard: a mirror-the-tree path must be re-checked against its subtree prefix, not only the repo root"

requirements-completed: [COLL-02]

# Metrics
duration: ~20min
completed: 2026-06-22
status: complete
---

# Phase 5 Plan 01: Soft-Lock Store Foundation Summary

**Server-authoritative, file-backed, clock-injected soft-lock store (`internal/locks`) with a deterministic 7-test suite, a `lock_gc` reaper job, `SessionConnectionIDKey`, and a ctx-gated GC ticker wired into `main.go` — the COLL-02 foundation for soft locks and presence.**

## Performance

- **Duration:** ~20 min
- **Started:** 2026-06-22T (plan execution start)
- **Completed:** 2026-06-22
- **Tasks:** 4
- **Files modified:** 6 (4 created, 2 modified)

## Accomplishments
- `internal/locks` package: `Lock`/`Owner`/`Editor` models + `Service` with `Acquire`/`Refresh`/`Force`/`Release`/`Get`/`List`/`EditorsFor`, an injected clock, and ALL I/O through `repo.*` (no raw `os.*` on a lock path).
- `lock_gc` job: `KindGC` const + `GCHandler(*Service)` + `Service.GC` reaping every expired lock file idempotently.
- `auth.SessionConnectionIDKey` const added beside `SessionUserIDKey`.
- `main.go`: `locks.NewService(contentRepo, lockExpiry)` constructed, `lock_gc` registered before `worker.Start`, and a ctx-gated `time.Ticker` (~60s) fire-and-forget enqueuing GC, cancelled on shutdown via the worker's ctx.
- Deterministic suite (injected clock, no Sleep): acquire lifecycle, held-by-other (no overwrite), refresh-holder-only, force-lock-only, expiry+GC, path-safety, torn-read fallback, EditorsFor self-exclusion — all green.

## Task Commits

Each task was committed atomically:

1. **Task 1: Lock model + lock-store service** - `eff23b0` (feat)
2. **Task 2: lock_gc job + SessionConnectionIDKey** - `836f24d` (feat)
3. **Task 3: Deterministic lock-store tests (+ subtree-escape guard)** - `a1400a3` (test)
4. **Task 4: Wire lock store + lock_gc + GC ticker into main.go** - `2a5f5e8` (feat)

_Task 1 is `tdd="true"`; its implementation and the Task 3 test suite together form the RED/GREEN cycle (the subtree-escape guard fix landed in the Task 3 commit alongside the failing-then-passing path-safety test)._

## Files Created/Modified
- `internal/locks/lock.go` - `Lock` (CONTEXT-locked JSON shape), `Owner` (server-trusted identity the HTTP layer fills from the session), `Editor` (presence snapshot row).
- `internal/locks/service.go` - `locks.Service`: clock-injected store; `lockPath` subtree guard; `Get`/`Acquire`/`Refresh`/`Force`/`Release`/`List`/`EditorsFor`; `walk()` helper that reads via `repo.Read` and yields repo-relative paths for GC.
- `internal/locks/gc.go` - `KindGC`, `Service.GC` (walk + `repo.Remove` expired locks), `GCHandler`.
- `internal/locks/service_test.go` - 7 deterministic tests with an advanceable fixed clock.
- `internal/auth/session.go` - `SessionConnectionIDKey = "connection_id"`.
- `cmd/okf-workspace/main.go` - `lockExpiry`/`lockGCInterval` consts, lock store construction, `lock_gc` registration, ctx-gated GC ticker goroutine.

## Decisions Made
- **Subtree containment guard (not just repo-root):** `repo.Resolve` only guarantees a path stays inside the repo ROOT. A crafted page path like `../../etc/passwd` cleans to `etc/passwd.lock` — inside the root but OUTSIDE `.okf-workspace/locks/`. `lockPath` now returns `ErrUnsafePagePath` when the cleaned mirror-tree path escapes the locks subtree. This is the chokepoint that makes `TestLockPathSafety` honest about the T-05-01 invariant ("a lock file can never escape `.okf-workspace/locks/`").
- **Walk reads via `repo.Read`:** the GC/presence subtree walk derives a repo-relative path and reads back through `repo.Read` (SEC-01) rather than `os.ReadFile` on the absolute path, so even the scan never touches a lock file outside the resolver. (`os.Stat` is still used to test the locks-dir existence — that is a directory probe, not a lock-file read/write/remove.)
- **Force is lock-file-only:** `Force` calls only the lock-write path; it has zero reference to any page body or revision. `TestForceTakesOwnershipLockOnly` asserts the on-disk file count is unchanged (it overwrites the same `.lock`).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing Critical] Added lock-subtree containment guard to lockPath**
- **Found during:** Task 3 (path-safety test)
- **Issue:** The plan's lockPath was a plain string join (`.okf-workspace/locks/ + pagePath + .lock`) relying solely on `repo.Resolve`. But a traversal-shaped page path (`../../etc/passwd`) cleans to a path that stays inside the repo root yet escapes the locks subtree, so `repo.Resolve` accepted it and `TestLockPathSafety` (which the plan mandates must error) failed. The threat model (T-05-01) requires a lock to never escape `.okf-workspace/locks/`, so this is a correctness/security requirement, not scope creep.
- **Fix:** `lockPath` now returns `(string, error)`, `filepath.Clean`s the mirror-tree path, and returns `ErrUnsafePagePath` unless the result is still under `lockSubtree`. All callers (`Get`/`Release`/`write`) propagate the error; `Acquire` inherits it via `Get`. Also converted the walk's per-file read from `os.ReadFile` to `repo.Read` so the verification grep for raw fs ops on lock paths is empty.
- **Files modified:** internal/locks/service.go
- **Verification:** `TestLockPathSafety` passes (traversal path errors, nothing escapes `t.TempDir()`); `grep 'os\.\(Read\|Write\|Remove\)File' internal/locks/*.go` is empty.
- **Committed in:** `a1400a3` (Task 3 commit)

---

**Total deviations:** 1 auto-fixed (1 missing-critical security guard)
**Impact on plan:** The guard is required for the plan's own mandated `TestLockPathSafety` and the T-05-01 invariant. No scope creep — no new dependencies, no new surface; it tightens the existing path chokepoint.

## Issues Encountered
- The lock record carries no page path, so GC could not reconstruct the lock-file path from a `Lock`. Resolved by adding a private `walk()` helper that pairs each parsed lock with its repo-relative path; `GC` removes by that path via `repo.Remove`, and `List` projects to `[]Lock`.

## User Setup Required
None - no external service configuration required (zero new dependencies this slice).

## Next Phase Readiness
- `lockStore` is constructed and GC-wired in `main.go`, in scope for Slices 2/3 to pass to new HTTP/SSE handlers (no routes added this slice, per plan).
- Slice 2 (soft-lock HTTP) must fill `Owner.Username`/`UserID` FROM THE SESSION and pass the client `connection_id` as `Owner.SessionID` (the SHAPE is fixed so a client-named username path cannot be introduced).
- `SessionConnectionIDKey` exists for the connection-id plumbing.

---
*Phase: 05-collaboration*
*Completed: 2026-06-22*

## Self-Check: PASSED

- Files created/modified all present on disk (6/6).
- Task commits all present in git history (eff23b0, 836f24d, a1400a3, 2a5f5e8).
- `go test ./internal/locks/ -count=1` green (7/7); `CGO_ENABLED=0 go build ./...` + `go vet` clean; no raw `os.*File` on lock paths.
