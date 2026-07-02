// remark-hashtag — a remark plugin that renders body `#tag` as a safe badge
// `<span className="cm-hashtag">#tag</span>` in the react-markdown read surface
// (MarkdownProse → HistoryPanel + AgentAnswer). EP-01 / Phase 36.
//
// SECURITY (the load-bearing invariant, EP-01 XSS guard):
// The badge is a typed mdast node carrying `data.hName`/`data.hProperties` —
// mdast-util-to-hast converts it into a hast `element` node (`<span>`), NOT raw
// HTML. rehype-raw stays OFF and rehype-sanitize stays ON; `span` is in the
// default sanitize schema, so the element survives sanitize intact. The tag
// text flows through as a hast `text` node — it is never parsed as markup, so
// untrusted page bytes can never become live DOM. This is the same no-raw-HTML
// invariant the CM6 HashtagWidget upholds (createElement + textContent only).
//
// RECOGNITION (CONTEXT D-01/D-01b): the boundary rules mirror hashtag.ts so the
// read surface badges the SAME `#tag` set the live editor badges:
//   • Start boundary: `#` begins a tag only when the preceding char is
//     start-of-input, whitespace, or a non-word punctuation boundary — NOT
//     inside a word (`foo#bar`), after a backslash (`\#esc`), or after a slash
//     (`/#frag`). This single rule excludes escaped and URL-fragment `#`.
//   • Body: [A-Za-z0-9_/-]+, MUST contain ≥1 non-digit (so `#1984` is not a tag).
//   • A trailing `/` is trimmed (it is a separator, not a segment).
//   • Code exclusion is STRUCTURAL: react-markdown parses inline code (`code`
//     nodes) and fenced code separately — this plugin walks `text` nodes only
//     and never enters `code`/`inlineCode` children, so `` `#tag` `` and fenced
//     `#tag` stay literal. ATX heading markers are block marks, not inline text,
//     so `# Heading` is never matched. Link URLs are in `link` node `url` props,
//     not `text` children, so `http://x/#frag` is never matched.
import type { Plugin } from "unified";
import type { Root, Text, PhrasingContent } from "mdast";
import { visit, SKIP } from "unist-util-visit";

// Body char classes — mirror hashtag.ts isBody/isNonDigit so the recognized
// set is identical. [A-Za-z0-9_/-]; a valid tag needs ≥1 non-digit ([A-Za-z_-]).
const isBodyChar = (c: string): boolean =>
  /[A-Za-z0-9_/-]/.test(c);
const isNonDigit = (c: string): boolean =>
  /[A-Za-z_-]/.test(c);

// A valid start boundary: start-of-input, whitespace, or a non-word char that
// is NOT a backslash or slash (those exclude `\#esc` and `/#frag`). Mirrors the
// hashtag.ts "preceding char is start-of-input OR whitespace" rule, broadened
// to common punctuation that does not form a word continuation.
const isStartBoundary = (prev: string | null): boolean => {
  if (prev === null) return true;
  // Whitespace boundaries.
  if (/\s/.test(prev)) return true;
  // Backslash → escaped `#`, never a tag.
  if (prev === "\\") return false;
  // Slash → URL fragment, never a tag.
  if (prev === "/") return false;
  // A word char immediately before → `foo#bar` is not a tag start.
  if (/[A-Za-z0-9_]/.test(prev)) return false;
  // Other punctuation (e.g. `>`, `(`, `[`, `*`, `-`) → valid boundary, like the
  // CM6 grammar's "after a block marker" rule (e.g. `>#tag`).
  return true;
};

// scanTag examines `text` starting at index `i` (which must be `#`), and returns
// the exclusive end index of a recognized tag body, or -1 when the `#` here is
// not a tag start.
function scanTag(text: string, i: number): number {
  if (text[i] !== "#") return -1;
  let end = i + 1;
  let hasNonDigit = false;
  while (end < text.length) {
    const c = text[end];
    if (!isBodyChar(c)) break;
    if (isNonDigit(c)) hasNonDigit = true;
    end++;
  }
  // Trim a trailing '/' (separator, not a segment).
  while (end > i + 1 && text[end - 1] === "/") end--;
  // Reject bare `#` or an all-digit/all-separator run.
  if (end <= i + 1 || !hasNonDigit) return -1;
  return end;
}

