---
created_at: 2026-06-30T16:00:00Z
modified_at: 2026-06-30T16:00:00Z
slug: qa-m1.4-phase22-verified
tags:
  - milestone-v1.4
  - phase22
  - verified
  - tag-materialization
---

# Phase 22 (Model-B materialization + tag UI) — VERIFIED pass (qa)

ED-02b/ED-02c. Client-side reconcile-as-diff materializes body #tags into frontmatter
`tags:` on save. Verdict: **pass** (572/572 web tests, build clean, tsc-test exit 0).
Artifact: `.planning/phases/22-model-b-materialization-tag-ui/22-VERIFICATION.md`.

Key facts a future phase (esp. Phase 23 LLM tag suggestions, which feed the SAME
frontmatter set) inherits:

- **The diff rule lives in `web/src/lib/tag-reconcile.ts` `reconcileFrontmatterTags`**:
  `removed = inline(lastSavedBody) − inline(currentBody)`;
  `reconciled = (readTags(fm) − removed) ∪ inline(currentBody)`. NOT a monotonic union.
  Byte-stable: `setTags` fires ONLY when the tag SET changed; else frontmatter returned
  verbatim. An LLM-only tag (in fm, never inline) persists — this is the seam Phase 23
  lands suggestions into.
- **`inlineTagsFromBody` (`web/src/lib/cm/hashtag.ts`) shares `tagParserExtensions`** with
  the CM6 render (`markdown.ts` imports the SAME array) → code-region/heading/escape-aware
  by construction; no parser fork, no import cycle. Phase-21 render⇄scanner divergence is
  CLOSED (`>#deploy` agrees in both render and materialize).
- **BLOCKER that was fixed (watch for regressions):** the Go server re-marshals frontmatter
  via yaml.v3 at **4-space** indent on `Repair` (internal/okf/emit.go, no SetIndent). The
  client `readTags`/`setTags` were 2-space-only and read a repaired page's tags as EMPTY →
  corrupted on first autosave + lost the LLM tag. Now indent-agnostic + quote-aware
  (`tag-frontmatter.ts`, `[ \t]*-[ \t]`). The fix-before test genuinely fails against pre-fix
  code (verified against `0c76c88^`). If anyone touches tag I/O, keep the indent-agnostic match.
- **Reconcile is woven in `PageEditor.tsx` runSaver (~229-260)** BEFORE the PUT, against
  `lastSavedBody.current` (advanced only post-PUT). `setTags(fm, [])` leaves a cosmetic
  `tags:\n\n` on last-tag removal — converges, non-corrupting, accepted follow-up (no req).
- **UI:** `InlineTagEditor` DELETED; `TagBadgeBar` is read-only, ✕ on non-inline tags only.

Gotcha for verifying web phases: run vitest/build from `web/` (root config has no jsdom);
`npm run build` deletes `internal/web/dist/.gitkeep` (restore after); single-file LSP
diagnostics are spurious — ground truth is `tsc -p tsconfig.test.json --noEmit` exit 0.
See [[archaeologist-v1.1-close]].
