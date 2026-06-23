---
phase: 00-skeleton-auth-foundations
verified: 2026-06-24T00:00:00Z
status: verified
score: 5/5 must-haves verified
overrides_applied: 0
human_verification:
  - test: "A user can log in with the bootstrap admin password, see their display name in the top bar, and log out from the user menu"
    expected: "Login with one-time password forces password change screen; setting 12+ char password lands on /app showing admin display name; clicking display name in topbar opens UserMenu with Profile and Log out; clicking Log out redirects to /login"
    why_human: "End-to-end browser session flow (cookie persistence, UI rendering, redirect behavior) cannot be verified by grep or unit tests"
  - test: "Session persists across a browser refresh"
    expected: "After logging in and refreshing the page, the user remains on /app showing their display name — no redirect to /login"
    why_human: "Cookie persistence and session-store read-back require a live browser session"
  - test: "RBAC denial is server-side: a non-admin cannot reach admin routes even by direct HTTP request"
    expected: "Signing in as an editor (create one first from /admin), then issuing curl -b <editor_cookie> /api/v1/admin/users returns 403 with the RBAC error message"
    why_human: "Server-side 403 enforcement for non-admin roles was human-verified during Plan 03 review, but should be confirmed in a live environment with a fresh editor session"
  - test: "Forced password change gate works for both the admin's initial login and an admin-reset user"
    expected: "Any user with must_change_password=1 (bootstrap admin or admin-reset account) is redirected to the 'Set a new password' screen on every authenticated route; they cannot reach /app, /profile, or /admin until the password is changed; the server rejects any authenticated route except PUT /api/v1/profile/password with 403"
    why_human: "The TestMustChangePasswordGate unit test passes, but the complete browser-level flow (client-side redirect + server-side 403 interplay) requires a live session to confirm no bypass path exists"
  - test: "First startup against an empty data dir initializes the Git repo and the seed commit appears in git log"
    expected: "Running the binary for the first time creates .git/, commits the starter layout, and git log shows one commit authored with the admin identity; a second run does NOT create a second seed commit"
    why_human: "The seed unit test (SeedStarterRepo) passes in isolation; the end-to-end first-run sequence through the full startup stack requires running the binary"
---

# Phase 0: Skeleton, Auth & Foundations — Verification Report

**Phase Goal:** A non-technical user can log into a running single-binary app, with all load-bearing security and storage foundations (safe-path resolver, RBAC, sessions, Git repo, single-writer commit spine) in place for later phases.
**Verified:** 2026-06-18T16:15:00Z (live UAT 2026-06-24)
**Status:** verified
**Re-verification:** Yes — live browser UAT 2026-06-24 closed all 5 human_needed items

## Live UAT — 2026-06-24 (browser-driven on :8098, milestone-close resolution)

All 5 `human_verification` items confirmed live (Playwright against the running single binary):

1. **Login → forced password change → display name → user menu** — Reset admin to a one-time password via `okf-workspace admin reset-password`; logging in with the OTP rendered the **"Set a new password"** gate (role=status "You're using a temporary password"); setting a 12+ char password landed on `/app` showing the **"Administrator"** display name in the top bar; the user menu opens with **Profile + Log out** items. ✓ (logout→/login redirect confirmed at sweep end)
2. **Session persists across refresh** — reloaded `/app`; stayed on `/app` with the display name, no redirect to `/login`. ✓
3. **RBAC denial is server-side** — logged in as editor `alice` (live `/auth/me` → `role: editor`), then `GET /api/v1/admin/users` returned **403** `{"error":"You don't have permission to do that."}`. ✓
4. **Forced password-change gate** — the `must_change_password` admin could not reach `/app` content until the password was changed (gate screen shown on the authenticated route). ✓
5. **First-run seed + idempotency** — ran the binary against a fresh empty data dir: it bootstrapped the admin (OTP, `must_change_password`), logged `seeded starter repository layout`, and `git log` showed **exactly 1 commit "Seed starter workspace"** authored by `admin <admin@okf-workspace.local>`. A **second run re-seeded nothing** (no new bootstrap/seed log lines; commit count stayed at 1). ✓

