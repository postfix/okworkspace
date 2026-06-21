# Phase 4: Eino Agent - Context

**Gathered:** 2026-06-21
**Status:** Ready for planning

<domain>
## Phase Boundary

Deliver an approval-gated CloudWeGo Eino agent that lets a user ask/summarize/rewrite/draft/propose-patch over the current page, selected text, a selected attachment, or the whole workspace. The agent reads through existing safe services and **proposes** changes; no write is applied or committed until the user explicitly approves a concrete diff. Covers AGNT-01..AGNT-11. The read/write boundary is structural (write tools are not in the Eino graph), and the `DiffReviewDialog` built here is reused in Phase 5.

Out of scope: direct agent writes, multi-turn server-side conversation memory, agent push to Git, agent shell/secret access, any change to the byte-stable Markdown round-trip.

</domain>

<decisions>
## Implementation Decisions

### LLM Provider & Agent Loop
- **Provider: DeepSeek `deepseek-chat`** (user decision) via the OpenAI-compatible endpoint — `base_url: https://api.deepseek.com/v1`, `api_key_env: DEEPSEEK_API_KEY` (key already present in the environment, 35 chars), `enabled: true`. Provider-agnostic per locked decision — no code change vs Ollama/OpenAI, only `config.yaml` + `eino-ext openai.ChatModelConfig{BaseURL, APIKey, Model}` values. `deepseek-chat` chosen over `deepseek-reasoner` because it reliably supports function/tool calling required by the Eino ReAct loop.
- **Agent loop = hybrid.** Eino ReAct (`react.NewAgent`) only for tool-using modes (Ask about page/selection/attachment/workspace — needs `read_page`/`search_*`/`read_attachment_text` tool calls). Direct single-shot ChatModel call for Summarize / Rewrite / Draft / Propose-patch, where the relevant context is supplied directly (fewer round-trips, lower cost, fewer failure modes).
- **Conversation model = single-turn, stateless** per request. Context is assembled per call; no server-side conversation store. Fits a 5-user MVP.
- **Cost/failure guards:** per-request timeout (~60s), capped output tokens, and a structured error surfaced to the UI when the agent is disabled or the provider is unreachable (never a silent hang).

### Patch Proposal & Approval Flow (load-bearing safety)
- **Patch representation:** the agent returns the **full proposed new body**; the **server computes the old↔new diff** for display. Avoids fragile unified-diff hunk application and reuses the opaque `okf.Doc.Body` byte-stable round-trip.
- **Apply path:** on approval, reuse `pages.Service.Save` with `BaseRevision` (optimistic concurrency) → existing single-writer `gitstore.Commit` with `Action="approved_agent_patch"`, `Source="agent"`. No bespoke write path.
- **Stale-during-review:** capture the page revision at proposal time; if it has moved by approval time, **block the apply and tell the user the page changed (re-run)** — never silently overwrite. (Dovetails with Phase 5 conflict handling.)
- **Approval UI:** a `DiffReviewDialog` (built on `react-diff-viewer-continued`) shows a **real diff, never a prose summary**, with explicit Approve/Reject. Rejection discards. This component is reused by Phase 5.

### Tool Sandbox & Safety Boundary (structural enforcement)
- **Read access:** every agent read tool goes through the existing `pages` / `attachments` / `search` services (which already resolve via `repo.Resolve`). No raw `os.ReadFile` / direct filesystem access inside tools.
- **Write boundary:** apply/write is **not** an Eino tool. The Eino tool graph contains only read + propose tools. Applying an approved patch is a separate HTTP endpoint gated by explicit user approval — the structural defense against indirect prompt injection.
- **Secret/scope isolation:** assembled agent context excludes `config`, session cookies, env, and other users' private data; whole-workspace mode is scoped to role-readable pages only (respect reader/editor role from the session, never the client).
- **Audit:** record prompt, proposal, and approval/rejection via `audit.Logger` — add `ActionAgentPrompt`, `ActionAgentPatchProposal`, `ActionAgentPatchApproval`. Audit logging is non-fatal (matches existing pattern).

### Prompt UX & Context Scoping
- **Prompt entry:** a persistent bottom prompt bar (SPEC §6.4) with a mode selector (Ask / Summarize / Rewrite / Draft / Propose-patch) and context auto-set from the current view (page / selection / attachment), plus an explicit "whole workspace" toggle.
- **Result rendering:** streamed answers render in a right-side collapsible **Agent panel** (SSE); patch/rewrite approvals open the **DiffReviewDialog** modal; "draft new page" opens the new page in the editor pending an explicit save.
- **Rewrite-selection:** capture the selection range from `LivePreviewEditor` → agent rewrites → show old-selection↔new diff in the dialog → on approve, replace the selection through the normal save flow.
- **Whole-workspace Q&A = search-backed (RAG):** the agent uses `search_pages` / `search_attachments` tools to retrieve top-K relevant chunks; it never dumps the entire workspace into the prompt (token + cost control).

