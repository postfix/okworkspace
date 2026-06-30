---
author: architect
---

# Phase 25: Code Syntax Highlighting — Context

**Gathered:** 2026-06-30
**Status:** Ready for planning

<domain>
## Phase Boundary

Language-aware syntax highlighting of fenced code on **both CM6 surfaces** — the live
editor (`LivePreviewEditor` edit mode) and the read-only `LivePreviewEditor` that
`PageView` renders — driven by ONE shared, theme-aware `HighlightStyle`, with the
per-language grammars **code-split** so the main/entry chunk stays in budget. This is a
**frontend-only** (`web/`) phase; there is no backend change (the server stores raw
Markdown bytes, unchanged). Requirement: **ED-04a**.

**In scope (ED-04a):**
- A single shared `HighlightStyle` (from `@codemirror/language`) mapping Lezer highlight
  tags → new `--syntax-*` design tokens (muted/Obsidian-like, ~5 roles), driving BOTH
  surfaces so live and read never diverge.
- Live editor: `markdown({ …, codeLanguages })` with the curated languages as
  `LanguageDescription.of({ load: () => import(...) })` (code-split).
- Read `CodeBlockWidget`: `highlightCode` from `@lezer/highlight`, emitting token spans via
  `createElement` + `textContent` (no `innerHTML`); renders plain first, then
  **re-highlights when the lazy parser resolves** (async-widget pattern), with a subtle
  CSS fade-in and zero layout shift.
- Curated v1.4 set: js/ts, python, go, rust, json, yaml, sql, html, css, bash, markdown,
  diff (bash/diff via `@codemirror/legacy-modes`). Unknown/absent language → plain
  monospace, no error.
- Bundle: grammars are absent from the main/entry chunk (structural guarantee), verified by
  an **automated build assertion** on the Vite build output; the core+style size delta is
  capped at the measured value + ~20% headroom. The assertion pattern is reused by Phase 26.
- A ` ```mermaid ` fence is **not** highlighted (left plain → routed to Phase 26 / ED-05).

**Out of scope / unchanged:**
- Line numbers; a copy-code button (deferred — ED-04a "Future").
- A light theme is NOT wired up (app is `color-scheme: dark` only); `--syntax-*` tokens are
  authored for dark now and named/placed so a future light theme is a pure token swap.
- Highlighting in the `MarkdownProse` (react-markdown) surfaces — `AgentAnswer`,
  `HistoryPanel`, `ImagePreviewDialog` — is OUT of scope (ED-04a targets the two CM6
  surfaces only). Deferred.
- New languages beyond the curated set (TOML / Dockerfile / ini considered, NOT added).
- The GFM-only parser contract (`markdown.ts`) — `codeLanguages` is additive and must not
  change which Markdown constructs are parsed.
</domain>

<decisions>
## Implementation Decisions

### Palette (D-01)
- **D-01 — Muted / Obsidian-like palette, ~5 desaturated roles, as new `--syntax-*` tokens.**
  Token roles + intended dark values (soft, blends into `--color-bg` #0b0d12): keyword = soft
  blue, string = soft green, comment = grey + italic, number/constant = amber,
  function/type = soft purple (harmonizes with the `--color-accent` #8b5cf6 brand). Authored
  in `web/src/styles/tokens.css` as `--syntax-*` vars. **Tokens-only contract holds** — no
  inline hex anywhere in `theme.ts`/the `HighlightStyle`; every color is a `var(--syntax-*)`.
  **Light mode:** author dark values only now, but name/structure the tokens so a future light
  theme is a pure token swap (mirrors `theme.ts`'s "a future theme needs no edits" philosophy);
  this satisfies ED-04a's "light/dark aware" structurally without an untestable light set.

### Shared highlight engine (D-02, D-03, D-04)
- **D-02 — One `HighlightStyle`, both surfaces.** A single `HighlightStyle.define([...])`
  maps `@lezer/highlight` tags → the `--syntax-*` tokens. The live editor consumes it via the
  `syntaxHighlighting(style)` extension (added in `mode.ts`'s `liveExtensions`); the read
  `CodeBlockWidget` passes the SAME style object to `highlightCode`. Single source of truth →
  live and read render identically.
- **D-03 — Live editor via `markdown({ codeLanguages })`.** Extend `markdownExtension`
  (`markdown.ts`) with `codeLanguages: [...]` of `LanguageDescription.of({ name, alias,
  load: () => import("@codemirror/lang-…") })`. Keep `base: commonmarkLanguage` + the existing
  `tagParserExtensions` (GFM-only + hashtag) UNCHANGED — `codeLanguages` is purely additive.
- **D-04 — Read widget async re-highlight.** `CodeBlockWidget.toDOM` is synchronous, so it
  renders the plain `<pre><code>` (today's behavior, `textContent`) FIRST, then kicks off the
  language's async `load()` and, on resolve, replaces the `<code>` children with
  `highlightCode`-generated token spans. Must guard the async window: the widget may have been
  torn down / replaced (`eq`) before the import resolves — do not write into a detached or
  superseded DOM node (cancel-on-destroy or liveness check). Mermaid/unknown langs skip the
  load and stay plain.

### Code-split + bundle budget (D-05)
- **D-05 — Grammars code-split out of the entry chunk; structural + measured budget,
  automated assertion.** Every grammar loads via `LanguageDescription.of({ load: () =>
  import(...) })` (and `@codemirror/legacy-modes/mode/…` for bash/diff) so Vite emits per-language
  chunks; **none** is static-imported into the main/entry chunk. The **primary, enforced
  guarantee is structural**: an automated build assertion (a test/script run against the Vite
  build output / chunk map, wired into the `web/` build or test path) asserts (a) no
  `lang-*` / `legacy-modes` grammar appears in the entry/main chunk, and (b) the main-chunk size
  delta vs the pre-phase baseline is ≤ the **measured** highlight core (`@lezer/highlight` +
  `HighlightStyle` machinery) + style, plus ~20% headroom (the KB figure is set from the real
  build, not guessed). This establishes the reusable assertion Phase 26 uses for mermaid-absence.

### Languages + fallback (D-06)
- **D-06 — Curated v1.4 set, ship as-is; graceful unknown fallback.** Languages: js/ts,
  python, go, rust, json, yaml, sql, html, css, bash, markdown, diff (bash/diff via
  `@codemirror/legacy-modes`). An unknown/absent language → the current plain-monospace
  `CodeBlockWidget` render, **no error/throw**. A ` ```mermaid ` info string is explicitly NOT
  highlighted (left plain; Phase 26 owns it).

