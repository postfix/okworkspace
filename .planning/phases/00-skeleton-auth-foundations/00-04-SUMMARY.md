---
phase: 00-skeleton-auth-foundations
plan: 04
subsystem: audit-config-packaging
tags: [go, audit, sec-05, slog, sqlite, config, env-overrides, systemd, docker, cgo-free, deployment]

# Dependency graph
requires:
  - phase: 00-01
    provides: shared SQLite store + migration runner, typed Config loader + applyEnvOverrides, chi server Deps wiring, login/logout handlers, users.BootstrapAdmin
  - phase: 00-02
    provides: first-run repo seed (users.SeedStarterRepo) + startup order in main.go
  - phase: 00-03
    provides: admin user-management handlers (create/role/reset/deactivate), self-service profile handlers, CLI admin reset-password, auth.CurrentUser session-bound user
provides:
  - internal/audit.Logger — Record(ctx, Event) dual-writes a SQLite audit_log mirror row AND one structured slog line per event; non-fatal on DB error; no secrets ever recorded (SEC-05)
  - audit_log table (0003_audit.sql) — operational-metadata mirror of who-did-what
  - audit wiring into all Phase-0 events — login/logout, user create/role-change/reset/deactivate, profile change, bootstrap, seed, CLI reset-password
  - completed internal/config schema — full SPEC §20.3 (server/storage/git/auth/agent/search/attachments) with defaults; OKF_DATA_DIR + OKF_LISTEN env overrides; api_key_env resolves OKF_LLM_API_KEY into an unexported, redacted-on-print field
  - deployment packaging — deploy/okf-workspace.service (systemd, non-root), deploy/Dockerfile (multi-stage, CGO_ENABLED=0 static, non-root runtime), deploy/README.md, top-level README.md
affects: [phase-1 (page/attachment changes extend the audit action set + write through the same Record), phase-4 (agent actions audited; agent.api_key_env feeds the Eino ChatModel via config.Agent.APIKey()), all phases (deployment artifacts are the self-host delivery vehicle)]

# Tech tracking
tech-stack:
  added: []   # no new Go/npm modules — reused stdlib log/slog + database/sql; Docker base images (node:20.19-bookworm-slim, golang:1.26-bookworm, alpine:3.21) are pinned but are build/runtime infra, not code deps
  patterns:
    - "audit.Record writes the slog line FIRST, then the SQLite mirror row — observability survives even if the DB write fails (non-fatal, T-00.04-03)"
    - "audit.Event has NO password/token field by construction — a secret cannot be recorded; callers pass only actor/target/short-detail/source"
    - "Handlers depend on a small auditRecorder interface; a nil Deps.Audit becomes a nopAudit so audit never breaks a request and tests can omit it"
    - "Resolved LLM API key lives in an UNEXPORTED config field; AgentConfig implements String()/GoString() that print APIKey:[REDACTED] so a logged Config can never leak the secret (T-00.04-02/05)"
    - "Deployment secrets (OKF_LLM_API_KEY) supplied via env/EnvironmentFile at runtime — never baked into the image or config.yaml"
    - "Runtime container base is a pinned minimal Alpine with git (not scratch/distroless) because single-writer Git shells out to the git CLI — still non-root"

key-files:
  created:
    - internal/audit/audit.go
    - internal/audit/audit_test.go
    - internal/store/migrations/0003_audit.sql
    - deploy/okf-workspace.service
    - deploy/Dockerfile
    - deploy/README.md
    - README.md
  modified:
    - internal/server/router.go
    - internal/server/handlers_auth.go
    - internal/server/handlers_users.go
    - internal/server/handlers_profile.go
    - internal/server/handlers_users_test.go
    - cmd/okf-workspace/main.go
    - cmd/okf-workspace/admin.go
    - internal/config/config.go
    - internal/config/config_test.go
    - config.example.yaml

