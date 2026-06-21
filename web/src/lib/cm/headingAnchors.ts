// headingAnchors — the github-slugger algorithm, ported BYTE-FOR-BYTE from the
// backend's internal/okf/headings.go (`slug` + `dedupSlug`). The unified read
// surface (06-04) stamps each heading line with a DOM id via
// Decoration.line({attributes:{id: slug}}) so search-result `#anchor` deep-links
// (SRCH-06 / T-03-15) land on the right heading. Because BOTH the backend
// (okf.ScanHeadings, which feeds the search index anchors) and the frontend read
// view compute ids here, the deep-link target and the rendered id are guaranteed
// to agree.
//
// The algorithm mirrors github-slugger's non-unique `slug()`:
//   - lowercase
//   - drop every rune that is not a Unicode letter / number / '-' / '_' / space
//   - replace each space with '-'
//   - NO whitespace collapsing, NO hyphen trimming  (github-slugger does neither)
// and dedups within a document as base, base-1, base-2, …
//
// Go uses unicode.IsLetter / unicode.IsNumber; the JS port uses the equivalent
// Unicode property escapes \p{L} (letter) and \p{N} (number) with the `u` flag so
// the same runes survive. Pure + unit-tested against the okf corpus headings.

// LETTER_OR_NUMBER matches a single Unicode letter or number code point. Mirrors
// Go's `unicode.IsLetter(r) || unicode.IsNumber(r)`.
const LETTER_OR_NUMBER = /[\p{L}\p{N}]/u;

// slug computes a GitHub-style anchor slug for `text` (without a leading '#').
export function slug(text: string): string {
  const lower = text.toLowerCase();
  let out = "";
  // Iterate by code point (for…of over a string yields code points, matching
  // Go's `range` over runes) so astral characters are classified correctly.
  for (const ch of lower) {
    if (ch === " ") {
      out += "-";
    } else if (ch === "-" || ch === "_") {
      out += ch;
    } else if (LETTER_OR_NUMBER.test(ch)) {
      out += ch;
    }
    // else: dropped (punctuation, symbols, control, etc.)
  }
  return out;
}

// dedupSlug returns `base` unchanged the first time it is seen and base+"-N"
// (N=1,2,…) on repeats, tracking occurrences exactly as github-slugger's
// BananaSlug.slug (and okf.dedupSlug) do, so the Nth duplicate heading gets the
// same suffix the backend assigned. `occurrences` is a mutable map the caller
// threads across the headings of one document.
export function dedupSlug(occurrences: Map<string, number>, base: string): string {
  let result = base;
  if (occurrences.has(result)) {
    for (;;) {
      occurrences.set(base, (occurrences.get(base) ?? 0) + 1);
      result = base + "-" + String(occurrences.get(base));
      if (!occurrences.has(result)) {
        break;
      }
    }
  }
  occurrences.set(result, 0);
  return result;
}

// headingText strips the leading ATX '#' run and the single required space from a
// heading line, returning the heading text 06-04 feeds to slug() when stamping the
// line id. A line that is not an ATX heading is returned unchanged. (Trailing-'#'
// closer handling lives in okf.atxHeading on the backend; the read surface only
// needs the leading-marker strip for id computation.)
export function headingText(line: string): string {
  const m = /^ {0,3}(#{1,6})(?:[ \t]+(.*))?$/.exec(line);
  if (!m) {
    return line;
  }
  return (m[2] ?? "").replace(/[ \t]+$/, "");
}
