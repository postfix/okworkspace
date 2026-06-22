---
phase: 04-eino-agent
verified: 2026-06-22T09:54:00Z
status: human_needed
score: 11/11 must-haves verified
behavior_unverified: 3
overrides_applied: 0
re_verification:
  previous_status: gaps_found
  previous_score: 9/11
  gaps_closed:
    - "AGNT-02 selection Ask: AppShell now reads selectionLength/selection from agentContext store; effectiveScope='selection' when a non-empty selection is live; subscribeAgentChat called with scope='selection' and the captured text (line 292-303 AppShell.tsx)"
    - "AGNT-03/06 attachment Ask + summarize: AttachmentCard 'Ask about this file' Sparkles button sets attachment context in agentContext store; AppShell dispatches subscribeAgentChat(scope='attachment', attachment_id) for Ask and summarizeAttachment(id) for Summarize when an attachment is in context (AppShell.tsx lines 365-377)"
    - "AGNT-07 rewrite: rewriteAvailable is now driven by real selectionLength (not hardcoded false); case 'rewrite' calls rewriteMutation.mutate({selection, instruction}) and routes the result to DiffReviewDialog; applyRewriteMutation applies only on explicit Approve; never auto-applies (AppShell.tsx lines 207-269)"
  gaps_remaining: []
  regressions: []
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
  - test: "Verify selection Ask streams answer scoped to selection (AGNT-02)"
    expected: "Select text on a page, type a question in Ask mode; PromptBar chip shows 'Selection (N chars)'; AgentPanel answer references the selected passage specifically"
    why_human: "Requires live server + DeepSeek; selection-scoped answer quality is perceptual"
  - test: "Verify workspace Ask cites retrieved pages (AGNT-04)"
    expected: "Toggle Whole-workspace; ask a cross-page question; 'Reasoned over: [page-a], [page-b]' citation links appear; answer is grounded, not a dump"
    why_human: "Requires live DeepSeek + Bleve-indexed content; tool-call trace is runtime-only"
  - test: "Verify Summarize page returns a grounded summary (AGNT-05)"
    expected: "Click Summarize mode with a page open; panel shows a concise summary of that page's content; not a hallucination"
    why_human: "Requires live LLM; summary quality and groundedness are perceptual"
  - test: "Verify attachment Ask + summarize (AGNT-03/06)"
    expected: "Click 'Ask about this file' Sparkles on an attachment card; PromptBar chip shows the filename; ask a question; answer is grounded in the file's extracted text. For Summarize mode, switch to Summarize and submit; panel returns a summary of the file content."
    why_human: "Requires live server + DeepSeek + extracted-text content; answer groundedness is perceptual"
  - test: "Verify Rewrite selection → DiffReviewDialog → Approve applies change (AGNT-07)"
    expected: "Select text in the editor; Rewrite option becomes enabled; submit a rewrite instruction; DiffReviewDialog opens with old=selection / new=rewrite (real diff, not prose); Reject discards; Approve replaces the selection span in the page and saves; page view refreshes"
    why_human: "Requires live server + DeepSeek + editor role; DiffReviewDialog diff quality and post-Approve page state require a running browser"
  - test: "Verify Propose a patch → DiffReviewDialog → Approve applies change (AGNT-09/10)"
    expected: "Select Propose mode; describe a one-line change; DiffReviewDialog opens with a real diff (react-diff-viewer-continued renders); Approve saves and page view refreshes; git log shows Action=approved_agent_patch"
    why_human: "End-to-end propose→approve→apply flow requires live server + DeepSeek + editor role; git commit inspection requires CLI"
  - test: "Verify stale revision 409 in the browser (AGNT-10)"
    expected: "Open DiffReviewDialog from propose or rewrite; in another tab, edit and save the same page; click Approve in the dialog; stale warning replaces Approve with Re-run/Close"
    why_human: "Concurrent-edit race requires two browser tabs and a live server"
  - test: "Verify agent-off / unreachable disables PromptBar"
    expected: "Set agent.enabled: false; reload; PromptBar shows disabled note, submit disabled, no hang on any interaction"
    why_human: "Requires config change and server restart"
  - test: "Verify streamed answers render through sanitized MarkdownProse (no XSS)"
    expected: "If the model returns a response containing '<img onerror=alert(1)>' the raw HTML does NOT execute; it appears as escaped text or is stripped"
    why_human: "XSS sanitization in the browser requires visual inspection in a real DOM"
