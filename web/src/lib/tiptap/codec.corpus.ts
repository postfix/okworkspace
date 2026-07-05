// codec.corpus — the Tier-1 round-trip corpus (RE-01 crit-2, D-02).
//
// One entry per bounded-slice construct, each a REAL Markdown string. This is the
// acceptance contract for the whole v1.7 milestone: codec.test.ts asserts, for every
// fixture, tree-equality (parse(roundTrip(src)) deep-equals parse(src)) + idempotence
// (roundTrip(roundTrip(src)) === roundTrip(src)); parity.test.ts re-parses the
// round-trip output through the CM6 @lezer/markdown GFM config and asserts the Lezer
// node-type structure matches the source's.
//
// The corpus MUST cover every construct in the bounded slice (D-02): basic marks,
// headings 1-6, bullet/ordered/NESTED lists, task lists, blockquote + blockquote-mixed,
// fenced code incl. a ```mermaid fence kept literal, link, hard break, CJK+emoji, and a
// kitchen-sink combining several. Nested lists and blockquote-mixed are the classic
// MarkedJS/ProseMirror serialization pain points (MIN-09) — they are deliberately here.
//
// HTML security fixtures are NOT in this corpus — embedded raw HTML is a CORRECT
// divergence (source parses HTML, round-trip yields inert escaped text), so it belongs
// only to the codec.test.ts security section, never to the tree-equality/parity gate.

// TIER1_CORPUS maps a stable fixture id to its Markdown source. Ordered roughly by
// construct complexity; ids are used verbatim in test titles and the GO/NO-GO report.
export const TIER1_CORPUS: Record<string, string> = {
  // Inline marks: bold, italic, strike (GFM), inline code.
  boldItalicStrikeCode: "Text with **bold**, *italic*, ~~strike~~, and `code`.",

  // Headings, all six ATX levels.
  headings: "# H1\n\n## H2\n\n### H3\n\n#### H4\n\n##### H5\n\n###### H6",

  // Flat bullet list.
  bulletList: "- one\n- two\n- three",

  // Flat ordered list.
  orderedList: "1. first\n2. second\n3. third",

  // Nested bullet + ordered, >=2 depth (the MIN-09 nested-list pain point).
  nestedMixed: "- top\n  - sub bullet\n  1. sub ordered\n    2. deeper",

  // Task list: unchecked + checked (- [ ] / - [x]).
  taskList: "- [ ] unchecked\n- [x] checked",

  // Blockquote with two paragraphs.
  blockquote: "> a quote\n>\n> second paragraph",

  // Blockquote containing a list AND a heading (the MIN-09 blockquote-mixed pain point).
  blockquoteMixed:
    "> quote intro\n>\n> - item one\n> - item two\n>\n> ## heading in quote",

  // Fenced code with a language tag (must stay literal, not rendered).
  fencedCode: "```js\nconst x = 1;\n```",

  // Fenced code with a ```mermaid tag — kept as LITERAL fenced code, NOT rendered
  // (the read view renders mermaid; the codec must round-trip the source bytes).
  mermaidFence: "```mermaid\ngraph TD; A-->B;\n```",

  // Inline link.
  link: "See [example](https://example.com) here.",

  // Hard break (two trailing spaces before the newline).
  hardBreak: "line one  \nline two",

  // CJK + accented + emoji text (multibyte / astral round-trip).
  cjkEmoji: "你好世界 hello 🎉 café",

  // Kitchen sink: heading + marks + task/nested list + blockquote-with-strike +
  // mermaid fence + link, all in one document.
  kitchenSink:
    "# Title\n\nSome **bold** and *italic* text with `code`.\n\n- [ ] task\n- item\n  - nested\n\n> quote with ~~strike~~\n\n```mermaid\ngraph TD; A-->B;\n```\n\n[link](https://example.com)",
};

