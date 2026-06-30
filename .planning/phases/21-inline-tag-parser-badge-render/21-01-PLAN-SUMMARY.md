---
author: engineer
responsible: engineer
phase: "21"
status: complete
milestone: v1.4
---

# Phase 21: Inline `#tag` Parser + Badge Render — SUMMARY

**Plan file:** `.planning/phases/21-inline-tag-parser-badge-render/21-01-PLAN.md`
**Requirement:** ED-02a
**Commits (on `main`):**
- `788aa89` feat(web): add Hashtag inline parser + parseInlineTags grammar
- `a5bea08` feat(web): render inline #tag as byte-stable badge on both CM6 surfaces
- `a2d13d8` test(web): harden #tag corpus + correct the render⇄scanner drift claim

## What was built

A client-only `@lezer/markdown` inline-parser extension that recognizes body `#tag`
tokens, plus a badge **decoration** that renders them as pills on both CM6 surfaces
(live editor + read mode) — **decoration-only and byte-stable**. No frontmatter writes
and no navigation (display-only badge; Phase 22 owns materialization).

- **`hashtag.ts` (new)** — `hashtagExtension: MarkdownConfig` registers a `Hashtag`
  inline node (`parseInline: [{ name, parse, before: "Link" }]`, `defineNodes`).
  `parseHashtag` fires only on `#`, requires a start-of-section/whitespace start
  boundary, scans the body `[A-Za-z0-9_/-]+` (≥1 non-digit, trailing `/` trimmed) via
  a shared `scanBody` helper, and `addElement(elt("Hashtag", pos, end))`. Also exports
  the pure `parseInlineTags(text): string[]` (de-duped, lower-cased) for Phase 22 reuse.
- **`markdown.ts`** — composes `hashtagExtension` into `markdownExtension`'s extensions
  array alongside the four GFM extensions (purely additive; no server-side counterpart).
- **`widgets.ts`** — `HashtagWidget extends WidgetType`: `toDOM` builds a single
  `<span class="cm-hashtag">` via `createElement` + `textContent` (`"#" + tag`, the `#`
  a literal prefix); `eq` compares the label; `ignoreEvent()` returns `false`.
- **`livePreview.ts`** — a `case "Hashtag":` in `buildDecorations` emits
  `Decoration.replace({ widget: new HashtagWidget(tag) })` over `[from,to)` UNLESS
  `isActive(node.from)` (active-line reveal / source mode show raw `#tag`; read mode
  keeps `activeLines` empty so it always badges). The widget range is **NOT** pushed
  into `widgetRanges`, so it is non-atomic (Pitfall 1 — never trap the cursor).
- **`theme.ts`** — a `.cm-hashtag` pill rule (accent text on `--color-accent-soft`,
  `--radius-sm`, `--space-xs` padding, `cursor: text`); font-size/line-height inherited
  so the reveal is layout-neutral (Pitfall 2).

## Gate outcomes

Both gates ran (hard-bar phase: the negative corpus + byte-stability + no-innerHTML +
non-atomic are load-bearing security/correctness invariants).

### `anvil-adversarial-tester` (worktree branch `anvil/spawn/anvil-adversarial-tester-p21-…`)
Verdict: **1 real defect reproduced; 48 adversarial cases added.** No input panicked,
hung, or mutated the document — byte-stability, no-innerHTML, and non-atomicity held
under every adversarial input (punctuation/unicode/surrogate-pair boundaries, nested
slashes, code-span/heading/list/blockquote/link interplay, tag-dense byte-stability).
The one defect was a **consistency** gap (below), not a safety gap. Its durable pins
were folded into `hashtag.test.ts` + `livePreview.test.ts` (commit `a2d13d8`); the
intentionally-RED drift fixture was dispositioned (below) into a green known-divergence
pin so the gap stays visible without misrepresenting it as a held invariant.

