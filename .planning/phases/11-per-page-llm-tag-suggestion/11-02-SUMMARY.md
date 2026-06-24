---
phase: 11-per-page-llm-tag-suggestion
plan: 02
subsystem: backend
status: complete
tags: [agent, eino, single-shot, validate-and-retry, tagging, csrf, optimistic-concurrency, byte-stability]
requires:
  - "okf.SetTags(d *okf.Doc, tags []string) — byte-stable block-style tags editor (11-01)"
  - "(*graph.Store).Vocabulary(ctx) ([]string, error) — distinct existing tag vocabulary (11-01)"
provides:
  - "agent.SuggestTags(ctx, path) ([]tags, []existing, baseRev, err) — single-shot vocab-biased capped tag-suggestion MODE"
  - "agent.MaxSuggestedTags constant (=5) + agent.ErrTagsInvalid sentinel"
  - "agent.ValidateTags(raw, vocab) — exported normalize/cap/dedupe/reject + existing flags (server re-validation gate)"
  - "POST /agent/suggest-tags (any-authed read) — suggestions + base_revision + per-tag existing flags"
  - "POST /agent/apply-tags (editor + CSRF) — server-side re-validate/normalize → okf.SetTags → pages.Save, 409 on stale"
affects:
  - internal/agent/agent.go
  - internal/agent/prompts.go
  - internal/agent/suggesttags.go
  - internal/server/handlers_agent.go
  - internal/server/router.go
  - cmd/okf-workspace/main.go
tech-stack:
  added: []
  patterns:
    - "single-shot ChatModel.Generate MODE mirroring Rewrite/Draft (NOT a 6th Eino tool)"
    - "provider-agnostic validate-and-retry (JSON array of strings parsed, NOT response_format) — 1 + 2 retries, structured ErrTagsInvalid on exhaustion"
    - "narrow one-method consumer interface (vocabularyReader) satisfied structurally by *graph.Store (no internal/graph import in internal/agent)"
    - "apply = non-tool editor+CSRF endpoint reusing pages.Save(baseRevision) → single-writer commit + 409 floor (mirrors /apply-patch)"
    - "byte-stable tags write owned in one helper: okf.Parse(AssembleSource) → okf.SetTags → Emit → re-Parse → RawFront region → pages.Save"
key-files:
  created:
    - internal/agent/suggesttags.go
    - internal/agent/suggesttags_test.go
    - internal/server/handlers_tags_test.go
  modified:
    - internal/agent/agent.go
    - internal/agent/prompts.go
    - internal/server/handlers_agent.go
    - internal/server/router.go
    - cmd/okf-workspace/main.go
decisions:
  - "Server-side validateTags on apply uses a nil vocab: on the WRITE the existing-vs-new flag is irrelevant (we write exactly the normalized set) — the same cap/normalize/dedupe/filter runs, flags are discarded. Bias matters only on the suggest READ."
  - "The suggest endpoint's input-validation 400 paths can only be reached when an agent is configured (h.agent==nil short-circuits to a 500 fail-closed FIRST, matching the existing summarize/rewrite handlers). Since h.agent is a CONCRETE *agent.Service (not an interface), the server-package external test cannot inject a fake model; the suggest happy-path + 400 input validation are proven KEY-FREE at the agent seam (TestSuggestTags) and the apply seam (TestApplyTagsBadRequest). The server test asserts the load-bearing HTTP property: suggest fails CLOSED (500, never a hang) with no agent, and is any-authed (a reader is NOT 403'd — it is not editor-gated)."
metrics:
  duration: ~25m
  completed: 2026-06-24
  tasks: 2
  files: 8
---

# Phase 11 Plan 02: SuggestTags Mode + Tag Endpoints Summary

Built the backend half of per-page tag suggestion (TAG-01): a `SuggestTags` single-shot agent MODE (vocab-biased, capped at `MaxSuggestedTags`=5, validate-and-retry, provider-agnostic, key-free testable) plus two HTTP endpoints — a read-side suggest endpoint and an editor+CSRF apply endpoint that re-validates/normalizes server-side and writes only the tags lines byte-stably via 11-01's `okf.SetTags` through `pages.Save` → the single-writer commit, 409-ing on a moved revision. The read-only 5-tool boundary and its set-equality build gate are untouched (no 6th tool).

## What Was Built

### Task 1 — SuggestTags mode + validateTags (internal/agent) — commit 6a88308
- `internal/agent/suggesttags.go`: `MaxSuggestedTags`=5 + `maxTagLen` named constants; `ErrTagsInvalid` sibling sentinel to `ErrProposalInvalid`.
- `validateTags(raw, vocab) ([]string, []bool, error)`: lowercase+trim; dedupe (first wins, order preserved); cap to `MaxSuggestedTags`; reject empty/whitespace/over-length/NUL/interior-whitespace/control tokens; `ErrTagsInvalid` on empty result; parallel existing-vs-new flag computed against the normalized vocab.
- `SuggestTags(ctx, path) (tags, existing, baseRev, err)`: single-shot `ChatModel.Generate` mode mirroring Rewrite/Draft — fetches the body via the role-scoped pages reader, captures `baseRev` via `Pages.Revision` AT suggest time (like ProposePatch), reads vocabulary via the narrow `vocabularyReader` dep (best-effort; nil/error tolerated), parses the reply as a JSON array of strings (lenient on a wrapping code fence, strict on contents), and runs validate-and-retry (1 + 2 = 3 attempts) via `generateOnce` (~60s timeout + explicit `MaxTokens`). `ValidateTags` exported wrapper for the apply path.
- `agent.go`: added the narrow `vocabularyReader` interface (`Vocabulary(ctx) ([]string, error)`) + the `Vocabulary` field on `Deps` — no `internal/graph` import; `*graph.Store` satisfies it structurally.
- `prompts.go`: `suggestTagsSystemPrompt` (renders `MaxSuggestedTags`, fixes the JSON-array contract, prefers reusing vocab) + `buildSuggestTagsMessages` (body delimited as untrusted DATA, vocab as a bias hint, retry hint on attempts > 0).
- `main.go`: wired `Vocabulary: graphStore` into the existing `agent.NewService(cfg.Agent, &agent.Deps{...})` call (the only main.go change).
- `suggesttags_test.go`: key-free fake-model + fake-vocab tests — normalize/dedupe/cap/garbage-reject, existing flags, one-call happy path with ~60s deadline + correct system prompt, untrusted-DATA delimiting + vocab-in-prompt, garbage→3-calls→`ErrTagsInvalid`, disabled fail-closed, nil-vocab + vocab-error tolerance.

