# Phase 0: Skeleton, Auth & Foundations - Context

**Gathered:** 2026-06-17
**Status:** Ready for planning

<domain>
## Phase Boundary

Phase 0 delivers a **running single-binary app a non-technical user can log into**, with all load-bearing security and storage foundations in place for later phases. Concretely:

- Single Go binary serves the embedded React/Vite SPA behind a login.
- Local username/password auth (Argon2id), server-side sessions (HTTPOnly + SameSite cookies), CSRF protection on mutating requests.
- Admin user bootstrapped on first startup; in-app user management; RBAC (admin / editor / reader).
- Data directory + Git repo initialized on first startup, with stale-lock self-heal.
- The cross-cutting **spines** introduced here for reuse by every later phase: the **safe-path resolver** (`internal/repo` chokepoint), the **single-writer Git commit service**, and the **async job worker**.
- Audit scaffolding for key actions (login/logout, account/config changes).

**Requirements covered:** AUTH-01, AUTH-02, AUTH-03, AUTH-04, AUTH-05, AUTH-06, SEC-01, SEC-03, SEC-04, SEC-05 (see `.planning/REQUIREMENTS.md`).

**Explicitly NOT in this phase (belongs to Phase 1+):** page create/edit/render, OKF frontmatter parse/repair, Markdown rendering, the file tree as a working navigator, automatic commit-on-save, page history/restore, attachments, search, agent. Phase 0 builds the *foundations and shell*, not the wiki loop.

</domain>

<decisions>
## Implementation Decisions

### First admin login (bootstrap & recovery)
- **D-01:** On first startup, generate a **strong random password**, hash it (Argon2id), create the admin user, and **print the plaintext once to stdout/server log**. Never ship a fixed/default password (PITFALLS §"Admin-bootstrap weak password"). Bootstrap runs only when no users exist.
- **D-02:** **Force a password change on first login** for the bootstrap admin (e.g. a `must_change_password` flag gates the app until reset).
- **D-03:** Admin **username is configurable** (config.yaml/env), default `admin`.
- **D-04:** Lockout recovery = a **local CLI subcommand** `okf-workspace admin reset-password <user>` that regenerates a one-time password printed to the log (same trust boundary as bootstrap: requires shell access to the box). No web-based "forgot password" / email flow in MVP.

### Team provisioning & account management
- **D-05:** Phase 0 ships a **minimal in-app `/admin` user-management screen** + backing API: list users, add user, assign role (admin/editor/reader), reset a user's password, deactivate/disable a user. Rationale: a *non-technical admin* must be able to onboard the ~5-person team without server shell access — aligns with the product core value and SPEC §18.1 (`/admin` route) + §18.2 (`AdminSettings`).
- **D-06:** **Self-service profile** — any logged-in user can change **their own password and display name**. The admin owns account creation and **role assignment** (a user cannot change their own role).
- **D-07:** Roles are the fixed SPEC set **admin / editor / reader**. In Phase 0 the only role-gated surface that fully exercises RBAC is **admin-only access to `/admin` + config/audit**; editor-vs-reader content gating becomes meaningful in Phase 1 (page editing). Roles + the `RequireRole` middleware must still be built and enforced now.

### First-run wiki content (repo seed)
- **D-08:** On first startup of a **brand-new** repo, bootstrap scaffolds the SPEC §9 starter layout as the **initial Git commit**: root `index.md` plus `runbooks/`, `architecture/`, `decisions/`, each containing an `index.md`. Each seeded file carries valid OKF frontmatter (so Phase 1 can open/render them without repair).
- **D-09:** **Only seed when the repo is genuinely new and empty.** If a remote is configured to pull existing content on startup (see D-12), do **not** seed — pull the existing repo instead, to avoid clobbering/divergence.
- **D-10:** Seeding produces the **first real commit through the single-writer Git spine** — use it as the end-to-end exercise of the commit service (do not bypass it with a raw `git` call from bootstrap).

### Phase-0 landing screen
- **D-11:** Build the **real AppShell** now (not a throwaway): left navigation rail + top bar showing the user's **display name** and a **logout control reachable from any page**, plus a main content pane. Render the **seeded tree read-only/disabled** with a clear "page editing arrives next" placeholder in the main pane. Admins additionally see the user-management screen. This front-loads most of the Phase-1 shell so Phase 1 slots straight in. (`UI hint: yes` on this phase — a `/gsd-ui-phase 0` design contract is appropriate before/with planning.)

