---
phase: 11-per-page-llm-tag-suggestion
verified: 2026-06-24T14:00:00Z
status: passed
score: 9/9 must-haves verified
behavior_unverified: 0
overrides_applied: 0
live_validation: "2026-06-24 — validated end-to-end against the running binary on :8098 with the REAL deepseek-v4-flash model (DEEPSEEK_API_KEY present). POST /agent/suggest-tags → 200 returning 5 sensible normalized tags (deployment/build/test/server/binary); POST /agent/apply-tags → 204; the page's frontmatter on disk now carries the tags BLOCK-STYLE with type/title/description above and timestamp below INTACT (byte-stable tags-only write); the hidden-Git single-writer commit landed (data/repo: '090e098 Edit deploy.md'). GAP FOUND + FIXED during live validation (commit a15a0dc): the initial live call 422'd because parseTagArray only accepted a bare/whole-fence JSON array, but DeepSeek wraps the array in prose/object/leading-prose-fence — the parser now de-fences anywhere, accepts {tags|suggestions|labels} objects, and extracts the first balanced [..] substring (validateTags unchanged as the gate). Key-free tests (fake model) never caught this since the fake returns clean JSON — only live validation did."
human_verification:
  - test: "Open a real page in the editor with a live LLM key (DEEPSEEK_API_KEY or OKF_LLM_API_KEY set) and click 'Suggest tags'"
    expected: "The approval surface opens within the single-shot ~60s window with up to 5 normalized suggestions; existing workspace tags are marked with no badge (existing=true); new tags carry the 'new' badge and default UNCHECKED; accepted existing tags default checked; the '{n} selected' count is live"
    why_human: "The full suggest→approve→apply flow requires a real LLM provider. All backend and frontend logic is proven key-free but the validate-and-retry loop against a real model (vocab-biased prompt, lenient code-fence stripping, 3-attempt retry) cannot be exercised without a network call."
---

# Phase 11: Per-Page LLM Tag Suggestion Verification Report

**Phase Goal:** A user can get LLM tag suggestions for the page they are on and approve them per tag, with approved tags merged byte-stably into the YAML frontmatter through the existing single-writer commit path — establishing trust before any bulk operation.
**Verified:** 2026-06-24T14:00:00Z
**Status:** human_needed
**Re-verification:** No — initial verification

