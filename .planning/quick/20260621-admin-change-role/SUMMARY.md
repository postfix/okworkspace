---
quick_id: 260621-1lr
slug: admin-change-role
status: complete
created: 2026-06-21
completed: 2026-06-21
---

# Summary: Admin "change user role" UI

## What changed

- `web/src/api/client.ts` — added `setUserRole(id, role)` → `PUT /api/v1/admin/users/{id}/role`
  with `{ role }`, reusing the CSRF-aware `mutate` helper.
- `web/src/routes/Admin.tsx` — added a per-row **Change role** action that opens a
  dialog with a role `<select>`; on confirm it calls `setUserRole` and invalidates
  the users query (`["admin","users"]`).
  - `isLastActiveAdmin(u)` mirrors the server `ErrLastAdmin` invariant; the dialog
    shows a note and disables **Save role** when demoting the only active admin
    (selected role ≠ admin).
  - **Save role** is also disabled when the role is unchanged.
  - Server errors (incl. a 409 last-admin race) surface inline via the mutation's
    `onError`.

## Why

Phase 0 UAT found the backend role endpoint (`handleSetRole`) was complete and
tested but unreachable from the UI — role was only settable at user creation.

## Verification

- `npm run build` (tsc -b && vite build) — passes, no type errors.
- Frontend test suite — 109/109 pass (14 files).
- Backend `go test ./internal/server -run 'Role|SetRole|User'` — ok (no backend change).

## No backend changes

The endpoint, RBAC gate, audit record, and last-admin guard already existed.

## Commit

- `436f9d7` fix(admin): wire change-user-role UI to existing setRole endpoint