---

# Phase 4: Eino Agent Verification Report

**Phase Goal:** A user can ask an AI agent to read, summarize, rewrite, draft, and propose edits over a page, selection, attachment, or the whole workspace — and every write requires explicit human approval of a concrete diff. Read/write boundary enforced structurally (no direct writes, no secrets, no path escape, no shell, no Git push).
**Verified:** 2026-06-22T09:54:00Z
**Status:** human_needed
**Re-verification:** Yes — after gap closure (plan 04-07)

## Goal Achievement

All three previously-failed UI-wiring gaps (AGNT-02, AGNT-03/06, AGNT-07) are now structurally closed. 11/11 requirements have complete code paths. The residual `human_needed` items are the live-LLM perceptual checks that require a running server and a DeepSeek key — not blocked by any code gap.

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | User can ask about the current page (AGNT-01) | ⚠️ PRESENT_BEHAVIOR_UNVERIFIED | Backend: AskStream fully implemented (ScopePage, sr.Close(), ToolCallingModel). Frontend: subscribeAgentChat wired in AppShell.runAsk with effectiveScope. SSE incremental delivery requires live LLM. |
| 2 | User can ask about selected text (AGNT-02) | ✓ VERIFIED | agentContext store publishes live CM6 selection; AppShell reads selectionLength/selection; effectiveScope='selection' when non-empty; subscribeAgentChat called with scope:'selection' + selection text (AppShell.tsx:292-303). AppShell.test.tsx asserts AGNT-02 dispatch. 34/34 scoped tests green. |
| 3 | User can ask about a selected attachment (AGNT-03) | ✓ VERIFIED | AttachmentCard "Ask about this file" Sparkles button sets agentContext.setAttachment({id,name}) and opens panel (AttachmentCard.tsx:162-168). AppShell effectiveScope='attachment' when attachment in context; subscribeAgentChat called with scope:'attachment', attachment_id (AppShell.tsx:305). AppShell.test.tsx asserts AGNT-03 dispatch. |
| 4 | User can ask about the whole workspace via search-backed RAG with citations (AGNT-04) | ⚠️ PRESENT_BEHAVIOR_UNVERIFIED | Backend: ScopeWorkspace, runSearch, scopeTrace citations, event:citation SSE frame. Frontend: effectiveScope=workspace when toggle on or no page; citation links rendered in AgentAnswer. Requires live DeepSeek to verify RAG and citation runtime behavior. |
| 5 | User can summarize a page (AGNT-05) | ✓ VERIFIED | Backend: SummarizePage + handleSummarizePage + router wiring. AppShell case 'summarize' calls runSingleShot(()=>summarizePage(currentPath)) when no attachment in context. TestDispatch/summarize_page passes key-free. AppShell.test.tsx confirms AGNT-06 path does NOT call summarizePage when attachment present. |
| 6 | User can summarize an attachment (AGNT-06) | ✓ VERIFIED | Backend: SummarizeAttachment + handleSummarizeAttachment + router wired. AppShell case 'summarize' with attachment in context calls runSingleShot(()=>summarizeAttachment(attachment.id)) (AppShell.tsx:367). AppShell.test.tsx asserts AGNT-06 dispatch. |
| 7 | User can rewrite selected text and receive a proposal (AGNT-07) | ✓ VERIFIED | rewriteAvailable=hasSelection (no longer hardcoded false) in PromptBar.tsx:112. AppShell case 'rewrite' calls rewriteMutation.mutate({selection, instruction}) (AppShell.tsx:391); result routes to DiffReviewDialog (rewriteProposal state, same dialog, title "Review the rewrite"). applyRewriteMutation applies on Approve only, splices selection span, 409s if span gone. AppShell.test.tsx asserts rewrite→dialog path; applyPatch NOT called until explicit Approve. |
| 8 | User can draft a new page (AGNT-08) | ✓ VERIFIED | Backend + AppShell case 'draft' + TestDispatch/draft all green. Unchanged from prior verification. |
| 9 | Propose patch shown as real diff, never prose (AGNT-09) | ✓ VERIFIED | CR-01 fixed; body-only contract; DiffReviewDialog always renders ReactDiffViewer (4/4 trust tests pass). TestApplyPatchBodyOnlyRoundTrip PASS. Unchanged. |
| 10 | Every write requires explicit human approval (AGNT-10) | ✓ VERIFIED | Backend: handleApplyPatch non-tool; ErrStaleRevision→409; TestApplyStaleRevision PASS. Frontend: DiffReviewDialog Approve not auto-focused; stale blocks approve; rewrite also routes through the same dialog (not auto-applied). 4/4 trust tests green. |
| 11 | Structural read/write boundary: no direct write/shell/secret/path-escape (AGNT-11) | ✓ VERIFIED | TestToolSetIsExactlyReadOnlyAllowList PASS (exactly 5 tools, set-equality). No os.ReadFile anywhere in internal/agent/. APIKey read only via cfg.APIKey() in chatmodel.go, never logged. apply is a non-tool HTTP endpoint. TestApplyStaleRevision PASS. Unchanged. |

