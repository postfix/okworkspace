---
phase: 04-eino-agent
plan: 04
subsystem: agent
tags: [eino, single-shot, generate, summarize, rewrite, draft, validate-and-retry, validateProposedBody, byte-stable, agnt-05, agnt-06, agnt-07, agnt-08, d4]

# Dependency graph
requires:
  - phase: 04-03
    provides: "agent.Service + per-scope ReAct Ask + the read-only 5-tool allow-list; Deps{Pages,Search,Attachments,Audit}; prompts.go assembly helpers (delimitUntrusted)"
  - phase: 04-01
    provides: "newChatModel (openai.NewChatModel from cfg), 60s/MaxTokens guard intent, ErrAgentDisabled fail-closed"
  - phase: 03 (okf)
    provides: "okf.Parse/Emit byte-stable round-trip — used by validateProposedBody to compare frontmatter keys"
provides:
  - "internal/agent.validateProposedBody(source, body) — rejects empty / whole-body ```-fenced / frontmatter key-set-or-order-changed bodies (okf.Parse key compare, never a raw-byte regex)"
  - "internal/agent.proposeWithRetry(ctx, source, gen) — <=2 retries (3 attempts), logs attempt+err (never the key), structured ErrProposalInvalid on exhaustion; NEVER returns a malformed body"
  - "internal/agent.ErrProposalInvalid + ErrNoExtractedText sentinels"
  - "internal/agent.Service single-shot modes: SummarizePage, SummarizeAttachment (read-only answers), Rewrite (proposal), Draft (editor body) — all via ChatModel.Generate, NOT the ReAct loop"
  - "internal/agent.generateOnce — the single choke point wrapping every single-shot call in context.WithTimeout(60s) + per-mode MaxTokens/Temperature"
  - "internal/agent.Service.cm retyped to model.ToolCallingChatModel (interface) so a fake model can be injected key-free"
  - "POST /agent/summarize-page, /summarize-attachment, /agent/rewrite, /agent/draft — awaited JSON, any-authed, fail-closed, audited"
affects: [04-05, 04-06, eino-agent, propose-patch, diff-review, apply-gate]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "single-shot ChatModel.Generate for context-supplied modes (Summarize/Rewrite/Draft) — no ReAct round-trips; cheaper, fewer failure modes (AI-SPEC §4)"
    - "validate-and-retry over trusting the provider: every body-producing mode runs proposeWithRetry → validateProposedBody before return (a malformed body never escapes)"
    - "deps held as INTERFACES (cm = model.ToolCallingChatModel) so unit tests inject fakes with no provider/network"
    - "generateOnce as the single bounded choke point — one place guarantees timeout + MaxTokens on every single-shot call"
    - "frontmatter preservation checked structurally via okf.Parse key comparison, never a regex on raw bytes"

key-files:
  created:
    - internal/agent/propose.go
    - internal/agent/propose_validate_test.go
    - internal/agent/dispatch_test.go
  modified:
    - internal/agent/agent.go
    - internal/agent/prompts.go
    - internal/server/handlers_agent.go
    - internal/server/router.go

key-decisions:
  - "cm field retyped from concrete *openai.ChatModel to the eino model.ToolCallingChatModel interface — openai.NewChatModel already satisfies it, so NO production wiring changed, but a fakeChatModel can now be injected to test the dispatch key-free."
  - "Summarize is read-only (returns an answer, not a candidate body) so it does NOT go through validateProposedBody; only Rewrite/Draft (which produce candidate bodies the diff/editor consumes) run the validate+retry harness."
  - "validateProposedBody compares frontmatter key-set AND order via okf.Parse (not a raw-byte regex), so a reorder or drop is caught structurally; when the source has no frontmatter only the empty/fenced rules apply."
  - "Empty attachment extraction short-circuits to ErrNoExtractedText BEFORE any model call (no token spend, no hallucinated summary) — handler maps it to 422."

patterns-established:
  - "Single-shot mode = (build messages in prompts.go) → generateOnce(timeout+caps) → [body modes only] proposeWithRetry/validateProposedBody. Propose-patch (slice 5) reuses this exact harness."
  - "Awaited JSON (not SSE) for whole-output modes: a body that must be validated/diffed cannot be half-streamed (AI-SPEC §4b)."

requirements-completed: [AGNT-05, AGNT-06, AGNT-07, AGNT-08]

# Metrics
duration: 7min
completed: 2026-06-21
status: complete
---

# Phase 4 Plan 04: Single-shot Summarize / Rewrite / Draft + validateProposedBody harness Summary

**The four single-shot modes (Summarize page/attachment, Rewrite selection, Draft new page) wired via direct `ChatModel.Generate` with a 60s timeout + per-mode token caps, plus the `validateProposedBody`+retry body-output contract that rejects fenced/empty/frontmatter-mangled bodies — all proven by a key-free `TestDispatch` with a fake model.**

## Performance

