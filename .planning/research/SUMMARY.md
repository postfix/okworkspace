---
author: architect (synthesis of 4 parallel project-researchers)
artifact_kind: research_summary
milestone: v1.4
emitted_at: 2026-06-30
parent_artifacts:
  - .planning/research/PITFALLS.md
  - .planning/research/_archive-v1.0/ (prior milestone research)
status: draft
---

# Research SUMMARY — v1.4 Editor Enhancements & Authoring Polish

Synthesis of 4 parallel researchers (Stack / Features / Architecture / Pitfalls). All four
**converge** on the same architecture and the same load-bearing decision. Full pitfalls detail
is in `PITFALLS.md`; the dimension notes (Stack/Features/Architecture) live in the kickoff
conversation and are condensed here.

## Headline

Four of six features need **no new dependency**. The **only new runtime dep is Mermaid**
(`mermaid@^11.16.0`) and it is the milestone's sole bundle/security risk — must be **lazy
`import()`-split** + `securityLevel:'strict'`. Every feature attaches to *existing seams*
(the CM6 decoration pipeline `livePreview.ts`/`widgets.ts`/`markdown.ts`, the byte-stable
frontmatter writers `okf.SetField`/`okf.SetTags`, and the suggest→approve→apply HTTP pattern).

## THE decision (load-bearing — ratify before planning ED-02): tag source-of-truth

Inline `#tag` **rendering** is byte-stable for free (decoration-only). But inline tags are
**invisible to search + graph** unless promoted into frontmatter `tags:` — both indexers read
tags ONLY from frontmatter (`indexjob.go` `readTags`; `graphjob.go`/`rebuild.go` `pageTags`).
Architecture **and** Features researchers independently recommend the same model:

- **Model B (RECOMMENDED): frontmatter `tags:` stays the single canonical persisted store;
  inline `#tag` is the authoring surface, materialized into frontmatter on save** (byte-stable
  via `okf.SetTags`, tags region only). Delivers Obsidian's union semantics; leaves search,
  graph, tag-sweep, and the deferred **v1.3 tag-management** contract 100% untouched.
- Model A (Obsidian-literal union; indexer also parses body) — higher blast radius into shipped
  + roadmapped code (v1.3 would have to rewrite body prose). Not recommended.

**Canonical rule (Model B):** on save, `frontmatter.tags = (inline #tags parsed from body) ∪
(non-inline tags retained from frontmatter, e.g. LLM-applied)`. Removal: deleting `#tag` from
the body drops it; LLM/imported tags persist and are removed via a **minimal badge-✕ bar** (the
small surviving remnant of the old chip field). Graph/search/sweep keep reading frontmatter only.

## Per-feature stack + integration

