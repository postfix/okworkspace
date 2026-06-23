---
phase: 04-eino-agent
plan: 02
subsystem: agent
status: complete
tags: [eino, react, tools, allow-list, sse, streaming, deepseek, audit, agnt-01, agnt-11]
requires:
  - "internal/agent.Service + NewService(cfg, deps) (04-01)"
  - "internal/agent.newChatModel — eino-ext openai.ChatModel (04-01)"
  - "audit.ActionAgentPrompt (04-01)"
provides:
  - "internal/agent.readTools — the 5 read-only tools (list_tree, read_page, search_pages, search_attachments, read_attachment_text) as the single tool-construction site"
  - "internal/agent: D5 set-equality build gate (TestToolSetIsExactlyReadOnlyAllowList) — any 6th/mutating tool fails the build"
  - "internal/agent.Service.buildReActAgent — react.NewAgent with ToolCallingModel (Model nil), MaxStep=12, UnknownToolsHandler"
  - "internal/agent.Service.AskStream — schema.StreamReader→http.Flusher SSE bridge with defer sr.Close()"
  - "internal/agent prompts (Ask) — untrusted page/attachment content delimited to the USER turn"
  - "POST /api/v1/agent/chat — any-authed SSE Ask endpoint (AGNT-01), fail-closed, ActionAgentPrompt-audited"
  - "attachments.Service.ExtractedText — repo.Resolve-backed .txt accessor backing read_attachment_text"
affects:
  - internal/agent/agent.go
  - internal/server/router.go
  - internal/server/handlers_auth.go
  - cmd/okf-workspace/main.go
  - internal/attachments/service.go
tech-stack:
  added: []
  patterns:
    - "read-only tool allow-list + parallel name list derived in ONE function (cannot drift)"
    - "ReAct via react.NewAgent — ToolCallingModel ONLY (deprecated Model left nil, no racy BindTools)"
    - "StreamReader→Flusher SSE bridge mirrors handlers_sse.go + the one addition: defer sr.Close()"
    - "untrusted retrieved content delimited in the USER turn, never the system prompt"
    - "fail-closed structured errors before the first SSE byte (disabled→503, unreachable→502)"
key-files:
  created:
    - internal/agent/tools.go
    - internal/agent/tools_test.go
    - internal/agent/prompts.go
    - internal/agent/stream.go
    - internal/server/handlers_agent.go
  modified:
    - internal/agent/agent.go
    - internal/agent/smoke_test.go
    - internal/server/router.go
    - internal/server/handlers_auth.go
    - cmd/okf-workspace/main.go
    - internal/attachments/service.go
decisions:
  - "Tools built per-request in buildReActAgent (the heavy ChatModel is reused) so a future slice can role-scope the tool set without mutating shared state."
  - "read_attachment_text routes through a new attachments.Service.ExtractedText (repo.Resolve-backed .txt read) rather than the search package's private readExtractedText — keeps the agent depending on a narrow public interface, no os.ReadFile."
  - "The Deps.Attachments interface method was named ExtractedText(ctx,id) (not the slice-1 placeholder GetPlainText) to match the on-disk .txt-sidecar reality; the slice-1 interface was tightened accordingly."
  - "Added a live key-gated TestSmokeReActAskStream (reusing the slice-1 deepseekConfig harness) to prove AGNT-01 end-to-end; it skips clean key-free so deterministic CI stays green."
metrics:
  duration_min: 14
  completed: 2026-06-21
  tasks: 3
  files: 11
  commits: 4
---

# Phase 4 Plan 02: Read-only Tools + ReAct Ask (SSE) Summary

Delivered the first user-visible agent capability — **Ask about the current page, streamed token-by-token** — on top of the structural AGNT-11 write boundary. Five read-only tools route every file access through existing `repo.Resolve`-backed services, a load-bearing set-equality test build-gates the allow-list (a 6th/mutating tool fails the build), and the ReAct agent (ToolCallingModel) streams a grounded answer over SSE wired through `Deps.Agent` from `main.go`. Verified live end-to-end against DeepSeek `deepseek-v4-flash`.

## What Was Built