---

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | Requesting tag suggestions for a page returns up to 5 normalized tags, each flagged existing-vs-new against the workspace vocabulary, plus the page's base revision — produced by a single-shot ChatModel.Generate mode (NOT a 6th agent tool) | ✓ VERIFIED | `suggesttags.go` exports `SuggestTags` (single-shot Generate, not a tool); `TestSuggestTags` key-free test: canned JSON → 3 tags + existing flags + baseRev, 1 model call; `tools.go` readToolNames has exactly 5 entries; `TestToolSetIsExactlyReadOnlyAllowList` and `TestReadToolNamesMatchesConstant` both PASS |
| 2 | The SuggestTags mode validates-and-retries the model output (array-of-strings, cap MAX 5, lowercase/trim/dedupe, reject garbage) and is fully testable WITHOUT an API key | ✓ VERIFIED | `validateTags` + `parseTagArray` + retry loop (1+2) in `suggesttags.go`; `MaxSuggestedTags = 5` named constant, no bare literal; `TestValidateTags` (7 sub-tests: clean, lowercase/trim, dedupe, cap, garbage, all-garbage→ErrTagsInvalid, existing-flag) and `TestSuggestTags` (7 sub-tests including garbage→3 calls) all PASS with `env -u DEEPSEEK_API_KEY -u OKF_LLM_API_KEY` |
| 3 | Applying approved tags is an editor-gated CSRF endpoint that re-validates+normalizes server-side, writes ONLY the tags lines byte-stably via okf.SetTags through pages.Save → the single-writer commit, and 409s on a stale revision | ✓ VERIFIED | `handleApplyTags` in `handlers_agent.go`: editor subgroup registration (`router.go` line 220), `agent.ValidateTags` re-validates before write, `setTagsFrontmatter` calls `okf.SetTags`, `pages.Save(baseRevision)` → `ErrStaleRevision` → HTTP 409; `TestApplyTagsStaleRevision` (PASS: stale→409 + body unchanged + revision unmoved; control→204 + tag written + body identical); `TestApplyTagsRenormalizesServerSide` (PASS: tampered/over-cap list normalized to 5 clean tags); `TestApplyTagsRBAC` (PASS: reader→403) |
| 4 | The read-only 5-tool allow-list and its set-equality build gate are UNCHANGED (no 6th tool added) | ✓ VERIFIED | `tools.go` `readToolNames` = ["list_tree","read_page","search_pages","search_attachments","read_attachment_text"] (5 entries, unchanged); `TestToolSetIsExactlyReadOnlyAllowList` PASS; `TestReadToolNamesMatchesConstant` PASS; `internal/graph` not imported by `internal/agent` (comment only) |
| 5 | Adding/replacing tags on a page changes ONLY the `tags` lines; body and all other frontmatter bytes are byte-identical | ✓ VERIFIED | `okf.SetTags` in `repair.go` sets `FrontDirty`; `TestSetTags` (3 fixtures: no-tags-key, existing-block-tags, other-frontmatter): GATE 1 body bytes == original body bytes; GATE 2 non-tags frontmatter keys preserved in content AND order (structural re-parse comparison); GATE 3 block-style sequence; `TestSetTags_NoOpRoundTripIsByteIdentical` (3 fixtures, all PASS) |
| 6 | The workspace tag vocabulary (distinct existing tags) is queryable from the derived graph store for prompt biasing | ✓ VERIFIED | `Store.Vocabulary` in `graph/query.go`: `SELECT DISTINCT tag FROM page_tags ORDER BY tag`, non-nil empty slice on zero tags; wired in `main.go` line 318 (`Vocabulary: graphStore`); `vocabularyReader` interface in `agent.go` (narrow dep, no graph import in agent); `go test ./internal/graph/ -run Vocabulary` PASS |
| 7 | An editor sees a dedicated 'Suggest tags' trigger near the page tags/frontmatter area; clicking it fetches suggestions and opens a per-tag approval list | ✓ VERIFIED | `TagSuggest.tsx` exports default `TagSuggest`; mounted in `PageEditor.tsx` line 646 inside `.pageeditor-frontmatter` div; trigger button with label "Suggest tags" (line 121); `canEdit` gate returns `null` for reader; `suggestMutation.mutate()` on click; `npx vitest run TagSuggest.test.tsx` — 11 tests PASS including "does not render the trigger for a reader" and loading state |
| 8 | Each suggested tag is a checkbox row; existing-vocab vs new (invented) tags are visually distinguished by a 'new' word-badge AND new tags default UNCHECKED; only checked tags are applied | ✓ VERIFIED | `TagSuggest.tsx` lines 50-61: `checked` initialized with `s.existing` (new=false → unchecked); `onApply` filters `suggestions.filter(s => checked[s.tag])` (line 93); "new" badge rendered only when `!s.existing`; `TagSuggest.test.tsx` — "renders new tags with the 'new' badge AND unchecked" PASS; "applies EXACTLY the checked tags + captured base_revision" PASS |
| 9 | Apply is the single accent action and is NEVER auto-focused (Cancel is DOM-first + focused); Esc/backdrop cancel without writing; a 409 stale response shows the stale state (no clobbering apply, Re-run offered) | ✓ VERIFIED | `TagSuggestList` in `TagSuggest.tsx`: `cancelRef` focused on open via `useEffect` (line 211); Esc handler calls `onCancelRef.current()` (line 216); backdrop `onMouseDown` calls `onCancel` (line 248); stale state (err.status===409) removes Apply path and shows Re-run (lines 73-78, 393-420); `TagSuggest.test.tsx` — "does NOT auto-focus Apply" PASS; "Esc cancels and calls applyTags ZERO times" PASS; "a 409 switches to the stale state (Apply removed; Re-run offered)" PASS |

**Score:** 9/9 truths verified (0 present, behavior-unverified)

