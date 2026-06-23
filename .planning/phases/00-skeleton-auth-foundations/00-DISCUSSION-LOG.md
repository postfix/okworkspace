# Phase 0: Skeleton, Auth & Foundations - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-06-17
**Phase:** 0-Skeleton, Auth & Foundations
**Areas discussed:** First admin login, Team provisioning, First-run wiki content, Phase-0 landing screen

---

## First admin login

### Q1 — How does the bootstrapped admin receive their initial password on first startup?

| Option | Description | Selected |
|--------|-------------|----------|
| Random pw to log | Generate a strong random password on first boot, print once to stdout/server log; admin logs in then forced to set a new one. Zero config, no fixed default. | ✓ |
| Creds from config/env | Read initial admin username + password from config.yaml / env var; force change on first login. | |
| Interactive CLI | Separate `okf-workspace admin create` command prompts for username + password before the server is usable. | |

**User's choice:** Random pw to log
**Notes:** Matches the "just run the binary" self-host goal; no fixed default to leak (PITFALLS flag). Forced change on first login retained.

### Q2 — How should admin password reset / lockout recovery work in Phase 0?

| Option | Description | Selected |
|--------|-------------|----------|
| CLI reset command | Local `okf-workspace admin reset-password <user>` regenerates a one-time password printed to the log (requires shell access — correct trust boundary). | ✓ |
| Re-bootstrap on empty | No dedicated reset; delete users table/app.db to re-trigger first-run bootstrap. | |
| Defer to later phase | Ship only the bootstrap path; recovery is a later concern. | |

**User's choice:** CLI reset command
**Notes:** Admin username configurable (default `admin`) decided as a minor config default, not a separate question.

---

## Team provisioning

### Q1 — How should teammate accounts + roles be created in Phase 0?

| Option | Description | Selected |
|--------|-------------|----------|
| In-app admin screen | Minimal /admin user-management UI (list, add, assign role, reset pw, deactivate) + API. Non-technical admin onboards team without shell access. | ✓ |
| Seed via config/CLI | Users from config.yaml or CLI; defer in-app UI. Requires server shell access to add anyone. | |
| Admin-only this phase | Only bootstrap admin exists; RBAC scaffolded but multi-user provisioning deferred. | |

**User's choice:** In-app admin screen
**Notes:** Aligns with the "non-technical teammate" core value and SPEC §18 (/admin route + AdminSettings). Makes RBAC real in Phase 0.

### Q2 — Can a regular (non-admin) user manage their own account in Phase 0?

| Option | Description | Selected |
|--------|-------------|----------|
| Self-service profile | Any logged-in user changes their OWN password + display name; admin manages accounts + role assignment. | ✓ |
| Admin-controlled only | Only admin changes passwords, display names, roles for everyone. | |
| You decide | Claude picks the conventional split during planning. | |

**User's choice:** Self-service profile
**Notes:** A user cannot change their own role; admin owns role assignment.

---

## First-run wiki content

### Q1 — What should first-run repo initialization create as content?

| Option | Description | Selected |
|--------|-------------|----------|
| Welcome index.md | Seed a single root index.md (valid frontmatter + welcome body) as the initial commit. | |
| index.md + starter folders | Also scaffold runbooks/, architecture/, decisions/ each with index.md (SPEC §9 layout). | ✓ |
| Empty repo | git init only; first page created in Phase 1. | |

**User's choice:** index.md + starter folders
**Notes:** Claude added guidance (not asked): only seed when the repo is brand-new; skip seeding if a remote is configured to pull existing content, to avoid clobbering. Seed produces the first commit through the single-writer Git spine.

---

## Phase-0 landing screen

### Q1 — What should a logged-in user see in Phase 0, before page editing exists?

| Option | Description | Selected |
|--------|-------------|----------|
| Real AppShell skeleton | Build the actual AppShell (left nav rail + top bar with display name + logout + main pane); show seeded tree read-only with a "page editing arrives next" placeholder. | ✓ |
| Minimal welcome page | Centered welcome/status screen (signed-in name, health, logout); no tree/shell chrome. | |
| You decide | Claude picks the right amount of shell during planning. | |

**User's choice:** Real AppShell skeleton
**Notes:** Front-loads most of the Phase-1 shell so Phase 1 slots in. Phase has `UI hint: yes` → a `/gsd-ui-phase 0` design contract is appropriate.

---

## Claude's Discretion

Decided with sensible defaults (surfaced to the user, not contested):
- **Session cookies:** SameSite=Lax, HTTPOnly, Secure-under-TLS; cookie `okf_session`; TTL 168h; SCS SQLite store.
- **CSRF:** nosurf on all mutating routes (no partial coverage).
- **Audit log:** write-only in Phase 0 (SQLite mirror + structured log line); no viewer UI yet.
- **Login hardening:** basic only (Argon2id verify + generic error); no rate-limiting/lockout at 5-user scale.
- **Remote Git sync:** Phase 0 local-only `git init`; if remote_enabled + pull_on_startup → fast-forward-only pull, alert on divergence (never auto-merge), skip seeding; push deferred to Phase 1.
- **Startup self-heal:** clear stale index.lock (no live git proc), quick health check, basic repo-health status.

## Deferred Ideas

- In-app audit-log viewer UI
- Login rate-limiting / account lockout
- Web-based password recovery (email / forgot-password)
- Git remote push (Phase 1)
- Configurable/custom starter-folder sets
- SSO / external identity (SPEC §4 non-goal)
