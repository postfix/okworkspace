---
author: codebase-archaeologist
responsible: codebase-archaeologist
phase: 21, 22, 25, 26, 28
milestone: v1.5
status: verified
confidence: direct_read
grounding: direct file read + SMTC Structural-tier queries (find_declarations/find_name_occurrences, daemon live 2026-07-01). Semantic call-graph tier (blast_radius/change_impact) COLD this session — cross-file index empty (file_count 0, built 2026-06-17); needs a daemon reindex against this root (daemon-ops, not agent-reachable).
head: f03cffc
---

# Codebase Map: CM6 Live-Preview Editor Subsystem

## Subsystem: `web/src/lib/cm/*` — the Obsidian-style live-preview Markdown surface

The load-bearing client subsystem behind the whole editor. It is a hand-built
CodeMirror 6 surface (no React wrapper): **the CM6 document IS the raw Markdown
string**, and everything rendered (bold, headings, links, images, tables, `#tag`
badges, syntax-highlighted code, mermaid diagrams) is a **view-layer decoration** —
never a re-serialization of the doc. That single fact is why the byte-stable
round-trip holds. Phases 21/22 (hashtags), 25 (syntax highlight), 26 (mermaid) all
extended this surface; **Phase 28 (v1.5)** touched it again — `drawSelection()` +
`EditorView.lineWrapping` on the host, and the `theme.ts` heading margin→padding
height-oracle fix — and reshaped the reading column around it (Breadcrumb +
PropertiesPanel + DocStatusBar; `TagBadgeBar` retired).

Original surface shipped in **Phase 6** (EDIT-01..04); this map covers the whole
current state including the v1.4 additions and the v1.5 Phase-28 changes.

### Files

