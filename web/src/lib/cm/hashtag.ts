// hashtag — the ED-02a/ED-02b inline `#tag` grammar, as a @lezer/markdown extension,
// plus a standalone Lezer parser that materializes body `#tags` into frontmatter.
//
// TWO consumers, ONE grammar AND ONE parser (CONTEXT D-01/D-01b):
//   • `hashtagExtension` — a MarkdownConfig that registers a `Hashtag` inline node so
//     the Lezer tree the livePreview ViewPlugin already walks gains `Hashtag` nodes;
//     livePreview.ts replaces each with a badge widget (decoration-only, byte-stable).
//   • `inlineTagsFromBody` — runs the SAME extension set (`tagParserExtensions`) over a
//     raw body string via a standalone `parser.configure(...)`, collecting `Hashtag`
//     node text (lower-cased, de-duped). Phase 22 (ED-02b) uses it to reconcile body
//     `#tags` into the canonical frontmatter `tags:` on save.
//
// Because BOTH consumers run the IDENTICAL Lezer extension set, materialization is
// code-region/heading/escape-aware BY CONSTRUCTION — it can never disagree with what
// the editor badges. This CLOSES the Phase-21 `parseInlineTags` "render⇄scanner
// divergence" (21-VERIFICATION.md): a `#` inside fenced/inline code, an ATX heading
// marker, an all-digit run, or an escape is excluded the same way in both, and a `#`
// directly after a block marker (`>#tag`) is a valid boundary in both. The Phase-21
// pure string scanner `parseInlineTags` — which had no block structure and so diverged
// on code regions and block-marker boundaries — is RETIRED, superseded by this parse.
//
// GRAMMAR (CONTEXT D-02 — the negative corpus is the authority):
//   • Start boundary: a `#` only begins a tag when the preceding char is start-of-
//     input OR whitespace. This single rule excludes `foo#bar` (after a letter),
//     `/#frag` (after a slash), and `\#esc` (after a backslash) — all required-negative.
//   • Body: [A-Za-z0-9_/-]+, MUST contain ≥1 non-digit (so `#1984` and `#12/34` are
//     not tags); `/` separates nested segments (`#a/b`) but a tag is never just
//     digits+slashes. A trailing `/` is trimmed (it is a separator, not a segment).
//   • `#`-hex colors ARE tags (`#abc`/`#fff`) — NOT special-cased (locked decision;
//     indistinguishable from short tags, and Obsidian renders them as tags).
//   • Code/heading exclusion is STRUCTURAL, not coded here: @lezer/markdown never runs
//     inline parsers inside InlineCode/FencedCode, and an ATX heading's leading `#` is
//     a block HeaderMark (not inline content) — so `` `#x` ``, fenced `#x`, and
//     `# Title` are excluded by construction. Every one is still corpus-pinned in
//     livePreview.test.ts (defense against a grammar/precedence regression).
import { parser, Table, Strikethrough, TaskList, Autolink } from "@lezer/markdown";
import type { MarkdownConfig, InlineContext } from "@lezer/markdown";

const HASH = 35; // '#'
const SLASH = 47; // '/'

// Whitespace that counts as a valid start boundary (space, tab, the unicode line
// terminators the inline section can contain, form feed).
const isWs = (c: number): boolean =>
  c === 32 || c === 9 || c === 10 || c === 13 || c === 12;

// A body char: [A-Za-z0-9_/-]. Digits ARE body (so `#fff`/`#a1` work) but a tag must
// carry ≥1 non-digit (isNonDigit below) — that is what rejects an all-numeric run.
const isBody = (c: number): boolean =>
  (c >= 48 && c <= 57) || // 0-9
  (c >= 65 && c <= 90) || // A-Z
  (c >= 97 && c <= 122) || // a-z
  c === 95 || // _
  c === 45 || // -
  c === SLASH; // /

// A non-digit body char: [A-Za-z_-]. (Slash is a separator, NOT a "non-digit" that
// would qualify a digits-and-slashes run like `#12/34` as a tag.)
const isNonDigit = (c: number): boolean =>
  (c >= 65 && c <= 90) || (c >= 97 && c <= 122) || c === 95 || c === 45;

// scanBody walks the body of a candidate tag starting at `start` (the char after the
// `#`), reading code units via `charAt(i)` over the half-open range [start, end).
// Returns the exclusive end of the tag body, or -1 when the candidate is not a tag
// (just `#`, or no non-digit). A trailing `/` is trimmed before the checks. Shared by
// both the Lezer parser and the pure string scanner so the BODY ALPHABET (char
// classes + trailing-slash trim + the ≥1-non-digit rule) can never drift between
// them. (The START BOUNDARY and code-region exclusion are NOT in scanBody — see the
// file header for where the two consumers legitimately differ.)
function scanBody(
  charAt: (i: number) => number,
  start: number,
  limit: number,
): number {
  let end = start;
  let hasNonDigit = false;
  while (end < limit) {
    const c = charAt(end);
    if (!isBody(c)) break;
    if (isNonDigit(c)) hasNonDigit = true;
    end++;
  }
  // A trailing '/' is a dangling separator, not a segment — trim it off the tag.
  while (end > start && charAt(end - 1) === SLASH) end--;
  // Reject a bare `#` (nothing after it) or an all-digit/all-separator run.
  if (end <= start || !hasNonDigit) return -1;
  return end;
}

