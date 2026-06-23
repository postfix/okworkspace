---
phase: 00-skeleton-auth-foundations
plan: 01
subsystem: auth
tags: [go, chi, sqlite, modernc, argon2id, scs-sessions, nosurf-csrf, react, vite, typescript, embed]

# Dependency graph
requires:
  - phase: none (greenfield)
    provides: SPEC.md, CLAUDE.md locked stack, 00-UI-SPEC.md design contract
provides:
  - Single static Go binary (CGO_ENABLED=0) serving the embedded React SPA + REST API from one process
  - internal/config typed Config loader (SPEC §20.3) with defaults + OKF_ADMIN_USERNAME env override
  - internal/store shared pure-Go SQLite *sql.DB with embedded idempotent migrations (users + sessions)
  - internal/auth Argon2id hashing, SCS SQLite-backed sessions, generic-error Authenticate, CSRF
  - internal/users User + Repository (parameterized) + first-run admin BootstrapAdmin (one-time password)
  - internal/server chi router + middleware stack + login/logout/me + /csrf API
  - internal/web embed.FS SPA handler with history fallback
  - web/ Vite+React+TS scaffold: token-driven Login + AppShell wired to the real auth API
affects: [phase-00 plan-02 (repo/gitstore/jobs spine + seed), plan-03 (RBAC/admin/profile/forced-pw-change), plan-04 (audit/config/packaging), phase-1 (pages reuse the shell + store + session model)]

# Tech tracking
tech-stack:
  added:
    - github.com/spf13/cobra v1.10.2
    - modernc.org/sqlite v1.52.0
    - gopkg.in/yaml.v3 v3.0.1
    - github.com/alexedwards/argon2id v1.0.0
    - github.com/alexedwards/scs/v2 v2.9.0
    - github.com/justinas/nosurf v1.2.0
    - github.com/go-chi/chi/v5 v5.3.0
    - react 19.2.7, react-dom 19.2.7, vite 8.0.16, typescript 6.0.3
    - "@vitejs/plugin-react 6.0.2, react-router-dom 7.18.0, @tanstack/react-query 5.101.0, zustand 5.0.14, lucide-react 0.469.0"
  patterns:
    - "Single shared *sql.DB in internal/store; all packages share it (no per-package SQLite opens)"
    - "Embedded idempotent migrations tracked by schema_migrations"
    - "auth.UserLookup interface decouples internal/auth from internal/users (breaks import cycle)"
    - "Generic 'Invalid username or password.' + dummy-hash timing equalization (no account enumeration)"
    - "nosurf CSRF on ALL mutating routes; SPA echoes X-CSRF-Token from GET /api/v1/csrf"
    - "SCS session renew-on-login (anti session-fixation); HttpOnly + SameSite=Lax + Secure-when-https"
    - "CSS custom-property token system in tokens.css; components reference var(--*) only"
    - "go:embed all:dist SPA handler with history fallback for non-/api routes"

key-files:
  created:
    - cmd/okf-workspace/main.go
    - internal/config/config.go
    - internal/store/db.go
    - internal/store/migrations.go
    - internal/store/migrations/0001_init.sql
    - internal/auth/password.go
    - internal/auth/auth.go
    - internal/auth/session.go
    - internal/auth/sqlitestore.go
    - internal/users/users.go
    - internal/users/bootstrap.go
    - internal/server/router.go
    - internal/server/middleware.go
    - internal/server/handlers_auth.go
    - internal/web/embed.go
    - web/src/api/client.ts
    - web/src/routes/Login.tsx
    - web/src/routes/AppShell.tsx
    - web/src/styles/tokens.css
    - config.example.yaml
  modified:
    - .gitignore
    - go.mod
    - go.sum

key-decisions:
  - "Custom pure-Go SCS SQLite session store (avoids the cgo mattn/go-sqlite3 sqlite3store, honoring CGO_ENABLED=0)"
  - "auth.UserLookup interface to break the auth<->users import cycle (users.BootstrapAdmin needs auth.HashPassword)"
  - "Vite builds into internal/web/dist because Go //go:embed cannot traverse '..'"
  - "Bumped @types/react-dom, eslint-plugin-react-hooks, typescript-eslint to eslint-10/TS-6-compatible versions"

