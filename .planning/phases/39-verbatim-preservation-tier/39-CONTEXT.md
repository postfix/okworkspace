---
author: architect
responsible: architect
phase: 39
phase_type: implementation
status: Ready for planning
hard_bar: true
security_relevant: true
design_fork: false
parent_artifacts:
  - .planning/ROADMAP.md
  - .planning/REQUIREMENTS.md
  - .planning/phases/38-markdown-codec-roundtrip-corpus/38-SUMMARY.md
---

# Phase 39: Verbatim Preservation Tier - Context

**Gathered:** 2026-07-02
**Status:** Ready for planning

<domain>
## Phase Boundary

Extend the Phase-38 codec with a **Tier-2 verbatim preservation** mechanism so unmodeled Markdown
constructs — GFM **tables**, `$…$`/`$$…$$` **math**, and raw **HTML** (block + inline) — round-trip
**byte-for-byte** (modulo outer trim) through an edit session, via `PreservedBlock`/`PreservedInline`
atom nodes that capture `token.raw` and re-emit it verbatim. Raw HTML stays **inert** (never a live DOM).

**Requirements:** RE-02 · **SECURITY: MAJ-04** (raw-HTML `toDOM` / preservation is the untrusted surface)

**Grounded mechanism (research-verified against `@tiptap/markdown` 3.27.1 source):**
- Every marked token carries verbatim source in `token.raw`.
- An extension's `markdownTokenName` + `parseMarkdown(token)` intercepts a native token (`table`,
  `html`) BEFORE the built-in fallback — capture `token.raw` into an attr.
- A custom NON-text node's `renderMarkdown(node)` return is emitted **verbatim** (it never passes
  through the escaper) — return the stored raw attr → byte-exact.
- `markdownTokenizer` registers the `$`/`$$` math tokens marked lacks (custom marked extension; runs
  before built-ins).

**Non-goals (out of this phase):**
- Editable/modeled tables or math (structured WYSIWYG editing) — explicitly deferred to Future
  (v1.7 preserves them verbatim only).
- Any editor UI / mounting / toggle — Phases 40-42.
- The Tier-1 supported-slice codec — that shipped in Phase 38 (this phase EXTENDS it).

**Parallelization:** touches ONLY `web/src/lib/tiptap/*` (extends the codec + adds preserved nodes +
corpus). Disjoint from Phase 40 (`PageEditor`/`editorMode`). MAY run in parallel with Phase 40.
</domain>

<decisions>
## Implementation Decisions

### D-01: PreservedBlock — block atom nodes capturing `token.raw` (table, block-HTML)
- A `PreservedBlock` TipTap atom node (`group:"block"`, `atom:true`, `code:true`,
  `addAttributes` → `raw`) whose `parseMarkdown(token)` stores `token.raw` and `renderMarkdown(node)`
  returns `node.attrs.raw` verbatim.
- **`markdownTokenName` is 1:1** (research): one registration per marked token type. Register the
  preserving handler for **`table`** and for block **`html`**. They may share ONE node type
  (`preservedBlock`) for render, but each needs its own parse-side `markdownTokenName` registration
  (two thin sibling extensions, or the node + a companion). **Capture, don't model** the table — do
  NOT register a modeled Table node.

### D-02: PreservedInline — inline atom node for inline verbatim (inline math, inline HTML)
- A `PreservedInline` atom node (`group:"inline"`, `inline:true`, `atom:true`, `code:true`, `raw` attr),
  `renderMarkdown` returns raw verbatim (cleanest — bypasses the mark placeholder dance).
- **Inline HTML caveat (research):** marked's `parseInlineTokens` has a pre-emptive inline-`html`
  branch that merges `<tag>…</tag>` and calls the built-in `parseHTMLToken` BEFORE the generic handler
  dispatch. To reliably intercept inline HTML for byte-parity, route it through a **custom inline
  `markdownTokenizer` whose name ≠ `'html'`** (so it lands in the generic handler path), not by
  registering `markdownTokenName:'html'` for inline.

### D-03: Math tokenizers (marked has none)
- Register a custom **block** `markdownTokenizer` for `$$…$$` (`level:"block"`, explicit
  `start:(src)=>src.indexOf("$$")`, `tokenize` returns `{type, raw:m[0]}` with raw = the full
  `$$…$$` slice) → routes into a block preserved node.