// parseHashtag is the @lezer/markdown InlineParser.parse function: called for every
// character of inline content, it returns -1 to decline or the end position of the
// `Hashtag` element it added. (Signature per the installed v1.6.4 d.ts.)
function parseHashtag(cx: InlineContext, next: number, pos: number): number {
  if (next !== HASH) return -1;
  // Start boundary: section start (pos === cx.offset) OR a whitespace char before the
  // `#`. cx.offset is the document position where this inline section begins, so
  // pos === cx.offset means "first char of the section" — a valid boundary. Reading
  // char(pos-1) only when pos > cx.offset keeps us inside the section bounds.
  const before = pos > cx.offset ? cx.char(pos - 1) : -1;
  if (before !== -1 && !isWs(before)) return -1; // mid-word / after '/' / after '\'
  const end = scanBody((i) => cx.char(i), pos + 1, cx.end);
  if (end < 0) return -1;
  return cx.addElement(cx.elt("Hashtag", pos, end));
}

// hashtagExtension registers the `Hashtag` inline node + parser. `before: "Link"`
// places it among the default inline parsers AFTER Escape/InlineCode (so `\#esc` is
// consumed as an Escape and `` `#x` `` stays inside InlineCode — never reaching us)
// and BEFORE the link parser. markdown.ts composes this into markdownExtension.
export const hashtagExtension: MarkdownConfig = {
  defineNodes: ["Hashtag"],
  parseInline: [{ name: "Hashtag", parse: parseHashtag, before: "Link" }],
};

// tagParserExtensions is the SINGLE source of truth for the Markdown extension set:
// the four GFM extensions plus the Hashtag inline node. BOTH the CM6 render
// (markdown.ts composes it into `markdownExtension`) and `inlineTagsFromBody` consume
// THIS exact array, so the render and the materialized tag set can never fork (D-01b).
// hashtag.ts imports the GFM bits straight from "@lezer/markdown" (never from
// markdown.ts) so there is no import cycle.
export const tagParserExtensions = [
  Table,
  Strikethrough,
  TaskList,
  Autolink,
  hashtagExtension,
];

// tagParser is a standalone CommonMark parser configured with tagParserExtensions. It
// parses a whole body string WITH full block structure, so a `#` inside fenced or
// inline code, an ATX heading marker, or after a non-boundary char is excluded
// structurally — identical to the render path (which walks the same node set).
const tagParser = parser.configure(tagParserExtensions);

// inlineTagsFromBody parses a raw body and returns the de-duped, lower-cased `Hashtag`
// tag names ('#Deploy' → 'deploy'). It is the code-region-aware materialization the
// Phase-22 save reconcile uses; sharing `tagParser` with the render is what guarantees
// agreement (CONTEXT D-01). De-dupe is case-insensitive (lower-cased before the Set
// add); first-seen order is insignificant — the reconcile re-orders deterministically.
export function inlineTagsFromBody(body: string): string[] {
  const tree = tagParser.parse(body);
  const out = new Set<string>();
  const cur = tree.cursor();
  do {
    if (cur.name === "Hashtag") {
      out.add(body.slice(cur.from + 1, cur.to).toLowerCase());
    }
  } while (cur.next());
  return [...out];
}

// InlineTagSpan is one recognized Hashtag node's byte-offset span in a body: the
// UTF-8 byte offset of the leading `#` (from), the exclusive byte offset just
// past the tag body (to), and the lower-cased name (without the `#`). Offsets are
// BYTE offsets — converted from the Lezer parser's UTF-16 code-unit positions —
// so the Go scanner (which is byte-native) can be compared span-for-span.
export interface InlineTagSpan {
  from: number;
  to: number;
  name: string;
}

// inlineTagSpansFromBody runs the SAME `tagParser` as inlineTagsFromBody and
// returns each Hashtag node as a BYTE-offset span, in source order (NOT de-duped
// — every occurrence is a byte the Go rewriter must splice). It is the parity
// test's source of truth: the byte spans the Go FindHashtags scanner must match.
export function inlineTagSpansFromBody(body: string): InlineTagSpan[] {
  const tree = tagParser.parse(body);
  const out: InlineTagSpan[] = [];
  const cur = tree.cursor();
  // Precompute a UTF-16-index → UTF-8-byte-offset map so a Lezer position (a
  // UTF-16 code-unit index into the JS string) becomes a byte offset.
  const byteAt = utf16ToByteOffsets(body);
  do {
    if (cur.name === "Hashtag") {
      out.push({
        from: byteAt[cur.from],
        to: byteAt[cur.to],
        name: body.slice(cur.from + 1, cur.to).toLowerCase(),
      });
    }
  } while (cur.next());
  out.sort((a, b) => a.from - b.from);
  return out;
}

// utf16ToByteOffsets returns an array `m` of length body.length+1 where m[i] is
// the UTF-8 byte offset of the character at UTF-16 index i. Handles surrogate
// pairs (a 2-unit astral char maps its trailing unit to the same byte start as
// the whole char is 4 bytes; the map is still monotone and correct at code-point
// boundaries, which is where Hashtag nodes always begin/end).
function utf16ToByteOffsets(body: string): number[] {
  const m = new Array<number>(body.length + 1);
  const enc = new TextEncoder();
  let byte = 0;
  let i = 0;
  while (i < body.length) {
    m[i] = byte;
    const cp = body.codePointAt(i)!;
    const units = cp > 0xffff ? 2 : 1;
    byte += enc.encode(String.fromCodePoint(cp)).length;
    if (units === 2) m[i + 1] = byte; // trailing surrogate maps to end of char
    i += units;
  }
  m[body.length] = byte;
  return m;
}