key-decisions:
  - "audit.Record is non-fatal: a DB write error is logged at warn level and returned, but every caller ignores it (audit failure must never take down auth — T-00.04-03)"
  - "Event carries only non-secret provenance; one-time passwords / hashes / session tokens are NEVER passed to Record (T-00.04-02)"
  - "Bootstrap/seed are audited from main.go (actor=system) and CLI reset from admin.go (actor=cli) — avoids changing the existing BootstrapAdmin/SeedStarterRepo/ResetPassword signatures and their Plan-01/02/03 tests"
  - "Resolved API key is unexported + redacted in String()/GoString(); read only via config.Agent.APIKey() (defense against accidental %+v logging)"
  - "Runtime Docker base is pinned Alpine (ships git) rather than distroless/scratch, which contain no git — git is a hard runtime dependency of the locked single-writer Git design; image still runs as the non-root okf user"

patterns-established:
  - "Audit action constants (ActionLogin..ActionConfigChange) are the extensible event vocabulary; later phases add page/attachment/agent actions without touching existing wiring"
  - "Env override precedence: config.yaml < OKF_* env vars (OKF_DATA_DIR/OKF_LISTEN/OKF_ADMIN_USERNAME) for container/systemd deployment"

requirements-completed: [SEC-05]

# Metrics
duration: 14min
completed: 2026-06-18
---

# Phase 0 Plan 04: Audit, Config & Packaging Summary

**The cross-cutting security floor and the self-host delivery vehicle: a write-only SEC-05 audit log that dual-writes every key Phase-0 action to a SQLite `audit_log` mirror AND a structured slog line (never recording a secret, never able to break a request), the completed SPEC §20.3 config schema with deployment env overrides and a redacted-on-print resolved LLM API key, and runnable systemd + multi-stage Docker packaging that builds a non-root, CGO-free single static binary + data directory.**

## Performance

- **Duration:** ~14 min
- **Completed:** 2026-06-18
- **Tasks:** 2 (Task 1 TDD: RED → GREEN; Task 2 auto)
- **Source files created/modified:** 17 (7 created, 10 modified)

## Accomplishments

- **SEC-05 audit log** (`internal/audit`): `Logger.Record(ctx, Event)` emits one structured `slog.Info("audit", …)` line (action/actor/target/source/detail/at) **and** inserts one mirror row into `audit_log` (`0003_audit.sql`). It is **non-fatal** — the slog line is written first so observability survives a DB failure, and a mirror-write error is logged at warn level and returned but every caller ignores it (auth cannot go down because the audit mirror is briefly unwritable). `Event` has **no password/token field by construction**, so a secret can never be recorded.
- **Audit wiring into every Phase-0 event:** login + logout (`handlers_auth.go`, actor = username), admin user create / role-change / reset-password / deactivate (`handlers_users.go`, actor = session admin, target = managed user), self-service profile + password change (`handlers_profile.go`, `profile_change`), admin bootstrap and repo seed (`main.go`, actor = `system`), and the CLI `admin reset-password` (`admin.go`, actor = `cli`). Handlers use a small `auditRecorder` interface; a nil `Deps.Audit` becomes a `nopAudit`.
- **Completed config schema** (`internal/config`): the full SPEC §20.3 `server`/`storage`/`git`/`auth`/`agent`/`search`/`attachments` schema parses with defaults; `OKF_DATA_DIR` and `OKF_LISTEN` join `OKF_ADMIN_USERNAME` as deployment env overrides; `agent.api_key_env` resolves the named variable (e.g. `OKF_LLM_API_KEY`) into an **unexported** `apiKey` field readable only via `AgentConfig.APIKey()`, with `String()`/`GoString()` printing `APIKey:[REDACTED]` so a logged `Config` can never leak the secret.
- **Deployment packaging:** `deploy/okf-workspace.service` (systemd, dedicated non-root `okf` user, `WorkingDirectory=/var/lib/okf-workspace`, `Restart=always`, env-file secret, `ProtectSystem=strict`); `deploy/Dockerfile` (stage 1 `npm ci && npm run build` → stage 2 `CGO_ENABLED=0 go build` with the embedded SPA → final pinned minimal Alpine running as non-root `okf` with `git`); `deploy/README.md` (systemd + Docker run, data-dir layout, where to read the one-time admin password) and a top-level `README.md` (SPEC §20.1 build/run + first-run admin flow).

## Task Commits

