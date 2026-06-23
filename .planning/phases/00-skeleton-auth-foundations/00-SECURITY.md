---
phase: 00
slug: skeleton-auth-foundations
status: verified
threats_open: 0
asvs_level: 1
created: 2026-06-18
---

# Phase 00 — Security

> Per-phase security contract: threat register, accepted risks, and audit trail.
> Register authored at plan time across the four PLAN `<threat_model>` blocks
> (00-01..00-04); each disposition verified against the implemented code by the
> gsd-security-auditor (read-only). Documentation/intent was not accepted as
> evidence — every `mitigate` has a code/grep match at the cited location.

---

## Trust Boundaries

| Boundary | Description | Data Crossing |
|----------|-------------|---------------|
| Browser (SPA) → Go HTTP API | Untrusted client input crosses here | Login credentials, CSRF token, cookies |
| HTTP handler → SQLite store | User-controlled username reaches a parameterized query | Username, profile fields |
| Server process → log / stdout | One-time bootstrap/reset password emitted here by design | Single-use credentials |
| HTTP path param → filesystem | User-facing path strings joined to repo root (traversal surface) | File paths |
| Filesystem symlinks → repo root | A symlink inside the repo can point outside it | File paths |
| Multiple writers → Git index | Concurrent `git` invocations contend on `.git/index.lock` | Commit intents |
| Optional Git remote → local repo | A diverged remote could clobber local content on pull | Repo content |
| Authenticated user → admin-only API | A non-admin must not perform user-management actions | Role assertion |
| User → own vs others' accounts | A user may edit only their own profile/password, never role | Profile/password |
| Application events → audit store | Record who did what without leaking secrets | Actor/target/action |
| Config/env → running process | Resolved LLM API key and session secrets must never reach logs | API key, session secret |
| Container/host → process | Process runs as non-root with a scoped data directory | Process identity |

---

## Threat Register

| Threat ID | Category | Component | Disposition | Mitigation | Status |
|-----------|----------|-----------|-------------|------------|--------|
| T-00.01-01 | Spoofing | `POST /auth/login` | mitigate | Argon2id `ComparePasswordAndHash` (constant-time); no bcrypt — `auth/password.go:13,19` | closed |
| T-00.01-02 | Info disclosure | login error path | mitigate | Single `ErrInvalidCredentials` + dummy-hash compare on unknown user — `auth/auth.go:50-68` | closed |
| T-00.01-03 | Tampering/CSRF | mutating auth routes | mitigate | nosurf outermost on all unsafe methods — `middleware.go:16-29`, `router.go:107-109` | closed |
| T-00.01-04 | Session theft | session cookie | mitigate | HttpOnly+SameSite=Lax+Secure-when-TLS; `RenewToken` on login — `session.go:39-42`, `handlers_auth.go:98` | closed |
| T-00.01-05 | EoP | admin bootstrap | mitigate | crypto/rand 28-char OTP, Argon2id, must_change=1, rejection sampling — `bootstrap.go:17,28-90` | closed |
| T-00.01-06 | Info disclosure | server log/stdout | mitigate | Only bootstrap/reset OTP logged by design; no plaintext on login/verify — `main.go:121`, `admin.go:54` | closed |
| T-00.01-07 | Tampering/SQLi | username → SQLite | mitigate | All `?` placeholders; no string-built SQL — `users.go` | closed |
| T-00.01-SC | Supply chain | go.mod / npm | mitigate | Locked deps; no `mattn/go-sqlite3`/`bcrypt` import; lockfiles clean | closed |
| T-00.02-01 | Tampering/EoP | `repo.Resolve` | mitigate | Lexical reject + os.Root + EvalSymlinks boundary; `FuzzResolve` — `repo/path.go:83-197` | closed |
| T-00.02-02 | Tampering | symlink escape | mitigate | EvalSymlinks real path + `withinRoot` boundary prefix — `repo/path.go:98-107` | closed |
| T-00.02-03 | DoS | concurrent git → index.lock | mitigate | Single mutex + single worker drain — `gitstore/git.go:28,49-60`, `jobs/worker.go:84-109` | closed |
| T-00.02-04 | DoS/availability | stale index.lock | mitigate | Startup self-heal clears stale lock + `git status`/`fsck` — `gitstore/health.go:29-61` | closed |
| T-00.02-05 | Tampering/data loss | diverged remote pull | mitigate | `merge --ff-only`; on non-ff set diverged, skip seed — `gitstore/git.go:100-122` | closed |
| T-00.02-06 | Cmd injection | git CLI shell-out | mitigate | `exec.CommandContext("git", args...)` arg slices; no shell — `gitstore/git.go:50` | closed |
| T-00.02-07 | Repudiation | commit authorship | mitigate | User/Action/Source on every commit; seed = admin identity — `commit.go:13-84`, `seed.go:95-101` | closed |
| T-00.02-SC | Supply chain | git binary | accept | AR-1 — host git binary in deploy trust boundary | closed |
| T-00.03-01 | EoP | admin-only routes | mitigate | `RequireRole` reads role from session only; 401/403 — `rbac.go:56-71`, `router.go:84` | closed |
| T-00.03-02 | EoP | self-service profile | mitigate | `UpdateOwnProfile`/`ChangeOwnPassword` no role param, id from session — `manage.go:219-250` | closed |
| T-00.03-03 | Tampering/CSRF | admin+profile mutations | mitigate | Under global nosurf in addition to RBAC — `router.go:77-90,109` | closed |
| T-00.03-04 | EoP | forced-pw-change bypass | mitigate | Server-side gate rejects all routes except change-password — `handlers_users.go:77-80` (CR-01) | closed |
| T-00.03-05 | Spoofing | password reset credential | mitigate | Strong OTP, Argon2id, must_change=1 — `manage.go:168-189` | closed |
| T-00.03-06 | DoS | deactivated account | mitigate | `active=0`; `Authenticate` + per-request check reject inactive — `manage.go:195-215`, `auth.go:65` | closed |
| T-00.03-07 | Info disclosure | OTP in operator log | accept | AR-2 — OTP to operator log only, never to audit | closed |
| T-00.03-SC | Supply chain | new npm/go deps | mitigate | No new deps; hand-built UI primitives; lockfiles unchanged | closed |
| T-00.04-01 | Repudiation | actions without trail | mitigate | `audit.Record` dual-writes slog + `audit_log` row at all event sites — `audit/audit.go:83-118` | closed |
| T-00.04-02 | Info disclosure | secrets in audit/logs | mitigate | `Event` has no secret field; API key redacted — `audit/audit.go:44-60`, `config.go:98-104` | closed |
| T-00.04-03 | Availability | audit failure breaks auth | mitigate | `Record` non-fatal; `nopAudit` fallback — `audit/audit.go:89-117` | closed |
| T-00.04-04 | EoP | container runs as root | mitigate | systemd `User=okf`; Dockerfile non-root `okf` — `service:29-30`, `Dockerfile:44,50` | closed |
| T-00.04-05 | Info disclosure | env var in image | mitigate | API key env-resolved at runtime, redacted, not baked — `config.go:139-143,90,98-104` | closed |
| T-00.04-SC | Supply chain | Docker base + npm ci | mitigate | Pinned bases; `npm ci` on committed lockfile; `CGO_ENABLED=0` — `Dockerfile:15,27,40` | closed |

