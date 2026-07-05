---
author: architect
responsible: architect
phase: 41
phase_type: implementation
status: Ready for planning
hard_bar: true
security_relevant: true
design_fork: false
parent_artifacts:
  - .planning/ROADMAP.md
  - .planning/REQUIREMENTS.md
  - .planning/phases/40-editor-host-abstraction/40-SUMMARY.md
  - .planning/phases/39-verbatim-preservation-tier/39-VERIFICATION.md
---

# Phase 41: TipTap Editor — #tag + Suggestions + Slash + Real Save - Context

**Gathered:** 2026-07-02
**Status:** Ready for planning

<domain>
## Phase Boundary

Build the LIVE `@tiptap/react` editor **surface** — mounted via the Phase-40 `EditorSurfaceHandle`
seam — with the app's authoring affordances: an inline `#tag` badge node, `#`-triggered tag-vocabulary
suggestions, and `/`-triggered slash commands. Body `#tags` materialize into frontmatter through the
**EXISTING** save path (not TipTap's own). Harden the paste + popup surfaces.

**Requirements:** RE-03 · **SECURITY: MAJ-05 (paste — the sharp surface) + MAJ-03 (suggestion popup)**

**Grounded (research-verified `@tiptap/react`/`@tiptap/suggestion`/core 3.27.1 — `agent://TipTapEditorResearch`):**
- `useEditor({ extensions:[...buildExtensions(), Tag, TagSuggest, SlashCommands], content:value,
  contentType:"markdown", editorProps:{transformPastedHTML}, onUpdate:({editor})=>onChange(editor.getMarkdown()) })`
  + `<EditorContent editor={editor}/>`. StrictMode-safe teardown (hook owns `destroy`). React 19 OK.
- The surface implements `EditorSurfaceHandle` via `forwardRef`/`useImperativeHandle`:
  `insertAtCursor(text) => editor.chain().focus().insertContent(text,{contentType:"markdown"}).run()`.
- Value-in = `content:md + contentType:"markdown"`; value-out = `editor.getMarkdown()` in `onUpdate`.

**Non-goals (out of this phase):**
- Exposing `"tiptap"` as a **user-selectable** mode / the toolbar toggle / the load guard — Phase 42
  (RE-04). This phase BUILDS the surface + wires `renderEditorSurface("tiptap")` to it, but users
  cannot reach it yet (no UI sets `"tiptap"`; the mode stays dev/test-only until Phase 42).
- Code-split / `assertLazyChunks` for the TipTap tree — Phase 42 (keep the surface behind ONE module
  boundary so 42 can lazy-load it; do NOT statically import it from the app entry).
- Editable/modeled tables or math — preserved verbatim (Phase 39); slash inserts only bounded-slice blocks.

**Depends on:** Phases 38 (codec), 39 (preserved tier), 40 (host seam) — all complete.
</domain>

<decisions>
## Implementation Decisions

### D-01: The Tag node is DISPLAY-only; data integrity flows through the Lezer materializer (BLK-01)
- The TipTap `Tag` inline-atom node (`group:"inline"`, `inline:true`, `atom:true`, `name` attr):
  - `renderHTML({node})` → `["span", {"data-tag":node.attrs.name, class:"tag-badge"}, `#${node.attrs.name}`]`
    — the 3rd array element is a **text hole (textContent, NEVER innerHTML)**; mirrors the shipped
    `PreservedInline` pattern.
  - **`renderMarkdown:(node)=>`#${node.attrs.name}`** — serializes back to **literal `#tag`** so the
    saved body carries `#tag`. THIS IS THE LOAD-BEARING FACT: tag materialization runs through the
    EXISTING `reconcileFrontmatterTags(body)` → `inlineTagsFromBody` (the Lezer recognizer) over the
    serialized body string on save (via the RE-05 host's `runSaver`) — NOT from TipTap storage. The
    Tag node never materializes tags itself.
  - `addInputRules` (`nodeInputRule`) + `addPasteRules` convert typed/pasted `#tag` into the node LIVE.
- **The input-rule regex MUST match the Lezer grammar corpus** (single-recognizer discipline): reject
  all-digit `#1984`, mid-word `foo#bar`, bare `#`, ATX `# heading`; accept `#a/b`, `#-1`, `#_`. If the
  live badge set forks from the Lezer materializer, that's a defect. Reuse/mirror `hashtag.ts`'s rules.

### D-02: `#`-suggestion + `/`-slash via `@tiptap/suggestion` (MAJ-03 popup safety)
- Two `Suggestion({ editor, char, pluginKey, items, command, render })` plugins wired via an
  Extension's `addProseMirrorPlugins`:
  - `#`-suggest: `items({query})` filters the tag **vocabulary** (`GET /api/v1/tags/vocabulary` — a
    NEW client fn `getTagVocabulary()` in `client.ts`, since none exists yet; any-authed read);
    `command` inserts a `tag` node. Reuses the existing vocabulary source (no new tag semantics).
  - `/`-slash: `items` = bounded-slice blocks (heading/list/code/quote); `command` runs
    `editor.chain().focus().deleteRange(range).setNode/toggle…().run()` — UI-only over EXISTING schema
    nodes (adds NO new schema node → serialization unchanged). Do NOT add a slash item that inserts a
    node absent from `buildExtensions()` (would break round-trip).
- **MAJ-03 (popup XSS):** the popup DOM is 100% developer-supplied (`render()` → onStart/onUpdate/
  onKeyDown/onExit; you mount your own element). Item names + labels MUST be set via **`textContent` /
  React children — NEVER `innerHTML`** — matching the shipped `TagSuggest.tsx:442-443` locked
  stored-XSS guard. A fixture with a markup-bearing vocabulary entry proves inertness.

### D-03: Paste hardening — BOTH layers (MAJ-05, the sharp surface)
- **Layer 1 (structural, confirmed):** ProseMirror's clipboard parser is schema-bound
  (`DOMParser.fromSchema`), so pasted `<script>`/`<iframe>`/unknown elements have no `parseDOM` rule
  and are **dropped**; parsed content is re-rendered inert via the schema's `renderHTML`.
- **Layer 2 (attributes):** the schema does NOT scrub dangerous ATTRIBUTES (`onerror`/`onclick`/
  `style`/`javascript:` URLs) on elements that DO map. Add `editorProps.transformPastedHTML(html)` to
  strip `<script>`/`<iframe>`/`<object>`/`<embed>`, event-handler attrs (`on*=`), and `javascript:` URLs
  BEFORE parsing.
- **The gate:** a paste-XSS fixture (`<img src=x onerror=…>`, `<script>…`, `<div onclick=…>`) proves
  the dangerous node/attr never becomes live — no element node with an executable attr in the doc, no
  execution. This is the milestone's SHARPEST untrusted surface → the security-review keystone.

### D-04: Materialization through the REAL save (BLK-01) — tested at the data-path level
- The RE-05 host's `runSaver` already calls `reconcileFrontmatterTags(surface's onChange body)` →
  `inlineTagsFromBody` (Lezer). So IF the TipTap surface's `getMarkdown()` output carries literal
  `#tag`, materialization works IDENTICALLY to CM6 — no new materializer.
- **Test the data path (BLK-01 crit-3):** assert that `reconcileFrontmatterTags` over a
  TipTap-serialized body (from typing `#tag` in the editor → `getMarkdown()`) materializes the SAME
  tags CM6 would — proving the surface routes through the real save path, not a headless-only harness.

### D-05: Fold the P3 finding from Phase 39 — tighten the inline-math tokenizer
- Phase 39 qa found the inline-math tokenizer over-matches two-dollar prose (`$5 and $10` → a
  `preservedInline` atom). Now that we have a LIVE editor where this is observable, **tighten the
  guard** (`preserved.ts`): apply the pandoc no-space-adjacent-to-`$` rule — the opening `$` not
  followed by whitespace, the closing `$` not preceded by whitespace (and optionally the closing `$`
  not followed by a digit) — so `$5 and $10` stays literal prose while `$x_1$`/`$a + b$` stay math.
  Add **prose-currency fixtures** (`It costs $5 and $10` stays plain text; real inline math still
  captured) to the codec corpus. Keep the byte-exact Tier-2 gate green.

### D-06: The surface is import-isolated (Phase 42 prep)
- Put the whole TipTap surface (component + Tag node + suggestion/slash extensions) behind ONE module
  boundary (e.g. `web/src/components/TipTapEditor.tsx` + `web/src/lib/tiptap/tagNode.ts` etc.). The app
  entry / CM6 path must NOT statically import it — so Phase 42 can `lazy()`-load it. This phase may
  wire `renderEditorSurface("tiptap")` to a **lazy/dynamic** import OR a direct import guarded so it's
  unreachable at runtime; the planner picks the shape that keeps it out of the entry chunk (verify with
  `assertLazyChunks` staying green — the surface must not inflate the entry bundle even now).

### The agent's discretion
- The exact module layout (one `TipTapEditor.tsx` vs split node/extension files) — keep it ONE boundary.
- Whether `renderEditorSurface("tiptap")` uses `React.lazy` now or a plain import that
  `assertLazyChunks` still tolerates (recommend lazy, to bank Phase 42's isolation early).
- The suggestion popup rendering (a small vanilla element vs a `ReactRenderer`) — textContent either way.
- Commit slicing (recommend: Tag node + codec round-trip first; then the React surface + insertAtCursor;
  then suggestion/slash; then paste hardening + security tests; then the P3 math-guard).
</decisions>

<disciplines>
## Lenses surfaced for this phase

~~~yaml
disciplines:
  - section: "D-03 paste hardening"
    name: "paste-is-the-sharp-untrusted-surface"
    scope: ["the TipTap surface editorProps", "the paste-XSS test"]
    guidance: "Pasted HTML/Markdown must parse through the bounded-slice schema ONLY. Rely on BOTH the
      schema-bound clipboard parser (drops unknown nodes: script/iframe) AND
      editorProps.transformPastedHTML (scrubs event-handler attrs, javascript: URLs, script/iframe/
      object/embed) — one without the other leaves a hole. A paste-XSS fixture must prove no dangerous
      node/attr becomes live. This is the milestone's sharpest untrusted-input surface and the security
      review keystone."
    rationale: "MAJ-05: the novel untrusted path in v1.7 is TipTap paste/serialization, not EP-01.
      ProseMirror re-renders parsed content inert, but a surviving onerror attribute on a mapped node
      is still a stored-XSS vector."
    override_signals:
      - "None. Both paste-defense layers are required; a paste-XSS fixture must pass."
  - section: "D-02 popup safety"
    name: "popup-text-never-innerHTML"
    scope: ["the suggestion + slash popups"]
    guidance: "The suggestion popup DOM is developer-supplied. Render vocabulary tags + command labels
      as textContent / React children, NEVER el.innerHTML — matching the shipped TagSuggest locked
      guard. A markup-bearing vocabulary entry must stay inert text in the popup."
    rationale: "MAJ-03: a @tiptap/suggestion popup is raw DOM and does not inherit React's auto-escape;
      innerHTML of a tag name would be a fresh XSS surface CM6's TagSuggest deliberately avoids."
    override_signals:
      - "None. textContent/React children only for popup item text."
  - section: "D-01/D-04 single-recognizer + real save"
    name: "tag-node-serializes-to-literal-and-lezer-materializes"
    scope: ["the Tag node", "the materialization test"]
    guidance: "The Tag node is display-only. renderMarkdown MUST emit literal `#tag` so the serialized
      body carries it, and materialization runs through the EXISTING reconcileFrontmatterTags ->
      inlineTagsFromBody (Lezer) on save — never from TipTap storage. The input-rule regex must match
      the Lezer grammar corpus or the live badge set forks from the saved tag set. Do NOT build a
      second materializer."
    rationale: "BLK-01 + single-recognizer discipline: tag DATA integrity must stay on the one Lezer
      recognizer; TipTap only DISPLAYS. A divergent TipTap recognizer would fork on-disk tags by editor."
    override_signals:
      - "None. One recognizer for data; TipTap Tag node is display + serialize-to-literal only."
~~~

</disciplines>

<canonical_refs>
## Canonical References

### Phase spec & requirements
- `.planning/ROADMAP.md` §"Phase 41" — goal, 5 success criteria, MAJ-05/MAJ-03.
- `.planning/REQUIREMENTS.md` §RE-03 (+ MAJ-01 clarified real-save, MAJ-05 paste, MAJ-03 popup).

### The seam + save path (Phases 38-40)
- `web/src/components/LivePreviewEditor.tsx:47-55,87-107` — `EditorSurfaceHandle { insertAtCursor }`
  (the interface the TipTap surface re-implements) + the CM6 reference implementation.
- `web/src/routes/PageEditor.tsx:733-752` — `renderEditorSurface()` (wire the `"tiptap"` branch to the
  TipTap surface; today it falls back to CM6 `"live"`).
- `web/src/lib/tag-reconcile.ts:51` — `reconcileFrontmatterTags` (the real materializer; runs
  `inlineTagsFromBody` Lezer over the serialized body — the BLK-01 data path).
- `web/src/lib/cm/hashtag.ts` — `inlineTagsFromBody` + the recognizer grammar the Tag node's regex must mirror.
- `web/src/lib/tiptap/{extensions,codec,preserved}.ts` — the codec/schema the live surface reuses; the
  P3 inline-math tokenizer to tighten (D-05).

### The MAJ-03 pattern to match
- `web/src/components/TagSuggest.tsx:442-443` — tag names as React text children, NEVER
  `dangerouslySetInnerHTML` (the locked stored-XSS guard).

### External (research — `agent://TipTapEditorResearch`)
- `@tiptap/react` 3.27.1 `useEditor`/`<EditorContent>` (StrictMode-safe); `getMarkdown()`;
  `insertContent(md,{contentType:"markdown"})`; `Node.create` (renderHTML text hole / renderMarkdown /
  `nodeInputRule`/`addPasteRules`); `@tiptap/suggestion` `Suggestion({char,items,command,render})`
  (developer-supplied popup); `editorProps.transformPastedHTML` + schema-bound clipboardParser.
- `GET /api/v1/tags/vocabulary` (`router.go:227`, `handleTagVocabulary`) — the `#`-suggest source
  (add `getTagVocabulary()` to `client.ts`).

### Test split
- Live editor → **jsdom** (`vitest.config.ts` default); codec no-DOM invariant stays **node** env.
</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable assets
- `EditorSurfaceHandle` seam (Phase 40) — the TipTap surface is a drop-in second implementation.
- `buildExtensions()` (Phases 38-39) — the bounded-slice + Tier-2 schema the live editor reuses.
- `reconcileFrontmatterTags`/`inlineTagsFromBody` — the real materializer (Lezer); no new one.
- `PreservedInline` (Phase 39) — the inline-atom + textContent + renderMarkdown pattern the Tag node mirrors.
- `TagSuggest.tsx` — the MAJ-03 textContent-not-innerHTML popup pattern to match.

### Established patterns
- Surface value:string-in (`content`+`contentType:markdown`) / string-out (`getMarkdown()` in onUpdate).
- textContent-only DOM for anything rendering tag/user text (the locked XSS guard).
- node-env (codec no-DOM) vs jsdom (live component) test split.

### Landmines
- **Tag node display-only** — renderMarkdown emits literal `#tag`; materialization via Lezer over the
  serialized body (BLK-01). A second/divergent materializer or a regex that forks the Lezer corpus is a defect.
- **Paste needs BOTH layers** (MAJ-05) — schema drops unknown nodes; `transformPastedHTML` scrubs attrs.
- **Popup textContent-only** (MAJ-03) — never innerHTML of a tag/label.
- **Slash items = existing schema nodes only** — a node absent from `buildExtensions()` breaks round-trip.
- **Import-isolate the surface** (Phase 42 prep) — keep it out of the entry chunk; `assertLazyChunks` stays green.
- **P3 math-guard (D-05)** — tighten the inline-math tokenizer so prose `$5 and $10` isn't a math atom.
- **Test split** — live editor under jsdom; codec no-DOM invariant stays node-env.
</code_context>

<deferred>
## Deferred Ideas

- **User-selectable `"tiptap"` + toolbar toggle + load guard** — Phase 42 (RE-04).
- **Code-split proof (`assertLazyChunks` + FORBIDDEN_IN_ENTRY for the TipTap tree)** — Phase 42.
- **Modeled tables/math editing** — Future (preserved verbatim in v1.7).
</deferred>

---

*Phase: 41-tiptap-tag-suggest-slash*
*Context gathered: 2026-07-02*
