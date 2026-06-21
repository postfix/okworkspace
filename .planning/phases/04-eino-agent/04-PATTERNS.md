# Phase 4: Eino Agent - Pattern Map

**Mapped:** 2026-06-21
**Files analyzed:** 17 (11 backend, 6 frontend)
**Analogs found:** 14 / 17 (3 net-new scaffolds with no in-repo analog — flagged)

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|-------------------|------|-----------|----------------|---------------|
| `internal/agent/agent.go` (service) | service | request-response | `internal/pages/service.go` | role-match (struct + injected deps + `now func()`) |
| `internal/agent/tools.go` | service/tool-layer | transform | `internal/search/query.go` (Query) + `internal/pages/service.go` (Get/Revision) | role-match |
| `internal/agent/tools_test.go` | test | n/a | `internal/audit/audit_test.go` table-driven; set-equality is net-new shape | partial |
| `internal/agent/chatmodel.go` | config/constructor | n/a | `internal/config/config.go` `AgentConfig.APIKey()` consumer | role-match (no in-repo eino analog — flag) |
| `internal/agent/propose.go` + `propose_test.go` | service | transform | `internal/pages/service.go` `Save`/`Revision` + `internal/okf` round-trip golden | role-match |
| `internal/agent/stream.go` | service | streaming | `internal/server/handlers_sse.go` | exact (Flusher+SSE template) |
| `internal/server/handlers_agent.go` (5 endpoints) | controller | request-response + streaming | `internal/server/handlers_pages.go` + `internal/server/handlers_sse.go` | exact |
| `internal/audit/audit.go` (modified) | model/const | event-driven | existing `Action*` const block (lines 28-54) | exact |
| `internal/config/config.go` | config | n/a | `AgentConfig` ALREADY EXISTS (lines 81-105) — no struct change | exact (no-op) |
| `cmd/okf-workspace/main.go` (modified) | wiring | n/a | `main.go` lines 230-290 (service construct + `server.Deps`) | exact |
| `internal/server/router.go` (modified) | route | request-response | `router.go` lines 94-192 (`editor.Use(RequireRole)` group) | exact |
| `web/src/api/client.ts` (modified) | utility/api | request-response + streaming | `client.ts` `mutate()` + `subscribeExtractionStatus()` (lines 50-85, 459-479) | exact |
| `web/src/components/DiffReviewDialog.tsx` (new, reused P5) | component | request-response | `web/src/components/Dialog.tsx` | role-match (built ON Dialog) |
| `web/src/components/PromptBar.tsx` | component | request-response | `web/src/routes/AppShell.tsx` chrome + `controls.css` | partial |
| `web/src/components/AgentPanel.tsx` + `AgentAnswer.tsx` | component | streaming | `MarkdownProse.tsx` render surface; SSE consumer net-new (flag) | partial |
| `web/src/stores/agentPanel.ts` | store | n/a | `web/src/stores/editorMode.ts` | exact |
| react-query provider wiring | provider | n/a | NONE — `@tanstack/react-query` unwired (flag) | none |

## Pattern Assignments

### `internal/agent/agent.go` (service, request-response)

**Analog:** `internal/pages/service.go` (lines 70-99)

The established backend service shape: a `Service` struct holding injected deps as **interfaces** (so unit tests inject fakes without standing up git/db), plus `now func() time.Time` for deterministic tests, constructed by a `NewService(...)` in `cmd/okf-workspace/main.go`.

**Struct + constructor pattern** (service.go:70-99):
```go
type Service struct {
	repo   *repo.Repo
	git    reviser      // interface, not *gitstore.GitStore — testability
	worker enqueuer     // interface
	db     *sql.DB
	now    func() time.Time
}
func NewService(r *repo.Repo, g *gitstore.GitStore, w *jobs.Worker, db *sql.DB, pushOnCommit bool) *Service {
	return &Service{repo: r, git: g, worker: w, db: db, now: time.Now}
}
```
For `agent.Service`: hold `cm *openai.ChatModel`, the prebuilt `*react.Agent` (or builder), narrow interfaces over `pages`/`search`/`attachments`/`audit`, and `now func()`. Define small consumer interfaces (like `reviser`/`enqueuer`) for `pages.Get`/`pages.Revision`/`pages.Save`, `search.Index.Query`, attachment text — do NOT depend on the concrete `*pages.Service` if a fake suffices for `tools_test`/`propose_test`.

**Sentinel-error pattern** (service.go:23-32): declare package-level `var ErrX = errors.New(...)` and let handlers map via `errors.Is`. Reuse `pages.ErrStaleRevision` directly for the apply path — do not invent a new stale error.

### `internal/agent/tools.go` (tool-layer, transform)

