---
author: qa
responsible: qa
phase: "22"
---

# Phase 22: Model-B Materialization + Tag UI — VERIFICATION

**Plan file:** `.planning/phases/22-model-b-materialization-tag-ui/22-01-PLAN.md`
**SUMMARY file:** `.planning/phases/22-model-b-materialization-tag-ui/22-01-PLAN-SUMMARY.md`
**Requirements:** ED-02b, ED-02c
**Commits verified (on `main`):** `5c32567`, `57ac048`, `fd3332f`, `b687521`, `5b2edd9`, `0c76c88`, `3709c71`

## Verdict

```yaml
gate_result:
  record_author: qa
  id: GR-22-verification
  phase: "22"
  verdict: pass
  basis: "On save, runSaver reconciles body #tags into the canonical frontmatter tags: as a DIFF (removedInline = inline(lastSavedBody) − inline(currentBody); tags = (readTags(fm) − removedInline) ∪ inline(currentBody)) BEFORE the PUT, against lastSavedBody.current (the prior saved body, advanced only post-PUT). The golden reconcile test is falsifiable: a monotonic union would yield [old,llm,deploy] and fail both `not.toContain('old')` and `toEqual(['llm','deploy'])` — it genuinely proves add+remove+LLM-persist in ONE call. inlineTagsFromBody runs `parser.configure(tagParserExtensions)` where tagParserExtensions is the SINGLE array markdown.ts also consumes (no fork, no cycle), so materialization is code-region/heading/escape-aware by construction — the Phase-21 render⇄scanner divergence is flipped to AGREEMENT pins (>#deploy now ['deploy'] in BOTH render and materialize). Byte-stability holds: setTags fires ONLY when the tag SET changed (case-insensitive), an unchanged save sends byte-identical frontmatter, and the 409/lock/single-flight loop is structurally unchanged. THE BLOCKER (server-repaired 4-space frontmatter) is reproduced by a failing-before test: the pre-fix readTags regex `/^tags:\\s*\\n((?:  - .*\\n?)*)/m` reads a 4-space block as EMPTY (the * matches zero 2-space items) and pre-fix setTags orphans the 4-space lines into a corrupt scalar + LOSES the llm tag — confirmed by reading 0c76c88^:tag-frontmatter.ts; the indent-agnostic `[ \\t]*-[ \\t]` fix makes the regression tests pass and they FAIL against the pre-fix source. All 7 commits are web/-only (git --stat: zero .go files) ⇒ search/graph/sweep readers structurally unchanged. InlineTagEditor.tsx/.css DELETED (no files on disk); TagBadgeBar wired with ✕ on non-inline tags only. Full web suite 572/572 (53 files), npm run build clean, tsc -p tsconfig.test.json --noEmit exit 0."
  tiers:
    evidence: smtc_honest
    reasoning: manual_review
  provenance:
    - "Read the implementation, not the SUMMARY: hashtag.ts, markdown.ts, tag-reconcile.ts, tag-frontmatter.ts, PageEditor.tsx (runSaver 209-329 + TagBadgeBar wiring 675-689), TagBadgeBar.tsx, and the test suites (tag-reconcile.test.ts, tag-frontmatter.test.ts, PageEditor.test.tsx, TagBadgeBar.test.tsx, livePreview.test.ts agreement block)"
    - "BLOCKER fix-before-test confirmed by reading the PRE-fix source (`git show 0c76c88^:web/src/lib/tag-frontmatter.ts`) and tracing the 4-space-→-empty read + orphaned-line rewrite — the regression tests would fail against it"
    - "Scope: `git show --stat 5c32567 57ac048 fd3332f b687521 5b2edd9 0c76c88 3709c71` → only web/src/* files, zero .go (criterion 6 by construction)"
    - "Removal confirmed: `grep -rn InlineTagEditor|parseInlineTags web/src` → only doc comments remain; `ls web/src/components/InlineTagEditor.*` → no files"
    - "Test runs (from web/): `npx vitest run` → 572/572 (53 files); `npm run build` (tsc -b && vite build) → clean; `npx tsc -p tsconfig.test.json --noEmit` → exit 0 (LSP diagnostics confirmed spurious). Restored internal/web/dist/.gitkeep deleted by vite emptyOutDir."
```

## Verification method

