---
author: architect
---

# Phase 21: Inline `#tag` Parser + Badge Render — Context

**Gathered:** 2026-06-30
**Status:** Ready for planning

<domain>
## Phase Boundary

Add a `@lezer/markdown` inline-parser extension that recognizes body `#tag` tokens, and a
badge **decoration** that renders them as pills on the two CM6 surfaces (live editor + read
mode) — **decoration-only, byte-stable**. This is the pure, demoable foundation Phase 22
builds the frontmatter reconciliation on; Phase 21 writes **no frontmatter and mutates no
document bytes**.

The single known correctness landmine — *which `#` is a tag* — is pinned here by a
fixture corpus in `livePreview.test.ts` (the negative cases as load-bearing as the positive).

**In scope (ED-02a only):**
- A `Hashtag` inline node composed into `markdownExtension` (`markdown.ts`).
- A `HashtagWidget` (`widgets.ts`) + a `Hashtag` case in `buildDecorations` (`livePreview.ts`).
- `.cm-hashtag` badge styling (`theme.ts`).
- The positive + negative fixture corpus (`livePreview.test.ts`) + widget unit test.

**Out of scope (later phases / Future):**
- Any frontmatter `tags:` write / reconciliation → **Phase 22** (ED-02b).
- The badge bar UI / chip-field removal → **Phase 22** (ED-02c).
- LLM tag suggestion → **Phase 23**.
- Badge **click→navigate to tag search** (display-only badge in 21; a tag *model* is a 22
  concern). No `data-*` action wired yet.
- `#tag` badges in the **react-markdown** path (agent chat / diff preview) — deferred (Future).
</domain>

<decisions>
- **D-01 — Parser placement (client-only, single parser).** Define a `@lezer/markdown`
  `MarkdownConfig` (`hashtagExtension`) with a `Hashtag` inline node and add it to the
  `extensions: [Table, Strikethrough, TaskList, Autolink]` array in `markdown.ts`. There is
  **no server-side `#tag` parser** (locked decision — avoids the client/server drift PITFALLS
  warns about). The Lezer tree the existing `livePreview` ViewPlugin already walks gains a
  `Hashtag` node for free; no new tree-walk machinery.

- **D-02 — Tag grammar (conservative boundary; corpus is the authority).**
  - **Start boundary:** a `#` only begins a tag when the **preceding char is start-of-input
    or whitespace**. This single rule makes `foo#bar` (preceded by a letter) and `/#frag`
    (preceded by `/`) NOT tags — both required-negative.
  - **Body:** `[A-Za-z0-9_/-]+`, **must contain ≥1 non-digit** (so `#1984` → not a tag);
    `/` permitted *between* segments for nesting (`#a/b` → tag) but a tag is not just digits.
  - **Escape:** `\#` must not badge — rely on `@lezer/markdown`'s built-in `Escape` inline
    parser consuming `\#` (precedence ahead of `Hashtag`), and additionally guard that the
    char before `#` is not `\`.
  - **`#`-hex colors ARE tags:** `#abc` / `#fff` badge (locked — do NOT special-case hex;
    indistinguishable from short tags, Obsidian renders them as tags).
  - Match is case-insensitive for identity (a 22 concern); Phase 21 **displays original case**.

- **D-03 — Code / heading exclusion is structural, but still corpus-pinned.** `@lezer/markdown`
  does not run inline parsers inside `InlineCode` or `FencedCode`, and an ATX heading's leading
  `#` is a block `HeaderMark` (not inline) — so `# Title`, `` `#x` ``, and fenced `#x` are
  excluded by construction. We still pin every one in the corpus (defense against a grammar/
  precedence regression).

- **D-04 — Render: replace-widget with active-line reveal (mirrors `ImageWidget`).** Add a
  `Hashtag` case to `buildDecorations` emitting `Decoration.replace({ widget: new
  HashtagWidget(tag) })`, **suppressed on active lines** via the existing `isActive(from)`
  reveal (so the raw `#tag` shows on the line being edited and in **source mode**, which has no
  `livePreviewExtension`). In **read mode** `cleanRender` keeps `activeLines` empty, so the
  badge always shows. This is a single inline (single-line) widget emitted from the ViewPlugin
  — like `ImageWidget`, **not** a block StateField. **Not** added to `atomicRanges** (a hashtag
  is short, inline text — no atomic-delete footgun; keep cursor free, Pitfall 1).

- **D-05 — `HashtagWidget` (mirror `BulletWidget`/`ImageWidget`).** In `widgets.ts`:
  `document.createElement("span")`, `className = "cm-hashtag"`, **`textContent` = the rendered
  label** (`#deploy`, Obsidian-parity; never `innerHTML`). `eq(other)` compares the tag string.
  `ignoreEvent()` returns `false` so a click can place the cursor (triggering reveal). The DOM
  is `createElement`+`textContent` ONLY — untrusted tag bytes can never become live markup
  (the security invariant criterion 3 locks; tests assert on `tagName`/`textContent`, never
  HTML strings).

- **D-06 — Styling.** Add `.cm-hashtag` to `theme.ts` (small rounded pill, dark-theme tokens,
  layout-neutral — must not change line-height so reveal stays non-reflowing, Pitfall 2).

- **D-07 — Byte-stability.** Decoration-only ⇒ zero document transactions ⇒ `doc.toString()`
  is verbatim; the Go `okf.Emit` golden round-trip (`TestGoldenRoundTrip`) is untouched by
  construction. The corpus additionally asserts the saved string still contains the literal
  `#deploy` after rendering.

- **D-08 — Open API item for the planner (verify, don't recall).** The exact current
  `@lezer/markdown` inline-parser surface — `MarkdownConfig.parseInline: [{ name, parse(cx,
  next, pos) }]`, `InlineContext.char`/`slice`, `cx.elt(name, from, to)`, `cx.addElement`, the
  `defineNodes` registration of `Hashtag`, and whether a `before: "Link"`/precedence hint is
  needed — **must be verified against the installed `@lezer/markdown` (`^1.6.4`) docs/source at
  plan time** (context7 / `node_modules`), not from memory. This is the phase's one
  external-knowledge gap.
</decisions>

<canonical_refs>
- `web/src/lib/cm/markdown.ts` — `markdownExtension`; compose `hashtagExtension` into `extensions[]`.
- `web/src/lib/cm/livePreview.ts` — `buildDecorations` switch (add `Hashtag` case); `isActive`
  reveal helper; `ImageWidget` case is the pattern to mirror (inline single-line replace widget).
- `web/src/lib/cm/widgets.ts` — `BulletWidget`/`ImageWidget` (the `HashtagWidget` template).
- `web/src/lib/cm/theme.ts` — add `.cm-hashtag` badge tokens.
- `web/src/lib/cm/livePreview.test.ts` — `decorationsFor(doc, selectionAt?, {clean})` harness;
  the positive + negative `#tag` corpus lands here.
- `web/src/lib/cm/widgets.test.ts` — `HashtagWidget` DOM unit test (tagName/textContent only).
- `web/src/components/LivePreviewEditor.tsx:156` — `cleanRender.of(true)` (read-mode wiring;
  PageView renders through this, so one `Hashtag` case serves both surfaces).
- `.planning/REQUIREMENTS.md` §ED-02a — the acceptance criteria + negative corpus list.
- `.planning/ROADMAP.md` §Phase 21 — success criteria.
</canonical_refs>