1. **Task 1: Audit log (SEC-05) — SQLite mirror + slog line, wired into Phase-0 events** (TDD)
   - `a033b54` test(00-04): failing audit log tests + audit_log migration (RED)
   - `6465118` feat(00-04): audit log (SEC-05) + wire into Phase-0 events (GREEN)
2. **Task 2: Complete config.yaml schema + systemd + Docker deployment packaging**
   - `781c913` feat(00-04): complete config schema + env overrides + systemd/Docker packaging

**Plan metadata:** committed separately (this SUMMARY + STATE/ROADMAP/REQUIREMENTS).

## Verification Results

- **Automated gate (all green):** `CGO_ENABLED=0 go test ./internal/audit/... ./internal/server/... ./internal/users/... ./cmd/okf-workspace/... ./internal/config/... -count=1` — **PASS** (full `./internal/...` + `./cmd/...` suite green); `CGO_ENABLED=0 go build ./...` — clean; `go vet ./...` — clean; `gofmt -l` — clean; `go.mod`/`go.sum` — unchanged (no new dependencies, T-00.04-SC).
- **Deployment artifacts present:** `deploy/okf-workspace.service`, `deploy/Dockerfile` exist; Dockerfile is multi-stage (`npm run build` then `CGO_ENABLED=0 go build`) with a non-root `USER okf` final stage; `grep -q "CGO_ENABLED=0" deploy/Dockerfile` passes.
- **Audit unit behavior** (`internal/audit`): `TestRecordWritesRowAndLine` (exactly one row + a JSON slog line with matching action/actor/source), `TestRecordNeverLogsSecrets` (no `"password"`/`"token"`/`"session"`/`"hash"` key in the line), `TestRecordNonFatalOnDBError` (no panic on a closed DB; warning still logged) — all PASS.
- **Audit integration** (`internal/server`, `TestAuditRowsForLoginLogoutCreate`): a login, a logout, and an admin user-create each produce the corresponding `audit_log` row; the `user_create` row records `actor=admin`, `target=created`.

### Manual verification — audit_log rows + matching structured lines

The integration test exercises the real handlers and asserts the rows; the captured structured `audit` slog lines from that run (a login, a logout, an admin user-create) — note no secret fields:

```
INFO audit action=login       actor=auditee source=web-ui   detail=""           target=""
INFO audit action=logout      actor=auditee source=web-ui   detail=""           target=""
INFO audit action=login       actor=admin   source=web-ui   detail=""           target=""
INFO audit action=user_create actor=admin   source=web-ui   detail="role=reader" target=created
```

Equivalent `SELECT action, actor, target FROM audit_log` after the same flow:

```
login        | auditee | (null)
logout       | auditee | (null)
login        | admin   | (null)
user_create  | admin   | created
```

## Decisions Made

- **Non-fatal audit.** `Record` logs the slog line first and treats a mirror-write error as non-fatal — auth/admin paths never fail because the audit DB is briefly unwritable (T-00.04-03).
- **No secret can be recorded.** `Event` deliberately omits any password/token field; reset/bootstrap/CLI paths record only actor/target/source, never the one-time password (T-00.04-02).
- **Audit bootstrap/seed/CLI from the callers** (`main.go`, `admin.go`) rather than threading a logger into `BootstrapAdmin`/`SeedStarterRepo`/`ResetPassword` — keeps the Plan-01/02/03 signatures and their tests untouched.
- **Redacted API key.** The resolved LLM key is unexported and `AgentConfig.String()`/`GoString()` redact it; `APIKey()` is the sole reader (T-00.04-02/05).
- **Alpine runtime base, not distroless/scratch.** The locked single-writer Git design shells out to the `git` CLI, which scratch/distroless-static images lack; the runtime image is a pinned minimal Alpine with `git`+`ca-certificates`, still running as the non-root `okf` user (T-00.04-04, T-00.04-SC).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Runtime image needs `git`; scratch/distroless cannot satisfy it**
- **Found during:** Task 2 (Dockerfile final stage).
- **Issue:** CLAUDE.md suggests a `scratch`/`distroless` final stage, but the locked single-writer Git design requires a `git` binary on `PATH` at runtime (carried forward from Plan 02: "a git binary must be on PATH"). `scratch`/`distroless-static` contain no git, so the container would fail every commit.
- **Fix:** Final stage is a **pinned minimal `alpine:3.21`** that installs `git` + `ca-certificates` and runs as the non-root `okf` user — preserving the non-root single-binary promise (T-00.04-04) while satisfying the hard git dependency. Documented the tradeoff in `deploy/README.md`.
- **Verification:** `CGO_ENABLED=0 go build ./...` clean; Dockerfile passes the `CGO_ENABLED=0` + multi-stage + non-root checks; base images are pinned and `npm ci` uses the committed lockfile (T-00.04-SC).
- **Committed in:** `781c913`

