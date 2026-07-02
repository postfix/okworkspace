// @vitest-environment node
//
// codec.test.ts — THE GO/NO-GO GATE for the v1.7 milestone (RE-01 crit-2 + crit-4).
//
// Runs under a `node` environment (the docblock above OVERRIDES the app's global jsdom
// in vitest.config.ts). This is LOAD-BEARING for security (D-04, MAJ-04-b): the codec's
// no-HTML->DOM property holds ONLY when `window.DOMParser` is absent. jsdom would supply
// it and silently re-open the HTML->DOM path — so the no-DOM assertions below both prove
// we are in node AND guard against a future accidental jsdom switch.
//
// The gate, per Tier-1 fixture:
//   1. TREE-EQUALITY  — parse(roundTrip(src)) deep-equals parse(src). Marker
//      canonicalization (*/+ -> -, __/_ -> **/*, loose lists tightened) is ALLOWED
//      Tier-1 normalization (D-02); the TipTap JSON tree must be identical.
//   2. IDEMPOTENCE    — roundTrip(roundTrip(src)) === roundTrip(src). Serialization is
//      stable on a second pass.
//
// Plus the MAJ-04-b security section: embedded raw HTML round-trips to inert
// entity-escaped literal text (no live DOM node, no executable attribute in the JSON).
//
// TDD: written before codec.ts existed, it fails for the right reason (missing module).
import { describe, it, expect } from "vitest";
import type { JSONContent } from "@tiptap/core";
import { parse, roundTrip } from "./codec";
import { TIER1_CORPUS, SECURITY_CORPUS } from "./codec.corpus";

// collectTree walks a TipTap JSON document and gathers, across every node: the set of
// node `type`s, the set of attribute keys, and the concatenated text. The security
// assertions use it to prove embedded HTML never became an element node with an
// executable attribute (used across every security fixture -> a real shared helper).
function collectTree(node: JSONContent): {
  types: Set<string>;
  attrKeys: Set<string>;
  text: string;
} {
  const types = new Set<string>();
  const attrKeys = new Set<string>();
  let text = "";
  const visit = (n: JSONContent): void => {
    if (typeof n.type === "string") types.add(n.type);
    if (n.attrs) for (const k of Object.keys(n.attrs)) attrKeys.add(k);
    if (typeof n.text === "string") text += n.text;
    if (n.content) for (const child of n.content) visit(child);
  };
  visit(node);
  return { types, attrKeys, text };
}

// The bounded-slice node types the codec is allowed to emit. Any type outside this set
// in a round-tripped SECURITY fixture would mean embedded HTML became a real node.
const ALLOWED_NODE_TYPES = new Set([
  "doc",
  "paragraph",
  "text",
  "heading",
  "bulletList",
  "orderedList",
  "listItem",
  "taskList",
  "taskItem",
  "blockquote",
  "codeBlock",
  "hardBreak",
]);

// Executable / DOM-only attribute names that MUST NOT appear as live node attributes
// after round-tripping raw HTML (they may only exist inside literal escaped text).
const EXECUTABLE_ATTRS = ["onerror", "onclick", "onload", "src", "href", "onmouseover"];

describe("tiptap markdown codec — Tier-1 round-trip gate (RE-01)", () => {
  // The node-env guard (D-04): proves we are NOT under jsdom. If a future change flips
  // the codec test to jsdom, window.DOMParser reappears and the HTML->DOM path re-opens
  // — this assertion fails loudly before that can become a silent security regression.
  it("runs headless in a node env (no window, no document)", () => {
    expect(typeof globalThis.window).toBe("undefined");
    expect(typeof globalThis.document).toBe("undefined");
  });

  for (const [id, src] of Object.entries(TIER1_CORPUS)) {
    describe(id, () => {
      it("tree-equality: parse(roundTrip(src)) deep-equals parse(src)", () => {
        expect(parse(roundTrip(src))).toEqual(parse(src));
      });

      it("idempotence: roundTrip(roundTrip(src)) === roundTrip(src)", () => {
        const once = roundTrip(src);
        expect(roundTrip(once)).toBe(once);
      });
    });
  }
});

describe("tiptap markdown codec — HTML inertness (MAJ-04-b, D-04)", () => {
  for (const [id, src] of Object.entries(SECURITY_CORPUS)) {
    describe(id, () => {
      it("round-trips embedded HTML to entity-escaped literal text (no raw tag)", () => {
        const out = roundTrip(src);
        // The angle brackets are entity-escaped...
        expect(out).toContain("&lt;");
        // ...and no raw opening tag survives (a `<` immediately followed by a letter or
        // a `/` would be a live tag; escaping turns every one into `&lt;`).
        expect(out).not.toMatch(/<[a-zA-Z/]/);
      });

      it("parses to text-only nodes with no element node and no executable attribute", () => {
        const { types, attrKeys } = collectTree(parse(roundTrip(src)));
        // Every node type is within the bounded slice — no `image`/`div`/`script` etc.
        for (const t of types) expect(ALLOWED_NODE_TYPES.has(t)).toBe(true);
        // No live executable/DOM attribute anywhere in the parsed JSON.
        for (const bad of EXECUTABLE_ATTRS) expect(attrKeys.has(bad)).toBe(false);
      });

      it("is idempotent (stable escaped text on a second pass)", () => {
        const once = roundTrip(src);
        expect(roundTrip(once)).toBe(once);
      });
    });
  }

  it("keeps a dangerous handler as inert text, never a live attribute", () => {
    const out = roundTrip(SECURITY_CORPUS.imgOnerror);
    // The literal characters survive (inside escaped text)...
    expect(out).toContain("onerror");
    // ...but there is no live `<img ...>` element to carry them.
    expect(out).not.toContain("<img");
    const { types, attrKeys } = collectTree(parse(out));
    expect(types.has("image")).toBe(false);
    expect(attrKeys.has("onerror")).toBe(false);
  });
});
