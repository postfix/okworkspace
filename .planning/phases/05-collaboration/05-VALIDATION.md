---
phase: 5
slug: collaboration
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-06-22
---

# Phase 5 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework (backend)** | `go test` (Go 1.26, table-driven; injected `now func()` for TTL/expiry) |
| **Framework (frontend)** | Vitest 3.x (`vitest run`) + @testing-library |
| **Config file** | `go.mod` · `web/vitest.config.ts` |
| **Quick run command** | `go test ./internal/locks/... ./internal/pages/...` · `cd web && npx vitest run src/components/SoftLockBanner src/components/PresenceIndicator src/components/DiffReviewDialog` |
| **Full suite command** | `go test ./...` · `cd web && npm test` |
| **Estimated runtime** | ~30–90s backend · ~20–40s frontend |

---

## Sampling Rate

- **After every task commit:** Run the quick command for the touched layer.
- **After every plan wave:** Run the full suite for the touched layer(s).
- **Before `/gsd-verify-work`:** `go test ./...` AND `cd web && npm test` must be green.
- **Max feedback latency:** 90 seconds.

---

## Per-Task Verification Map

> Populated by the planner from the PLAN.md task list. The load-bearing tests below (from RESEARCH §Validation Architecture) are mandatory deliverables.

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| (planner) | — | — | COLL-03 | T-05-* | **Stale save still 409s AFTER force-edit** (force-edit is lock-only; never bypasses BaseRevision) — the load-bearing rule | unit | `go test ./internal/pages/ -run TestForceEditStillRejectsStaleSave` | ❌ W0 | ⬜ pending |
| (planner) | — | — | COLL-02 | T-05-* | Lock acquire/refresh/force/release + **expired lock GC'd** (now()-driven TTL) | unit | `go test ./internal/locks/ -run 'TestLock|TestGC'` | ❌ W0 | ⬜ pending |
| (planner) | — | — | COLL-04 | — | **Save-as-copy creates a fresh page and never mutates the original**; overwrite re-saves at the current revision | unit | `go test ./internal/pages/ -run TestSaveAsCopy` | ❌ W0 | ⬜ pending |
| (planner) | — | — | COLL-04 | — | DiffReviewDialog conflict mode renders a real diff; focus NOT on Overwrite; 3 choices wired | unit | `cd web && npx vitest run src/components/DiffReviewDialog` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/locks/store_test.go` — acquire/refresh/force/release + expiry (injected `now`) + the GC reaper.
- [ ] `internal/pages/forceedit_test.go` (or extend the existing save test) — the COLL-03 load-bearing rule: a stale save 409s even after a force-edit took the lock.
- [ ] `internal/pages/saveascopy_test.go` — save-as-copy births a new page (uniquePath) and leaves the original byte-identical.
- [ ] `web/src/components/DiffReviewDialog.test.tsx` — extend with conflict-mode assertions (real diff, focus not on Overwrite, 3 buttons).

*Backend `go test` + frontend Vitest infrastructure already exist (Phases 0–7) — Wave 0 adds the collaboration-specific test files, not new frameworks.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Presence indicator updates live across two sessions | COLL-01 | Needs two concurrent browser sessions + live SSE | Open the same page as two users; confirm each sees "X is editing" appear/disappear as the other enters/leaves edit mode. |
| Soft-lock warning + force-edit | COLL-02 | Perceptual + two sessions | User A edits (acquires lock); user B opens → sees the warning banner + read-only editor + "Force edit"; B force-edits → takes over. |
| Conflict resolution 3-way | COLL-04 | Needs a real concurrent-edit race + human diff judgment | Two sessions edit + save the same page; the second save shows the conflict dialog; verify Overwrite / Manual merge / Save-as-copy each behave correctly and the original is never silently lost. |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags (`vitest run`, not `vitest`)
- [ ] Feedback latency < 90s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
