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
import { TIER1_CORPUS, TIER2_CORPUS, MIXED_CORPUS, SECURITY_CORPUS } from "./codec.corpus";

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

// The node types the codec is allowed to emit: the Tier-1 bounded slice PLUS the Tier-2
// preserved atoms. Crucially, the preserved atoms hold raw HTML as a `raw` STRING attr —
// they are NOT element nodes for that HTML — so their presence is the CORRECT inert
// outcome, while any FORBIDDEN element type below would mean HTML became a live node.
const ALLOWED_NODE_TYPES: Record<string, true> = {
  doc: true,
  paragraph: true,
  text: true,
  heading: true,
  bulletList: true,
  orderedList: true,
  listItem: true,
  taskList: true,
  taskItem: true,
  blockquote: true,
  codeBlock: true,
  hardBreak: true,
  // Tier-2 verbatim-preservation atoms (RE-02): carriers of exact source bytes, inert.
  preservedBlock: true,
  preservedInline: true,
  blockMath: true,
};

// Element node types the raw-HTML fixtures MUST NEVER become — appearance of any of these
// in the parsed JSON means embedded HTML was turned into a live DOM element (MAJ-04 breach).
const FORBIDDEN_ELEMENT_TYPES = ["image", "div", "script", "span", "em", "iframe"];

// Executable / DOM-only attribute names that MUST NOT appear as live node attribute KEYS
// after round-tripping raw HTML (they may only exist as characters inside the preserved
// `raw` string, never as a real attribute the schema recognizes).
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

// The Tier-2 gate: verbatim preservation is EXACT by contract, so this asserts BYTE
// equality (modulo outer trim) — stronger than Tier-1's tree-equality — plus idempotence,
// for every Tier-2 fixture AND every mixed Tier-1+Tier-2 document. `.trim()` on both sides
// is the sanctioned "modulo outer trim" allowance (the serializer owns inter-block
// blank-line joins; each construct's own bytes are preserved). A byte mismatch here is a
// real preservation failure — the assertion is NEVER loosened to "close enough" (D-05).
describe("tiptap markdown codec — Tier-2 verbatim byte-equality (RE-02, D-05)", () => {
  const tier2 = { ...TIER2_CORPUS, ...MIXED_CORPUS };
  for (const [id, src] of Object.entries(tier2)) {
    describe(id, () => {
      it("byte-for-byte (modulo outer trim): roundTrip(src).trim() === src.trim()", () => {
        expect(roundTrip(src).trim()).toBe(src.trim());
      });

      it("idempotence: roundTrip(roundTrip(src)) === roundTrip(src)", () => {
        const once = roundTrip(src);
        expect(roundTrip(once)).toBe(once);
      });
    });
  }
});

// MAJ-04 (SECURITY, D-04) — the sharpened raw-HTML section. Phase 38 merely proved HTML
// became inert ESCAPED text; Phase 39 proves the STRONGER pair of properties: raw HTML
// round-trips to its EXACT SOURCE BYTES *and* stays INERT. Byte-exactness comes from the
// preserved `raw` string attr; inertness is that the parsed JSON holds NO element node
// (no image/div/script/span/em) and NO executable attribute KEY (onerror/onclick/…) — the
// HTML lives only as characters inside the `raw` string, and `generateJSON`/DOM is never
// invoked (guaranteed by the node env: no window.DOMParser).
describe("tiptap markdown codec — raw-HTML byte-exact + inert (MAJ-04, D-04)", () => {
  // Re-assert the node-env guard here too: byte-exact preservation must NOT become a
  // byte-exact EXECUTION path. If jsdom ever supplied window.DOMParser, the built-in
  // parseHTMLToken DOM branch could re-open — this fails first.
  it("runs headless in a node env (no window, no document)", () => {
    expect(typeof globalThis.window).toBe("undefined");
    expect(typeof globalThis.document).toBe("undefined");
  });

  for (const [id, src] of Object.entries(SECURITY_CORPUS)) {
    describe(id, () => {
      it("round-trips raw HTML to its EXACT source bytes (byte-equality, not escaped)", () => {
        // The preserved node re-emits the source verbatim — angle brackets and all,
        // NOT entity-escaped. This is the Tier-2 byte contract applied to raw HTML.
        expect(roundTrip(src).trim()).toBe(src.trim());
      });

      it("stays inert: no element node, no executable attribute in the parsed JSON", () => {
        const { types, attrKeys } = collectTree(parse(roundTrip(src)));
        // Every emitted node type is allowed (bounded slice + preserved atoms) — the HTML
        // is carried by a preservedBlock/preservedInline `raw` string, never a live node.
        for (const t of types) expect(ALLOWED_NODE_TYPES[t]).toBe(true);
        // And none of the forbidden element node types appear (no image/div/script/…).
        for (const bad of FORBIDDEN_ELEMENT_TYPES) expect(types.has(bad)).toBe(false);
        // No live executable/DOM attribute KEY anywhere (only `raw` should carry bytes).
        for (const bad of EXECUTABLE_ATTRS) expect(attrKeys.has(bad)).toBe(false);
      });

      it("carries the HTML only as a preserved `raw` string attribute", () => {
        const { attrKeys } = collectTree(parse(roundTrip(src)));
        // The sole attribute the preserved atoms expose is `raw` — a plain string. No
        // schema-recognized HTML attribute was ever constructed.
        expect(attrKeys.has("raw")).toBe(true);
      });

      it("is idempotent (stable exact bytes on a second pass)", () => {
        const once = roundTrip(src);
        expect(roundTrip(once)).toBe(once);
      });
    });
  }

  it("keeps a dangerous <img onerror> as inert bytes, never a live image/attribute", () => {
    const out = roundTrip(SECURITY_CORPUS.imgOnerror);
    // The exact source bytes survive verbatim (byte-equality) — including the literal
    // `onerror` characters and the `<img` tag text...
    expect(out.trim()).toBe(SECURITY_CORPUS.imgOnerror.trim());
    expect(out).toContain("onerror");
    expect(out).toContain("<img");
    // ...but nothing live was built: the parsed JSON has no `image` node and no `onerror`
    // attribute key — the `<img …>` text is held inside a preserved `raw` string.
    const { types, attrKeys } = collectTree(parse(out));
    expect(types.has("image")).toBe(false);
    expect(attrKeys.has("onerror")).toBe(false);
    expect(attrKeys.has("raw")).toBe(true);
  });
});
