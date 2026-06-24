# Phase 12: Bulk Sweep & Batch Review Queue - Context

**Gathered:** 2026-06-24
**Status:** Ready for planning
**Mode:** Smart discuss (autonomous) — scope/roles/queue-location resolved by the user; mechanics pinned from v1.0 ARCHITECTURE/PITFALLS research. FINAL phase of milestone v1.0.

<domain>
## Phase Boundary

Deliver the bulk tagging sweep + batch review queue (TAG-05, TAG-06): an admin starts a sweep that enqueues per-page suggestion jobs on the EXISTING async worker, staging results as a pending review queue (writing NOTHING automatically); an admin reviews the queue per page and approves through the SAME byte-stable apply path from Phase 11, committed in BATCHES. Resumable; never bypasses human approval.

**In scope:** the `KindTagSuggest` job (per-page, on the existing single-drain worker); a `tag_suggestions` staging table (pending); a sweep-start endpoint; a review-queue read endpoint; the dedicated review route UI; batched approval/commit; resumability.

**Out of scope:** any change to the Phase-11 per-page suggest/apply primitives (REUSE them); auto-apply (explicitly forbidden); embedding/synonym merge (v2). This is the LAST phase — keep it additive over Phases 8–11.
</domain>

<decisions>
## Implementation Decisions

### Sweep scope — USER DECISION: untagged by default, toggle for all
- Default target: pages with NO tags yet (the backfill case — cheaper, most common). Provide a toggle/param to sweep ALL pages.
- Identify untagged pages from the Phase-8 `page_tags` table (a page absent from page_tags = untagged) intersected with the live page set.

### Roles — USER DECISION: admin starts AND reviews
- Starting a bulk sweep is **admin-only** (workspace-wide LLM operation). Reviewing/approving the queue is **also admin-only** (tightest control). Gate both via the existing `RequireRole(admin)` subgroup; role read from session only.
- Approval still flows through the Phase-11 apply path (editor+CSRF endpoint) — admins are a superset of editors, so this composes.

### Review queue location — USER DECISION: dedicated review route
- A dedicated route (e.g. `/app/tag-review`) listing pages with pending suggestions; reviewed ONE PAGE AT A TIME via the existing Phase-11 per-tag approval list (checkbox rows, new-vs-existing, new default unchecked). Reachable from a nav entry (admin-visible).

### Backend mechanics (from ARCHITECTURE/PITFALLS research — high confidence)
- **`KindTagSuggest` job** on the EXISTING single-drain `internal/jobs` worker (serial drain = natural LLM rate-limiting + the worker's built-in retry/backoff). The sweep-start endpoint enqueues one job per target page and returns immediately — it WRITES NOTHING.
- Each job calls the Phase-11 `SuggestTags` mode and stages the result into a **`tag_suggestions`** table with status `pending` (page_path, suggested tags + existing-vs-new flags, base_revision captured at suggestion time, created_at). A new migration consistent with existing numbered migrations.
- **Resumability / safety (Pitfall 5 — go/no-go):** the sweep ONLY produces pending rows; a write happens ONLY via a human-approved apply. Killing + restarting the worker re-runs pending jobs but NEVER auto-writes tags. Prove with a test: drain jobs, assert NO frontmatter/commit happened without an explicit approve.
- **Batched commits (Pitfall 6):** approving a batch routes through the Phase-11 byte-stable apply (okf.SetTags → pages.Save → single-writer commit) but commits in BATCHES, NOT one commit per page. Respect the single-writer path; a per-page stale base_revision 409s that page without failing the batch. Re-validate/normalize server-side on apply (never trust client).

### Claude's Discretion
- Exact table schema, endpoint paths, batch size/granularity, the review-route layout, nav placement, and how a stale base_revision in a batch is surfaced — at Claude's discretion within the above contracts.
</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- Phase-11: `agent.SuggestTags` (single-shot mode, robust real-model parse), `agent.ValidateTags`, `okf.SetTags`, the apply-tags endpoint (editor+CSRF → pages.Save → single-writer commit → 409), the TagSuggest per-tag approval UI — REUSE all; this phase fans out + queues, it does not re-implement suggestion or apply.
- Phase-8 `internal/graph` `page_tags` + `Store.Vocabulary` — to find untagged pages + bias prompts.
- `internal/jobs` single-drain worker (queue.go/worker.go — retry/backoff, fire-and-forget) — host `KindTagSuggest`; mirror the `KindGraph`/`KindIndex` registration + the existing migration pattern (latest migration ~0009/0010).
- The existing "Rebuild graph index" admin endpoint/button + Admin RBAC subgroup — the pattern for the admin sweep-start control.
- The Phase-10 lazy-route + nav-entry pattern (GraphView) — the precedent for the `/app/tag-review` route + nav entry.
- DiffReviewDialog / TagSuggest approval patterns + tokens.css for the review UI.

### Established Patterns
- New job kind on the single worker; staging/operational data in SQLite (NOT content); numbered migrations; mutations via the single-writer commit path; admin endpoints in the RequireRole(admin) subgroup; key-free agent tests (fake model); React lazy routes + react-query + token-only CSS + no dangerouslySetInnerHTML.

### Integration Points
- `internal/jobs` or a new staging package: `KindTagSuggest` job + `tag_suggestions` table + migration; enqueue from the sweep-start endpoint.
- `internal/server`: admin sweep-start endpoint, admin review-queue read endpoint, batched approve (reusing the Phase-11 apply); routes + nav.
- `web/src/`: the `/app/tag-review` route + nav entry + the queue list + per-page review (reuse TagSuggest) + batched approve; api fns + react-query.
</code_context>

<specifics>
## Specific Ideas

- The go/no-go safety invariant (Pitfall 5): the sweep stages pending suggestions ONLY; NO tag is ever written without an explicit human approve, even across worker kill/restart. This needs an explicit test and is the phase's load-bearing guarantee.
- Batched commits (Pitfall 6): approving routes through the proven Phase-11 byte-stable apply but batches commits; a stale page 409s individually without sinking the batch.
- Reuse Phase-11 primitives wholesale — this phase is fan-out (KindTagSuggest) + a staging table + a review UI, not new suggestion/apply logic.
- Serial drain of the existing worker is the natural LLM rate-limiter — do not add a parallel LLM caller.

## Milestone close
This is the FINAL phase of v1.0. After it verifies, the milestone lifecycle (audit → complete → cleanup) runs.
</specifics>

<deferred>
## Deferred Ideas

- Embedding-based near-synonym tag merge (TAG-F1) — v2.
- Scheduled/automatic periodic sweeps — not in scope (admin-triggered only).
- Auto-apply without review — explicitly OUT (locked safety model).
</deferred>
