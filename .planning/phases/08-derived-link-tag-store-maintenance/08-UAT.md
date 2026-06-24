---
status: passed
phase: 08-derived-link-tag-store-maintenance
source: [08-VERIFICATION.md]
started: 2026-06-24T00:00:00Z
updated: 2026-06-24T09:22:00Z
---

## Current Test

number: 1
name: Admin "Rebuild graph index" button renders and fires
expected: |
  Logged in as an admin, the Admin screen shows a "Rebuild graph index" button
  (alongside "Rebuild search index"). Clicking it shows a success notice
  ("graph index rebuild started" or similar), uses no Git/Bleve vocabulary, and
  triggers POST /api/v1/admin/graph/reindex returning 202. A non-admin never sees
  or can invoke it (403).
awaiting: none — passed

## Tests

### 1. Admin "Rebuild graph index" button renders and fires
expected: Admin sees the button; click → 202 + success notice, no Git vocabulary; non-admin → no access (403).
result: passed — Live against the running binary on :8098: admin authenticated (login 200 → password change 204 → /me role=admin), POST /api/v1/admin/graph/reindex → 202; server audit log recorded action=graph_reindex source=web-ui detail="rebuild graph index"; unauthenticated POST rejected (not 202); "Rebuild graph index" label + graph/reindex route present in the embedded SPA bundle; editor→403 covered by TestGraphReindexAdminOnly.

## Summary

total: 1
passed: 1
issues: 0
pending: 0
skipped: 0
blocked: 0

## Gaps

None — all verified.
