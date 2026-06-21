---
phase: 04-eino-agent
plan: 05
subsystem: agent
tags: [eino, propose-patch, apply-gate, safety-core, stale-revision, 409, go-udiff, churn, byte-stable, agnt-09, agnt-10, agnt-11, d4, d8]

# Dependency graph
requires:
  - phase: 04-04
    provides: "validateProposedBody + proposeWithRetry (the body-output contract) + generateOnce choke point + the cm model.ToolCallingChatModel interface seam (key-free fakes)"
  - phase: 04-03
    provides: "agent.Service + Deps{Pages,Search,Attachments,Audit,PageWriter} + the read-only 5-tool allow-list and its TestToolSet build gate"
  - phase: 03 (pages/okf/gitstore)
    provides: "pages.Service.Save(BaseRevision) → ErrStaleRevision 409 floor → single-writer gitstore.Commit; pages.Revision; okf.Parse/Emit byte-stable round-trip"
provides:
  - "internal/agent.Service.ProposePatch(ctx, path, instruction) (newBody, baseRev, err) — the ONLY whole-body proposal path: fetch current body, capture pages.Revision AT proposal time, single-shot Generate + validateProposedBody/retry; NEVER writes"
  - "internal/agent.ChurnRatio / ValidateProposedBody — exported wrappers for the HTTP layer (audit Detail churn metric + apply-time defense-in-depth re-validation)"
  - "internal/agent churnRatio (go-udiff udiff.Lines server-side, metric only — never apply/render)"
  - "pageReader gains Revision(ctx,path) — the optimistic-concurrency token ProposePatch captures"
  - "POST /agent/propose-patch (editor) → {old_body, new_body, base_revision} for a real client-rendered diff (never a prose summary)"
  - "POST /agent/apply-patch (editor + CSRF) → re-validate → pages.Save(baseRevision) → single-writer commit (Action=approved_agent_patch, Source=agent); ErrStaleRevision → 409, no write"
  - "ActionAgentPatchProposal + ActionAgentPatchApproval audit rows (non-fatal)"
affects: [04-06, eino-agent, diff-review-dialog, apply-gate]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "full-new-body proposal (RESEARCH §Item 7): the agent returns the COMPLETE revised body; the SERVER diffs old↔new and the browser renders from the two strings — no fragile hunk application"
    - "go-udiff (udiff.Lines) used server-side ONLY for the churn metric / D4 threshold / audit Detail — never to apply or render a patch"
    - "the apply write boundary is a NON-tool editor-gated HTTP endpoint reusing pages.Save → the single-writer commit; the model can never reach a file write (AGNT-11)"
    - "base revision captured at PROPOSAL time → re-checked by pages.Save at APPLY time → a moved revision 409s (never a silent overwrite of a concurrent edit)"
    - "exported thin wrappers (ChurnRatio/ValidateProposedBody) keep the HTTP layer out of agent internals"

key-files:
  created:
    - internal/agent/propose_test.go
    - internal/agent/apply_test.go
  modified:
    - internal/agent/propose.go
    - internal/agent/agent.go
    - internal/agent/smoke_test.go
    - internal/server/handlers_agent.go
    - internal/server/router.go

key-decisions:
  - "ProposePatch + churn helper landed in propose.go (the body-output contract's home) rather than a new file — net scope identical to the plan's files_modified (propose.go, agent.go)."
  - "pageReader gained Revision(ctx,path) so ProposePatch captures the optimistic-concurrency token through the SAME narrow reader interface the read tools use (no new dependency, fakeable key-free)."
  - "The D8 stale-409 test reproduces the pages.Service.Save 409 floor in a key-free stub (revision advances N→N+1 between propose and apply) AND binds the stub to the real pageReader/pageWriter interfaces with compile-time assertions, so an interface drift breaks the test instead of passing against a stale shape."
  - "apply re-validates the body with ValidateProposedBody BEFORE pages.Save (defense in depth) — a client that tampered the approved body still cannot push a fenced/empty/frontmatter-mangled body to the write path."
  - "churnRatio counts removed+added lines over total old+new lines via udiff.Lines; a one-line edit lands well under a 0.25 threshold, a whole-body reformat well over — the D4 over-eager-reformat guard."

