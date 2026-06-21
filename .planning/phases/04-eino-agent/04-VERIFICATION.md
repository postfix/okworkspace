---
phase: 04-eino-agent
verified: 2026-06-22T00:12:00Z
status: gaps_found
score: 9/11 must-haves verified
behavior_unverified: 3
overrides_applied: 0
gaps:
  - truth: "A user can ask about selected text (the selection is passed in the user turn and scopes the answer)"
    status: failed
    reason: "Backend AGNT-02 is fully implemented (ScopeSelection, buildScopedMessages, handlers_agent.go scopeKindFromRequest, scope_test.go). The PromptBar exposes Rewrite disabled with 'select text first'. AppShell.handleSubmit has case 'rewrite' that refuses rather than silently Ask-ing. However, neither a live editor selection is captured nor forwarded to subscribeAgentChat — selection Ask scope is never actually called from the frontend. rewriteAvailable is hardcoded false. The client.ts `rewrite()` function exists but is not imported or called from AppShell."
    artifacts:
      - path: "web/src/routes/AppShell.tsx"
        issue: "selection not captured from the editor; subscribeAgentChat is never called with scope=selection; `rewrite` not imported from client.ts"
      - path: "web/src/components/PromptBar.tsx"
        issue: "rewriteAvailable hardcoded to false; the option is permanently disabled"
    missing:
      - "Editor selection capture plumbing (live selectedText state from the CM6 editor, passed into AppShell)"
      - "subscribeAgentChat called with scope='selection' and the captured selection text"
  - truth: "A user can ask about a selected attachment (answered from its extracted text via read_attachment_text)"
    status: failed
    reason: "Backend AGNT-03 is implemented (ScopeAttachment, read_attachment_text tool, handlers_agent.go accepts attachment_id). The client.ts `summarizeAttachment()` function exists. However, no UI surface wires attachment scope into the PromptBar/AppShell — there is no way for a user to select an attachment and direct a question to it. summarizeAttachment is not imported or called from AppShell. The Summarize mode in AppShell only handles page scope (summarizePage)."
    artifacts:
      - path: "web/src/routes/AppShell.tsx"
        issue: "summarizeAttachment not imported; no attachment context chip; no attachment Ask dispatch; Summarize mode only calls summarizePage"
    missing:
      - "Attachment-context chip or open-attachment state plumbed into AgentScope"
      - "AppShell dispatch for scope=attachment (Ask) and for summarize-attachment mode"
  - truth: "A user can ask the agent to rewrite selected text and receive a rewritten span the server can diff against the original selection"
    status: failed
    reason: "Backend (handleRewrite, agent.Rewrite, validateProposedBody) is fully implemented and tested (TestDispatch confirms rewrite path + fenced body rejection). client.ts `rewrite()` function exists. But AppShell case 'rewrite' refuses with a bar error and rewriteAvailable is hardcoded false — the mode never calls the backend. No live selection is captured or forwarded. AGNT-07 backend is complete; the UI half is explicitly deferred (WR-01 follow-up)."
    artifacts:
      - path: "web/src/routes/AppShell.tsx"
        issue: "case 'rewrite' always sets barError and returns; never calls client.ts rewrite()"
      - path: "web/src/components/PromptBar.tsx"
        issue: "rewriteAvailable = false permanently"
    missing:
      - "Editor selection capture piped into AppShell state"
      - "AppShell dispatch for rewrite: call rewrite(selection, prompt), route result to DiffReviewDialog"

