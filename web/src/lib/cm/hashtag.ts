// hashtag — the ED-02a inline `#tag` grammar, as a @lezer/markdown extension plus a
// pure string scanner that shares the exact same rules.
//
// TWO consumers, ONE grammar (CONTEXT D-01/D-02):
//   • `hashtagExtension` — a MarkdownConfig that registers a `Hashtag` inline node so
//     the Lezer tree the livePreview ViewPlugin already walks gains `Hashtag` nodes;
//     livePreview.ts replaces each with a badge widget (decoration-only, byte-stable).
//   • `parseInlineTags` — a pure (no-CM) scanner over a raw string, returning the
//     de-duped, lower-cased tag names. Phase 22 reuses it to materialize body `#tags`
//     into frontmatter; defining it HERE, beside the parser, guarantees the rendered
//     badges and the materialized set can never diverge.
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
// both the Lezer parser and the pure string scanner so they cannot drift.
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

// parseInlineTags scans a raw string with the SAME boundary/body rules as the Lezer
// parser and returns the de-duped, lower-cased tag names ('#Deploy' → 'deploy').
// Reused by Phase 22 for client-side frontmatter materialization.
//
// SCOPE NOTE (Phase 21): this string scan does NOT skip fenced/inline-code regions
// (it has no block structure to consult) — acceptable because the function is UNUSED
// in the render path here (render excludes code structurally via the Lezer tree).
// Phase 22 (ED-02b) reconciles its scope when it wires materialization.
export function parseInlineTags(text: string): string[] {
  const out = new Set<string>();
  const charAt = (i: number) => text.charCodeAt(i);
  for (let i = 0; i < text.length; i++) {
    if (charAt(i) !== HASH) continue;
    const before = i > 0 ? charAt(i - 1) : -1;
    if (before !== -1 && !isWs(before)) continue;
    const end = scanBody(charAt, i + 1, text.length);
    if (end < 0) continue;
    out.add(text.slice(i + 1, end).toLowerCase());
    i = end - 1; // skip past the consumed tag body (the for-loop's i++ lands on `end`)
  }
  return [...out];
}
