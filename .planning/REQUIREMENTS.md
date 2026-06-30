# Milestone v1.4 Requirements — Editor Enhancements & Authoring Polish

## Overview

Make the page editor a richer, lower-friction authoring surface: the agent generates the page
description (no manual field), tags become first-class via inline `#tag` + LLM suggestions
(the manual Tags field is dropped), fenced code gets language-aware syntax highlighting,
` ```mermaid ` blocks render as diagrams, and Obsidian-style page templates speed up new pages.

**Locked design decisions (from kickoff + research):**
- **Tag source-of-truth = Model B (frontmatter-canonical).** Inline `#tag` is the authoring
  surface; on save the editor materializes body `#tags` into frontmatter `tags:` (byte-stable
  via `okf.SetTags`). Search, graph, tag-sweep, and the deferred v1.3 tag-management keep
  reading frontmatter ONLY — zero blast radius. LLM-applied tags that were never inline persist
  and are removed via a minimal badge-✕ control.
- **All agent writes stay propose/suggest → review → approve** (no silent writes), byte-stable.
- **Mermaid is the only new runtime dependency** — lazy `import()` chunk, `securityLevel:'strict'`.
- **Syntax highlighting is CM6-native** (`highlightCode` + opt-in `@codemirror/lang-*`), one
  parser set for both the live editor and the read `CodeBlockWidget`. No Shiki/highlight.js/Prism.
- **v1.4 scope is the CM6 surfaces** (live + read). Rendering `#tag` badges in the
  react-markdown path (agent chat / diff preview) is deferred (Future).

Phases continue from v1.3's last (Phase 20) → **v1.4 starts at Phase 21**.

---

## ED-01 — LLM-Generated Description

### ED-01a: Remove the manual Description field
**Requirement:** The page editor no longer shows a hand-entered Description input.
**Acceptance criteria:**
- [ ] The Description `<input>` is removed from `PageEditor.tsx`; no editor regression.
- [ ] The frontmatter `description` key still exists on every page (required-field repair via `okf.Repair` is unchanged — it only *adds* a missing key, never overwrites).
- [ ] An existing human-written description is preserved (never blanked by removing the field).

### ED-01b: Agent generates the description on demand
**Requirement:** An editor can ask the agent to generate (or refresh) the page description.
**Acceptance criteria:**
- [ ] A "Generate description" affordance in the editor triggers a single-shot agent call over the page body.
- [ ] The generated text is a short plain-text meta description (~1 sentence, soft cap ~160 chars), shown for review BEFORE apply, and is editable + regenerable.
- [ ] Approve writes `description` via byte-stable `okf.SetField` (only the `description` value node changes) through the existing revision-checked save (409 on a moved revision).
- [ ] Generation is **on demand only** — never auto-runs on save or create (no silent write, no per-save LLM cost).
- [ ] Output is re-validated server-side before write (never trust raw model output).
**Security:** editor-gated apply endpoint; CSRF on apply; page content treated as untrusted input to the prompt.

---

## ED-02 — Inline `#tag` Badges (foundational)

### ED-02a: Parse and render inline `#tag` as badges
**Requirement:** `#tag` tokens typed in the page body render as labeled badges on the live-preview and read surfaces.
**Acceptance criteria:**
- [ ] A `@lezer/markdown` inline-parser extension defines a `Hashtag` node (composed with the existing GFM extensions in `markdown.ts`).
- [ ] `#tag` renders as a badge in live preview and in read (clean-render) mode; the raw `.md` keeps the literal `#tag` text (decoration-only → byte-stable; the document is never mutated).
- [ ] Source mode (and the active editing line in live mode) shows the raw `#tag`.
- [ ] Tag syntax follows Obsidian: alphanumerics + `_` `-` and `/` (nested `#a/b`); must contain ≥1 non-digit; no spaces; case-insensitive match, original-case display.
- [ ] `#` is NOT a tag inside fenced code, inline code, an ATX heading (`# Title`), a URL fragment (`/#frag`), a hex color (`#RRGGBB`), an all-numeric run (`#1984`), or when escaped (`\#`). Pinned by `livePreview.test.ts` fixtures.
- [ ] The badge widget DOM is built with `createElement` + `textContent` only (no `innerHTML`).
**Security:** decoration/widget only — no document transaction; no HTML injection from tag text.

### ED-02b: Materialize inline tags into frontmatter on save (Model B)
**Requirement:** Saving a page unions its body `#tags` into the canonical frontmatter `tags:`.
**Acceptance criteria:**
- [ ] On save, body `#tags` are parsed and unioned into `frontmatter.tags` via byte-stable `okf.SetTags` (only the `tags:` region changes).
- [ ] Deleting a `#tag` from the body removes it from frontmatter on the next save (unless still present as a non-inline tag).
- [ ] Search, graph (`page_tags`), tag-sweep vocabulary, and v1.3 tag-management read frontmatter unchanged (no indexer change).
- [ ] Normalization matches existing tag rules (trim, case-fold) and server-side re-validation before write.

### ED-02c: Remove the manual Tags chip field; keep a minimal removal control
**Requirement:** The chip Tags field is replaced; LLM/imported tags remain removable.
**Acceptance criteria:**
- [ ] The `InlineTagEditor` chip field is removed from `PageEditor.tsx`.
- [ ] A minimal read-only badge bar shows the page's effective tags; each non-inline (LLM/imported) tag has an ✕ to remove it (byte-stable frontmatter write).
- [ ] Inline-origin tags are managed by editing the body (`#tag`), not the badge bar.

