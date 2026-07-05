---
author: engineer
responsible: architect
phase: 41
phase_type: implementation
status: EXECUTE COMPLETE — awaiting qa
verdict: COMPLETE
requirements: [RE-03]
---

# Phase 41: TipTap Editor — #tag + Suggestions + Slash + Real Save - Summary

**Verdict: EXECUTE COMPLETE.** A live `@tiptap/react` editor surface implementing the Phase-40
`EditorSurfaceHandle`, with a `#tag` badge node, `#`-vocabulary suggestions, `/`-slash block commands,
materialization through the EXISTING Lezer save path, and hardened paste + popup surfaces. TipTap is
fully code-split (out of the entry chunk).

> **Recovery note (author of record):** the engineer subagent built all artifacts AND ran full
> verification to green (937 tests, `npm run build` exit 0, assertLazyChunks GREEN, isolation
> confirmed), but its task wrapper was cancelled on a final *cosmetic* step (rephrasing comments to
> dodge a literal-`innerHTML` grep). The architect recovered this SUMMARY after re-verifying the built
> state directly on disk. No code authored by the architect — only this SUMMARY + verification.

## What shipped

- **Deps:** `@tiptap/react` + `@tiptap/suggestion` at exact **3.27.1** (Phase 38 had deferred these).
  `npm audit` clean (no new advisories).
- **`web/src/lib/tiptap/tagNode.ts`** — `Tag` inline-atom node. `renderHTML` → a `span[data-tag].tag-badge`
  with a **text child (`#${name}`, textContent — never innerHTML)**; **`renderMarkdown` → literal
  `` `#${name}` ``** (BLK-01: the saved body carries `#tag` so the EXISTING Lezer materializer sees it).
  `nodeInputRule` + `nodePasteRule` with a regex **mirroring `cm/hashtag.ts`** (rejects all-digit/mid-word/
  bare/heading; accepts `#a/b`, `#-1`, `#_`). Display-only; never materializes tag data itself.
- **`web/src/components/TipTapEditor.tsx`** — `forwardRef<EditorSurfaceHandle>` `@tiptap/react` surface:
  `useEditor({ extensions:[...buildExtensions(), Tag, TagSuggest, SlashCommands], content:value,
  contentType:"markdown", editable:!readOnly, editorProps:{transformPastedHTML: sanitizePastedHtml},
  onUpdate:({editor})=>onChange(editor.getMarkdown()) })` + `<EditorContent/>`; `insertAtCursor` via
  `useImperativeHandle` → `insertContent(text,{contentType:"markdown"})`. Same props as `LivePreviewEditor`
  — a drop-in surface. StrictMode-safe (hook owns teardown).
- **`web/src/lib/tiptap/suggestion.ts`** — `TagSuggest` (`char:"#"`, items from vocabulary) +
  `SlashCommands` (`char:"/"`, heading/list/code/quote via EXISTING schema commands). Popups render item
  text via **`textContent` / DOM text nodes — never `innerHTML`** (MAJ-03; the code comments document the
  guard — those are the only `innerHTML` string occurrences, no actual usage).
- **`web/src/lib/tiptap/sanitizePaste.ts`** — `sanitizePastedHtml` strips `<script>/<iframe>/<object>/<embed>`,
  `on*=` handler attrs, and `javascript:` URLs (MAJ-05 Layer 2).
- **`web/src/api/client.ts`** — `getTagVocabulary()` (`GET /api/v1/tags/vocabulary`, any-authed).
- **`web/src/routes/PageEditor.tsx`** — `renderEditorSurface("tiptap")` mounts the TipTap surface via
  `React.lazy(() => import(...))` in `<Suspense>` (code-split; still NOT user-selectable — Phase 42 adds the UI).
- **`web/src/lib/tiptap/preserved.ts`** — the Phase-39 P3 fold: the inline-math tokenizer tightened
  (pandoc no-space-adjacent-to-`$`) so prose `$5 and $10` stays literal; real math (`$x_1$`, `$a + b$`) still captured.

## Security (the milestone keystone)

- **MAJ-05 (paste):** BOTH layers. Layer 1 = ProseMirror's schema-bound `clipboardParser` drops unknown
  nodes (`<script>`/`<iframe>` have no `parseDOM` rule → dropped structurally). Layer 2 =
  `editorProps.transformPastedHTML` scrubs dangerous attrs/URLs. The `TipTapEditor.test.tsx` paste test
  drives the FAITHFUL two-layer path (sanitize → `DOMParser.fromSchema` parse) and asserts a
  `<img onerror>`/`<script>`/`<div onclick>` payload yields no live node and no executable attribute.
- **MAJ-03 (popup):** suggestion + slash popup item text is set via `textContent`/DOM text nodes; a
  markup-bearing vocabulary entry stays inert text. No `el.innerHTML` usage (the token appears only in
  guard-documenting comments).

## Evidence (re-verified on disk by the architect)

- `cd web && npx vitest run` → **69 files, 937 tests passed** (898 after Phase 39/40 → +39 Phase-41 tests).
- `cd web && npx tsc --noEmit` → exit 0.
- `cd web && npm run build` → exit 0; **`assertLazyChunks` GREEN**; entry chunk (`index-*.js`) has
  **ZERO** `@tiptap/react`/`useEditor` refs — TipTap lives ONLY in the lazy `TipTapEditor-*.js` chunk
  (banks Phase-42 code-split isolation).
- BLK-01 materialization: the live-editor test feeds the TipTap-serialized body through the REAL
  `reconcileFrontmatterTags` → `inlineTagsFromBody` (Lezer) and asserts the same materialization CM6
  produces — no second materializer.
- P3: `It costs $5 and $10` stays literal prose (no `preservedInline` atom); real inline math still captured.

## Notes / carried forward

- `"tiptap"` is wired + mountable but NOT user-selectable (no UI sets the mode) — **Phase 42** adds the
  toggle + load guard + the FORBIDDEN_IN_ENTRY assertion for the whole TipTap tree.
- Pre-existing repo-wide `react-hooks` 7.x eslint findings (in untouched files: PresenceIndicator,
  TagManagement, GraphView, etc.) are NOT from this phase; the Phase-41 files are eslint-clean.
