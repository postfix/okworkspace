---
phase: 00
slug: skeleton-auth-foundations
status: approved
nyquist_compliant: true
wave_0_complete: true
created: 2026-06-18
---

# Phase 00 — Validation Strategy

> Per-phase validation contract. Reconstructed retroactively from PLAN/SUMMARY artifacts
> after execution (State B), then gap-filled by the Nyquist auditor (frontend harness added).

---

## Test Infrastructure

This phase ships **two** test stacks: a Go backend suite (present from Wave 1) and a
React frontend component suite (added during this validation pass).

| Property | Backend (Go) | Frontend (React) |
|----------|--------------|------------------|
| **Framework** | `go test` (stdlib + fuzzing) | vitest 3.x + React Testing Library |
| **Config file** | none — stdlib | `web/vitest.config.ts` + `web/src/test/setup.ts` |
| **Quick run command** | `CGO_ENABLED=0 go test ./internal/auth/... ./internal/server/... -count=1` | `cd web && npm run test` |
| **Full suite command** | `CGO_ENABLED=0 go test ./internal/... ./cmd/... -count=1` | `cd web && npm run test` |
| **Estimated runtime** | ~2 s (fuzz pass +20 s when explicitly run) | ~1 s |

---

## Sampling Rate

- **After every task commit:** Run the relevant quick command (Go for backend tasks, vitest for UI tasks).
- **After every plan wave:** Run both full suites.
- **Before `/gsd-verify-work`:** Both suites must be green.
- **Max feedback latency:** ~3 seconds (excluding the optional 20 s fuzz run).

---

## Per-Task Verification Map

Granularity is per-requirement (reconstructed from the four plans). "Plan" column = the
plan that delivered the requirement; "Wave" = execution order of that plan within the phase.

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| AUTH-01 | 01 | 1 | Log in with username + password | — | Argon2id verify; generic error on failure (no enumeration) | unit (go) | `go test ./internal/auth/ ./internal/server/ -run 'Authenticate\|Login\|MeAfterLogin'` | ✅ | ✅ green |
| AUTH-02 | 03 | 3 | Log out from any page | — | Session destroyed on logout; control reachable on every authed page | unit (go) + component (vitest) | `go test ./internal/server/ -run AuditRows`; `npm run test` (UserMenu) | ✅ | ✅ green |
| AUTH-03 | 01 | 1 | Session persists via secure cookie | T-00.01 | `HttpOnly` + `SameSite=Lax` + Secure-when-https | unit (go) | `go test ./internal/server/ -run TestLoginSuccessSetsSecureCookie` | ✅ | ✅ green |
| AUTH-04 | 01 | 1 | Admin user created on first startup | — | One-time password, `must_change_password=1`, no-op on non-empty DB | unit (go) | `go test ./internal/users/ -run TestBootstrapAdmin` | ✅ | ✅ green |
| AUTH-05 | 03 | 3 | Actions gated by role (admin/editor/reader) | T-00.03-01 | Role read from session-bound user only; 401/403 server-side | unit (go) | `go test ./internal/auth/ -run TestRequireRole`; `go test ./internal/server/ -run 'EditorForbidden\|MustChangePasswordGate'` | ✅ | ✅ green |
| AUTH-06 | 01 | 1 | Display name shown in the UI | — | `/me` returns `display_name`; AppShell top bar renders it | unit (go) + component (vitest) | `go test ./internal/server/ -run MeAfterLogin`; `npm run test` (AppShell) | ✅ | ✅ green |
| SEC-01 | 02 | 2 | Safe-path resolver (`../`, absolute, symlink escape) | T-00.02-01 | Lexical reject + EvalSymlinks + boundary prefix + `os.Root` | unit + fuzz (go) | `go test ./internal/repo/ -count=1` (fuzz: `-run=Fuzz -fuzz=FuzzResolve -fuzztime=20s`) | ✅ | ✅ green |
| SEC-03 | 01 | 1 | Passwords hashed with Argon2id | — | PHC-format Argon2id; no bcrypt in production | unit (go) | `go test ./internal/auth/ -run 'HashPassword\|VerifyPassword'` | ✅ | ✅ green |
| SEC-04 | 01 | 1 | HTTPOnly/SameSite cookies; CSRF-protected mutations | T-00.01 | nosurf on all mutating routes; SPA echoes `X-CSRF-Token` | unit (go) | `go test ./internal/server/ -run 'SecureCookie\|WithoutCSRF'` | ✅ | ✅ green |
| SEC-05 | 04 | 4 | Key actions recorded in audit log | T-00.04-02 | Dual write (SQLite + slog); non-fatal; no secrets recorded | unit (go) | `go test ./internal/audit/ ./internal/server/ -run 'Record\|AuditRows'` | ✅ | ✅ green |
| D-02 | 03 | 3 | Forced first-login password change gates the app | T-00.03 / CR-01 | Server-side gate (403 on protected routes); ≥12 chars + match | unit (go) + component (vitest) | `go test ./internal/server/ -run TestMustChangePasswordGate`; `npm run test` (ForcePasswordChange) | ✅ | ✅ green |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