- Register a custom **inline** `markdownTokenizer` for `$…$` (`level:"inline"`,
  `start:(src)=>src.indexOf("$")`, raw = full `$…$`) → routes into `PreservedInline`.
- **`.raw` MUST be the exact matched slice** (the manager's synthesized `start` does
  `src.indexOf(result.raw)` — a wrong raw mislocates the token and corrupts surrounding text).

### D-04: SECURITY (MAJ-04) — HTML byte-parity WITHOUT a DOM path
- **This phase CHANGES the HTML handling.** The Phase-38 default (`htmlAsLiteralText`) is inert but
  **lossy** (strips trailing whitespace, then the resulting text node is entity-escaped on
  serialize). For byte-for-byte, register an explicit `html`-token handler that stores `token.raw`
  into a `PreservedBlock`/`PreservedInline` attr and re-emits it verbatim.
- **The security property MUST hold:** the preserved HTML is stored as a STRING attr and rendered as
  **literal text** — `generateJSON`/DOM is NEVER invoked. An `<img onerror=…>`/`<script>` fixture
  round-trips to its exact source bytes as inert text, never a live element node with an executable
  attribute. The codec test stays under the `node` env (no `window.DOMParser`). This is the sharper
  MAJ-04 assertion than Phase 38 (which merely escaped) — now we preserve exact bytes AND stay inert.
- marked MERGES consecutive block/inline HTML tokens — capture the **merged** `.raw` (one preserved
  node), not a fragment.

### D-05: The Tier-2 gate (corpus)
- **Byte-for-byte (modulo outer trim) + idempotence** per Tier-2 fixture: tables, `$$` block math,
  `$` inline math, block HTML, inline HTML — `roundTrip(src)` byte-equals `src` (trimmed);
  `roundTrip(roundTrip(src))` stable.
- **Mixed fixtures:** Tier-1 + Tier-2 in one document (e.g. a paragraph, a table, inline math, a
  fenced code block) still round-trips — the preserved nodes compose with the Phase-38 slice.
- **Security fixtures:** the raw-HTML inertness assertions (MAJ-04) — exact bytes preserved AND no
  element node / executable attr in the parsed JSON.
