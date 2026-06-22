---
status: testing
phase: 04-eino-agent
source: [04-VERIFICATION.md]
started: 2026-06-22T09:54:00Z
updated: 2026-06-22T09:54:00Z
---

## Current Test

number: 1
name: Ask streams token-by-token (AGNT-01, page scope)
expected: |
  With DEEPSEEK_API_KEY set and the app running, POST /agent/chat (Ask, page scope)
  delivers SSE data: frames progressively into the AgentPanel; status cycles
  Thinking… → Streaming… → idle; no silent hang.
awaiting: user response

## Tests

### 1. Ask streams token-by-token (AGNT-01, page scope)
expected: SSE frames arrive progressively in AgentPanel; status Thinking→Streaming→idle; no hang.
result: [pending]

### 2. Selection Ask scoped to selection (AGNT-02)
expected: Select text → PromptBar chip "Selection (N chars)"; Ask answer references the selected passage.
result: [pending]

### 3. Workspace Ask cites retrieved pages (AGNT-04)
expected: Whole-workspace toggle → cross-page question → "Reasoned over: [page-a], [page-b]" citations; grounded answer, not a dump.
result: [pending]

### 4. Summarize page returns a grounded summary (AGNT-05)
expected: Summarize mode with a page open → concise grounded summary, not a hallucination.
result: [pending]

### 5. Attachment Ask + summarize (AGNT-03/06)
expected: "Ask about this file" Sparkles on an attachment card → chip shows filename → grounded answer; Summarize mode returns a file summary.
result: [pending]

### 6. Rewrite selection → DiffReviewDialog → Approve (AGNT-07)
expected: Select text → Rewrite enabled → submit instruction → DiffReviewDialog opens (real diff old=selection/new=rewrite); Reject discards; Approve replaces the span + saves; view refreshes.
result: [pending]

### 7. Propose a patch → DiffReviewDialog → Approve (AGNT-09/10)
expected: Propose mode → one-line change → DiffReviewDialog real diff; Approve saves + view refreshes; git log shows Action=approved_agent_patch.
result: [pending]

### 8. Stale-revision 409 in the browser (AGNT-10)
expected: Open the dialog from propose/rewrite; edit+save the same page in another tab; click Approve → stale warning replaces Approve with Re-run/Close.
result: [pending]

### 9. Agent-off / unreachable disables PromptBar
expected: agent.enabled:false + reload → PromptBar shows disabled note, submit disabled, no hang.
result: [pending]

### 10. Streamed answers render through sanitized MarkdownProse (no XSS)
expected: A model response containing `<img onerror=alert(1)>` does NOT execute; appears escaped/stripped.
result: [pending]

## Summary

total: 10
passed: 0
issues: 0
pending: 10
skipped: 0
blocked: 0

## Gaps