patterns-established:
  - "Files-as-truth invariant scaffolded: SQLite holds operational data only (users/sessions), never content"
  - "Startup order: config -> store -> migrate -> bootstrap admin -> build server -> listen"
  - "One-time admin password printed once via slog.Warn; plaintext never logged on any other path"

requirements-completed: [AUTH-01, AUTH-03, AUTH-04, AUTH-06, SEC-03, SEC-04]

# Metrics
duration: 18min
completed: 2026-06-17
---

# Phase 0 Plan 01: Skeleton & Auth Foundations Summary

**A single CGO-free Go binary that bootstraps an admin (one-time logged password), serves an embedded React login -> AppShell, and authenticates against SQLite via Argon2id with SCS HttpOnly+SameSite sessions and nosurf CSRF.**

## Performance

- **Duration:** ~18 min
- **Started:** 2026-06-17T16:34:38Z
- **Completed:** 2026-06-17T16:52:08Z
- **Tasks:** 3
- **Files modified:** 43 (created/modified across go + web)

## Accomplishments

- Walking Skeleton proven end-to-end (live, port 8099): empty-DB first run prints a one-time admin password; `POST /api/v1/auth/login` with it returns 200 + `Set-Cookie: okf_session=...; HttpOnly; SameSite=Lax`; `GET /api/v1/auth/me` returns `{username, display_name:"Administrator", role:"admin"}`; the embedded SPA serves `/` and history-falls-back `/app` to index.html.
- Real auth spine: Argon2id PHC hashing (SEC-03), generic login error with dummy-hash timing equalization (no account enumeration), SCS SQLite-backed sessions with renew-on-login (anti session-fixation), nosurf CSRF on all mutating routes (SEC-04).
- Pure-Go single-binary deploy: `modernc.org/sqlite` (no cgo), `//go:embed` SPA — `CGO_ENABLED=0 go build ./...` is clean; zero forbidden libs (`mattn/go-sqlite3`/`crypto/bcrypt` absent).
- Token-driven frontend: every UI-SPEC token in `tokens.css`; Login card ("Sign in to your workspace" / accent "Sign in") and AppShell (display name + "Log out", read-only nav tree, "Your workspace is ready").

## Task Commits

1. **Task 1: Scaffold Go module, config loader, shared SQLite store** (TDD)
   - `6bc2c66` test(00-01): failing config + store tests (RED)
   - `20a7af9` feat(00-01): module + config + store + migrations + serve (GREEN)
2. **Task 2: Auth spine — Argon2id, SCS sessions, CSRF, bootstrap, login/me API** (TDD)
   - `efc183f` test(00-01): failing auth/bootstrap/handlers tests (RED)
   - `b6e56eb` feat(00-01): auth + users + server wired into serve (GREEN)
3. **Task 3: React scaffold + login -> AppShell, embedded and runnable**
   - `44c130f` feat(00-01): web scaffold + tokens + Login/AppShell + embed.FS handler

**Plan metadata:** committed separately (this SUMMARY + STATE/ROADMAP/REQUIREMENTS).

## Files Created/Modified

- `cmd/okf-workspace/main.go` — cobra root + `serve --config`; startup config->store->migrate->bootstrap->server->listen, slog JSON
- `internal/config/config.go` — typed Config (SPEC §20.3), defaults (okf_session, 168h, admin), OKF_ADMIN_USERNAME override
- `internal/store/{db.go,migrations.go,migrations/0001_init.sql}` — shared pure-Go SQLite, idempotent embedded migrations, users+sessions tables
- `internal/auth/{password.go,auth.go,session.go,sqlitestore.go}` — Argon2id, generic Authenticate via UserLookup, SCS manager, pure-Go session store
- `internal/users/{users.go,bootstrap.go}` — User + Repository (parameterized), one-time admin bootstrap
- `internal/server/{router.go,middleware.go,handlers_auth.go}` — chi router, middleware stack, login/logout/me + /csrf
- `internal/web/embed.go` — go:embed all:dist + SPA history fallback
- `web/src/{api/client.ts,routes/Login.tsx,routes/AppShell.tsx,styles/tokens.css,App.tsx,main.tsx}` — SPA wired to the real auth API
- `config.example.yaml`, `.gitignore` — runtime config example, ignore rules
- Test files: `internal/{config,store,auth,users,server}/*_test.go`