| ED | Feature | New dep | Integration |
|----|---------|---------|-------------|
| ED-01 | LLM description (remove field) | none | new `GenerateDescription` agent mode + `okf.SetField(description)`; clone of suggest/apply-tags. `okf.Repair` keeps the frontmatter key, so required-field logic is untouched. On-demand, review→approve (NOT auto-on-save). |
| ED-02 | Inline `#tag` badges | none | `@lezer/markdown` `MarkdownConfig` inline parser → `Hashtag` node in `markdown.ts`; badge mark/widget in `widgets.ts` (createElement+textContent); `case "Hashtag"` in `livePreview.ts`; promote-to-frontmatter on save. |
| ED-03 | LLM tag suggestions | none | extend shipped `SuggestTags` + `apply-tags`; suggestions land in **frontmatter** (not inserted into prose); de-dupe vs the union. |
| ED-04 | Code syntax highlighting | `@codemirror/lang-*` (opt-in, code-split) | CM6-native: `markdown({codeLanguages})` for the live editor + `highlightCode` (`@lezer/highlight`, already transitive) for the read `CodeBlockWidget`. One `HighlightStyle`, both surfaces. NOT Shiki/highlight.js/Prism. |
| ED-05 | Mermaid | **`mermaid@^11.16.0`** | new `MermaidWidget` + branch `codeBlockField` on `CodeInfo==="mermaid"` (StateField — block widgets can't come from a ViewPlugin). **Dynamic `import()`**, `securityLevel:'strict'`, SVG via DOMParser+appendChild (never innerHTML), raw-code fallback on error. |
| ED-06 | Page templates | none | `Templates/` folder of `.md` files (files-as-truth); list endpoint + new-page picker; `{{title/date/time}}` via `String.replaceAll`+`Intl.DateTimeFormat` (no templating engine; no executable JS templates). |

## Top risks (from PITFALLS.md)

1. **Mermaid** — ~2.7 MB + d3/dagre/cytoscape/katex/dompurify (~18 deps); documented SVG-XSS
   class (GHSA-wvh5-6vjm-23qh, CVE-2026-0752). Prevent: lazy chunk + `securityLevel:'strict'` +
   no innerHTML. Owner: Phase 25.
2. **Inline `#tag`** — must be Lezer-tree-driven decoration, not a text regex, or it false-
   positives on code spans, fenced code, headings (`# Title`), URL fragments (`/#frag`), hex
   colors (`#RRGGBB`), issue refs (`#123`). Pin with `livePreview.test.ts` fixtures. Owner: 21.
3. **LLM writes** — stay propose→approve + byte-stable `okf.SetField` (never `yaml.Marshal`
   a struct). `Repair` is safe (adds missing only, never overwrites description). Owners: 21/23.

Other preventions: code highlighting via `highlightTree` callbacks → createElement+textContent
(no HTML strings); block widgets from a StateField; templates substitute into frontmatter
*values* via `SetField` + resolve paths through the fuzz-tested safe resolver.

## Recommended build order (continues from Phase 20)

- **Phase 21 — ED-02 inline `#tag` + tag source-of-truth** (foundational; carries THE decision; unblocks ED-03). Spawn roadmap red-team here.
- **Phase 22 — ED-03 LLM tag suggestions** (hard-depends on 21).
- **Phase 23 — ED-01 LLM description** (the other frontmatter-field deletion + suggest→apply mode; independent of 21/22).
- **Phase 24 — ED-04 code syntax highlighting** (settles widget-highlight + lazy-language patterns).
- **Phase 25 — ED-05 Mermaid** (shares widget files with 24; heaviest/riskiest).
- **Phase 26 — ED-06 page templates** (fully independent; could parallelize).

## Open questions for the architect (settle in planning)

1. Tag removal/provenance UX — confirm the minimal badge-✕ bar for non-inline (LLM) tags.
2. Highlighting language set for v1.4 (~8–10: js/ts, py, go, rust, json, yaml, sql, html/css, bash via legacy-modes). Each is opt-in/code-splittable.
3. Read-widget highlight: preload the language set (sync `toDOM`) vs render-plain-then-async-rerender. Recommend preload.
4. Do `#tag` badges need to render in the **react-markdown** path (agent chat / diff preview) too, or only the CM6 surfaces? Determines whether a mirror remark plugin is in scope.
5. Templates: new-page picker only (recommended) vs also insert-at-cursor into an existing page.
6. Mermaid `securityLevel:'strict'` confirmed (no feature needs `'loose'`).

## Verified versions (npm / published .d.ts, 2026-06-30)

`mermaid` 11.16.0 · `@codemirror/language` 6.12.4 · `@lezer/highlight` 1.2.3 (transitive) ·
`@lezer/markdown` 1.6.4 · `@codemirror/lang-markdown` 6.5.0 · `@codemirror/lang-{javascript 6.2.5,
python 6.2.1, go 6.0.1, rust 6.0.2, json 6.0.2, html 6.4.11, css 6.3.1, sql 6.10.0, yaml 6.1.3}` ·
`@codemirror/legacy-modes` 6.5.3 (shell/bash/diff/dockerfile via StreamLanguage).
