// tagNode — the LIVE-EDITOR `#tag` inline-atom node (RE-03, D-01).
//
// DISPLAY-ONLY, single-recognizer discipline. The Tag node renders a body `#tag`
// as an inert badge in the WYSIWYG surface and serializes back to the LITERAL
// `#tag` string via renderMarkdown. It never materializes tag DATA: on save the
// serialized body flows through the EXISTING reconcileFrontmatterTags ->
// inlineTagsFromBody (the ONE Lezer recognizer) exactly as the CM6 surface's body
// does. So this node is a visual affordance only; the on-disk tag set is owned by
// Lezer, never by TipTap storage (BLK-01). See tag-reconcile.ts / cm/hashtag.ts.
//
// It is registered ONLY in the live surface's extension list (TipTapEditor.tsx),
// NEVER in buildExtensions() — the headless codec stays tag-free so the Phase-38
// Tier-1 round-trip corpus is unperturbed. The tag round-trip is instead proven by
// a dedicated codec fixture (`#tag` in prose stays literal, codec.corpus.ts).
//
// SECURITY (MAJ-03 mirror): renderHTML's 3rd DOMOutputSpec element is a TEXT HOLE
// (`#${name}`) that ProseMirror's DOM serializer emits as textContent, NEVER
// innerHTML — matching the shipped PreservedInline / HashtagWidget / TagSuggest
// inert-text guard. A tag name can never smuggle markup into the DOM.
import { Node, nodeInputRule, nodePasteRule } from "@tiptap/core";

// THE RECOGNIZER GRAMMAR (mirrors cm/hashtag.ts scanBody + parseHashtag exactly):
//   • body alphabet: [A-Za-z0-9_/-] (digits ARE body so `#fff`/`#a1` work);
//   • a tag MUST contain >=1 NON-DIGIT-NON-SLASH char ([A-Za-z_-]) — this is what
//     rejects an all-digit run (`#1984`) and a digits+slashes run (`#12/34`);
//   • a trailing `/` is a dangling separator, trimmed off the name (`#a/` -> `a`).
// TAG_BODY is the shared source fragment; the lookahead requires the >=1-non-digit
// rule, then the run is consumed. `/` is a separator, so it is NOT a "non-digit"
// that qualifies the run (identical to hashtag.ts's isNonDigit).
const TAG_BODY = "#(?=[A-Za-z0-9_/-]*[A-Za-z_-])[A-Za-z0-9_/-]+";

// TAG_RE anchors the body fragment for a WHOLE-STRING recognizer check (used by the
// unit test to prove agreement with the Lezer corpus). It carries no boundary rule
// itself — the boundary (start-of-input OR whitespace before `#`) lives in the
// input/paste rules below (and in Lezer's parseHashtag), never inside scanBody.
export const TAG_RE = new RegExp(`^${TAG_BODY}$`);

// A tag NAME is the match text minus the leading `#` and any trailing separator `/`
// (the `.slice(1).replace(/\/+$/, "")` below mirrors scanBody's trailing-slash trim so
// the live badge name === the Lezer materializer's name).

// INPUT_RE fires the node LIVE as the user types: `#tag` at start-of-block or after
// whitespace, COMMITTED by a trailing space. The trailing-space trigger is what lets
// the user keep typing `#deploy` without the atom sealing at `#d` (the `#`-suggestion
// popup is the primary affordance; this input rule is the type-through convenience).
// nodeInputRule re-inserts the trailing space and replaces only the `#tag` run
// (match[1]); a leading space (match[0][0]) is preserved because it sits before the
// replaced range.
const INPUT_RE = new RegExp(`(?:^|\\s)(${TAG_BODY})\\s$`);

// PASTE_RE converts pasted `#tag` runs into nodes. Global + a variable-length
// lookbehind `(?<=^|\\s)` enforces the same start/whitespace boundary WITHOUT
// consuming the preceding char, so the paste range covers only `#tag` (never the
// leading space) — matching how nodePasteRule slices match[0].
const PASTE_RE = new RegExp(`(?<=^|\\s)${TAG_BODY}`, "g");

// Tag — the inline-atom badge node. group:"inline" + inline + atom mirror the
// shipped PreservedInline shape; `name` is the tag string WITHOUT the leading `#`.
export const Tag = Node.create({
  name: "tag",
  group: "inline",
  inline: true,
  atom: true,
  selectable: true,

  addAttributes() {
    return { name: { default: "" } };
  },

  parseHTML() {
    return [
      {
        tag: "span[data-tag]",
        getAttrs: (el) => ({
          name: (el as HTMLElement).getAttribute("data-tag") ?? "",
        }),
      },
    ];
  },

  // The 3rd array element `#${name}` is a TEXT HOLE — ProseMirror's DOM serializer
  // renders it as textContent, never innerHTML (the inert-text guard, MAJ-03).
  renderHTML({ node }) {
    return [
      "span",
      { "data-tag": node.attrs.name as string, class: "tag-badge" },
      `#${node.attrs.name as string}`,
    ];
  },

  // renderMarkdown serializes to the LITERAL `#tag` string (D-01, BLK-01): the saved
  // body carries `#tag` so the existing Lezer materializer sees it on save.
  renderMarkdown: (node) => `#${(node.attrs?.name as string) ?? ""}`,

  addInputRules() {
    return [
      nodeInputRule({
        find: INPUT_RE,
        type: this.type,
        getAttributes: (match) => ({ name: match[1].slice(1).replace(/\/+$/, "") }),
      }),
    ];
  },

  addPasteRules() {
    return [
      nodePasteRule({
        find: PASTE_RE,
        type: this.type,
        getAttributes: (match) => ({ name: match[0].slice(1).replace(/\/+$/, "") }),
      }),
    ];
  },
});