**Analog (read services):** `internal/search/query.go:105` `func (s *Index) Query(ctx, q) ([]Result, error)`; `internal/pages/service.go:147` `Get`, `:176` `Revision`.

`search.Result` is the typed DTO each `search_pages`/`search_attachments` tool returns (query.go:14-21): `{Kind, Title, Path, Snippet, Anchor, PageTitle}` — carries path + snippet + score-equivalent, enough for D3 citation. `pages.Get` returns `Page{Frontmatter, Body, Revision}` (service.go:104-108) for `read_page`.

**Read-only allow-list as single source of truth** (per RESEARCH §Pattern 2): build the `[]tool.BaseTool` slice AND a parallel `[]string` name list in ONE function; every closure calls a `repo.Resolve`-backed service (`pages.Get`, `search.Query`, attachment text) — never `os.ReadFile`. No write tool exists in this file or anywhere.

### `internal/agent/tools_test.go` (test — D5 load-bearing, AGNT-11)

**Analog (test idiom):** `internal/audit/audit_test.go` (table-driven, key-free). The set-equality assertion shape is net-new (RESEARCH §Pattern 2 gives the exact body): build `want` map of the 5 names, call `svc.readTools(...)`, `reflect.DeepEqual(want, got)` — any extra/missing tool fails the build. Runs offline, no API key. This is the build gate for the structural write boundary.

### `internal/agent/chatmodel.go` (constructor)

**Analog:** `internal/config/config.go` `AgentConfig` (lines 81-105) — already exists, **no struct change**. Consume `cfg.BaseURL`, `cfg.Model`, and `cfg.APIKey()` (the ONLY secret accessor, line 96; never log it — the redacted `String()`/`GoString()` at 99-105 keep `%v` safe).

**No in-repo eino analog (FLAG):** `openai.NewChatModel(ctx, &openai.ChatModelConfig{...})` is greenfield. Use the verified field set from RESEARCH §Item 4: `BaseURL`, `APIKey: cfg.APIKey()`, `Model: cfg.Model`, `Temperature *float32`, `MaxTokens *int` (always set). Default model `deepseek-v4-flash` (NOT `deepseek-chat` — retires 2026-07-24).

### `internal/agent/propose.go` + `propose_test.go` (service, transform)

**Analog:** `internal/pages/service.go:186` `Save(ctx, path, body, frontmatter, baseRevision, user) error` and `:176` `Revision(ctx, path)`; round-trip golden idiom from `internal/okf`.

Apply reuses the verified `Save` signature exactly (RESEARCH §"Apply"). Capture `Revision(ctx, path)` at propose time for the stale-during-review check (D8). `propose_test` asserts byte-stable round-trip + frontmatter preservation + churn ratio (D4) reusing the `internal/okf` golden pattern. Single-shot `Generate` returns the full new body; `validateProposedBody` + ≤2 retries (RESEARCH §4b).

### `internal/agent/stream.go` (service, streaming)

**Analog:** `internal/server/handlers_sse.go` (lines 50-117) — EXACT template.

**Flusher + header set** (handlers_sse.go:50-61):
```go
flusher, ok := w.(http.Flusher)
if !ok { writeError(w, http.StatusInternalServerError, "Streaming is not supported."); return }
w.Header().Set("Content-Type", "text/event-stream")
w.Header().Set("Cache-Control", "no-cache")
w.Header().Set("Connection", "keep-alive")
w.Header().Set("X-Accel-Buffering", "no")  // reuse — never let nginx buffer SSE
```
**Event framing** (handlers_sse.go:84): `fmt.Fprintf(w, "data: %s\n\n", payload); flusher.Flush()`. **Disconnect/cancel loop** (handlers_sse.go:101-116): `select` on `ctx.Done()` (`ctx := r.Context()`). Bridge `ag.Stream(ctx, msgs)` → `sr.Recv()` until `io.EOF`; **`defer sr.Close()` is non-negotiable** (RESEARCH §Item 6, goroutine leak). The existing handler has no `defer Close()` because it owns no producer goroutine — the eino StreamReader does, so this is the one addition.

### `internal/server/handlers_agent.go` (controller, request-response + streaming)

**Analog:** `internal/server/handlers_pages.go` (lines 181-215, `handleSavePage`) + `internal/server/handlers_sse.go` (the chat SSE).

**Handler skeleton + error mapping** (handlers_pages.go:181-214): nil-dep guard → decode request struct → call service → map sentinel errors via `errors.Is` → `writeJSON`/`writeError` → audit. The apply-patch handler mirrors `handleSavePage` exactly, including the stale branch:
```go
if errors.Is(err, pages.ErrStaleRevision) {
	writeError(w, http.StatusConflict, "This page changed since the assistant read it. Re-run to get a fresh proposal.")
	return
}
```
**writeJSON / writeError** live in `handlers_auth.go:73,79`. **Actor** via `h.actorUsername(r.Context())` (used at handlers_pages.go:195) — that resolves the session user; never read the user from the client body.

