---
author: engineer
responsible: engineer
phase: "15-01"
status: accepted
artifact_kind: SUMMARY
emitted_at: 2026-06-27T20:47:00Z
requirements: [UX-01]
commits:
  - 712fb12..<HEAD>
provenance:
  plan: .planning/phases/15-ux-improvement/15-01-PLAN.md
  review: .planning/phases/15-ux-improvement/15-REVIEW.md
---

## Phase 15-01 SUMMARY — Bulk-Approve Non-Blocking Progress

**Objective:** Replace the fire-and-forget bulk-approve UX with a live progress indicator. Backend exposes sweep job progress; admin UI polls status every 2s while jobs are in-flight.

### Tasks Completed

#### Task 1: Sweep progress tracking in tagsweep store

- **`SweepProgress`** struct (`Queued`, `Processing`, `Done`, `Failed`, `Total` ints, all JSON-tagged for the HTTP response).
- **`Store.SweepStatus(ctx)`** — queries the shared `jobs` table (`SELECT status, COUNT(1) WHERE kind='tag_suggest' GROUP BY status`), mapping `pending→Queued`, `running→Processing`, `done→Done`, `failed→Failed`. Scoped to `tag_suggest` kind only so other job types are not counted.
- **`Store.PendingApprovalCount(ctx)`** — `SELECT COUNT(1) FROM tag_suggestions WHERE status='pending'` for the review queue badge count.
- Status constants duplicated locally (documented rationale: avoids import cycle with `internal/jobs`).
- Nil-db guards on both methods (zero value, no panic).
- Unit tests: `TestSweepStatusGroupsByStatus`, `TestSweepStatusNilDB`, `TestSweepStatusEmpty`, `TestPendingApprovalCount`, `TestPendingApprovalCountNilDB`.
- **Files:** `internal/tagsweep/store.go`, `internal/tagsweep/store_test.go`

#### Task 2: GET /api/v1/admin/tags/sweep/status endpoint

- **`handleTagSweepStatus`** handler: calls `SweepStatus` and `PendingApprovalCount`, returns `{progress: SweepProgress, pending_approval: int}` as JSON 200. Error paths logged and return `tagSweepUnavailable` (no internal vocabulary leaked).
- Route registered in `router.go` inside the `admin` subgroup (behind `RequireRole(admin)`): `admin.Get("/admin/tags/sweep/status", h.handleTagSweepStatus)`.
- Server tests: `TestTagSweepStatusAdminOnly` (403 vs 200 RBAC gate), `TestTagSweepStatusReturnsProgress` (seeded jobs + pending approval, verifies JSON shape), `TestTagSweepStatusEmpty`.
- **Files:** `internal/server/handlers_tagsweep.go`, `internal/server/router.go`, `internal/server/handlers_tagsweep_test.go`

#### Task 3: Progress polling in admin UI

- **`SweepProgress`** and **`SweepStatusPayload`** TypeScript interfaces matching backend JSON shape.
- **`getSweepStatus()`** API client function (`GET /api/v1/admin/tags/sweep/status`).
- **`TagReviewView`** updated with:
  - `useQuery` polling `getSweepStatus` at 2s interval via `refetchInterval`; auto-stops (`return false`) when no pending/running jobs remain.
  - `hasActiveSweep` / `sweepCompleted` derived state for display logic.
  - Progress bar with native `<progress>` element + "Processing sweep... N/M pages" during active sweep.
  - Completion state: checkmark + "N pages applied (M failed)" or error indicator when all jobs failed.
  - Non-blocking: polling re-activates on re-mount if jobs remain (status query key is independent of queue key, no cross-contamination).
- CSS: `.tagreview-sweep-progress`, `.tagreview-sweep-bar`, `.tagreview-sweep-text`, `.tagreview-sweep-check`, `.tagreview-sweep-check-failed`.
- **Files:** `web/src/api/client.ts`, `web/src/components/TagReviewView.tsx`, `web/src/components/TagReviewView.css`

### Code Review Dispositions

All findings from `anvil-code-reviewer` accepted and resolved:

| Finding | Severity | Disposition |
|---|---|---|
| `PendingApprovalCount` error silently swallowed | MAJOR | Fixed: error is now logged + returns 500 |
| Completion message omits failure count | MINOR | Fixed: shows "(N failed)" when `failed > 0` |
| Sweep completion doesn't handle all-failed case | MINOR | Fixed: distinct error indicator when all jobs failed |

### Verification Gates

- `go build ./...` — OK
- `go test ./internal/tagsweep/...` — OK (20 tests)
- `go test ./internal/server/... -run "Tag..."` — OK (20 tests, including 3 new status endpoint tests)
- `npx tsc --noEmit` (from `web/`) — OK
- `npx vitest run src/components/TagReviewView.test.tsx` — OK (8 tests)

### Hoare Triple Walkthrough

- **Precondition P:** User is admin on TagReviewView; sweep jobs may or may not be in-flight; jobs table contains tag_suggest rows with status ∈ {pending, running, done, failed}.
- **System S:** Backend `SweepStatus` queries jobs table grouped by status (filtered to kind=tag_suggest) + tag_suggestions pending count. Handler wraps as JSON. Frontend polls every 2s and renders progress bar.
- **Postcondition Q:** UI shows "Processing sweep... N/M pages" while any tag_suggest jobs are pending/running; when all jobs are terminal, shows "N pages applied (M failed)" with appropriate icon. No server resources are leaked. On error, both handler and UI surface the generic "unavailable" message.

### Key Design Decisions

1. **Status constants duplicated** in `tagsweep` package (not imported from `internal/jobs`) to avoid circular dependency. Values documented to match.
2. **`refetchInterval` returns `false`** when no active jobs, not just `undefined` — ensures polling cleanly stops without needing `enabled` gymnastics.
3. **Progress/queue query keys are independent** (`SWEEP_STATUS_KEY` vs `QUEUE_KEY`) so polling the status endpoint never races with or invalidates the review queue cache.
