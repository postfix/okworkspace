---
quick_id: 260621-1lr
slug: admin-change-role
status: in-progress
created: 2026-06-21
source: Phase 0 UAT gap (.planning/phases/00-skeleton-auth-foundations/00-UAT.md)
---

# Quick Task: Admin "change user role" UI

## Problem

Phase 0 UAT found that an admin cannot change an existing user's role from the UI.
The backend `PUT /api/v1/admin/users/{id}/role` (`handleSetRole`) exists and is
tested — including a last-admin guard (409 `ErrLastAdmin`) — but the SPA never
wired it. `web/src/api/client.ts` has no role function and `Admin.tsx` renders the
role as a read-only `<RoleBadge>`. Role was only settable at user-creation time.

## Change

1. `web/src/api/client.ts` — add `setUserRole(id, role)` calling
   `PUT /api/v1/admin/users/{id}/role` with `{ role }` (reuses the CSRF `mutate` helper).
2. `web/src/routes/Admin.tsx` — add a per-row "Change role" action that opens a
   dialog with a role `<select>`; on confirm calls `setUserRole` and invalidates
   the users query.
   - Mirror the server last-admin invariant (like `canDeactivate`): when the target
     is the only active admin, disable demotion to editor/reader.
   - Disable confirm when the role is unchanged.
   - Surface the server's 409 message inline if the guard is somehow hit.

## Verification

- `npm run build` (tsc + vite) passes — no type errors.
- Existing Go tests for `handleSetRole` remain green (no backend change).

## Out of scope

- No backend changes (endpoint already complete + tested).