**Score:** 11/11 truths verified (8 VERIFIED, 3 PRESENT_BEHAVIOR_UNVERIFIED — code present and wired, live LLM required for behavioral confirmation)

### Re-verification: Gap Closure Summary

| Gap (prior) | Status | Evidence |
|-------------|--------|----------|
| AGNT-02 selection Ask — selection never captured, rewriteAvailable=false | CLOSED | agentContext.ts store (new); CM6 selectionListener in LivePreviewEditor.tsx publishes live selection on selectionSet; AppShell reads selectionLength/selection from store; effectiveScope='selection'; subscribeAgentChat passes scope:'selection'+text. |
| AGNT-03/06 attachment Ask/summarize — no attachment chip, not imported | CLOSED | AttachmentCard.tsx "Ask about this file" Sparkles button (reader-safe, not editor-gated); sets agentContext attachment + opens panel; AppShell case 'summarize' routes to summarizeAttachment when attachment context is set; Ask with attachment passes scope:'attachment'+attachment_id. |
| AGNT-07 rewrite — case 'rewrite' always refused, rewriteAvailable=false | CLOSED | rewriteAvailable=hasSelection (real selection length, no bare false); case 'rewrite' calls rewrite(selection, instruction), routes result to DiffReviewDialog (old=selection/new=rewrite); Approve splices and saves via applyPatch path; never auto-applies. |

### Required Artifacts

| Artifact | Status | Details |
|----------|--------|---------|
| `web/src/stores/agentContext.ts` | ✓ VERIFIED | NEW — ephemeral (non-persisted) zustand store: selection+selectionLength+attachment with setters; 8/8 unit tests pass |
| `web/src/stores/agentContext.test.ts` | ✓ VERIFIED | NEW — 8 tests: defaults, raw length, clear, setter independence, not-persisted (no localStorage key) |
| `web/src/components/LivePreviewEditor.tsx` | ✓ VERIFIED | selectionListener added to BOTH read-only and editable extension arrays; fires on selectionSet; clears on unmount |
| `web/src/components/PromptBar.tsx` | ✓ VERIFIED | rewriteAvailable=hasSelection (from selectionLength prop, no bare false); precedence chip chain (workspace→selection→attachment→page→default); TextSelect/Paperclip icons from lucide-react; stays presentational (no store import) |
| `web/src/components/PromptBar.test.tsx` | ✓ VERIFIED | +4 new tests: Rewrite enable/disable by selectionLength, Selection chip renders, attachment chip renders, workspace override wins — 9/9 total pass |
| `web/src/components/attachments/AttachmentCard.tsx` | ✓ VERIFIED | Sparkles ghost button (reader-safe); e.stopPropagation(); useAgentContext.getState().setAttachment({id,name}); useAgentPanel.getState().setOpen(true) |
| `web/src/routes/AppShell.tsx` | ✓ VERIFIED | Imports rewrite+summarizeAttachment; reads agentContext store; effectiveScope precedence (workspace→selection→attachment→page→workspace); case 'rewrite' uses rewriteMutation + DiffReviewDialog; case 'summarize' branches on attachment; runAsk passes scope/selection/attachment_id; one DiffReviewDialog drives both propose and rewrite |
| `web/src/routes/AppShell.test.tsx` | ✓ VERIFIED | +5 new dispatch tests (AGNT-07 rewrite→dialog/no-auto-apply, AGNT-02 selection Ask, AGNT-03 attachment Ask, AGNT-06 summarize-attachment, page summarize unchanged) — 7/7 total pass |
| `internal/agent/agent.go` | ✓ VERIFIED | Unchanged from prior verification |
| `internal/agent/chatmodel.go` | ✓ VERIFIED | Unchanged |
| `internal/agent/tools.go` | ✓ VERIFIED | Unchanged |
| `internal/server/handlers_agent.go` | ✓ VERIFIED | Unchanged — all endpoints (handleAgentChat, handleSummarizeAttachment, handleRewrite) remain correctly wired |
| `web/src/components/DiffReviewDialog.tsx` | ✓ VERIFIED | 4/4 trust contract tests still pass; unchanged |

