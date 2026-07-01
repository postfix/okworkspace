---
author: qa
responsible: architect
phase: 29
status: complete
verdict: passed
parent_artifacts:
  - .planning/phases/29-tag-regression-fix/29-01-PLAN.md
  - .planning/phases/29-tag-regression-fix/29-CONTEXT.md
  - .planning/phases/29-tag-regression-fix/29-SUMMARY.md
  - .planning/REQUIREMENTS.md
---

# Phase 29: Tag-Regression Fix (server-authoritative) — VERIFICATION

**Verified:** 2026-07-01 by qa (independent, goal-backward, against committed HEAD `3e7b32f`).
**Substrate:** SMTC daemon OFFLINE — every code claim below is grep/read confidence (Structural/Honest
tier), never Decision-tier. Test claims are RE-RUN observations (Empirical), not SUMMARY trust.

## Verdict: **PASSED**

All six requirements (TR-01…TR-06) are satisfied against code reality. Two **LOW** findings route up to
the architect (neither blocking; both test-granularity / preview-cosmetics, not correctness/security).
The TR-02 anti-drift centerpiece was **independently confirmed by perturbation** — it is real, not hollow.
The tree was left CLEAN (drift perturbation reverted, `internal/web/dist/.gitkeep` restored).

## Method

Re-read the load-bearing source (hashtag.go, validate.go, batch.go, tagscan.go, handlers_tags_admin.go,
hashtag.ts span helper, TagManagement.tsx, client.ts) and re-ran every gate + suite myself. I did NOT
trust the SUMMARY — the engineer was aborted mid-final-verification and the architect's recovery is not
an independent check. The one source mutation I made (a deliberate corpus perturbation for the TR-02
drift proof) was reverted; `git status` is clean of any qa change.

## Gate results (re-run at HEAD `3e7b32f`)

| Gate | Command | Result |
|------|---------|--------|
| Backend build | `go build ./...` | exit 0 |
| Backend vet | `go vet ./...` | exit 0 |
| Backend test | `go test ./...` | **18 pkgs ok**, 2 no-test (okf, pages, server all green) |
| Frontend types | `npx tsc -b` | exit 0 |
| Frontend unit | `npx vitest run` | **803 passed / 65 files** (matches SUMMARY; parity test present) |
| Frontend build | `npm run build` | exit 0 (assertLazyChunks active; only benign INEFFECTIVE_DYNAMIC_IMPORT notices) |

`npm run build` wiped `internal/web/dist/.gitkeep` (expected per plan; `dist/` is gitignored except the
keep file) — **restored via `git checkout`**, tree re-confirmed clean.

---

## Per-requirement verdicts

### TR-01 — `okf.RewriteHashtags` body-rewrite primitive — **PASSED**

