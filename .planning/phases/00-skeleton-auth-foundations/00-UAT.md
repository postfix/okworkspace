---
status: testing
phase: 00-skeleton-auth-foundations
source: [00-VERIFICATION.md]
started: "2026-06-18T13:09:36Z"
updated: "2026-06-18T13:09:36Z"
---

## Current Test

number: 1
name: Login, display name, and logout from any page
expected: |
  Build and run the binary against an empty data dir (see setup below). Sign in
  as admin (after the forced password change) and confirm your display name shows
  in the top bar, and that "Log out" in the top-bar user menu works from any page.
awaiting: user response

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
result: [pending]

### 2. Session persists across browser refresh (HTTPOnly + SameSite cookie)
expected: After signing in, refresh the browser (F5) → you remain authenticated (no re-login). DevTools → Application → Cookies shows `okf_session` with HttpOnly and SameSite=Lax. (AUTH-03)
result: [pending]

### 3. Server-side RBAC denial (curl bypass returns 403)
expected: Create an editor `ed`; capture its session cookie (or use the SPA), then `curl` an admin route with the editor's cookie, e.g. `GET /api/v1/admin/users` → HTTP 403 (server-enforced, not just hidden in the UI). A non-admin navigating to /admin in the SPA is also denied + redirected. (AUTH-05)
result: [pending]

### 4. Forced password-change gate cannot be bypassed in a live session (CR-01 fix)
expected: Sign in as a user holding a one-time/temporary password but do NOT change it. Attempt to call a protected endpoint directly (e.g. `curl -b <session> PUT /api/v1/profile` or `GET /api/v1/admin/users`) → HTTP 403 ("Set a new password to continue."), while `GET /api/v1/auth/me` and the password-change endpoint remain reachable. The temp-password session is NOT privileged. (D-02, AUTH-06 / CR-01)
result: [pending]

### 5. First-run Git seed produces exactly one seed commit
expected: After a fresh first run against an empty data dir, `git -C /tmp/okf-verify-data/repo log --oneline` shows exactly one admin-authored "Seed starter workspace" commit and the SPEC §9 starter layout on disk. Restarting does not re-seed. (SEC-04)
result: [pending]

## Summary

total: 5
passed: 0
issues: 0
pending: 5
skipped: 0
blocked: 0

## Gaps
