// hashtag.fixtures — the SINGLE-SOURCED whole-document parity corpus (TR-02).
//
// These fixtures are the ONE source of truth for the Lezer↔Go `#tag`-boundary
// parity test. `hashtag.parity.test.ts` runs the REAL Lezer parser
// (`tagParser`/`inlineTagsFromBody`'s extension set) over each `body`, computes
// the Hashtag node spans as BYTE offsets, and regenerates
// `internal/okf/testdata/hashtag_corpus.json`. The Go test
// (internal/okf/hashtag_test.go) loads that JSON and asserts its FindHashtags
// scanner produces the SAME byte spans. A Lezer-corpus addition the Go scanner
// would miss therefore fails CI (the offsets diverge), which is the anti-drift
// contract the Phase-21 "second #tag parser" retirement demands.
//
// Fixtures MUST cover: the full hashtag.test.ts positive/negative corpus PLUS
// block contexts (indented code, HTML block, GFM table, lazy blockquote
// continuation, `>#tag` block-marker abut) — the cases a paragraph-only corpus
// would miss.

export interface HashtagFixture {
  name: string;
  body: string;
}

export const hashtagFixtures: HashtagFixture[] = [
  // ── Positive corpus (ED-02a) ───────────────────────────────────────────────
  { name: "after-whitespace", body: "see #deploy" },
  { name: "start-of-string", body: "#deploy here" },
  { name: "nested-slash", body: "#a/b" },
  { name: "multiple-dedupe-case", body: "#Deploy and #deploy" },
  { name: "hex-like", body: "#abc #fff" },
  { name: "underscore-dash-body", body: "#a_b-c" },

  // ── Negative corpus (ED-02a) ───────────────────────────────────────────────
  { name: "all-digit", body: "#1984" },
  { name: "mid-word", body: "foo#bar" },
  { name: "after-slash-frag", body: "a/#frag" },
  { name: "escaped-hash", body: "\\#esc" },
  { name: "bare-hash", body: "#" },
  { name: "atx-heading-shape", body: "# Heading" },
  { name: "digits-and-slashes", body: "#12/34" },

  // ── Body / slash edge cases (Phase 21) ─────────────────────────────────────
  { name: "leading-slash-body", body: "#/a" },
  { name: "single-dash", body: "#-1" },
  { name: "bare-underscore", body: "#_" },
  { name: "bare-dash", body: "#-" },
  { name: "consecutive-slashes", body: "#a//b" },
  { name: "single-trailing-slash", body: "#a/" },
  { name: "multi-trailing-slash", body: "#A/B/" },
  { name: "deep-nested", body: "#a/b/c" },
  { name: "all-slash", body: "#///" },
  { name: "trailing-punct-dot", body: "#tag." },
  { name: "trailing-punct-comma", body: "#tag," },
  { name: "trailing-punct-bang", body: "#tag!" },
  { name: "inner-dot", body: "#a.b" },
  { name: "hex-0xff", body: "#0xFF" },

  // ── Start-boundary edge cases (Phase 21) ───────────────────────────────────
  { name: "after-paren", body: "(#tag)" },
  { name: "after-comma", body: ",#tag" },
  { name: "after-dquote", body: '"#tag' },
  { name: "after-emdash", body: "a —#tag" },
  { name: "emdash-start", body: "—#tag" },
  { name: "triple-hash-word", body: "###tag" },
  { name: "after-list-dot", body: "1.#tag" },
  { name: "after-midline-tab", body: "a\t#deploy" },
  { name: "after-soft-break", body: "a\n#deploy" },

  // ── Unicode boundary + body (Phase 21) ─────────────────────────────────────
  { name: "non-ascii-body-terminates", body: "#café" },
  { name: "non-ascii-before-hash", body: "é#tag" },
  { name: "emoji-before-hash", body: "😀#tag" },
  { name: "emoji-space-then-tag", body: "😀 #deploy" },

  // ── Code-region aware (Phase 22) ───────────────────────────────────────────
  { name: "indented-code-tab", body: "\t#deploy" },
  { name: "fenced-block", body: "```\n#x\n```" },
  { name: "inline-code-span", body: "`x #code`" },
  { name: "after-inline-code", body: "`x`#tag" },

  // ── Block-context whole-document fixtures (TR-02) ───────────────────────────
  {
    name: "indented-code-after-blank",
    body: "para\n\n    #deploy\n\nmore #ops here",
  },
  {
    name: "html-block-div",
    body: "<div>\n#notag inside html\n</div>\n\n#realtag after",
  },
  {
    name: "gfm-table",
    body: "| a | b |\n| - | - |\n| #alpha | #beta |\n",
  },
  {
    name: "lazy-blockquote-continuation",
    body: "> quote #one\ncontinues #two\n",
  },
  { name: "blockquote-marker-abut", body: ">#deploy" },
  { name: "blockquote-space-then-tag", body: "> #deploy" },
  {
    name: "fenced-then-live",
    body: "```js\nconst x = '#nope';\n```\n\nafter #yes tag",
  },
  {
    name: "heading-with-inline-tag",
    body: "## Release #v2rc\n\nbody #ship",
  },
  {
    name: "multi-tag-paragraph",
    body: "tags: #ops and #devops and #ops again",
  },
  {
    name: "delete-context-line",
    body: "see #ops here\nfoo #ops.\n#ops\n",
  },
];
