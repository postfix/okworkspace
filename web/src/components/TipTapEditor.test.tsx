// TipTapEditor.test.tsx — the LIVE @tiptap/react surface under jsdom (RE-03).
//
// This is a DOM component, so it runs under the app's DEFAULT jsdom env (vitest.config.ts).
// It deliberately has NO per-file environment docblock — the headless codec keeps its own
// node-env directive, but the live editor legitimately needs a DOM.
// @testing-library/react + user-event drive real typing; the ProseMirror view is real.
//
// Coverage:
//   • RE-03-1/2 — typing `#tag ` yields a span[data-tag] badge AND onChange /
//     getMarkdown carry literal `#tag`; insertAtCursor inserts markdown at the caret.
//   • RE-03-5 (BLK-01) — the TipTap-serialized body, fed through the REAL
//     reconcileFrontmatterTags → inlineTagsFromBody (Lezer), materializes the SAME
//     tag CM6's body would — proving the surface routes through the one save path.
//   • RE-03-4 (MAJ-05) — a paste-XSS payload yields NO live node and NO executable
//     attribute: Layer 2 (sanitizePastedHtml) scrubs attrs/scripts, Layer 1
//     (ProseMirror's schema-bound clipboard parser) drops unknown nodes.
//   • RE-03-3 (MAJ-03) — the `#`-suggest popup renders a markup-bearing vocabulary
//     entry as INERT text (textContent only, no child element, no handler).
import { describe, it, expect, vi, beforeAll, beforeEach } from "vitest";
import { render, act } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createRef } from "react";
import { getSchema } from "@tiptap/core";
import type { JSONContent } from "@tiptap/core";
import { Markdown } from "@tiptap/markdown";
import { DOMParser as PMDOMParser } from "@tiptap/pm/model";

// A vocabulary entry carrying HTML markup — if the popup ever used innerHTML this would
// become a live <img> with an onerror handler. MAJ-03 requires it to stay inert text.
// Defined via vi.hoisted so the (hoisted) vi.mock factory below can reference it without
// a temporal-dead-zone crash during module collection.
const { MARKUP_TAG } = vi.hoisted(() => ({
  MARKUP_TAG: "<img src=x onerror=alert(1)>",
}));

// getTagVocabulary is mocked so the `#`-suggest has a deterministic vocabulary — one
// benign tag and one MARKUP-BEARING entry (the MAJ-03 stored-XSS probe).
vi.mock("../api/client", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../api/client")>();
  return {
    ...actual,
    getTagVocabulary: vi.fn(async () => ["deploy", MARKUP_TAG]),
  };
});

import TipTapEditor from "./TipTapEditor";
import { sanitizePastedHtml } from "../lib/tiptap/sanitizePaste";
import { buildExtensions } from "../lib/tiptap/extensions";
import { Tag } from "../lib/tiptap/tagNode";
import { reconcileFrontmatterTags } from "../lib/tag-reconcile";
import { inlineTagsFromBody } from "../lib/cm/hashtag";
import type { EditorSurfaceHandle } from "./LivePreviewEditor";
// jsdom does not implement the layout APIs ProseMirror touches — getClientRects /
// elementFromPoint on selection change, and getBoundingClientRect on scrollToSelection
// (fired from a post-transaction timer). Stub them so mounting/typing a real ProseMirror
// view does not throw (incl. the async scroll AFTER a test). Scoped to this DOM-editor
// test; the headless codec's no-DOM invariant is unaffected.
const emptyRectList = (): DOMRectList =>
  Object.assign([], { item: () => null }) as unknown as DOMRectList;
const zeroRect = (): DOMRect => ({
  x: 0,
  y: 0,
  width: 0,
  height: 0,
  top: 0,
  right: 0,
  bottom: 0,
  left: 0,
  toJSON: () => ({}),
});
beforeAll(() => {
  if (!Element.prototype.getClientRects) {
    Element.prototype.getClientRects = emptyRectList;
  }
  if (!Range.prototype.getClientRects) {
    Range.prototype.getClientRects = emptyRectList;
  }
  if (!Element.prototype.getBoundingClientRect) {
    Element.prototype.getBoundingClientRect = zeroRect;
  }
  if (!Range.prototype.getBoundingClientRect) {
    Range.prototype.getBoundingClientRect = zeroRect;
  }
  if (!document.elementFromPoint) {
    document.elementFromPoint = () => null;
  }
});

