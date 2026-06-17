# Walking Skeleton — OKF Workspace

**Phase:** 0 (Skeleton, Auth & Foundations)
**Generated:** 2026-06-17

## Capability Proven End-to-End

> One sentence: the smallest user-visible capability that exercises the full stack.

A non-technical teammate runs one Go binary, reads the one-time admin password from the server log, opens the web UI, signs in with username + password, and lands on the authenticated AppShell showing their display name — proving the full path: React login form → `POST /api/v1/auth/login` → Argon2id verify against the SQLite `users` table → SCS server-side session cookie (HTTPOnly + SameSite=Lax) → `GET /api/v1/auth/me` → authenticated AppShell render.

## Architectural Decisions

| Decision | Choice | Rationale |
|---|---|---|
| Language / process model | Go 1.26, single static binary (`CGO_ENABLED=0`) | LOCKED (CLAUDE.md). `embed.FS` ships the SPA; one artifact, no C toolchain, cross-compiles. |
| HTTP router | `github.com/go-chi/chi/v5` v5.3.0 | LOCKED. `net/http`-compatible, composable middleware (recover, request-id, session, CSRF, RBAC, audit attach here). |
| Operational DB | `modernc.org/sqlite` v1.52.0 (pure-Go), one shared `*sql.DB` in `internal/store` | LOCKED. Pure-Go keeps the single-binary promise. SQLite holds operational/derived data ONLY (users, sessions, jobs, audit mirror) — NEVER wiki content (files-as-truth invariant, SPEC §8.1). |
| Password hashing | `github.com/alexedwards/argon2id` v1.0.0 (over `golang.org/x/crypto/argon2`) | LOCKED. SEC-03. PHC-format hash w/ embedded params; `ComparePasswordAndHash` is constant-time. bcrypt is forbidden as default (CLAUDE.md "What NOT to Use"). |
| Sessions | `github.com/alexedwards/scs/v2` v2.9.0 with the SQLite store (sessions persist in `app.db`) | LOCKED. AUTH-03 / SEC-04. Cookie `okf_session`, HTTPOnly, `SameSite=Lax`, `Secure` when behind TLS, TTL 168h (config `auth.session_ttl_hours`). |
| CSRF | `github.com/justinas/nosurf` v1.2.0 (double-submit) on ALL mutating routes | LOCKED. SEC-04. SPA fetches the token from a cookie/endpoint and echoes it in a header on every mutating request — no partial coverage. |
| Logging | stdlib `log/slog` (JSON handler) | LOCKED. Zero deps; structured; the audit log emits one slog line per event in addition to the SQLite mirror. |
| Versioning store | shell-out to the `git` CLI via `internal/gitstore`, driven by ONE single-writer worker | LOCKED. Never `git` from a request handler or two goroutines. First real commit = the repo seed (D-10). Stale `.git/index.lock` self-heal on startup. |
| Safe-path access | `internal/repo` safe-path resolver is the single chokepoint for every filesystem access | SEC-01. Rejects `../`, absolute paths, encoded traversal, NUL, and symlink escape (`filepath.EvalSymlinks` + prefix check; prefer `os.Root`). Fuzz-tested. Built before any file op depends on it. |
| Async work | `internal/jobs` worker draining a SQLite-persisted queue | Spine introduced here; CommitJob (Phase 1), ExtractJob (Phase 2), IndexJob (Phase 3) plug in later. |
| CLI | `github.com/spf13/cobra` v1.10.2 | `serve --config`, `admin reset-password <user>` (D-04). |
| Config | `gopkg.in/yaml.v3` v3.0.1, `config.yaml` per SPEC §20.3 | Typed struct; env overrides (e.g. `OKF_LLM_API_KEY`); admin username configurable (D-03). |
| Frontend | React 19.2.7 + Vite 8.0.16 + TS 6.0.3, built to `web/dist`, embedded via `embed.FS` | LOCKED. `@tanstack/react-query` for server state, `zustand` for UI state, `react-router-dom` for routes, `lucide-react` icons. No Tailwind/Radix/shadcn (UI-SPEC decision); hand-built token-driven primitives. |
| Directory layout | SPEC §16 package layout, verbatim (`cmd/okf-workspace`, `internal/{config,server,web,auth,users,store,repo,gitstore,jobs,audit}`) | No deviation (ARCHITECTURE.md). `internal/repo` is the security chokepoint. |

