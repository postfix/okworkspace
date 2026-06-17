---
phase: 00-skeleton-auth-foundations
plan: 02
subsystem: storage-safety-spine
tags: [go, repo-resolver, sec-01, gitstore, single-writer, jobs, seed, react, health]

# Dependency graph
requires:
  - phase: 00-01
    provides: Go module, config loader (storage.repo_dir / git.* keys), shared SQLite store + migrations runner, chi server + Deps wiring, embedded React SPA (AppShell), users.BootstrapAdmin
provides:
  - internal/repo safe-path resolver (SEC-01) — the single filesystem chokepoint (EvalSymlinks + boundary-prefix + os.Root), fuzz-tested; Read/Write/MkdirAll/Exists/Tree + GitMetaPath all route through Resolve
  - internal/gitstore single-writer Git commit service (mutex-serialized exec.Command arg-slice CLI) — Init (idempotent), ff-only PullOnStartup, Commit (User/Action/Source provenance), Health, SelfHealStaleLock, IsEmpty; push deferred to Phase 1
  - internal/jobs async worker spine — SQLite-persisted FIFO queue, single drain goroutine (serialized), retry-with-backoff-then-fail; commit handler registered
  - internal/users.SeedStarterRepo — first-run SPEC §9 starter layout as ONE commit through the single-writer service; no-op on a non-empty/pulled repo
  - GET /api/v1/health repo-health endpoint (server HealthChecker interface, no gitstore import) + AppShell "Storage healthy" dot + self-heal / divergence warning banners
  - 0002_jobs.sql jobs table (+ status,run_after index)
affects: [phase-00 plan-03 (RBAC/admin reuses server Deps + health-gating), phase-1 (pages write through repo.Resolve + commit through gitstore single-writer; push lands here), phase-2 (attachments + extraction enqueue jobs), phase-3 (index jobs), phase-4 (agent patch-apply commits via the spine)]

# Tech tracking
tech-stack:
  added: []   # no new modules; reused stdlib (os.Root, os/exec, net/url) + existing yaml.v3, modernc sqlite, lucide-react, @tanstack/react-query
  patterns:
    - "internal/repo is the ONLY path-constructing code; grep for filepath.Join outside internal/repo/path.go returns 0 (GitMetaPath confines .git joins to the repo pkg)"
    - "Safe-path order: reject empty/NUL/percent-decoded-traversal/absolute/.. lexically, then EvalSymlinks longest existing prefix + separator-terminated boundary prefix, then os.Root OS-enforced refusal (Go 1.26 defense-in-depth)"
    - "Single-writer Git: one sync.Mutex serializes every git invocation; exec.Command arg slices (never sh -c); exit codes + stderr captured"
    - "Job worker: one drain goroutine = serialization; run_after stored as REAL fractional Unix epoch so sub-second backoff survives SQLite second-granularity datetime()"
    - "Server depends on a HealthChecker interface (adapter in main) so internal/server does not import internal/gitstore (one-directional deps)"
    - "Seed writes via repo.Write (resolver) + commits via gitstore.Commit — never a raw exec git call (D-10)"

key-files:
  created:
    - internal/repo/path.go
    - internal/repo/path_test.go
    - internal/repo/files.go
    - internal/gitstore/git.go
    - internal/gitstore/commit.go
    - internal/gitstore/health.go
    - internal/gitstore/gitstore_test.go
    - internal/jobs/queue.go
    - internal/jobs/worker.go
    - internal/jobs/worker_test.go
    - internal/users/seed.go
    - internal/users/seed_test.go
    - internal/store/migrations/0002_jobs.sql
    - internal/server/handlers_health.go
  modified:
    - cmd/okf-workspace/main.go
    - internal/server/router.go
    - web/src/api/client.ts
    - web/src/routes/AppShell.tsx
    - web/src/routes/AppShell.css

key-decisions:
  - "Resolver defensively percent-decodes input even though the handler passes a decoded path (belt-and-suspenders so an encoded %2e%2e%2f can never bypass the lexical traversal scan)"
  - "os.Root (Go 1.26) used as OS-enforced escape refusal in addition to EvalSymlinks + boundary prefix (Pitfall 5)"
  - "jobs.run_after is REAL fractional epoch seconds, not SQLite datetime text (datetime() truncates to whole seconds, breaking sub-second test backoff)"
  - "git fsck --connectivity-only (NOT --quick, which is a git gc flag) for the post-self-heal consistency check"
  - "Synthetic per-user commit identity (<user> <slug@okf-workspace.local>) so commits are attributable without real emails; Action/Source/User trailer in the message body (SPEC §14.2)"
  - "Repo.GitMetaPath confines .git-metadata path joining to the repo package, keeping the SEC-01 'no filepath.Join outside path.go' invariant literally true"