## Single-binary build + run

```bash
# 1. Build the SPA (outputs to internal/web/dist, the embed root)
cd web && npm install && npm run build && cd ..

# 2. Build the CGO-free binary with the SPA embedded
CGO_ENABLED=0 go build -o okf-workspace ./cmd/okf-workspace

# 3. Configure and run
cp config.example.yaml config.yaml   # adjust server.listen / storage.data_dir as needed
./okf-workspace serve --config ./config.yaml
```

**Observed first-run flow (verified live on 127.0.0.1:8099):**
1. On an empty DB the log emits a `slog.Warn` line: `admin user created — save this password, it will NOT be shown again` with `username=admin`, `one_time_password=<28 random chars>`, `must_change_password=true`.
2. Open `/login`, enter `admin` + that password, click **Sign in** -> lands on `/app` (AppShell) showing display name **Administrator**.
3. Refresh keeps the session (the `okf_session` HttpOnly cookie persists; `GET /api/v1/auth/me` returns the user).

## Decisions Made

- **Pure-Go SCS SQLite session store** (`internal/auth/sqlitestore.go`): the upstream `scs/sqlite3store` depends on the cgo `mattn/go-sqlite3` driver, forbidden by CLAUDE.md. Implemented a thin `scs.CtxStore` over the shared modernc `*sql.DB` using the same `sessions` schema. Keeps `CGO_ENABLED=0`.
- **`auth.UserLookup` interface**: `users.BootstrapAdmin` needs `auth.HashPassword`, and `Authenticate` needs user lookup — a direct mutual import is a cycle. `Authenticate` now depends on a small `UserLookup` interface (implemented by `users.Repository.LookupForAuth`), so `auth` no longer imports `users`.
- **Embed root = `internal/web/dist`**: Go `//go:embed` cannot traverse `..` and resolves relative to the embedding file. Vite `outDir` set to `../internal/web/dist`.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Pure-Go SCS SQLite session store instead of cgo sqlite3store**
- **Found during:** Task 2 (session wiring)
- **Issue:** `github.com/alexedwards/scs/sqlite3store` pulls the cgo `mattn/go-sqlite3` driver (forbidden by CLAUDE.md; breaks CGO_ENABLED=0 single-binary goal).
- **Fix:** Implemented `internal/auth/sqlitestore.go` (a thin `scs.CtxStore` over the shared modernc `*sql.DB`, reusing the `sessions` table schema).
- **Verification:** Sessions persist across requests; live `/me` after login works; `grep mattn/go-sqlite3` returns 0 real imports.
- **Committed in:** `b6e56eb`

**2. [Rule 3 - Blocking] Broke auth<->users import cycle via UserLookup interface**
- **Found during:** Task 2 (build failed: import cycle not allowed)
- **Issue:** `users/bootstrap.go` imports `auth` (HashPassword) while `auth.Authenticate` imported `users` — a cycle.
- **Fix:** Introduced `auth.UserLookup`/`auth.AuthUser`/`auth.ErrUserNotFound`; `Authenticate` returns the user id; `users.Repository.LookupForAuth` adapts. Updated the handler to fetch the user by id post-auth.
- **Verification:** `CGO_ENABLED=0 go build ./...` clean; all auth/users/server tests pass.
- **Committed in:** `b6e56eb`

**3. [Rule 3 - Blocking] Frontend dependency versions adjusted for ESLint 10 / TS 6 peer compatibility**
- **Found during:** Task 3 (npm install ERESOLVE)
- **Issue:** `@types/react-dom@19.2.7` is unpublished (latest 19.2.x is 19.2.3); `eslint-plugin-react-hooks@5.2.0` and `typescript-eslint@8.46.1` do not peer-support ESLint 10 (and 8.46.1 caps TypeScript `<6.0.0`).
- **Fix:** `@types/react-dom`->`19.2.3`, `eslint-plugin-react-hooks`->`7.1.1`, `typescript-eslint`->`8.61.1` (eslint-10 + TS `<6.1.0`); added `@eslint/js@10.0.1` for the flat config. CLAUDE-locked runtime deps (react/vite/ts/router/query/zustand/lucide) were NOT changed.
- **Verification:** `npm install` 0 vulnerabilities; `npm run build` and `npm run lint` (exit 0); `go build` with embedded SPA OK.
- **Committed in:** `44c130f`