patterns-established:
  - "Propose→approve→apply: ProposePatch (read, captures baseRev) → client diff → /apply-patch (write, re-checks baseRev → 409 on stale). Slice 6 wires the DiffReviewDialog to these two endpoints verbatim."
  - "Every Source=agent commit reconciles to an ActionAgentPatchApproval row (audit provenance)."

requirements-completed: [AGNT-09, AGNT-10, AGNT-11]

# Metrics
duration: 5min
completed: 2026-06-21
status: complete
---

# Phase 4 Plan 05: Propose-patch + apply gate (the safety core) Summary

**`ProposePatch` returns the FULL proposed new body plus the page revision captured at proposal time; `/propose-patch` (editor) hands the frontend `{old_body, new_body, base_revision}` for a real diff; `/apply-patch` (editor + CSRF) is a SEPARATE non-tool endpoint that re-validates the body then reuses `pages.Save(baseRevision)` → the single-writer `gitstore.Commit(approved_agent_patch)`, blocking a moved revision with a 409 — proven by a key-free D4 round-trip/churn test and a D8 stale-revision-409 test, with the read-only 5-tool allow-list (TestToolSet) unchanged.**

## Performance

- **Duration:** ~5 min
- **Started:** 2026-06-21T20:22:51Z
- **Completed:** 2026-06-21T20:28:11Z
- **Tasks:** 3 (1 TDD)
- **Files modified:** 7 (2 created, 5 modified)

## Accomplishments
- `Service.ProposePatch(ctx, path, instruction) (newBody, baseRev, err)` — the single whole-body proposal path: 60s ctx, fetch current body via the role-scoped `pages.Get`, capture `pages.Revision` AT proposal time (the stale-during-review token), single-shot `Generate` with the propose system prompt (return ONLY the complete revised body, no prose, no fences, change only what is asked) and the current body delimited as untrusted DATA, then `validateProposedBody` + the slice-4 retry harness. NEVER writes.
- `churnRatio` (and exported `ChurnRatio`) via `go-udiff` (`udiff.Lines`) server-side ONLY — the changed-line fraction for the audit Detail and the D4 over-eager-reformat threshold; never to apply or render a patch (the browser diffs the two strings; apply ships the whole new body).
- `pageReader` gained `Revision(ctx, path)` so the proposal captures the optimistic-concurrency token through the same narrow reader interface the read tools use.
- `POST /agent/propose-patch` (editor) → `{page_path, old_body, new_body, base_revision}` JSON so the frontend renders a REAL diff (never a prose "I updated it" summary); audits `ActionAgentPatchProposal` with the churn metric in Detail (non-fatal).
- `POST /agent/apply-patch` (editor + CSRF) → re-validates the body (defense in depth), then `pages.Save(r.Context(), path, newBody, frontmatter, baseRevision, actor)`; `errors.Is(err, pages.ErrStaleRevision)` → **409** "page changed, re-run" with **no write**; on success audits `ActionAgentPatchApproval` non-fatally. `pages.Save` → `gitstore.Commit(Action="approved_agent_patch", Source="agent")` is the ONLY write path.
- D4 (`TestProposePatchDiff`): byte-stable `okf.Parse→Emit` round-trip on a frontmatter+table+code-fence fixture, frontmatter key-set/order preservation (reorder AND drop rejected), and diff-locality (one-line edit churn < 0.25; whole-body reformat > 0.25 and strictly greater than the local edit).
- D8 (`TestApplyStaleRevision`): propose@`rev-N` → page moves to `rev-N+1` → apply (Save with the stale baseRev) returns `pages.ErrStaleRevision` and writes nothing (concurrent edit NOT overwritten); a control proves a fresh apply still writes.

## Task Commits

