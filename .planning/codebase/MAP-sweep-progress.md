---
author: codebase-archaeologist
responsible: codebase-archaeologist
phase: 15, 16
milestone: v1.1
status: verified
confidence: smtc_honest
---

# Codebase Map: Sweep Progress & Security Hardening

## Subsystem: Bulk Tag Sweep (Phases 12 baseline + 15/16 additions)

### Files — Backend

| File | Role |
|------|------|
| `internal/tagsweep/store.go` | Core store. v1.1 additions: `SweepProgress` struct (`{Queued, Processing, Done, Failed, Total}`), `SweepStatus()` method (queries jobs table grouped by status, filtered to `kind='tag_suggest'`), `PendingApprovalCount()` method (counts pending `tag_suggestions` rows). Nil-DB guards on all methods. |
| `internal/tagsweep/store_test.go` | 5 tests: `TestSweepStatusGroupsByStatus`, `TestSweepStatusNilDB`, `TestSweepStatusEmpty`, `TestPendingApprovalCount`, `TestPendingApprovalCountNilDB`. |
| `internal/server/handlers_tagsweep.go` | `handleTagSweepStatus` (line 283): returns `{progress: SweepProgress, pending_approval: int}` as JSON 200. `handleStartTagSweep` (line 49): updated to read `SweepConfig.MaxConcurrentJobs`, cap enqueue loop, return `{ok, queued, total}`. Charset validation added to `handleApproveTagSuggestions` (line 199). |
| `internal/server/handlers_tagsweep_test.go` | 3 new tests: `TestTagSweepStatusAdminOnly`, `TestTagSweepStatusReturnsProgress`, `TestTagSweepStatusEmpty`. |
| `internal/server/router.go` | Line 298: `admin.Get("/admin/tags/sweep/status", h.handleTagSweepStatus)`. |
| `internal/config/config.go` | `SweepConfig` struct with `MaxConcurrentJobs int` (yaml tag `max_concurrent_jobs`). `DefaultSweepMaxConcurrent = 10`. Default applied in `applyDefaults()`. |
| `internal/server/handlers_agent.go` | `handleApplyTags` (line 710): added charset validation after `agent.ValidateTags`. |
| `internal/okf/repair.go` | `ValidateTagCharset` (line 158): charset regex `^[a-zA-Z0-9_ -]+$`. `SetTags` calls it per-tag as last-resort gate. |

### Files — Frontend

| File | Role |
|------|------|
| `web/src/api/client.ts` | `SweepProgress` and `SweepStatusPayload` TypeScript interfaces (lines 1183-1195). `getSweepStatus()` function (line 1200) calling `GET /api/v1/admin/tags/sweep/status`. |
| `web/src/components/TagReviewView.tsx` | `useQuery` polls `getSweepStatus` at 2s `refetchInterval`. Auto-stops (`return false`) when no active jobs. Progress bar with `<progress>`. Completion state with checkmark + "N pages applied (M failed)". |
| `web/src/components/TagReviewView.css` | `.tagreview-sweep-progress`, `.tagreview-sweep-bar`, `.tagreview-sweep-text`, `.tagreview-sweep-check`, `.tagreview-sweep-check-failed`. |

### Config (config.example.yaml)

```yaml
# Tag sweep configuration (SEC-01).
sweep:
  # Max concurrent tag-suggest jobs per bulk sweep run; remaining pages are
  # left for a subsequent run (re-staging supersedes).
  max_concurrent_jobs: 10
```

### Defensive Flow for Tags

```
Client tags
  -> agent.ValidateTags (normalize/cap/dedupe)
  -> okf.ValidateTagCharset (charset gate — handlers_agent.go:753, handlers_tagsweep.go:199)
  -> pages.SetTagsFrontmatter
  -> okf.SetTags (redundant charset gate — repair.go:181)
  -> YAML structural write
```

### Key Invariants

1. Status constants duplicated in `tagsweep` package (not imported from `internal/jobs`) to avoid circular dependency. Values must remain in sync manually.
2. `PendingApprovalCount` error is logged and returns 500 (fixed from Phase 15 MAJOR finding).
3. Sweep enqueue cap is a hard stop, not a semaphore — remaining targets need a re-run.
4. Charset regex `^[a-zA-Z0-9_ -]+$` places the dash at end of class to avoid range interpretation.
5. Polling auto-stops via `refetchInterval` returning `false`, not `enabled: false`.
6. `SWEEP_STATUS_KEY` and `QUEUE_KEY` are independent query keys — polling never races with review queue cache.

### Blast Radius

- Changes to `SweepProgress` struct affect: `SweepStatus()`, `handleTagSweepStatus`, `client.ts` TS interfaces, `TagReviewView.tsx` rendering.
- Changes to `ValidateTagCharset` affect: `SetTags`, `handleApplyTags`, `handleApproveTagSuggestions`, all tests in `charset_test.go` and `settags_test.go`.
- Changes to `SweepConfig` affect: `config.go`, `handlers_tagsweep.go` loop cap, `config.example.yaml`.