**Audit on apply** (handlers_pages.go:208-213): `_ = h.audit.Record(r.Context(), audit.Event{Action: audit.ActionAgentPatchApproval, Actor: ..., Target: path, Source: auditSourceWeb})` — non-fatal, ignore the error.

5 endpoints: `/agent/chat` (SSE, GET-style stream — any authed user may Ask), `/summarize-page`, `/summarize-attachment`, `/propose-patch` (editor), `/apply-patch` (editor, CSRF, the one consequential write).

### `internal/audit/audit.go` (model/const — modified)

**Analog:** the existing `Action*` const block (lines 28-54). Add alongside (exact same idiom, string literals):
```go
ActionAgentPrompt        = "agent_prompt"
ActionAgentPatchProposal = "agent_patch_proposal"
ActionAgentPatchApproval = "agent_patch_approval"
```
`Event` struct (lines 59-75) and `Record` (lines 98-133) are unchanged — they already carry only non-secret provenance and are non-fatal. Never put the prompt's secret-shaped content or the API key in `Detail`.

### `internal/server/router.go` (route — modified)

**Analog:** lines 94-192. Ask/summarize read endpoints go in the `authed.Group` (any authenticated user, line 94-95, after `authed.Use(h.loadCurrentUser)`). Propose/apply mutations go in the **editor-gated subgroup** (lines 138-176):
```go
authed.Group(func(editor chi.Router) {
	editor.Use(auth.RequireRole(auth.RoleEditor))
	editor.Post("/agent/propose-patch", h.handleProposePatch)
	editor.Post("/agent/apply-patch", h.handleApplyPatch)
})
```
CSRF (`nosurf`) is applied globally on mutating methods (router.go:210 `csrfProtect`) — apply-patch inherits it. Add `Agent *agent.Service` to the `Deps` struct (router.go:21-49) and the `authHandlers` field block (router.go:70-73), mirroring `Pages`/`Search`.

### `cmd/okf-workspace/main.go` (wiring — modified)

**Analog:** lines 230-290. Construct `agentSvc := agent.NewService(...)` next to `pagesSvc` (line 230) / `attachSvc` (line 236), threading `cfg.Agent`, `pagesSvc`, `searchIdx`, `attachSvc`, `auditLog`. Pass it into `server.New(server.Deps{... Agent: agentSvc})` (lines 279-290). `auditLog` is already built at line 114. The agent is gated on `cfg.Agent.Enabled` — when false, construct a disabled service (UI shows the "turned off" state) rather than skipping wiring, so handlers can return a structured "agent off" error.

### `web/src/api/client.ts` (utility/api — modified)

**Analog:** `mutate<T>()` (lines 50-85) for propose/apply POSTs; `subscribeExtractionStatus()` (lines 459-479) for the SSE consumer.

**Mutating call (propose/apply)** — reuse `mutate()` verbatim: it does `credentials: "same-origin"`, fetches+caches the CSRF token (`ensureCSRF`, lines 45-48), sets `X-CSRF-Token`, and throws an `Error & {status?}` so a 409 surfaces as `err.status === 409` (the stale-revision path, mirrors `savePage` at lines 248-253).

**SSE consumer** — `subscribeExtractionStatus` (lines 459-479) is the in-repo `EventSource` template: open `new EventSource(url)`, parse `JSON.parse(e.data)` per `onmessage`, `es.close()` on `onerror`, return an unsubscribe `() => es.close()`. **FLAG:** the agent chat stream is token-append (not a status enum) and the AI-SPEC streams an authenticated POST-with-body; `EventSource` cannot send a body or CSRF header. RESEARCH §Item 6 + AI-SPEC §4b expect a **fetch-stream reader** (`fetch(..., {body}).then(r => r.body.getReader())`) — a net-new helper, NOT a copy of `subscribeExtractionStatus`. Use the existing helper only as the lifecycle/parse/cleanup shape.

### `web/src/components/DiffReviewDialog.tsx` (component — new, reused Phase 5)

**Analog:** `web/src/components/Dialog.tsx` (focus trap, Esc/backdrop **cancel-only**, busy state).

Build ON `Dialog`: it already gives the focus trap (lines 53-94), `aria-modal`, and the **backdrop-never-confirms** contract (lines 9-10, 103-105) the trust gate requires. Props per UI-SPEC: `{ title, oldText, newText, summary?, onApprove, onReject, stale?, busy? }`. **Deviation from the analog (load-bearing, UI-SPEC §Trust Contract):** Dialog focuses the *first* focusable control (line 58-61); DiffReviewDialog must NOT auto-focus Approve — initial focus lands on Reject / the diff scroll region. The diff body is `react-diff-viewer-continued` (unwired dep — first use). Reuse `Dialog`'s `busy` → "Saving…" pattern for the apply round-trip.