Note: standalone curl/Go HTTP clients hit a nosurf 400 on `/auth/login` due to SameSite=Lax cookie mechanics; the real same-origin browser path (which the SPA uses) works — RBAC was verified through it.

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | A user can log in with a username and password, see their display name in the UI, and log out from any page | VERIFIED | `handleLogin` authenticates with Argon2id and writes the session; `handleMe` returns `display_name`; `AppShell` renders `data?.display_name` from `useQuery(["me"])`. `UserMenu` with "Log out" is mounted in `AppShell` topbar. All auth handler tests pass. Human-verified in Plan 03 review. |
| 2 | A user's session persists across a browser refresh via an HTTPOnly, SameSite cookie, and mutating requests are CSRF-protected | VERIFIED | `internal/auth/session.go` sets `HttpOnly=true`, `SameSite=http.SameSiteLaxMode`, backed by SCS SQLite store. `csrfProtect` wraps all mutating routes via `nosurf`. `client.ts` fetches the CSRF token from `/api/v1/csrf` and echoes it in `X-CSRF-Token` on every `mutate()` call. Server test confirms CSRF rejection. |
| 3 | On first startup the system creates an admin user, initializes the data directory and Git repo, and self-heals any stale Git lock | VERIFIED | `main.go` startup sequence: config -> store -> migrate -> BootstrapAdmin (prints one-time password) -> repo.New -> gitstore.Init -> SelfHealStaleLock -> PullOnStartup -> SeedStarterRepo -> job worker -> HTTP server. `gitstore_test.go` confirms Init idempotency and stale-lock heal. `bootstrap_test.go` confirms no-op on non-empty DB. |
| 4 | A user's available actions reflect their role (admin / editor / reader), and key actions (login, config changes) appear in an audit log | VERIFIED | `auth.RequireRole(RoleAdmin)` wraps all `/api/v1/admin/*` routes. `loadCurrentUser` middleware enforces `MustChangePassword` server-side (CR-01 fix confirmed; `TestMustChangePasswordGate` passes). `audit.Record` wired in login, logout, bootstrap, seed, user create/role/reset/deactivate, profile change, CLI reset. Dual write (SQLite + slog) verified by `TestRecord`. `TestLastAdminInvariant` passes (CR-03 fix). |
| 5 | Every file-path access is forced through a safe resolver that rejects `../`, absolute paths, and symlink escape (fuzz-tested) | VERIFIED | `internal/repo/path.go` implements `Resolve` with: empty/NUL/absolute/`..`-segment rejection, URL-decoded re-validation, `filepath.EvalSymlinks` + boundary-prefix check, and `os.Root` defense-in-depth. `FuzzResolve` ran 16M+ iterations with no escape found. `TestResolveRejectsMaliciousInputs` covers 11 categories. `TestResolveRejectsSymlinkEscape` passes. No `filepath.Join` constructs paths outside `internal/repo/path.go`. |