---

## ED-03 — LLM Tag Suggestions

### ED-03a: Suggest tags into the Model-B tag set
**Requirement:** An editor can get agent tag suggestions that feed the canonical tag set.
**Acceptance criteria:**
- [ ] Reuses the shipped `SuggestTags` agent mode + `apply-tags` path (vocabulary-biased, capped, server-revalidated).
- [ ] Suggestions are presented for approval (no silent writes); approved tags land in **frontmatter `tags:`** — never inserted as `#tag` into the user's prose.
- [ ] Suggestions are de-duped (case-insensitive) against the **union** of body inline tags + frontmatter tags, so an approved suggestion never duplicates an existing tag.
- [ ] A suggestion failing tag-charset validation is rejected.
**Security:** editor-gated apply; CSRF; page content untrusted to the prompt.

---

## ED-04 — Code Syntax Highlighting

### ED-04a: Highlight fenced code on both CM6 surfaces
**Requirement:** Fenced code blocks render with language-aware highlighting in the live editor and the read widget.
**Acceptance criteria:**
- [ ] One shared `HighlightStyle` (token colors from design tokens, light/dark aware) drives both surfaces.
- [ ] Live editor: highlighting via `markdown({ codeLanguages })`; read `CodeBlockWidget`: via `highlightCode` from `@lezer/highlight`, emitting spans through `createElement` + `textContent` (no `innerHTML`).
- [ ] A curated language set ships in v1.4: **js/ts, python, go, rust, json, yaml, sql, html, css, bash, markdown, diff** (bash/diff/dockerfile via `@codemirror/legacy-modes`). Languages are preloaded so the read widget highlights synchronously.
- [ ] An unknown/absent language tag falls back to plain monospace (no error).
- [ ] A ` ```mermaid ` fence is routed to ED-05, not highlighted as code.
**Security:** highlighter output is token spans built with textContent only (no HTML strings).
**Out of scope (this feature):** line numbers; a copy-code button (deferred — see Future).

---

## ED-05 — Mermaid Diagrams

### ED-05a: Render ` ```mermaid ` blocks as diagrams
**Requirement:** Fenced `mermaid` blocks render as diagrams on the read/live surface.
**Acceptance criteria:**
- [ ] A `MermaidWidget` (block widget from a StateField, branching `codeBlockField` on `CodeInfo === "mermaid"`) renders the diagram; supports flowchart, sequence, class, state, ER at minimum.
- [ ] `mermaid` is loaded via dynamic `import()` (its own async chunk) — never in the main editor bundle.
- [ ] Rendered with `securityLevel: 'strict'`; the SVG is injected via DOMParser + appendChild (never `innerHTML` of page content).
- [ ] A parse/render error falls back to the raw fenced code block (existing `CodeBlockWidget`), never a blank or a crash.
- [ ] Light/dark theme is honored; live-preview re-render is debounced (not per-keystroke).
**Security:** strict sandbox + sanitized SVG; client-only render (no SSR/CDN); untrusted diagram source.

---

## ED-06 — Page Templates

### ED-06a: Templates stored as files, picked on new-page
**Requirement:** Creating a page can seed it from an Obsidian-style template.
**Acceptance criteria:**
- [ ] Templates are plain `.md` files in a `Templates/` folder in the repo (files-as-truth; git-versioned; editable as normal pages).
- [ ] A list endpoint enumerates available templates; the create-page flow offers a template picker (a blank/no-template default remains).
- [ ] The chosen template's frontmatter + body seed the new page; `{{title}}`, `{{date}}`, `{{time}}` are substituted (`String.replaceAll` + `Intl.DateTimeFormat`; no templating engine).
- [ ] Template insertion is byte-stable and runs through the existing required-field repair (a template's frontmatter that omits required keys is repaired).
- [ ] Template paths resolve through the existing fuzz-tested safe path resolver.
**Security:** no executable/JS templates (static text + the three safe `{{…}}` placeholders only); path-safety on template reads.
**Out of scope (this feature):** insert-at-cursor into an existing page (deferred — see Future).

---

## Future Requirements (deferred)

- `#tag` badge rendering in the **react-markdown** path (agent chat / diff preview) — v1.4 covers the CM6 surfaces only.
- Copy-code button + language-label header on code blocks.
- Templater-style per-folder default templates; insert-at-cursor into existing pages.
- Description surfaced in search-result cards / graph hover (consumer wiring).

## Out of Scope (explicit)

- **Model A (Obsidian-literal tag union where indexers parse body)** — too large a blast radius into shipped search/graph/sweep and the v1.3 tag-management contract.
- **Auto-generating the description on save/create** — silent write; violates the locked approval model and costs an LLM call per save.
- **Executable/script (Templater-style) templates** — code-execution surface in a multi-user agent-era wiki.
- **Line numbers / full 300-language Prism set** in code blocks — noise / YAGNI for a prose wiki.
- **A second Markdown engine or Shiki-rendered read surface** — keep CM6 `cleanRender` canonical.

## Traceability

| REQ-ID | Phase | Status |
|--------|-------|--------|
| ED-01a/b | 23 | planned |
| ED-02a/b/c | 21 | planned |
| ED-03a | 22 | planned |
| ED-04a | 24 | planned |
| ED-05a | 25 | planned |
| ED-06a | 26 | planned |

*(Phase mapping confirmed by the ROADMAP; this table is the roadmap's source of truth for coverage.)*