### Key Link Verification

| From | To | Via | Status |
|------|----|-----|--------|
| `LivePreviewEditor.tsx` | `agentContext.ts` | EditorView.updateListener on selectionSet calls `useAgentContext.getState().setSelection/clearSelection` | ✓ WIRED |
| `AttachmentCard.tsx` | `agentContext.ts` | "Ask about this file" onClick calls `useAgentContext.getState().setAttachment({id,name})` | ✓ WIRED |
| `AppShell.tsx` | `agentContext.ts` | `useAgentContext((s)=>s.selection/selectionLength/attachment)` selector reads; effectiveScope derived from these | ✓ WIRED |
| `AppShell.tsx` | `client.ts rewrite()` | `case 'rewrite'` calls `rewriteMutation.mutate({selection, instruction})` → `rewrite(sel, instruction)` | ✓ WIRED |
| `AppShell.tsx` | `client.ts summarizeAttachment()` | `case 'summarize'` with attachment calls `runSingleShot(()=>summarizeAttachment(attachment.id))` | ✓ WIRED |
| `AppShell.tsx` | `client.ts subscribeAgentChat()` | `runAsk` passes `scope: effectiveScope`; scope='selection' also sends `selection`; scope='attachment' sends `attachment_id` | ✓ WIRED |
| `AppShell.tsx` (rewriteMutation success) | `DiffReviewDialog` | `setRewriteProposal({selection, rewritten})` drives `open={proposal!==null \|\| rewriteProposal!==null}` | ✓ WIRED |
| `DiffReviewDialog` (Approve, rewrite path) | `applyRewriteMutation` | `onApprove` checks `rewriteProposal` → `applyRewriteMutation.mutate(rewriteProposal)` | ✓ WIRED |
| `internal/agent/tools.go` | `internal/pages/service.go` | read_page closure calls deps.Pages.Get | ✓ WIRED |
| `internal/agent/tools.go` | `internal/search/` | search_pages/search_attachments call deps.Search.Query | ✓ WIRED |
| `internal/server/handlers_agent.go` | `internal/agent/agent.go` | h.agent.AskStream / ProposePatch / SummarizePage / SummarizeAttachment / Rewrite | ✓ WIRED |
| `web/src/api/client.ts` | `/api/v1/agent/apply-patch` | applyPatch via mutate(); 409 → err.status===409 | ✓ WIRED |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Go build (no backend touched) | `CGO_ENABLED=0 go build ./...` | exit 0 | ✓ PASS |
| All agent tests | `go test ./internal/agent/... -count=1` | ok (prior run 8.255s) | ✓ PASS (unchanged backend) |
| CR-01 body-only round-trip | `go test ./internal/pages/... -run TestApplyPatchBodyOnlyRoundTrip` | PASS | ✓ PASS (unchanged) |
| D8 stale revision | `go test ./internal/agent/... -run TestApplyStaleRevision` | PASS | ✓ PASS (unchanged) |
| D5 allow-list gate | `go test ./internal/agent/... -run TestToolSetIsExactlyReadOnlyAllowList` | PASS | ✓ PASS (unchanged) |
| agentContext store 8/8 | `npx vitest run src/stores/agentContext` | 8 passed | ✓ PASS |
| PromptBar 9/9 (incl. +4 new) | `npx vitest run src/components/PromptBar` | 9 passed | ✓ PASS |
| AppShell 7/7 (incl. +5 new dispatch) | `npx vitest run src/routes/AppShell` | 7 passed | ✓ PASS |
| DiffReviewDialog 4/4 trust | `npx vitest run src/components/DiffReviewDialog` | 4 passed | ✓ PASS |
| AgentPanel 6/6 | `npx vitest run src/components/AgentPanel` | 6 passed | ✓ PASS |
| Full scoped suite (5 suites) | `npx vitest run src/stores/agentContext src/components/PromptBar src/routes/AppShell src/components/AgentPanel src/components/DiffReviewDialog` | **34/34 passed** | ✓ PASS |
| TypeScript | `npx tsc --noEmit` | clean (no errors) | ✓ PASS |

