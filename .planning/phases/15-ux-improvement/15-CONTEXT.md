---
author: architect
---

# Phase 15: UX Improvement - Context

**Gathered:** 2026-06-27
**Status:** Ready for planning

<domain>
## Phase Boundary

Improve the bulk-approve UX for tag suggestions. Currently, after bulk-approve, the user sees a success response but the async commit may not have landed yet. Option B chosen: non-blocking progress indicator — show "queued" state with live progress, user can keep working.

**In scope:** Backend job progress endpoint (counts: queued/processing/done/failed), admin UI progress display, non-blocking UX.

**Out of scope:** Dedicated admin job queue panel (Option C), real-time WebSocket updates, job cancellation.
</domain>

<decisions>
## Implementation Decisions

- **D-01:** Option B — Non-blocking progress indicator. After bulk-approve, UI shows "Applying... N/M pages" with live poll. User can navigate away.
- **D-02:** Backend adds `GET /api/v1/admin/tags/sweep/status` returning `{queued, processing, done, failed}` counts from the job worker.
- **D-03:** Admin UI polls status endpoint every 2s while jobs are pending, shows progress bar + count.
</decisions>

<canonical_refs>
### Key source files
- `internal/server/handlers_tagsweep.go` — bulk-approve endpoint, sweep start
- `internal/tagsweep/` — job store, worker, status
- `web/src/routes/Admin.tsx` — admin UI (tag review queue)
- `web/src/api/client.ts` — API client types and functions

### Requirements
- `.planning/REQUIREMENTS.md` §UX-01
</canonical_refs>
