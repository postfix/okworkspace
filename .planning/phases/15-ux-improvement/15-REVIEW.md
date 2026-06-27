---
author: anvil-code-reviewer
responsible: engineer
phase: "15"
status: draft
artifact_kind: REVIEW
provenance:
  spawned_by: engineer
  spawn_role: anvil-code-reviewer
  spawn_via: anvil-spawn/claude-cli
  model: opus
  tool_policy: "allowed=[Read Grep Glob Bash mcp__smtc] disallowed=[Write Edit mcp__smtc__rename_symbol mcp__smtc__replace_symbol_body mcp__smtc__insert_before_symbol mcp__smtc__insert_after_symbol mcp__smtc__safe_delete_symbol mcp__smtc__apply_virtual_edit mcp__smtc__create_file mcp__smtc__replace_in_file mcp__smtc__fuzzy_edit mcp__smtc__format_code] mode=default"
  task_hash: 4dc5fd20
  output_sha256: ba4e884cd100
  duration_ms: 145159
  cost_usd: 1.6448360000000002
  is_error: false
  session_id: ac7ffb55-870c-4414-a7ed-7e9adb4709e0
  spawn_depth: 1
  spawned_at: "2026-06-27T17:46:31Z"
---

All 20+ tests pass cleanly. Now I have everything I need. Here is the review:

---

```yaml
---
author: anvil-code-reviewer
responsible: engineer
emitted_at: 2026-06-27T17:46:00Z
phase: "15-01"
artifact_kind: review
parent_artifacts:
  - .planning/phases/15-ux-improvement/15-01-PLAN.md
status: draft
---
```

# Code review — Phase 15-01 (bulk-approve non-blocking progress)

**Verdict:** SHIP — 1 major, 2 minors
Bottom line: a swallowed error from `PendingApprovalCount` silently reports `pending_approval: 0` when the query fails; otherwise clean.

## Findings

### [MAJOR] `PendingApprovalCount` error silently swallowed in status handler · lens: Error & failure handling · `internal/server/handlers_tagsweep.go:279`

- **violated property:** error-handling discipline — every error path in the handler is logged + surfaced; this one is silently discarded.
- **defect:** `pending, _ := h.tagSuggestions.PendingApprovalCount(r.Context())` discards the error. If `PendingApprovalCount` fails (e.g., the `tag_suggestions` table is missing while the `jobs` table exists), the handler returns HTTP 200 with `pending_approval: 0` — silently wrong data.
- **why it bites:** the UI would show "0 approvals pending" when there are genuinely pages waiting for review. The admin has no signal that the data is incomplete.
- **evidence:** pattern-suspected — the `_` discard is visible at `handlers_tagsweep.go:279`. The blast radius is low in practice (both queries hit the same in-process SQLite DB; if `SweepStatus` succeeded at line 273, `PendingApprovalCount` is vanishingly unlikely to fail independently), but the code violates its own error-handling contract.
- **fix:** log and 500 like `SweepStatus` does:
  ```go
  pending, err := h.tagSuggestions.PendingApprovalCount(r.Context())
  if err != nil {
      slog.Error("tag sweep pending count failed", slog.String("error", err.Error()))
      writeError(w, http.StatusInternalServerError, tagSweepUnavailable)
      return
  }
  ```

### [MINOR] Completion message omits failure count · lens: Correctness (UX completeness) · `web/src/components/TagReviewView.tsx:199-200`

- **violated property:** the completion display should accurately reflect the sweep outcome — a partially-failed sweep looks indistinguishable from a fully-successful one.
- **defect:** the checkmark line reads `Applied {sweepStatus!.progress.done} pages` — `done` only counts successful jobs, not failed ones. If 5 pages were queued and 2 failed, the user sees "Applied 3 pages" with no hint that 2 failures occurred.
- **why it bites:** the admin may believe all pages were processed when some silently failed. The `failed` count is in the data but never surfaced in the completion display.
- **fix:** include the failure count when `failed > 0`, e.g.:
  ```tsx
  {sweepStatus!.progress.done} pages applied
  {sweepStatus!.progress.failed > 0 && ` (${sweepStatus!.progress.failed} failed)`}
  ```