### `web/src/components/PromptBar.tsx` / `AgentPanel.tsx` / `AgentAnswer.tsx` (components)

**Analogs:** `web/src/routes/AppShell.tsx` (3-row/3-column flex chrome), `web/src/styles/controls.css` (`.btn`/`.btn-primary`/`.select`/`.input`), `web/src/components/MarkdownProse.tsx` (the sanitized read-only render surface for streamed answers — rehype-raw OFF), `web/src/components/AutosaveStatus.tsx` (`aria-live="polite"` + `.spinner` live-status idiom for "Thinking…/Streaming…").

`AgentAnswer` renders streamed Markdown through the SAME `MarkdownProse` surface (UI-SPEC §Typography) so agent output matches page content and inherits the stored-XSS guard.

### `web/src/stores/agentPanel.ts` (store)

**Analog:** `web/src/stores/editorMode.ts` — EXACT pattern. Copy the `create<…>()(persist((set) => ({...}), { name: "okf.agent.panelOpen" }))` shape (editorMode.ts:22-31) for the collapse/expand boolean. zustand + `persist` middleware, localStorage-backed.

## Shared Patterns

### Authentication / Role gating
**Source:** `internal/auth/rbac.go` (`RequireRole`, lines 56-71; `CurrentUser`, lines 43-49)
**Apply to:** all `handlers_agent.go` endpoints. Reads role from the SESSION-bound user (`auth.CurrentUser(ctx)`), never client input. Editor gate on propose/apply via `editor.Use(auth.RequireRole(auth.RoleEditor))`; admin satisfies editor (roleSatisfies, lines 76-81). Ask/summarize are any-authed (the `authed.Group`).

### Error handling / sentinel mapping
**Source:** `internal/pages/service.go:23-32` (sentinels) + `handlers_pages.go:196-207` (`errors.Is` → status)
**Apply to:** all agent service methods and handlers. Reuse `pages.ErrStaleRevision` → 409 for apply; define `agent.Err*` sentinels (provider-unreachable, validation-failed, step-exhausted) the same way and map to a structured UI error (never a silent hang). `writeJSON`/`writeError` from `handlers_auth.go:73,79`.

### Audit (non-fatal)
**Source:** `internal/audit/audit.go` `Record` (lines 98-133) + call site `handlers_pages.go:208-213`
**Apply to:** prompt (`ActionAgentPrompt`), proposal, approval. Always `_ = h.audit.Record(...)` — ignore the error; never log the API key or secret-shaped prompt content in `Detail`.

### Secret handling
**Source:** `internal/config/config.go` `AgentConfig.APIKey()` (line 96) + redacted `String()` (lines 99-105)
**Apply to:** `chatmodel.go` only. `cfg.APIKey()` is the ONLY read path; the redacted Stringer keeps any logged config safe. Tools/context assembly never touch `config`, env, or session.

### Frontend fetch + CSRF + credentials
**Source:** `web/src/api/client.ts` `mutate()` / `ensureCSRF()` (lines 45-85)
**Apply to:** every new agent mutating call (propose/apply). `credentials: "same-origin"` + cached `X-CSRF-Token`; thrown `Error & {status?}` lets the dialog branch on 409 (stale) exactly like `savePage`.

## No Analog Found

| File / Concern | Role | Data Flow | Reason — planner must scaffold |
|------|------|-----------|--------|
| `internal/agent/chatmodel.go` (eino `openai.NewChatModel`) | constructor | n/a | No eino dependency in the repo yet (`go.mod` has no `cloudwego`). Greenfield; use RESEARCH §Item 4 verified signatures. Slice-1 `go get` + pin `go.sum`. |
| Agent chat SSE consumer in `client.ts` | utility | streaming | `subscribeExtractionStatus` uses `EventSource` (GET, no body, no CSRF). The agent stream is an authenticated POST-with-body token stream → needs a `fetch`+`ReadableStream` reader, a NEW helper. Existing helper is the lifecycle/cleanup model only. |
| `@tanstack/react-query` provider | provider | n/a | Dependency present in `package.json` (5.101.0) but UNWIRED — no `QueryClientProvider` anywhere in `web/src`. First-use setup (QueryClient + provider in `App.tsx`/`main.tsx`) is net-new scaffolding for propose/apply mutation state. |

## Metadata

**Analog search scope:** `internal/{pages,server,search,audit,config,auth}`, `cmd/okf-workspace`, `web/src/{api,components,stores,routes,styles}`
**Files scanned:** ~25 read/grepped
**Pattern extraction date:** 2026-06-21
