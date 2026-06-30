---
author: engineer
responsible: engineer
phase: "22"
status: complete
milestone: v1.4
---

# Phase 22: Model-B Materialization + Tag UI — SUMMARY

**Plan file:** `.planning/phases/22-model-b-materialization-tag-ui/22-01-PLAN.md`
**Requirements:** ED-02b, ED-02c
**Commits (on `main`):**
- `5c32567` feat(web): code-region-aware inlineTagsFromBody via shared Lezer parser
- `57ac048` feat(web): pure tag reconciliation engine (reconcileFrontmatterTags)
- `fd3332f` feat(web): reconcile body #tags into frontmatter on save (runSaver)
- `b687521` feat(web): TagBadgeBar — read-only effective-tag display (ED-02c)
- `5b2edd9` feat(web): wire TagBadgeBar into PageEditor; delete orphaned InlineTagEditor
- `0c76c88` fix(web): harden tag I/O — indent-agnostic + quote-aware; persist badge removal *(gate disposition)*
- `3709c71` perf(web): memoize TagBadgeBar body parse *(gate disposition)*

## What was built

On **save**, body `#tags` are reconciled into the canonical frontmatter `tags:` —
**client-side**, as a **DIFF (add AND remove)** — reusing the Phase-21 Lezer parse (no
second parser, no indexer change). The manual `InlineTagEditor` chip field is replaced
by a minimal read-only `TagBadgeBar` whose ✕ removes only non-inline (LLM/imported) tags.

- **`hashtag.ts`** — exports `tagParserExtensions` (the SINGLE shared
  `[Table, Strikethrough, TaskList, Autolink, hashtagExtension]` list) and
  `inlineTagsFromBody(body): string[]`, a standalone `parser.configure(tagParserExtensions)`
  that walks the tree collecting `Hashtag` nodes (lower-cased, de-duped). Because it runs
  the SAME Lezer extension set as the render, it is code-region/heading/escape-aware **by
  construction**. **`parseInlineTags` (the Phase-21 string scanner) was DELETED** — it had
  zero non-test consumers and `inlineTagsFromBody` supersedes it.
- **`markdown.ts`** — now imports `tagParserExtensions` from `hashtag.ts` (D-01b: one
  source of truth; render and materialize can never fork). No import cycle (`hashtag.ts`
  imports only `@lezer/markdown`).
- **`tag-reconcile.ts` (new)** — pure `reconcileFrontmatterTags({lastSavedBody, currentBody,
  frontmatter}) → {frontmatter, changed}`: `removedInline = inline(lastSavedBody) −
  inline(currentBody)`; `reconciled = normalize((readTags(frontmatter) − removedInline) ∪
  inline(currentBody))`; byte-stable guard — `setTags` is called ONLY when the tag SET
  changed, else the frontmatter is returned verbatim.
- **`PageEditor.tsx` `runSaver`** — the reconcile is woven BETWEEN the `sentBody`/
  `sentFrontmatter` capture and the `savePage` PUT, diffing against `lastSavedBody.current`
  (the PRIOR saved body, advanced only after a successful PUT). `sentFrontmatter` became a
  `let`; on `changed` it is reassigned + `setFrontmatter`/`frontmatterRef` synced so the
  next loop's `frontmatterRef === lastSavedFrontmatter` check holds (no re-fire). The
  409/lock/single-flight logic is untouched.
- **`TagBadgeBar.tsx` (new)** — read-only effective-tag pills: `union(readTags(frontmatter),
  inlineTagsFromBody(body))`, fm order then inline-only appended, de-duped ci. Inline-origin
  tags carry no ✕; non-inline tags get an ✕ → `onRemoveTag`. `"#" + tag` are inert JSX text
  nodes (no innerHTML). Replaces `InlineTagEditor` in `PageEditor` (644–655).

## Fate of `parseInlineTags` and `InlineTagEditor`

- **`parseInlineTags`: DELETED.** Re-grepped before removal — referenced only in test files
  + its own definition, never in the render/materialize path. `inlineTagsFromBody` (Lezer,
  code-region-aware) replaces it. Its grammar corpus in `hashtag.test.ts` was migrated onto
  the live `inlineTagsFromBody`, and the Phase-21 "render⇄scanner divergence" pins (in
  `hashtag.test.ts` + `livePreview.test.ts`) were **flipped to AGREEMENT** assertions —
  documenting the closed divergence (code regions excluded; `>#deploy` now extracts
  `["deploy"]`, matching the render).
