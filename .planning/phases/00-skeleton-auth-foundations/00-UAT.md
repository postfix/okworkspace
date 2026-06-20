---
status: complete
phase: 00-skeleton-auth-foundations
source: [00-VERIFICATION.md]
started: "2026-06-18T13:09:36Z"
updated: "2026-06-21T00:00:00Z"
---

## Current Test

[testing complete]

## Setup

```bash
cd /home/john/go/src/github.com/postfix/okworkspace
( cd web && npm run build ) \
  && CGO_ENABLED=0 go build -o okf-workspace ./cmd/okf-workspace \
  && rm -rf /tmp/okf-verify-data \
  && ./okf-workspace serve --config /tmp/okf-verify.yaml
# App on http://localhost:8099 ; copy the one-time admin password from the log.
```

## Tests

### 1. Login, display name, and logout from any page
expected: Sign in as admin → forced "Set a new password" → set a 12+ char password → land on /app with your display name in the top bar. Open the user menu and "Log out" from /admin (any page) → redirect to /login. (AUTH-01, AUTH-02)
result: pass

### 2. Session persists across browser refresh (HTTPOnly + SameSite cookie)
expected: After signing in, refresh the browser (F5) → you remain authenticated (no re-login). DevTools → Application → Cookies shows `okf_session` with HttpOnly and SameSite=Lax. (AUTH-03)
result: pass

### 3. Server-side RBAC denial (curl bypass returns 403)
expected: Create an editor `ed`; capture its session cookie (or use the SPA), then `curl` an admin route with the editor's cookie, e.g. `GET /api/v1/admin/users` → HTTP 403 (server-enforced, not just hidden in the UI). A non-admin navigating to /admin in the SPA is also denied + redirected. (AUTH-05)
result: pass
note: Verified via incognito browser as editor — admin route denied (403); SPA /admin redirects editor to main page.

### 4. Forced password-change gate cannot be bypassed in a live session (CR-01 fix)
expected: Sign in as a user holding a one-time/temporary password but do NOT change it. Attempt to call a protected endpoint directly (e.g. `curl -b <session> PUT /api/v1/profile` or `GET /api/v1/admin/users`) → HTTP 403 ("Set a new password to continue."), while `GET /api/v1/auth/me` and the password-change endpoint remain reachable. The temp-password session is NOT privileged. (D-02, AUTH-06 / CR-01)
result: pass
note: |
  Verified by Claude via curl against a live server (fresh /tmp/okf-verify-data, temp-password admin session):
  - GET /auth/me           -> 200 (must_change_password:true, reachable)
  - GET /admin/users       -> 403 {"error":"Set a new password to continue."}
  - PUT /profile           -> 403 {"error":"Set a new password to continue."}
  - PUT /profile/password  -> 400 (length validation only — NOT gated; the gate lets it through)
  Note: CSRF on mutating calls requires a same-origin signal (nosurf v1.2.0 ensureSameOrigin) — curl must send `Sec-Fetch-Site: same-origin` (or a matching Origin/Referer). This is correct app behavior, not a defect.

### 5. First-run Git seed produces exactly one seed commit
expected: After a fresh first run against an empty data dir, `git -C /tmp/okf-verify-data/repo log --oneline` shows exactly one admin-authored "Seed starter workspace" commit and the SPEC §9 starter layout on disk. Restarting does not re-seed. (SEC-04)
result: pass
note: |
  Verified by Claude. Fresh run: `git rev-list --count HEAD` = 1; single commit `b84ac35 Seed starter workspace` authored by `admin <admin@okf-workspace.local>`. Tracked files: .okf-workspace/manifest.json, index.md, architecture/index.md, decisions/index.md, runbooks/index.md (SPEC §9 layout). Restart against the same data dir produced no "admin user created"/"seeded" log lines and commit count remained 1 — no re-seed.

## Summary

total: 5
passed: 5
issues: 1
pending: 0
skipped: 0
blocked: 0

## Gaps

- truth: "An admin can change an existing user's role from the /admin user-management UI"
  status: resolved
  resolution: "Fixed by quick task admin-change-role (commit 436f9d7): added setUserRole() to the API client and a per-row 'Change role' dialog in Admin.tsx with the last-admin guard + users-query invalidation. Build + 109 frontend tests + backend role tests green."
  reason: "User reported: it's impossible to edit a user's rights as admin after creating them. Backend PUT /api/v1/admin/users/{id}/role (handleSetRole) exists and is tested, but web/src/routes/Admin.tsx has no setRole control — the API client exports no setRole, and the Role column is a read-only RoleBadge. Role can only be set at creation time (add-role dropdown)."
  severity: major
  test: 3
  found_during: "Test 3 setup (creating editor 'ed')"
  artifacts: ["internal/server/handlers_users.go:179 handleSetRole", "internal/server/router.go:132 PUT /admin/users/{id}/role", "web/src/routes/Admin.tsx", "web/src/api/client.ts (no setRole)"]
  missing: ["web/src/api/client.ts setRole() client fn", "Admin.tsx per-row 'Change role' control wired to PUT /admin/users/{id}/role with last-admin guard + query invalidation"]
