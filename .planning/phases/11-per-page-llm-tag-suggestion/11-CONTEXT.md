# Phase 11: Per-Page LLM Tag Suggestion - Context

**Gathered:** 2026-06-24
**Status:** Ready for planning
**Mode:** Smart discuss (autonomous) — UX trigger/approval + guardrails resolved by the user; remainder pinned from v1.0 ARCHITECTURE/PITFALLS research.

<domain>
## Phase Boundary

Deliver on-demand, per-page LLM tag suggestion with a per-tag suggest→approve flow that writes approved tags BYTE-STABLY into the page's YAML frontmatter through the existing single-writer commit path (TAG-01..04). This establishes trust BEFORE the Phase-12 bulk sweep.

**In scope:** the byte-stable `okf.SetTags` primitive; a `SuggestTags` agent mode (single-shot, validate-and-retry) biased to existing vocabulary + capped; a dedicated "Suggest tags" trigger + per-tag approval UI; an apply path reusing the existing propose→approve→apply→commit + 409 stale-revision floor.

**Out of scope:** the bulk/untagged sweep + batch review queue (Phase 12 — TAG-05/06); shared-tag graph edges (Phase 9, already shipped); any change to the read-only agent tool boundary.
</domain>

<decisions>
## Implementation Decisions

### Trigger + approval UX — USER DECISION: dedicated tags control + approval list
- A dedicated **"Suggest tags"** control near the page's tags/frontmatter area (in the page/editor view) — NOT folded into the agent PromptBar.
- Clicking it requests suggestions and opens a focused **per-tag approval list**: each suggested tag is a checkbox row; **existing** workspace tags vs **new (invented)** tags are visually distinguished (e.g. a "new" badge); **new tags default to UNCHECKED** (existing-vocab tags may default checked). An **Apply** action writes only the approved tags.
- Reuse existing component/token patterns (the DiffReviewDialog / approval-dialog patterns, AgentPanel styling, tokens.css). No new dependency.

### Guardrails — USER DECISION: research defaults
- **Max 5 tags** suggested per page (named constant).
- Suggestions **biased toward the existing workspace tag vocabulary** — feed the current tag set (from Phase-8 `page_tags`) into the prompt so the model prefers reusing tags over inventing near-synonyms.
- **New (invented) tags default unchecked** in the approval UI (the user opts INTO new vocabulary).
- **Normalize on write**: lowercase, trim, dedupe (against each other AND the page's existing tags).
- **YAML style: block-style sequence** for the `tags` key — pinned once, consistent with the existing frontmatter convention; when a page has no `tags` key yet, create it block-style.

### Backend mechanics (from ARCHITECTURE/PITFALLS research — high confidence)
- **`okf.SetTags(d *Doc, tags []string)`** — the ONE new byte-stable primitive: a `yaml.SequenceNode` analog of the existing `okf.SetField` (repair.go), driven by `FrontDirty` so `Emit()` rewrites ONLY the `tags` lines; body + all other frontmatter stay byte-identical. GATE this phase on a golden-file round-trip test: adding/replacing tags changes only the `tags` lines in the diff (Pitfall 1 — the #1 risk).
- **`SuggestTags` agent mode** — a single-shot `ChatModel.Generate` mode mirroring the existing Rewrite/Draft modes + the validate-and-retry harness (validateTags: enforce array-of-strings, cap, normalization, reject hallucinated/garbage). It is NOT a 6th agent tool — the read-only 5-tool boundary + its set-equality test stay UNCHANGED (suggestion is a mode, apply is a non-tool endpoint).
- **Apply path** — a NON-tool editor-gated CSRF endpoint that reuses `pages.Save(baseRevision)` → single-writer `gitstore.Commit` (mirror `/apply-patch`). A moved/stale revision 409s rather than clobbering (proven by a test). Suggestion captures the base revision; apply re-validates + normalizes server-side (never trust client-sent tags blindly).

### Claude's Discretion
- Exact endpoint paths/names, the approval component's precise layout, the prompt wording, and whether existing-vocab tags default checked vs unchecked (new MUST default unchecked) are at Claude's discretion within the above contracts.
</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- `internal/okf/repair.go` `SetField` + `internal/okf/emit.go` `Emit`/`FrontDirty` — the proven byte-stable single-key-edit pattern to mirror for `okf.SetTags` (sequence-node variant).
- `internal/agent/` Rewrite/Draft single-shot modes + the validate-and-retry harness (propose.go / chatmodel.go) — mirror for `SuggestTags` + `validateTags`.
- `internal/server/handlers_agent.go` `/apply-patch` (editor-gated, CSRF, reuses pages.Save → single-writer commit, 409 on stale) — mirror for the apply-tags endpoint.
- Phase-8 `page_tags` (the existing workspace tag vocabulary) — read to bias the prompt + to mark existing-vs-new in the UI.
- Frontend: DiffReviewDialog / agent approval-dialog patterns, AgentPanel styling, react-query mutations, tokens.css; the page/editor view where the tags control mounts.

### Established Patterns
- Agent modes are single-shot ChatModel.Generate with a validate-and-retry output contract (NOT response_format/JSON-schema — provider-agnostic, DeepSeek/Ollama).
- Apply is always a NON-tool CSRF editor endpoint reusing pages.Save(baseRevision) → single-writer commit; the 5-tool read-only boundary is build-gated by a set-equality test — do NOT add a 6th tool.
- Byte-stable frontmatter: FrontDirty + Emit; golden round-trip tests are the gate.
- key-free agent tests use a fake model (TestDispatch pattern) — the new mode must be testable without an API key.

### Integration Points
- `internal/okf`: add `SetTags` + golden round-trip test.
- `internal/agent`: add `SuggestTags` mode + `validateTags` (vocab-biased prompt, cap, normalize) + key-free test.
- `internal/server/handlers_agent.go` + router: a suggest-tags endpoint (returns suggestions + base revision + existing-vs-new flags) and an apply-tags endpoint (editor + CSRF, re-validate/normalize, pages.Save → commit, 409 on stale).
- `web/src/`: the "Suggest tags" control + per-tag approval list, api client fns + react-query mutations, mounted in the page/editor view.
</code_context>

<specifics>
## Specific Ideas

- The golden-file byte-stability test for `okf.SetTags` is the load-bearing gate (Pitfall 1) — adding a tag must touch ONLY the `tags` lines; the body and other frontmatter bytes must be identical. Prove it for: page with no tags key, page with existing block tags, page with other frontmatter fields preserved/reordered-safe.
- Server-side re-validation + normalization on apply (never trust the client's tag list) prevents a tampered request from injecting un-normalized/over-cap tags.
- Establish trust at single-page scale here so Phase-12's bulk sweep can multiply a CORRECT primitive, not a buggy one.
- key-free tests (fake model) must cover the SuggestTags mode + validateTags so CI needs no API key.
</specifics>

<deferred>
## Deferred Ideas

- Bulk/untagged sweep + batch review queue + the KindTagSuggest job + tag_suggestions staging table — Phase 12 (TAG-05/06).
- Embedding-based near-synonym merge — v2 (TAG-F1).
- Auto-apply without review — explicitly OUT (violates the locked safety model).
</deferred>
