// cmCorpus — the eight okf golden-corpus fixture strings, copied VERBATIM from
// internal/okf/testdata/corpus/*.md (byte-for-byte, including CRLF line endings
// in 07 and the absent trailing newline in 06). This is the shared byte-stability
// fixture set the EDIT-02 mode-toggle test and the EDIT-03 verbatim-bytes test
// iterate, so the frontend exercises exactly the same inputs as the backend
// TestGoldenRoundTrip gate.
//
// IMPORTANT: do not "tidy" these strings. The \r\n in CRLF and the missing final
// \n in no-trailing-newline are load-bearing — they prove the CM6 document never
// rewrites bytes on toggle/round-trip.

export interface CorpusFixture {
  name: string;
  text: string;
}

// 01-headings-lists.md
const f01 =
  "---\ntype: Page\ntitle: Headings and Lists\ndescription: A page exercising headings, ordered and nested lists.\ntags:\n  - docs\n  - sample\ntimestamp: 2026-06-18T10:00:00Z\n---\n\n# Top Heading\n\nSome intro text.\n\n## Second Level\n\n1. First item\n2. Second item\n   1. Nested ordered\n   2. Another nested\n3. Third item\n\n- Bullet one\n- Bullet two\n  - Nested bullet\n    - Deeper bullet\n\n### Third Level\n\nClosing paragraph.\n";

// 02-codeblock-with-fence.md
const f02 =
  "---\ntype: Page\ntitle: Code Block Containing a Fence\ndescription: The body has a fenced code block whose contents include a literal --- line and a key value line.\ntags: []\ntimestamp: 2026-06-18T10:05:00Z\n---\n\n# Tricky Body\n\nThe following code block contains lines that LOOK like frontmatter but are body:\n\n```yaml\n---\ntype: NotFrontmatter\ntitle: This is inside a code block\ndescription: parsers must not treat this as a fence\n---\n```\n\nAnd an inline `key: value` outside a block should also stay body text.\n\nkey: value\n\nDone.\n";

// 03-table-links-images.md
const f03 =
  "---\ntype: Page\ntitle: Tables, Links and Images\ndescription: GFM table, inline and reference links, an image, and a relative .md attachment-style link.\ntags:\n  - reference\ntimestamp: 2026-06-18T10:10:00Z\n---\n\n# Links and Tables\n\nA GFM table:\n\n| Name  | Role   |\n| ----- | ------ |\n| Alice | Editor |\n| Bob   | Reader |\n\nAn inline link to [the deploy runbook](../runbooks/deploy.md) and a\n[reference-style link][home].\n\nAn image: ![logo](./assets/logo.png)\n\n[home]: ./index.md\n";

// 04-quirky-frontmatter.md
const f04 =
  "---\n# A YAML comment that MUST survive the round-trip.\ntype: Page\ntitle: \"Quirky: a title with a colon and quotes\"\ndescription: 'single-quoted scalar with #hash inside'\ntags:\n  - \"quoted-tag\"\n  - plain\ntimestamp: 2026-06-18T10:15:00Z\ncustom_field: preserved\nnested:\n  a: 1\n  b: 2\n---\n\n# Body\n\nFrontmatter above has a comment, unusually-quoted scalars, and extra fields\nthat all must be preserved verbatim.\n";

// 05-no-frontmatter.md
const f05 = "# Plain Page\n\nThis file has no frontmatter at all.\n\n- just\n- markdown\n";

// 06-no-trailing-newline.md (deliberately omits the final newline)
const f06 =
  "---\ntype: Page\ntitle: No Trailing Newline\ndescription: this file deliberately omits the final newline\ntags: []\ntimestamp: 2026-06-18T10:20:00Z\n---\n\nBody with no trailing newline.";

// 07-crlf.md (windows line endings, preserved verbatim)
const f07 =
  "---\r\ntype: Page\r\ntitle: CRLF File\r\ndescription: windows line endings must be preserved verbatim\r\ntags: []\r\ntimestamp: 2026-06-18T10:25:00Z\r\n---\r\n\r\n# CRLF Body\r\n\r\nEvery line here ends with carriage-return + line-feed.\r\n";

// 08-empty-body.md
const f08 =
  "---\ntype: Page\ntitle: Empty Body\ndescription: closing fence is effectively the end\ntags: []\ntimestamp: 2026-06-18T10:30:00Z\n---\n";

export const cmCorpus: CorpusFixture[] = [
  { name: "01-headings-lists.md", text: f01 },
  { name: "02-codeblock-with-fence.md", text: f02 },
  { name: "03-table-links-images.md", text: f03 },
  { name: "04-quirky-frontmatter.md", text: f04 },
  { name: "05-no-frontmatter.md", text: f05 },
  { name: "06-no-trailing-newline.md", text: f06 },
  { name: "07-crlf.md", text: f07 },
  { name: "08-empty-body.md", text: f08 },
];