// TIER2_CORPUS — the Tier-2 VERBATIM-PRESERVATION fixtures (RE-02, D-05). Each is a REAL
// Markdown construct the Tier-1 bounded slice does NOT model — a GFM table, `$$…$$` block
// math, `$…$` inline math, block raw HTML, and inline raw HTML. codec.test.ts asserts, per
// fixture, BYTE-FOR-BYTE round-trip modulo outer trim (`roundTrip(src).trim() === src.trim()`
// — STRONGER than Tier-1's tree-equality, because verbatim preservation is exact by
// contract) plus idempotence. These are captured, NOT modeled: the codec re-emits the exact
// source bytes via a PreservedBlock/PreservedInline/BlockMath `raw` attribute.
export const TIER2_CORPUS: Record<string, string> = {
  // GFM table: header + alignment row + two body rows (a multi-line native `table` token).
  table: "| a | b |\n| --- | --- |\n| 1 | 2 |\n| 3 | 4 |",

  // GFM table with per-column alignment markers (`:--`, `:-:`, `--:`) — must survive exact.
  tableAligned: "| L | C | R |\n| :-- | :-: | --: |\n| 1 | 2 | 3 |",

  // `$$…$$` block math (single line inside the delimiters).
  blockMath: "$$\nE = mc^2\n$$",

  // `$$…$$` block math spanning multiple lines with LaTeX backslashes (byte-fragile).
  blockMathMultiline:
    "$$\n\\begin{aligned}\na &= b \\\\\nc &= d\n\\end{aligned}\n$$",

  // `$…$` inline math inside a paragraph (the delimiters + content preserved verbatim).
  inlineMath: "Euler's identity is $e^{i\\pi}+1=0$ exactly.",

  // Two inline-math spans in one paragraph (proves the inline tokenizer re-locates each).
  inlineMathTwice: "Both $a^2$ and $b^2$ appear here.",

  // Block raw HTML: a multi-line `<div>` with nested markup — the merged block `html`
  // token, preserved byte-for-byte as INERT literal text (never a live DOM).
  blockHtml: '<div class="x">\n  <p>hi</p>\n</div>',

  // Inline raw HTML inside a paragraph — the research-flagged fragile construct, routed
  // through the custom inline tokenizer (name != 'html') for byte parity.
  inlineHtml: "text with <span class=\"hl\">inline raw</span> here",
};

// MIXED_CORPUS — documents combining Tier-1 (paragraph, heading, marks, fenced code,
// blockquote) WITH Tier-2 (table, inline math, block math, inline HTML) in one doc, to
// prove the preserved atom nodes COMPOSE with the Phase-38 bounded slice: round-trip is
// byte-for-byte modulo outer trim + idempotent even when tiers interleave.
export const MIXED_CORPUS: Record<string, string> = {
  // Heading + bold + inline math, then a table, then block math, then a fenced code
  // block, then a blockquote — one construct of each tier, interleaved.
  headingTableMathCode:
    "# Title\n\nSome **bold** text with inline math $x^2$.\n\n| a | b |\n| --- | --- |\n| 1 | 2 |\n\n$$\n\\int_0^1 x\\,dx\n$$\n\n```js\nconst x = 1;\n```\n\n> quote",

  // A paragraph carrying BOTH inline math and inline HTML, followed by a bullet list
  // (Tier-1) and a block-math block (Tier-2).
  inlineMixedThenList:
    "Prose with $\\alpha$ and <em>raw</em> together.\n\n- one\n- two\n\n$$\n\\sum_i x_i\n$$",

  // Block HTML sandwiched between two Tier-1 paragraphs.
  blockHtmlBetweenParas:
    'first paragraph\n\n<div class="note">\n  <p>kept verbatim</p>\n</div>\n\nsecond paragraph',
};

// SECURITY_CORPUS maps a fixture id to Markdown embedding raw HTML. These are the MAJ-04
// HTML-inertness fixtures — asserted in codec.test.ts's security section. Phase 39 SHARPENS
// the contract vs Phase 38: raw HTML now round-trips to its EXACT SOURCE BYTES (via the
// PreservedBlock/PreservedInline `raw` attr) AND stays INERT — the parsed JSON has no
// element node (no `image`/`div`/`script`) and no executable attribute (`onerror`/`onclick`/
// …), because the HTML is held as a literal STRING and re-emitted verbatim; `generateJSON`/
// DOM is never invoked (node env). They remain OUTSIDE the Tier-1 tree-equality/parity gate
// (raw HTML is a Tier-2 preserved construct, not a bounded-slice one).
export const SECURITY_CORPUS: Record<string, string> = {
  imgOnerror: '<img src=x onerror="alert(1)">',
  scriptTag: "<script>alert(1)</script>",
  divOnclick: '<div onclick="steal()">click me</div>',
  inlineEm: "<em>emphasis via raw html</em>",
};