### Claude's Discretion (decided with sensible defaults, not asked)
- **Session cookies:** `SameSite=Lax` (SPA-friendly, same-origin), HTTPOnly, `Secure` when behind TLS; cookie name `okf_session`; TTL 168h (per config). SCS with the SQLite store so sessions persist in `app.db`.
- **CSRF:** `nosurf` double-submit on **all** mutating routes (login, account changes, admin actions, config) — no partial coverage (PITFALLS §security).
- **Audit log:** write-only in Phase 0 → SQLite mirror **and** structured log line per event (login, logout, admin account changes, config changes, bootstrap, password reset). **No** in-app audit viewer UI this phase (deferred).
- **Login hardening:** basic protection only (constant-time compare via the Argon2id verify, generic "invalid credentials" error). Rate-limiting / lockout is **not** required for a 5-user internal tool in Phase 0 — revisit only if needed.
- **Remote Git sync (D-12):** Phase 0 is **local-only `git init`** by default. If `git.remote_enabled` + `git.pull_on_startup` are set: do a **fast-forward-only** pull on startup; on divergence, **alert and refuse to auto-merge** (never silently merge), and skip seeding (D-09). **Push is deferred to Phase 1**, where commits actually happen. (Flagged for Phase 0/1 planning by ROADMAP Notes.)
- **Startup self-heal:** detect and clear a stale `.git/index.lock` (only when no live git process), run a quick health check, and surface a basic repository-health status (SPEC §6.6) — the single-writer service owns this.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Product & technical spec (source of truth)
- `SPEC.md` §6.5 — User management (login, admin bootstrap, display name, roles admin/editor/reader, session cookies, logout)
- `SPEC.md` §16 + §16.1 — Backend service design & package layout (`internal/{config,server,auth,users,repo,gitstore,jobs,audit,web}`, `cmd/okf-workspace/main.go`); auth/repo/git service responsibilities
- `SPEC.md` §17.1 — Auth API shape (`POST /api/v1/auth/login`, `/logout`, `GET /auth/me` and its response)
- `SPEC.md` §18.1–§18.2 — Frontend routes (`/login`, `/app`, `/admin`) and components (`AppShell`, `LoginForm`, `AdminSettings`)
- `SPEC.md` §20.2–§20.3 — Data directory layout and `config.yaml` (server, storage, git, auth.session_*, search, attachments keys)
- `SPEC.md` §21.1 — Path safety (the safe-path resolver requirements)
- `SPEC.md` §21.4 — Auth security (Argon2id/bcrypt, HTTPOnly, SameSite, CSRF, admin bootstrap)
- `SPEC.md` §21.5 — Audit log event list
- `SPEC.md` §9 — Repository layout (drives the D-08 seed: `index.md`, `runbooks/`, `architecture/`, `decisions/`, `.okf-workspace/`)
- `SPEC.md` §22.1 — Required backend tests (safe-path resolver listed first)
- `SPEC.md` §23 (Phase 0) + §24 — Phase-0 deliverables and the first-milestone vertical slice ("login as admin… open index.md")

### Architecture & build order
- `.planning/research/ARCHITECTURE.md` — Component responsibility table, Phase-0 build order (1. config+store+cmd+startup → 2. `repo.path` safe resolver FIRST, fuzz-tested → 3. gitstore init/pull → 4. web embed + server + auth/users + bootstrap + login), single-writer Git pattern (Pattern 3), `internal/store` addition for shared `*sql.DB`+migrations, structure rationale (resolver is the chokepoint)
- `.planning/research/PITFALLS.md` — Pitfall 2 (concurrent Git `index.lock` → single-writer + startup self-heal), Pitfall 5 (path traversal/symlink escape → one resolver, `filepath.EvalSymlinks` + prefix check, prefer `os.Root`), Security-mistakes table (CSRF on all routes; no default admin password), Pitfall-to-Phase mapping (auth/session/CSRF + resolver + single-writer all Phase 0)