### `anvil-code-reviewer` (`/tmp/anvil-spawn-out/21-REVIEW.md`)
Verdict: **SHIP — minor notes; no blocker/major.** Independently confirmed
byte-stability (zero transactions), the no-innerHTML DOM invariant, non-atomic cursor
safety, `parseInlineTags` termination (strictly-increasing `i`, O(n)), index-space
consistency of `scanBody` across the two callers, single-line reveal correctness
(`isActive(from)` == `rangeActive(from,to)` since `isBody` excludes `\n`/`\r`), and
the structural code/heading exclusion. Surfaced the SAME drift finding as the
adversarial gate plus two nits.

## Review disposition

| Finding | Source | Severity | Disposition |
|---------|--------|----------|-------------|
| `parseInlineTags` "byte-for-byte / can never drift" claim is false: it lacks the render's structural **code-region exclusion** AND its **block-section start boundary** (`cx.offset`), so render and scanner disagree on `` `a #b` `` and `>#deploy` | both gates | SHOULD-FIX | **FIXED (comments) + DOCUMENTED + PINNED.** The claim was the actual defect. Corrected `hashtag.ts` comments to state the truth: `scanBody` shares only the **body alphabet**; the two render-only behaviors are explicitly out of `parseInlineTags`' scope and reconciled in Phase 22 (ED-02b). `parseInlineTags` is **unused in Phase-21 render**, so zero Phase-21 runtime impact. Fully matching is infeasible for a pure string scanner (needs block structure) → correctly Phase-22's owned scope. Both divergences pinned (`hashtag` "KNOWN scope gap" + `livePreview` "render⇄scanner divergence") so Phase 22 inherits a concrete failing target. |
| Punctuation-only bodies (`#-`, `#_`) qualify as tags (≥1-non-digit counts `_`/`-`) — unpinned boundary | code-reviewer | NIT | **PINNED as intended.** Matches the literal requirement ("≥1 non-digit"; `_`/`-` are non-digits). Locked by `hashtag.test.ts` cases `#-`→`-`, `#_`→`_` and the render corpus. No code change (spec-compliant; not tightened to "≥1 letter" — that would exceed the requirement). |
| Trailing-slash trim leaves a dangling literal `/` beside the badge (`#a/` → pill `#a` + text `/`) | code-reviewer | NIT | **PINNED as intended.** Trailing `/` is a separator, not a segment (grammar D-02). Byte-stable. Locked by `#a/` → `a` in both test files. |

## Conditional-gate skip-witness

None — this is a hard-bar phase; both `[GATE]` conditions held, so both gates fired
(no `not_applicable` skip-witness is emitted).

## Files modified

| File | Change |
|------|--------|
| `web/src/lib/cm/hashtag.ts` (new) | `hashtagExtension` (Hashtag inline parser, `before:"Link"`), `scanBody` (shared body-alphabet scan), `parseHashtag`, pure `parseInlineTags`. Header + SCOPE NOTE document the two render-only divergences reconciled in Phase 22. |
| `web/src/lib/cm/hashtag.test.ts` (new) | Pure-grammar corpus (positive/negative) + folded adversarial pins: body/slash edge cases, start-boundary edges, unicode/surrogate, KNOWN code-region scope gap. 41 tests. |
| `web/src/lib/cm/markdown.ts` | Compose `hashtagExtension` into `markdownExtension.extensions`. |
| `web/src/lib/cm/widgets.ts` | `HashtagWidget` (createElement + textContent, no innerHTML). |
| `web/src/lib/cm/widgets.test.ts` | `HashtagWidget` DOM unit tests: span/class/textContent, single-inert-text-node (no-innerHTML guard), `eq`, `ignoreEvent`. |
| `web/src/lib/cm/livePreview.ts` | `case "Hashtag":` — replace-widget badge with `isActive` reveal; NOT added to `widgetRanges` (non-atomic). |
| `web/src/lib/cm/livePreview.test.ts` | ED-02a corpus (positive/negative/reveal/byte-stable) + folded adversarial render corpus (structural boundaries, span validity, non-atomic atomicRanges=0, byte-stability under churn) + the dispositioned render⇄scanner divergence pin. 54 tests. |
| `web/src/lib/cm/theme.ts` | `.cm-hashtag` pill rule (design tokens, layout-neutral). |