- **`internal/agent/tools.go`** — the ONLY tool-construction site. `readTools(deps)` builds five Eino tools via `utils.InferTool` (small flat jsonschema-tagged in/out structs per the DeepSeek-reliability guidance) and a parallel name list derived in the same function: `list_tree` (pages.Tree → page paths), `read_page` (pages.Get → {Body, Found}, soft-miss), `search_pages`/`search_attachments` (search.Query, kind-filtered, top-5), `read_attachment_text` (attachments.ExtractedText, repo.Resolve-backed .txt). No `os.ReadFile`, no write/apply tool anywhere.
- **`internal/agent/tools_test.go`** — `TestToolSetIsExactlyReadOnlyAllowList` (D5/AGNT-11): asserts the registered tool-name set EQUALS exactly the 5 read tools, cross-checking each tool's `Info().Name`; a 6th tool fails the build. Plus `TestReadToolNamesMatchesConstant`. Both run key-free/offline with nil Deps.
- **`internal/agent/agent.go`** — `buildReActAgent` constructs `react.NewAgent` with `ToolCallingModel` only (deprecated `Model` left nil), `MaxStep=12`, the read-only tool list, and an `UnknownToolsHandler` absorbing DeepSeek hallucinated tool names. Added `pageReader`/`ExtractedText` reader interfaces + `Deps.Pages`.
- **`internal/agent/prompts.go`** — per-mode system prompts (Ask), `buildAskMessages` (system + user turn carrying the question + scope hint), and `delimitUntrusted` so retrieved content stays DATA in the USER turn (T-04-05).
- **`internal/agent/stream.go`** — `AskStream`: `ag.Stream` → `schema.StreamReader` → `http.Flusher` SSE, mirroring `handlers_sse.go` headers/framing with `defer sr.Close()` (goroutine-leak guard) and request-ctx cancel; `escapeSSE` keeps multiline deltas SSE-safe; structured errors before the first byte, terminal SSE error frame mid-stream.
- **`internal/server/handlers_agent.go`** — `handleAgentChat`: any-authed SSE Ask, decodes `{prompt, page_path}` (actor from session, never body), length-caps the prompt, audits `ActionAgentPrompt` non-fatally (prompt text never in Detail), streams via `AskStream`, fail-closed (disabled→503, unreachable→502, never a hang).
- **Wiring** — `Deps.Agent` + `authHandlers.agent`; `POST /agent/chat` mounted in the authed group (inherits nosurf CSRF); `agentSvc` constructed in `main.go` with pages/search/attachments/audit deps.
- **`internal/attachments/service.go`** — `ExtractedText(ctx, id)`: repo.Resolve-backed `.txt` read, soft-miss on absent sidecar, no os.ReadFile.

## Tasks

| Task | Name | Commit |
|------|------|--------|
| 1 | Five read-only tools + D5 allow-list set-equality test (TDD) | 0729009 |
| 2 | ReAct agent (ToolCallingModel) + Ask prompts + StreamReader→SSE bridge | 40b5523 |
| 3 | /agent/chat SSE handler + router + main.go DI + ActionAgentPrompt audit | 414c1bd |
| + | Live key-gated ReAct AskStream end-to-end smoke test | 82eb22c |

## Verification

- `CGO_ENABLED=0 go build ./...` — green.
- `go vet ./internal/agent/ ./internal/server/` — clean.
- `go test ./internal/agent/ -run TestToolSet` — passes; verified it FAILS when a 6th tool name is injected, then restored (the build gate works).
- `go test ./internal/agent/...` — all pass (key present: the live single-shot Generate AND the live ReAct AskStream both ran).
- grep gates: `ToolCallingModel` set, `Model:` field NOT set, `sr.Close()` present, `Agent` in router.go, `agent.` in main.go, `ActionAgentPrompt` in handlers_agent.go.
- `go test ./internal/server/ ./internal/attachments/ ./internal/pages/` — green (no regressions from the new field/method).

## Live ReAct Ask — exercised? YES (key present)