### Claude's Discretion
- Exact Eino `react.NewAgent` / `AgentConfig` / `utils.InferTool` wiring and tool schemas (verify against current eino + eino-ext source during phase research — Eino is pre-1.0, v0.9.9).
- Choice of Go helper for computing the display diff and for splitting body vs frontmatter on rewrite.
- SSE event framing details and the React `EventSource`/streaming-fetch consumer implementation.
- Right-side Agent panel visual specifics (defer to the UI-SPEC produced next).
- Context-truncation strategy and exact top-K for RAG retrieval.

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- **`config.AgentConfig`** already exists (`internal/config/config.go`): `Enabled`, `Provider`, `Model`, `BaseURL`, `APIKeyEnv`, redacted `apiKey` + `APIKey()` accessor, resolved from env at startup. **No struct change needed** — only `config.yaml` values for DeepSeek.
- **`repo.Resolve(rel string) (string, error)`** (`internal/repo/path.go`) — lexical + symlink + boundary path-safety chokepoint. All agent reads route through services that already use it.
- **`pages.Service`** (`internal/pages/service.go`) — `Get`/`Save(...BaseRevision)` with `ErrStaleRevision` (409). The approved-patch apply path reuses `Save`.
- **`okf.Parse`/`Emit`** (`internal/okf/okf.go`) — byte-stable round-trip; `Doc.Body` is opaque Markdown. Rewrite/patch operate on body, frontmatter preserved.
- **`gitstore.Commit(ctx, CommitSpec{Paths, Message, User, Action, Source})`** (`internal/gitstore/commit.go`) — single-writer commit; supports `Action="approved_agent_patch"`, `Source="agent"`.
- **`audit.Logger.Record(ctx, Event{Action, Actor, Target, Detail, Source, At})`** (`internal/audit/audit.go`) — add 3 agent action constants.
- **`search.Index`** (`internal/search`) — backs the RAG `search_*` tools.
- **SSE pattern** (`internal/server/handlers_sse.go`) — `text/event-stream` + `http.Flusher` (extraction-status stream) is the template for streamed chat.
- **Service DI + routing** — services constructed in `cmd/okf-workspace/main.go`; chi routes under `/api/v1` with `loadCurrentUser` + `auth.RequireRole(auth.RoleEditor)` + CSRF in `internal/server`.
- **Frontend** — `web/src`: `api/client.ts` (fetch + CSRF + credentials), `components/` (Dialog.tsx base, LivePreviewEditor.tsx), `stores/` (zustand). `@tanstack/react-query` and `react-diff-viewer-continued` are in package.json but not yet wired.

### Established Patterns
- Backend service = struct with injected deps + `now func() time.Time` for tests; constructed in main; thin chi handlers in `internal/server` calling the service.
- Handlers: `writeJSON` / `writeError` ({error}); editor-gated routes via `RequireRole`.
- Optimistic concurrency via `BaseRevision` → 409 `ErrStaleRevision`.
- Frontend dialogs built on `Dialog.tsx`; editor is raw CodeMirror 6.

### Integration Points
- New package `internal/agent/` (service + tools + eino wiring) — listed in SPEC §16 service shape.
- New `internal/server/handlers_agent.go` — 5 endpoints: `/api/v1/agent/chat` (SSE), `/summarize-page`, `/summarize-attachment`, `/propose-patch`, `/apply-patch`.
- New `web/src/components/DiffReviewDialog.tsx` (reused in Phase 5) + `PromptBar` + right-side `AgentPanel` + an SSE/streaming consumer + react-query hooks.
- `config.yaml` `agent:` block updated to DeepSeek + `enabled: true`.

</code_context>

<specifics>
## Specific Ideas

- DeepSeek is the LLM for this build; the API key is already in the environment as `DEEPSEEK_API_KEY` — wire `agent.api_key_env: DEEPSEEK_API_KEY`, do not hardcode or log the key.
- The DiffReviewDialog must render a **real diff** (never a prose summary) — this is the explicit human gate against indirect prompt injection.
- Phase research is REQUIRED before planning (ROADMAP note): re-verify `react.NewAgent` / `AgentConfig` / `utils.InferTool` / `openai.NewChatModel` signatures against current eino + eino-ext source, confirm the interrupt/resume (approval-gate) pattern, and smoke-test DeepSeek with `utils.InferTool`-generated tool schemas before building the full loop. Pin `eino` + `eino-ext` in `go.sum` immediately after `go get`.

</specifics>

<deferred>
## Deferred Ideas

- Multi-turn conversational memory / persisted agent threads — out of MVP scope (single-turn stateless chosen).
- Agent-initiated attachment creation / file uploads — keep to page propose/apply for this phase.
- Streaming token-by-token for summarize/rewrite/patch — only Ask/chat streams in MVP; proposals return JSON.
- Phase 5 collaboration conflict resolution reuses DiffReviewDialog but is built in Phase 5.

</deferred>
