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

// SECURITY_CORPUS maps a fixture id to Markdown embedding raw HTML. These are the
// MAJ-04-b HTML-inertness fixtures — asserted ONLY in codec.test.ts's security section
// (they must round-trip to entity-escaped literal text with no live DOM / no executable
// attribute in the parsed JSON). They are intentionally OUTSIDE the tree-equality/parity
// gate because HTML->escaped-text is a correct, expected divergence.
export const SECURITY_CORPUS: Record<string, string> = {
  imgOnerror: '<img src=x onerror="alert(1)">',
  scriptTag: "<script>alert(1)</script>",
  divOnclick: '<div onclick="steal()">click me</div>',
  inlineEm: "<em>emphasis via raw html</em>",
};