Goal-backward verification from the ED-02b / ED-02c acceptance criteria to the real
materialize path, reading the code (not trusting the SUMMARY) and re-running the
build/tests as independent evidence. The blocker is verified the only way a fix-before
test can be: by reading the **pre-fix source** and confirming the regression test
genuinely fails against it. SMTC's LSP semantic tier is Go/Rust/Java; for this TS phase
the strongest grounding is structural reads + test execution + git `--stat` + the pre-fix
source diff, so the evidence tier is `smtc_honest`, not a Decision-grade cert.

---

## 1. Verification acceptance handshake

| Check | Status |
|-------|--------|
| Inputs complete (CONTEXT, PLAN, SUMMARY, REQUIREMENTS §ED-02b/§ED-02c) | PASS |
| PLAN ↔ SUMMARY consistency (claimed symbols exist + wired) | PASS — `tagParserExtensions`/`inlineTagsFromBody` (hashtag.ts:121,140), `reconcileFrontmatterTags` (tag-reconcile.ts:51), reconcile insert (PageEditor.tsx:240), `TagBadgeBar` (PageEditor.tsx:675), indent-agnostic `readTags`/`setTags` (tag-frontmatter.ts:14,57) all present and wired |
| Goal verifiability (diff formula, byte-stability, code-region exclusion, blocker reproduction are concrete + falsifiable) | PASS |
| Lenses engageable (every criterion checkable by test, code read, or git) | PASS |

No refusal: inputs are consistent and verifiable.

---

## 2. Per-criterion verdict

### Criterion 1 — Reconcile is a DIFF; the golden test is falsifiable (not a vacuous union)

**Truth:** PASS.

**Evidence:**
- **The diff is implemented, not a union.** `reconcileFrontmatterTags` (`tag-reconcile.ts:51-85`):
  `removed = prevInline − curInline` (case-insensitive, `:57-58`); `reconciled =
  (readTags(fm) − removed) ∪ curInline` (`:66-78`, fm-order then inline-only appended). The
  LLM-only tag is in `readTags(fm)` and never in `removed`, so it survives (`:74-77`).
- **The golden test is falsifiable.** `tag-reconcile.test.ts:10-30`: `lastSavedBody="intro
  #old and more"` (prevInline=`[old]`), `currentBody="…\nshipping #deploy"` (curInline=`[deploy]`),
  `frontmatter` tags=`[old, llm]`. Asserts in ONE call: `toContain("deploy")` (added),
  `not.toContain("old")` (removed), `toContain("llm")` (LLM persists), and `toEqual(["llm",
  "deploy"])`. **A monotonic union would produce `[old, llm, deploy]`** → both
  `not.toContain("old")` and `toEqual(["llm","deploy"])` would FAIL. The test genuinely
  proves removals work; it is not vacuous.
- The adversarial suite (`tag-reconcile.adversarial.test.ts`, 52 tests) reinforces with
  idempotence (`reconcile∘reconcile = reconcile`), `!changed ⇒ verbatim`, metamorphic
  order-independence, and a 3-step diff-window.

### Criterion 2 — Materialize on save (add / remove / LLM-persists) via the savePage payload

**Truth:** PASS.

**Evidence:**
- **Wired before the PUT.** `runSaver` (`PageEditor.tsx:229-260`) captures `sentBody` /
  `let sentFrontmatter`, calls `reconcileFrontmatterTags({ lastSavedBody:
  lastSavedBody.current, currentBody: sentBody, frontmatter: sentFrontmatter })`, reassigns
  `sentFrontmatter = rec.frontmatter`, and PUTs that (`:256-260`). `lastSavedBody.current` is
  advanced only AFTER a successful PUT (`:311`), so the diff window is exactly "since the
  server's copy".
- **Integration tests assert the payload** (`PageEditor.test.tsx`, `fmOfLastSave()` reads the
  real `savePage` mock arg): `materializes a body #deploy` (`:289-304`) → `tags:` contains
  `deploy`; `removes a deleted #deploy AND keeps an LLM-only tag` (`:306-333`) → after add both
  `llm`+`deploy`, after delete `not.toContain("deploy")` but still `toContain("llm")`.
- **Found by tag search/graph with no indexer change:** the materialized `deploy` lands in the
  same frontmatter `tags:` sink the existing (unchanged — criterion 6) search/graph/sweep
  already read. Architectural inference grounded in the zero-Go-file change set, not a runtime
  search test.