- **Duration:** ~7 min
- **Started:** 2026-06-21T20:11:04Z
- **Completed:** 2026-06-21T20:18:21Z
- **Tasks:** 4 (2 TDD)
- **Files modified:** 7 (3 created, 4 modified)

## Accomplishments
- `validateProposedBody` + `proposeWithRetry` — the body-output contract every body-producing mode depends on (Rewrite/Draft here, Propose-patch in slice 5). Rejects empty, whole-body ```-fenced, and frontmatter key-set/order-changed bodies; ≤2 retries then a structured `ErrProposalInvalid`; a malformed body is NEVER returned.
- Single-shot `SummarizePage` / `SummarizeAttachment` (read-only grounded answers, head+tail truncation over budget), `Rewrite` (proposal), `Draft` (editor body) — each via `ChatModel.Generate` (NOT the ReAct loop), wrapped in `context.WithTimeout(60s)` with explicit per-mode `MaxTokens`/`Temperature`.
- `cm` field retyped to the `model.ToolCallingChatModel` interface (production wiring unchanged) so the dispatch is unit-testable with an injected fake — no provider, no network.
- `/agent/summarize-page`, `/summarize-attachment`, `/agent/rewrite`, `/agent/draft` mounted in the any-authed group: awaited JSON, fail-closed on disabled/unreachable/validation-exhaustion, audited via `ActionAgentPrompt` with the non-secret mode in Detail.
- Key-free `TestDispatch`: per-mode path routing, Rewrite/Draft → validateProposedBody pass-through (a fenced fake body is rejected + retried 3×, never surfaced), and the ~60s deadline on the `Generate` ctx.

## Task Commits

Each task was committed atomically:

1. **Task 1: validateProposedBody + retry harness** - `703b686` (feat, TDD)
2. **Task 2: single-shot Summarize/Rewrite/Draft dispatch + prompts** - `30426ad` (feat)
3. **Task 3: summarize endpoints + rewrite/draft wiring** - `06888f3` (feat)
4. **Task 4: key-free TestDispatch (fake model)** - `fcc507e` (test, TDD)

_TDD note: Tasks 1 and 4 wrote test + implementation as cohesive units (the test file alone does not compile without its target); each was committed once with the test and the code it exercises._

## Files Created/Modified
- `internal/agent/propose.go` (new) - `validateProposedBody` (okf.Parse frontmatter-key compare; empty/fenced rules) + `proposeWithRetry` (3 attempts, structured `ErrProposalInvalid`); `bodyGenerator` seam.
- `internal/agent/propose_validate_test.go` (new) - key-free `TestValidateProposedBody` (empty/fenced/reordered/dropped/clean) + `TestProposeWithRetry*` (exhaust, succeed-on-second, provider-error).
- `internal/agent/dispatch_test.go` (new) - `fakeChatModel` (model.ToolCallingChatModel) + `TestDispatch` (per-mode path, validate pass-through, 60s deadline, empty-extraction short-circuit, disabled fail-closed) — runs with `DEEPSEEK_API_KEY` unset.
- `internal/agent/agent.go` - `cm` retyped to `model.ToolCallingChatModel`; `generateOnce` choke point; `SummarizePage`/`SummarizeAttachment`/`Rewrite`/`Draft`; `truncateForBudget`; `ErrNoExtractedText`.
- `internal/agent/prompts.go` - summarize/rewrite/draft system prompts ("return ONLY the body, no fences") + `buildSummarizeMessages`/`buildRewriteMessages`/`buildDraftMessages` + `retryHint`; `ModeSummarize/Rewrite/Draft`.
- `internal/server/handlers_agent.go` - 4 awaited-JSON handlers + `auditAgentMode` + `writeAgentModeError` (fail-closed mapping).
- `internal/server/router.go` - mounted the 4 routes in the any-authed group.

## Decisions Made
- **cm → interface, no production change.** Retyping `cm` to `model.ToolCallingChatModel` was the single enabler for the key-free `TestDispatch`; `openai.NewChatModel` already returns a value satisfying it, so `NewService`/`buildReActAgent`/main wiring were untouched.
- **Summarize bypasses validateProposedBody.** Summaries are answers, not candidate bodies that will be written/diffed — only Rewrite/Draft (and slice-5 Propose-patch) feed the validate+retry harness.
- **Empty extraction short-circuits before the model.** `SummarizeAttachment` returns `ErrNoExtractedText` with zero model calls when extraction is pending/empty (no token spend, no hallucination) → 422.
- **Awaited JSON, not SSE,** for all four modes (a body that must be validated/diffed cannot be half-streamed — AI-SPEC §4b); only Ask/chat streams.

## Deviations from Plan

### Plan-structure adjustment (not a behavior deviation)

**1. Single-shot mode methods landed in `agent.go` (not a separate file)**
- The plan's `files_modified` lists `internal/agent/agent.go` for Task 2; the single-shot methods (`generateOnce`, `SummarizePage`, etc.) were added there as specified rather than split into a new `modes.go`. Net scope identical to the plan.

### Auto-added (Rule 2 — correctness/robustness)

**2. [Rule 2] `ErrNoExtractedText` + empty-extraction short-circuit**
- **Found during:** Task 2 (SummarizeAttachment).
- **Issue:** `ExtractedText` returns `("", nil)` when extraction is pending/absent (never an error). Summarizing empty text would burn a model call and risk a hallucinated summary of nothing.
- **Fix:** short-circuit to a new `ErrNoExtractedText` sentinel before any Generate; the handler maps it to a structured 422.
- **Files modified:** `internal/agent/agent.go`, `internal/server/handlers_agent.go`.
- **Verification:** `TestDispatch` asserts the model is called 0 times for an empty attachment.
- **Committed in:** `30426ad` / `06888f3`.

**3. [Rule 2] NUL + length input caps on the new endpoints**
- **Found during:** Task 3.
- **Issue:** the new rewrite/draft/summarize bodies are untrusted; an unbounded selection/instruction is a prompt/DoS vector and NUL bytes should never reach the prompt (mirrors the slice-3 selection hardening).
- **Fix:** reuse `maxSelectionLen`/`maxPromptLen` caps + NUL rejection across `page_path`/`attachment_id`/`selection`/`instruction`.
- **Files modified:** `internal/server/handlers_agent.go`.
- **Verification:** server suite green; manual reasoning per T-04-14.
- **Committed in:** `06888f3`.

---

**Total deviations:** 2 auto-fixed (both Rule 2 — correctness/robustness) + 1 structure note.
**Impact on plan:** Both auto-fixes are correctness/DoS guards directly on this slice's new surface (the threat register's T-04-14 mitigation). No scope creep.

## Threat Surface

No new surface beyond the plan's `<threat_model>`. Dispositions implemented:
- **T-04-12** (Rewrite/Draft body tampering) — `validateProposedBody` rejects fenced/empty/frontmatter-mangled bodies; `proposeWithRetry` retries ≤2 then returns `ErrProposalInvalid`. `TestDispatch` proves a fenced fake body is rejected + retried 3× and NEVER returned for both Rewrite and Draft.
- **T-04-13** (indirect injection via summarized content) — structural: no write tool; Draft → editor pending save, Rewrite → proposal (neither auto-writes); page/attachment/selection text is delimited as untrusted DATA in the USER turn (`delimitUntrusted`), never the system prompt.
- **T-04-14** (large-body / unbounded Summarize) — `truncateForBudget` (head+tail window) over `maxSingleShotInput`; every call wrapped in `context.WithTimeout(60s)` with explicit `MaxTokens` via `generateOnce`; selection/instruction length-capped at the handler.
- **T-04-15** (role-scoped content disclosure) — page/attachment fetched via the role-scoped `Deps.Pages`/`Deps.Attachments` (repo.Resolve-backed) services; config/env/session never enter the prompt by construction.

## Live single-shot mode exercised? NO (for the NEW modes) — key-free coverage complete

`DEEPSEEK_API_KEY` IS present in the environment, and the **pre-existing** slice-2/3 smoke tests (`TestSmokeChatModelGenerate`, `TestSmokeReActAskStream`, `TestSmokeWorkspaceAskCitesRetrievedPage`) ran LIVE as part of the agent suite (~8.5s). However, this slice deliberately added **no live smoke test for the new Summarize/Rewrite/Draft modes** — the plan required only the deterministic key-free `TestDispatch`. So no NEW single-shot mode was exercised against a real provider; the per-mode dispatch, the validate-and-retry pass-through, and the 60s timeout are proven entirely key-free with a fake model. A live Summarize/Rewrite/Draft is left to manual VALIDATION.md / slice-5 wiring.

## Issues Encountered
None. All four `<verify>` gates passed first attempt; the full key-free agent+server suite is green.

## User Setup Required
None - no external service configuration required (the agent provider is already configured from slice 1; these modes reuse it).

## Next Phase Readiness
- **Slice 5 (Propose-patch + apply gate)** can reuse `proposeWithRetry`/`validateProposedBody` verbatim — a `ProposePatch` single-shot method follows the exact `generateOnce` → validate+retry shape, and `validateProposedBody` already enforces frontmatter preservation against the current page body (the byte-stable round-trip the diff dialog needs).
- The `cm` interface seam means slice-5 propose logic is unit-testable key-free the same way `TestDispatch` is.
- No blockers.

## Self-Check: PASSED

- Files exist: `internal/agent/propose.go`, `propose_validate_test.go`, `dispatch_test.go` (created); `internal/agent/agent.go`, `prompts.go`, `internal/server/handlers_agent.go`, `router.go` (modified) — all present.
- Commits exist in git history: `703b686`, `30426ad`, `06888f3`, `fcc507e`.
- Gates: `CGO_ENABLED=0 go build ./...` green; `go vet ./...` clean; `TestDispatch` (key-free), `TestValidateProposedBody`, `TestToolSet` all green; `grep summarize internal/server/router.go` + `grep Generate internal/agent/agent.go` present.

---
*Phase: 04-eino-agent*
*Completed: 2026-06-21*