### [MINOR] Sweep completion state condition doesn't survive page with 0 `done` + positive `failed` · lens: Correctness (totality) · `web/src/components/TagReviewView.tsx:70-71`

- **violated property:** `sweepCompleted` should be true whenever a sweep ran to completion (all terminal), regardless of success/failure mix.
- **defect:** `sweepCompleted` requires `sweepStatus.progress.total > 0`. This correctly excludes the "no sweep ever ran" state, but `total > 0` with `done === 0` and `failed > 0` (every job failed) would show the checkmark reading "Applied 0 pages" — a confusing signal. This is a degenerate case (all LLM calls failing), not a likely production state.
- **fix:** guard the checkmark on `done > 0` and show a distinct error-style banner when `failed === total && total > 0`:
  ```tsx
  {done > 0 && <Check ... />}
  <span>{done} applied{failed > 0 ? `, ${failed} failed` : ''}</span>
  ```

## Checked clean

- **Correctness:** SQL query shape (`GROUP BY status WHERE kind='tag_suggest'`), `PendingApprovalCount` query, status constant values (match `internal/jobs`), JSON shape back↔front end — all verified.
- **Numeric:** no arithmetic beyond bounded row counts; no overflow risk.
- **Resource & lifecycle:** `rows.Close()` properly deferred, no goroutine/file/connection leaks, context propagated to all DB calls.
- **Concurrency:** store methods are read-only queries over `*sql.DB` (safe for concurrent use); frontend polling uses independent query keys (`SWEEP_STATUS_KEY` ≠ `QUEUE_KEY`) — no race between polling invalidation and queue invalidation; `refetchInterval` correctly returns `false` when no active jobs.
- **Contract & invariants:** route is inside `admin.Group()` with `RequireRole(admin)` (router.go:298); handler nil-store guard matches existing pattern; error messages use generic `tagSweepUnavailable` (no internal job/queue/worker vocabulary leaked).
- **Robustness:** GET endpoint (no request body validation needed); queries use parameterized SQL or hardcoded literals (no SQLi); result sizes bounded by distinct status values (max 4) and one integer.
- **Idiom:** duplicated status constants documented with rationale (avoid circular import with `internal/jobs`); `defer rows.Close()` pattern consistent with `ListPending`.
- **TypeScript:** `SweepProgress` and `SweepStatusPayload` interfaces match backend JSON shape exactly (`queued`/`processing`/`done`/`failed`/`total` nested under `progress`, `pending_approval` at top level).

## Records

```yaml
handoff:
  record_author: engineer
  accountable_owner: engineer
  id: H-SP-4dc5fd20
  from: engineer
  to: anvil-code-reviewer
  artifact: /tmp/anvil-spawn-out/15-REVIEW.md
  intent: "spawned anvil-code-reviewer worker (task 4dc5fd20)"
  status: sent
```

```yaml
evidence:
  record_author: engineer
  evidence_producer: "anvil-spawn:anvil-code-reviewer:claude-cli"
  commissioned_by: engineer
  accountable_owner: engineer
  id: E-SP-4dc5fd20
  subject_ref: "/tmp/anvil-spawn-out/15-REVIEW.md"
  kind: external_ref
  issuance:
    kind: producer_signed
  warrant:
    kind: external_cited
    scope:
      - "/tmp/anvil-spawn-out/15-REVIEW.md"
    rationale: "produced by spawned anvil-code-reviewer bounded to [Read Grep Glob Bash mcp__smtc]; output hash + invocation in envelope.provenance"
  freshness:
    status: fresh
    checked_against: HEAD
```