### Stack (locked versions & library choices)
- `CLAUDE.md` / `.planning/research/STACK.md` — Locked: Go 1.26 + chi v5; pure-Go `modernc.org/sqlite`; Argon2id via `github.com/alexedwards/argon2id`; sessions via `github.com/alexedwards/scs/v2` (SQLite store); CSRF via `github.com/justinas/nosurf`; React 19 + Vite 8 + TS 6 embedded via `embed.FS`; `gopkg.in/yaml.v3`; logging `log/slog`; CLI `github.com/spf13/cobra`; shell-out `git` CLI (CGO_ENABLED=0, single binary)

### Project framing & requirements
- `.planning/PROJECT.md` — Key Decisions table (files-as-truth, SQLite operational-only, Git hidden, chi/Bleve/React/Eino locks)
- `.planning/REQUIREMENTS.md` — AUTH-01..06, SEC-01/03/04/05 definitions + traceability
- `.planning/ROADMAP.md` — Phase 0 goal, success criteria (5), and Notes (SEC-* as the security floor; single-writer Git + async job worker introduced here; remote-push divergence + `pull_on_startup` semantics to define during Phase 0/1; "no phase research needed")

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- **Greenfield repo** — no Go or TS source exists yet (`SPEC.md`, `LICENSE`, `.gitignore`, `.planning/` only). Phase 0 establishes the foundations the rest of the build reuses; there are no prior components to reuse.

### Established Patterns (to *create* here, reused later)
- **`internal/repo` safe-path resolver** is the single chokepoint for all filesystem access — build and fuzz-test it first; nothing else may construct repo absolute paths. Reused by attachments (Phase 2) and the agent (Phase 4).
- **Single-writer Git commit service** (`internal/gitstore` driven by one worker) — the only path to repo writes; introduced here (first commit = the seed), consumed by Phases 1/2/4.
- **Async job worker** (`internal/jobs`) spine — introduced here, drains a SQLite-persisted queue; CommitJob (Phase 1), ExtractJob (Phase 2), IndexJob (Phase 3) plug in later.
- **`internal/store`** — one shared `*sql.DB` + migrations for operational data (users, sessions, jobs, audit mirror); all packages share it rather than each opening SQLite.
- **One-way projection invariant** — files are truth; SQLite/Bleve are rebuildable caches (not exercised much in Phase 0, but the schema/discipline starts here).

### Integration Points
- `cmd/okf-workspace/main.go` — `serve` command + DI wiring + **startup order**: config → store(SQLite+migrate) → repo/data-dir init → gitstore init (+ optional ff-only pull) + stale-lock self-heal → (seed if brand-new) → job worker → http server with middleware (recover, request-id, session, CSRF, RBAC, audit).
- `internal/web` — `embed.FS` of `web/dist` with SPA history-fallback; the Go binary serves the React app + the `/api/v1` REST surface from one process.
- `internal/server` middleware stack is where session/CSRF/RBAC/audit attach before handlers.

</code_context>

<specifics>
## Specific Ideas

- First-run flow should be **zero-friction for a self-hoster**: run the binary → read the one-time admin password from the log → log in → forced password change → land on the AppShell with the seeded tree visible. This mirrors SPEC §24's milestone narrative ("Login as admin… see left tree… open index.md").
- The `/admin` user-management UI should be deliberately minimal (table + add-user form + per-row role/reset/deactivate) — enough to onboard ~5 people, not a full IAM console.
- Seeded starter files should be valid, openable OKF pages (correct required frontmatter) so Phase 1 inherits clean content with no repair churn.

</specifics>

<deferred>
## Deferred Ideas

- **In-app audit-log viewer** — Phase 0 writes audit events (SQLite mirror + log); a UI to browse/filter them is a later concern.
- **Login rate-limiting / account lockout / brute-force protection** — not required at 5-user internal scale; revisit if the deployment surface widens.
- **Web-based password recovery (email / forgot-password)** — out of scope; CLI reset (D-04) is the MVP recovery path.
- **Git remote push** — deferred to Phase 1 (no commits to push in Phase 0 beyond the seed; push semantics + divergence UX land with the page-save commit path).
- **Configurable/custom starter-folder sets** — Phase 0 hardcodes the SPEC §9 layout; making the seed structure configurable is a nice-to-have.
- **SSO / external identity** — explicit SPEC §4 non-goal; local username/password only.

None of these were scope creep into other *capabilities*; they are hardening/UX refinements of Phase 0's own foundations or items the roadmap already places later.

</deferred>

---

*Phase: 0-Skeleton, Auth & Foundations*
*Context gathered: 2026-06-17*