`DEEPSEEK_API_KEY` (35 chars) was set. `TestSmokeReActAskStream` drove the full path against `deepseek-v4-flash`: the ReAct agent called `read_page` on a fake in-memory page, grounded its answer in that body, and streamed 299 bytes of token-by-token SSE (`data:` frames) terminated by `event: done` — in ~2.6s. AGNT-01 is proven end-to-end (tool loop + grounding + SSE bridge), not just wired.

## Allow-list test status

**PASS, and proven to be a real gate.** `TestToolSetIsExactlyReadOnlyAllowList` asserts exact set equality of the 5 read-tool names (with an `Info().Name` cross-check so the name list cannot lie). Injecting a 6th name (`write_page`) made the suite FAIL; removing it restored green. The head comment states it must never be relaxed to expect a new tool — apply/write is a non-tool HTTP endpoint (slice 5), never an Eino tool.

## Deviations from Plan

### Auto-fixed / interface adjustments (Rule 2/3)

**1. [Rule 3 - Blocking] Slice-1 `attachmentReader` interface didn't match on-disk reality**
- **Found during:** Task 1 (wiring read_attachment_text).
- **Issue:** The slice-1 `Deps.Attachments` interface declared `GetPlainText(ctx, id)`, but `attachments.Service` had no such method — only the search package privately read the `.txt` sidecar. Routing the tool through a non-existent method would not compile.
- **Fix:** Added `attachments.Service.ExtractedText(ctx, id)` (repo.Resolve-backed `.txt` read, soft-miss on absent sidecar) and renamed the agent's `attachmentReader` method to `ExtractedText` to match. This keeps the read routed through a narrow public interface, never `os.ReadFile`.
- **Files:** internal/attachments/service.go, internal/agent/agent.go.
- **Commits:** 0729009 (interface), 414c1bd (method + wiring).

**2. [Rule 2 - Robustness] Added `Deps.Pages` + `pageReader` interface**
- The slice-1 Deps had no page-read seam; `read_page`/`list_tree` need `pages.Get`/`pages.Tree`. Added a narrow `pageReader` interface (`*pages.Service` satisfies it) + `Deps.Pages` so the tools route through a testable seam without depending on the concrete service.
- **Commit:** 0729009.

### Notes (not deviations)

- **Live ReAct test added beyond the plan's task list.** The plan's deterministic gate is the load-bearing requirement; the live test is an additive key-gated proof of AGNT-01 (skips clean key-free), reusing the slice-1 harness. No production code changed for it.
- The slice-1 `pageWriter` interface remains declared (slice-5 apply scaffolding) and is intentionally NOT exposed as a tool — confirmed by the allow-list gate.

## Threat Surface

No new surface beyond the plan's `<threat_model>`. All five dispositions implemented:
- **T-04-03** (write-tool leak) — tools_test.go set-equality build gate, proven to fail on a 6th tool.
- **T-04-04** (read path-args) — every read routes through pages.Get / search.Query / attachments.ExtractedText (all repo.Resolve-backed); soft-miss returns Found:false, never raw bytes off a model-supplied path.
- **T-04-05** (indirect prompt injection) — no write tool reachable from /agent/chat; untrusted content delimited to the USER turn (belt-and-suspenders).
- **T-04-06** (stream DoS) — `defer sr.Close()`, request-ctx cancel on disconnect, `MaxStep=12`, slice-1 60s/MaxTokens caps.
- **T-04-07** (workspace-scope) — search tools take their query from the server-built prompt; config/env/session never assembled into the prompt; the API key never enters a prompt, tool result, or SSE frame.

## Known Stubs

None that block the slice goal. The `pageWriter` Deps interface is unwired forward-looking scaffolding for slice 5 (apply), documented in code and correctly absent from the tool surface (the allow-list gate enforces this). The other agent modes (Summarize/Rewrite/Draft/Propose) are intentionally later slices — `systemPromptFor` falls back to the safe read-only Ask prompt for any unknown mode.

## Self-Check: PASSED

All created files exist on disk (tools.go, tools_test.go, prompts.go, stream.go, handlers_agent.go) and all four commits (0729009, 40b5523, 414c1bd, 82eb22c) exist in git history.
