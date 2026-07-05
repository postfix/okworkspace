// preserved — the Tier-2 VERBATIM-PRESERVATION nodes (RE-02, D-01/D-02/D-03/D-04).
//
// Tier-1 (Phase 38, extensions.ts) round-trips the bounded slice tree-equal. Tier-2
// adds BYTE-FOR-BYTE preservation of constructs TipTap can't (yet) model — GFM tables,
// `$…$`/`$$…$$` math, and raw HTML (block + inline) — by capturing each construct's
// verbatim marked-token source (`token.raw`) into a `raw` attribute of an ATOM node and
// re-emitting that attribute unchanged.
//
// WHY THIS IS BYTE-EXACT (grounded in @tiptap/markdown 3.27.1 source):
//   - Every marked token carries the exact consumed source in `token.raw`
//     (marked drives its block loop with `src = src.substring(token.raw.length)`).
//   - `markdownTokenName` maps one marked token type -> this extension's parseMarkdown,
//     and registered handlers run in parseToken BEFORE the built-in fallback
//     (index.js:571-590), so we intercept the native `table`/`html` token with its raw.
//   - A custom NON-text node's `renderMarkdown(node)` return is emitted VERBATIM — it
//     never passes through encodeTextForMarkdown's markdown-escaper/entity-encoder
//     (index.js:1076-1080 short-circuits only text nodes). So returning `node.attrs.raw`
//     re-emits the captured source unchanged.
//   - `markdownTokenizer` registers the `$`/`$$` math tokens marked lacks (marked runs
//     custom block/inline extensions before its built-in tokenizers).
//
// SECURITY (D-04, MAJ-04): raw HTML is preserved as an INERT STRING attribute and
// re-emitted as literal source — the DOM-construction path (the built-in HTML->JSON
// generator behind parseHTMLToken) is NEVER invoked for page bytes. The codec runs
// headless under a `node` env; the manager's no-window guard (index.js:950) is the belt,
// and this handler is the suspenders — it intercepts the `html` token BEFORE
// parseHTMLToken can ever reach that DOM path. An `<img onerror>`/`<script>` fixture
// becomes a `preservedBlock`/`preservedInline` holding the HTML as `raw` text — no
// element node, no executable attribute in the parsed JSON. See codec.test.ts.
//
// IMPORT-ISOLATION (RE-04): like the rest of web/src/lib/tiptap/*, nothing reachable
// from the app entry imports this module until Phase 42 — it flows only through
// buildExtensions() into the codec and its tests.
import { Node, Extension } from "@tiptap/core";
import type { MarkdownToken, MarkdownTokenizer } from "@tiptap/core";

// A shared attribute spec: `raw` holds the verbatim source slice. `rendered: false`
// keeps it OUT of the auto-merged DOM attributes (it is surfaced as inert textContent by
// each node's renderHTML instead) so the future editor host never reflects page bytes
// into a live DOM attribute — reinforcing the D-04 inert-literal-text invariant.
const rawAttribute = { raw: { default: "", rendered: false } } as const;

// PreservedBlock — a block-level ATOM node that captures a construct's verbatim source
// and re-emits it byte-for-byte. It is the RENDER node for both the native `table` token
// (parse registered here) and the block `html` token (parse registered by the thin
// PreservedBlockHtml sibling below — markdownTokenName is 1:1, so one string per node,
// but both route to this node type, which owns the single renderMarkdown).
export const PreservedBlock = Node.create({
  name: "preservedBlock",
  group: "block",
  atom: true,
  code: true,
  selectable: true,
  addAttributes() {
    return rawAttribute;
  },
  // Editor DOM (Phase 40+). Inert: the raw source is textContent (never innerHTML), so
  // no page byte is ever parsed as live markup by the browser.
  parseHTML() {
    return [{ tag: "div[data-preserved-block]", getAttrs: (el) => ({ raw: (el as HTMLElement).textContent ?? "" }) }];
  },
  renderHTML({ node }) {
    return ["div", { "data-preserved-block": "" }, (node.attrs.raw as string) ?? ""];
  },
  // Intercept the native GFM `table` token -> store verbatim. marked greedily FOLDS the
  // trailing inter-block separator (`\n\n`) into a table's `.raw` when another block
  // follows; that separator belongs to the serializer (research caveat #7), not the
  // construct, so strip trailing newlines only (the rows themselves are byte-exact). The
  // serializer re-inserts one `\n\n` between blocks — without the strip, interior tables
  // would double the blank line and break idempotence.
  markdownTokenName: "table",
  parseMarkdown: (token: MarkdownToken) => ({
    type: "preservedBlock",
    attrs: { raw: (token.raw ?? "").replace(/\n+$/, "") },
  }),
  // Re-emit the captured source unchanged (bypasses the text escaper entirely).
  renderMarkdown: (node) => (node.attrs?.raw as string) ?? "",
});