- **`InlineTagEditor`: DELETED** (`.tsx` + `.css`). Re-grepped — imported only by
  `PageEditor` (now replaced by `TagBadgeBar`); zero remaining consumers. **Note:** the plan
  listed `InlineTagEditor.test.tsx` for deletion but no such file existed on disk (a no-op).
  `suggestTags` in `api/client` (used by the old field's Suggest button) stays — still
  consumed by `TagReviewView`/`TagSuggest` and reserved for Phase 23.

## Gate outcomes

Both gates ran. Hard-bar phase: byte-stability + code-region awareness + the
add/remove/LLM-persist reconcile are load-bearing correctness/data-integrity invariants.

### `anvil-adversarial-tester`
Verdict: **1 defect family reproduced (quoted block tag), fixed; 52 durable adversarial
tests added** (quoted/scalar/empty/no-key byte-stability, code-region negative corpus,
diff-window 3-step, and properties: idempotence/loop-termination, LLM-persistence,
`!changed ⇒ verbatim`, metamorphic order-independence) + 2 PageEditor integration tests
(no-re-fire, quoted byte-identical). All invariants otherwise HELD. *(Delivery note: the
worker's edits landed in the working tree, not its branch — I reviewed each file directly
and accepted them; the isolated branch carried only a stray `node_modules` gitlink, not
merged.)*

### `anvil-code-reviewer` (`/tmp/anvil-spawn-out/22-REVIEW.md`)
Verdict: **FIX FIRST — 1 blocker, 1 major, 3 minors.** Independently confirmed the
single-flight/coalescing loop, the 409/conflict path, the held-by-other gate, the
`let sentFrontmatter` reassignment safety, the absence of an import cycle, the orphan
deletion, and the byte-stability guard are all sound. The blocker is a real
data-corruption path (below), which I **verified independently** with a Go probe.

## Review disposition

| Finding | Source | Severity | Disposition |
|---------|--------|----------|-------------|
| **Indent-sensitivity corrupts tags on server-repaired frontmatter.** The Go server re-marshals frontmatter via yaml.v3 at **4-space** indent when `Repair` adds a missing field (`internal/okf/emit.go`, no `SetIndent`). The client `readTags` 2-space-only regex read that block as **empty**, so the reconcile thought a repaired page had no tags and corrupted it on the first autosave (orphaned 4-space line folded into a scalar). | code-reviewer | **BLOCKER** | **FIXED (`0c76c88`).** Verified independently with a Go probe: `yaml.Marshal` of a `tags` node emits `tags:\n    - deploy` (4-space). Made `readTags`/`setTags` **indent-agnostic** (`[ \t]*-[ \t]` item match) so `readTags` reads any indent and `setTags` consumes the WHOLE old block on replace. Pinned by `tag-frontmatter.test.ts` (the exact corruption case) + a PageEditor integration test (4-space page saves byte-identical). Client-only fix (the broken module); the server was left unchanged (within web scope; no indexer/server change). |
| Same root: repaired-frontmatter tags vanish from `TagBadgeBar` (un-removable). | code-reviewer | MAJOR | **FIXED by the same indent-agnostic `readTags`.** A 4-space LLM tag now renders a badge with a working ✕. |
| `TagBadgeBar` runs a full Lezer parse per render (unmemoized). | code-reviewer | MINOR | **FIXED (`3709c71`).** Wrapped in `useMemo(() => …, [body])`. |
| The badge ✕ does not schedule a save → a lone removal is lost on navigate-away. | code-reviewer | MINOR | **FIXED (`0c76c88`).** Added `scheduleAutosave()` to `onRemoveTag`. A deliberate step **beyond** D-05's literal "mirror the old `onTagsChange`" — ED-02c says non-inline tags "remain removable", which a removal that vanished would not honour. Pinned by a PageEditor ✕-persist integration test. |
| `readTags` block branch kept surrounding quotes (`- "deploy"` → `'"deploy"'`) while flow stripped them → spurious churn + duplicate. (The OKF corpus ships `- "quoted-tag"`.) | adversarial-tester | DEFECT (≈MAJOR) | **FIXED (`0c76c88`).** Block branch now strips quotes like the flow branch. Pinned by `tag-frontmatter.test.ts` + the adversarial quoted-tag byte-stability suite + a PageEditor quoted-byte-identical integration test. |
| Any tag-set change lowercases the case of all surviving frontmatter tags (`ProjectX` → `projectx`). | code-reviewer | MINOR | **INTENDED — no change.** D-04 mandates case-fold normalization; the server re-validates. The byte-stable guard means it never fires on a no-op (an unrelated tag's case is only touched when the set genuinely changes). |
| `setTags(fm, [])` leaves a dangling `tags:\n\n` (`tags: null`) rather than dropping the key. | code-reviewer | MINOR | **PRE-EXISTING — no change (noted).** Reachable now via "remove the last tag"; converges (`readTags` reads it as `[]`, the next reconcile is a no-op) and is cosmetic, not corrupting. Out of the phase's scope (no requirement on empty-tags formatting); flagged for a possible follow-up. |

## Conditional-gate skip-witness

None — this is a hard-bar phase; both `[GATE]` conditions held, so both gates fired
(no `not_applicable` skip-witness is emitted).

## Files modified

| File | Change |
|------|--------|
| `web/src/lib/cm/hashtag.ts` | Added `tagParserExtensions` + standalone `inlineTagsFromBody` (Lezer, code-region-aware). **Deleted `parseInlineTags`.** Header rewritten (two consumers share ONE parser; Phase-21 divergence closed). |
| `web/src/lib/cm/markdown.ts` | Imports `tagParserExtensions` from `hashtag.ts` (one source of truth). |
| `web/src/lib/cm/hashtag.test.ts` | Migrated grammar corpus onto `inlineTagsFromBody`; flipped code-region / `>#tag` divergence cases to agreement. 42 tests. |
| `web/src/lib/cm/livePreview.test.ts` | Swapped `parseInlineTags` → `inlineTagsFromBody`; the "render⇄scanner divergence" block now asserts AGREEMENT (gap closed). 54 tests. |
| `web/src/lib/tag-reconcile.ts` (new) | Pure `reconcileFrontmatterTags` (diff add/remove; byte-stable guard). |
| `web/src/lib/tag-reconcile.test.ts` (new) | Golden add+remove+LLM-persist in one call; byte-stability (block/flow/scalar/case); code-region safety; 4-space (BLOCKER) no-churn + clean-rewrite. 11 tests. |
| `web/src/lib/tag-reconcile.adversarial.test.ts` (new) | Adversarial-tester suite (accepted): quoted/scalar/empty byte-stability, code-region negatives, diff-window, idempotence/persistence/order properties. 52 tests. |
| `web/src/lib/tag-frontmatter.ts` | `readTags`/`setTags` made **indent-agnostic + quote-aware** (BLOCKER + DEFECT fix). |
| `web/src/lib/tag-frontmatter.test.ts` (new) | Indent (2/4/tab) + quote + clean-replace, incl. the exact corruption case. 12 tests. |
| `web/src/routes/PageEditor.tsx` | Reconcile woven into `runSaver` before the PUT; `InlineTagEditor` → `TagBadgeBar`; `onRemoveTag` (ref-based byte-stable removal + `scheduleAutosave`). |
| `web/src/routes/PageEditor.test.tsx` | Integration: add, remove+LLM-persist, byte-identical no-op, no-re-fire, quoted byte-identical, 4-space byte-identical, badge-✕-persists. 12 tests. |
| `web/src/components/TagBadgeBar.tsx` / `.css` / `.test.tsx` (new) | Read-only effective-tag bar; ✕ on non-inline only; memoized parse. 4 tests. |
| `web/src/components/InlineTagEditor.tsx` / `.css` | **DELETED** (orphan). |

## Verification

- `npx vitest run` (full web suite, from `web/`) — **572/572 PASS**, 53 files (no consumer
  regression; the existing 409/lock/single-flight PageEditor tests still green).
- Tag suites — `tag-frontmatter` 12, `tag-reconcile` 11, `tag-reconcile.adversarial` 52,
  `TagBadgeBar` 4, `PageEditor` 12, `hashtag` 42, `livePreview` 54.
- `npm run build` (tsc -b && vite build) — **clean** (the >500 kB main-chunk warning is
  pre-existing, not introduced here).
- BLOCKER independently verified: Go probe shows yaml.v3 re-marshals `tags` at 4-space;
  the fix's regression tests fail against the pre-fix regex (confirmed via probe).

## Hoare-triple summary

- **Q (postcondition):** after a save, `frontmatter.tags = (priorFrontmatter −
  inlineRemovedFromBody) ∪ inlineInBody`; an LLM-only tag persists; an unchanged set leaves
  the frontmatter bytes verbatim; a `#tag` inside code never materializes; the badge bar
  shows the effective set with ✕ only on non-inline tags.
- **S (code shape):** `reconcileFrontmatterTags` computes the diff against `lastSavedBody`
  and rewrites via `setTags` only when the set changed; `runSaver` applies it before the PUT.
- **P (precondition):** `inlineTagsFromBody` is the render's parser; `readTags`/`setTags`
  correctly read/write the on-disk YAML at **any** indent and quote form; `lastSavedBody`
  holds the prior saved body.
- **S establishes Q:** the diff (not a union) makes removals work; `readTags − removedInline`
  keeps the LLM tag; the `changed` guard makes a no-op byte-stable; the shared parser makes
  code-region exclusion structural. **The BLOCKER fix repaired a broken P** — `readTags` did
  NOT reflect the server's 4-space store, so `S` could not establish `Q` on repaired pages;
  the indent-agnostic read restores the precondition.
- **Invariant (object-state):** across the editor's lifetime, the saved frontmatter's tag
  set is a pure function of `(body, priorFrontmatter, lastSavedBody)`; `reconcile∘reconcile =
  reconcile` (idempotent → the autosave loop terminates, no 409-thrash); a save with an
  unchanged set issues exactly one byte-identical PUT (no re-fire).

```yaml
handoff:
  record_author: engineer
  accountable_owner: engineer
  id: H-22-execute
  from: engineer
  to: architect
  artifact: .planning/phases/22-model-b-materialization-tag-ui/22-01-PLAN-SUMMARY.md
  intent: "Phase 22 execution complete — client-side reconcile-as-diff materializes body #tags into frontmatter on save (add/remove/LLM-persist, byte-stable), code-region-aware via the shared Lezer parser (Phase-21 divergence closed), InlineTagEditor replaced by read-only TagBadgeBar + deleted. Both gates run + dispositioned; a server-repaired 4-space-frontmatter corruption BLOCKER was found, independently verified, and fixed (indent-agnostic tag I/O)."
  status: ready
```

```yaml
evidence:
  record_author: engineer
  evidence_producer: engineer
  commissioned_by: engineer
  accountable_owner: engineer
  id: E-22-test-run
  subject_ref: "npx vitest run (web/) + npm run build"
  kind: test_result
  issuance:
    kind: producer_signed
  warrant:
    kind: external_cited
    scope:
      - "carry=full web suite 572/572 PASS (53 files)"
      - "carry=golden reconcile (add+remove+LLM-persist in one call) PASS"
      - "carry=4-space BLOCKER + quoted-tag DEFECT regression tests PASS"
      - "carry=npm run build (tsc -b && vite build) clean"
    rationale: "exercises reconcileFrontmatterTags, inlineTagsFromBody (code-region corpus), the runSaver weave (add/remove/no-re-fire/byte-identical incl. repaired 4-space + quoted frontmatter), and TagBadgeBar against a headless EditorView/RTL under jsdom; full suite confirms no 409/lock/single-flight regression"
  freshness:
    status: fresh
    checked_against: HEAD
```

```yaml
gate_result:
  record_author: engineer
  status: applicable
  gate: anvil-code-reviewer
  phase: "22"
  basis: "Pre-handoff diff review (every SUMMARY, not scope-gated). Verdict FIX FIRST — 1 blocker (4-space repaired-frontmatter corruption, independently verified + fixed), 1 major (badge display, same fix), 3 minors (2 fixed, 2 intended/pre-existing). All dispositioned above. Artifact: /tmp/anvil-spawn-out/22-REVIEW.md"
```

```yaml
gate_result:
  record_author: engineer
  status: applicable
  gate: anvil-adversarial-tester
  phase: "22"
  basis: "Hard-bar phase (byte-stability + code-region awareness + add/remove/LLM-persist data integrity). 1 defect family reproduced (quoted block tag → churn/duplicate), fixed; 52 adversarial unit tests + 2 PageEditor integration tests added (accepted after direct review). All other invariants held."
```