Each task was committed atomically:

1. **Task 1: ProposePatch + go-udiff churn metric** — `a546a6b` (feat)
2. **Task 2: D4 round-trip/churn + D8 stale-409 tests (key-free)** — `92f835e` (test, TDD)
3. **Task 3: /propose-patch + /apply-patch endpoints (editor + CSRF) + stale-409** — `8bd1212` (feat)

## Files Created/Modified
- `internal/agent/propose.go` (modified) — `ProposePatch`, `proposePatchSystemPrompt` + `buildProposePatchMessages`, per-mode timeout/token/temperature caps, `currentSource`, `churnRatio`/`countLines`, exported `ChurnRatio`/`ValidateProposedBody` wrappers.
- `internal/agent/agent.go` (modified) — `pageReader` gained `Revision(ctx, path)`.
- `internal/agent/smoke_test.go` (modified) — `fakePageReader` gained `frontmatter`/`revision` fields and a `Revision` method (the new interface method).
- `internal/agent/propose_test.go` (new) — D4 `TestProposePatchDiff` (round-trip, frontmatter preservation, churn locality), key-free.
- `internal/agent/apply_test.go` (new) — D8 `TestApplyStaleRevision` with a `stalePagesStub` reproducing the `pages.Save` 409 floor; compile-time `pageReader`/`pageWriter` bindings.
- `internal/server/handlers_agent.go` (modified) — `handleProposePatch`, `handleApplyPatch` (stale-409 branch + both audit events), `assembleSource` helper.
- `internal/server/router.go` (modified) — mounted `/agent/propose-patch` + `/agent/apply-patch` in the editor-gated subgroup (global nosurf CSRF).

## Decisions Made
- **ProposePatch + churn in propose.go.** The plan's `files_modified` lists `propose.go` + `agent.go`; the proposal method and churn helper landed in `propose.go` (the body-output contract's home), with only the `pageReader` interface change in `agent.go`. Net scope identical.
- **Revision through the narrow pageReader.** Rather than inject a new collaborator, `ProposePatch` captures the base revision via `pageReader.Revision` — the same interface the read tools already use — so it stays fakeable key-free.
- **Apply re-validates before Save.** The body the client sends to `/apply-patch` is re-run through `ValidateProposedBody` so a tampered (fenced/empty/frontmatter-mangled) body never reaches the write path, even though `ProposePatch` already validated it at propose time.
- **D8 reproduces the real 409 floor + binds to the real interfaces.** The stub advances the revision between propose and apply and 409s exactly as `pages.Service.Save` does; compile-time `var _ pageReader/pageWriter` assertions ensure the stub can't drift from the production contract.

## Deviations from Plan

### Plan-structure adjustment (not a behavior deviation)

**1. ProposePatch + churn helper landed in `propose.go` (not a new file).** The plan's Task-1 `<files>` names `propose.go` + `agent.go`; both were modified as specified (no new agent file). Identical net scope.

### Auto-added (Rule 2 — correctness/robustness)

**2. [Rule 2] `pageReader.Revision` added to the narrow interface**
- **Found during:** Task 1.
- **Issue:** `ProposePatch` must capture `pages.Revision` at proposal time, but the existing `pageReader` interface exposed only `Get`/`Tree` — the revision token was unreachable through the injected reader.
- **Fix:** added `Revision(ctx, path)` to `pageReader` (satisfied by `*pages.Service`); updated the key-free `fakePageReader` to implement it.
- **Files modified:** `internal/agent/agent.go`, `internal/agent/smoke_test.go`.
- **Committed in:** `a546a6b`.

**3. [Rule 2] apply-time body re-validation (defense in depth)**
- **Found during:** Task 3.
- **Issue:** the body POSTed to `/apply-patch` is client-supplied; a tampered body could bypass the propose-time validation and reach `pages.Save`.
- **Fix:** `handleApplyPatch` re-runs `ValidateProposedBody` before `pages.Save`; a non-clean body returns 422 and never writes.
- **Files modified:** `internal/server/handlers_agent.go`, `internal/agent/propose.go` (exported wrapper).
- **Committed in:** `8bd1212`.