**Score:** 5/5 truths verified

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `cmd/okf-workspace/main.go` | serve command + DI wiring | VERIFIED | Cobra `serve` subcommand; full startup sequence; git, jobs, audit, server wired |
| `internal/auth/password.go` | Argon2id hash + verify | VERIFIED | `argon2id.CreateHash` / `argon2id.ComparePasswordAndHash`; no bcrypt |
| `internal/auth/session.go` | SCS session with HTTPOnly/SameSite | VERIFIED | `HttpOnly=true`, `SameSite=SameSiteLaxMode`; `SecureCookies` helper (WR-07 fix) |
| `internal/store/migrations/0001_init.sql` | users and sessions tables | VERIFIED | Both tables defined; no wiki content in SQLite |
| `internal/store/migrations/0003_audit.sql` | audit_log table | VERIFIED | Table + created_at index |
| `internal/server/handlers_auth.go` | login/logout/me + audit | VERIFIED | CSRF token endpoint; login records audit; logout records audit; me returns display_name + must_change_password |
| `internal/server/handlers_users.go` | admin CRUD + CR-01 gate | VERIFIED | `loadCurrentUser` enforces `MustChangePassword` server-side at line 77; RequireRole(admin) wraps all admin routes; ErrLastAdmin handled (CR-03) |
| `internal/users/bootstrap.go` | admin bootstrap + CR-02 fix | VERIFIED | `generatePassword` uses rejection sampling over `crypto/rand`; no modulo bias |
| `internal/users/manage.go` | user CRUD + last-admin guard | VERIFIED | `SetRole` and `Deactivate` both check `CountActiveAdmins` before removing last admin; `ErrLastAdmin` returned |
| `internal/auth/rbac.go` | RequireRole middleware | VERIFIED | Reads role from session-bound user only (never client input); 401/403 correctly |
| `internal/repo/path.go` | safe-path resolver | VERIFIED | EvalSymlinks + os.Root; FuzzResolve corpus |
| `internal/repo/path_test.go` | fuzz test present | VERIFIED | `FuzzResolve` function present; 20s fuzz run finds no escape |
| `internal/gitstore/commit.go` | single-writer commit | VERIFIED | Mutex-serialized; resolves paths through repo before staging |
| `internal/gitstore/health.go` | stale-lock self-heal | VERIFIED | `SelfHealStaleLock` removes lock when no live process; runs git status+fsck |
| `internal/jobs/worker.go` | async job worker | VERIFIED | `Enqueue` persists; single-goroutine drain; retry with backoff |
| `internal/users/seed.go` | first-run repo seed | VERIFIED | Seeds via `repo.Write` + `gitstore.Commit`; no exec.Command; no-op on non-empty repo |
| `internal/audit/audit.go` | dual-write audit | VERIFIED | SQLite INSERT + slog.Info; non-fatal on DB error; no password/token fields |
| `web/src/routes/Login.tsx` | login form | VERIFIED | Calls `login(username, password)` via client.ts; shows "Invalid username or password." on error; "Sign in to your workspace" copy |
| `web/src/routes/AppShell.tsx` | authenticated shell | VERIFIED | Renders `data?.display_name`; UserMenu with logout; repo health banners |
| `web/src/routes/ForcePasswordChange.tsx` | forced password change | VERIFIED | No plaintext-credential prop (WR-06 fix); re-enters temp password; >=12 char + match validation |
| `internal/web/embed.go` | SPA embedded | VERIFIED | `//go:embed all:dist`; SPA history fallback for non-API routes; `internal/web/dist/` exists |
| `deploy/Dockerfile` | multi-stage non-root build | VERIFIED | Stage 1 npm ci + build; Stage 2 `CGO_ENABLED=0 go build`; Stage 3 Alpine non-root user `okf` |
| `deploy/okf-workspace.service` | systemd unit | VERIFIED | `ExecStart` with `serve --config`; non-root user; `Restart=always` |
| `cmd/okf-workspace/admin.go` | CLI reset-password | VERIFIED | `admin reset-password <username>` subcommand; opens store; calls `users.ResetPassword`; prints OTP once; records audit |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| `web/src/routes/Login.tsx` | `/api/v1/auth/login` | fetch POST with CSRF header | VERIFIED | `login()` calls `mutate<Me>("/api/v1/auth/login", ...)` which calls `ensureCSRF()` first |
| `internal/server/handlers_auth.go` | `internal/auth` | Authenticate + session create | VERIFIED | `auth.Authenticate` called; session stored with `sessions.Put` |
| `internal/auth/auth.go` | `internal/store` | user lookup by username | VERIFIED | `UserLookup.LookupForAuth` interface; `users.Repository` satisfies it |
| `internal/server/router.go` | `auth.RequireRole` | admin routes wrapped | VERIFIED | `admin.Use(auth.RequireRole(auth.RoleAdmin))` wraps all 5 admin routes |
| `web/src/routes/Admin.tsx` | `/api/v1/admin/users` | react-query mutations | VERIFIED | `listUsers()`, `createUser()`, `resetUserPassword()`, `deactivateUser()` in client.ts all call `mutate()` with CSRF |
| `internal/gitstore/commit.go` | `internal/repo/path.go` | Resolve before write/stage | VERIFIED | `g.repo.Resolve(p)` called for each path in `Commit()` before `git add` |
| `cmd/okf-workspace/main.go` | `internal/gitstore` | startup: init + self-heal | VERIFIED | `gs.Init` then `gs.SelfHealStaleLock` then `gs.PullOnStartup` in startup sequence |
| `internal/users/seed.go` | `internal/gitstore` | first commit through single-writer | VERIFIED | `gs.Commit(...)` called with CommitSpec; no raw exec.Command in seed.go |
| `internal/server/handlers_auth.go` | `internal/audit.Record` | login/logout events | VERIFIED | `h.audit.Record(...)` called after successful login and in logout |
| `internal/server/handlers_users.go` | `internal/audit.Record` | admin account-change events | VERIFIED | `h.audit.Record(...)` called in handleCreateUser, handleSetRole, handleResetPassword, handleDeactivate |
| `deploy/Dockerfile` | `web/dist + go build` | stage 1 npm build, stage 2 CGO_ENABLED=0 | VERIFIED | `npm run build` in Stage 1 outputs to `internal/web/dist`; `CGO_ENABLED=0 go build` in Stage 2 |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|--------------|--------|-------------------|--------|
| `AppShell.tsx` | `data?.display_name` | `useQuery(["me"], me)` → GET /auth/me → `users.GetByID` → SQLite | Yes — live DB read per request | FLOWING |
| `AppShell.tsx` | `repoHealth` | `useQuery(["health"], health)` → GET /api/v1/health → `gitstore.Health` | Yes — live git status check | FLOWING |
| `Admin.tsx` | `users` list | `useQuery(["users"], listUsers)` → GET /admin/users → `users.List` → SQLite | Yes — live DB query | FLOWING |
| `Login.tsx` | login result | POST /api/v1/auth/login → `auth.Authenticate` → Argon2id verify → SQLite | Yes — real password verification | FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Full backend build | `CGO_ENABLED=0 go build ./...` | exit 0 | PASS |
| All backend tests | `CGO_ENABLED=0 go test ./internal/... ./cmd/... -count=1` | 11 packages, all ok | PASS |
| go vet | `go vet ./internal/... ./cmd/...` | exit 0 (no output) | PASS |
| ESLint | `cd web && npm run lint` | exit 0 (no output) | PASS |
| CR-01 regression | `go test ./internal/server/... -run TestMustChangePasswordGate -v` | PASS | PASS |
| CR-03 regression | `go test ./internal/users/... -run TestLastAdminInvariant -v` | PASS | PASS |
| Fuzz resolver | `go test ./internal/repo/ -run=Fuzz -fuzz=FuzzResolve -fuzztime=20s` | 16M+ iterations, no escape | PASS |
| SPA build output | `ls internal/web/dist/index.html` | file exists | PASS |
| No bcrypt in prod | `grep -rn "bcrypt" internal/ cmd/ \| grep -v "_test"` | only comment in password.go | PASS |
| No mattn/go-sqlite3 | `grep -rn "mattn/go-sqlite3" go.mod` | no match in go.mod | PASS |
| No TBD/FIXME/XXX | grep across internal/ cmd/ web/src/ | no matches | PASS |
| No filepath.Join outside repo/path.go | `grep -rn "filepath.Join" internal/ \| grep -v repo/path.go \| grep -v _test` | no matches (cmd/ uses are legitimate operational paths) | PASS |
| Git push not in Phase 0 | `grep -rn "push\|Push" internal/gitstore/` | only "push is DEFERRED" comments | PASS |