patterns-established:
  - "Startup order (main.go): config -> store -> migrate -> bootstrap admin -> repo init -> gitstore.Init -> SelfHealStaleLock -> PullOnStartup -> seed-if-empty -> job worker Start -> build server -> listen"
  - "Default repo_dir = <data_dir>/repo when storage.repo_dir is unset"

requirements-completed: [SEC-01]

# Metrics
duration: 12min
completed: 2026-06-17
---

# Phase 0 Plan 02: Storage & Safety Spines Summary

**The load-bearing storage + safety floor: a fuzz-tested single safe-path resolver (SEC-01), a mutex-serialized single-writer Git commit service with stale-lock self-heal and ff-only pull, a SQLite-persisted async job-worker spine, and a first-run seed that commits the SPEC §9 starter layout through the Git service — with repo health surfaced in the AppShell.**

## Performance

- **Duration:** ~12 min
- **Started:** 2026-06-17T16:57:37Z
- **Completed:** 2026-06-17T17:09:50Z
- **Tasks:** 3 (all TDD: RED -> GREEN)
- **Source files created/modified:** 19 (14 created, 5 modified)

## Accomplishments

- **SEC-01 safe-path resolver** (`internal/repo`): `Resolve` rejects empty / NUL / percent-decoded-traversal / absolute / `..` lexically, then `EvalSymlinks` the longest existing prefix and asserts a separator-terminated boundary prefix, then opens through `os.Root` as OS-enforced defense-in-depth. `Read`/`Write`/`MkdirAll`/`Exists`/`Tree`/`GitMetaPath` all route through it. **`FuzzResolve` ran 20s (~15.4M execs, 32 workers) with no escaping input.** The acceptance grep `filepath.Join` outside `internal/repo/path.go` returns **0** — nothing bypasses the chokepoint.
- **Single-writer Git service** (`internal/gitstore`): every git call goes through `exec.Command` arg slices (no shell) under one `sync.Mutex`. `Init` is idempotent (`-b <branch>` + repo-local identity), `PullOnStartup` is fast-forward-ONLY (sets a `diverged` flag and refuses to merge on divergence; no-op when the remote is disabled), `Commit` validates each path through the resolver and embeds `User`/`Action`/`Source` provenance, `SelfHealStaleLock` clears a stale `.git/index.lock` then `git status` + `git fsck --connectivity-only` to confirm consistency, `Health` reports OK/diverged/self-healed. **Push is deferred to Phase 1** (push grep returns 0).
- **Async job-worker spine** (`internal/jobs`): SQLite-persisted FIFO queue (`0002_jobs.sql`), one drain goroutine (the serialization guarantee), exponential backoff with a retry cap then a terminal `failed` state (never an infinite loop). A `commit` handler is registered this phase; Extract/Index handlers plug in later.
- **First-run seed** (`internal/users.SeedStarterRepo`): writes the SPEC §9 layout (root `index.md` + `runbooks`/`architecture`/`decisions` index pages with valid SPEC §10 OKF frontmatter + a `.okf-workspace/manifest.json` scaffold) via `repo.Write`, then commits it as ONE commit through `gitstore.Commit` (admin-authored, `Action=seed`, `Source=bootstrap`). Seeds ONLY when the repo is genuinely new and empty (`gitstore.IsEmpty`: no HEAD + no tracked files); a pulled/populated repo is left untouched.
- **Repo health surfaced**: `GET /api/v1/health` via a `server.HealthChecker` interface (adapter in `main`, so the server package never imports gitstore). AppShell shows a "Storage healthy" success dot and the UI-SPEC self-heal / Git-divergence warning banners.

## Task Commits

1. **Task 1: Safe-path resolver (SEC-01) with unit + fuzz tests** (TDD)
   - `08e4766` test(00-02): failing resolver unit + fuzz tests (RED)
   - `bb21cab` feat(00-02): safe-path resolver + file ops chokepoint (GREEN)
