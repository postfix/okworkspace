# Phase 4: Eino Agent - Research

**Researched:** 2026-06-21
**Domain:** Go-native LLM agent integration (CloudWeGo Eino ReAct + OpenAI-compatible ChatModel) against DeepSeek, with a structural human-in-the-loop write boundary
**Confidence:** HIGH (all 7 ⚠ VERIFY-AT-BUILD items resolved against current `cloudwego/eino` + `eino-ext` `main` and the live DeepSeek API docs)

---

## Summary

Phase 4 wires an approval-gated Eino agent into the existing Go single-binary wiki. The heavy lifting (framework lock, mode mapping, eval strategy, UI contract) is already done in `04-AI-SPEC.md`, `04-CONTEXT.md`, and `04-UI-SPEC.md`. This research's job was to **resolve the pre-1.0 Eino API uncertainty** and **de-risk the DeepSeek backend** — both done. Eino is **not yet a dependency** (no `cloudwego` in `go.mod`/`go.sum`); this is greenfield integration into a mature codebase whose service seams (`pages`, `search`, `attachments`, `audit`, `repo.Resolve`, `gitstore`, SSE) all already exist and are confirmed below.

All seven flagged API signatures were verified against current source. The two material surprises that change the plan: **(1) `react.AgentConfig.Model` is now formally deprecated** — `ToolCallingModel model.ToolCallingChatModel` is the correct field, and `openai.ChatModel` declares `var _ model.ToolCallingChatModel = (*ChatModel)(nil)`, so it fits directly. **(2) `deepseek-chat` retires 2026-07-24 15:59 UTC — ~5 weeks after this research date.** The phase must ship config defaulting to `deepseek-v4-flash` (the documented non-thinking successor) and treat `deepseek-chat` as a transitional alias only. Because the design is provider-agnostic (`base_url`+`model` in `config.yaml`), this is a one-line config change, not a code change — but the plan must bake the new model id in from day one or the agent breaks dead in production within the milestone window.

A second pleasant surprise: `eino-ext/components/model/openai@latest` now resolves to a **real semver tag `v0.1.13`**, not a raw pseudo-version — the CLAUDE.md "pin the pseudo-version" note is outdated; pinning is now a normal `go get @v0.1.13`. `eino@latest` is **v0.9.9** stable (v0.10.0 is alpha-only — do **not** use the alpha).

**Primary recommendation:** Build in 6 vertical slices, deps-and-smoke-test first. Pin `eino@v0.9.9` + `eino-ext/components/model/openai@v0.1.13`, commit `go.sum` immediately, and write the `tools_test.go` allow-list assertion (D5) in the *same* slice that introduces the first tool — it is the load-bearing safety test and must never lag the tools it guards.

---

## User Constraints (from CONTEXT.md)

### Locked Decisions
- **Provider: DeepSeek** via OpenAI-compatible endpoint — `base_url: https://api.deepseek.com/v1`, `api_key_env: DEEPSEEK_API_KEY`, `enabled: true`. Provider-agnostic; no code change vs Ollama/OpenAI, only `config.yaml` values + `openai.ChatModelConfig{BaseURL, APIKey, Model}`. (Model id: see Pitfall/⚠ Item 5 — must be `deepseek-v4-flash`, not `deepseek-chat`.)
- **Agent loop = hybrid.** `react.NewAgent` ReAct loop ONLY for tool-using modes (Ask page/selection/attachment/workspace). Direct single-shot `ChatModel.Generate` for Summarize / Rewrite / Draft / Propose-patch.
- **Conversation model = single-turn, stateless** per request. No server-side conversation store.
- **Cost/failure guards:** per-request `context.WithTimeout(~60s)`, capped `MaxTokens`, structured error to UI on disabled/unreachable provider (never a silent hang).
- **Patch representation:** agent returns the **full proposed new body**; the **server** computes the old↔new diff for display. No fragile hunk application; reuse the opaque byte-stable `okf.Doc.Body`.
- **Apply path:** on approval, reuse `pages.Service.Save(...baseRevision...)` → existing single-writer `gitstore.Commit` with `Action="approved_agent_patch"`, `Source="agent"`. No bespoke write path.
- **Stale-during-review:** capture revision at proposal time; if it moved by approval, **block the apply** ("page changed, re-run") — never silently overwrite.
- **Approval UI:** `DiffReviewDialog` (on `react-diff-viewer-continued`) shows a **real diff, never a prose summary**, explicit Approve/Reject; reused in Phase 5.
- **Read access:** every agent read tool goes through existing `pages`/`attachments`/`search` services (all `repo.Resolve`-backed). NO raw `os.ReadFile`.
- **Write boundary:** apply/write is **NOT** an Eino tool — it is a separate approval-gated HTTP endpoint. The Eino graph contains read+propose tools only. This is the structural defense against indirect prompt injection.
- **Secret/scope isolation:** assembled context excludes `config`, session cookies, env, other users' private data; whole-workspace mode scoped to role-readable pages only (server session role, never the client).
- **Audit:** record prompt, proposal, approval/rejection via `audit.Logger` (`ActionAgentPrompt`, `ActionAgentPatchProposal`, `ActionAgentPatchApproval`); non-fatal.
- **Whole-workspace Q&A = search-backed RAG** via `search_pages`/`search_attachments` top-K; never dump the workspace.
- **Result rendering:** streamed Ask answers in a right-side collapsible Agent panel (SSE); patch/rewrite approvals open `DiffReviewDialog`; "draft new page" opens in editor pending explicit save.

### Claude's Discretion
- Exact Eino `react.NewAgent`/`AgentConfig`/`utils.InferTool` wiring and tool schemas (**resolved below**).
- Choice of Go helper for computing the display diff and for splitting body vs frontmatter on rewrite (**recommended below**).
- SSE event framing details and the React `EventSource`/streaming-fetch consumer.
- Right-side Agent panel visual specifics (defer to UI-SPEC — done).
- Context-truncation strategy and exact top-K for RAG retrieval.

### Deferred Ideas (OUT OF SCOPE)
- Multi-turn conversational memory / persisted agent threads.
- Agent-initiated attachment creation / file uploads.
- Streaming token-by-token for summarize/rewrite/patch (only Ask/chat streams in MVP; proposals return JSON).
- Phase 5 collaboration conflict resolution (reuses DiffReviewDialog but built in Phase 5).

---

## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| AGNT-01 | Ask about current page | ReAct `react.Agent` + `read_page` tool (⚠1–3); single-shot fallback if body supplied directly. |
| AGNT-02 | Ask about selected text | Selection passed in user turn; ReAct may still call tools; `LivePreviewEditor` selection range capture (UI-SPEC). |
| AGNT-03 | Ask about a selected attachment | `read_attachment_text` tool over `attachments` extracted text (`readExtractedText`/`GetPlainText`). |
| AGNT-04 | Ask about whole workspace | RAG: `search_pages`/`search_attachments` tools → `search.Index.Query` top-K; role-scoped. |
| AGNT-05 | Summarize a page | Single-shot `ChatModel.Generate`; body from `pages.Get`. Map-reduce if over budget. |
| AGNT-06 | Summarize an attachment | Single-shot; text from `attachments` extracted text. |
| AGNT-07 | Rewrite selected text → proposal | Single-shot; server diffs old-selection↔new → `DiffReviewDialog`. |
| AGNT-08 | Draft a new page | Single-shot; full body → editor pending explicit Save. |
| AGNT-09 | Propose a patch, shown as diff | Single-shot returns full new body; server diffs old↔new (go-udiff); `DiffReviewDialog`. |
| AGNT-10 | Explicit approval before apply+commit | `/apply-patch` endpoint reuses `pages.Save(baseRevision)` → `gitstore.Commit(Action="approved_agent_patch")`. |
| AGNT-11 | Agent can't write/secret/shell/escape/push | Structural: read-only tool allow-list (⚠3) + `tools_test.go` set-equality (D5) + `repo.Resolve`-backed services. |

---

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| LLM orchestration (ReAct loop, tool dispatch) | API / Backend (`internal/agent`) | — | Eino is a Go library compiled into the single binary; never the client. |
| Tool execution (read_page, search_*, read_attachment_text) | API / Backend | Database/Storage (via `repo.Resolve`-backed services) | Tools must run server-side so the read-only boundary and role scope are enforced in Go, not prompt. |
| Patch diff computation (old↔new) | API / Backend (`go-udiff` for locality metric/server) **and** Browser (display via `react-diff-viewer-continued`) | — | Server holds authoritative old body; browser renders the visual diff from old+new strings. |
| Approval gate (write decision) | Browser (DiffReviewDialog) → API (`/apply-patch`) | — | The consequential click is in the UI; the apply is a gated backend endpoint. Never a tool. |
| Apply/commit | API / Backend (`pages.Save` → `gitstore.Commit`) | — | Reuse existing single-writer commit path; `Source="agent"`. |
| SSE streaming of Ask answers | API / Backend (`http.Flusher`) → Browser (EventSource/fetch stream) | — | `Agent.Stream` → `schema.StreamReader` → SSE; React consumer appends tokens. |
| Context assembly / RAG retrieval | API / Backend | Database/Storage (Bleve `search.Index`) | Role-scoped retrieval + truncation done server-side before the prompt is built. |
| Prompt entry, mode/scope selection, answer/diff render | Browser (PromptBar, AgentPanel, DiffReviewDialog) | — | Pure UI per UI-SPEC; no business logic in the client. |

---

## Standard Stack

### Core (LOCKED — Eino is non-negotiable per PROJECT/SPEC/CLAUDE)
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `github.com/cloudwego/eino` | **v0.9.9** | ReAct agent, tool helpers, `schema.Message`/`StreamReader` | `@latest` = v0.9.9 (stable). v0.10.0 is **alpha-only** — do not adopt. `[VERIFIED: go list -m -versions, proxy.golang.org]` |
| `github.com/cloudwego/eino-ext/components/model/openai` | **v0.1.13** | OpenAI-compatible ChatModel (drives DeepSeek via BaseURL) | `@latest` now resolves to a **real semver tag** (not a pseudo-version — CLAUDE.md note is stale). Declares `var _ model.ToolCallingChatModel = (*ChatModel)(nil)`. `[VERIFIED: go list -m, proxy.golang.org]` |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `github.com/aymanbagabas/go-udiff` | **v0.4.1** | Pure-Go line/unified diff (server-side diff-locality metric for D4; optional server unified-diff) | **Pick.** µDiff is the modern successor to the gopls-internal differ; clean API, actively maintained, pure-Go (preserves single-binary). Use for the D4 "changed-line ratio" churn metric and any server-side diff. `[CITED: github.com/aymanbagabas/go-udiff]` |
| `react-diff-viewer-continued` | 4.2.2 (already in `web/package.json`) | The **visual** old↔new diff in `DiffReviewDialog` | Already a dep, unwired. Diffs `oldText`/`newText` strings in-browser — server need only send both bodies. `[VERIFIED: web/package.json]` |
| `@tanstack/react-query` | 5.101.0 (already in `web/package.json`) | Agent call caching / mutation state for propose/apply | Already a dep, unwired. `[VERIFIED: web/package.json]` |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| `go-udiff` (server diff) | `hexops/gotextdiff` v1.0.3 | gotextdiff is the older gopls-internal copy; produces unified diffs too. go-udiff is the actively-maintained successor with a friendlier API. Either is pure-Go and acceptable. |
| Server computes a diff at all | **Send old+new bodies only; diff entirely in-browser** | The locked decision is "server diffs old↔new for display," but `react-diff-viewer-continued` only needs the two strings. **Recommendation:** server sends both bodies (authoritative old from `pages.Get`); use `go-udiff` server-side *only* for the D4 churn metric and audit detail, not for rendering. This keeps the byte-stable body opaque. |
| `eino@v0.10.0-alpha.*` | — | Alpha; API may shift again. Stay on v0.9.9 stable. |

**Installation:**
```bash
# Backend — pin both, commit go.sum immediately (pre-1.0 / fast-moving).
go get github.com/cloudwego/eino@v0.9.9
go get github.com/cloudwego/eino-ext/components/model/openai@v0.1.13
go get github.com/aymanbagabas/go-udiff@v0.4.1
go mod tidy
git add go.mod go.sum   # pin the deps in the same commit

# Frontend — already present; just wire:
#   @tanstack/react-query 5.101.0, react-diff-viewer-continued 4.2.2 (no npm install needed)
```

**Version verification (this session):**
- `go list -m github.com/cloudwego/eino@latest` → `v0.9.9` (verified)
- `go list -m -versions github.com/cloudwego/eino` → latest non-alpha is **v0.9.9** (v0.10.0-alpha.2..6 exist; skip)
- `go list -m github.com/cloudwego/eino-ext/components/model/openai@latest` → `v0.1.13` (verified — real tag, not pseudo)
- `go list -m github.com/aymanbagabas/go-udiff@latest` → `v0.4.1` (verified)
- `go list -m github.com/hexops/gotextdiff@latest` → `v1.0.3` (verified)

---

## Package Legitimacy Audit

| Package | Registry | Age | Source Repo | Verdict | Disposition |
|---------|----------|-----|-------------|---------|-------------|
| `github.com/cloudwego/eino` | Go proxy | Mature (CloudWeGo/ByteDance OSS) | github.com/cloudwego/eino | OK | Approved (LOCKED) |
| `github.com/cloudwego/eino-ext/.../openai` | Go proxy | Active, real tag v0.1.13 | github.com/cloudwego/eino-ext | OK | Approved (LOCKED) |
| `github.com/aymanbagabas/go-udiff` | Go proxy | v0.4.1, maintained | github.com/aymanbagabas/go-udiff | OK | Approved |
| `react-diff-viewer-continued` | npm | maintained fork | github.com/Aeolun/react-diff-viewer-continued | OK | Already in package.json |

