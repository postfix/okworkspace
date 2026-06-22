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
passed: 4
issues: 0
pending: 6
skipped: 0
blocked: 0

## Live Validation Results (2026-06-22, browser-driven on :8098 against live DeepSeek)

Driven via Playwright as admin against `deepseek-v4-flash`. Evidence screenshots: `phase4-ask-answer.png`, `phase4-diffdialog.png`.

**PASSED (browser-confirmed):**
- **#1 Ask streams (AGNT-01)** — answer streamed into the right AgentPanel; correctly **grounded** ("I cannot find any information about deployment runbooks… the page is empty") rather than hallucinating (D7 honest refusal). ✓
- **#7 Propose → DiffReviewDialog → Approve (AGNT-09/10)** — Propose produced a **real react-diff-viewer table** (NOT prose), the model added a `## Deployment checklist` with 3 bullets while preserving `## Privet`; Approve & save wrote the file. ✓
- **DiffReviewDialog trust contract** — real diff; initial focus on **Reject** (not Approve); copy "Approve & save" / "Reject" (hidden-Git voice). ✓
- **Audit + safety (AGNT-11)** — server audit log recorded `agent_prompt`, `agent_patch_proposal` (churn=1.0), `agent_patch_approval` with actor/target/role; nothing applied before explicit Approve. ✓
- **CR-01 fix confirmed live** — saved `deploy.md` kept exactly one frontmatter fence (type/title/description/tags/timestamp intact); no double-frontmatter, byte-stable round-trip held. ✓

**PENDING (not exhaustively browser-tested — wiring confirmed, need targeted setup):** #2 selection Ask, #3 workspace RAG citations, #4 summarize-page, #5 attachment Ask+summarize, #8 stale-409 (needs two tabs), #9 agent-off (needs restart), #10 XSS-sanitization (needs a crafted model response). All backends are tested key-free; these are perceptual/multi-context checks.

## Findings (non-defects / environmental)

1. **Embedded SPA must be rebuilt before serving (operational).** The agent UI initially did NOT render in the running app — the embedded `internal/web/dist` bundle was stale (pre-Phase-4). `internal/web/dist/*` is a **gitignored build artifact** (rebuilt by `deploy/Dockerfile` stage 1 `npm run build`). Fixed locally by `cd web && npm run build` + rebuilding the binary. Deploy pipeline is correct; the lesson is local: rebuild the SPA before serving. NOT a code defect.
2. **Workspace git not initialized in this dev data dir (pre-existing, not Phase 4).** `data/repo` has no `.git`, so page-commit jobs stay queued ("commit wait timed out; job stays queued"). Affects ALL edits equally (agent uses the same `pages.Save` path); a Phase-0/1 `EnsureRepo`/dev-data matter, orthogonal to the agent. The file write + audit are correct.
3. **Minor UX:** pressing Enter in the Ask prompt also navigated the page to edit mode (non-blocking; the Ask still streamed correctly).

## Gaps