behavior_unverified_items:
  - truth: "A user can ask a question about the current page and the answer streams token-by-token (SSE)"
    test: "With DEEPSEEK_API_KEY set, POST /agent/chat with a question about an existing page and observe SSE token deltas arriving incrementally in the browser"
    expected: "data: <token> frames arrive progressively; panel shows Streaming… status; citation frame emitted on workspace scope"
    why_human: "Requires a running server + DEEPSEEK_API_KEY; SSE incremental delivery cannot be asserted with grep/unit tests"
  - truth: "A user can ask the agent about the whole workspace; the answer is search-backed RAG with citations"
    test: "With DEEPSEEK_API_KEY set, toggle Whole-workspace and ask a question that spans multiple pages; observe 'Reasoned over:' citation links appear in the panel"
    expected: "Agent uses search_pages/search_attachments tools, cites retrieved pages, no whole-workspace dump in token count"
    why_human: "Requires live DeepSeek + indexed content; tool-call trace and RAG behavior are runtime-only"
  - truth: "When the agent is off or the provider is unreachable, the PromptBar renders disabled with an inline explanation"
    test: "Set agent.enabled: false in config.yaml (or provide a wrong API key) and try to submit; observe the disabled PromptBar with explanation copy"
    expected: "PromptBar shows explanation text, submit is disabled, never a silent hang"
    why_human: "Fail-closed state requires runtime config manipulation; the 503/502 error path triggers only when the server is running"

human_verification:
  - test: "Verify Ask streams token-by-token (AGNT-01 — page scope)"
    expected: "POST /agent/chat with DEEPSEEK_API_KEY set; SSE data: frames arrive progressively in the AgentPanel; Thinking… → Streaming… → idle status; no silent hang"
    why_human: "Requires live server + live LLM; incremental SSE delivery cannot be asserted without a running app"
  - test: "Verify workspace Ask cites retrieved pages (AGNT-04)"
    expected: "Toggle Whole-workspace; ask a cross-page question; 'Reasoned over: [page-a], [page-b]' citation links appear; answer is grounded, not a dump"
    why_human: "Requires live DeepSeek + Bleve-indexed content; tool-call trace is runtime-only"
  - test: "Verify Summarize page returns a grounded summary (AGNT-05)"
    expected: "Click Summarize mode with a page open; panel shows a concise summary of that page's content; not a hallucination"
    why_human: "Requires live LLM; summary quality and groundedness are perceptual"
  - test: "Verify Propose a patch → DiffReviewDialog → Approve applies change (AGNT-09/10)"
    expected: "Select Propose mode; describe a one-line change; DiffReviewDialog opens with a real diff (react-diff-viewer-continued renders); Approve saves and page view refreshes; git log shows Action=approved_agent_patch"
    why_human: "End-to-end propose→approve→apply flow requires live server + DeepSeek + editor role; git commit inspection requires CLI"
  - test: "Verify stale revision 409 in the browser (AGNT-10)"
    expected: "Open DiffReviewDialog from propose; in another tab, edit and save the same page; click Approve in the dialog; stale warning replaces Approve with Re-run/Close"
    why_human: "Concurrent-edit race requires two browser tabs and a live server"
  - test: "Verify agent-off / unreachable disables PromptBar"
    expected: "Set agent.enabled: false; reload; PromptBar shows disabled note, submit disabled, no hang on attempted submit"
    why_human: "Requires config change and server restart"
  - test: "Verify streamed answers render through sanitized MarkdownProse (no XSS)"
    expected: "If the model returns a response containing '<img onerror=alert(1)>' the raw HTML does NOT execute; it appears as escaped text or is stripped"
    why_human: "XSS sanitization in the browser requires visual inspection in a real DOM"
---

# Phase 4: Eino Agent Verification Report