Evidence (internal/okf/hashtag.go, 468 ln):
- **Full Lezer exclusion set.** `classifyBlocks` (:137) marks fenced code (reuses `fenceAt`/
  `skipFencedBlock`), indented (4-space/tab) code (:173, paragraph-interruption aware), and HTML blocks
  (`htmlBlockAt` :355, CommonMark type-1 + type-6). `FindHashtags` (:83) additionally skips inline code
  (`skipInlineCode`), `\#` escapes (:105), and ATX-heading leading `#` (`isATXHeadingLine` :312 keeps the
  marker run bounded; its `#`s fail `scanTagBody`). This is the FULL set the requirement demands (link
  scanner's fenced/inline/escape PLUS indented/HTML/ATX).
- **Boundary predicates ported exactly.** `scanTagBody` (:244) = body alphabet `[A-Za-z0-9_/-]`, ≥1
  non-digit `[A-Za-z_-]` (slash is a separator not a qualifier), trailing-`/` trim; `hashStartOK` (:229)
  is the start-of-line/whitespace/blockquote-content-start rule. Mirrors hashtag.ts:44-88.
- **Conservative / byte-stable.** Only recognized spans are edited; on no match the ORIGINAL slice is
  returned. `TestRewriteHashtags_NoMatchSameSlice` asserts the SAME backing array (`&got[0]==&body[0]`) —
  a true no-alloc round-trip, not merely byte-equal.
- **D-03a delete rule.** `TestRewriteHashtags_Delete` (7 cases) proves: space-bounded-both-sides collapses
  exactly one space (`see #ops here`→`see here`); trailing punctuation untouched (`foo #ops.`→`foo .`);
  start-of-line no left-collapse; and **never merges across a newline** (`a #ops\nb`→`a \nb`).
- **D-03b merge adjacent-dup.** `TestRewriteHashtags_MergeAdjacentDup`: `#a #b` merge a→b → `#b #b` (no
  dedupe).
- **Byte-stability.** `TestRewriteHashtags_ByteStability`: only the matched `#ops`→`#devops` bytes change;
  a `#ops` inside a fence and a `#keep` tag are preserved verbatim.

Run: `go test ./internal/okf/ -run RewriteHashtags -v` — all 6 tests PASS.

### TR-02 — Lezer↔Go offset-parity test (anti-drift) — **PASSED (drift independently confirmed)**

Evidence:
- **Offset-level, not name-set.** `TestRewriteHashtags_LezerParity` (hashtag_test.go:43) asserts each
  span's `Start`/`End` byte offsets AND name against the corpus, and guards against a vacuous pass
  (`totalSpans==0` → Fatal). It compares byte spans, exactly the offset-parity discipline (a set-equal
  test would be a false-safe).
- **Single-sourced from the REAL Lezer parser.** `hashtag.parity.test.ts` runs
  `inlineTagSpansFromBody` (hashtag.ts:167) — which uses the SAME `tagParser` as the render/materializer
  — over the shared `hashtag.fixtures.ts`, converting UTF-16 positions to UTF-8 byte offsets
  (`utf16ToByteOffsets` :192) so the Go byte-native scanner is comparable. It writes/diffs
  `internal/okf/testdata/hashtag_corpus.json`. Both sides pin to that ONE JSON.
- **Corpus coverage.** 54 whole-document fixtures: full hashtag.test.ts positive/negative corpus,
  Phase-21 adversarial (slash policy, trailing punctuation, Unicode `#café`/emoji boundaries), AND block
  contexts (indented-code-after-blank, HTML block, GFM table, lazy blockquote continuation, `>#tag`
  marker abut, fenced-then-live, in-heading tag).
- **Regenerate-and-diff mechanism.** `hashtag.parity.test.ts` test 2 regenerates from the current grammar
  and asserts byte-identity with the checked-in JSON → a grammar/fixture change not committed to JSON
  fails CI, and the Go test (pinned to the same JSON) then fails if the Go scanner doesn't match.

**DRIFT CONFIRMATION (the SUMMARY-flagged item #1) — done independently:**
1. Perturbed one span in `hashtag_corpus.json` (`after-whitespace` `to`: 11→10).
2. `go test ./internal/okf/ -run LezerParity` → **FAILED**:
   `hashtag_test.go:57: after-whitespace: span 0 = [4,11), want [4,10) (body="see #deploy")`.
3. `npx vitest run src/lib/cm/hashtag.parity.test.ts` → **FAILED** (regenerate-and-diff mismatch on the
   corpus JSON).
4. `git checkout -- internal/okf/testdata/hashtag_corpus.json` → both PASS again; `git status` clean.

**Conclusion: the anti-drift guarantee is REAL.** A corpus change the Go scanner misses fails CI on both
sides. This was the phase's central risk (Phase-21 "second `#tag` parser" drift) and it is genuinely
retired, not hollow.

Run: `go test ./internal/okf/ -run LezerParity` PASS; `npx vitest run …/hashtag.parity.test.ts` 3/3 PASS.

### TR-03 — rename/merge/delete rewrite body `#tags` + validator + predicate + body channel — **PASSED**

Evidence:
- **Validator (BLOCKER).** `okf.ValidateInlineTag` (validate.go:32) is the inline-grammar subset
  (non-empty, `[A-Za-z0-9_/-]`, ≥1 non-digit, slash policy no leading/trailing/doubled). It is the
  **SOLE** `Validate*` call in the handlers (grep: only `ValidateInlineTag` at handlers_tags_admin.go:278
  rename-`newName`, :362 merge-`target`; no `ValidateTagCharset` call remains) — 400 before any write.
  `ValidateTagCharset` is unchanged (unit-guarded by `TestValidateInlineTag_DoesNotLoosenCharset`).
  Delete calls no new-token validator (correct — it only removes). 28-case `TestValidateInlineTag`
  proves reject: space/tab/newline/all-digit/`12/34`/`/frag`/`a/`/`a//b`/`<script>`/`café`/leading-`#`;
  accept: `deploy`/`a/b`/`a_b-c`/`world123`.
- **Widened predicate.** `buildTagApplyItems` (:240) includes a page when `changed || bodyWillChange`;
  `SkipFrontmatter=!changed` (:245) keeps a body-only page's tags region byte-identical.
- **TOCTOU-safe body channel.** `TagApplyItem` gains `BodyOld/BodyNew` (batch.go:61); `ApplyTagsBatch`
  recomputes the new body from its OWN freshly-read `doc.Body` (batch.go:150-152) via `RewriteHashtags`
  and passes it (not verbatim) to `emitForWrite` (:168). The old verbatim `doc.Body` emit is gone for
  affected pages. No-op guard (:176) skips a byte-identical write.
- **Regression goldens (all substantive, RE-RUN pass).**
  - `TestBodyRewriteRenameGolden`: rewrites body `#ops`→`#devops` + frontmatter `[devops]`; asserts
    `#ops` gone AND feeds the committed page back through the recognizer (`bodyHasTag`) to prove
    reconcile cannot re-add `ops` — the regression is closed at the source.
  - `TestBodyRewriteDeleteGolden`: `#ops` gone + D-03a whitespace (`text #ops here`→`text here`).
  - `TestBodyRewriteMergeGolden`: body `#ops`→`#devops`.
  - `TestBodyRewriteBodyOnlyGolden`: frontmatter has `docs` (no `ops`), body `#ops` only → page STILL
    written, body rewritten, **frontmatter region byte-identical** (`frontRegion` before==after) — the
    body-only case that was previously dropped at `changed=false`.
  - `TestBodyRewriteStaleClient409`: after the server rewrite lands, a stale open client saving the
    PRE-rewrite body at the PRE-rewrite revision is rejected `ErrStaleRevision`; the rewrite is NOT
    clobbered (`#ops` absent). This is the path a client-only fix cannot cover.
- **Invariants preserved.** One `commitPayload` → one `EnqueueCommit` (batch.go:200-213, N pages → 1
  commit); per-page 409 excludes a stale page without sinking the batch (batch.go:131-134,
  `TestApplyTagsBatchStaleDoesNotSink` PASS); audit records preserved (handlers :336 rename, :413 merge).
- **v1.3 regression suite intact.** `TestTagRename/Merge/Delete_PreviewThenApply`,
  `TestTagRename_EmptyName/CaseOnlyNoOp/BadCharset`, `TestTagMerge_SameSourceTarget`,
  `TestApplyTagsStaleRevision` all PASS.

Run: `go test ./internal/pages/ ./internal/server/ -run 'BodyRewrite|Stale|Tag...'` — all PASS.

### TR-04 — body-inclusive enumeration in the pages service — **PASSED**

Evidence:
- **In the pages service, SEC-01 resolver.** `Service.PagesWithBodyTag` (tagscan.go:65) walks
  `s.repo.Tree()` + `s.repo.Read(path)` (path from Tree — within-root by construction), mirroring the
  `templates.go` `Tree()`+`Resolve` SEC-01 pattern. `isLivePage` (:29) skips directories, non-`.md`, and
  the trash prefix. Recognition is single-sourced through `okf.FindHashtags` (no third parser).
- **Finds body-only pages; excludes fenced/trashed/non-.md.** `TestPagesWithBodyTag` seeds body-only,
  frontmatter-only, both, fenced-code, trashed, and `.txt` pages; asserts the result is exactly
  `{bodyonly.md, both.md}`. `TestPagesWithBodyTag_CaseAndEmpty` proves case-normalization + empty guard.
- **Wired into all 3 handlers.** rename (:299), merge (:376), delete (:443) union `graph.PagesByTag` with
  `pages.PagesWithBodyTag` via `uniqueStrings`. Merge correctly enumerates body-`#source` only (not
  body-`#target`).
- **graph/query.go no-body invariant HOLDS.** `git diff cdef56f..HEAD -- internal/graph/query.go` = **0
  lines** (Phase 29 never touched it). The `reads NO bodies` invariant comment is intact at query.go:256;
  the `repo.Read`+`okf.Parse` at :75/:467 are PRE-EXISTING frontmatter/title reads (guarded by
  `doc.HasFrontmatter`) in a different function, not new body-`#tag` scans.

### TR-05 — preview surfaces body-rewrite scope — **PASSED (LOW finding 1)**

Evidence:
- **Server field.** `tagPreviewPage.BodyAffected` (`json:"body_affected"`, handlers:66), set from
  `RewriteHashtags(pg.Body,…)` in `buildTagApplyItems` (:238).
- **Client type.** `TagPreviewPage.body_affected?: boolean` (client.ts:1357).
- **UI marker.** `TagManagement.tsx` Rename (:147-157) and Merge (:220-230) dialogs render BOTH a summary
  note ("N page(s) also have body #tags that will be rewritten") AND a per-page "prose" badge.

**LOW finding 1 (route up):** (a) The **Delete** dialog (:278-293) does NOT surface body scope — it shows
only "Remove … from N page(s)", even though delete DOES rewrite bodies. (b) There is **no frontend
rendering test** for the marker; the vitest delta (+3) is only the parity test. Task 5's acceptance asked
for "a minimal assertion that a body-affected preview entry renders its marker" — not delivered.
**Judgment / severity rationale:** LOW, non-blocking. The body-rewrite correctness itself is well-covered
server-side (preview builds `body_affected`; goldens exercise the rewrite). The marker is presentational
JSX whose only logic is `.some()/.filter()`. Rename & merge — the operations most likely to surprise an
admin with prose changes — DO surface it. The delete omission is a minor preview-UX inconsistency, not a
correctness or security defect. Recommend the architect either add the delete-dialog marker + a
TagManagement render test in Phase 30, or accept the server-field coverage as sufficient and note the
delete gap. **Server-side field coverage is adequate; a rendering test is nice-to-have, not required.**

### TR-06 — detect+report pre-fix stale-body corruption — **PASSED**

Evidence:
- **Detect-only.** `Service.DetectStaleBodyTags` (tagscan.go:118) walks live pages and reports every page
  whose body `#tag` is absent from its frontmatter (the half-reverted pre-fix signature). It NEVER writes.
- **Substantive test.** `TestDetectStaleBodyTags` seeds a corrupt page (body `#ops`, frontmatter
  `[devops]`), a coherent page (body `#ops` ∈ frontmatter), a frontmatter-only page, and a fenced-only
  page; asserts exactly `corrupt.md` with `BodyOnlyTags=[ops]`. PASS.
- **Surface.** No handler/CLI caller — it is a test-only read-only report method, which D-07 explicitly
  permits ("a read-only report method + a test is sufficient; a full admin panel is optional").
- **Honesty.** The ROADMAP (lines 123-125) is ALREADY honestly framed: the fix closes the regression "for
  all future operations — including pages never reopened", and "Historical corruption already left by
  pre-fix ops is handled separately (TR-06), not silently claimed away." The SUMMARY's phrasing that it
  "softened the ROADMAP over-claim" is slightly imprecise (the ROADMAP text was already honest), but the
  substantive disposition — detect+report + a surfaced historical set, no silent claim — is correct and
  honest. TR-06's honesty bar is met.

---

## Invariant + regression findings

- **Byte-stability:** only tags-region frontmatter lines + matched body `#tag` bytes change —
  `TestRewriteHashtags_ByteStability` + `TestBodyRewriteBodyOnlyGolden` (frontmatter region byte-identical
  for a body-only rewrite). HELD.
- **Single-writer batched commit (N→1):** batch.go:200-213 — one payload, one `EnqueueCommit`. HELD.
- **Per-page 409 + audit logging:** batch.go:131-134 + handlers audit records; `…StaleDoesNotSink` PASS.
  HELD.
- **No second `#tag` parser:** `okf.FindHashtags` is the single recognizer reused by `RewriteHashtags`
  (hashtag.go), `bodyHasTag`/`PagesWithBodyTag`/`DetectStaleBodyTags` (tagscan.go); offset-parity is
  CI-enforced against the Lezer corpus. HELD.
- **Phase-28 reading-shell surface untouched:** `git diff cdef56f..HEAD` touches exactly the 16 planned
  files; no `web/src/components/reading/**`, no `web/src/routes/**`, no `navSearch.ts`, no
  `graph/query.go`. HELD.
- **hashtag.ts additive only:** +57 lines (the `inlineTagSpansFromBody` byte-offset span helper +
  `utf16ToByteOffsets`); existing exports (`inlineTagsFromBody`, `tagParser`) unchanged. HELD.

## Findings routed UP to the architect

| # | Severity | Finding | Evidence | Recommendation |
|---|----------|---------|----------|----------------|
| 1 | LOW | Delete preview dialog omits the body-affected marker; no frontend render test for the marker (Task 5's minimal assertion undelivered). | TagManagement.tsx:278-293 (delete dialog has no `body_affected` render); vitest delta +3 = parity test only. | Optional Phase-30 follow-up: add delete-dialog marker + a TagManagement render test; OR accept server-field coverage. Non-blocking. |
| 2 | LOW | Handler-level validator test does not independently prove `ValidateInlineTag` is the active HTTP gate; merge has no validator-rejection handler test. | `TestTagRename_BadCharset` uses `<script>` (rejected by BOTH old+new validators — not distinguishing); no all-digit/space case; no merge-invalid-target test. | Optional: add a rename-`1984` (all-digit) + merge-invalid-target 400 handler test. Mitigated: `ValidateInlineTag` is exhaustively unit-tested (28 cases) and grep confirms it is the SOLE `Validate*` call in both handlers, so the functional guarantee holds; this is test-granularity only. Non-blocking. |

## Tree state

CLEAN. The only untracked path is `.planning/phases/30-v13-closure/` (a concurrent agent's Phase-30 work
that appeared during this session — NOT mine, and outside the Phase-29 committed surface). My TR-02 drift
perturbation was reverted (`git checkout`) and `internal/web/dist/.gitkeep` was restored after the build.
No qa source change remains. HEAD unchanged at `3e7b32f`.