### Criterion 3 — Code-region-aware via the SHARED parser; Phase-21 divergence CLOSED

**Truth:** PASS.

**Evidence:**
- **One parser, no fork.** `tagParserExtensions = [Table, Strikethrough, TaskList, Autolink,
  hashtagExtension]` is the single source (`hashtag.ts:121-127`). `markdown.ts:11,25-28`
  imports it and passes it to `markdown({ base: commonmarkLanguage, extensions:
  tagParserExtensions })`. `inlineTagsFromBody` uses `parser.configure(tagParserExtensions)`
  (`hashtag.ts:133,140-150`). `hashtag.ts` imports the GFM bits straight from `@lezer/markdown`
  (`:36`), never from `markdown.ts` → **no import cycle.** Render and materialize cannot fork.
- **Divergence flipped to agreement.** `livePreview.test.ts:633-656` ("render⇄materialize
  AGREEMENT — Phase-21 gap closed"): `renderedTags(">#deploy") === ["deploy"]` AND
  `inlineTagsFromBody(">#deploy") === ["deploy"]` — the exact Phase-21 pin (`parseInlineTags`
  returned `[]` for `>#deploy`) is now an equality. The space-after-marker form `"> #deploy"`
  agrees too.
- **Code/heading/escape negatives hold.** `tag-reconcile.test.ts:112-122`: a `#secret` inside
  ```` ``` ```` fence → `changed=false`, set unchanged (not materialized). `hashtag.test.ts`
  (42 tests) re-pins the corpus on the live `inlineTagsFromBody`: `#1984`/`foo#bar`/`\#esc` →
  `[]`, `#a/b` → `["a/b"]`, de-dupe `#deploy #deploy` → `["deploy"]`.

### Criterion 4 — Byte-stable / no thrash; 409/lock/single-flight UNCHANGED

**Truth:** PASS.

**Evidence:**
- **setTags only on a real change.** `changed = !sameSet(readTags(fm), reconciled)`
  (`tag-reconcile.ts:80`); the return is `changed ? setTags(...) : args.frontmatter`
  (`:82`) — an unchanged set returns the **original string verbatim**. Pinned for block,
  flow (no flow→block fold), and case-collision forms (`tag-reconcile.test.ts:33-69`), and at
  the integration level: a no-op save sends byte-identical frontmatter
  (`PageEditor.test.tsx:335-352`), a quoted block tag stays byte-identical (`:407-421`), and a
  reconciled autosave fires **exactly one** PUT — no re-fire loop (`:359-400`, because
  `lastSavedFrontmatter.current = sentFrontmatter` stores the reconciled value at `:312`).
- **409/lock/single-flight intact.** I read the whole `runSaver` (`:209-329`): the lock gate
  (`:215`), the single-flight `saving.current` guard (`:216-217`), the coalescing while-loop
  (`:223-227`), the 409 → conflict-dialog branch (`:262-291`, `baseRevision` deliberately not
  advanced), the read-back/advance (`:300-312`), and the lock refresh (`:317`) are unchanged.
  The reconcile is purely additive between the capture and the PUT — it touches none of these
  branches.

### Criterion 5 — THE BLOCKER: indent-agnostic tag I/O + a genuine fix-before regression test

**Truth:** PASS. *(This is the load-bearing check — a fix without a failing-before test is unverified. It is verified.)*

**The original corruption, confirmed against the pre-fix source** (`git show
0c76c88^:web/src/lib/tag-frontmatter.ts`):
- Pre-fix `readTags` matched `/^tags:\s*\n((?:  - .*\n?)*)/m`. The item group requires **exactly
  two leading spaces** then `- `. The server re-marshals frontmatter via yaml.v3 at **4-space**
  indent when `Repair` adds a missing required field (no `SetIndent`). Against a 4-space block,
  `^tags:\s*\n` matches `tags:\n`, then `(?:  - …)*` matches **zero** items (the `*` allows
  empty) → `tagsSection[1] === ""` → `readTags` returns **`[]`**. A repaired page's tags read as
  EMPTY.
- Consequence: on the first autosave the reconcile believed the page had no tags. With any inline
  tag present, `changed=true` → pre-fix `setTags` replaced only `tags:\n` (its block regex also
  matched zero 4-space items), leaving `    - deploy` / `    - llm` **orphaned** below the new
  header — which yaml.v3 then folds into a corrupt scalar — **and the LLM-only `llm` tag is
  lost** (it was never in the phantom-empty read). Genuine data corruption **and** data loss.

**The fix** (`tag-frontmatter.ts:14,57`): the item match is now `[ \t]*-[ \t]` (any indent /
tab), and `setTags`'s `blockRe` consumes the **whole** old block at any indent (`:57-59`), plus
the block branch now strips surrounding quotes like the flow branch (`:37-41`).

**The regression tests genuinely fail against the pre-fix code** (I traced each against the
pre-fix regex above):
- `tag-frontmatter.test.ts:19-24` — `readTags("…tags:\n    - deploy\n    - llm\n")` expects
  `["deploy","llm"]`; pre-fix returns `[]` → **FAILS** (reproduces the phantom-empty read).
- `tag-frontmatter.test.ts:53-59` — `setTags("…tags:\n    - old\n    - x\n", ["a","b"])` expects
  the clean `…tags:\n  - a\n  - b\n`; pre-fix produces `…tags:\n  - a\n  - b\n    - old\n    -
  x\n` (orphaned lines) → **FAILS** (reproduces the corruption).
- Integration: `PageEditor.test.tsx:463-478` — a server-repaired 4-space page with a matching
  inline tag saves **byte-identical** (no phantom-empty → no rewrite → no corruption).

The client-only scope is correct: the broken module was the client `readTags` (it did not
reflect the server's 4-space store); the server was left unchanged (within web scope; no
indexer/server change). The fix repairs the broken precondition so the reconcile's
postcondition can hold on repaired pages.

### Criterion 6 — No indexer / server change (client-only)

**Truth:** PASS.

**Evidence:** `git show --stat` for all 7 commits touches **only** `web/src/*` files — zero
`.go`, zero `internal/`, zero server/indexer files. Search, graph (`page_tags`), tag-sweep, and
v1.3 tag-management keep reading frontmatter unchanged; the materialization writes to that same
frontmatter sink. There is no second server-side `#tag` parser (locked decision honored).

### Criterion 7 — UI: InlineTagEditor deleted; TagBadgeBar with ✕ on non-inline only

**Truth:** PASS.

**Evidence:**
- **InlineTagEditor gone.** `ls web/src/components/InlineTagEditor.*` → no files (`.tsx`+`.css`
  DELETED). `grep -rn InlineTagEditor web/src` → a single **doc comment** in `TagBadgeBar.tsx:2`
  ("replaces the InlineTagEditor chip field"), no import or usage. (`parseInlineTags` likewise
  survives only in comments — the function is deleted.)
- **TagBadgeBar wired.** `PageEditor.tsx:675-689`: `<TagBadgeBar body={body}
  frontmatter={frontmatter} onRemoveTag={…} />`. `onRemoveTag` reads the **latest**
  `frontmatterRef.current` (not a stale render closure), does a byte-stable
  `setTags(cur, readTags(cur).filter(t ≠ tag))`, syncs `setFrontmatter`/`frontmatterRef`, and
  `scheduleAutosave()` so the removal persists.
- **Effective set + ✕ policy.** `TagBadgeBar.tsx:30-50`: `effective = union(readTags(fm),
  inlineTagsFromBody(body))`, fm-order then inline-only, de-duped ci; inline-origin → no ✕,
  non-inline → ✕ calling `onRemoveTag`. `"#"+tag` are inert JSX text nodes (no innerHTML).
  `TagBadgeBar.test.tsx`: both badges render, only `llm` (non-inline) has Remove, click calls
  `onRemoveTag("llm")`, `#Deploy`-vs-`deploy` de-dupes to one inline badge, empty renders
  nothing.

---

## 3. ED-02b / ED-02c acceptance criteria (REQUIREMENTS.md)

| REQ | Criterion | Status |
|-----|-----------|--------|
| ED-02b | `removedInline = inlineTags(lastSavedBody) − inlineTags(currentBody)`; `tags = (priorFM − removedInline) ∪ inlineTags(currentBody)`; client `setTags`; NOT a monotonic union | PASS (§2 C1) |
| ED-02b | Add `#deploy` → fm gains `deploy`; delete → loses it; an LLM-applied non-inline tag persists | PASS (§2 C2) |
| ED-02b | Search/graph/sweep/tag-management read frontmatter unchanged (no indexer change) | PASS (§2 C6) |
| ED-02b | Normalization (trim, case-fold, collision→one); server re-validates | PASS (`tag-reconcile.ts:23,57-78`; case-collision pin `tag-reconcile.test.ts:58-69`) |
| ED-02b | Golden test: add + remove + LLM-persist in a single save | PASS — falsifiable (§2 C1) |
| ED-02c | `InlineTagEditor` chip field removed from `PageEditor.tsx` | PASS (§2 C7) |
| ED-02c | Read-only badge bar; non-inline = `fmTags − inlineTags(body)` each ✕ (byte-stable write); inline-origin no ✕ | PASS (§2 C7) |

---

## 4. Build / test verification (re-run independently from `web/`)

| Command | Result |
|---------|--------|
| `npx vitest run` (full web suite) | PASS — **572 passed (572), 53 files**. Matches the SUMMARY exactly. No 409/lock/single-flight regression. |
| `npm run build` (`tsc -b && vite build`) | PASS — clean, `built in 428ms`. The `>500 kB` warning targets the pre-existing `index-*.js` (1,450.81 kB) app bundle — same advisory Phase 21 noted; not introduced here. |
| `npx tsc -p tsconfig.test.json --noEmit` | PASS — **exit 0**, no errors. Confirms the editor/LSP diagnostics (`toBeInTheDocument` / "Cannot find module './hashtag'") the lead flagged are **spurious** (single-file LSP using the wrong tsconfig). |

*Housekeeping:* `npm run build` deleted `internal/web/dist/.gitkeep` (vite `emptyOutDir`); I
restored it (`git checkout -- internal/web/dist/.gitkeep`). The working tree is clean; not a
Phase-22 defect.

---

## 5. The open cosmetic item — `setTags(fm, [])` → `tags:\n\n`

The engineer flagged that removing the **last** tag leaves a dangling `tags:\n\n`
(`tags: null`) rather than dropping the key. **My read: acceptable follow-up, NOT a Phase-22
gap.** I traced it rather than taking the SUMMARY's word:

- **Reachable** in Phase 22 (delete the last inline `#tag`, or ✕ the last non-inline tag).
- **It converges and does not thrash.** `readTags("tags:\n\n")` returns `[]` (no item-line
  match → flow miss → scalar empty → `[]`). The next reconcile then computes `changed =
  !sameSet([], []) = false` → no `setTags` → byte-identical. No 409-thrash, terminates.
- **It does not corrupt other keys.** `tags:\n\ndescription: hi` parses as `tags: null,
  description: "hi"` (valid YAML; the blank line is cosmetic). Re-adding a tag later recovers a
  clean block via the scalar branch (`setTags` → `tags:\n  - new\n\ndescription: hi`).
- **No requirement covers empty-tags formatting** — ED-02b/ED-02c say nothing about it.

Recommendation: track as a one-line cosmetic polish for a future phase (drop the `tags:` key
or emit `tags: []` when the set empties). Not blocking.

---

## 6. Conclusion

**Verdict: PASS.** Phase 22 delivers the ED-02b / ED-02c goal: on save the client reconciles
body `#tags` into the canonical frontmatter `tags:` as a true **diff** (add + remove +
LLM-persist), proven by a falsifiable golden test that a monotonic union would fail;
materialization shares the render's Lezer parser (no fork, no cycle), so it is code-region/
heading/escape-aware by construction and the Phase-21 render⇄scanner divergence is closed and
re-pinned as agreement; reconcile is byte-stable (setTags only on a real change) and the
409/lock/single-flight loop is untouched; the server-repaired **4-space-frontmatter corruption
BLOCKER is reproduced by a genuine failing-before regression test** (confirmed against the
pre-fix source) and fixed with indent-agnostic, quote-aware tag I/O; all 7 commits are
client-only (zero Go files); and `InlineTagEditor` is deleted in favour of a read-only
`TagBadgeBar` with ✕ on non-inline tags only. Full suite 572/572, build clean, test-tsconfig
type-check exit 0.

No gaps found. No refusal needed. The one cosmetic item (`tags:\n\n` on the last removal) is a
converging, non-corrupting follow-up, not a Phase-22 defect.
