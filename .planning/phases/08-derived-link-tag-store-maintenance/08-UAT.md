---
status: testing
phase: 08-derived-link-tag-store-maintenance
source: [08-VERIFICATION.md]
started: 2026-06-24T00:00:00Z
updated: 2026-06-24T00:00:00Z
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
awaiting: user response

## Tests

### 1. Admin "Rebuild graph index" button renders and fires
expected: Admin sees the button; click → 202 + success notice, no Git vocabulary; non-admin → no access (403).
result: [pending]

## Summary

total: 1
passed: 0
issues: 0
pending: 1
skipped: 0
blocked: 0

## Gaps