| File | Role |
|------|------|
| `web/src/lib/cm/mode.ts` | The **Live/Source toggle seam** (EDIT-02). `modeCompartment` holds either `liveExtensions` or `sourceExtensions`; `setMode` reconfigures it with a dispatch carrying `effects` ONLY (never `changes`) → byte-identical `doc.toString()` across Live⇄Source. `toggleKeymap` = Cmd/Ctrl-E (`Mod-e`). `liveExtensions` = `[markdownExtension, livePreviewExtension, theme, syntaxHighlighting(codeHighlightStyle)]`; `sourceExtensions` = `[markdownExtension, theme, sourceTheme]`. |
| `web/src/lib/cm/markdown.ts` | The CM6 Markdown language, configured **GFM-only** (`base: commonmarkLanguage`, `extensions: tagParserExtensions`, `codeLanguages`). Deliberately NOT the `markdownLanguage` superset (would add emoji/sub/superscript that server Goldmark + read `remark-gfm` do not support → render divergence, RESEARCH Pitfall 4). |
| `web/src/lib/cm/livePreview.ts` | **The decoration engine.** A `ViewPlugin` (inline marks + zero-width marker hides + heading-line classes + inline image/hashtag widgets) PLUS two `StateField`s — `tableField` and `codeBlockField` — for **block** widgets. Exports the `cleanRender` `Facet` (read-mode switch) and the `livePreviewExtension` bundle. Owns the active-line reveal + atomicRanges logic. |
| `web/src/lib/cm/widgets.ts` | The `WidgetType` family: `ImageWidget`, `BulletWidget`, `CodeBlockWidget` (async syntax highlight), `MermaidWidget` (async render), `HashtagWidget`, `TableWidget`. Home of the **no-innerHTML / textContent-only** security invariant. |
| `web/src/lib/cm/highlight.ts` | ED-04a. The ONE shared `codeHighlightStyle` (`HighlightStyle.define`, ~5 `tok-*` roles, class-only → colors are `--syntax-*` CSS vars) + the curated `codeLanguages` (`LanguageDescription[]`, every grammar loaded via dynamic `import()` → lazy chunks). |
| `web/src/lib/cm/mermaid.ts` | ED-05a. `renderMermaid(code)` — lazy `import("mermaid")`, `securityLevel:'strict'` + `suppressErrorRendering:true`, init-once, **serialized behind a module-level promise `queue`** (mermaid's per-diagram DBs are global + non-reentrant). Returns a DOMPurify-sanitized SVG string; rejects on invalid input. |
| `web/src/lib/cm/hashtag.ts` | ED-02a/b. `hashtagExtension` (a `@lezer/markdown` `MarkdownConfig` registering a `Hashtag` inline node), `tagParserExtensions` (the SINGLE source of truth: `[Table, Strikethrough, TaskList, Autolink, hashtagExtension]`), `inlineTagsFromBody(body)` (standalone parser reusing the SAME extension set → materialized tags can never fork from the badges), and (v1.5) `inlineTagSpansFromBody(body)` — byte-offset spans (NOT de-duped) that generate the Lezer↔Go parity corpus (`internal/okf/testdata/hashtag_corpus.json`) the server-side `okf.FindHashtags` is pinned against. See `MAP-tag-rewrite-subsystem.md`. |
| `web/src/lib/cm/theme.ts` | The `EditorView.theme` — every value a `var(--…)` token (no hex/px). `theme` (shared) + `sourceTheme` (mono document-wide). Decoration classes (`.cm-strong/.cm-em/.cm-heading-N/.cm-md-link/.cm-hashtag/.cm-inline-code/.cm-code-block`) styled here at UI-SPEC token values → Live mode looks byte-for-byte like read mode. **v1.5 (Phase 28):** heading line classes use `padding`, NOT `margin`, for vertical rhythm — CM6's height oracle measures `.cm-line` `offsetHeight` (padding included, margin excluded), so a margin drifts `posAtCoords`/`coordsAtPos` below every heading (visible as `drawSelection` landing a row off). `.cm-content` moved to a centered doc column + bottom padding. |
| `web/src/lib/cm/sanitizeSrc.ts` | `sanitizeImageSrc(src)` — the image-src allowlist (http(s) + app-relative only; blocks `javascript:`/`vbscript:`/executable `data:`/protocol-relative). Gate for `ImageWidget`. |
| `web/src/lib/cm/headingAnchors.ts` / `linkNav.ts` | github-slugger heading ids (read-mode `#hash` deep-link) and the internal-link DOM click router (resolve `.md` links through react-router). Pre-v1.4; unchanged. |
| `web/vite-plugins/assertLazyChunks.ts` | The reusable **build-time guard** (Rollup `generateBundle`): fails `npm run build` if any `forbidden` module id leaks into an entry chunk, or the entry exceeds `maxEntryBytes`. Wired in `web/vite.config.ts`. |

### Consumers (the two mount points)

- `web/src/components/LivePreviewEditor.tsx` — the ONE CM6 host component. Edit mode
  and read mode are the SAME component with different extension sets:
  - **Edit mode**: `modeCompartment.of(mode === "live" ? liveExtensions : sourceExtensions)`, history, keymaps, `toggleKeymap`, placeholder, `onChange` (fires only on `docChanged`, bytes verbatim — EDIT-03).
  - **Read mode** (`readOnly`): `cleanRender.of(true)` + `modeCompartment.of(liveExtensions)` + `headingAnchors`, non-editable. **T-06-10: read mode reuses the SAME decoration pipeline as edit Live — there is no second, weaker read renderer.** (`LivePreviewEditor.tsx:138-160`.) **v1.5:** both configs also add `drawSelection()` (CM6 selection/cursor layers instead of native — robust drag/copy over decorations + atomic ranges) and `EditorView.lineWrapping`.
- `web/src/routes/PageView.tsx` — mounts `LivePreviewEditor` read-only as the unified read surface. **v1.5 reading shell:** it now frames the surface with `Breadcrumb` (folder trail) + `PropertiesPanel` (metadata table, replaces the retired `TagBadgeBar`) + `DocStatusBar` (backlink/word/char footer); `PageEditor.tsx` mirrors the chrome in edit mode.

### How edit vs read compose (the `cleanRender` facet)

`cleanRender` (`livePreview.ts:68`) is a boolean `Facet`, combined with `.some(Boolean)`.
**Only `LivePreviewEditor.tsx:156` ever provides `cleanRender.of(true)`, in read mode.**
Edit mode never provides it → defaults `false`. The single facet flips three behaviors
inside `buildDecorations` / `buildCodeBlockDecorations`:

| Behavior | Edit (`clean=false`) | Read (`clean=true`) |
|---|---|---|
| Active-line reveal | ON — a marker on a selection-touched line is NOT hidden (raw shown for editing) | OFF — `activeLines` stays empty, so EVERY marker hides (even line 1's `# `) |
| Unordered list `-`/`*`/`+` | left raw/editable | swapped for a `BulletWidget` `•` |
| Blockquote `> ` | left raw/editable | hidden + line styled (`cm-blockquote`) |
| Non-mermaid fenced code | kept raw fences + `cm-code-block` styling | replaced by a clean `CodeBlockWidget` `<pre>` (no fences) |
| `mermaid` fence | rendered diagram (Obsidian parity) UNLESS selection intersects | rendered diagram (always) |

### Decoration pipeline (deterministic)

```
CM6 doc (verbatim Markdown)
  → markdownExtension parses the Lezer GFM tree (Table/Strike/TaskList/Autolink + Hashtag)
  → livePreview ViewPlugin.buildDecorations (per visibleRanges):
        inline marks (cm-strong/em/inline-code/code-block/md-link)
      + heading-LINE classes (cm-heading-1..6)
      + zero-width marker HIDES (**, *, "# ", backticks, link [] and (url))   ← NON-atomic
      + inline Image widget            ← atomic (widgetRanges)
      + inline Hashtag badge           ← NON-atomic (short inline text)
      + (read only) bullet widget / blockquote line
  → tableField StateField.buildTableDecorations (WHOLE doc):
        block Table widget             ← atomic
  → codeBlockField StateField.buildCodeBlockDecorations (WHOLE doc):
        block mermaid MermaidWidget    ← atomic (read: always; edit: unless selection intersects)
      + (read only) block CodeBlockWidget for every other fence   ← atomic
  → EditorView renders decorations (line decos sorted ahead of inline — Decoration.set(_, true))
```

The ViewPlugin issues **no document transactions and never serializes the tree back to
text** — `doc.toString()` is always the verbatim Markdown (EDIT-02/EDIT-03 hold by
construction).

### Async-widget + lazy-chunk pattern (D-04/D-06, shared by CodeBlockWidget + MermaidWidget)

Both heavy widgets follow one discipline:
1. `toDOM` renders a **synchronous safe fallback FIRST** (plain `<pre><code>` text via
   `textContent`) — ships even if the async work never resolves.
2. Kick off a **fire-and-forget async pass** (dynamic `import()` of the grammar / mermaid).
3. On success, build the token/SVG DOM into a **detached fragment**, then **atomically
   swap** it in (`while (firstChild) removeChild` → `appendChild`).
4. **Teardown/stale guard**: a per-instance `destroyed` flag (set by CM6 `destroy()`) AND
   an `isConnected` liveness check on both sides of the `await` — a resolved result is
   discarded rather than written into a dead/detached node. `MermaidWidget` also calls
   `view?.requestMeasure?.()` after the height-changing swap.
5. Any throw (import, parse, emit) is swallowed → the plain fallback survives.

Lazy grammars/mermaid are code-split by Vite because the ONLY place they are imported is
a dynamic `import()` inside a `load`/`render` closure — the `assertLazyChunks` build guard
proves they never leak into an entry chunk.

## Undocumented invariants & landmines (per-claim confidence)

1. **`atomicRanges` covers ONLY block/inline WIDGETS (image, table, mermaid, read-mode
   code block) — NEVER the text-marker hides and NEVER the `HashtagWidget`.** (HIGH —
   `livePreview.ts:39,178-182,220,290,411`.) This is Pitfall 1 (the cursor trap): making a
   zero-width marker-hide atomic would trap the caret on a marker run and make backspace
   feel broken. A future widget added to the inline path MUST decide deliberately whether
   it is atomic; short inline text (like the hashtag badge) must stay non-atomic.

2. **Marker reveal is SELECTION-driven (`update` recomputes on `u.selectionSet`), never
   timer/debounce-driven.** (HIGH — `livePreview.ts:397-405`, comment at 398.) A debounce
   would lag the cursor. Don't "optimize" the recompute onto a timer.

3. **Reveal must be LAYOUT-NEUTRAL (Pitfall 2).** No decoration class changes line-height;
   bold is weight-only on body text, heading lines are heading-height whether or not the
   `# ` is revealed. (HIGH — `theme.ts:22-25,52-59,110-118`.) A future style that resizes
   on reveal would reflow neighbors when the cursor enters/leaves a line.

4. **Block widgets (table, mermaid, read-mode code block) MUST come from a `StateField`,
   not the `ViewPlugin`.** (HIGH — `livePreview.ts:317-321,418-424,477-492`.) CM6 forbids
   block / line-break-spanning replace decorations from a `ViewPlugin.decorations`
   provider. That is the entire reason `tableField` + `codeBlockField` exist separately
   from the inline plugin. A new block-level construct goes in a StateField.

5. **The StateFields compute over the WHOLE document (no viewport).** (HIGH —
   `livePreview.ts:424-425,493`.) A `StateField` has no viewport; fine at this app's page
   sizes (CM6 only renders visible decorations anyway), but a landmine if page sizes grow
   dramatically — it would become O(doc) per selection change.

6. **`securityLevel:'strict'` + `suppressErrorRendering:true` in `mermaid.ts` are BOTH
   load-bearing and must never be dropped.** (HIGH — `mermaid.ts:24-29`; security review
   F-01.) Strict routes the SVG through DOMPurify (the ONLY defense against event-handler
   XSS — the widget's DOMParser injection blocks `<script>` execution but does NOT strip
   `onload=`/`onclick=` attributes). `suppressErrorRendering` is what makes `render` throw
   (→ raw-code fallback) AND clean up its temp `<div>` instead of leaving a "Syntax error"
   SVG + orphan node on `document.body`. Re-run the F-01 trace on any mermaid **major** bump.

7. **`renderMermaid` calls are serialized through a module-level promise `queue`.** (HIGH —
   `mermaid.ts:45,53-69`.) mermaid's per-diagram-type DB singletons (e.g. `flowDb`) are
   module-global and non-reentrant; a page with ≥2 `mermaid` blocks fires concurrent
   renders whose parse→render await-gaps would clobber each other. The `.catch(() => {})`
   on the chain is deliberate — one bad diagram must not wedge the queue.

8. **All widget DOM is built with `createElement` + `textContent`/`createTextNode` — NEVER
   `innerHTML` of page content.** (HIGH — `widgets.ts:17-22,57-60,142-143,184-189,386-411`;
   security review F-01/F-02.) Alt text, table cells, hashtag labels, code token runs, the
   raw-markdown image fallback: all through `textContent`. Tests assert on element
   structure (tagName/className/textContent), never HTML strings, specifically to lock this
   in. Adding a widget that assigns `innerHTML` from page bytes is a stored-XSS regression.

9. **Grammar matching is `matchLanguageName(…, fuzzy=false)` — exact name/alias only.**
   (HIGH — `widgets.ts:159`, `highlight.ts:82-85`.) An UNLISTED info string (`jsonl`,
   `mermaid`, `diffstat`) must fall through to plain text, never substring-match a curated
   grammar. `mermaid` is deliberately ABSENT from `codeLanguages` (routed to the mermaid
   widget instead) — do not add it there.

10. **LANDMINE — `parseInlineTags` is RETIRED, not dead code to resurrect.** (HIGH — grep:
    zero non-comment/non-test references; `hashtag.ts:15-20`.) The Phase-21 pure string
    scanner diverged from the render on code regions and block-marker boundaries. It was
    SUPERSEDED by `inlineTagsFromBody` (structural Lezer parse sharing the render's exact
    extension set). Do not reintroduce a string-scanner tag extractor — the whole point of
    `tagParserExtensions` being a single shared array is render⇄materializer agreement.

11. **`inlineTagsFromBody` is NOT dead — it is the materializer.** (HIGH — consumed by
    `components/PropertiesPanel.tsx:6,57`, `components/TagSuggest.tsx:12,138`, and the save
    reconcile `lib/tag-reconcile.ts:21,52-53`.) It runs a full (non-incremental) Lezer parse
    of the whole body on each call; that is intentional and cheap at page scale. **v1.5:
    `TagBadgeBar.tsx` (its old consumer) was DELETED — `PropertiesPanel.tsx` replaced it as
    the metadata surface.** Its byte-offset sibling `inlineTagSpansFromBody` also feeds the
    Lezer↔Go parity corpus (`hashtag.parity.test.ts`).

12. **The read surface has NO separate renderer.** (MEDIUM-HIGH — `LivePreviewEditor.tsx:
    138-160`, `PageView.tsx`.) A common wrong assumption is that read mode uses
    `react-markdown`. It does not — read mode is `LivePreviewEditor` + `cleanRender.of(true)`
    over the same decoration pipeline. Changing a decoration changes BOTH edit-Live and read.
    (Note: `react-markdown`/`rehype-sanitize` are still project deps and may be used
    elsewhere; the page *body* read surface is CM6.)

13. **The GFM-only parser is a parity contract, not a limitation.** (HIGH — `markdown.ts:
    1-8`.) Editor parse set MUST equal server Goldmark's GFM set and the read view's
    remark-gfm set. Adding a construct here (e.g. switching to `markdownLanguage`) silently
    forks what badges on screen from what the server stores. `codeLanguages` is purely
    additive (it only tells the parser which nested fences to lazy-highlight) and does NOT
    change the parsed construct set.

## Blast radius

### SMTC-grounded dependency edges (Structural tier — `find_name_occurrences`/`find_declarations`, daemon live 2026-07-01)

> Tier note: these are REAL cross-file (+ cross-language) import/call-target edges resolved by the
> SMTC structural index against this workspace — stronger than grep (context-classified). The
> semantic call-graph `blast_radius`/`change_impact` (transitive callers ranked by distance) is
> COLD this session (empty cross-file index, built 2026-06-17); those return empty until the daemon
> is reindexed against this root. So the edges below are the DIRECT dependency ring, SMTC-verified;
> transitive-closure ranking is not yet available.

The CM6 editor surface converges on **`web/src/lib/cm/mode.ts`** (the extension-composition hub):
- `theme.ts` exports `theme` (:28) + `sourceTheme` (:134) → imported ONLY by `mode.ts:18`. **The v1.5
  margin→padding height-oracle change (theme.ts) has a single production consumer: `mode.ts`.**
- `livePreview.ts` `livePreview`/`livePreviewExtension` → `mode.ts:17` + `LivePreviewEditor.tsx:14`
  (`cleanRender`). `markdown.ts` `markdownExtension` → `mode.ts:16`.
- `mode.ts` exports `liveExtensions`/`sourceExtensions`/`setMode`/`modeCompartment`/`toggleKeymap`
  → the editor wiring + `mode.test.ts` / `markdown.test.ts` (the composed extension arrays are the
  editor's entry surface).
- **Cross-cutting (highest fan-out): `hashtag.ts` `inlineTagsFromBody`** → 3 production consumers —
  `PropertiesPanel.tsx:57` (tag union), `TagSuggest.tsx:138` (suggest merge), `tag-reconcile.ts:52`
  (save reconcile). This is the tag surface the v1.5 regression fix depends on; `inlineTagSpansFromBody`
  additionally feeds the Go parity corpus (see the cross-language ripple below + `MAP-tag-rewrite-subsystem.md`).

### Direct-read observations (retained; complement the edges above)
- `livePreview.ts` `buildDecorations` / the `cleanRender` facet → affects BOTH `LivePreviewEditor`
  edit-Live AND `PageView` read mode simultaneously. Highest-fan-out file in the subsystem.
- `widgets.ts` widget DOM → the security invariant surface; a change here is a security-review
  trigger (route to security-engineer). `CodeBlockWidget` is reused by `MermaidWidget` as its
  fallback, so editing it touches mermaid too.
- `highlight.ts` `codeHighlightStyle` → consumed by both `mode.ts` (`syntaxHighlighting`) and
  `widgets.ts` (`highlightCode`) — one source of truth, so a token change re-colors both surfaces.
- `highlight.ts` `codeLanguages` → adding/removing a grammar changes the Vite chunk graph;
  a new **static** import (vs dynamic) will fail `assertLazyChunks` at build. Update
  `FORBIDDEN_IN_ENTRY` in `vite.config.ts` in lockstep.
- `hashtag.ts` `tagParserExtensions` → the shared array; a change ripples to the render
  (`markdown.ts`), the materializer (`inlineTagsFromBody`), and the save reconcile
  (`lib/tag-reconcile.ts`). Its corpus is pinned in `hashtag.test.ts` + `livePreview.test.ts`.
  **v1.5 cross-language ripple:** the grammar is ALSO the source of truth for the server-side
  `okf.FindHashtags` scanner via `inlineTagSpansFromBody` → `hashtag.parity.test.ts` →
  `internal/okf/testdata/hashtag_corpus.json`. A grammar change now requires regenerating that
  corpus (`UPDATE_CORPUS=1 npx vitest run src/lib/cm/hashtag.parity.test.ts`) or CI fails on
  BOTH sides. See `MAP-tag-rewrite-subsystem.md`.
- `mermaid.ts` `MERMAID_CONFIG` → security-critical; changing `securityLevel` is a security regression.
- `assertLazyChunks.ts` / `vite.config.ts` `maxEntryBytes` (1,420,000; measured entry ~1,180,566)
  → re-measure and raise deliberately if the entry legitimately grows; never raise to paper over a
  stray static import.

### Test corpus (where the invariants are pinned)

`livePreview.test.ts`, `widgets.test.ts` + `widgets.adversarial.test.ts`,
`mermaid.test.ts` + `mermaid.adversarial.test.ts`, `hashtag.test.ts`, `hashtag.parity.test.ts`
(v1.5 Lezer↔Go corpus), `highlight.test.ts`,
`markdown.test.ts`, `mode.test.ts`, `sanitizeSrc.test.ts`, `assertLazyChunks.test.ts`.
The adversarial suites are the security-invariant guards — treat a failure there as a
potential XSS/round-trip regression, not a flaky test.