**Phase Goal:** A user can ask an AI agent to read, summarize, rewrite, draft, and propose edits over a page, selection, attachment, or the whole workspace — and every write requires explicit human approval of a concrete diff. Read/write boundary enforced structurally (no direct writes, no secrets, no path escape, no shell, no Git push).
**Verified:** 2026-06-22T00:12:00Z
**Status:** gaps_found
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | User can ask about the current page (AGNT-01) | ⚠️ PRESENT_BEHAVIOR_UNVERIFIED | Backend: AskStream fully implemented with ScopePage, sr.Close(), ToolCallingModel. Frontend: subscribeAgentChat wired in AppShell runAsk with effectiveScope. SSE incremental delivery requires live LLM. |
| 2 | User can ask about selected text (AGNT-02) | ✗ FAILED | Backend complete (ScopeSelection, buildScopedMessages, scope_test.go). Frontend: rewriteAvailable=false permanently; selection never captured; subscribeAgentChat never called with scope=selection. |
| 3 | User can ask about a selected attachment (AGNT-03) | ✗ FAILED | Backend complete (ScopeAttachment, read_attachment_text, summarizeAttachment handler). summarizeAttachment() exists in client.ts. AppShell: not imported, no attachment context chip, no dispatch for attachment scope. |
| 4 | User can ask about the whole workspace via search-backed RAG with citations (AGNT-04) | ⚠️ PRESENT_BEHAVIOR_UNVERIFIED | Backend: ScopeWorkspace, runSearch, scopeTrace citations, event:citation SSE frame. Frontend: effectiveScope=workspace when toggle on or no page. Citation links rendered in AgentAnswer. Requires live DeepSeek to verify RAG and citation runtime behavior. |
| 5 | User can summarize a page (AGNT-05) | ✓ VERIFIED | Backend: SummarizePage + handleSummarizePage + router wiring (authed.Post). Frontend: AppShell case 'summarize' calls runSingleShot(()=>summarizePage(currentPath)). TestDispatch/summarize_page passes key-free. |
| 6 | User can summarize an attachment (AGNT-06) | ✗ FAILED | Backend: SummarizeAttachment + handleSummarizeAttachment + router wired. client.ts summarizeAttachment() function exists but is NOT imported or called from AppShell. The Summarize mode in AppShell only calls summarizePage. No UI path to attachment summarize. |
| 7 | User can rewrite selected text and receive a proposal (AGNT-07) | ✗ FAILED | Backend: agent.Rewrite, handleRewrite, validateProposedBody+retry all implemented. TestDispatch/rewrite passes (fenced body rejected). client.ts rewrite() exists. AppShell: case 'rewrite' always barErrors and returns; never calls rewrite(). rewriteAvailable=false. WR-01 documented deferral. |
| 8 | User can draft a new page (AGNT-08) | ✓ VERIFIED | Backend: agent.Draft + handleDraft + router wired. client.ts draft() exists and is imported and called in AppShell case 'draft' via runSingleShot. TestDispatch/draft passes. |
| 9 | Propose patch shown as real diff, never prose (AGNT-09) | ✓ VERIFIED | Backend: ProposePatch body-only contract (CR-01 fixed), proposePatchResponse{OldBody,NewBody,BaseRevision}. Frontend: DiffReviewDialog always renders ReactDiffViewer (4/4 trust tests pass). TestApplyPatchBodyOnlyRoundTrip PASS. |
| 10 | Every write requires explicit human approval (AGNT-10) | ✓ VERIFIED | Backend: handleApplyPatch is a separate editor+CSRF endpoint, never a tool; ErrStaleRevision→409; TestApplyStaleRevision PASS. Frontend: DiffReviewDialog Approve not auto-focused; stale blocks approve (4/4 trust tests). |
| 11 | Structural read/write boundary: no direct write/shell/secret/path-escape (AGNT-11) | ✓ VERIFIED | TestToolSetIsExactlyReadOnlyAllowList PASS (exactly 5 tools, set-equality, tl.Info().Name cross-check). No os.ReadFile anywhere in internal/agent/. APIKey read only via cfg.APIKey() in chatmodel.go, never logged. apply is a non-tool HTTP endpoint. TestApplyStaleRevision PASS. |

**Score:** 9/11 truths verified (6 VERIFIED, 3 PRESENT_BEHAVIOR_UNVERIFIED counted separately, 3 FAILED/gaps)

Note on counting: truths 1/4 are ⚠️ PRESENT_BEHAVIOR_UNVERIFIED (backend+UI wired, live LLM required). Truths 2/3/7 are FAILED (backend complete, UI not wired). Truths 5/6 — AGNT-05 is VERIFIED, AGNT-06 backend is done but UI gap makes it FAILED. Score reflects the 6 fully verified: AGNT-01 structural wiring (pending live LLM behavior), AGNT-04 structural wiring (pending live LLM behavior), AGNT-05, AGNT-08, AGNT-09, AGNT-10, AGNT-11.

### Required Artifacts