// PreservedBlockHtml — a parse-only sibling that routes the native block `html` token to
// the PreservedBlock node type, capturing the MERGED verbatim raw. It adds NO schema
// node (a plain Extension); render is owned by PreservedBlock's renderMarkdown (keyed by
// node name in the manager's nodeTypeRegistry). Registering this handler for `html` wins
// over the built-in parseHTMLToken (registered handlers run before the fallback), which
// is what upgrades block HTML from Phase-38 lossy-escaped text to byte-exact inert text.
export const PreservedBlockHtml = Extension.create({
  name: "preservedBlockHtml",
  markdownTokenName: "html",
  // Same block-separator strip as the table handler: marked folds a trailing `\n\n`
  // into a block html token's merged `.raw` when a block follows; that separator is the
  // serializer's, not the construct's. Internal newlines (multi-line HTML) are preserved.
  parseMarkdown: (token: MarkdownToken) => ({
    type: "preservedBlock",
    attrs: { raw: (token.raw ?? "").replace(/\n+$/, "") },
  }),
});

// PreservedInline — an inline ATOM node for verbatim inline constructs (inline math and
// inline HTML). An inline atom (not a mark) is the cleanest carrier: renderMarkdown
// output is emitted as-is, so arbitrary bytes round-trip without the mark open/close
// placeholder dance. It is the RENDER node for the `inlineMath` and `inlineHtml` tokens.
export const PreservedInline = Node.create({
  name: "preservedInline",
  group: "inline",
  inline: true,
  atom: true,
  code: true,
  selectable: true,
  addAttributes() {
    return rawAttribute;
  },
  parseHTML() {
    return [{ tag: "span[data-preserved-inline]", getAttrs: (el) => ({ raw: (el as HTMLElement).textContent ?? "" }) }];
  },
  renderHTML({ node }) {
    return ["span", { "data-preserved-inline": "" }, (node.attrs.raw as string) ?? ""];
  },
  renderMarkdown: (node) => (node.attrs?.raw as string) ?? "",
});

// blockMathTokenizer — marked has no `$$…$$` rule. Register a custom BLOCK tokenizer
// whose `.raw` is the EXACT `$$…$$` slice (a wrong raw would mislocate the token via the
// manager's `src.indexOf(result.raw)` and corrupt surrounding text). The `tokenize` regex
// is anchored `^` so marked only forms the token at a block boundary; `[\s\S]+?` is lazy
// so the FIRST closing `$$` ends it.
//
// `start` is a paragraph-INTERRUPT hint (marked cuts a paragraph short at the returned
// index to give this extension a chance). A naive `indexOf("$$")` reports a mid-line `$$`
// that has NO closing pair (e.g. prose "text $$ here"), which cuts the paragraph and
// injects a stray newline — corrupting Tier-1 bytes. So `start` reports only the index of
// a COMPLETE, line-anchored `$$…$$` block (the same shape `tokenize` will accept),
// stripping any leading newline the anchor consumed; otherwise src.length (= no interrupt).
const blockMathTokenizer: MarkdownTokenizer = {
  name: "blockMath",
  level: "block",
  start: (src) => {
    const m = /(?:^|\n)\$\$[\s\S]*?\$\$/.exec(src);
    if (!m) return src.length;
    return m.index + (src[m.index] === "\n" ? 1 : 0);
  },
  tokenize: (src) => {
    const m = /^\$\$([\s\S]+?)\$\$/.exec(src);
    return m ? { type: "blockMath", raw: m[0] } : undefined;
  },
};

// BlockMath — the block-math render node + its tokenizer. Verbatim in, verbatim out.
export const BlockMath = Node.create({
  name: "blockMath",
  group: "block",
  atom: true,
  code: true,
  selectable: true,
  addAttributes() {
    return rawAttribute;
  },
  parseHTML() {
    return [{ tag: "div[data-block-math]", getAttrs: (el) => ({ raw: (el as HTMLElement).textContent ?? "" }) }];
  },
  renderHTML({ node }) {
    return ["div", { "data-block-math": "" }, (node.attrs.raw as string) ?? ""];
  },
  markdownTokenName: "blockMath",
  parseMarkdown: (token: MarkdownToken) => ({ type: "blockMath", attrs: { raw: token.raw ?? "" } }),
  renderMarkdown: (node) => (node.attrs?.raw as string) ?? "",
  markdownTokenizer: blockMathTokenizer,
});