- **Do NOT regress Tier-1:** the Phase-38 corpus + parity tests still pass unchanged (the new
  extensions are additive; a `$` in prose that isn't math, a stray `<` — must not break Tier-1 fixtures).

### The agent's discretion
- One shared `preservedBlock`/`preservedInline` node with sibling parse registrations vs distinct
  nodes per construct (research: shared render OK if same node name; separate parse per token name).
- The exact math regexes (must set `.raw` precisely; guard against `$` in code/escaped `$`).
- Whether inline HTML uses the custom-tokenizer route (D-02) or is scoped as best-effort (if inline
  HTML byte-parity proves fragile per research caveat, state it — a documented partial is honest;
  block HTML + tables + math are the priority).
- Commit slicing.
</decisions>

<disciplines>
## Lenses surfaced for this phase

~~~yaml
disciplines:
  - section: "D-04 raw HTML inert"
    name: "preserve-bytes-never-a-dom-path"
    scope: ["the preserved HTML handler", "codec + preserved nodes", "the node-env codec test"]
    guidance: "Raw HTML must round-trip byte-for-byte AS INERT TEXT. Store token.raw in a string attr;
      renderMarkdown returns it verbatim. NEVER call generateJSON / construct DOM from page HTML. The
      ProseMirror no-innerHTML-of-page-bytes invariant does not auto-apply — enforce it explicitly.
      The codec test stays node-env; an <img onerror>/<script> fixture stays exact-source inert text
      with no element node / executable attr in the parsed JSON. This is MAJ-04, flagged for the
      milestone security review."
    rationale: "TipTap paste/serialization is the milestone's sharp untrusted surface. Byte-for-byte
      preservation must not become a byte-for-byte EXECUTION path — the whole point is inert bytes."
    override_signals:
      - "None. HTML is preserved as inert literal text; a DOM path is never acceptable for page bytes."
  - section: "D-05 byte-for-byte gate"
    name: "verbatim-means-verbatim"
    scope: ["the Tier-2 corpus", "PreservedBlock/PreservedInline"]
    guidance: "Tier-2's contract is byte-for-byte (modulo outer trim). If a construct can't preserve
      exact bytes after honest effort (e.g. inline HTML per the research caveat), do NOT loosen the
      assertion to 'close enough' — either fix it via the custom-tokenizer route, or document the
      exact limitation (which constructs are byte-exact vs best-effort) honestly, as Phase 38 did for
      its NO-GO discipline. A silently-loosened byte check is the failure mode."
    rationale: "The two-tier fidelity contract is the milestone's acceptance gate. Tier-2's whole
      value is exactness for constructs TipTap can't model."
    override_signals:
      - "A genuinely byte-fragile construct is documented as a stated limitation, not hidden behind a widened assertion."
~~~

</disciplines>

<canonical_refs>
## Canonical References

### Phase spec & requirements
- `.planning/ROADMAP.md` §"Phase 39: Verbatim Preservation Tier" — goal, 4 success criteria, MAJ-04.
- `.planning/REQUIREMENTS.md` §RE-02 (+ MAJ-04 raw-HTML `toDOM` folded).

### The codec to extend (Phase 38)
- `web/src/lib/tiptap/extensions.ts` — `buildExtensions()` (add the preserved nodes + tokenizers here).
- `web/src/lib/tiptap/codec.ts` — `parse`/`serialize`/`roundTrip` (unchanged API; new extensions flow through).
- `web/src/lib/tiptap/codec.corpus.ts` — extend with a `TIER2_CORPUS` + mixed fixtures.
- `web/src/lib/tiptap/codec.test.ts` — the node-env gate (add Tier-2 byte-equality + the sharper MAJ-04 HTML section).

### External (research-verified — `agent://PreservedTierResearch`)
- `@tiptap/markdown` 3.27.1 `MarkdownManager`: `markdownTokenName` (1:1 token→handler, runs before
  fallback), `parseMarkdown(token)` (`token.raw` verbatim), `renderMarkdown(node)` (verbatim emit),
  `markdownTokenizer` (custom marked extension for math), `code:true` (text/mark escaping bypass).
  Caveats: default HTML path lossy; inline-HTML pre-emptive merge branch; merged HTML raw; exact `.raw`.
- The research's minimal REAL code sketch (PreservedBlock/PreservedInline/BlockMath/inline math) uses
  only real 3.27.1 fields — the starting point for the node definitions.
</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable assets
- The Phase-38 codec (`MarkdownManager` + `buildExtensions` + `parse`/`serialize`/`roundTrip`) — extend.
- The node-env codec test harness + the tree-equality/idempotence helpers.
- The research code sketch — real 3.27.1 API for the preserved nodes + math tokenizers.

### Established patterns
- Tier-1 gate discipline (tree-equality/idempotence; node-env; honest NO-GO/limitation) — Tier-2
  mirrors it with byte-equality.
- Import-isolation (the codec stays out of the app entry until Phase 42).

### Landmines
- **Default HTML path is LOSSY** (strips trailing ws + entity-escapes) — MUST add an explicit
  `token.raw`-storing handler for byte-parity (D-04).
- **Inline HTML is the most fragile** — use a custom inline tokenizer (name ≠ `'html'`) to bypass
  marked's pre-emptive branch; if still fragile, document the limitation honestly.
- **`.raw` must be the exact slice** in custom tokenizers — a wrong raw corrupts surrounding text.
- **marked merges consecutive HTML** — capture the merged raw or one block splits into many nodes.
- **Outer-trim caveat** — "byte-for-byte" is per-construct-raw; whole-doc parity is modulo outer
  trim (the serializer controls blank-line joins between blocks). The corpus asserts modulo outer trim.
- **Don't regress Tier-1** — the new extensions are additive; the Phase-38 corpus + parity must stay green.
- **Security (MAJ-04)** — preserved HTML is inert literal text, never a DOM path; node-env test.
</code_context>

<deferred>
## Deferred Ideas

- **Modeled/editable tables + math** (structured WYSIWYG) — Future (v1.7 preserves verbatim only).
- **Inline-HTML byte-parity** if it proves fragile — document as a stated limitation (block HTML +
  tables + math are the priority); revisit if needed.
</deferred>

---

*Phase: 39-verbatim-preservation-tier*
*Context gathered: 2026-07-02*