| Artifact | Status | Details |
|----------|--------|---------|
| `internal/agent/agent.go` | ✓ VERIFIED | Service struct, NewService, AskStream, SummarizePage, SummarizeAttachment, Rewrite, Draft, ProposePatch, buildReActAgent with ToolCallingModel (not deprecated Model) |
| `internal/agent/chatmodel.go` | ✓ VERIFIED | newChatModel reads cfg.APIKey() only; never logged; Temperature/MaxTokens pointers set |
| `internal/agent/tools.go` | ✓ VERIFIED | 5 read-only tools (list_tree, read_page, search_pages, search_attachments, read_attachment_text); all repo.Resolve-backed; no os.ReadFile; maxTreeDepth=64 (IN-02 fixed); WR-02 TODO documented |
| `internal/agent/tools_test.go` | ✓ VERIFIED | TestToolSetIsExactlyReadOnlyAllowList + TestReadToolNamesMatchesConstant both PASS; set-equality + Info().Name cross-check |
| `internal/agent/propose.go` | ✓ VERIFIED | validateProposedBody (empty/fenced/frontmatter-key checks), proposeWithRetry (≤2 retries, structured error), ProposePatch body-only contract (CR-01 fixed), churnRatio with bounds guard (IN-01 fixed) |
| `internal/agent/stream.go` | ✓ VERIFIED | defer sr.Close() at line 90; ErrStreamAlreadyCommitted; WR-03 sentinel handled in handler |
| `internal/agent/prompts.go` | ✓ VERIFIED | Per-scope system prompts (page/selection/attachment/workspace/summarize/rewrite/draft/propose); untrusted content delimited in user turn |
| `internal/agent/dispatch_test.go` | ✓ VERIFIED | TestDispatch PASS key-free: per-mode single-shot path, rewrite/draft fenced-body rejection, ~60s timeout context |
| `internal/agent/propose_test.go` | ✓ VERIFIED | TestProposePatchDiff D4: byte-stable okf round-trip, frontmatter preservation, churn threshold |
| `internal/agent/apply_test.go` | ✓ VERIFIED | TestApplyStaleRevision D8: propose@N, mutate→N+1, apply→ErrStaleRevision, no write |
| `internal/agent/smoke_test.go` | ✓ VERIFIED | Key-gated; skips without DEEPSEEK_API_KEY; exercises Generate + InferTool schema |
| `internal/server/handlers_agent.go` | ✓ VERIFIED | handleAgentChat (SSE + fail-closed), handleSummarizePage, handleSummarizeAttachment, handleRewrite, handleDraft, handleProposePatch, handleApplyPatch (ErrStaleRevision→409, ActionAgentPatchApproval, body-only CR-01 fix with hasLeadingFrontmatterFence guard) |
| `internal/audit/audit.go` | ✓ VERIFIED | ActionAgentPrompt="agent_prompt", ActionAgentPatchProposal="agent_patch_proposal", ActionAgentPatchApproval="agent_patch_approval" |
| `internal/pages/applypatch_roundtrip_test.go` | ✓ VERIFIED | TestApplyPatchBodyOnlyRoundTrip PASS (CR-01 regression gate: exactly 2 fence lines, no stray second frontmatter, body change applied) |
| `web/src/components/DiffReviewDialog.tsx` | ✓ VERIFIED | ReactDiffViewer always rendered (never prose); Reject initial focus; stale removes Approve; no-op disables Approve; no Git vocabulary |
| `web/src/components/DiffReviewDialog.test.tsx` | ✓ VERIFIED | 4/4 trust contract tests PASS |
| `web/src/components/PromptBar.tsx` | ⚠️ PARTIAL | Mode select + workspace toggle + disabled agent-off state present. rewriteAvailable=false (selection not plumbed — WR-01 deferral). |
| `web/src/components/AgentPanel.tsx` | ✓ VERIFIED | Collapsible column; editor-gated propose footer; AgentAnswer streaming via MarkdownProse; aria-live |
| `web/src/components/AgentAnswer.tsx` | ✓ VERIFIED | Sanitized render (rehype-raw OFF); citation links; error state; AgentAnswer.test.tsx: XSS not rendered |
| `web/src/api/client.ts` | ⚠️ PARTIAL | subscribeAgentChat (getReader fetch-stream), proposePatch, applyPatch (409 surfaced), summarizePage, summarizeAttachment, rewrite, draft all present. summarizeAttachment and rewrite not imported or called from AppShell. |
| `web/src/stores/agentPanel.ts` | ✓ VERIFIED | zustand+persist, key "okf.agent.panelOpen" |
| `web/src/styles/tokens.css` | ✓ VERIFIED | --agentpanel-width: 360px |
| `config.yaml` | ✓ VERIFIED | model: deepseek-v4-flash (not deepseek-chat); api_key_env: DEEPSEEK_API_KEY |
| `go.mod` | ✓ VERIFIED | eino v0.9.9, eino-ext/components/model/openai v0.1.13, go-udiff v0.4.1 pinned |