### Task 2 — suggest-tags + apply-tags endpoints (internal/server) — commit 3415336
- `handlers_agent.go`:
  - `handleSuggestTags` (any-authed read, mirrors handleRewrite): validate `page_path`, `auditAgentMode`, call `SuggestTags`, return `{page_path, suggestions:[{tag,existing}], base_revision}`; fail closed via `writeAgentModeError` (extended to map `ErrTagsInvalid` → 422).
  - `handleApplyTags` (editor + CSRF, mirrors handleApplyPatch): validate `page_path` + reject NUL + cap list size + non-empty; RE-validate/normalize via `agent.ValidateTags` (nil vocab → empty set = 422); `pages.Get` (404); build the new frontmatter region via `setTagsFrontmatter`; `pages.Save(body, newFrontmatter, base_revision, actor)` → map `ErrPageNotFound`→404, `ErrStaleRevision`→409 (write nothing); audit `ActionAgentPatchApproval` (non-secret detail only); 204.
  - `setTagsFrontmatter(frontmatter, body, tags)` helper: `okf.Parse(pages.AssembleSource(...))` → `okf.SetTags` → `Emit` → re-`Parse` → `string(RawFront)` — the one place the byte-stable tags write is owned (pages.Save owns final assembly, no hand-rolled fence).
- `router.go`: `POST /agent/suggest-tags` in the any-authed group (next to rewrite/draft); `POST /agent/apply-tags` in the editor subgroup (next to apply-patch, under global nosurf). Comments note neither is an Eino tool.
- `handlers_tags_test.go` (real HTTP seam via pageFixture + real git): `TestApplyTagsStaleRevision` (stale→409+no-write, current→204 writes once, body byte-stable), `TestApplyTagsRenormalizesServerSide` (tampered/un-normalized/over-cap/garbage list cleaned+capped to exactly 5 before write — proven by `countTagLines`), `TestApplyTagsEmptyAfterNormalize` (all-garbage→422), `TestApplyTagsRBAC` (reader→403), `TestApplyTagsBadRequest` (empty tags / missing page_path → 400), `TestSuggestTagsEndpoint` (fail-closed 500 with no agent; any-authed reader not 403'd).

## Deviations from Plan

### Adjustments

**1. [Adaptation - real signatures] `vocabularyReader.Vocabulary` method name + `Deps.Vocabulary` field**
- The plan suggested a generic narrow interface; implemented as `Vocabulary(ctx) ([]string, error)` so `*graph.Store.Vocabulary` (11-01) satisfies it structurally with zero adapter. Field on `Deps` is `Vocabulary` (call site `s.deps.Vocabulary.Vocabulary(ctx)`).

**2. [Adaptation - real seam] suggest-endpoint tests are HTTP fail-closed + RBAC, not 400-input, at the server layer**
- `h.agent` is a CONCRETE `*agent.Service` (its `cm` is unexported), so the `server_test` external package cannot inject a fake model. Per the plan's own guidance ("if the seam makes the full round-trip heavy, drive assertions at the seam available"), the suggest happy path + JSON-array/validate-and-retry are proven KEY-FREE at the agent seam (`TestSuggestTags`); the input-validation 400 logic is exercised at the apply seam (`TestApplyTagsBadRequest`). The server test asserts the HTTP-layer safety property: suggest fails closed (500, never a hang) with no agent and is any-authed (reader not 403'd). The `h.agent==nil`-first ordering matches every existing single-shot handler (summarize/rewrite/draft).

**3. [Adaptation - apply uses pages.AssembleSource]**
- `setTagsFrontmatter` uses the exported `pages.AssembleSource` (the canonical assembler, WR-05) instead of the unexported `assemble`, so the handler shares ONE assembly implementation with the writer and cannot drift.

No new dependency added; CGO-free single binary preserved; no 6th agent tool.

## Self-Check: PASSED

- `internal/agent/suggesttags.go` — FOUND
- `internal/agent/suggesttags_test.go` — FOUND
- `internal/server/handlers_tags_test.go` — FOUND
- Commits 6a88308, 3415336 — FOUND in git log

Verification output (real, pasted):
- `CGO_ENABLED=0 go build ./...` → exit 0
- `go vet ./internal/agent/ ./internal/server/` → clean
- `go test ./internal/agent/ ./internal/server/ ./internal/okf/ ./internal/graph/` → ok (all four)
- key-free (no DEEPSEEK_API_KEY / OKF_LLM_API_KEY) `go test ./internal/agent/ ./internal/server/` → ok
- `go test ./internal/agent/ -run TestToolSetIsExactlyReadOnlyAllowList` → PASS (the unchanged 5-tool set-equality gate)
- `TestSuggestTags` / `TestValidateTags` → PASS key-free; `TestApplyTagsStaleRevision` (409) → PASS