**4. [Rule 3 - Blocking] Embed root / SPA build output relocated to internal/web/dist**
- **Found during:** Task 3 (`go build`: `pattern all:dist: no matching files found`)
- **Issue:** Plan placed `//go:embed` in `internal/web/embed.go` but vite output (`web/dist`) is unreachable — Go embed cannot use `..`.
- **Fix:** Vite `outDir` set to `../internal/web/dist`; committed `internal/web/dist/.gitkeep` (renamed from `web/dist/.gitkeep`); updated `.gitignore`.
- **Verification:** `npm run build` writes `internal/web/dist/index.html`; binary serves `/` and `/app` from the embed; SPA history fallback confirmed live.
- **Committed in:** `44c130f`

**5. [Rule 1 - Bug] Added @types/react-dom + CSS side-effect type declarations**
- **Found during:** Task 3 (tsc error TS2882 under `noUncheckedSideEffectImports`)
- **Issue:** TS 6 strict flags reject side-effect CSS imports without a module declaration.
- **Fix:** Added `web/src/vite-env.d.ts` (`declare module "*.css"` + vite client types).
- **Verification:** `npm run build` (tsc -b) clean.
- **Committed in:** `44c130f`

---

**Total deviations:** 5 auto-fixed (4 Rule 3 blocking, 1 Rule 1 bug)
**Impact on plan:** All necessary to honor CLAUDE.md locks (CGO-free, no cgo sqlite/bcrypt) and to produce a building, linting, runnable single binary. No scope creep — all five are mechanical/compatibility fixes within the plan's intent. One acceptance criterion text ("`web/dist/index.html` exists") was satisfied as `internal/web/dist/index.html` due to the Go embed `..` limitation.

## Issues Encountered

- A stray host service occupies port 8080; live verification used 127.0.0.1:8099 to avoid the conflict. The default config still ships `0.0.0.0:8080` per SPEC.
- nosurf v1.2.0 enforces a same-origin check (Sec-Fetch-Site / Origin / Referer) in addition to the double-submit token; server tests set `Sec-Fetch-Site: same-origin` to simulate a same-origin SPA fetch. This is correct, stronger CSRF behavior — no production change needed (browsers send these headers on same-origin fetch).

## Known Stubs

- `web/src/routes/AppShell.tsx` renders a STATIC, read-only placeholder nav tree (4 hardcoded nodes). This is intentional per D-11/UI-SPEC ("editing arrives next"); the real seeded tree is wired in **Plan 02**. The `must_change_password` flag is persisted on the bootstrap admin but enforcement (forced first-login change UI) is **Plan 03**.

## TDD Gate Compliance

Tasks 1 and 2 followed RED->GREEN: a `test(00-01)` commit precedes each `feat(00-01)` commit (`6bc2c66`->`20a7af9`, `efc183f`->`b6e56eb`). No REFACTOR commits were needed. Task 3 is a non-TDD scaffold task per the plan (`type="auto"` without `tdd`).

## User Setup Required

None — no external service configuration required. (LLM/agent config keys are parsed-but-unused placeholders this phase.)

## Next Phase Readiness

- Ready for **Plan 02**: the shared `*sql.DB`/migrations, startup sequence, and session/CSRF model are in place for the safe-path resolver (`internal/repo`), single-writer Git spine (`internal/gitstore`), job worker (`internal/jobs`), and the first repo seed commit.
- Ready for **Plan 03**: `must_change_password` is already persisted; RBAC `RequireRole`, `/admin`, profile, and forced password-change build on the existing auth handlers.
- Concern (carried from STATE): Phase 1's byte-stable Markdown round-trip remains the eventual exit gate; not touched here.

## Self-Check: PASSED

All 13 spot-checked created files exist on disk; all 5 task commits (`6bc2c66`, `20a7af9`, `efc183f`, `b6e56eb`, `44c130f`) are present in git history.

---
*Phase: 00-skeleton-auth-foundations*
*Completed: 2026-06-17*