// collectDoc walks a TipTap/ProseMirror JSON document and gathers, across every node,
// the set of node `type`s and the set of attribute KEYS — the inertness oracle (a live
// script/image node or an onerror attribute KEY would appear here).
function collectDoc(node: JSONContent): { types: Set<string>; attrKeys: Set<string> } {
  const types = new Set<string>();
  const attrKeys = new Set<string>();
  const visit = (n: JSONContent): void => {
    if (typeof n.type === "string") types.add(n.type);
    if (n.attrs) for (const k of Object.keys(n.attrs)) attrKeys.add(k);
    if (n.content) for (const child of n.content) visit(child);
  };
  visit(node);
  return { types, attrKeys };
}

// mountEditor renders the surface and waits a tick for the vocabulary fetch to settle.
async function mountEditor(value = "") {
  const onChange = vi.fn();
  const ref = createRef<EditorSurfaceHandle>();
  const utils = render(
    <TipTapEditor
      ref={ref}
      value={value}
      onChange={onChange}
      currentPath="notes/a.md"
      mode="live"
    />,
  );
  await act(async () => {
    await Promise.resolve();
  });
  const pmEl = utils.container.querySelector(".ProseMirror");
  const pm = pmEl as HTMLElement; // well-known DOM node: the contenteditable host.
  return { ...utils, onChange, ref, pm };
}

// tick resolves after a macrotask so the suggestion plugin's async items() settles.
function tick(ms = 20): Promise<void> {
  const { promise, resolve } = Promise.withResolvers<void>();
  setTimeout(resolve, ms);
  return promise;
}

