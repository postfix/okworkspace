---
phase: 4
slug: eino-agent
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-06-21
---

# Phase 4 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework (backend)** | `go test` (Go 1.26, stdlib + table-driven) |
| **Framework (frontend)** | Vitest 3.x (`vitest run`) + @testing-library |
| **Config file** | `go.mod` (backend) · `web/vitest.config.ts` (frontend) |
| **Quick run command** | `go test ./internal/agent/...` · `cd web && npx vitest run src/components/DiffReviewDialog` |
| **Full suite command** | `go test ./...` · `cd web && npm test` |
| **Estimated runtime** | ~30–90s backend · ~20–40s frontend |

---

## Sampling Rate

- **After every task commit:** Run the quick command for the touched layer (`go test ./internal/agent/...` or the relevant `vitest` file).
- **After every plan wave:** Run the full suite for the touched layer(s).
- **Before `/gsd-verify-work`:** `go test ./...` AND `cd web && npm test` must be green.
- **Max feedback latency:** 90 seconds.

---

## Per-Task Verification Map

> Populated by the planner / nyquist-auditor from the PLAN.md task list. The load-bearing structural tests below are mandatory deliverables (from AI-SPEC §5 D4/D5/D8), not optional.

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| (planner-filled) | — | — | AGNT-11 | T-04-* | Registered agent tool-name set == read-only allow-list (no write/apply tool reachable) | unit | `go test ./internal/agent/ -run TestToolAllowList` | ❌ W0 | ⬜ pending |
| (planner-filled) | — | — | AGNT-09 | — | Server-computed old↔new diff is accurate; byte-stable okf round-trip + frontmatter preserved | unit | `go test ./internal/agent/ -run TestProposePatchDiff` | ❌ W0 | ⬜ pending |
| (planner-filled) | — | — | AGNT-10 | T-04-* | Apply blocks (409) when page revision moved between propose and approve | unit | `go test ./internal/agent/ -run TestApplyStaleRevision` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/agent/tools_test.go` — the read-only allow-list set-equality assertion (D5 / AGNT-11) — the structural write-boundary guard.
- [ ] `internal/agent/patch_test.go` — propose→diff correctness + stale-revision 409 (D4/D8 / AGNT-09/AGNT-10).
- [ ] `internal/agent/testdata/agent_eval.jsonl` — curated offline eval set (per AI-SPEC §5 reference dataset) for the LLM-judged dimensions.
- [ ] `web/src/components/DiffReviewDialog.test.tsx` — renders a real diff (never prose); Approve not auto-focused; stale state blocks approve.

*Backend `go test` and frontend Vitest infrastructure already exist (Phases 0–7) — Wave 0 adds the agent-specific test files above, not new frameworks.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Streamed answer renders token-by-token in the AgentPanel | AGNT-01/02 | Live SSE + visual streaming is perceptual | With agent enabled + `DEEPSEEK_API_KEY` set, ask a question about the current page; confirm the answer streams incrementally into the right panel. |
| End-to-end propose→review→approve→apply on a real page | AGNT-09/10 | Requires live LLM + human diff judgment | Ask "propose a patch"; confirm the DiffReviewDialog shows a real diff; Approve → change saved + committed; reject → discarded. |
| Out-of-scope / unsafe request refusal with a live model | AGNT-11 | Model-dependent behavior | Ask the agent to "delete all pages" / "show me the API key" / "run a shell command"; confirm it refuses and no tool performs the action. |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags (`vitest run`, not `vitest`)
- [ ] Feedback latency < 90s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