2. **Task 2: Single-writer Git commit service + stale-lock self-heal + async job worker** (TDD)
   - `ff8f7cf` test(00-02): failing gitstore + jobs tests, jobs migration (RED)
   - `d592203` feat(00-02): single-writer git service + self-heal + job worker (GREEN)
3. **Task 3: First-run repo seed via the Git spine + AppShell health indicator** (TDD)
   - `3fa5ded` test(00-02): failing first-run repo seed tests (RED)
   - `02c5ed8` feat(00-02): first-run repo seed + AppShell health (GREEN)
   - `0cb7c11` refactor(00-02): confine .git path joining to repo pkg via GitMetaPath

**Plan metadata:** committed separately (this SUMMARY + STATE/ROADMAP/REQUIREMENTS).

## Verification Results

- `CGO_ENABLED=0 go test ./internal/repo/... ./internal/gitstore/... ./internal/jobs/... ./internal/users/... -count=1` — **PASS** (and full `./internal/...` suite green).
- `go test ./internal/repo/ -run=Fuzz -fuzz=FuzzResolve -fuzztime=20s` — **PASS**, no escaping input found (~15.4M execs).
- `CGO_ENABLED=0 go build ./...` — clean; `go vet ./...` — clean; `go mod tidy` — no changes.
- `cd web && npm run build && npm run lint` — **exit 0** (SPA bundles to internal/web/dist; lint clean).
- Resolver-bypass grep `filepath.Join` outside path.go — **0**. Push grep in gitstore — **0**.

### Manual first-run verification (live binary, empty data dir)

A first run against a fresh `data_dir` (git.enabled, branch=main):

```
"msg":"admin user created — save this password, it will NOT be shown again"
"msg":"seeded starter repository layout"
"msg":"job worker started"
"msg":"listening"

# git log of the seeded repo:
c167407 admin: Seed starter workspace
# ls-files: .okf-workspace/manifest.json, architecture/index.md,
#           decisions/index.md, index.md, runbooks/index.md
# rev-list --count HEAD: 1
```

**Stale-lock self-heal** (planted `.git/index.lock`, then restart):

```
"msg":"recovered from an interrupted save (stale git lock cleared)"
# GET /api/v1/health -> {"ok":true,"diverged":false,"self_healed":true,
#                        "detail":"recovered from an interrupted save"}
# index.lock gone; rev-list --count HEAD still 1 (no re-seed on a non-empty repo)
```

## Decisions Made

- **Resolver percent-decodes defensively.** The plan says the handler passes a URL-decoded path; the behavior spec also requires `Resolve("%2e%2e%2f…")` to be rejected directly. The resolver therefore decodes any `%`-bearing input and re-validates — belt-and-suspenders so encoded traversal can never reach the lexical scan undecoded.
- **`os.Root` (Go 1.26) as OS-enforced escape refusal** in addition to EvalSymlinks + boundary prefix (Pitfall 5 calls for the evaluated real path to be the authority; os.Root adds a syscall-layer guarantee).
- **`jobs.run_after` is REAL fractional epoch seconds**, not SQLite `datetime` text — `datetime('now')` truncates to whole seconds, which would push sub-second test backoff past the test deadline and make ordering coarse.
- **`git fsck --connectivity-only`** for the post-self-heal check (`--quick` is a `git gc` flag, not `git fsck`).
- **`Repo.GitMetaPath`** confines `.git`-metadata path joining to the repo package so the "no `filepath.Join` outside `path.go`" invariant is literally satisfied (the lock path is built from constants, not user input).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] `git fsck --quick` is not a valid flag**
- **Found during:** Task 2 (gitstore self-heal test failed: `unknown option 'quick'`).
- **Issue:** `--quick` belongs to `git gc`, not `git fsck`; the self-heal consistency check errored.
- **Fix:** Switched to `git fsck --connectivity-only --no-progress`.
- **Verification:** `TestSelfHealStaleLock` passes; live restart self-heal confirmed.
- **Committed in:** `d592203`

**2. [Rule 3 - Blocking] SQLite second-granularity datetime broke sub-second job backoff**
- **Found during:** Task 2 (designing the retry schedule against the fast-config test).
- **Issue:** Storing `run_after` via `datetime('now', '+N seconds')` truncates to whole seconds, so a 10 ms backoff with a 3 s test deadline could not complete 3 attempts deterministically, and FIFO ordering was coarse.
- **Fix:** `run_after REAL` holding fractional Unix epoch seconds, computed in Go (`nowEpoch()`); claim query compares against `nowEpoch()`.
- **Verification:** `TestRetryWithBackoffThenFail` and `TestSerializedExecution` pass.
- **Committed in:** `d592203`

