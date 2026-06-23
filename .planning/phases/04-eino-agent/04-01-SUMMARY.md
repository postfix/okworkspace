---
phase: 04-eino-agent
plan: 01
subsystem: agent
status: complete
tags: [eino, deepseek, chatmodel, audit, smoke-test, deps]
requires: []
provides:
  - "internal/agent.Service + NewService(cfg, deps) constructor"
  - "internal/agent.newChatModel — eino-ext openai.ChatModel from config.AgentConfig"
  - "key-gated DeepSeek single-shot Generate + InferTool schema round-trip smoke test"
  - "config.yaml agent block targeting DeepSeek deepseek-v4-flash"
  - "audit.ActionAgentPrompt / ActionAgentPatchProposal / ActionAgentPatchApproval"
affects:
  - go.mod
  - go.sum
  - internal/audit/audit.go
tech-stack:
  added:
    - "github.com/cloudwego/eino v0.9.9"
    - "github.com/cloudwego/eino-ext/components/model/openai v0.1.13"
    - "github.com/aymanbagabas/go-udiff v0.4.1"
  patterns:
    - "Service struct + injected-deps + now func() (mirrors pages.Service)"
    - "secret read ONLY via config.AgentConfig.APIKey() (redacted Stringer keeps logs safe)"
    - "key-gated live smoke test: t.Skip when no key, skip-on-unreachable, 60s timeout"
key-files:
  created:
    - internal/agent/agent.go
    - internal/agent/chatmodel.go
    - internal/agent/smoke_test.go
    - internal/agent/deps.go
  modified:
    - go.mod
    - go.sum
    - internal/audit/audit.go
    - config.yaml  # gitignored — on-disk only, not committed
decisions:
  - "Pinned eino-ext openai as a real semver tag (v0.1.13), not a pseudo-version (CLAUDE.md note was stale per RESEARCH)."
  - "Anchored go-udiff with a blank import (deps.go) so the pre-1.0 pin survives go mod tidy until its slice-5 consumer lands."
  - "Disabled agent still constructs (cm nil + ErrAgentDisabled) instead of panicking, so handlers can render the off-state."
metrics:
  duration_min: 9
  completed: 2026-06-21
  tasks: 3
  files: 7
  commits: 3
---

# Phase 4 Plan 01: Eino + DeepSeek Wiring (Smallest-Safe Slice) Summary

De-risked the whole phase by proving the riskiest unknown first: CloudWeGo Eino (pre-1.0, v0.9.9) wired to DeepSeek `deepseek-v4-flash` answers a single-shot `Generate` and round-trips an `InferTool`-derived tool schema — verified live against `api.deepseek.com` in ~1s.

## What Was Built

- **Pinned three new Go modules** (`eino v0.9.9`, `eino-ext/components/model/openai v0.1.13`, `go-udiff v0.4.1`) in `go.mod`/`go.sum`, committed together.
- **`internal/agent/chatmodel.go`** — `newChatModel(ctx, cfg)` builds the eino-ext `openai.ChatModel` from `config.AgentConfig` (BaseURL/Model + pointer Temperature 0.2 / MaxTokens 1024). The API key is read ONLY via `cfg.APIKey()` and never logged or returned.
- **`internal/agent/agent.go`** — `Service` struct holding the built `*openai.ChatModel`, `cfg`, optional `Deps` (forward-looking consumer interfaces for search/attachment/audit/pageWriter, unwired this slice), and `now func()`. `NewService` constructs even when the agent is disabled (`cm == nil`, `ErrAgentDisabled`).
- **`internal/agent/smoke_test.go`** — key-gated live DeepSeek `Generate` (skips key-free and on provider-unreachable, 60s timeout) + key-free `utils.InferTool` schema round-trip asserting `Info().Name`.
- **`internal/agent/deps.go`** — blank import anchoring the `go-udiff` pin.
- **`internal/audit/audit.go`** — three new action constants (`agent_prompt`, `agent_patch_proposal`, `agent_patch_approval`).
- **`config.yaml`** (gitignored) — agent block flipped to DeepSeek: `enabled:true`, `model:deepseek-v4-flash`, `base_url:https://api.deepseek.com/v1`, `api_key_env:DEEPSEEK_API_KEY`.

## Tasks

