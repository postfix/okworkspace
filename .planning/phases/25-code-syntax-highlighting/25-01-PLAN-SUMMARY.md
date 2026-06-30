---
author: engineer
responsible: engineer
phase: 25-code-syntax-highlighting
plan: 01
type: implement
status: complete
requirements: [ED-04a]
---

# Phase 25 — Code Syntax Highlighting (ED-04a) — Engineer SUMMARY

**PLAN ACCEPTED** after reviewing all four tasks against the real codebase: every
CM6/Lezer signature + version in the plan's `<research_facts>` verified against
`node_modules` .d.ts; the `(code, lang)` constructor contract, the additive
`markdown({ codeLanguages })` wiring, and the `generateBundle`/`moduleIds` approach
all confirmed feasible. One nuance surfaced during build (see Task 4) — handled, not a
blocker.

## What was built, per task

### Task 1 — shared HighlightStyle + code-split codeLanguages + tokens (commit `98eb4ca`)
- `web/src/lib/cm/highlight.ts` (NEW): `codeHighlightStyle = HighlightStyle.define([...])`
  with explicit `class:` names (`tok-keyword/string/comment/number/function`) mapping
  `@lezer/highlight` tags → those classes, **scoped to CODE tags only** (no heading/
  emphasis/strong, so the editor's existing decorations are untouched). `codeLanguages:
  LanguageDescription[]` — the curated v1.4 set (js/ts, python, go, rust, json, yaml, sql,
  html, css, shell, markdown, diff), every grammar behind a dynamic `import()` inside
  `load` (NO static grammar import at module top).
- `tokens.css`: `--syntax-*` dark palette (5 muted/Obsidian-like roles), structured so a
  future light theme is a pure value swap.
- `LivePreviewEditor.css`: `.tok-* { color: var(--syntax-*) }` (tokens-only, no hex) +
  `tok-fade-in` keyframe (color/opacity only — zero layout shift).
- Deps pinned (caret): `@lezer/highlight`, `@codemirror/lang-{javascript,python,go,rust,
  json,yaml,sql,html,css}`, `@codemirror/legacy-modes`.
- Tests: `highlight.test.ts` (12) — style maps code tags / leaves Markdown tags null;
  curated names+aliases; each `load()` resolves a `LanguageSupport`; structural assertion
  that highlight.ts has NO static grammar import (via `?raw`).

### Task 2 — wire the live editor (commit `93ad433`)
- `markdown.ts`: added `codeLanguages` to `markdown({...})`; `base: commonmarkLanguage` +
  `extensions: tagParserExtensions` **unchanged** → GFM-only parser contract preserved.
- `mode.ts`: `syntaxHighlighting(codeHighlightStyle)` added to `liveExtensions` only.
- Tests: `markdown.test.ts` (6) — GFM regression (Table/Strikethrough/TaskList/Autolink +
  Hashtag still parse; no Emoji/Superscript superset); `highlightingFor` resolves `tok-*`
  in `liveExtensions` and **null** in `sourceExtensions`; a ```go fence still parses to
  FencedCode/CodeInfo/CodeText.

### Task 3 — read CodeBlockWidget async highlight (commit `2734fe6`, hardened in `ff6b226`)
- `widgets.ts`: `CodeBlockWidget` extended (constructor + `eq` unchanged → livePreview.ts
  mount untouched). `toDOM` renders plain `<pre><code>` via `textContent` first, then
  loads the matched grammar and re-emits token spans via `highlightCode` (createElement +
  `textContent` ONLY — never innerHTML). `destroy()` flips a `destroyed` flag; the async
  write is guarded by `destroyed || !codeEl.isConnected`. Unknown lang + ```mermaid stay
  plain. Adds `cm-md-codeblock--highlighted` → CSS fade-in.
- Tests: `widgets.test.ts` +8 (plain-first sync render; real-grammar re-highlight by
  element structure; markup-as-text; unknown/mermaid plain; both teardown paths).

### Task 4 — automated bundle assertion (commit `9bbb88c`)
- `web/vite-plugins/assertLazyChunks.ts` (NEW): reusable Rollup `generateBundle` plugin —
  `assertLazyChunks({ forbidden: RegExp[]; maxEntryBytes?: number })`. Fails the build if a
  forbidden module appears in any entry chunk (`chunk.moduleIds`) or the entry exceeds the
  byte budget (`Buffer.byteLength(chunk.code)`). Parameterized for Phase 26 (`forbidden:
  [/mermaid/]`).
- Wired into `vite.config.ts`. Added `@types/node` + tsconfig.node.json owns `vite-plugins/`.
- Tests: `assertLazyChunks.test.ts` (10) against synthetic bundles — forbidden-leak errors,
  clean/lazy passes, oversize/under-budget, non-entry/asset ignored, core grammars allowed,
  lazy grammars forbidden, anchoring boundary, custom-forbidden reuse.

## Measured bundle number + budget (D-05)
- **Measured entry chunk = 1,180,566 bytes** (clean build, grammars lazy). After the gate
  fixes the entry is 1,180,610 bytes (1,180.61 kB) — essentially unchanged.
- **`maxEntryBytes = 1,420,000`** (~20.3% headroom over the measured figure).
- The 6 genuinely-lazy grammars (python/go/rust/json/yaml/sql) + legacy-modes (shell/diff)
  emit their own `dist-*.js` chunks — proven absent from the entry chunk by the assertion.

### Important structural finding (handled, not scope-invented)
The plan's illustrative forbidden regex `/lang-/` would have **wrongly failed the build**:
`@codemirror/lang-markdown` is the editor's CORE base language (statically imported in
markdown.ts) and hard-depends on `@codemirror/lang-html`, which transitively pulls
`lang-css` + `lang-javascript`. Those four are unavoidably in the entry chunk **and were
already there pre-phase**. I excluded exactly those four via an anchored negative lookahead
`lang-(?!(?:markdown|html|css|javascript)\/)` while keeping every code-fence grammar
forbidden. (The first build run reproduced the leak on `lang-css`, confirming the analysis;
the `INEFFECTIVE_DYNAMIC_IMPORT` warnings for those four are expected — their dynamic import
resolves to the already-loaded core module, uniform code shape, harmless.)

## Gate outcomes + dispositions
**anvil-code-reviewer** — verdict SHIP, 3 MINOR. **anvil-adversarial-tester** — 2 defects;
both core security invariants (no-innerHTML, teardown-safe) HELD under 40 attacks.

| Finding | Severity | Disposition |
|---|---|---|
| Post-load section outside try/catch → could blank `<code>` + leak unhandled rejection (both reviewers) | MED | **FIXED** (`ff6b226`): whole pass guarded; tokens built into a detached fragment, swapped atomically only on success → failure leaves plain baseline, no throw |
| `matchLanguageName` fuzzy=true → unlisted info strings (jsonl→json) highlighted | LOW | **FIXED** (`ff6b226`): `fuzzy=false` → exact name/alias (case-insensitive) only |
| Unanchored forbidden lookahead (prefix match) | MINOR | **FIXED** (`ff6b226`): anchored with trailing `/` |
| Fade-in "only in editor" | MINOR | **WON'T-FIX**: non-issue — read mode keeps the `.livepreview-editor` class (proven by the existing `.cm-md-codeblock` styling), fade applies to both surfaces |

Durable adversarial suite landed at `web/src/lib/cm/widgets.adversarial.test.ts` (45 tests),
including regression guards pinning both fixes. (The suite intentionally contains
bidi/lone-surrogate/NUL payloads as adversarial inputs — `grep` sees it as binary; this is
deliberate test data in a test file.)

## Verification evidence (from `web/`)
- `npx tsc -b` → clean (exit 0).
- `npx vitest run` → **685 passed / 59 files**, 0 fail (no regression in existing
  cm/livePreview/PageView/markdown suites; +12 highlight, +6 markdown, +8 widgets,
  +10 assertLazyChunks, +45 adversarial).
- `npm run build` → clean with the assertion ACTIVE. Both assertion arms proven e2e:
  the forbidden-regex arm (first build failed on `lang-css` before the core exclusion) and
  the budget arm (temporarily set to 1000 → build failed `over the 1000-byte budget`,
  then restored).
- `internal/web/dist/.gitkeep` restored after each `emptyOutDir` build; no dist artifacts
  committed.

## Handoff
```yaml
handoff:
  record_author: engineer
  accountable_owner: engineer
  from: engineer
  to: architect
  artifact: .planning/phases/25-code-syntax-highlighting/25-01-PLAN-SUMMARY.md
  intent: "ED-04a code syntax highlighting built + gated; ready for verify"
  status: sent
  commits: [98eb4ca, 93ad433, 2734fe6, 9bbb88c, ff6b226]
  notes: >
    Frontend-only, no backend change. Both CM6 surfaces highlight via one shared
    HighlightStyle; read widget is async/teardown-safe/textContent-only. Entry chunk
    1,180,566 B measured, budget 1,420,000 B, assertion active in the build (reusable by
    Phase 26 for mermaid). The four core grammars (markdown+html+css+js) are legitimately
    in entry (lang-markdown dep chain) and excluded from the forbidden list — flag for
    verify so it isn't read as a leak.
```

**EXECUTE COMPLETE.**