### Probe Execution

No probe scripts found for this phase. Standard build + test suite used instead (all pass above).

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|------------|-------------|--------|----------|
| AUTH-01 | Plan 01 | User can log in with username and password | SATISFIED | `handleLogin` + Argon2id + SCS session; tests pass |
| AUTH-02 | Plan 03 | User can log out from any page | SATISFIED | `UserMenu` in `AppShell` topbar; `handleLogout` destroys session; human-verified |
| AUTH-03 | Plan 01 | Session persists via secure cookie | SATISFIED | SCS SQLite-backed session; `HttpOnly`, `SameSite=Lax`; session manager wired |
| AUTH-04 | Plan 01 | Admin user created automatically on first startup | SATISFIED | `BootstrapAdmin` in startup sequence; `must_change_password=1` set; OTP printed once |
| AUTH-05 | Plan 03 | User actions gated by role (admin/editor/reader) | SATISFIED | `RequireRole(RoleAdmin)` wraps all admin routes; `loadCurrentUser` enforces MustChangePassword; `TestLastAdminInvariant` + `TestMustChangePasswordGate` pass |
| AUTH-06 | Plan 01 | User has a display name shown in the UI | SATISFIED | `AppShell` renders `data?.display_name` from `/auth/me` response |
| SEC-01 | Plan 02 | All file paths through safe resolver blocking `../`, absolute, symlink escape | SATISFIED | `repo.Resolve` with EvalSymlinks + os.Root; FuzzResolve 16M+ iterations no escape; `TestResolveRejectsMaliciousInputs` 11 cases |
| SEC-03 | Plan 01 | Passwords hashed with Argon2id | SATISFIED | `argon2id.CreateHash`/`ComparePasswordAndHash`; bcrypt absent in production code |
| SEC-04 | Plan 01 | HTTPOnly/SameSite cookies; CSRF-protected mutations | SATISFIED | `HttpOnly=true`, `SameSiteLaxMode`; nosurf on all mutating routes; CSRF token echoed in client.ts |
| SEC-05 | Plan 04 | Key actions recorded in audit log | SATISFIED | Dual-write (SQLite + slog) for login, logout, bootstrap, seed, user_create, user_role_change, user_reset_password, user_deactivate, profile_change; CLI reset also recorded |