// inlineMathTokenizer — custom INLINE tokenizer for `$…$`. Guards:
//   - `(?!\$)` after the opening `$` so it never eats a `$$` block delimiter;
//   - `[^$\n]+?` so the content has no `$`/newline (a lone `$` in prose can't pair
//     across the whole line into a false match, and `$5 ... $10` won't span);
//   - `.raw` is the exact `$…$` slice (delimiters included) for correct relocation.
// A stray `$` with no closing pair returns undefined -> marked treats it as plain text
// (no Tier-1 regression; the Tier-1 corpus has no `$` at all).
const inlineMathTokenizer: MarkdownTokenizer = {
  name: "inlineMath",
  level: "inline",
  start: (src) => {
    const i = src.indexOf("$");
    return i < 0 ? src.length : i;
  },
  tokenize: (src) => {
    const m = /^\$(?!\$)([^$\n]+?)\$/.exec(src);
    return m ? { type: "inlineMath", raw: m[0] } : undefined;
  },
};

// InlineMath — a parse-only + tokenizer Extension routing `inlineMath` tokens to the
// PreservedInline node. No schema node (render owned by PreservedInline).
export const InlineMath = Extension.create({
  name: "inlineMath",
  markdownTokenName: "inlineMath",
  parseMarkdown: (token: MarkdownToken) => ({ type: "preservedInline", attrs: { raw: token.raw ?? "" } }),
  markdownTokenizer: inlineMathTokenizer,
});

// inlineHtmlTokenizer — the inline-HTML byte-parity route (D-02 / research caveat).
// marked's parseInlineTokens has a PRE-EMPTIVE inline-`html` branch (index.js:773-812)
// that merges `<tag>…</tag>` and calls parseHTMLToken directly, BEFORE the generic
// handler dispatch — a `markdownTokenName:'html'` inline registration can never see it.
// So we tokenize inline HTML OURSELVES with a token type `!= 'html'` (`inlineHtml`) so it
// lands in the generic handler path (index.js:822-826) and routes to PreservedInline.
//
// The regex matches, in order: a paired element `<tag …>…</tag>` (lazy inner, same tag
// name via backreference), then a self-closing/void/standalone single tag `<tag …/?>`,
// then a bare closing tag `</tag>`. `.raw` is the exact matched slice. Guards against a
// stray `<` in prose: the open form requires `[a-zA-Z]` immediately after `<`, so `a < b`
// (space after `<`) never matches -> falls through to marked's text tokenizer (no Tier-1
// regression; the Tier-1 corpus has no `<`).
const INLINE_HTML_RE =
  /^<([a-zA-Z][\w-]*)(?:\s[^<>]*?)?>[\s\S]*?<\/\1\s*>|^<[a-zA-Z][\w-]*(?:\s[^<>]*?)?\/?>|^<\/[a-zA-Z][\w-]*\s*>/;

const inlineHtmlTokenizer: MarkdownTokenizer = {
  name: "inlineHtml",
  level: "inline",
  start: (src) => {
    const i = src.indexOf("<");
    return i < 0 ? src.length : i;
  },
  tokenize: (src) => {
    const m = INLINE_HTML_RE.exec(src);
    return m ? { type: "inlineHtml", raw: m[0] } : undefined;
  },
};

// InlineHtml — parse-only + tokenizer Extension routing `inlineHtml` tokens to the
// PreservedInline node. Keeps inline HTML inert literal text (D-04), byte-for-byte.
export const InlineHtml = Extension.create({
  name: "inlineHtml",
  markdownTokenName: "inlineHtml",
  parseMarkdown: (token: MarkdownToken) => ({ type: "preservedInline", attrs: { raw: token.raw ?? "" } }),
  markdownTokenizer: inlineHtmlTokenizer,
});

// preservedExtensions — the full Tier-2 set, appended by buildExtensions(). Order:
// render nodes first (schema), then parse-only siblings (routing). The manager registers
// each; parse handlers intercept before the built-in fallback, tokenizers run before
// marked's built-ins.
export const preservedExtensions = [
  PreservedBlock,
  PreservedInline,
  BlockMath,
  PreservedBlockHtml,
  InlineMath,
  InlineHtml,
];