describe("TipTapEditor — live @tiptap/react surface (RE-03)", () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it("mounts one ProseMirror surface seeded from the markdown value", async () => {
    const { container } = await mountEditor("hello **world**");
    const editors = container.querySelectorAll(".ProseMirror");
    expect(editors.length).toBe(1);
    // The bold mark round-trips from the markdown seed into a live <strong>.
    expect(container.querySelector(".ProseMirror strong")?.textContent).toBe("world");
  });

  // RE-03-1/2: typing `#deploy ` fires the input rule → a span[data-tag] badge, and the
  // surface's onChange (editor.getMarkdown()) carries the LITERAL `#deploy`.
  it("typing #tag creates a badge node and serializes to literal #tag", async () => {
    const { pm, onChange, container } = await mountEditor("start ");
    const user = userEvent.setup();
    pm.focus();
    await user.type(pm, "#deploy ");

    const badge = container.querySelector("span[data-tag]");
    expect(badge).not.toBeNull();
    expect(badge?.getAttribute("data-tag")).toBe("deploy");
    // Badge label is inert text (textContent) — the '#' prefix + the name, no markup.
    expect(badge?.textContent).toBe("#deploy");

    const lastMarkdown = onChange.mock.calls.at(-1)?.[0];
    expect(lastMarkdown).toContain("#deploy");
  });

  // RE-03-2: insertAtCursor inserts markdown at the caret and fires onChange.
  it("insertAtCursor inserts markdown at the caret", async () => {
    const { pm, onChange, ref } = await mountEditor("intro ");
    pm.focus();
    act(() => {
      ref.current?.insertAtCursor("**bold**");
    });
    const lastMarkdown = onChange.mock.calls.at(-1)?.[0];
    expect(lastMarkdown).toContain("**bold**");
  });

  // RE-03-5 (BLK-01): the DISPLAY-only Tag node serializes to literal `#tag`; the
  // materialization runs through the EXISTING reconcileFrontmatterTags → inlineTagsFromBody
  // (Lezer) over the SERIALIZED body — never TipTap storage. Prove the TipTap-serialized
  // body materializes the SAME tag CM6's identical body would (no second materializer).
  it("materializes body #tags through the REAL reconcileFrontmatterTags (BLK-01)", async () => {
    const { pm, onChange } = await mountEditor("");
    const user = userEvent.setup();
    pm.focus();
    await user.type(pm, "#deploy ");

    const tiptapBody = onChange.mock.calls.at(-1)?.[0] ?? "";
    // The serialized body carries the literal tag (the load-bearing fact).
    expect(tiptapBody).toContain("#deploy");

    // Feed the TipTap-serialized body through the REAL save-path materializer.
    const result = reconcileFrontmatterTags({
      lastSavedBody: "",
      currentBody: tiptapBody,
      frontmatter: "",
    });
    expect(result.changed).toBe(true);
    expect(result.frontmatter).toContain("deploy");

    // Parity: the Lezer recognizer materializes the SAME set from the TipTap body as it
    // would from a hand-written CM6 body — the two surfaces cannot fork the saved tag set.
    expect(inlineTagsFromBody(tiptapBody)).toEqual(inlineTagsFromBody("#deploy"));
  });

  // RE-03-4 (MAJ-05): the paste-XSS keystone. A payload combining <img onerror>,
  // <script>, and <div onclick> must yield NO live node and NO executable attribute
  // through BOTH defense layers, exactly as a real paste flows:
  //   Layer 2 — sanitizePastedHtml (editorProps.transformPastedHTML) scrubs the raw HTML.
  //   Layer 1 — ProseMirror's schema-bound clipboard parser (DOMParser.fromSchema) then
  //             emits ONLY schema-valid nodes; unknown elements are structurally dropped.
  describe("paste hardening (MAJ-05)", () => {
    const PAYLOAD =
      '<img src=x onerror="alert(1)"><script>alert(1)</script><div onclick="steal()">z</div><a href="javascript:evil()">l</a>';

    it("Layer 2: sanitizePastedHtml strips scripts, event-handler attrs, and javascript: URLs", () => {
      const cleaned = sanitizePastedHtml(PAYLOAD);
      expect(cleaned).not.toMatch(/<script/i);
      expect(cleaned).not.toMatch(/onerror/i);
      expect(cleaned).not.toMatch(/onclick/i);
      expect(cleaned).not.toMatch(/javascript:/i);
    });

    it("Layer 1+2: parsing the sanitized paste through the schema yields no live node/attr", () => {
      // Build the SAME schema the live editor parses the clipboard with (buildExtensions
      // + Markdown + Tag; the suggestion extensions add no schema nodes).
      const schema = getSchema([...buildExtensions(), Markdown, Tag]);
      const cleaned = sanitizePastedHtml(PAYLOAD);

      // Reconstruct the clipboard-parse path: the sanitized HTML becomes a DOM, then
      // ProseMirror's schema-bound DOMParser emits only schema-valid nodes (the exact
      // editorProps.clipboardParser default). Building the DOM to parse is how ProseMirror
      // itself reads the clipboard — the inert-string guard is the component's job (done).
      const holder = document.createElement("div");
      holder.innerHTML = cleaned;
      const pmDoc = PMDOMParser.fromSchema(schema).parse(holder);
      const { types, attrKeys } = collectDoc(pmDoc.toJSON());

      // No live element node for the pasted markup.
      for (const forbidden of ["script", "iframe", "image", "div"]) {
        expect(types.has(forbidden)).toBe(false);
      }
      // No executable/DOM attribute survived as a schema-recognized attribute key.
      for (const forbidden of ["onerror", "onclick", "onload", "onmouseover"]) {
        expect(attrKeys.has(forbidden)).toBe(false);
      }
    });
  });

  // RE-03-3 (MAJ-03): the `#`-suggest popup renders a markup-bearing vocabulary entry as
  // INERT text — the item's textContent is the LITERAL string, with no child element and
  // no executed handler (the popup uses textContent / DOM text nodes, never innerHTML).
  it("suggestion popup renders a markup-bearing vocabulary entry as inert text (MAJ-03)", async () => {
    const { pm } = await mountEditor("");
    const user = userEvent.setup();
    pm.focus();
    await user.type(pm, "#");
    await act(async () => {
      await tick();
    });

    const popup = document.querySelector(".tiptap-suggest");
    expect(popup).not.toBeNull();
    // The markup-bearing entry appears as LITERAL text in the popup's textContent.
    expect(popup?.textContent).toContain(MARKUP_TAG);
    // …and it produced NO live element: no <img> (or any injected child) exists.
    expect(popup?.querySelectorAll("img").length).toBe(0);
    // Each vocabulary entry is exactly one row; the markup entry did not spawn children.
    const items = popup?.querySelectorAll(".tiptap-suggest-item") ?? [];
    expect(items.length).toBe(2);
    const markupRow = Array.from(items).find((el) => el.textContent === MARKUP_TAG);
    expect(markupRow).toBeDefined();
    expect(markupRow?.children.length).toBe(0);
  });
});