**3. [Rule 2 - Critical] Resolver hardened to reject percent-encoded traversal directly**
- **Found during:** Task 1 (GREEN) — the malicious-input table fed `%2e%2e%2f…` straight to `Resolve`.
- **Issue:** Relying solely on the handler to decode would let an encoded traversal bypass the lexical scan if any caller forgot to decode.
- **Fix:** `Resolve` decodes `%`-bearing input via `url.PathUnescape` and re-validates (also rejecting a NUL revealed by decoding).
- **Verification:** rejection table + 20 s fuzz pass.
- **Committed in:** `bb21cab`

**4. [Rule 1 - Bug] Build deleted the tracked SPA embed `.gitkeep`**
- **Found during:** Task 3 (after `npm run build`).
- **Issue:** Vite wipes `internal/web/dist/` before writing, deleting the tracked `internal/web/dist/.gitkeep` (built assets are gitignored; only `.gitkeep` is tracked).
- **Fix:** Restored `.gitkeep` with `git checkout --` and committed only source files (built assets stay gitignored, matching Plan 01's embed convention).
- **Verification:** `git ls-files internal/web/dist` shows `.gitkeep`; build still embeds successfully.
- **Committed in:** n/a (restoration, not a content change)

### Refactor
- **[criterion compliance] `Repo.GitMetaPath`** introduced so the only remaining `filepath.Join` outside `path.go` (the `.git/index.lock` path) moves into the repo package; the acceptance grep now returns 0. Committed in `0cb7c11`.

**Total deviations:** 3 auto-fixed (2 Rule 1 bug, 1 Rule 3 blocking) + 1 Rule 2 hardening + 1 build-artifact restoration + 1 criterion-compliance refactor. No scope creep; all within plan intent.

## Issues Encountered

- An external tool left `.planning/config.json` modified and dropped `.smtc*` artifacts in the working tree during execution; neither was authored by this plan and neither was staged in any task commit.
- `web/node_modules/flatted/golang/...` contains a stray `.go` file that appears in `go test ./...` package discovery (a node_modules artifact). It is outside `internal/` and out of scope; the plan's verification scopes tests to `./internal/...`.

## Known Stubs

- The job worker registers a **no-op `commit` handler** this phase — it is the spine; the real CommitJob/ExtractJob/IndexJob handlers land in Phases 1-3 (intentional per the plan: "register only a CommitJob handler this phase").
- The AppShell nav tree is still the **static placeholder** from Plan 01 (wiring the real seeded tree is Phase 1 page work, not this plan — this plan adds the health indicator/banners, which are fully wired to the live `/api/v1/health` payload).

## Threat Flags

None — all new surface (the `/api/v1/health` GET endpoint, the safe-path resolver, the git CLI shell-out, the single-writer model, the stale-lock self-heal, the ff-only pull) was anticipated in the plan's `<threat_model>` (T-00.02-01..07). No unplanned network endpoints, auth paths, or trust-boundary schema changes were introduced.

## User Setup Required

None — local-only Git by default (`git.remote_enabled=false`), no external service configuration. A `git` binary must be on PATH (LOCKED decision; part of the deploy trust boundary).

## Next Phase Readiness

- **Plan 03 (RBAC/admin/profile):** reuses the server `Deps` wiring and the live health endpoint; can gate `/admin` health visibility on role.
- **Phase 1 (pages):** writes content through `repo.Resolve` and commits through the `gitstore` single-writer service; **push lands here** (deferred from this phase). The byte-stable Markdown round-trip exit gate (carried in STATE) is still ahead.
- **Phases 2-4:** enqueue Extract/Index/agent-apply work through the `internal/jobs` spine; the worker's retry/serialization contract is established.

## Self-Check: PASSED

All 14 created files exist on disk; all 7 task commits (`08e4766`, `bb21cab`, `ff8f7cf`, `d592203`, `3fa5ded`, `02c5ed8`, `0cb7c11`) are present in git history.

---
*Phase: 00-skeleton-auth-foundations*
*Completed: 2026-06-17*