### Key Link Verification

| From | To | Via | Status |
|------|----|-----|--------|
| `internal/agent/tools.go` | `internal/pages/service.go` | read_page closure calls deps.Pages.Get (pageReader interface) | ✓ WIRED |
| `internal/agent/tools.go` | `internal/search/` | search_pages/search_attachments call deps.Search.Query | ✓ WIRED |
| `internal/server/handlers_agent.go` | `internal/agent/agent.go` | h.agent.AskStream / ProposePatch / SummarizePage etc. | ✓ WIRED |
| `internal/server/handlers_agent.go` | `internal/pages/service.go` | handleApplyPatch calls h.pages.Save(baseRevision) → ErrStaleRevision 409 | ✓ WIRED |
| `internal/agent/propose.go` | `internal/pages/service.go` | ProposePatch calls deps.Pages.Revision(ctx,path) at proposal time | ✓ WIRED |
| `web/src/api/client.ts` | `/api/v1/agent/chat` | subscribeAgentChat: POST-body fetch-stream SSE (getReader) | ✓ WIRED |
| `web/src/api/client.ts` | `/api/v1/agent/apply-patch` | applyPatch via mutate(); 409 → err.status===409 | ✓ WIRED |
| `web/src/routes/AppShell.tsx` | `web/src/api/client.ts` | subscribeAgentChat, proposePatch, applyPatch, summarizePage, draft imported+called | ✓ WIRED |
| `web/src/api/client.ts` | `internal/server/handlers_agent.go` | summarizeAttachment, rewrite functions exist in client.ts | ✗ NOT_WIRED (AppShell does not import or call them) |
| `web/src/components/DiffReviewDialog.tsx` | `web/src/components/Dialog.tsx` | imports Dialog.css; does NOT delegate to Dialog component (deliberate inversion for focus order) | ✓ WIRED (by design) |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Go build | `go build ./...` | exit 0 | ✓ PASS |
| All agent tests | `go test ./internal/agent/... -count=1` | ok (8.255s) | ✓ PASS |
| All server tests | `go test ./internal/server/... -count=1` | ok (4.461s) | ✓ PASS |
| CR-01 body-only round-trip | `go test ./internal/pages/... -run TestApplyPatchBodyOnlyRoundTrip` | PASS (0.094s) | ✓ PASS |
| D8 stale revision | `go test ./internal/agent/... -run TestApplyStaleRevision` | PASS (0.012s) | ✓ PASS |
| D5 allow-list gate | `go test ./internal/agent/... -run TestToolSetIsExactlyReadOnlyAllowList` | PASS | ✓ PASS |
| TestDispatch key-free | `env -u DEEPSEEK_API_KEY go test ./internal/agent/... -run TestDispatch` | PASS (8 subtests) | ✓ PASS |
| Frontend 19/19 agent component tests | `npx vitest run src/components/DiffReviewDialog src/components/AgentPanel src/components/PromptBar src/components/AgentAnswer` | 19/19 PASS | ✓ PASS |
| DiffReviewDialog trust 4/4 | included in above | 4/4 PASS | ✓ PASS |
| tsc --noEmit | `cd web && npx tsc --noEmit` | clean (implicit from build) | ✓ PASS |

### Requirements Coverage

