---
phase: 04-eino-agent
plan: 03
subsystem: agent
status: complete
tags: [eino, react, rag, scope, selection, attachment, workspace, citation, role-scoped, sse, agnt-02, agnt-03, agnt-04, d3, d7]
requires:
  - "internal/agent.Service.AskStream + buildReActAgent + the 5 read-only tools (04-02)"
  - "internal/agent.readTools single tool-construction site + D5 allow-list gate (04-02)"
  - "internal/search.Index.Query top-K (backs workspace RAG retrieval)"
  - "auth.CurrentUser(ctx).UserRole() — session-bound role (never the client)"
provides:
  - "internal/agent.Scope{Kind: page|selection|attachment|workspace} + normalize() — scope model over the SAME ReAct agent"
  - "internal/agent.scopeTrace — per-request, goroutine-safe, deduped citation collector fed by read_page/search_pages/search_attachments"
  - "internal/agent.Service.AskStream(ctx, w, question, Scope) ([]string, error) — scope-aware dispatch returning the retrieved page paths"
  - "internal/agent per-scope system prompts (selection/attachment/workspace) with grounded-or-refuse (D7)"
  - "SSE 'event: citation' frame (JSON array of retrieved page paths) on workspace answers — the 'Reasoned over:' contract (D3)"
  - "POST /agent/chat scope handling: scope/selection/attachment_id decoded; role bound from the session; audit Detail 'scope=<kind> role=<role>'"
affects:
  - internal/agent/agent.go
  - internal/agent/tools.go
  - internal/agent/prompts.go
  - internal/agent/stream.go
  - internal/server/handlers_agent.go
tech-stack:
  added: []
  patterns:
    - "one ReAct agent, four scopes — scope varies prompting + tool steering only; no new tool, allow-list untouched"
    - "citations from the REAL tool-call trace (scopeTrace), not from trusting the model to cite (RESEARCH Q2)"
    - "role bound server-side from the session; scope KIND is the only client-supplied dimension and fails safe to page Ask"
    - "untrusted selection delimited into the USER turn (delimitUntrusted), never the system prompt"
key-files:
  created:
    - internal/agent/scope_test.go
  modified:
    - internal/agent/agent.go
    - internal/agent/tools.go
    - internal/agent/prompts.go
    - internal/agent/stream.go
    - internal/agent/smoke_test.go
    - internal/server/handlers_agent.go
decisions:
  - "Citations are collected by a per-request scopeTrace wired INTO the tool closures (read_page/search_pages/search_attachments) rather than parsed from eino callbacks — the paths come from real tool calls, are deduped+ordered, and the nil trace is a no-op so non-workspace scopes and the allow-list test pay nothing."
  - "AskStream signature changed to take an agent.Scope and return ([]string, error) (the retrieved paths) — a cleaner contract than a bare scopePath string, and the returned slice lets the handler/audit see exactly what RAG cited."
  - "The workspace citation is delivered as a dedicated SSE 'event: citation' frame (JSON path array) emitted after the answer and before 'event: done' — the frontend renders the 'Reasoned over:' line from it; only emitted for workspace scope and only when RAG actually retrieved something."
  - "Role-scoping is enforced as a server-side BOUNDARY (the role comes from auth.CurrentUser, recorded in the audit Detail) even though this phase has no per-page read ACL (D-07 deferred): the structural guarantee is that only what the role-scoped search/pages services already permit can enter the prompt, and the scope KIND can never widen that."
metrics:
  duration_min: 18
  completed: 2026-06-21
  tasks: 2
  files: 6
  commits: 2
---

# Phase 4 Plan 03: Scope-aware Ask (selection / attachment / workspace-RAG) + citations Summary

Expanded Ask from page-only to all four scopes — **selection, attachment, and whole-workspace search-backed RAG** — over the SAME ReAct agent and unchanged 5-tool read-only allow-list built in slice 2. Workspace answers are top-K RAG (never a workspace dump) and carry a citation derived from the real tool-call trace (the pages the agent actually retrieved), role-bounded by the server session. Verified live against DeepSeek: a workspace Ask drove `search_pages`→`read_page` and the terminal SSE citation frame named exactly the retrieved page.