Backend infrastructure existed from Wave 1 (Go `go test`). This validation pass installed the
**frontend** test harness and added the three UI component suites:

- [x] `web/vitest.config.ts` — vitest config (jsdom env, jest-dom matchers via setup)
- [x] `web/src/test/setup.ts` — `@testing-library/jest-dom` matcher registration
- [x] `web/tsconfig.test.json` — vitest globals types for editor/IDE support
- [x] vitest + RTL devDependencies installed; `"test": "vitest run"` script added to `web/package.json`
- [x] `web/src/routes/AppShell.test.tsx` — AUTH-06 (display name render)
- [x] `web/src/components/UserMenu.test.tsx` — AUTH-02 (logout reachable + invokes logout)
- [x] `web/src/routes/ForcePasswordChange.test.tsx` — D-02 (≥12-char + match validation + submit)

Backend: existing infrastructure covers all backend requirements (50+ test functions, all green).

---

## Manual-Only Verifications

Every phase requirement has automated coverage (above). The following are **supplementary
live-environment confirmations** of full browser/disk/git stack flows whose constituent parts
are already automated — they are not the sole verification of any requirement. They are
persisted as UAT in `00-UAT.md`.

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Login → display name → logout, end-to-end in a browser | AUTH-01, AUTH-02 | SPA render + cookie + redirect can't be unit-verified | 00-UAT.md #1 |
| Session survives a browser refresh (F5) | AUTH-03 | Live cookie round-trip + session-store read-back | 00-UAT.md #2 |
| Server-side RBAC denial via `curl` with an editor cookie | AUTH-05 | Needs a live editor-role session cookie | 00-UAT.md #3 |
| Forced-password gate cannot be bypassed in a live session (CR-01) | D-02, AUTH-06 | Client redirect + server 403 interplay needs a live session | 00-UAT.md #4 |
| First-run Git seed produces exactly one seed commit | SEC-04, AUTH-04 | Full startup against a real data dir + git repo | 00-UAT.md #5 |

---

## Validation Sign-Off

- [x] All requirements have an `<automated>` verify (Go unit/fuzz and/or vitest component)
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [x] Wave 0 covers all MISSING references (frontend harness installed; UI gaps filled)
- [x] No watch-mode flags (`vitest run`, not `vitest --watch`)
- [x] Feedback latency < 3 s
- [x] `nyquist_compliant: true` set in frontmatter

**Approval:** approved 2026-06-18

---

## Validation Audit 2026-06-18

| Metric | Count |
|--------|-------|
| Requirements audited | 10 + D-02 |
| Automated (backend, pre-existing) | 8 fully + 2 backend-half |
| Gaps found (frontend UI) | 3 |
| Resolved (frontend component tests added) | 3 |
| Escalated | 0 |
| Manual-only (live-environment E2E confirmations) | 5 (persisted as UAT) |

**Frontend harness added:** vitest 3.x + React Testing Library (11 tests across 3 files, all green).
Backend suite unchanged (do-not-touch); `CGO_ENABLED=0 go test ./internal/... ./cmd/...` green across 11 packages.