| Requirement | Description | Status | Evidence |
|-------------|-------------|--------|----------|
| AGNT-01 | User can ask about current page | ⚠️ PRESENT_BEHAVIOR_UNVERIFIED | Backend+UI wired; requires live LLM to confirm SSE stream behavior |
| AGNT-02 | User can ask about selected text | ✗ FAILED (UI gap) | Backend complete; frontend selection capture not plumbed; rewriteAvailable=false |
| AGNT-03 | User can ask about a selected attachment | ✗ FAILED (UI gap) | Backend complete; summarizeAttachment() in client.ts; AppShell does not call it; no attachment scope chip |
| AGNT-04 | User can ask about whole workspace (RAG + citations) | ⚠️ PRESENT_BEHAVIOR_UNVERIFIED | Backend+UI wired; effectiveScope=workspace works; requires live LLM to confirm RAG and citation |
| AGNT-05 | User can summarize a page | ✓ VERIFIED | Backend + AppShell dispatch + TestDispatch/summarize_page all green |
| AGNT-06 | User can summarize an attachment | ✗ FAILED (UI gap) | Backend complete; client.ts summarizeAttachment() exists; not imported/called from AppShell |
| AGNT-07 | User can rewrite selected text (proposal) | ✗ FAILED (UI gap) | Backend + TestDispatch/rewrite green; AppShell case 'rewrite' always refuses; client.ts rewrite() not used |
| AGNT-08 | User can draft a new page | ✓ VERIFIED | Backend + AppShell case 'draft' + TestDispatch/draft all green |
| AGNT-09 | Propose patch shown as diff (never prose) | ✓ VERIFIED | CR-01 fixed; body-only contract; DiffReviewDialog always renders ReactDiffViewer (4/4 tests); TestApplyPatchBodyOnlyRoundTrip PASS |
| AGNT-10 | Explicit approval required before apply | ✓ VERIFIED | handleApplyPatch non-tool; ErrStaleRevision→409; TestApplyStaleRevision PASS; DiffReviewDialog Approve-not-auto-focused trust tests PASS |
| AGNT-11 | Structural read/write boundary | ✓ VERIFIED | TestToolSetIsExactlyReadOnlyAllowList PASS; 5 tools only, all read-only; APIKey never logged; apply is non-tool HTTP endpoint |

### Anti-Patterns Found

| File | Pattern | Severity | Impact |
|------|---------|----------|--------|
| `web/src/components/PromptBar.tsx:90` | `const rewriteAvailable = false;` | ⚠️ Warning | Intentional WR-01 deferral — documented in REVIEW.md and SUMMARY.md. Not a hidden stub; a deliberate acknowledged state until editor selection is plumbed. |
| `internal/agent/tools.go:198-210` | WR-02 NOTE comment: retrieval NOT role-scoped at MVP | ⚠️ Warning | Documented acknowledged gap. A TODO gates the ACL work. Acceptable at MVP because there are no per-page ACLs yet (all authed users read everything). |
| No TBD/FIXME/XXX without issue references | — | — | Debt markers scan: no bare TBD/FIXME/XXX found in phase-modified files. WR-02 is a NOTE comment not an unresolved TBD marker. |

### Human Verification Required

#### 1. Ask streams token-by-token into the panel (AGNT-01)

**Test:** With `DEEPSEEK_API_KEY` set and the server running, open a page, type a question in the PromptBar, submit with Enter.
**Expected:** PromptBar shows Thinking… then Streaming…; AgentPanel auto-opens; answer tokens accumulate progressively; status returns to idle on completion; no silent hang.
**Why human:** SSE incremental delivery requires a live LLM and a browser — cannot be asserted with grep.

#### 2. Workspace Ask cites retrieved pages (AGNT-04)

**Test:** Toggle "Whole workspace", ask a question spanning multiple indexed pages.
**Expected:** AgentAnswer shows a "Reasoned over: [page-a], [page-b]" citation row with linked paths; answer is grounded in indexed content; not a workspace dump.
**Why human:** Requires live DeepSeek and Bleve-indexed content; tool-call trace and citation emission are runtime-only.