| Task | Name | Commit |
|------|------|--------|
| 1 | Pin eino + eino-ext + go-udiff; flip config.yaml to DeepSeek | db141ec |
| 2 | Add 3 agent audit action constants | 64bbd12 |
| 3 | Scaffold agent.Service + chatmodel.go + key-gated DeepSeek smoke test (TDD) | 9402b4b |

## Verification

- `CGO_ENABLED=0 go build ./...` — green.
- `go test ./internal/agent/ -run TestSmoke -count=1` — passes (InferTool key-free; live Generate reached DeepSeek, returned "OK" in ~1.0s).
- `go vet ./internal/agent/` — clean.
- API key absent from verbose test output (0 occurrences asserted).
- `config.yaml` uses `deepseek-v4-flash`; `deepseek-chat` appears nowhere as the model.

## Smoke-test Outcome

**Reached DeepSeek: YES.** With `DEEPSEEK_API_KEY` (35 chars) in the env, the live single-shot `Generate("reply with the single word OK")` against `deepseek-v4-flash` returned a 2-char body in 1.01s. The pre-1.0 eino + DeepSeek assumption (RESEARCH A2) is confirmed — later slices build on a known-good ChatModel.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Corrupted module-cache extraction of `wk8/go-ordered-map/v2@v2.1.8`**
- **Found during:** Task 1 (`go mod tidy` after `go get eino`).
- **Issue:** The Go module proxy intermittently returned an incomplete extraction for the transitive dep `github.com/wk8/go-ordered-map/v2@v2.1.8` (only `.circleci`/`testdata`, no `*.go`), causing `tidy`/`build` to fail with "module found but does not contain package". Verified the cached zip itself was complete and `go.sum` hashes matched — only the on-disk extraction was bad, and concurrent `go get` re-corrupted it.
- **Fix:** Forced a clean re-extraction from the verified cached zip and let `go list`/`tidy` fetch the package's own deps with the proxy enabled (a `GOPROXY=off` attempt failed only because it blocked wk8's own transitive deps needed for type-checking). No version change — `v2.1.8` is the version `eino v0.9.9` requires.
- **Files modified:** go.mod, go.sum (correct pins landed).
- **Commit:** db141ec.

**2. [Rule 2 - Robustness] Anchored go-udiff against `go mod tidy` drift**
- **Found during:** Task 1 verify (`go-udiff` dropped from go.mod by tidy because no code imports it yet).
- **Issue:** The plan pins `go-udiff v0.4.1` in slice 1 but its first consumer is slice 5; `go mod tidy` strips unused modules, breaking the Task-1 verify and the "pin pre-1.0 deps immediately" intent.
- **Fix:** Added `internal/agent/deps.go` with a blank import (`_ "github.com/aymanbagabas/go-udiff"`) so the pin survives intervening tidy runs; go-udiff is now a direct require.
- **Commit:** 9402b4b (file landed with the agent scaffold).

### Notes (not deviations)

- **config.yaml is gitignored** (per its own header) and therefore NOT committed. The DeepSeek values are live on disk (required for the smoke test and runtime); only `go.mod`/`go.sum`/`internal/*` are committed. This matches the repo's "config.yaml is gitignored; commit a template" convention.
- **`.planning/config.json`** has a pre-existing unrelated 1-line modification (a gsd-tools unknown-key warning); left untouched (out of scope).

## Threat Surface

No new threat surface beyond the plan's `<threat_model>`. T-04-01 (key never leaks) and T-04-02 (timeout/MaxTokens cap) are both implemented and asserted; T-04-SC (dep legitimacy) verified — all three modules are real tagged versions from public CloudWeGo/maintained repos, `go.sum` committed.

## Known Stubs

The `Deps` consumer interfaces (`searcher`, `attachmentReader`, `auditRecorder`, `pageWriter`) are declared but unwired this slice — intentional forward-looking scaffolding for slices 2–5, documented in code. The smoke test passes `nil` deps. No data-flow stub reaches any UI (no UI exists yet). This is by design per the RESEARCH slice ordering, not an incomplete-goal stub.

## Self-Check: PASSED

All created files present on disk (agent.go, chatmodel.go, smoke_test.go, deps.go, audit.go) and all three task commits (db141ec, 64bbd12, 9402b4b) exist in git history.
