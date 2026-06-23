---
phase: 04-eino-agent
plan: 07
subsystem: ui
tags: [react, zustand, codemirror, agent, selection, attachment, rewrite, diff]

# Dependency graph
requires:
  - phase: 04-06
    provides: "CM6 LivePreviewEditor (selection/copy in read+edit), DiffReviewDialog trust gate, AppShell agent session, client.ts rewrite/summarizeAttachment/subscribeAgentChat with selection+attachment fields"
provides:
  - "agentContext zustand store (ephemeral selection + attachment channel across the route->shell boundary)"
  - "CM6 selection capture published live to the shell on selectionSet"
  - "PromptBar real rewriteAvailable + selection/attachment context chips"
  - "AttachmentCard 'Ask about this file' affordance (reader-safe)"
  - "AppShell dispatch: rewrite->DiffReviewDialog, selection Ask, attachment Ask, summarize-attachment"
  - "AGNT-02 / AGNT-03 / AGNT-06 / AGNT-07 reachable through the UI"
affects: [phase-05, agent-ui, frontend]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Ephemeral (non-persisted) zustand store as the route->shell channel for transient content (vs. persisted UI-preference stores)"
    - "CM6 EditorView.updateListener publishing selection imperatively via getState() (no React subscription inside the view)"
    - "One DiffReviewDialog, two drivers (propose patch + rewrite) — agent writes never auto-apply"

key-files:
  created:
    - web/src/stores/agentContext.ts
    - web/src/stores/agentContext.test.ts
  modified:
    - web/src/components/LivePreviewEditor.tsx
    - web/src/components/PromptBar.tsx
    - web/src/components/PromptBar.test.tsx
    - web/src/components/attachments/AttachmentCard.tsx
    - web/src/components/attachments/AttachmentCard.css
    - web/src/routes/AppShell.tsx
    - web/src/routes/AppShell.test.tsx

key-decisions:
  - "agentContext store is deliberately NOT persisted — selection/attachment are transient content, not a UI preference; a reload must not scope the next prompt to stale text"
  - "Selection is published via a separate CM6 updateListener, never through onChange (onChange stays the verbatim-bytes document channel — EDIT-03)"
  - "Rewrite apply replaces the captured selection span in the cached page body and reuses the existing applyPatch path — no new write endpoint; a missing span / stale revision 409s into the dialog's stale state"
  - "PromptBar stays presentational (selectionLength + attachmentName props); AppShell owns the agentContext store reads"

patterns-established:
  - "Effective-scope precedence (workspace -> selection -> attachment -> page -> default) shared by the AppShell dispatch and the PromptBar chip"
  - "Reader-safe agent affordances (Ask/Summarize not gated on canEdit; only Propose/Rewrite-apply touch the write path)"

requirements-completed: [AGNT-02, AGNT-03, AGNT-06, AGNT-07]

# Metrics
duration: 22min
completed: 2026-06-22
status: complete
---

# Phase 4 Plan 07: Agent UI gap closure Summary

**Frontend-only wiring that makes AGNT-02/03/06/07 reachable: a new ephemeral agentContext store carries the live CM6 selection and the chosen attachment up to the AppShell agent session, which now dispatches rewrite through DiffReviewDialog (never auto-applies), selection-scope Ask, attachment-scope Ask, and summarize-attachment.**

## Performance

- **Duration:** ~22 min
- **Completed:** 2026-06-22
- **Tasks:** 3 (all TDD: test -> implement -> verify, committed atomically)
- **Files modified:** 9 (2 created, 7 modified)

## Accomplishments

- **agentContext store** — the one new cross-component channel. Ephemeral (non-persisted) zustand store holding `selection` (verbatim text), `selectionLength` (raw char count), and `attachment` ({id, name}); setters are independent (selection and attachment coexist).
- **CM6 selection capture** — LivePreviewEditor publishes the live selection on `selectionSet` via an `EditorView.updateListener` added to BOTH the read-only and editable surfaces, and clears it on unmount (covers PageEditor + PageView). The verbatim-bytes `onChange` document channel is untouched (EDIT-03).
- **PromptBar** — `rewriteAvailable` is now driven by real `selectionLength` (no more hardcoded `false`); a precedence chip chain (workspace -> selection -> attachment -> page -> default) renders `Selection (N chars)` / the attachment filename with TextSelect / Paperclip icons. Stays presentational (no store import).
- **AttachmentCard** — an "Ask about this file" Sparkles ghost button sets the attachment agent context and opens the panel; reader-safe (not gated on canEdit); stops propagation so it never downloads/previews.
- **AppShell dispatch** — `case 'rewrite'` calls `rewrite(selection, instruction)` and routes the result into the SAME DiffReviewDialog (old = selection, new = rewrite); Approve replaces the selection span in the cached page body and saves via the existing `applyPatch` path; it NEVER auto-applies. Selection-scope and attachment-scope Ask pass the captured selection text / attachment_id to `subscribeAgentChat`. Summarize routes to `summarizeAttachment(id)` when an attachment is in context (AGNT-05 page summarize unchanged).