---

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/okf/repair.go` | `func SetTags` — byte-stable block-style sequence editor | ✓ VERIFIED | Lines 148-191: block SequenceNode, FrontDirty set, in-place replace or append, no normalization in SetTags |
| `internal/okf/settags_test.go` | `func TestSetTags` — golden byte-stability gate | ✓ VERIFIED | Three sub-tests + `TestSetTags_NoTagsKey_OnlyTagsLinesAdded` + `TestSetTags_NoOpRoundTripIsByteIdentical`; structural re-parse assertion (not substring grep); all PASS |
| `internal/okf/testdata/settags/` | Three fixtures | ✓ VERIFIED | `no-tags-key.md`, `existing-block-tags.md`, `other-frontmatter.md` all present |
| `internal/graph/query.go` | `func (s *Store) Vocabulary` | ✓ VERIFIED | Lines 378-403: SELECT DISTINCT tag FROM page_tags ORDER BY tag; non-nil empty slice; `go test ./internal/graph/` PASS |
| `internal/agent/suggesttags.go` | SuggestTags + validateTags + MaxSuggestedTags | ✓ VERIFIED | All present; MaxSuggestedTags=5 named constant; ErrTagsInvalid; ValidateTags exported for server re-use; retry loop (tagsMaxRetries=2) |
| `internal/agent/suggesttags_test.go` | Key-free TestSuggestTags + TestValidateTags | ✓ VERIFIED | 7 TestValidateTags sub-tests + 7 TestSuggestTags sub-tests; fakeVocabReader; all PASS without API key |
| `internal/server/handlers_agent.go` | handleSuggestTags + handleApplyTags + setTagsFrontmatter | ✓ VERIFIED | Lines 647-827; setTagsFrontmatter helper isolates byte-stable write; 409 on ErrStaleRevision; re-validate via agent.ValidateTags |
| `internal/server/handlers_tags_test.go` | TestApplyTagsStaleRevision + renormalize + endpoint tests | ✓ VERIFIED | 6 test functions; all PASS; stale→409 proven; body unchanged; cap+dedupe proven |
| `web/src/api/client.ts` | suggestTags + applyTags functions | ✓ VERIFIED | Lines 960-980; TagSuggestion + SuggestTagsResult interfaces; applyTags uses mutate() (CSRF-bearing); 409 surfaces via err.status |
| `web/src/components/TagSuggest.tsx` | TagSuggest trigger + approval surface | ✓ VERIFIED | Editor-gated; checkbox rows; new badge; unchecked-default for new; Cancel focused; Esc/backdrop cancel; stale state; no dangerouslySetInnerHTML |
| `web/src/components/TagSuggest.test.tsx` | vitest coverage of locked interaction contract | ✓ VERIFIED | 11 tests covering all interaction guarantees; all PASS |

---

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| `internal/okf/repair.go` | `internal/okf/emit.go` | SetTags sets `d.FrontDirty = true` so Emit re-marshals ONLY tags lines | ✓ WIRED | Lines 184 and 190 in repair.go; confirmed by golden tests |
| `internal/graph/query.go` | `page_tags` table | Vocabulary SELECTs DISTINCT from page_tags | ✓ WIRED | `SELECT DISTINCT tag FROM page_tags ORDER BY tag` at line 390 |
| `internal/agent/suggesttags.go` | `internal/graph/query.go` | SuggestTags reads vocabulary via `s.deps.Vocabulary.Vocabulary(ctx)` | ✓ WIRED | Line 192; narrow `vocabularyReader` interface; `*graph.Store` satisfies it structurally; wired in main.go line 318 |
| `internal/server/handlers_agent.go` | `internal/okf/repair.go` | handleApplyTags calls `okf.SetTags` via `setTagsFrontmatter` | ✓ WIRED | Line 817 in handlers_agent.go; setTagsFrontmatter helper at lines 812-827 |
| `internal/server/handlers_agent.go` | `internal/pages/service.go` | handleApplyTags calls `pages.Save(baseRevision)` → `ErrStaleRevision` → 409 | ✓ WIRED | Lines 775-786; `errors.Is(err, pages.ErrStaleRevision)` → HTTP 409 |
| `web/src/components/TagSuggest.tsx` | `web/src/api/client.ts` | react-query mutations call suggestTags (read) + applyTags (CSRF write) | ✓ WIRED | Lines 48-79; `suggestMutation` calls `suggestTags(pagePath)`; `applyMutation` calls `applyTags({page_path, tags, base_revision})`; 409 via `err.status === 409` |
| `web/src/routes/PageEditor.tsx` | `web/src/components/TagSuggest.tsx` | Trigger mounts in `.pageeditor-frontmatter` block | ✓ WIRED | Line 646: `<TagSuggest pagePath={path} />` inside the div.pageeditor-frontmatter |

---

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|------------|-------------|--------|----------|
| TAG-01 | 11-02, 11-03 | User can request LLM tag suggestions on demand for the page they are viewing/editing | ✓ SATISFIED | POST /agent/suggest-tags (any-authed) + SuggestTags single-shot mode + TagSuggest trigger in editor |
| TAG-02 | 11-03 | Suggested tags presented for human review per tag; new vs. existing distinguished; newly-invented tags default unchecked | ✓ SATISFIED | TagSuggest checkbox rows; `existing:false` → "new" badge + UNCHECKED default; vitest confirms |
| TAG-03 | 11-01, 11-02 | Approved tags merged byte-stably into YAML frontmatter tags field; only tags lines change; single-writer Git path; stale revision 409s | ✓ SATISFIED | okf.SetTags (FrontDirty→Emit); setTagsFrontmatter; pages.Save+ErrStaleRevision; golden tests + TestApplyTagsStaleRevision |
| TAG-04 | 11-01, 11-02 | Tag suggestions biased toward existing workspace vocabulary; capped at max 5; normalized on write (lowercase, trimmed, deduped) | ✓ SATISFIED | Store.Vocabulary fed into prompt; MaxSuggestedTags=5 constant; validateTags lowercase/trim/dedupe/cap; server re-validates on apply |

---

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|---------------|--------|--------------------|--------|
| `TagSuggest.tsx` | `suggestions` / `baseRevision` | `suggestTags()` → POST /agent/suggest-tags → `SuggestTags()` → real LLM Generate (key-required path) / fake model in tests | Yes — live path uses ChatModel.Generate; test path uses fakeChatModel | ✓ FLOWING (key-free tests) / human-needed (live LLM) |
| `TagSuggest.tsx` | `checked` (selection state) | Initialized from `res.suggestions.map(s => [s.tag, s.existing])` | Yes — real data from suggest response | ✓ FLOWING |
| `handleApplyTags` | `newFrontmatter` | `setTagsFrontmatter(pg.Frontmatter, pg.Body, normalized)` → `okf.SetTags` → `Emit` | Yes — pages.Get returns real frontmatter; SetTags mutates it; Emit re-serializes | ✓ FLOWING |

---

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| `okf.SetTags` golden byte-stability gate | `go test ./internal/okf/ -run TestSetTags -v` | PASS: 3 fixtures × 3 gates (body, non-tags keys, block-style sequence) + no-op round-trip | ✓ PASS |
| SuggestTags key-free validate-and-retry | `go test ./internal/agent/ -run TestSuggestTags -v` | PASS: 7 sub-tests including 3-call retry on garbage | ✓ PASS |
| apply-tags 409 stale floor | `go test ./internal/server/ -run TestApplyTagsStaleRevision -v` | PASS: stale→409+page unchanged; control→204+tag written+body identical | ✓ PASS |
| apply-tags server-side renormalize | `go test ./internal/server/ -run TestApplyTagsRenormalizesServerSide -v` | PASS: tampered payload normalized to exactly 5 clean tags | ✓ PASS |
| 5-tool set-equality gate unchanged | `go test ./internal/agent/ -run 'TestToolSetIsExactlyReadOnlyAllowList\|TestReadToolNamesMatchesConstant' -v` | PASS: 5 tools, names match constant, no 6th tool | ✓ PASS |
| TagSuggest trust contract | `npx vitest run src/components/TagSuggest.test.tsx` | PASS: 11 tests (editor-gate, new-unchecked, only-checked-applied, Cancel-focused, Esc/backdrop-cancel, 409-stale, loading/empty/error) | ✓ PASS |
| TypeScript compilation | `npx tsc -b` | exit 0, no errors | ✓ PASS |
| Full CGO-free build | `CGO_ENABLED=0 go build ./...` | exit 0 | ✓ PASS |
| Key-free cross-package tests | `env -u DEEPSEEK_API_KEY -u OKF_LLM_API_KEY go test ./internal/okf/ ./internal/agent/ ./internal/server/ ./internal/graph/` | All packages PASS | ✓ PASS |

---

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| `internal/server/handlers_agent.go` | 27 | `TODO` in a pre-existing WR-02 comment about per-page ACLs | ℹ️ Info | Pre-existing, references a known deferred design concern (per-page ACLs), not introduced by phase 11; not a phase-11 gap |
| `web/src/components/TagSuggest.tsx` | 24 | `dangerouslySetInnerHTML` appears in a comment prohibiting it | ℹ️ Info | Comment is the locked stored-XSS guard documentation (intentional), not an actual use |

No BLOCKER debt markers (TBD/FIXME/XXX without issue references) in any phase-11 file. No new npm dependency (package.json unchanged across all phase-11 commits: e5e81e9, 6c37834, 3415336, 6a88308, 42c79e2). No new CSS token.

---

### Human Verification Required

#### 1. Live end-to-end LLM suggestion flow

**Test:** With a real LLM key configured (DEEPSEEK_API_KEY or OKF_LLM_API_KEY), open a page in edit mode, click "Suggest tags", and exercise the full approval flow — approve some, reject others (leave new tags unchecked), click Apply.

**Expected:** Up to 5 tags are suggested within the single-shot ~60s timeout. Existing workspace tags appear without the "new" badge (default checked); invented tags carry the "new" word-badge and start UNCHECKED. The "{n} selected" count is live as checkboxes toggle. Clicking Apply writes exactly the checked tags to the page's YAML frontmatter (only tags lines change, body is byte-identical). A concurrent edit between suggest and apply produces a 409 with the "Re-run" recovery path.

**Why human:** The full suggest→approve→apply roundtrip requires a real LLM provider (ChatModel.Generate over the network). The mode, validate-and-retry loop, endpoints, and UI are all proven key-free, but the model's actual output quality (vocab biasing working, code-fence stripping working, retry triggered on prose output) can only be observed with a live API call.

---

### Gaps Summary

No gaps. All 9 observable truths are VERIFIED. The only item requiring human attention is the live LLM end-to-end flow which is inherently un-automatable without a real API key — this is classified as human_needed (standard for any agent-mode phase), not a gap.

---

_Verified: 2026-06-24T14:00:00Z_
_Verifier: Claude (gsd-verifier)_