### Security (D-07)
- **D-07 — `textContent`-only token spans (no `innerHTML`).** The highlighter output (both the
  read-widget re-highlight and any editor-side path) builds token spans with `createElement` +
  `textContent` ONLY — never HTML strings — extending the established widget security invariant
  (`widgets.ts` T-06-08). Tests assert on element structure (tagName/children/textContent),
  never on HTML strings, to lock it in.

### Re-highlight UX (D-08)
- **D-08 — Zero layout shift + subtle fade-in.** Plain and highlighted render use identical
  font/size/padding (only color changes), so neighbors never reflow — extends the
  layout-neutral-reveal invariant (`theme.ts` Pitfall 2) to code blocks. A short CSS
  color/opacity transition fades the highlighted spans in so the async swap reads as intentional
  rather than a flicker. Only the FIRST block of each language waits on its dynamic import
  (cached after).

### Surfaces (D-09)
- **D-09 — Both CM6 surfaces only.** "Both surfaces" = the `LivePreviewEditor` in edit mode and
  the read-only `LivePreviewEditor` (`PageView`). `MarkdownProse` (react-markdown:
  `AgentAnswer`, `HistoryPanel`, `ImagePreviewDialog`) is OUT of scope and deferred.

### Claude's Discretion
- Exact hex values for the `--syntax-*` tokens within the muted/Obsidian-like brief (subject to
  legible contrast on #0b0d12); the precise Lezer-tag → role mapping in the `HighlightStyle`;
  the concrete shape of the build-assertion (vitest test vs build-step script) so long as it is
  automated and reusable by Phase 26; the fade-in duration/easing.
</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Requirements / roadmap
- `.planning/ROADMAP.md` §Phase 25 — goal + the four observable success criteria; §Phase 26 —
  the widget-highlight + lazy-language pattern this phase must leave reusable.
- `.planning/REQUIREMENTS.md` §ED-04a — the locked acceptance criteria (shared HighlightStyle,
  `highlightCode` read widget, code-split + async re-highlight, curated set, bundle budget,
  textContent security); §ED-05 boundary (mermaid not highlighted here).

### Code to extend (the CM6 surfaces)
- `web/src/lib/cm/widgets.ts` — `CodeBlockWidget` (line 101) + the `createElement`/`textContent`
  no-`innerHTML` invariant shared by all widgets; the read widget to extend with async highlight.
- `web/src/lib/cm/livePreview.ts` — the decoration plugin that mounts `CodeBlockWidget` for
  `FencedCode` in read mode (import line 57; mount ≈ line 480-492; read-block path ≈ line 252).
- `web/src/lib/cm/markdown.ts` — `markdownExtension` (`markdown({ base, extensions })`); add
  `codeLanguages` here while preserving GFM-only parsing.
- `web/src/lib/cm/mode.ts` — composes `liveExtensions`/`sourceExtensions` (markdownExtension +
  livePreviewExtension + theme); where `syntaxHighlighting(highlightStyle)` is added.
- `web/src/lib/cm/theme.ts` — the `EditorView.theme` + `.cm-code-block`/`.cm-md-codeblock`
  styling; the tokens-only contract the `HighlightStyle` + fade-in CSS must honor.
- `web/src/components/LivePreviewEditor.tsx` — the shared edit+read surface; `LivePreviewEditor.css`
  (`.cm-md-codeblock`, line ~118) for the fade-in + token CSS.
- `web/src/routes/PageView.tsx` — the read-only `LivePreviewEditor` consumer (read surface).

### Tokens + build
- `web/src/styles/tokens.css` — existing color tokens (`--color-bg`, `--color-text`,
  `--color-accent` #8b5cf6, `--color-code-bg`); add the `--syntax-*` token block here.
- `web/vite.config.ts`, `web/package.json` (`build: tsc -b && vite build`, `test: vitest run`) —
  code-splitting / chunk output and the home for the automated build assertion.

### Prior-phase invariants (context)
- `.smtc/memories/qa-m1.4-phase24-verified.md`, `.smtc/memories/qa-m1.4-phase23-verified.md` —
  the layout-neutral-reveal + no-`innerHTML` widget invariants this phase extends.
</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `CodeBlockWidget` (`widgets.ts:101`): EXTEND (don't replace) — it already renders plain
  `<pre data-lang><code>` via `textContent`; add the async highlight pass on top.
- The `WidgetType` + `createElement` + `textContent` pattern (Image/Table/Hashtag/Bullet/Code
  widgets) — the security template the highlighter must follow.
- `theme.ts` token approach + `markdown.ts` parser config — extend in place.

### Established Patterns
- Tokens-only contract (`var(--…)` for every value; no inline hex in `theme.ts`).
- No-`innerHTML` widget security invariant (`textContent`/`createElement` only).
- Layout-neutral reveal (no reflow on state change) — extend to code blocks.
- GFM-only parser parity (editor ≡ server Goldmark ≡ read view) — `codeLanguages` must not
  perturb it.
- One component (`LivePreviewEditor`) renders BOTH edit and read; one `HighlightStyle` must
  feed both.

### Integration Points
- `livePreview.ts` decoration plugin mounts the read `CodeBlockWidget`.
- `mode.ts` composes the extension list (add `syntaxHighlighting(style)`).
- `tokens.css` holds the design tokens (add `--syntax-*`).
- `vite.config.ts` / build is where the code-split chunks emit and the assertion runs.
- No grammar packages installed yet — `@lezer/highlight`, `@codemirror/lang-*`,
  `@codemirror/legacy-modes` must be added as deps.
</code_context>

<specifics>
## Specific Ideas
- Palette inspiration: Obsidian's default code theme feel — desaturated, calm, NOT a vivid
  classic-editor scheme. Function/type role should harmonize with the existing purple accent.
- The async re-highlight should feel like an intentional fade, never a flicker; and must never
  shift layout (color-only swap).
</specifics>

<deferred>
## Deferred Ideas
- Syntax highlighting in the `MarkdownProse` (react-markdown) surfaces — agent answers,
  history diff, image dialog — would be its own follow-up (different render path).
- Additional languages: TOML, Dockerfile, ini/properties (considered; not in the v1.4 set).
- Line numbers and a copy-code button (ED-04a "Future").
- Wiring an actual light theme (only the token structure is prepared here).
</deferred>

---

*Phase: 25-code-syntax-highlighting*
*Context gathered: 2026-06-30*