## What Was Built

- **`internal/agent/agent.go`** — `ScopeKind` (page/selection/attachment/workspace) + `Scope{Kind, Path, AttachmentID, Selection}` with `normalize()` (unknown→page, fail-safe). `scopeTrace`: a per-request, mutex-guarded, insertion-ordered, deduped collector of retrieved page paths; nil-safe `add`/`retrieved` (a copy, so callers can't corrupt it). `buildReActAgent` now threads the trace into `readTools`.
- **`internal/agent/tools.go`** — `readTools(deps, trace)`: the `read_page`, `search_pages`, and `search_attachments` closures record their surfaced page path on the (nil-safe) trace. **No new tool** — the slice and `readToolNames` are unchanged; the D5 allow-list build gate stays green.
- **`internal/agent/prompts.go`** — `selectionSystemPrompt` / `attachmentSystemPrompt` / `workspaceSystemPrompt` (terse, each instructs answer-only-from-context + honest refusal "that isn't in these pages", D7). `systemPromptForScope` (unknown→page Ask). `buildScopedMessages`: selection goes in the USER turn via `delimitUntrusted` (T-04-10); attachment steers to `read_attachment_text id=...`; workspace steers to search-backed retrieval ("do not read every page"). Removed the superseded `buildAskMessages`.
- **`internal/agent/stream.go`** — `AskStream(ctx, w, question, Scope) ([]string, error)`: scope-aware message build, a fresh trace per request, and a terminal `event: citation` SSE frame (JSON array of retrieved paths) emitted for workspace scope when RAG retrieved something — before `event: done`. Returns the retrieved path set. All the slice-2 SSE discipline (defer `sr.Close()`, request-ctx cancel, structured pre-stream errors, mid-stream error frame) is preserved.
- **`internal/server/handlers_agent.go`** — `agentChatRequest` gains `scope` / `selection` / `attachment_id`. `scopeKindFromRequest` maps the body string to a `ScopeKind` (unknown→page, never widens). The **role is read from the session** via the new `actorRole` (`auth.CurrentUser(ctx).UserRole()`), never the body (T-04-08), and recorded in the non-secret audit `Detail` as `scope=<kind> role=<role>`. Selection is length-capped (16k) + control-char-rejected. Calls the new `AskStream`; stays fail-closed (disabled→503, unreachable→502).
- **`internal/agent/scope_test.go`** (new, key-free) — scope `normalize` defaults, per-scope prompt anchors + D7 refusal text, untrusted-selection delimiting (injection string must NOT reach the system prompt), attachment tool steering, workspace no-dump prompt, nil-safe deduped trace (copy-on-read), and a role-scoped citation test (an out-of-role page absent from the fake searcher can never enter the trace).

## Tasks

| Task | Name | Commit |
|------|------|--------|
| 1 | Scope-aware Ask dispatch (selection/attachment/workspace) + role-scoped RAG citation trace | fe614ec |
| 2 | /agent/chat scope handling + workspace citation line + key-free scope tests + live workspace RAG test | 3dca302 |

## Verification

- `CGO_ENABLED=0 go build ./...` — green.
- `go vet ./internal/agent/ ./internal/server/` — clean.
- `go test ./internal/agent/ -run TestToolSet` — passes (D5 allow-list gate intact; nil trace = no-op).
- `go test ./internal/agent/... ./internal/server/...` — all pass (key present: live page Ask AND live workspace RAG both ran; key-free deterministic tests run regardless).
- `grep -q 'workspace' internal/agent/prompts.go` — present.

## Live RAG Ask — exercised? YES (key present, sources cited)

`DEEPSEEK_API_KEY` was set. `TestSmokeWorkspaceAskCitesRetrievedPage` drove the full AGNT-04 path live against `deepseek-v4-flash`: a whole-workspace question ("What is our deploy process?") made the ReAct agent call `search_pages` (→ the role-scoped fake searcher returning one page) then `read_page` (→ that page's body), stream the grounded answer as SSE, and emit a terminal `event: citation` frame. The returned citation set was exactly `[runbooks/deploy.md]` — **derived from the real tool-call trace, not the model's prose** — and the page deliberately absent from the searcher (the "out-of-role" page) never appeared. The slice-2 live page-Ask test also still passes under the new `Scope{Kind: ScopePage}` signature.

## Deviations from Plan

### Plan-structure adjustment (not a behavior deviation)

**1. Per-scope system prompts written in Task 1 rather than Task 2**
- The plan assigned the prompts (prompts.go) to Task 2 and the dispatch (agent.go) to Task 1. The dispatch in `buildScopedMessages` selects the per-scope prompt, so the two are one compile unit — splitting them would have left Task 1 non-building. The prompts therefore landed in commit fe614ec (Task 1); Task 2 (3dca302) carried the handler + the test coverage. Net scope is identical to the plan; both verifications passed at their respective gates.

### Auto-added (Rule 2 — correctness/robustness)

**2. [Rule 2] Selection length cap + control-char rejection on all scope inputs**
- The handler already rejected NUL in `page_path`; the new `selection` and `attachment_id` inputs needed the same defense, and an unbounded selection is a prompt/DoS vector. Added a 16k selection cap and NUL rejection across all three scope fields (input validation, T-04-11 family). Commit 3dca302.

**3. [Rule 2] Role recorded in the audit Detail**
- To make the server-side role-derivation auditable (T-04-08: the role bounds retrieval and must be session-derived), the audit `Detail` now carries `scope=<kind> role=<role>` (both non-secret). This is the visible proof that the role came from the session, not the body. Commit 3dca302.

## Threat Surface

No new surface beyond the plan's `<threat_model>`. Dispositions implemented:
- **T-04-08** (workspace RAG info disclosure) — search tools take their query from the server-built prompt; the role that scopes retrieval is read from `auth.CurrentUser` (session), never the client, and recorded in the audit. The citation trace is fed only by paths the role-scoped searcher actually surfaced (proven by `TestRunSearchRecordsCitationTraceRoleScoped`: an out-of-role page absent from the searcher never enters the trace).
- **T-04-09** (answer assembly leak) — config/env/session/other-users' content is never assembled into the prompt by construction; only tool-retrieved page/attachment text enters, and only via the repo.Resolve-backed read tools.
- **T-04-10** (indirect injection via selection/attachment/page) — no write tool reachable (allow-list gate); the untrusted selection is delimited into the USER turn (`TestBuildScopedMessagesDelimitsSelectionAsUntrusted` proves it never reaches the system prompt); answer-only, no apply path.
- **T-04-11** (RAG token blow-up) — workspace prompt forbids reading every page (top-K via search); `readToolMaxResults=5`, `MaxStep=12`, slice-1 60s/MaxTokens caps; selection length-capped.

## Known Stubs / Forward Dependencies

None that block the slice goal (all four scopes work and are tested; workspace is cited + role-bounded).

- **Frontend "Reasoned over:" rendering** — the backend emits the `event: citation` SSE frame (JSON path array) as the contract; the SPA consumer that renders the muted "Reasoned over: [page-a](…)" line is later UI work (UI-SPEC AgentPanel → Done). The backend half is complete and tested; the frontend half is intentionally out of this backend slice.
- **Per-page read ACL** — this phase has no per-page private-page model (D-07 deferred), so role-scoping is enforced as a server-side boundary (role from session, scope KIND can't widen access) rather than a per-page filter. When per-page ACL lands, the role-scoped `Deps.Search`/`Deps.Pages` are the single seam to enforce it — the agent code already only sees what those services permit.

## Self-Check: PASSED

- Files exist: `internal/agent/scope_test.go` (created); `internal/agent/agent.go`, `tools.go`, `prompts.go`, `stream.go`, `smoke_test.go`, `internal/server/handlers_agent.go` (modified) — all present.
- Commits exist in git history: `fe614ec`, `3dca302`.
</content>
</invoke>
