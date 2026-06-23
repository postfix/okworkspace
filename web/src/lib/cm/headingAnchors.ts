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

// trimATXClosing removes a trailing run of '#' (the optional ATX closing sequence)
// and the whitespace that precedes it. Ported BYTE-FOR-BYTE from Go's
// okf.trimATXClosing (internal/okf/headings.go) so the frontend id matches the
// backend Anchor for headings that use a closing marker. CommonMark §4.2 requires
// the closing '#' run be preceded by whitespace to count as a closer; otherwise
// (e.g. "foo#") the '#' run is part of the text.
function trimATXClosing(s: string): string {
  const t = s.replace(/#+$/, ""); // strings.TrimRight(s, "#")
  if (t === s) {
    // No trailing '#' run — just trim trailing whitespace.
    return s.replace(/[ \t]+$/, ""); // strings.TrimRight(s, " \t")
  }
  // There was a trailing '#' run; it is a valid closer only if preceded by
  // whitespace. Trim the trailing whitespace before it.
  const trimmed = t.replace(/[ \t]+$/, ""); // strings.TrimRight(t, " \t")
  if (trimmed !== t) {
    // There was whitespace before the '#' run — valid closer, return trimmed.
    return trimmed;
  }
  // No whitespace before the '#' run (e.g. "foo#") — not a closer; keep the
  // original minus trailing whitespace.
  return s.replace(/[ \t]+$/, ""); // strings.TrimRight(s, " \t")
}

// headingText strips the leading ATX '#' run and the single required space from a
// heading line, then strips any trailing ATX closing '#' run (mirroring
// okf.atxHeading's trimATXClosing on the backend), returning the heading text
// 06-04 feeds to slug() when stamping the line id. A line that is not an ATX
// heading is returned unchanged.
//
// The backend trims " \t\r" off the right of the text BEFORE trimATXClosing, so a
// trailing '\r' (CRLF line) must be removed first here too — otherwise a heading
// like "## Title ##\r" would not match the closing-marker logic byte-for-byte.
export function headingText(line: string): string {
  const m = /^ {0,3}(#{1,6})(?:[ \t]+(.*))?$/.exec(line);
  if (!m) {
    return line;
  }
  // strings.TrimRight(rest, " \t\r") then TrimLeft(rest, " \t") on the backend.
  const rest = (m[2] ?? "").replace(/[ \t\r]+$/, "").replace(/^[ \t]+/, "");
  return trimATXClosing(rest);
}

// ---------------------------------------------------------------------------
// headingAnchors() — the CM6 extension that stamps each ATX-heading line with a
// DOM `id` equal to its github-slugger slug (deduped across the document exactly
// like okf.ScanHeadings: base, base-1, base-2 …). A StateField (NOT a ViewPlugin)
// computes the line decorations over the WHOLE document so a heading anchor exists
// even before its line scrolls into view — the scroll-to-#hash-on-mount lookup
// (scrollToHash below) needs every heading id present regardless of viewport.
//
// The id is rendered VERBATIM (never `user-content-`-prefixed) so it equals the
// backend anchor byte-for-byte (T-06-11): a search-result deep-link to
// /app/page/<path>#<heading-slug> targets exactly this line id.
// ---------------------------------------------------------------------------

import { Decoration, EditorView } from "@codemirror/view";
import type { DecorationSet } from "@codemirror/view";
import { StateField } from "@codemirror/state";
import type { Extension, Range } from "@codemirror/state";
import type { EditorState } from "@codemirror/state";
import { syntaxTree } from "@codemirror/language";

// ATXHeading1..6 → its level (1..6); anything else → 0. (Same shape the
// livePreview plugin uses; kept local so headingAnchors has no cross-import.)
function atxHeadingLevel(name: string): number {
  const m = /^ATXHeading([1-6])$/.exec(name);
  return m ? Number(m[1]) : 0;
}

// buildHeadingAnchors walks the syntax tree for ATX headings (in source order),
// computes each heading's deduped slug, and emits a Decoration.line carrying
// `id: <slug>` on the heading's start line. Source order + a single shared
// `occurrences` map make the dedup suffixes match okf.ScanHeadings exactly.
function buildHeadingAnchors(state: EditorState): DecorationSet {
  const occurrences = new Map<string, number>();
  const deco: Range<Decoration>[] = [];
  syntaxTree(state).iterate({
    enter: (node) => {
      const level = atxHeadingLevel(node.name);
      if (level === 0) {
        return;
      }
      const line = state.doc.lineAt(node.from);
      const id = dedupSlug(occurrences, slug(headingText(line.text)));
      deco.push(
        Decoration.line({ attributes: { id } }).range(line.from),
      );
      return false; // a heading has no nested headings to descend into
    },
  });
  return Decoration.set(deco, true);
}

// headingAnchorField holds the per-document heading-id line decorations,
// recomputed only when the document changes (ids are content-derived, not
// selection-derived — a cursor move never alters them).
const headingAnchorField = StateField.define<DecorationSet>({
  create: (state) => buildHeadingAnchors(state),
  update(value, tr) {
    if (tr.docChanged) {
      return buildHeadingAnchors(tr.state);
    }
    return value.map(tr.changes);
  },
  provide: (f) => EditorView.decorations.from(f),
});

// headingAnchors is the composable extension PageView's read-only surface adds so
// every heading line carries its github-slugger id.
export const headingAnchors: Extension = headingAnchorField;

// scrollToHash reads window.location.hash and, if a heading line carries a matching
// `id`, scrolls that heading into view. It is called once on mount (after the view
// is constructed + the field's decorations are applied). The hash is used ONLY as a
// lookup key against the rendered heading ids — it is never written into the DOM or
// used to build markup (T-06-12). Returns true if it scrolled to a heading.
//
// `hash` defaults to window.location.hash so callers can pass an explicit value in
// tests (jsdom has no real navigation). A leading '#' is stripped; the remainder is
// URL-decoded so a percent-encoded Unicode anchor (e.g. `#caf%C3%A9`) matches the
// raw slugged id (`café`).
export function scrollToHash(
  view: EditorView,
  hash: string = typeof window !== "undefined" ? window.location.hash : "",
): boolean {
  if (!hash || hash === "#") {
    return false;
  }
  let target = hash.startsWith("#") ? hash.slice(1) : hash;
  try {
    target = decodeURIComponent(target);
  } catch {
    // Malformed percent-encoding — fall back to the raw value (still only used as
    // a lookup key, never as markup).
  }
  if (target === "") {
    return false;
  }
  // The heading line ids live on the .cm-line elements the field decorated. A
  // CSS.escape keeps an id with special chars a valid selector; the value itself
  // is still only matched, never executed.
  const escaped = cssEscape(target);
  const el = view.dom.querySelector<HTMLElement>(`#${escaped}`);
  if (!el) {
    return false;
  }
  el.scrollIntoView({ block: "start" });
  return true;
}

// cssEscape escapes `s` for safe use as the value part of an id selector
// (`#<value>`). It prefers the native CSS.escape (all modern browsers, jsdom 20+);
// when CSS.escape is absent it falls back to a blanket identifier escape that
// backslash-escapes every character that is NOT a CSS-safe identifier char
// ([A-Za-z0-9_-]). The previous fallback escaped only `"` and `\`, leaving
// `.`/`#`/`[`/`:`/etc. unescaped so a crafted hash could form a compound selector
// and match an unintended element. `[A-Za-z0-9_-]` is always safe unescaped in a
// CSS identifier; every other char is escaped, which is what CSS.escape produces
// for the non-empty, non-numeric-leading identifiers all slug anchors satisfy.
function cssEscape(s: string): string {
  if (typeof CSS !== "undefined" && typeof CSS.escape === "function") {
    return CSS.escape(s);
  }
  return s.replace(/[^A-Za-z0-9_-]/g, (ch) => `\\${ch}`);
}