## Task Commits

Each task was committed atomically (TDD test + implementation together per task):

1. **Task 1: agentContext store + CM6 selection capture** - `9aa204b` (feat)
2. **Task 2: real rewriteAvailable + selection/attachment chips; Ask-about-this-file** - `a82db18` (feat)
3. **Task 3: AppShell dispatch — rewrite->diff, selection Ask, attachment Ask/summarize** - `b8c2d8e` (feat)

## Files Created/Modified

- `web/src/stores/agentContext.ts` - Ephemeral zustand store: live selection + chosen attachment, the route->shell channel.
- `web/src/stores/agentContext.test.ts` - 8 tests: defaults, setSelection raw length, clear, setter independence, not-persisted.
- `web/src/components/LivePreviewEditor.tsx` - selectionSet updateListener publishing selection in both surfaces; clears on unmount.
- `web/src/components/PromptBar.tsx` - real rewriteAvailable; selectionLength + attachmentName props; precedence chip chain (TextSelect / Paperclip).
- `web/src/components/PromptBar.test.tsx` - +4 tests: Rewrite enable/disable by selection, Selection chip, attachment chip, workspace override.
- `web/src/components/attachments/AttachmentCard.tsx` - "Ask about this file" button (sets attachment context, opens panel, reader-safe).
- `web/src/components/attachments/AttachmentCard.css` - token-only `.attachment-card-ask` rule.
- `web/src/routes/AppShell.tsx` - agentContext reads; effective-scope precedence; rewrite->diff + apply; selection/attachment Ask; summarize-attachment.
- `web/src/routes/AppShell.test.tsx` - +5 tests: rewrite->dialog (no applyPatch), selection Ask scope, attachment Ask scope, summarize-attachment vs summarize-page.

## Decisions Made

- **agentContext is NOT persisted** — unlike agentPanel/editorMode (which persist a UI preference), this carries transient content; persisting it would silently scope the next prompt to text the user is no longer viewing.
- **Selection via a separate updateListener, not onChange** — onChange must remain the byte-stable document channel (EDIT-03); a selection is not an edit.
- **Rewrite apply reuses applyPatch** — the rewrite span is spliced into the cached page body and saved through the existing editor+CSRF apply path; no new write endpoint was introduced. If the original selection is no longer found in the body (it changed under the user), the apply 409s into the dialog's stale state rather than writing a guessed span.
- **One DiffReviewDialog, two drivers** — propose patch and rewrite share the single mounted dialog (title/old/new switch on which proposal is active); the trust contract (real diff, Approve not auto-focused, stale removes Approve, never auto-applies) is preserved exactly.

## Deviations from Plan

None - plan executed exactly as written. PageEditor.tsx / PageView.tsx required no changes (the single LivePreviewEditor unmount cleanup covers both routes, as the plan anticipated), and they were not touched. AttachmentsSection.tsx needed no new prop (the store-direct approach the plan preferred).

## Issues Encountered

None. The benign `act(...)` warnings in the AppShell async-dispatch tests are React test-environment noise (async state settles after the assertion); all 7 AppShell tests pass.

## Verification

- `cd web && npx vitest run src/routes/AppShell src/components/PromptBar src/components/AgentPanel src/components/DiffReviewDialog src/stores/agentContext` — **34/34 green** across 5 suites (incl. DiffReviewDialog 4/4 trust tests).
- `cd web && npx tsc --noEmit` — **clean**.
- `CGO_ENABLED=0 go build ./...` — **green** (no backend touched, no API contract change).
- Grep gates: `useAgentContext` present in agentContext.ts / LivePreviewEditor.tsx / AppShell.tsx / AttachmentCard.tsx; `selectionSet` in LivePreviewEditor.tsx; `rewrite(` + `summarizeAttachment(` called in AppShell.tsx; no bare `rewriteAvailable = false` remains in PromptBar.tsx.
- Trust gate intact: rewrite opens DiffReviewDialog and `applyPatch` is NOT called until explicit Approve (asserted in AppShell.test.tsx).

## User Setup Required

None - no external service configuration required. (Live agent behavior still needs `DEEPSEEK_API_KEY` + a running server to exercise end-to-end, per 04-VERIFICATION human-verification items — unchanged by this plan.)

## Next Phase Readiness

- All three 04-VERIFICATION frontend gaps (AGNT-02 selection Ask, AGNT-03/06 attachment Ask + summarize, AGNT-07 rewrite) are now reachable through the UI.
- Remaining Phase 4 verification items are the live-LLM behavioral checks (SSE streaming, RAG citations, summary groundedness, propose->approve->apply, stale 409, XSS sanitization) that require a running server + DeepSeek key — not blocked by code.

## Self-Check: PASSED

- Files verified on disk: `web/src/stores/agentContext.ts`, `web/src/stores/agentContext.test.ts`, `.planning/phases/04-eino-agent/04-07-SUMMARY.md`.
- Commits verified in git log: `9aa204b`, `a82db18`, `b8c2d8e`.

---
*Phase: 04-eino-agent*
*Completed: 2026-06-22*