## Verification

- `npx vitest run src/lib/cm/` (from `web/`) — **147/147 PASS** (hashtag 41, livePreview 54, widgets 14, + sibling suites).
- `npx vitest run` (full web suite) — **485/485 PASS**, 49 files (no regression in touched-file consumers).
- `npm run build` (tsc -b && vite build) — **clean** (the >500 kB main-chunk warning is pre-existing, not introduced here).

## Hoare-triple summary

- **Q (postcondition):** a body `#tag` (per the D-02 grammar) renders as a `cm-hashtag`
  badge on both CM6 surfaces; the document bytes are unchanged; the negative corpus
  produces no badge; the badge DOM is createElement+textContent only and non-atomic.
- **S (code shape):** an inline parser emits a `Hashtag` node; `buildDecorations`
  replaces it with a `HashtagWidget` unless the line is active.
- **P (precondition):** `markdownExtension`'s extension array, the `buildDecorations`
  switch + `isActive`/`widgetRanges` machinery, the widget template, and the
  `decorationsFor` harness all pre-exist.
- **S establishes Q:** the parser's start-boundary + ≥1-non-digit + structural
  code/heading exclusion gives exactly the D-02 grammar (every corpus case pinned);
  the replace decoration is the only state touched (no transaction ⇒ byte-stable);
  `HashtagWidget.toDOM` never assigns innerHTML; the widget is omitted from
  `widgetRanges` ⇒ non-atomic.
- **Invariant (object-state):** across the ViewPlugin's lifetime, `decorations` and
  `atomicRanges` are pure functions of `(doc, selection, facet)`; `atomicRanges` never
  contains a Hashtag range. The plugin issues zero document transactions, so
  `doc.toString()` is verbatim on every rebuild (pinned in edit and clean mode).

```yaml
handoff:
  record_author: engineer
  accountable_owner: engineer
  id: H-21-execute
  from: engineer
  to: architect
  artifact: .planning/phases/21-inline-tag-parser-badge-render/21-01-PLAN-SUMMARY.md
  intent: "Phase 21 execution complete — Hashtag parser + badge render shipped byte-stable; both gates run + dispositioned; render⇄scanner code-region + block-boundary divergence documented and pinned for Phase 22 (ED-02b) reconciliation"
  status: ready
```

```yaml
evidence:
  record_author: engineer
  evidence_producer: engineer
  commissioned_by: engineer
  accountable_owner: engineer
  id: E-21-test-run
  subject_ref: "npx vitest run (web/)"
  kind: test_result
  issuance:
    kind: producer_signed
  warrant:
    kind: external_cited
    scope:
      - "carry=src/lib/cm/ 147/147 PASS"
      - "carry=full web suite 485/485 PASS"
      - "carry=npm run build (tsc -b && vite build) clean"
    rationale: "exercises the Hashtag parser, parseInlineTags, HashtagWidget DOM, and the render corpus (positive/negative/reveal/byte-stable/non-atomic) against a headless EditorView under jsdom; full suite confirms no consumer regression"
  freshness:
    status: fresh
    checked_against: HEAD
```

```yaml
gate_result:
  record_author: engineer
  status: applicable
  gate: anvil-code-reviewer
  phase: "21"
  basis: "Pre-handoff diff review (every SUMMARY, not scope-gated). Verdict SHIP — no blocker/major; 1 should-fix (drift claim) + 2 nits, all dispositioned above. Artifact: /tmp/anvil-spawn-out/21-REVIEW.md"
```

```yaml
gate_result:
  record_author: engineer
  status: applicable
  gate: anvil-adversarial-tester
  phase: "21"
  basis: "Hard-bar phase (negative corpus + byte-stability + no-innerHTML + non-atomic). 48 adversarial cases run; 1 consistency defect reproduced (render⇄scanner drift), dispositioned + pinned; safety invariants held under all inputs. Durable pins folded into the suite (commit a2d13d8)."
```