---

**Total deviations:** 1 auto-fixed (1 Rule 3 blocking). No scope creep — the substitution is the minimal correct way to keep a non-root single-binary deploy that can actually run git.

## Issues Encountered

- The acceptance grep `grep -rn "api_key\|APIKey" internal/config | grep -i "slog\|log\.\|Print"` returns **1** line — but it is the redaction method `AgentConfig.String()`, which prints `APIKey:[REDACTED]` (the grep matches `APIKey` + `Sprintf`). This is the *opposite* of a leak: `TestAPIKeyNeverInStringOrLog` asserts the real secret never appears under `%v`/`%+v`/`%#v`. The companion grep `grep ... "api_key\|password" internal/audit internal/config | grep slog|log.|Print` (without the `APIKey` term) returns **0**. No code logs the resolved key.
- An external tool continues to leave `.planning/config.json` modified and `.smtc*` artifacts in the working tree; none were authored by this plan and none were staged in any commit.
- In `TestAuditRowsForLoginLogoutCreate` a helper first attempts to change the admin password with an unknown bootstrap password (an intentional, logged no-op) before logging in via a repo-driven reset (`loginAsAdmin`); the `change admin password fallback` log line is expected, not a failure.

## Known Stubs

- The audit log is **write-only** this phase — there is intentionally **no in-app viewer** (per 00-CONTEXT.md "Claude's Discretion: audit is write-only in Phase 0"). Rows are queryable via SQLite and lines via the structured log; a viewer UI is a later-phase concern.
- `agent`/`search`/`attachments` config sections remain **parsed-but-unused placeholders** (Phases 4/3/2). `config.Agent.APIKey()` is fully wired and resolves now; it is consumed by the Eino ChatModel in Phase 4. These are planned scope boundaries, not defects.

## Threat Flags

None — all new surface (the `audit_log` table + `audit.Record`, the resolved/redacted API key, the env overrides, the systemd unit, the Docker image) was anticipated in the plan's `<threat_model>` (T-00.04-01..05, T-00.04-SC). No new network endpoints, no new auth paths, no new npm/Go dependencies. The Alpine runtime base is a deliberate, documented substitution for the git dependency (still non-root), not an unplanned trust-boundary change.

## User Setup Required

- For systemd/Docker: supply `OKF_LLM_API_KEY` (or whatever `agent.api_key_env` names) via the environment **only when the agent is enabled** (Phase 4); Phase 0 ignores it. A `git` binary must be present at runtime (bundled in the Docker image; install on the host for systemd).

## Next Phase Readiness

- **Phase 1 (pages):** page/attachment-change events extend the audit action vocabulary and write through the same `audit.Record`; content writes still go through `repo.Resolve` + the single-writer `gitstore`. The byte-stable Markdown round-trip exit gate (carried in STATE) remains the Phase-1 gate.
- **Phase 4 (agent):** `config.Agent.APIKey()` + `BaseURL`/`Model` drive the Eino OpenAI-compatible ChatModel; agent actions get audited via new action constants.
- **Deployment:** the systemd unit and Docker image are the delivery vehicle for every subsequent milestone — no infra changes needed to ship later phases.

## Self-Check: PASSED

All 7 created files exist on disk (`internal/audit/audit.go`, `internal/audit/audit_test.go`, `internal/store/migrations/0003_audit.sql`, `deploy/okf-workspace.service`, `deploy/Dockerfile`, `deploy/README.md`, `README.md`); all 3 task commits (`a033b54`, `6465118`, `781c913`) are present in git history; the full Go test suite + build + vet are green.

---
*Phase: 00-skeleton-auth-foundations*
*Completed: 2026-06-18*