**Packages removed due to SLOP verdict:** none.
**Packages flagged suspicious (SUS):** none.
> Go modules verified directly via `go list -m` against `proxy.golang.org`; all resolve to real tagged versions with public source repos. No legitimacy gate concerns.

---

## ⚠ VERIFY-AT-BUILD Items — RESOLVED

> All verified against `cloudwego/eino` + `cloudwego/eino-ext` `main` on 2026-06-21 and the live DeepSeek API docs. Each gives the verified answer, source, and exact snippet.

### Item 1 — `react.AgentConfig` model field

**Resolved:** Use **`ToolCallingModel model.ToolCallingChatModel`**. The legacy `Model model.ChatModel` field still exists but `model.ChatModel` is **formally deprecated** (its `BindTools` mutates in place → data race when shared across goroutines). `openai.ChatModel` satisfies `ToolCallingChatModel` directly (`var _ model.ToolCallingChatModel = (*ChatModel)(nil)`), so pass it as `ToolCallingModel`. `[VERIFIED: github.com/cloudwego/eino/flow/agent/react/react.go + components/model/interface.go]`

The full current `AgentConfig` (verified):
```go
type AgentConfig struct {
    ToolCallingModel model.ToolCallingChatModel // ← USE THIS (recommended)
    Model            model.ChatModel            // legacy/deprecated — leave nil
    ToolsConfig      compose.ToolsNodeConfig
    MessageModifier  MessageModifier
    MessageRewriter  MessageModifier
    MaxStep          int `json:"max_step"`
    ToolReturnDirectly    map[string]struct{}
    StreamToolCallChecker func(ctx context.Context, modelOutput *schema.StreamReader[*schema.Message]) (bool, error)
    GraphName, ModelNodeName, ToolsNodeName string
}

func NewAgent(ctx context.Context, config *AgentConfig) (*Agent, error)
func (r *Agent) Generate(ctx context.Context, input []*schema.Message, opts ...agent.AgentOption) (*schema.Message, error)
func (r *Agent) Stream(ctx context.Context, input []*schema.Message, opts ...agent.AgentOption) (*schema.StreamReader[*schema.Message], error)
```

### Item 2 — `compose.ToolsNodeConfig{Tools: ...}` field name/type

**Resolved:** Field is **`Tools []tool.BaseTool`** (confirmed verbatim). `tool.InvokableTool` (what `InferTool` returns) satisfies `tool.BaseTool`. The ReAct agent receives the tool list via `AgentConfig.ToolsConfig` (type `compose.ToolsNodeConfig`). `[VERIFIED: github.com/cloudwego/eino/compose/tool_node.go]`

```go
// compose.ToolsNodeConfig (relevant fields):
type ToolsNodeConfig struct {
    Tools               []tool.BaseTool // ← read-only allow-list goes here
    ToolAliases         map[string]ToolAliasConfig
    UnknownToolsHandler func(ctx context.Context, name, input string) (string, error)
    ExecuteSequentially bool
    // ToolArgumentsHandler, ToolCallMiddlewares ...
}
```
Note `UnknownToolsHandler` is useful for DeepSeek hallucinated-tool resilience (Item 5).

### Item 3 — `utils.InferTool` signature + read-only registration

**Resolved:** `[VERIFIED: github.com/cloudwego/eino/components/tool/utils/invokable_func.go]`
```go
type InvokeFunc[T, D any] func(ctx context.Context, input T) (output D, err error)
func InferTool[T, D any](toolName, toolDesc string, i InvokeFunc[T, D], opts ...Option) (tool.InvokableTool, error)
func NewTool[T, D any](desc *schema.ToolInfo, i InvokeFunc[T, D], opts ...Option) tool.InvokableTool
```
A typed Go in/out struct becomes a tool: `InferTool` derives the JSON schema from `T`'s `jsonschema:"..."` struct tags; the framework validates the model's tool-call args against that schema before your closure runs. **Structural write-boundary guard:** build a single package-level slice of exactly the read tools, pass it as `ToolsConfig.Tools`, and assert its name-set in `tools_test.go` (see D5). Apply/write is NEVER constructed as a tool.

### Item 4 — `openai.NewChatModel` / `openai.ChatModelConfig` fields

**Resolved:** `[VERIFIED: github.com/cloudwego/eino-ext/components/model/openai/chatmodel.go]`
```go
func NewChatModel(ctx context.Context, config *ChatModelConfig) (*ChatModel, error)

type ChatModelConfig struct {
    APIKey         string                          // from env DEEPSEEK_API_KEY (never logged)
    Timeout        time.Duration                   // per-call timeout at the model layer
    HTTPClient     *http.Client
    ByAzure        bool                            // leave false for DeepSeek
    BaseURL        string                          // "https://api.deepseek.com/v1"
    APIVersion     string                          // Azure only — leave empty
    Model          string                          // "deepseek-v4-flash" (NOT deepseek-chat — Item 5)
    MaxTokens      *int                            // ALWAYS set in prod
    Temperature    *float32                        // 0.2 grounded / 0.4 draft
    TopP           *float32
    Stop           []string
    ResponseFormat *ChatCompletionResponseFormat   // JSON mode — prefer validate-and-retry over trusting it
    Seed           *int
    // PresencePenalty, FrequencyPenalty, LogitBias, User, ExtraFields,
    // ReasoningEffort, Modalities, Audio, MaxCompletionTokens ...
}
```
Confirmed: `Temperature` and `MaxTokens` are pointers (nil = provider default — never leave `MaxTokens` nil in prod). `Timeout` exists at the model layer **in addition to** the request `context.WithTimeout`.

### Item 5 — DeepSeek model validity + tool/JSON reliability ⚠ **PLAN-CRITICAL**

**Resolved — model id must change.** `deepseek-chat` and `deepseek-reasoner` are **retired 2026/07/24 15:59 UTC** ("fully retired and inaccessible"; requests fail after). Today is **2026-06-21** — ~5 weeks of runway. They map to the non-thinking / thinking modes of `deepseek-v4-flash`. The live, non-deprecated successors: **`deepseek-v4-flash`** (recommended — non-thinking, cheaper, reliable tool-calling) and `deepseek-v4-pro`. Both support tool calls + JSON output. `[CITED: api-docs.deepseek.com/quick_start/pricing, /updates]`

**Plan requirement:** ship `config.yaml` with **`model: deepseek-v4-flash`** as the default. `deepseek-chat` may be accepted as a transitional value but MUST NOT be the committed default, or the agent dies inside this milestone. Provider-agnostic design = this is `config.yaml` only, no code change.

**Tool-calling reliability:** DeepSeek's function-calling is real but **less consistent than GPT-4-class** — it "does not always generate valid JSON, and may hallucinate parameters not defined by your function schema." Mitigations (all in the locked design): keep tool schemas small/flat; terse tool descriptions; cap `MaxStep` (≤12); set `compose.ToolsNodeConfig.UnknownToolsHandler` to gracefully handle hallucinated tool names; **validate-and-retry** (≤2 retries, §4b `validateProposedBody`) rather than trusting `ResponseFormat` JSON mode; surface a structured error on step-exhaustion (never infinite retry). `[CITED: api-docs.deepseek.com/guides/function_calling]`