## Stack Touched in Phase 0 (Skeleton — Plan 01)

- [x] Project scaffold — Go module (`go.mod`), `cmd/okf-workspace/main.go` with cobra `serve`, Vite/React/TS scaffold, `web/dist` embed, build + lint wiring
- [x] Routing — chi router with `/api/v1/auth/login`, `/api/v1/auth/logout`, `/api/v1/auth/me`, SPA history fallback
- [x] Database — real SQLite read AND write: `users` table created via migration, admin row inserted on bootstrap (write), looked up on login (read), `sessions` table persists the SCS session (write/read)
- [x] UI — interactive login form wired to the API; on success renders the AppShell top bar with the user's display name
- [x] Deployment — documented local full-stack run: `cd web && npm install && npm run build`, then `go build -o okf-workspace ./cmd/okf-workspace`, then `./okf-workspace serve --config ./config.yaml` (full systemd/Docker packaging lands in Plan 04)

## Out of Scope (Deferred to Later Slices / Plans within Phase 0)

> Explicit so later work does not re-litigate the skeleton's minimalism.

- **Safe-path resolver, Git commit spine, job worker, repo seed** → Plan 02 (the skeleton only needs SQLite + login; `repo`/`gitstore`/`jobs` foundations land next).
- **RBAC `RequireRole` enforcement, `/admin` user management, self-service profile, forced first-login password change, CLI password reset** → Plan 03.
- **Audit log (SQLite mirror + slog line), full `config.yaml` schema coverage, systemd + Docker packaging** → Plan 04.
- Page create/edit/render, frontmatter parse/repair, Markdown rendering, file tree as a working navigator, commit-on-save, history/restore → **Phase 1**.
- Attachments, text extraction → **Phase 2**. Search → **Phase 3**. Eino agent → **Phase 4**. Soft locks / presence / conflict UX → **Phase 5**.
- Login rate-limiting / account lockout, web-based password recovery, Git remote push, configurable starter-folder sets, SSO → **deferred** (CONTEXT.md Deferred Ideas).

## Subsequent Slice Plan

Each later phase adds one vertical slice on top of this skeleton without altering its architectural decisions:

- **Phase 1 — OKF Pages, Navigation & Hidden Git:** page create/edit/render, frontmatter parse + byte-stable round-trip, working file tree, automatic batched commit-on-save (reuses the Plan-02 single-writer Git spine + `internal/jobs`), history/restore, per-file optimistic-concurrency floor.
- **Phase 2 — Attachments & Text Extraction:** upload/download originals, MIME-sniffed validation (reuses the Plan-02 safe-path resolver), three-part attachment model, ExtractJob (reuses the job worker), commit-on-upload.
- **Phase 3 — Search:** Bleve index over pages + extracted text, IndexJob, rebuild-from-files reindex.
- **Phase 4 — Eino Agent:** sandboxed ReAct tool-caller (reads route through `repo.Resolve`), approval-gated diff apply.
- **Phase 5 — Collaboration:** soft locks + presence (SSE) + conflict resolution UX hardening the optimistic-concurrency floor.

## Architectural Invariants (contract for all later phases)

1. **Files are truth; SQLite/Bleve are rebuildable caches.** Never store wiki content in SQLite. Data flows one way: write the file first, then project to caches.
2. **Every filesystem access for content goes through `internal/repo`'s safe resolver.** Nothing else constructs repo absolute paths.
3. **Every repo write (incl. `git add`/`commit`/push) goes through the single-writer `internal/gitstore` worker.** Never `git` from a handler or concurrently.
4. **Passwords are Argon2id (never bcrypt as default); secrets never enter logs or, later, the agent context.**
5. **Sessions are HTTPOnly + SameSite cookies; every mutating request is CSRF-protected (no partial coverage).**
6. **Authorization is enforced server-side on every protected action; the client is never trusted for role gating.**