### Requirements Coverage

| Requirement | Description | Status | Evidence |
|-------------|-------------|--------|----------|
| AGNT-01 | User can ask about current page | ⚠️ PRESENT_BEHAVIOR_UNVERIFIED | Backend+UI wired (confirmed unchanged); requires live LLM for SSE streaming confirmation |
| AGNT-02 | User can ask about selected text | ✓ VERIFIED | agentContext store + CM6 selectionListener + AppShell effectiveScope='selection' + subscribeAgentChat(scope:'selection',selection) — AppShell.test.tsx asserts dispatch |
| AGNT-03 | User can ask about a selected attachment | ✓ VERIFIED | AttachmentCard "Ask about this file" → agentContext.setAttachment → AppShell effectiveScope='attachment' → subscribeAgentChat(scope:'attachment',attachment_id) — AppShell.test.tsx asserts dispatch |
| AGNT-04 | User can ask about whole workspace (RAG + citations) | ⚠️ PRESENT_BEHAVIOR_UNVERIFIED | Backend+UI wired (confirmed unchanged); requires live LLM for RAG and citation confirmation |
| AGNT-05 | User can summarize a page | ✓ VERIFIED | Unchanged from prior; AppShell.test.tsx confirms page summarize still fires when no attachment context |
| AGNT-06 | User can summarize an attachment | ✓ VERIFIED | AppShell case 'summarize' with attachment calls summarizeAttachment(attachment.id) — AppShell.test.tsx asserts dispatch |
| AGNT-07 | User can rewrite selected text (proposal, never auto-applies) | ✓ VERIFIED | rewriteAvailable=hasSelection; case 'rewrite' → rewriteMutation → DiffReviewDialog; applyRewriteMutation on Approve only; AppShell.test.tsx asserts dialog opens and applyPatch not called until Approve |
| AGNT-08 | User can draft a new page | ✓ VERIFIED | Unchanged from prior |
| AGNT-09 | Propose patch shown as diff (never prose) | ✓ VERIFIED | Unchanged from prior; 4/4 trust tests pass |
| AGNT-10 | Explicit approval required before apply (propose + rewrite) | ✓ VERIFIED | Both propose and rewrite flow through DiffReviewDialog; rewrite adds applyRewriteMutation which is only called on explicit onApprove; stale 409s into dialog stale state |
| AGNT-11 | Structural read/write boundary | ✓ VERIFIED | Unchanged from prior; no new write paths introduced; rewrite uses existing applyPatch endpoint (editor+CSRF gated) |

### Anti-Patterns Found

| File | Pattern | Severity | Impact |
|------|---------|----------|--------|
| `AppShell.tsx` comment line 357 | Comment still references "WR-01" deferral (old) in a JSDoc | ℹ️ Info | Copy artifact from before the gap was closed; the actual code correctly dispatches rewrite. No behavior impact. |
| No TBD/FIXME/XXX without issue references | — | — | Scan clean on all 04-07 modified files. The original WR-02 NOTE comment in tools.go (retrieval not role-scoped at MVP) is unchanged and is a NOTE, not a TBD/FIXME/XXX. |

### Human Verification Required

#### 1. Ask streams token-by-token into the panel (AGNT-01)

**Test:** With `DEEPSEEK_API_KEY` set and the server running, open a page, type a question in the PromptBar, submit with Enter.
**Expected:** PromptBar shows Thinking… then Streaming…; AgentPanel auto-opens; answer tokens accumulate progressively; status returns to idle on completion; no silent hang.
**Why human:** SSE incremental delivery requires a live LLM and a browser — cannot be asserted with grep.

#### 2. Selection Ask scopes the answer (AGNT-02)

**Test:** Select a paragraph of text in a page; verify the PromptBar chip changes to "Selection (N chars)"; type a question and submit in Ask mode.
**Expected:** AgentPanel answer is noticeably scoped to the selected passage; the model references the selection content specifically rather than the whole page.
**Why human:** Selection-scoped answer quality requires live DeepSeek and perceptual judgment.