---

**Total deviations:** 2 auto-fixed (both Rule 2 — correctness/robustness) + 1 structure note.
**Impact on plan:** Both auto-fixes harden the safety-core write boundary the threat register demands (T-04-16/17/18). No scope creep.

## Threat Surface

No new surface beyond the plan's `<threat_model>`. Dispositions implemented:
- **T-04-16** (indirect prompt injection → coerced write) — STRUCTURAL: apply is a non-tool editor-gated HTTP endpoint; the read-only 5-tool allow-list and `TestToolSet` are unchanged (verified: 4 `apply/write/commit` matches in `tools.go` are all security-boundary COMMENTS, no tool). The model cannot reach a file write.
- **T-04-17** (stale-revision overwrite) — `pages.Save(baseRevision)` → `ErrStaleRevision` → 409 "page changed, re-run"; the D8 `TestApplyStaleRevision` proves the block + no write + un-overwritten concurrent edit.
- **T-04-18** (byte-stable round-trip corruption) — the full new body is preserved through `okf.Parse→Emit`; `validateProposedBody` rejects fenced/mangled bodies at BOTH propose and apply; D4 `TestProposePatchDiff` proves round-trip + frontmatter preservation + low churn.
- **T-04-19** (unapproved commit / Git push) — apply reuses `gitstore.Commit(Action="approved_agent_patch", Source="agent")` ONLY after explicit approval; no push tool, no bespoke write path.
- **T-04-20** (who approved which change) — `ActionAgentPatchProposal` + `ActionAgentPatchApproval` audited non-fatally; every Source=agent commit reconciles to an approval row.

## Live propose exercised? NO — key-free structural coverage complete

`DEEPSEEK_API_KEY` IS present, and the pre-existing slice-2/3 live smoke tests ran (the agent suite took ~8.1s). This slice added **no live propose-patch smoke test** — the plan required only the deterministic key-free D4/D8 structural tests. So `ProposePatch` was NOT exercised against the real provider; the proposal/validate/retry path, the base-revision capture, the churn metric, and the stale-409 apply branch are all proven key-free. A live propose→approve→apply is left to the slice-6 DiffReviewDialog wiring / manual VALIDATION.md.

## Issues Encountered
None. Every `<verify>` gate passed (one interface-method addition for `ProposePatch.Revision` surfaced a fake-update, fixed immediately). Full agent + server suites green; `CGO_ENABLED=0 go build ./...` + `go vet ./...` clean.

## User Setup Required
None — the agent provider is already configured from slice 1; this slice reuses it and the existing pages/gitstore write path.

## Next Phase Readiness
- **Slice 6 (DiffReviewDialog wiring)** consumes `/agent/propose-patch` → `{old_body, new_body, base_revision}` directly: render the diff from the two strings, send the approved `{page_path, new_body, frontmatter, base_revision}` to `/agent/apply-patch`, and surface the 409 "page changed, re-run" copy on a stale conflict.
- No blockers.

## Self-Check: PASSED

- Files exist: `internal/agent/propose_test.go`, `apply_test.go` (created); `internal/agent/propose.go`, `agent.go`, `smoke_test.go`, `internal/server/handlers_agent.go`, `router.go` (modified) — all present.
- Commits exist in git history: `a546a6b`, `92f835e`, `8bd1212`.
- Gates: `CGO_ENABLED=0 go build ./...` green; `go vet ./...` clean; `TestProposePatchDiff` (D4) + `TestApplyStaleRevision` (D8) + `TestToolSet` (exactly 5 read tools, apply NOT a tool) all green key-free; `grep ErrStaleRevision`/`ActionAgentPatchApproval` present in `handlers_agent.go`; `internal/agent/` + `internal/server/` suites pass.

---
*Phase: 04-eino-agent*
*Completed: 2026-06-21*