### Item 6 — Streaming bridge `Agent.Stream` → `http.Flusher` SSE

**Resolved.** `Agent.Stream(ctx, msgs)` returns `*schema.StreamReader[*schema.Message]`; `Recv()` yields `io.EOF` at end; **`Close()` is mandatory** or the producer goroutine leaks. The existing `internal/server/handlers_sse.go` (extraction-status stream) is the exact, in-repo template — reuse its header set, including `X-Accel-Buffering: no`. `[VERIFIED: eino/schema/stream.go + internal/server/handlers_sse.go]`

```go
func streamChat(ctx context.Context, ag *react.Agent, w http.ResponseWriter, msgs []*schema.Message) error {
    fl, ok := w.(http.Flusher)
    if !ok { return errors.New("streaming unsupported") }
    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")
    w.Header().Set("X-Accel-Buffering", "no") // matches existing handlers_sse.go

    sr, err := ag.Stream(ctx, msgs)
    if err != nil { return err }
    defer sr.Close() // NON-NEGOTIABLE — else producer goroutine leaks

    for {
        chunk, err := sr.Recv()
        if errors.Is(err, io.EOF) { break }
        if err != nil { return err }
        if _, werr := fmt.Fprintf(w, "data: %s\n\n", escapeSSE(chunk.Content)); werr != nil {
            return werr // client disconnect → request ctx cancels → Recv unblocks
        }
        fl.Flush()
    }
    return nil
}
```
Pass the **request `context.Context`** so a client disconnect cancels the model call and unblocks `Recv()`. (`escapeSSE` must collapse `\n` in the delta into `data:`-safe framing — SSE payloads can't contain raw newlines without a new `data:` prefix.)

### Item 7 — Unified-diff / patch approach (byte-stable safe)

**Resolved.** The locked design returns the **full new body**, so there is **no fragile hunk application** and **no risk to the okf round-trip** — apply just calls `pages.Save(path, newBody, frontmatter, baseRevision, user)` with the opaque body. For *display*, `react-diff-viewer-continued` diffs the two strings in-browser; the server need only send `oldBody` (authoritative, from `pages.Get`) and `newBody`. **Recommendation:** use **`github.com/aymanbagabas/go-udiff@v0.4.1`** server-side *only* for the D4 diff-locality (changed-line ratio) churn metric and audit `Detail`, not for rendering. Splitting body vs frontmatter on rewrite uses the existing `okf.Parse`/`Emit` (frontmatter preserved; only `Doc.Body` changes). `[VERIFIED: internal/okf, internal/pages/service.go; CITED: github.com/aymanbagabas/go-udiff]`

---

## Architecture Patterns

### System Architecture Diagram

```
                        ┌─────────────── Browser (web/src) ───────────────┐
  user prompt ─────────▶│ PromptBar (mode + scope + submit)               │
                        │   │                                              │
                        │   ├─ Ask  ─────────────▶ fetch SSE ──┐           │
                        │   └─ Summarize/Rewrite/ POST JSON ──┐ │           │
                        │      Draft/Propose                  │ │           │
                        │ AgentPanel ◀── token stream ────────┼─┘           │
                        │ DiffReviewDialog ◀── old+new body ──┘             │
                        │   │ Approve click                                 │
                        └───┼──────────────────────────────────────────────┘
                            │ POST /apply-patch (NOT a tool)
  ┌─────────────────────────▼──────────── API: internal/server/handlers_agent.go ───────────┐
  │ loadCurrentUser → RequireRole(editor) [+CSRF on mutations]                               │
  │  /agent/chat(SSE) /summarize-page /summarize-attachment /propose-patch  /apply-patch     │
  └─────┬──────────────────────────────────────────────────────────────────────┬───────────┘
        │                                                                        │
        ▼  internal/agent (Service)                                              ▼ apply (gated, NOT a tool)
  ┌─────────────────────────────────────────────┐                  ┌──────────────────────────────┐
  │ mode dispatch:                               │                  │ stale check: rev@propose==now?│
  │  ReAct path  ── react.Agent ─┐               │                  │   no → 409 "page changed"     │
  │  single-shot ── ChatModel ───┤               │                  │   yes → pages.Save(baseRev)   │
  │                              ▼               │                  │        → gitstore.Commit      │
  │   Tools (READ-ONLY allow-list, repo.Resolve) │                  │          Action=approved_     │
  │   read_page│search_pages│search_attachments  │                  │          agent_patch          │
  │   read_attachment_text│list_tree             │                  │        → audit.Record         │
  └───┬──────────┬───────────────┬───────────────┘                  └──────────────────────────────┘
      ▼          ▼               ▼
  pages.Get   search.Index   attachments(text)   ── all role-scoped, repo.Resolve-backed
      │          │               │
      ▼          ▼               ▼
   okf files   Bleve         extracted text      (config/env/session NEVER assembled into prompt)
```

Trace the primary use case (Propose-patch): user types instruction → POST `/propose-patch` → `internal/agent` single-shot `ChatModel.Generate(currentBody, instruction)` → validate body (retry ≤2) → return `{newBody, baseRevision}` → browser opens `DiffReviewDialog(old=currentBody, new=newBody)` → Approve → POST `/apply-patch` → stale check → `pages.Save` → `gitstore.Commit` → `audit.Record`.

### Recommended Project Structure
```
internal/agent/
├── agent.go         # Service: builds ChatModel + ReAct agent from config.AgentConfig; mode dispatch
├── tools.go         # InferTool defs (read_page, search_pages, search_attachments,
│                    #   read_attachment_text, list_tree) — read-only allow-list, repo.Resolve-backed
├── tools_test.go    # D5: asserts registered tool-name set == read-only allow-list (load-bearing)
├── prompts.go       # per-mode system prompts; untrusted content delimited in USER turn
├── propose.go       # single-shot propose-patch → full new body; validateProposedBody + retry
├── stream.go        # StreamReader → SSE http.Flusher bridge (mirror handlers_sse.go)
└── eval/            # offline LLM-judge harness (build-tagged; testdata/agent_eval.jsonl)
internal/server/
└── handlers_agent.go # /agent/chat(SSE), /summarize-page, /summarize-attachment,
                      #   /propose-patch, /apply-patch (apply = NOT a tool, separate gated endpoint)
internal/audit/      # add ActionAgentPrompt / ActionAgentPatchProposal / ActionAgentPatchApproval
web/src/
├── components/ DiffReviewDialog.tsx (reused Phase 5) · PromptBar.tsx · AgentPanel.tsx · AgentAnswer.tsx
├── lib/ sse consumer (fetch-stream)            stores/ agentPanel.ts (zustand, mirror editorMode.ts)
```

### Pattern 1: Service struct with injected deps + `now func()`
**What:** Match the established backend pattern — `agent.Service` is a struct holding the `*openai.ChatModel`, the prebuilt `*react.Agent` (or a builder), `pages`/`search`/`attachments` service refs, `audit.Logger`, and `now func() time.Time` for tests. Constructed in `cmd/okf-workspace/main.go` next to `pagesSvc`/`attachSvc`.
**When:** Always — consistency with every other service.

### Pattern 2: Read-only tool allow-list as a single source of truth
```go
// tools.go — the ONLY place tools are constructed. No write tool exists in this file or anywhere.
func (s *Service) readTools(ctx context.Context, sess Session) ([]tool.BaseTool, []string, error) {
    readPage, err := utils.InferTool("read_page", "Read the Markdown body of a workspace page by path.",
        func(ctx context.Context, in readPageIn) (readPageOut, error) {
            p, err := s.pages.Get(ctx, in.Path) // repo.Resolve-backed; role-scoped via sess
            if err != nil { return readPageOut{Found: false}, nil } // soft-miss
            return readPageOut{Body: p.Body, Found: true}, nil
        })
    // ... search_pages, search_attachments, read_attachment_text, list_tree ...
    tools := []tool.BaseTool{readPage /*, ...*/}
    names := []string{"read_page", "search_pages", "search_attachments", "read_attachment_text", "list_tree"}
    return tools, names, err
}
```
```go
// tools_test.go — D5 load-bearing test (build gate, key-free).
func TestToolSetIsExactlyReadOnlyAllowList(t *testing.T) {
    want := map[string]bool{"read_page":true,"search_pages":true,"search_attachments":true,
        "read_attachment_text":true,"list_tree":true}
    _, names, _ := svc.readTools(ctx, fakeSession)
    got := map[string]bool{}
    for _, n := range names { got[n] = true }
    if !reflect.DeepEqual(want, got) {
        t.Fatalf("tool set drifted from read-only allow-list: got %v", names) // any extra/missing tool fails the build
    }
}
```

### Anti-Patterns to Avoid
- **Putting apply/write/commit/push as an Eino tool.** Apply is `/apply-patch` only. The whole safety model is structural.
- **Using `AgentConfig.Model` (deprecated, mutating `BindTools`).** Use `ToolCallingModel`.
- **Returning early from a stream without `defer sr.Close()`.** Guaranteed goroutine leak.
- **Splicing untrusted page/attachment content into the SYSTEM prompt.** Untrusted content goes in the USER turn, delimited (`--- BEGIN PAGE CONTENT (untrusted) --- … --- END ---`).
- **Committing `deepseek-chat` as the default model.** It retires 2026-07-24; default `deepseek-v4-flash`.
- **Wrapping the proposed body in ``` fences or reflowing untouched paragraphs** — `validateProposedBody` must reject these (D4 churn).
- **Trusting DeepSeek `ResponseFormat` JSON mode** as a correctness guarantee — validate-and-retry.

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| JSON-schema for tool args | Hand-written JSON schema | `utils.InferTool` + `jsonschema` struct tags | Framework derives + validates the schema; one source of truth. |
| ReAct loop (call→exec→feed back) | Custom OpenAI loop | `react.NewAgent` | More surface to get the safety boundary + streaming wrong. |
| Streaming plumbing | Custom chunk channel | `Agent.Stream` → `schema.StreamReader` | Lifecycle (`Close`/`io.EOF`) already handled; just bridge to SSE. |
| SSE handler scaffold | New SSE writer | Copy `internal/server/handlers_sse.go` | In-repo template with `X-Accel-Buffering`, flush, terminal-event handling. |
| Optimistic concurrency on apply | New revision check | `pages.Save(...baseRevision...)` → `ErrStaleRevision` (409) | Already implements the stale-block floor (D8). |
| Commit on apply | New git path | `gitstore.Commit(Action="approved_agent_patch", Source="agent")` | Single-writer commit already exists. |
| Line-diff for churn metric | Custom diff | `aymanbagabas/go-udiff` | Pure-Go, maintained, gopls-lineage. |
| Visual diff render | Custom diff UI | `react-diff-viewer-continued` (already a dep) | Themed to tokens per UI-SPEC. |
| Password/secret handling in tools | Anything | Nothing — tools never touch `config`/env/session | Context assembly excludes them by construction. |

**Key insight:** Nearly every load-bearing capability already exists in the repo or in Eino. The net-new Go is the `internal/agent` orchestration + the 5 thin handlers; everything safety-critical (stale block, commit, path resolution, SSE) is reuse.

---

## Common Pitfalls

### Pitfall 1: Shipping a model id that retires mid-milestone
**What goes wrong:** Agent works in dev, then every request 4xx/errors after 2026-07-24 15:59 UTC.
**Root cause:** `deepseek-chat` retirement.
**Avoid:** default `model: deepseek-v4-flash` in `config.yaml`; smoke-test against it in slice 1.
**Warning sign:** model id `deepseek-chat`/`deepseek-reasoner` anywhere in committed config.

### Pitfall 2: Deprecated `Model` field silently disables/destabilizes tool calling
**What goes wrong:** ReAct loop misbehaves or races under concurrency.
**Root cause:** `model.ChatModel.BindTools` mutates in place.
**Avoid:** set `ToolCallingModel`, leave `Model` nil.
**Warning sign:** race detector flags on concurrent Ask; tools intermittently not called.

### Pitfall 3: Stream goroutine leak
**What goes wrong:** goroutines accumulate; memory creep.
**Root cause:** missing `defer sr.Close()` or non-cancelable ctx.
**Avoid:** `defer sr.Close()` always; pass request ctx.
**Warning sign:** rising goroutine count under `pprof`.

### Pitfall 4: A future edit leaks a mutating tool into the graph
**What goes wrong:** silent-write path opens; AGNT-11 broken.
**Root cause:** someone adds a "convenient" write tool.
**Avoid:** `tools_test.go` set-equality (D5) gates the build — any extra tool fails CI.
**Warning sign:** the allow-list test edited to "expect" a new tool name.

### Pitfall 5: DeepSeek emits malformed/empty tool-call args or hallucinated tools
**What goes wrong:** ReAct loops to step-exhaustion or errors.
**Root cause:** DeepSeek tool-calling is less consistent than GPT-4-class.
**Avoid:** flat/terse schemas; `MaxStep≤12`; `UnknownToolsHandler`; validate-and-retry; structured error on exhaustion.
**Warning sign:** rising step-exhaustion / malformed-tool-call rate in `slog`.

### Pitfall 6: Over-eager rewrite churn (huge diff for a one-line ask)
**What goes wrong:** unreviewable diff; authorial intent lost; D4 fail.
**Root cause:** model reformats the whole body.
**Avoid:** `validateProposedBody` rejects fenced/empty/frontmatter-mangled bodies; churn-ratio threshold via `go-udiff`; terse "return only the body, change only what's asked" system prompt.
**Warning sign:** changed-line ratio over threshold on a small instruction.

---

## Code Examples

### config.yaml agent block (DeepSeek)
```yaml
# Source: existing config.yaml lines 35-43 (replace placeholder values)
agent:
  enabled: true
  provider: "openai-compatible"
  model: "deepseek-v4-flash"          # NOT deepseek-chat (retires 2026-07-24)
  base_url: "https://api.deepseek.com/v1"
  api_key_env: "DEEPSEEK_API_KEY"     # 35-char key confirmed present in env; never logged
```
> `config.AgentConfig` struct is unchanged (`Enabled/Provider/Model/BaseURL/APIKeyEnv` + redacted `apiKey` + `APIKey()`); `apiKey` resolved from `DEEPSEEK_API_KEY` at startup (`config.go:145-146`).

### Single-shot propose-patch (server diffs / sends both bodies)
```go
// Source: 04-AI-SPEC.md §4 pattern, adapted to verified pages.Save signature.
func (s *Service) ProposePatch(ctx context.Context, path, instruction string) (newBody, baseRev string, err error) {
    ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
    defer cancel()
    p, err := s.pages.Get(ctx, path)
    if err != nil { return "", "", err }
    baseRev, err = s.pages.Revision(ctx, path) // capture for stale-during-review
    if err != nil { return "", "", err }
    out, err := s.cm.Generate(ctx, []*schema.Message{
        schema.SystemMessage(proposePatchSystemPrompt), // "Return ONLY the complete revised Markdown body. No prose, no fences. Change only what is asked."
        schema.UserMessage("--- BEGIN PAGE BODY (untrusted) ---\n"+p.Body+"\n--- END ---\n\nINSTRUCTION:\n"+instruction),
    })
    if err != nil { return "", "", err }
    if v := validateProposedBody(out.Content); v != nil { return "", "", v } // retry wrapper around this
    return out.Content, baseRev, nil
}
```

### Apply (gated endpoint — verified Save signature)
```go
// Source: internal/pages/service.go:186 Save(ctx, path, body, frontmatter, baseRevision, user) error
func (h *agentHandlers) applyPatch(w http.ResponseWriter, r *http.Request) {
    // ... decode {path, newBody, frontmatter, baseRevision}; RequireRole(editor) already applied ...
    err := h.pages.Save(r.Context(), req.Path, req.NewBody, req.Frontmatter, req.BaseRevision, currentUser(r))
    if errors.Is(err, pages.ErrStaleRevision) {
        writeError(w, http.StatusConflict, "This page changed since the assistant read it. Re-run to get a fresh proposal.")
        return // D8: never silently overwrite
    }
    // pages.Save already enqueues the single-writer gitstore.Commit; audit ActionAgentPatchApproval here.
}
```

---

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `eino-ext` openai pinned via raw pseudo-version | Real semver tag `v0.1.13` available | by 2026-06 | Pin with `@v0.1.13`; CLAUDE.md pseudo-version note is stale. |
| `react.AgentConfig.Model model.ChatModel` | `ToolCallingModel model.ToolCallingChatModel` | eino v0.9.x | `model.ChatModel` deprecated (mutating BindTools). |
| `deepseek-chat` / `deepseek-reasoner` | `deepseek-v4-flash` / `deepseek-v4-pro` | retire 2026-07-24 15:59 UTC | Default model MUST be v4-flash. |

**Deprecated/outdated:**
- `model.ChatModel` interface — use `model.ToolCallingChatModel`.
- `deepseek-chat`/`deepseek-reasoner` model ids — retiring within the milestone window.
- `eino@v0.10.0-alpha.*` — alpha; stay on v0.9.9 stable.

---

## Smallest-Safe Vertical Slice Ordering

> Sequenced so each slice ends green and de-risks the next. Slice 1 proves the riskiest unknown (Eino+DeepSeek wiring) before any UI.

1. **Deps + config + ChatModel smoke-test.** `go get` + pin (`eino v0.9.9`, `eino-ext openai v0.1.13`, `go-udiff v0.4.1`), commit `go.sum`. Update `config.yaml` agent block (`enabled:true`, `deepseek-v4-flash`). Build `agent.Service` constructor + a single-shot `ChatModel.Generate` smoke test that hits DeepSeek (gated behind the env key) and a `utils.InferTool`-derived schema round-trip. **Proves the pre-1.0 wiring + DeepSeek tool-calling before anything else.**
2. **Read tools + `tools_test.go` (D5) + ReAct Ask (page).** Build the 5 read tools (`read_page`, `search_pages`, `search_attachments`, `read_attachment_text`, `list_tree`) through existing services; write the allow-list set-equality test in the SAME slice; wire `react.NewAgent` (ToolCallingModel) + `/agent/chat` SSE (mirror `handlers_sse.go`) for Ask-page. AGNT-01. Add `ActionAgentPrompt` audit.
3. **Ask scope expansion: selection / attachment / workspace-RAG.** Selection passthrough, `read_attachment_text`, and top-K RAG via `search.Index.Query` (role-scoped). AGNT-02/03/04. Source-attribution (D3) on workspace answers.
4. **Single-shot modes: Summarize / Rewrite / Draft.** `/summarize-page`, `/summarize-attachment`; rewrite selection (server diffs old-selection↔new); draft → editor. `validateProposedBody` + retry. AGNT-05/06/07/08.
5. **Propose → diff → approve → apply (the safety core).** `/propose-patch` (full new body + baseRevision) → `/apply-patch` (stale check → `pages.Save` → `gitstore.Commit(Action="approved_agent_patch")` → audit `ActionAgentPatchProposal`/`ActionAgentPatchApproval`). D4 round-trip + D8 stale concurrency tests. AGNT-09/10/11.
6. **Frontend surfaces.** Wire `@tanstack/react-query` + SSE consumer; build `PromptBar`, `AgentPanel`/`AgentAnswer`, `DiffReviewDialog` (on `react-diff-viewer-continued`, Reject-focused, never prose-only) per UI-SPEC; agent-off/unreachable disabled states; `--agentpanel-width: 360px` token.

(Eval harness `internal/agent/eval/` + `agent_eval.jsonl` can land alongside slices 2–5; the deterministic D5/D4/D8 tests live with their slices.)

---

## Runtime State Inventory

> Greenfield agent integration (new package + new endpoints + config value). No rename/refactor/migration of existing stored state.

| Category | Items Found | Action Required |
|----------|-------------|------------------|
| Stored data | None — agent is single-turn/stateless; no new datastore. Audit rows append to existing audit table via existing `audit.Logger`. | Add 3 action constants (code only). |
| Live service config | `config.yaml` `agent:` block exists with placeholder values (`enabled:false`, Ollama URL). | Update to DeepSeek + `enabled:true` (config edit). |
| OS-registered state | None. | None. |
| Secrets/env vars | `DEEPSEEK_API_KEY` (35 chars) confirmed present in env; read via existing `APIKeyEnv`→`apiKey` resolution (`config.go:145`). Never logged (redacted Stringer). | None — wire `api_key_env: DEEPSEEK_API_KEY`. |
| Build artifacts | None — Eino not yet in `go.mod`/`go.sum`. | `go get` + commit `go.sum`. |

---

## Validation Architecture

> `workflow.nyquist_validation` not disabled → included. The eval/test strategy is fully specified in `04-AI-SPEC.md §5`; this maps it to the existing Go test infra.

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go stdlib `testing` (table-driven), as used across `internal/*` (e.g. `internal/pages/service_test.go`) |
| Config file | none (Go convention) |
| Quick run command | `go test ./internal/agent/...` |
| Full suite command | `go test ./... && go vet ./...` |
| LLM-judge suite (key required) | `go test -tags=eval -run TestAgentEval ./internal/agent/eval/...` |

### Phase Requirements → Test Map
| Req | Behavior | Test Type | Command | File Exists? |
|-----|----------|-----------|---------|-------------|
| AGNT-11 / D5 | tool set == read-only allow-list | unit (key-free) | `go test ./internal/agent/ -run TestToolSet` | ❌ Wave 0 |
| AGNT-10 / D8 | stale revision blocks apply (409, no write) | unit (key-free) | `go test ./internal/agent/ -run TestApplyStale` | ❌ Wave 0 (model on `pages/service_test.go:194`) |
| AGNT-09 / D4 | propose body byte-stable round-trip, frontmatter preserved, low churn | unit (key-free) | `go test ./internal/agent/ -run TestProposeRoundTrip` | ❌ Wave 0 (reuse `internal/okf` golden pattern) |
| AGNT-01..04 / D1 | ReAct picks correct read tool per scope; no step-exhaustion | unit trace + LLM-judge | `go test -tags=eval -run TestAgentEval` | ❌ Wave 0 |
| AGNT-04 / D2,D3,D7 | groundedness, citation, refusal | LLM-judge (offline) | `go test -tags=eval -run TestAgentEval` | ❌ Wave 0 |
| D6 | injection cannot cause silent write / leak | structural unit + judge | `go test ./internal/agent/...` + eval | ❌ Wave 0 |

### Sampling Rate
- **Per task commit:** `go test ./internal/agent/...` (deterministic, key-free, fast)
- **Per wave merge:** `go test ./... && go vet ./...`
- **Phase gate:** deterministic suite green; LLM-judge harness run pre-deploy/nightly with key.

### Wave 0 Gaps
- [ ] `internal/agent/tools_test.go` — D5 allow-list set-equality (covers AGNT-11)
- [ ] `internal/agent/propose_test.go` — D4 round-trip/frontmatter/churn (AGNT-09)
- [ ] `internal/agent/apply_test.go` — D8 propose@N → mutate N+1 → apply 409 (AGNT-10)
- [ ] `internal/agent/eval/` + `internal/agent/testdata/agent_eval.jsonl` (~30 labeled cases) — D1/D2/D3/D6/D7
- [ ] Eval build tag wiring (`//go:build eval`) + `OKF_EVAL_JUDGE_MODEL` env (judge ≠ DeepSeek to dodge self-preference)

---

## Security Domain

> `security_enforcement` enabled (absent = enabled). This phase is on the untrusted-input surface (agent tools, page/attachment content, approval gate) — SPEC §21.3.

### Applicable ASVS Categories
| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V1 Architecture / Trust Boundaries | **yes** | Structural write boundary — apply is a non-tool HTTP endpoint; read-only tool allow-list (`tools_test.go`). |
| V4 Access Control | **yes** | `RequireRole(editor)` on mutating/propose endpoints; workspace RAG scoped to session role (server, never client). |
| V5 Input Validation / Sanitization | **yes** | Untrusted page/attachment content delimited in USER turn; `validateProposedBody` rejects fenced/mangled bodies; streamed answers render through the existing sanitized Markdown surface (rehype-raw OFF). |
| V7 Error Handling & Logging | **yes** | `audit.Logger` (3 new actions); structured `slog` error classes; API key never logged (redacted Stringer). |
| V12 File / Path | **yes** | All reads via `repo.Resolve`-backed services; NO `os.ReadFile` in tools; path-traversal returns error not content (SEC-01 reuse). |
| V6 Cryptography | no (new) | Reuses existing session/CSRF/Argon2 stack; no new crypto. |
| CSRF | **yes** | mutating endpoints (`/apply-patch`) carry the existing `nosurf` CSRF middleware + `api/client.ts` token. |

### Known Threat Patterns for {Eino agent + DeepSeek + untrusted wiki content}
| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Indirect (content-borne) prompt injection → coerced write | Tampering / EoP | **Structural:** apply is a non-tool, human-gated endpoint + mandatory real-diff approval (never prose). Prompt delimiting is belt-and-suspenders only. |
| Injection → exfiltrate other pages / config into an answer | Information Disclosure | Role-scoped context assembly; `config`/env/session never assembled into prompt; D6/D7 adversarial eval cases. |
| Path traversal via tool args | Tampering / EoP | `repo.Resolve` on every read (lexical + symlink + boundary); soft-miss returns `Found:false`, never raw content. |
| Stale-revision silent overwrite | Tampering (data loss) | `pages.Save(baseRevision)` → `ErrStaleRevision` 409 block (D8). |
| Mutating tool leaks into graph (regression) | EoP | `tools_test.go` set-equality build gate (D5). |
| Unbounded loop / hung provider (DoS-ish) | DoS | `MaxStep≤12`, `MaxTokens` set, `context.WithTimeout(60s)`, structured fail-closed error. |
| API key disclosure in logs | Information Disclosure | Redacted `AgentConfig` Stringer; never log `APIKey()`. |

### SMTC semantic-analysis note
The SMTC MCP server (Go + TS) is connected for this repo. During execution the planner/executor should use SMTC for impact/reachability checks — specifically: **assert no call path reaches a file-write/`gitstore.Commit`/`os.*Write` from within `internal/agent`'s tool closures** (reinforces D5/AGNT-11 beyond the name-set test), and blast-radius review when wiring the new endpoints into the router. (Skill noted as available; no findings to report from research.)

---

## Project Constraints (from CLAUDE.md)

- **Go single static binary**, `CGO_ENABLED=0`; no Python sidecar — Eino (pure Go) preserves this. ✓
- **chi router**, `net/http`-native; mutating routes editor-gated + CSRF (existing pattern). ✓
- **Files-as-truth / byte-stable round-trip is sacrosanct** — patch returns full body, applied via `pages.Save` over opaque `okf.Doc.Body`; frontmatter preserved. ✓
- **Provider-agnostic LLM** via `config.yaml` only — DeepSeek/Ollama/OpenAI swap with no code change. ✓
- **`slog` structured logging**; API key never logged. ✓
- **Pin pre-1.0 eino + eino-ext in `go.sum` immediately after `go get`.** ✓ (now real semver tags, not pseudo-versions)
- **Do NOT** enable `rehype-raw` without sanitize on streamed answers — render through the existing sanitized Markdown surface. ✓
- **Note:** CLAUDE.md says `golang.org/x/crypto v0.53.0`; actual `go.mod` has `v0.14.0` (indirect). Not agent-relevant; flagged for accuracy only.

---

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | `eino-ext openai v0.1.13` is API-compatible with `eino v0.9.9` (mixed minor lines) | Standard Stack | Low — `var _ model.ToolCallingChatModel = (*ChatModel)(nil)` confirms interface fit; `go build` in slice 1 verifies. Re-pin if build breaks. |
| A2 | `deepseek-v4-flash` honors the same OpenAI-compatible tool-call + streaming shape as `deepseek-chat` | ⚠ Item 5 | Medium — docs say both v4 models support tool calls + JSON; slice-1 smoke test confirms. If flaky, fall back to `deepseek-v4-pro` or validate-and-retry harder. |
| A3 | Top-K=5 is a reasonable RAG default for a 5-user workspace | Context Window | Low — tunable at plan time; Claude's discretion per CONTEXT. |
| A4 | `search.Index.Query` returns enough (path + snippet + score) to drive citation (D3) | RAG / AGNT-04 | Low — `internal/search/query.go` `Result` DTO confirmed to carry typed results; verify fields at build. |

**If this table is empty:** it is not — A2 is the one to confirm in slice 1's smoke test.

---

## Open Questions (RESOLVED)

> Both questions have recommended resolutions below, incorporated into the plans (Q1 → 04-02 StreamToolCallChecker fallback; Q2 → 04-03 tool-call-trace citations). They are smoke-test-confirmable in slice 1/3, not blockers.

1. **RESOLVED (recommendation):** Does `deepseek-v4-flash` stream tool-call deltas cleanly via the eino-ext openai Stream path?
   - Known: both v4 models support tool calls + streaming per docs; eino-ext openai implements `Stream`.
   - Unclear: DeepSeek-specific streaming tool-call chunk behavior under Eino's `StreamToolCallChecker`.
   - Recommendation: smoke-test in slice 1; if checker misfires, set a custom `AgentConfig.StreamToolCallChecker` or fall back to non-streaming Generate for tool-heavy turns.

2. **Citation surfacing (D3) — prompt-driven vs. structured?**
   - Known: workspace RAG must name source pages.
   - Unclear: whether to have the model emit citations in prose or attach them from the tool-call trace.
   - Recommendation: attach from the tool-call trace (paths the agent actually retrieved) — more reliable than trusting the model to cite; render as the AgentPanel "Reasoned over: …" line.

---

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go toolchain | Build | ✓ | go 1.26 (`go.mod`) | — |
| `DEEPSEEK_API_KEY` env | Live agent calls | ✓ | 35-char key in env | Ollama via config (provider-agnostic) for offline dev/tests |
| DeepSeek API reachability | Ask/Summarize/etc. | (network) | `https://api.deepseek.com/v1` | structured "provider unreachable" UI state (fail-closed) |
| `cloudwego/eino` + `eino-ext` | Agent | ✗ (not yet) | v0.9.9 / v0.1.13 (to add) | none — required; `go get` in slice 1 |
| Bleve `search.Index` | RAG (AGNT-04) | ✓ | bleve v2.6.0 (indirect dep) | — (Phase 3 search must be functional; note SRCH-* are Pending) |

**Missing dependencies with no fallback:** Eino modules — added in slice 1. **Phase 3 search (`SRCH-*`) is Pending** in REQUIREMENTS.md — workspace-RAG (AGNT-04) depends on a functional `search.Index`; if Phase 3 indexing isn't complete, slice 3 (workspace RAG) is gated on it. **Flag for the planner:** confirm `search.Index.Query` is operational before sequencing AGNT-04, or scope AGNT-04 to whatever the index currently returns.
**Missing dependencies with fallback:** Live DeepSeek calls fall back to a structured error UI; offline tests use the key-free deterministic suite + (optionally) Ollama.

---

## Sources

### Primary (HIGH confidence)
- `cloudwego/eino` `main` — `flow/agent/react/react.go` (`AgentConfig`, `NewAgent`, `Generate`/`Stream`), `compose/tool_node.go` (`ToolsNodeConfig.Tools []tool.BaseTool`), `components/tool/utils/invokable_func.go` (`InferTool`/`NewTool`/`InvokeFunc`), `components/model/interface.go` (`ChatModel` deprecated vs `ToolCallingChatModel`) — verified 2026-06-21
- `cloudwego/eino-ext` `main` — `components/model/openai/chatmodel.go` (`ChatModelConfig` fields, `NewChatModel`, `WithTools`→`ToolCallingChatModel`, `var _ model.ToolCallingChatModel`) — verified 2026-06-21
- `go list -m` / `go list -m -versions` against `proxy.golang.org` — `eino v0.9.9` (latest stable; v0.10.0 alpha-only), `eino-ext openai v0.1.13`, `go-udiff v0.4.1`, `gotextdiff v1.0.3` — verified 2026-06-21
- Repo source — `internal/pages/service.go` (`Save(ctx,path,body,frontmatter,baseRevision,user)`, `ErrStaleRevision`, `Get`, `Revision`), `internal/server/handlers_sse.go` (SSE template), `internal/config/config.go` (`AgentConfig`, redacted Stringer, env resolution), `internal/audit/audit.go` (action constants), `internal/search/query.go` (`Result`), `go.mod`, `config.yaml`, `web/package.json` — read 2026-06-21

### Secondary (MEDIUM confidence)
- DeepSeek API docs — `quick_start/pricing` + `updates` (model retirement 2026/07/24 15:59 UTC; `deepseek-v4-flash`/`deepseek-v4-pro` current; tool calls + JSON support), `guides/function_calling` (validate args; may hallucinate params) — https://api-docs.deepseek.com
- `aymanbagabas/go-udiff` README + pkg.go.dev — pure-Go unified/line diff, gopls-lineage successor — https://github.com/aymanbagabas/go-udiff

### Tertiary (LOW confidence)
- WebSearch summaries of DeepSeek V4 migration blogs (corroborated against official docs above)

---

## Metadata

**Confidence breakdown:**
- Standard stack (eino/eino-ext/diff versions): HIGH — direct `go list -m` against proxy + source-verified interfaces.
- Eino API (all 7 ⚠ items): HIGH — verified against current `main` source, including the `ToolCallingChatModel` interface fit assertion in eino-ext.
- DeepSeek model status: HIGH (retirement date + successors from official docs); tool-call reliability MEDIUM (documented caveat → mitigated by validate-and-retry, smoke-test in slice 1).
- Architecture/integration points: HIGH — every seam (`pages.Save`, `ErrStaleRevision`, SSE handler, `AgentConfig`, audit) confirmed in-repo.

**Research date:** 2026-06-21
**Valid until:** ~2026-07-21 for eino (pre-1.0, fast-moving — re-verify if a new minor lands). **Hard expiry 2026-07-24** for the `deepseek-chat` model id (retires); `deepseek-v4-flash` guidance valid beyond.