// A badge mdast node: a custom `hashtag` node that mdast-util-to-hast renders as
// `<span className="cm-hashtag">#tag</span>` via data.hName/hProperties/hChildren.
// The children are hast `text` nodes (set via hChildren) so the tag bytes are
// never parsed as HTML — they flow through rehype-sanitize as inert text.
interface HashtagNode {
  type: "hashtag";
  value: string; // the full "#tag" text
  data: {
    hName: string;
    hProperties: { className: string[] };
    hChildren: PhrasingContent[];
  };
}

// remarkHashtag is the remark plugin. It walks `text` nodes in the mdast tree
// (skipping `code`/`inlineCode` — their text is never walked) and splits each
// text node containing a recognized `#tag` into [text, hashtag, text, …].
//
// ESCAPE DETECTION: remark consumes `\#` as an escape, MERGING the `#` into
// the adjacent text node — so the text value contains a bare `#` with no
// preceding `\`. To detect this, we use the text node's position to check the
// ORIGINAL SOURCE (from the vfile): if the source char before the `#`'s source
// offset is `\`, the `#` was escaped and must stay literal.
export const remarkHashtag: Plugin<[], Root> = () => {
  return (tree: Root, file) => {
    const source = typeof file?.value === "string" ? file.value : "";
    visit(tree, (node, index, parent) => {
      // Only split `text` nodes — inline code and fenced code are `code` nodes
      // and are never visited here (their value is in `value`, not children),
      // so `` `#tag` `` and fenced `#tag` stay literal.
      if (node.type !== "text") return;

      const text = (node as Text).value;
      if (!text.includes("#")) return;

      // Map text-node value indices to source offsets using position data, so
      // we can detect `\#` escapes by checking the source. When position is
      // unavailable (synthetic trees), we fall back to the within-node check.
      const pos = (node as Text).position;
      let srcOffsets: number[] | null = null;
      if (pos && source.length > 0 && pos.start.offset != null && pos.end.offset != null) {
        const start = pos.start.offset;
        const end = pos.end.offset;
        const span = source.slice(start, end);
        if (span.length === text.length) {
          // 1:1 mapping — no escapes consumed in this node.
          srcOffsets = text.split("").map((_, i) => start + i);
        } else {
          // Escapes consumed `\` chars. Walk the source span, skipping `\X`
          // pairs, and map each value char to its source offset.
          srcOffsets = [];
          let si = 0;
          for (let vi = 0; vi < text.length && si < span.length; ) {
            if (span[si] === "\\" && si + 1 < span.length) {
              // The escaped char in source maps to the current value char.
              srcOffsets.push(start + si + 1);
              si += 2;
              vi++;
            } else {
              srcOffsets.push(start + si);
              si++;
              vi++;
            }
          }
        }
      }

      const segments: Array<Text | HashtagNode> = [];
      let last = 0;
      for (let i = 0; i < text.length; i++) {
        if (text[i] !== "#") continue;
        // Check the within-node boundary.
        const boundaryChar = i > 0 ? text[i - 1] : null;
        if (!isStartBoundary(boundaryChar)) continue;

        // ESCAPE CHECK: if we have source offsets, check whether the source
        // char before the `#`'s source offset is `\` (escape). If so, stay
        // literal.
        if (srcOffsets && srcOffsets[i] != null) {
          const srcOff = srcOffsets[i];
          if (srcOff > 0 && source[srcOff - 1] === "\\") {
            continue;
          }
        }

        const end = scanTag(text, i);
        if (end < 0) continue;

        // Push the literal text before the tag.
        if (i > last) {
          segments.push({ type: "text", value: text.slice(last, i) });
        }
        // Push the badge node.
        const tagText = text.slice(i, end);
        segments.push({
          type: "hashtag",
          value: tagText,
          data: {
            hName: "span",
            hProperties: { className: ["cm-hashtag"] },
            hChildren: [{ type: "text", value: tagText }],
          },
        });
        last = end;
        i = end - 1; // the for-loop `i++` advances past the tag
      }
      // No tag found → leave the text node untouched.
      if (segments.length === 0) return;

      // Push any trailing literal text.
      if (last < text.length) {
        segments.push({ type: "text", value: text.slice(last) });
      }

      // Replace the single text node with the segments, in place.
      if (parent && index !== undefined) {
        parent.children.splice(index, 1, ...(segments as PhrasingContent[]));
        return SKIP;
      }
    });
  };
};