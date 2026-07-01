# Editor Subsystem (CM6 Live-Preview)

The Markdown editor and the page read view are the **same** hand-built CodeMirror 6
(CM6) surface — no React editor wrapper, no second read renderer. The defining
property: **the CM6 document *is* the raw Markdown string**. Everything you see
rendered — bold, headings, links, images, GFM tables, `#tag` badges, syntax-highlighted
code, mermaid diagrams — is a CM6 **view-layer decoration**, never a re-serialization of
a block model. That is why editing is byte-stable: `doc.toString()` round-trips verbatim
through Live⇄Source and through save.

**Source:** `web/src/lib/cm/*`, hosted by `web/src/components/LivePreviewEditor.tsx` and
mounted read-only by `web/src/routes/PageView.tsx`. Build guard:
`web/vite-plugins/assertLazyChunks.ts` (wired in `web/vite.config.ts`). In v1.5 the reading
column around this surface became a document (Breadcrumb + PropertiesPanel + DocStatusBar,
TagBadgeBar retired) and the CM6 host gained `drawSelection()` + `EditorView.lineWrapping` —
see [Reading shell + selection/coordinate accuracy (v1.5)](#reading-shell--selectioncoordinate-accuracy-v15).

---

## Files

| File | Role |
|------|------|
| [`mode.ts`](../../web/src/lib/cm/mode.ts) | Live/Source toggle. A `Compartment` holds either `liveExtensions` or `sourceExtensions`; `setMode` reconfigures it with `effects` **only** (never `changes`) → byte-identical document. `Cmd/Ctrl-E` (`Mod-e`) toggles. |
| [`markdown.ts`](../../web/src/lib/cm/markdown.ts) | The CM6 Markdown language, configured **GFM-only** (Table, Strikethrough, TaskList, Autolink + the `Hashtag` inline node), plus the curated `codeLanguages` for nested-fence highlighting. |
| [`livePreview.ts`](../../web/src/lib/cm/livePreview.ts) | The decoration engine: a `ViewPlugin` for inline decorations + the two block-widget `StateField`s (`tableField`, `codeBlockField`). Exports the `cleanRender` facet and the `livePreviewExtension` bundle. |
| [`widgets.ts`](../../web/src/lib/cm/widgets.ts) | The `WidgetType` family: `ImageWidget`, `BulletWidget`, `CodeBlockWidget`, `MermaidWidget`, `HashtagWidget`, `TableWidget`. Home of the no-`innerHTML` security invariant. |
| [`highlight.ts`](../../web/src/lib/cm/highlight.ts) | The one shared `codeHighlightStyle` + the curated `codeLanguages` (each grammar dynamic-`import()`ed → lazy chunk). |
| [`mermaid.ts`](../../web/src/lib/cm/mermaid.ts) | `renderMermaid` — lazy `import("mermaid")`, strict sandbox, serialized render queue. |
| [`hashtag.ts`](../../web/src/lib/cm/hashtag.ts) | The `#tag` grammar (`hashtagExtension`), the shared `tagParserExtensions` array, `inlineTagsFromBody` (materializer), and `inlineTagSpansFromBody` (the byte-offset span source that generates the Lezer↔Go parity corpus — see [Inline hashtags](#inline-hashtags)). |
| [`theme.ts`](../../web/src/lib/cm/theme.ts) | The CM6 theme — every value a design token (`var(--…)`); no hex/px. **v1.5:** heading vertical rhythm is `padding`, NOT `margin` — CM6's height oracle measures `offsetHeight` (includes padding, excludes margin), so a margin drifts `posAtCoords`/`coordsAtPos` below every heading (visible as selection landing a row off once `drawSelection` is on). |
| [`sanitizeSrc.ts`](../../web/src/lib/cm/sanitizeSrc.ts) | `sanitizeImageSrc` — the image-src allowlist. |

## Edit mode vs read mode — one pipeline, one facet

`LivePreviewEditor` renders both modes. Read mode is edit-Live **plus one facet**:
`cleanRender.of(true)` (provided only in read mode, `LivePreviewEditor.tsx`). Edit mode
never provides it, so it defaults `false`. That single boolean flips:

| Behavior | Edit (`clean = false`) | Read (`clean = true`) |
|---|---|---|
| Active-line reveal | ON — markers on the cursor's line stay raw for editing | OFF — every marker hides (finished prose) |
| Unordered list markers | raw `-`/`*`/`+` | typographic `•` bullet widget |
| Blockquote `> ` | raw | hidden, line styled |
| Non-mermaid fenced code | raw fences + styling | clean `<pre>` widget (no fences) |
| Mermaid fenced code | diagram, unless the cursor is in the block | diagram (always) |

**Read mode reuses the exact same decoration pipeline as edit-Live** — there is no
separate, weaker read renderer. A change to a decoration changes both surfaces at once.

## The decoration pipeline

1. `markdownExtension` parses the Lezer GFM tree.
2. The `livePreview` **`ViewPlugin`** walks the tree over the visible ranges and emits:
   inline marks (`cm-strong`, `cm-em`, `cm-inline-code`, `cm-code-block`, `cm-md-link`),
   per-level heading line classes, zero-width **hides** for syntax markers (`**`, `# `,
   backticks, link brackets/URL), inline **image** widgets, and inline **hashtag** badges.
3. Two **`StateField`s** emit the **block** widgets — `tableField` (GFM tables) and
   `codeBlockField` (mermaid diagrams always; other fenced code as clean `<pre>` in read
   mode). CM6 forbids block/line-spanning replace decorations from a `ViewPlugin`, so
   block widgets must live in a `StateField`.

The `ViewPlugin` issues no document transactions and never serializes the tree back to
text, so the stored bytes are always exactly what the user typed.

### Active-line reveal

Reveal is **selection-driven**: the plugin recomputes decorations on `docChanged`,
`viewportChanged`, or `selectionSet`, and simply skips emitting a hide for any marker on a
line the selection touches. It is never timer/debounce-driven (that would lag the cursor).
Crucially, reveal is **layout-neutral** — no decoration class changes line-height, so
entering/leaving a line never reflows its neighbors.

### atomic ranges (the cursor-trap rule)

`EditorView.atomicRanges` is applied **only** to block/inline **widgets** (image, table,
mermaid, read-mode code block) so backspace/cursor-motion treats a rendered widget as one
unit. The zero-width text-marker hides — and the short inline `#tag` badge — are
deliberately **non-atomic**, so the caret is never trapped on a marker run.

## Async widgets + lazy chunks (syntax highlight, mermaid)

Heavy renderers (`CodeBlockWidget` syntax highlighting, `MermaidWidget` diagrams) share
one discipline:

1. `toDOM` renders a **synchronous plain-text fallback first** (via `textContent`).
2. It kicks off a fire-and-forget async pass — a dynamic `import()` of the grammar
   (`@codemirror/lang-*`) or of `mermaid`.
3. On success it builds the highlighted tokens / parsed `<svg>` into a **detached
   fragment** and **atomically swaps** it in.
4. A per-instance `destroyed` flag + an `isConnected` check guard both sides of the
   `await`, so a resolved result is discarded rather than written into a torn-down node.
5. Any failure (import/parse/render) is swallowed — the plain fallback survives.

**Syntax highlighting (ED-04a):** one shared `codeHighlightStyle` drives both surfaces —
the live editor via `syntaxHighlighting(codeHighlightStyle)`, and the read
`CodeBlockWidget` via `highlightCode(...)` — so live and read render identically. Colors
are `--syntax-*` CSS variables (the style maps tags to `tok-*` class names only).
Curated languages: javascript/ts, python, go, rust, json, yaml, sql, html, css, shell,
markdown, diff. An **unknown** fence info string stays plain (exact match, `fuzzy=false`).

**Mermaid (ED-05a):** `mermaid` is large, so it loads only via a dynamic `import("mermaid")`.
It renders in read mode and in the live editor (Obsidian parity), revealing raw source only
while the cursor is inside the block. Render runs under `securityLevel:'strict'`
(mermaid → DOMPurify) + `suppressErrorRendering:true`, and calls are serialized behind a
module-level promise queue (mermaid's per-diagram DB singletons are global + non-reentrant).

## Bundle guard

`assertLazyChunks` (a Rollup `generateBundle` plugin) **fails `npm run build`** if any
forbidden module id (the code-split grammars, or mermaid + its heavy transitive deps) leaks
into an entry chunk, or if the entry chunk exceeds a measured byte budget. This is how "the
grammars and mermaid stay lazy" is a *checked* fact, not a hope. The forbidden list and
`maxEntryBytes` live in [`web/vite.config.ts`](../../web/vite.config.ts).

## Inline hashtags

The `#tag` inline node is registered as a `@lezer/markdown` extension in `hashtag.ts`. The
**same** extension array (`tagParserExtensions`) is used by the CM6 render *and* by
`inlineTagsFromBody`, the function that materializes body `#tags` into the frontmatter
`tags:` on save. Because both run the identical Lezer parse, what badges on screen can
never disagree with what gets written to frontmatter — a `#` inside code, an ATX heading
marker, an all-digit run, or an escape is excluded the same way in both. (The old Phase-21
string scanner `parseInlineTags` diverged on code regions and is **retired** — do not
reintroduce a string-scanner tag extractor.)

**Server-side parity (v1.5).** The identical grammar now also has a Go counterpart:
`okf.FindHashtags` (`internal/okf/hashtag.go`) recognizes the SAME body `#tag` spans
server-side, so a workspace-wide rename/merge/delete rewrites the body `#tags` where the
editor would badge them. It is pinned offset-for-offset to this Lezer grammar by a
single-sourced parity corpus: `hashtag.parity.test.ts` runs the real parser via
`inlineTagSpansFromBody` and regenerates `internal/okf/testdata/hashtag_corpus.json`; the Go
test asserts byte `from`/`to` spans against it, so drift fails CI on either side. See
[tag-subsystem.md](tag-subsystem.md#okffindhashtagsbody----the-single-body-tag-recognizer-internalokfhashtaggo).

## Security invariants (do not break)

- **No `innerHTML` of page content, ever.** Every widget builds DOM via
  `createElement` + `textContent`/`createTextNode`. Alt text, table cells, hashtag labels,
  and syntax-highlight token runs are all untrusted page bytes set through `textContent`.
  Tests assert on element structure, never HTML strings, to lock this in.
- **Image src is allowlisted** (`sanitizeImageSrc`): http(s) + app-relative only. A blocked
  src falls back to rendering the raw `![alt](src)` as plain text — never a loaded/executed URL.
- **Mermaid strict mode is the event-handler-XSS defense.** The widget's `DOMParser` +
  `importNode`/`appendChild` injection blocks `<script>` execution but does not strip
  `onload=`/`onclick=` attributes — that stripping is done upstream by mermaid's
  `securityLevel:'strict'` → DOMPurify. Keep strict mode; re-audit on any mermaid major bump.
- **GFM-only parser** keeps the editor, server (Goldmark), and read view (remark-gfm) in
  agreement — do not switch to the `markdownLanguage` superset.

## Tests

`web/src/lib/cm/*.test.ts` (and the `*.adversarial.test.ts` suites for widgets + mermaid)
pin the byte-stable round-trip, the reveal behavior, the hashtag corpus, and the security
invariants. `web/vite-plugins/assertLazyChunks.test.ts` covers the build guard. Treat an
adversarial-suite failure as a possible XSS/round-trip regression, not flakiness.

## Reading shell + selection/coordinate accuracy (v1.5)

The v1.5 reading-shell fold-in reshaped the column AROUND the CM6 surface into a document,
and hardened the surface's coordinate mapping.

### Reading column as a document

`PageView` (read) and `PageEditor` (edit) now frame the CM6 body with Obsidian-parity chrome
instead of full-width action rows:

- **`Breadcrumb.tsx`** — the folder trail above the title (`Cooking / Recipes`); each crumb
  navigates to `folder/index.md`. A top-level page renders nothing.
- **`PropertiesPanel.tsx`** — the compact metadata table under the title (tags, description,
  arbitrary scalar frontmatter keys). It **replaces `TagBadgeBar` (retired)** and folds the
  tag/description verbs into small inline ✨ slots inside the relevant property row. Tags are
  authored as `#tag` in the body (or via ✨ suggest), so the panel shows them read-only + their
  ✨ slot; it consumes `inlineTagsFromBody` to mark body-origin tags. Read mode (`readOnly`)
  shows values only.
- **`DocStatusBar.tsx`** — the footer strip (backlink count via the shared `useBacklinks`
  query, word count, character count).

`TagBadgeBar.tsx` is deleted; do not reintroduce it. The other `inlineTagsFromBody`
consumers are `PropertiesPanel.tsx`, `TagSuggest.tsx`, and the save reconcile
`lib/tag-reconcile.ts`.

### `drawSelection` + `lineWrapping` + the padding height-oracle fix

`LivePreviewEditor.tsx` adds `drawSelection()` and `EditorView.lineWrapping` to BOTH the read
and edit configs:

- **`drawSelection()`** renders CM6's OWN selection + cursor layers instead of the browser's
  native selection. Native selection fights the live-preview decorations (hidden markers, `#tag`
  badges, image/table/code widgets, atomic ranges) and produces janky drag-select + wrong-text
  copy; the CM layer maps selection to document offsets, so drag + Cmd-C stay robust over
  rendered constructs.
- **`EditorView.lineWrapping`** wraps long lines to the pane width (no horizontal scroll).
- **The padding fix (`theme.ts`)** is what makes `drawSelection` land correctly: the heading
  line classes (`.cm-heading-1..6`) switched their vertical rhythm from `margin` to `padding`.
  These are LINE decorations on `.cm-line`, and CM6's height oracle measures each line's
  `offsetHeight`, which INCLUDES padding but EXCLUDES margin. A margin is invisible to the
  oracle, so `posAtCoords`/`coordsAtPos` drift by the margin below every heading — with
  `drawSelection` on, the selection lands a row off. Padding is measured, so coordinates stay
  exact; the two are visually identical (headings have no background/border). The `.cm-content`
  block also moved to a centered doc column governed by the `.pageeditor`/`.pageview` max-width
  with generous bottom padding.