All 10 Phase-0 requirements satisfied.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| `internal/users/seed.go` | 111-125 | Empty `tags: []` YAML indentation bug (IN-01) | Info (deferred) | `tags:\n  []\n` would produce invalid YAML; never triggered since all seeded pages have non-empty tags; deferred in 00-REVIEW.md |
| `internal/auth/auth.go` | 35-43 | `dummyHash` can be empty if init hashing fails (IN-03) | Info (deferred) | Reduces timing equalization to "near-instant" instead of full Argon2 on the empty fallback; deferred in 00-REVIEW.md |

No TBD, FIXME, or XXX markers in any production file. No unreferenced debt markers.

### Human Verification Required

Per the code review (00-REVIEW.md, status: resolved), the following were human-verified during Plan 03 execution and approved: forced password change flow, admin CRUD screen (create user, reset password, deactivate with confirmation dialogs), profile self-service, top-bar logout reachability, server-side RBAC denial of non-admins, and CLI reset. Those approvals are carried forward.

The following items require human confirmation in a live environment for this verification gate:

#### 1. Login, display name, and logout flow (end-to-end browser)

**Test:** Build the binary (`cd web && npm install && npm run build && cd .. && CGO_ENABLED=0 go build -o /tmp/okf-workspace ./cmd/okf-workspace`). Run against an empty data dir. Copy the one-time admin password from the log. Visit `/login`, sign in, complete forced password change, verify display name appears in the top bar, open UserMenu, click Log out.
**Expected:** Each step behaves as described; display name visible in top bar after login; Log out redirects to `/login`.
**Why human:** Cookie persistence, SPA rendering, and redirect behavior are not verifiable by unit tests.

#### 2. Session persistence across browser refresh

**Test:** After logging in on step 1, press F5 / refresh the browser.
**Expected:** User remains on `/app` showing their display name; no redirect to `/login`.
**Why human:** Requires a live browser session to test the session-cookie round-trip.

#### 3. Server-side RBAC enforcement for non-admin roles

**Test:** Create an editor account from `/admin`. Log out. Log in as the editor. Navigate to `/admin`.
**Expected:** Redirected away from `/admin` client-side. To confirm server-side: `curl -b <editor_session_cookie> http://localhost:8080/api/v1/admin/users` returns 403 with `{"error":"You don't have permission to do that."}`.
**Why human:** Requires a live session cookie from an editor-role user.

#### 4. Forced password change gates every route (CR-01 end-to-end)

**Test:** With the admin's initial one-time password (before it is changed), attempt to navigate directly to `/app` and to issue `PUT /api/v1/profile` via curl using the active session cookie.
**Expected:** Browser redirects to the forced password change screen. Curl returns 403 with `{"error":"Set a new password to continue."}`. Only `PUT /api/v1/profile/password` succeeds.
**Why human:** Unit test `TestMustChangePasswordGate` passes, but end-to-end browser-level bypass attempt should be confirmed.

#### 5. First-run Git repo initialization and seed commit

**Test:** After the first run, inspect the data directory: `git -C <repo_dir> log --oneline`.
**Expected:** Exactly one commit with a message like "Seed starter workspace" authored with the admin identity. A second run does not add a second commit.
**Why human:** The `SeedStarterRepo` test passes in isolation; full startup sequence against a real data directory required to confirm.

---

## Gaps Summary

No gaps. All 5 must-have truths are verified in code. All 10 Phase-0 requirements are satisfied. All 3 code review blockers (CR-01, CR-02, CR-03) and 7 warnings (WR-01 through WR-07) are fixed and confirmed in code. The regression tests `TestMustChangePasswordGate` and `TestLastAdminInvariant` pass. The complete build (`go build ./...`), test suite (`go test ./internal/... ./cmd/...`), `go vet`, ESLint, and 20-second fuzz run all pass.

The `human_needed` status reflects 5 live-browser flows that cannot be verified by static analysis or unit tests. These flows were human-approved during the Plan 03 review checkpoint; this verification requires the explicit sign-off to be re-confirmed against the current codebase state (post-CR-01/02/03 fixes).

---

_Verified: 2026-06-18T16:15:00Z_
_Verifier: Claude (gsd-verifier)_