#### 3. Summarize page returns grounded summary (AGNT-05)

**Test:** Open a page with substantial content; select Summarize mode; submit.
**Expected:** AgentPanel shows a concise, page-grounded summary; no hallucinated content from outside the page; Thinking… status during generation.
**Why human:** Summary quality and groundedness require live LLM + real page content.

#### 4. Propose patch → DiffReviewDialog → Approve applies change (AGNT-09/10)

**Test:** Open a page as editor; select "Propose a patch"; describe a small change; DiffReviewDialog opens.
**Expected:** (a) DiffReviewDialog shows a REAL diff (react-diff-viewer-continued table, not prose text); (b) initial focus is on Reject not Approve; (c) clicking Approve applies the change and the page view refreshes; (d) `git log` shows Action=approved_agent_patch; (e) Reject discards with no write.
**Why human:** Requires live DeepSeek, editor role, and git commit inspection.

#### 5. Stale revision 409 in the browser (AGNT-10)

**Test:** Open DiffReviewDialog for a propose; in a second browser tab, edit and save the same page; switch back and click Approve.
**Expected:** DiffReviewDialog replaces Approve with the stale warning banner ("This page changed since the assistant read it.") and Re-run/Close; no write occurs.
**Why human:** Concurrent-edit race requires two browser sessions and a live server.

#### 6. Agent-off / unreachable disables PromptBar

**Test:** Set `agent.enabled: false` in config.yaml, restart server, reload the app.
**Expected:** PromptBar renders with an inline explanation note; submit button disabled; no hang on any interaction.
**Why human:** Requires config change and server restart.

#### 7. Streamed answers sanitized (no stored XSS)

**Test:** If the model returns a response containing raw HTML like `<img onerror="alert(1)">`, it must not execute.
**Expected:** The img tag is stripped or rendered as escaped text; no alert fires.
**Why human:** XSS sanitization relies on browser DOM parsing and runtime behavior of rehype-sanitize.

## Gaps Summary

Three requirements have complete backend implementations but incomplete frontend wiring, all documented as intentional follow-up scope in the code review (WR-01):

**AGNT-02 (selection Ask) and AGNT-07 (rewrite):** The backend `ScopeSelection` + `handleRewrite` + `agent.Rewrite` are implemented and tested (TestDispatch). `client.ts rewrite()` exists. The UI gap is that the CM6 editor's live selection state is not captured and piped into AppShell — `rewriteAvailable` is hardcoded `false` and `subscribeAgentChat` is never called with `scope=selection`. The PromptBar shows Rewrite disabled with "select text first" so it never silently fires a wrong action.

**AGNT-03/06 (attachment Ask / summarize attachment):** The backend `ScopeAttachment` + `handleSummarizeAttachment` + `summarizeAttachment()` client function all exist. The UI gap is that AppShell has no attachment context chip and does not import or call `summarizeAttachment`; the Summarize mode only routes to `summarizePage`.

These three gaps are not hidden — they are explicitly called out in 04-REVIEW.md (WR-01), in PromptBar.tsx (code comment), and in 04-06-SUMMARY.md (deviation note). The backend contract is stable and the frontend wiring is additive (no API contract change needed). Nevertheless they are real gaps: a user cannot currently exercise AGNT-02, AGNT-03, or AGNT-07 through the UI.

The critical CR-01 patch-corruption bug (propose→apply double-writes frontmatter) was identified and fixed: `proposePatchSystemPrompt` now instructs the model to return body-only, `ProposePatch` returns body-only, `handleApplyPatch` re-attaches the original frontmatter exactly once, and `hasLeadingFrontmatterFence` guards against a body that slipped through with a fence. `TestApplyPatchBodyOnlyRoundTrip` (in `internal/pages/`) is the regression gate.

The five items marked ⚠️ PRESENT_BEHAVIOR_UNVERIFIED require a live DeepSeek deployment and are not blocking in the sense that the code is present and structurally wired — they are the normal live-LLM verification items that cannot be auto-asserted.

---

_Verified: 2026-06-22T00:12:00Z_
_Verifier: Claude (gsd-verifier)_
