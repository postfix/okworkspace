---
phase: 00-skeleton-auth-foundations
plan: 03
subsystem: auth
tags: [go, rbac, require-role, user-management, profile, forced-password-change, logout, cli-reset, react, dialog, csrf, argon2id]

# Dependency graph
requires:
  - phase: 00-01
    provides: session-bound user + Authenticate + Argon2id hash/verify, users.Repository (Create/GetByID/List/UpdateRole/UpdatePassword/SetActive/UpdateDisplayName) + bootstrap admin, chi server Deps + nosurf CSRF, embedded React SPA (AppShell, api/client.ts, Login)
  - phase: 00-02
    provides: server Deps wiring + health endpoint that the RBAC/admin surface reuses; the single-writer Git + safe-path spine (not exercised by this plan but part of the same Deps)
provides:
  - internal/auth RBAC — RequireRole(role) middleware authorizing from the SESSION-bound user only (401 unauth, 403 "You don't have permission to do that."), Role constants (admin/editor/reader), CurrentUser(ctx)/WithCurrentUser(ctx) + SessionUser interface (keeps auth free of a users import; admin is a superset for this phase's gates)
  - internal/users management service — Create (one-time password, must_change_password=1), List, SetRole, ResetPassword (one-time, returned once, never a fixed default), Deactivate (active=0; Authenticate rejects inactive), self-service UpdateOwnProfile + ChangeOwnPassword (no role parameter — a user can never change their own role, D-06)
  - internal/server admin + profile API — /api/v1/admin/users (GET list, POST create), /{id}/role (PUT), /{id}/reset-password (POST), /{id}/deactivate (POST) all behind RequireRole(admin)+CSRF; /api/v1/profile (PUT) + /api/v1/profile/password (PUT) for the current user only
  - cmd/okf-workspace admin reset-password <username> — CLI lockout-recovery subcommand (D-04), same shell trust boundary as bootstrap, prints a one-time password once
  - React surfaces — token-driven Dialog (focus-trapped, Esc/backdrop cancel, backdrop NEVER confirms a destructive action), Table, RoleBadge, UserMenu (popover with Profile + Log out reachable from any authenticated page, AUTH-02); routes /admin, Profile, ForcePasswordChange (gates the app when must_change_password); api/client.ts admin+profile calls with CSRF header
affects: [phase-1 (editor-vs-reader content gating gains meaning on top of RequireRole; pages write behind the same RBAC + CSRF boundary), phase-2 (attachment routes reuse RequireRole), phase-4 (agent prompt/approval routes role-gated), phase-0 plan-04 (audit log wires the account/config change events these handlers are audit-ready for)]

# Tech tracking
tech-stack:
  added: []   # no new modules; reused stdlib net/http + existing argon2id (via internal/auth), modernc sqlite, nosurf CSRF, react-query, lucide-react. UI uses hand-built primitives (Plan-01 convention) — no new npm dep.
  patterns:
    - "Authorization is ALWAYS derived from the session-bound user's role (auth.CurrentUser), never from a client header/body/query field — RequireRole reads only the SessionUser on the request context (T-00.03-01)"
    - "auth declares a minimal SessionUser interface (UserID/UserRole) and *users.User satisfies it via a server-side adapter, so internal/auth never imports internal/users (one-directional deps, mirrors Plan-01's auth.UserLookup break of the import cycle)"
    - "Self-service mutations (UpdateOwnProfile/ChangeOwnPassword) accept NO role parameter and operate only on the session user's id — role escalation is impossible by construction, not by validation (D-06, T-00.03-02)"
    - "Reset/Create always generate a strong one-time password (Argon2id-hashed) with must_change_password=1 — never a fixed/default credential (T-00.03-05)"
    - "Dialog focus effect depends on [open] ONLY; the latest onCancel is read through a ref synced in a separate effect, so keystroke-driven re-renders never re-fire the focus-into-dialog logic and steal the caret"
    - "Dialog backdrop-click and Esc invoke onCancel only; the explicit confirm button is the sole path to onConfirm, so a stray backdrop click can never confirm a destructive Deactivate/Reset (UI-SPEC interaction contract)"

key-files:
  created:
    - internal/auth/rbac.go
    - internal/auth/rbac_test.go
    - internal/users/manage.go
    - internal/users/manage_test.go
    - internal/server/handlers_users.go
    - internal/server/handlers_users_test.go
    - internal/server/handlers_profile.go
    - cmd/okf-workspace/admin.go
    - cmd/okf-workspace/admin_test.go
    - web/src/routes/Admin.tsx
    - web/src/routes/Profile.tsx
    - web/src/routes/ForcePasswordChange.tsx
    - web/src/components/Dialog.tsx
    - web/src/components/Table.tsx
    - web/src/components/RoleBadge.tsx
    - web/src/components/UserMenu.tsx
  modified:
    - internal/server/router.go
    - cmd/okf-workspace/main.go
    - web/src/routes/AppShell.tsx
    - web/src/api/client.ts

key-decisions:
  - "RequireRole reads role only from the session-bound user via a SessionUser interface; admin is a superset for this phase (editor/reader content gating becomes meaningful in Phase 1, D-07)"
  - "auth defines SessionUser (UserID/UserRole) so it never imports users — *users.User is adapted on the server side, preserving the one-directional auth<->users dependency"
  - "Self-service profile/password paths take no role parameter at all (D-06) — own-role escalation is structurally impossible, not merely validated away"
  - "Create/ResetPassword always emit a strong one-time password with must_change_password=1 (managedPasswordLen=28); never a fixed default (T-00.03-05)"
  - "Dialog keeps the latest onCancel in a ref and depends on [open] only — fixes the verification-found caret-stealing bug where the focus effect re-fired on every keystroke (commit 0fc476e)"
  - "RBAC denial copy is the single neutral message 'You don't have permission to do that.' (UI-SPEC), and 401 uses 'Your session expired. Sign in again to continue.'"

patterns-established:
  - "Admin routes mounted as a chi sub-router with admin.Use(auth.RequireRole(auth.RoleAdmin)) layered on top of the global nosurf CSRF (defense in depth: RBAC + CSRF on every mutating admin action)"
  - "Profile routes are authenticated-only and act on the session user's own id (no id path param), so a user can only ever edit themselves"
  - "Token-driven UI primitives (Dialog/Table/RoleBadge/UserMenu) use only var(--…) tokens; accent reserved for the single primary CTA + active nav + focus ring; destructive red reserved for Deactivate"

requirements-completed: [AUTH-02, AUTH-05]

# Metrics
duration: 84min
completed: 2026-06-17
---

# Phase 0 Plan 03: RBAC & Team-Management Summary

**Server-side `RequireRole` RBAC enforced from the session (never client input), an admin user-management screen + API (create / set-role / reset / deactivate), self-service profile that can change everything but the caller's own role, a forced first-login password change that gates the app, a logout control reachable from any page, and a CLI `admin reset-password` recovery path — plus a verification-found Dialog caret-stealing bug fixed.**

## Performance

- **Duration:** ~84 min wall (RED 20:16 → fix 21:40), including the human-verification checkpoint
- **Started:** 2026-06-17T20:16:54+03:00 (RED)
- **Completed:** 2026-06-17T21:40:21+03:00 (dialog focus fix)
- **Tasks:** 2 (Task 1 TDD: RED → GREEN; Task 2 UI + human-verify checkpoint, approved)
- **Source files created/modified:** 20 (16 created, 4 modified)

## Accomplishments

- **AUTH-05 — server-side RBAC** (`internal/auth/rbac.go`): `RequireRole(role)` middleware authorizes solely from the SESSION-bound user (via `CurrentUser(ctx)` / the `SessionUser` interface) — never a client header, body field, or query param. Returns **401** ("Your session expired…") when unauthenticated and **403** ("You don't have permission to do that.") when the role is insufficient; admin is a superset of every gate this phase. `auth` declares the minimal `SessionUser` interface so it never imports `users` (the server adapts `*users.User`).
- **D-05 — admin user management** (`internal/users/manage.go` + `internal/server/handlers_users.go`): `Create` (generates a one-time password, Argon2id-hashed, `must_change_password=1`), `List`, `SetRole`, `ResetPassword` (new one-time password, returned once), `Deactivate` (`active=0`; `Authenticate` rejects inactive). Exposed at `/api/v1/admin/users` (+ `/{id}/role`, `/{id}/reset-password`, `/{id}/deactivate`), all behind `RequireRole(RoleAdmin)` **and** nosurf CSRF.
- **D-06 — self-service profile** (`UpdateOwnProfile`, `ChangeOwnPassword` + `internal/server/handlers_profile.go`): a user can change their own display name and password (verifies the current password, enforces ≥12 chars) but the API accepts **no role parameter** and acts only on the session user's own id — own-role escalation is structurally impossible.
- **D-02 — forced first-login password change** (`web/src/routes/ForcePasswordChange.tsx`): when `must_change_password` is set the SPA gates the whole app behind "Set a new password" with the UI-SPEC copy ("You're using a temporary password…", "Choose a longer password — at least 12 characters.", "The two passwords don't match."); the flag is server-tracked, not just hidden in the UI.
- **AUTH-02 — logout from any page** (`web/src/components/UserMenu.tsx` in `AppShell`): a top-bar popover (display name → Profile, Log out) is present on every authenticated page; `/admin` nav shows only to admins.
- **D-04 — CLI recovery** (`cmd/okf-workspace/admin.go`): a cobra `admin reset-password <username>` subcommand resets the named user and prints a one-time password once (same shell trust boundary as bootstrap); an unknown username exits non-zero.
- **Token-driven UI primitives**: `Dialog` (focus-trapped, Esc/backdrop cancel, backdrop **never** confirms a destructive action), `Table`, `RoleBadge`, `UserMenu` — all using `var(--…)` tokens (accent reserved for the single primary CTA + active nav + focus ring; destructive red reserved for Deactivate).
- **Verification-found bug fixed**: during the human-verify checkpoint the add-user/profile dialogs stole the caret after the first keystroke. Root cause: the focus-into-dialog `useEffect` depended on `onCancel`, whose identity changes every render, so every keystroke re-ran the effect and re-focused the first field. Fixed by reading `onCancel` through a ref and depending on `[open]` only (commit `0fc476e`).

## Task Commits

1. **Task 1: RBAC middleware + user-management service + CLI reset** (TDD)
   - `35cfd23` test(00-03): failing RBAC + user-management + CLI reset tests (RED)
   - `da74fe6` feat(00-03): RBAC middleware + user-management service + CLI reset (GREEN)
2. **Task 2: Admin screen + profile + forced password change + logout (UI)** (checkpoint: human-verify)
   - `e6a46cc` feat(00-03): admin screen + profile + forced password change + logout UI
   - `0fc476e` fix(00-03): dialog focus effect re-fired on each keystroke, stealing caret *(verification-found bug)*

**Plan metadata:** committed separately (this SUMMARY + STATE/ROADMAP/REQUIREMENTS + restored `internal/web/dist/.gitkeep`).

## Files Created/Modified

- `internal/auth/rbac.go` — `RequireRole` middleware, `Role` constants, `CurrentUser`/`WithCurrentUser`, `SessionUser` interface, neutral 401/403 JSON copy.
- `internal/users/manage.go` — `Create`/`List`/`SetRole`/`ResetPassword`/`Deactivate` + self-service `UpdateOwnProfile`/`ChangeOwnPassword` (no role param), one-time-password generation.
- `internal/server/handlers_users.go` — admin user CRUD handlers (admin-only + CSRF); `handlers_profile.go` — self-only profile + password handlers.
- `internal/server/router.go` — admin sub-router wrapped in `RequireRole(RoleAdmin)`; profile routes mounted authenticated-only.
- `cmd/okf-workspace/admin.go` + `main.go` — cobra `admin reset-password` subcommand attached to root.
- `web/src/components/Dialog.tsx` (focus-trapped, caret-fix), `Table.tsx`, `RoleBadge.tsx`, `UserMenu.tsx` — token-driven primitives.
- `web/src/routes/Admin.tsx`, `Profile.tsx`, `ForcePasswordChange.tsx`, `AppShell.tsx` (UserMenu in top bar), `web/src/api/client.ts` (admin+profile calls with CSRF header).

## Decisions Made

- **Session-only authorization.** `RequireRole` reads the role exclusively from the session-bound user; admin is a superset this phase (editor/reader content gating arrives in Phase 1, D-07).
- **`SessionUser` interface in `auth`.** Keeps `auth` from importing `users` — the server adapts `*users.User`, preserving the one-directional dependency established in Plan 01.
- **No role parameter on self-service paths.** Own-role escalation is impossible by construction, not validation (D-06).
- **One-time passwords always.** Create/Reset emit strong Argon2id-hashed one-time passwords with `must_change_password=1`; never a fixed default (T-00.03-05).
- **Dialog focus effect depends on `[open]` only.** The latest `onCancel` is read via a ref synced in its own effect — the fix for the caret-stealing bug found in verification.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Dialog focus effect stole the caret on every keystroke**
- **Found during:** Task 2 human-verification (typing into the add-user / profile dialogs).
- **Issue:** The focus-into-dialog `useEffect` listed `onCancel` as a dependency; callers pass a fresh `onCancel` identity every render (`onCancel={() => setOpen(false)}`), so each keystroke-driven re-render re-ran the effect and re-focused the first field, stealing the caret mid-typing.
- **Fix:** Keep the latest `onCancel` in a ref synced by a separate `useEffect`; the focus/keydown effect now depends on `[open]` only and reads `onCancelRef.current`.
- **Files modified:** `web/src/components/Dialog.tsx`
- **Verification:** Re-ran `npm run build` + `npm run lint` (clean); human re-verified the dialogs accept continuous typing and confirmed all six checkpoint steps; approved.
- **Committed in:** `0fc476e`

---

**Total deviations:** 1 auto-fixed (1 Rule 1 bug, surfaced by the human-verify checkpoint).
**Impact on plan:** The fix was necessary for the dialogs to be usable; no scope creep. All other work matched the plan as written.

## Issues Encountered

- The `npm run build` step (Vite) wipes `internal/web/dist/` before writing, deleting the tracked `internal/web/dist/.gitkeep` — the same artifact behavior noted in Plan 02. Built assets are gitignored (only `.gitkeep` is tracked, per the Plan-01 embed convention), so the fresh build was regenerated and embedded, `.gitkeep` was restored, and `CGO_ENABLED=0 go build ./...` confirmed the binary embeds the verified UI (with the dialog fix).
- An external tool continues to leave `.planning/config.json` modified and `.smtc*` artifacts in the working tree; none were authored by this plan and none were staged.

## Verification Results

- **Automated gate (all green):** `CGO_ENABLED=0 go test ./internal/auth/... ./internal/users/... ./internal/server/... ./cmd/okf-workspace/... -count=1` — **PASS**; `CGO_ENABLED=0 go build ./...` — clean (embeds the fresh SPA); `cd web && npm run build && npm run lint` — **exit 0**.
- **RBAC server-side denial:** `rbac_test` confirms admin passes, editor → 403, reader → 403, no session → 401 — all from the session-bound role; `handlers_users_test` exercises the admin API behind `RequireRole(RoleAdmin)`. Every `/api/v1/admin/...` route in `router.go` is wrapped in `RequireRole(RoleAdmin)` (grep-confirmed).
- **Service behavior:** `manage_test` covers create-with-one-time-password, set-role, reset-password (new hash verifies), deactivate-blocks-login, change-own-password (rejects <12 chars + wrong current), and that profile/password paths take no role parameter.
- **CLI:** `admin_test` confirms `admin reset-password <user>` resets and exits 0; unknown user exits non-zero.
- **Human verification: PASSED.** The user approved the checkpoint after the dialog fix — all six steps (forced password change gates the app, self-service profile, admin CRUD with destructive confirmation dialogs, logout reachable from any page, server-side RBAC denial of non-admins, CLI reset) behave as specified.

## Known Stubs

None. Editor-vs-reader *content* gating beyond admin-only `/admin` is intentionally deferred to Phase 1 per D-07 (the `RequireRole` middleware and the three roles are fully built and enforced now); this is a planned scope boundary, not a stub.

## Threat Flags

None — all new surface (the admin + profile API, the RBAC middleware, the forced-password-change gate, the CLI reset, the one-time-password credential path) was anticipated in the plan's `<threat_model>` (T-00.03-01..07, T-00.03-SC). No new npm/go dependencies were added (T-00.03-SC satisfied — UI uses the Plan-01 hand-built primitives). No unplanned network endpoints or trust-boundary schema changes were introduced.

## User Setup Required

None — no external service configuration. The CLI `admin reset-password` shares the bootstrap trust boundary (requires shell access to the host), by design (D-04).

## Next Phase Readiness

- **Plan 04 (audit/observability):** the admin + profile handlers are audit-ready (account/config-change events) for the SEC-05 audit log wiring.
- **Phase 1 (pages):** editor-vs-reader content gating layers onto `RequireRole`; page write routes reuse the same RBAC + CSRF boundary. The byte-stable Markdown round-trip exit gate (carried in STATE) remains ahead.

## Self-Check: PASSED

All 16 created files exist on disk; all 4 task commits (`35cfd23`, `da74fe6`, `e6a46cc`, `0fc476e`) are present in git history. Full Go test suite for the touched packages and the SPA build/lint are green; the human-verify checkpoint was approved.

---
*Phase: 00-skeleton-auth-foundations*
*Completed: 2026-06-17*