#### 3. Workspace Ask cites retrieved pages (AGNT-04)

**Test:** Toggle "Whole workspace", ask a question spanning multiple indexed pages.
**Expected:** AgentAnswer shows a "Reasoned over: [page-a], [page-b]" citation row with linked paths; answer is grounded in indexed content; not a workspace dump.
**Why human:** Requires live DeepSeek and Bleve-indexed content; tool-call trace and citation emission are runtime-only.

#### 4. Summarize page returns grounded summary (AGNT-05)

**Test:** Open a page with substantial content; select Summarize mode; submit.
**Expected:** AgentPanel shows a concise, page-grounded summary; no hallucinated content from outside the page; Thinking… status during generation.
**Why human:** Summary quality and groundedness require live LLM + real page content.

#### 5. Attachment Ask + Summarize (AGNT-03/06)

**Test:** Click the Sparkles "Ask about this file" button on a PDF or DOCX attachment card (any role — readers included). Verify the PromptBar chip changes to the filename. Submit a question.
**Expected:** (Ask) AgentPanel answer is grounded in the file's extracted text. (Summarize) Switch mode to Summarize, submit — panel returns a summary of the file content.
**Why human:** Requires live server + DeepSeek + extracted-text content; answer groundedness is perceptual.

#### 6. Rewrite selection → DiffReviewDialog → Approve applies change (AGNT-07)

**Test:** Open a page as editor; select text; verify Rewrite option becomes enabled; submit a rewrite instruction.
**Expected:** DiffReviewDialog opens with old=selection/new=rewrite (real diff rendered, not prose); Reject discards; Approve replaces the selection span in the page and saves; page view refreshes with the rewritten content.
**Why human:** Requires live server + DeepSeek + editor role; diff quality and post-Approve page state require a running browser.

#### 7. Propose patch → DiffReviewDialog → Approve applies change (AGNT-09/10)

**Test:** Open a page as editor; select "Propose a patch"; describe a small change; DiffReviewDialog opens.
**Expected:** (a) Real diff rendered; (b) initial focus on Reject not Approve; (c) Approve applies and page view refreshes; (d) `git log` shows Action=approved_agent_patch; (e) Reject discards with no write.
**Why human:** Requires live DeepSeek, editor role, and git commit inspection.

#### 8. Stale revision 409 in the browser (AGNT-10)

**Test:** Open DiffReviewDialog for a propose or rewrite; in a second browser tab, edit and save the same page; switch back and click Approve.
**Expected:** DiffReviewDialog replaces Approve with the stale warning banner; no write occurs.
**Why human:** Concurrent-edit race requires two browser sessions and a live server.

#### 9. Agent-off / unreachable disables PromptBar

**Test:** Set `agent.enabled: false` in config.yaml, restart server, reload the app.
**Expected:** PromptBar renders with an inline explanation note; submit button disabled; no hang on any interaction.
**Why human:** Requires config change and server restart.

#### 10. Streamed answers sanitized (no stored XSS)

**Test:** If the model returns a response containing raw HTML like `<img onerror="alert(1)">`, it must not execute.
**Expected:** The img tag is stripped or rendered as escaped text; no alert fires.
**Why human:** XSS sanitization relies on browser DOM parsing and runtime behavior of rehype-sanitize.

## Gaps Summary

No code gaps remain. All 11 AGNT requirements are structurally implemented and covered by automated tests. The three previously-failed requirements are now verified:

- **AGNT-02** closed by: `agentContext.ts` store + `LivePreviewEditor.tsx` CM6 selectionListener + AppShell effectiveScope='selection' + subscribeAgentChat scope wiring.
- **AGNT-03/06** closed by: `AttachmentCard.tsx` "Ask about this file" affordance + agentContext attachment channel + AppShell summarize/Ask attachment dispatch.
- **AGNT-07** closed by: `PromptBar.tsx` real `rewriteAvailable` + AppShell `rewriteMutation` + shared `DiffReviewDialog` with old=selection/new=rewrite + apply-on-Approve-only contract.

The 10 human verification items above are live-LLM perceptual checks requiring a running server and a DeepSeek key. They are not blocked by any code deficiency.

---

_Verified: 2026-06-22T09:54:00Z_
_Verifier: Claude (gsd-verifier)_
_Re-verification: gap closure plan 04-07_