*Status: open · closed*
*Disposition: mitigate (implementation required) · accept (documented risk) · transfer (third-party)*

---

## Accepted Risks Log

| Risk ID | Threat Ref | Rationale | Accepted By | Date |
|---------|------------|-----------|-------------|------|
| AR-1 | T-00.02-SC | Host `git` CLI is part of the deploy trust boundary. LOCKED architectural decision (CLAUDE.md). No shell string is ever constructed (`exec.CommandContext("git", args...)`, arg slices only — T-00.02-06), so residual exposure is the integrity of the `git` binary, same trust position as the OS. Dockerfile installs a pinned `git`; systemd unit documents the PATH requirement. | deployer (host/image hardening) | 2026-06-18 |
| AR-2 | T-00.03-07 | One-time passwords printed once to the operator log on bootstrap (`main.go:121`) and admin/CLI reset (`admin.go:54`) so a self-hoster can recover access (D-01/D-04). Bounded: strong single-use credential forcing change on first login (must_change_password=1, server-enforced — T-00.03-04); NEVER written to the audit trail (T-00.04-02); plaintext never logged on interactive login/verify (T-00.01-06). Exposure limited to operators who already hold host-log access. | operator (log retention/rotation) | 2026-06-18 |

*Accepted risks do not resurface in future audit runs.*

---

## Security Audit Trail

| Audit Date | Threats Total | Closed | Open | Run By |
|------------|---------------|--------|------|--------|
| 2026-06-18 | 30 | 30 | 0 | gsd-security-auditor (opus) |

---

## Prior Review Cross-Check

`00-REVIEW.md` (3 BLOCKER + 7 WARNING + 4 INFO) is marked resolved; the
security-relevant resolutions were re-verified in code and either strengthen or
are neutral to the dispositions above:

- **CR-01** server-side `must_change_password` gate — `handlers_users.go:77-80` — strengthens T-00.03-04.
- **CR-02** modulo-bias removed from credential generator (rejection sampling) — `bootstrap.go:72-90` — strengthens T-00.01-05 / T-00.03-05.
- **CR-03** last-admin lockout guard — `manage.go:140-162,195-215`, `users.go:106-113` — availability hardening, no regression.
- **WR-01** username validation feeding git author identity — `manage.go:58-68,96` — hardens T-00.02-07.
- **WR-02** session revocation on deactivate/reset — `users.go:124-154`, `manage.go:187,213` — hardens T-00.03-06.
- **WR-07** single source of truth for cookie `Secure` flag — `session.go:23-25,42`, `router.go:107-109` — hardens T-00.01-03/04.

The 4 deferred INFO items (IN-01..IN-04) map to no registered threat for this
phase. IN-04 (`repo.Tree` symlink listing) is adjacent to T-00.02-01/02 but
correctly deferred — `Tree` is used only for seed verification this phase; all
content read/write paths route through the `Resolve` chokepoint that
T-00.02-01/02 verify is intact.

---

## Unregistered Flags

None. All four SUMMARYs declare `## Threat Flags: None`. Every piece of new
attack surface (`GET /api/v1/health`, safe-path resolver, git CLI shell-out,
single-writer model, stale-lock self-heal, ff-only pull, admin/profile API,
RBAC middleware, forced-password-change gate, CLI reset, OTP credential path,
`audit_log` table + `audit.Record`, resolved/redacted API key, env overrides,
systemd unit, Docker image) maps to a registered threat ID. No unplanned
network endpoint, auth path, or trust-boundary schema change appeared.

---

## Sign-Off

- [x] All threats have a disposition (mitigate / accept / transfer)
- [x] Accepted risks documented in Accepted Risks Log
- [x] `threats_open: 0` confirmed
- [x] `status: verified` set in frontmatter

**Approval:** verified 2026-06-18
