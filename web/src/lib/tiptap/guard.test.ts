// @vitest-environment node
//
// guard.test.ts — the LOAD-GUARD unit gate (RE-04, D-02). Pure codec logic, no DOM, so
// it runs under a `node` environment (the docblock OVERRIDES the app's global jsdom in
// vitest.config.ts) — mirroring codec.test.ts. This is also the D-04 security posture:
// the codec's no-HTML->DOM property holds only when `window.DOMParser` is absent, and the
// guard composes that same headless codec, so exercising it in node keeps the assertions
// honest (a jsdom switch would re-open the HTML->DOM path).
//
// The guard is the informed-consent boundary (the DECIDED fork resolution, D-02/D-03):
//   - Round-trippable content (the whole Tier-1/Tier-2 corpus)      -> true.
//   - Modeled-syntax normalization (`* ` bullets -> `- `, `__`/`_`
//     emphasis -> `**`/`*`, loose lists tightened)                 -> STILL true
//     (tree-equal; the one-time notice covers the byte reflow — NOT a byte-delta guard).
//   - Genuine content loss (a construct outside the bounded slice +
//     preserved tier that the codec drops/mangles) OR any throw     -> false
//     (a backstop: TipTap is made unavailable; PageEditor falls back to CM6).
import { describe, it, expect } from "vitest";
import { isLosslessRoundTrip } from "./guard";
import { roundTrip } from "./codec";
import {
  TIER1_CORPUS,
  TIER2_CORPUS,
  MIXED_CORPUS,
} from "./codec.corpus";

describe("isLosslessRoundTrip — round-trippable content passes (RE-04-2)", () => {
  // Every Tier-1 (tree-equal) + Tier-2 (byte-exact) corpus fixture is safe to open in
  // TipTap: prose, headings, lists, tables, `$math$`, blockquotes, code, raw HTML.
  const safe = { ...TIER1_CORPUS, ...TIER2_CORPUS, ...MIXED_CORPUS };
  for (const [id, src] of Object.entries(safe)) {
    it(`${id} -> true`, () => {
      expect(isLosslessRoundTrip(src)).toBe(true);
    });
  }

  it("a #tag body (literal text in the headless codec) -> true", () => {
    expect(isLosslessRoundTrip("a #tag here and #x/y too")).toBe(true);
  });

  it("empty string -> true", () => {
    expect(isLosslessRoundTrip("")).toBe(true);
  });
});

describe("isLosslessRoundTrip — modeled-syntax normalization still passes (informed consent, D-02/D-03)", () => {
  // The guard is TREE-equality, not byte-equality (the co-driver's decision — byte-delta
  // was the REJECTED option ii). A `*`/`+` bullet, `__`/`_` emphasis, or a loose list
  // reflows to the canonical spelling on save (`* `->`- `, `__`->`**`), but the TipTap tree
  // is identical, so the guard PASSES — the one-time consent notice informs the user those
  // bytes reflow for all viewers. If the guard were quietly byte-equality, these would
  // (wrongly) fall back to CM6 and cripple the feature; asserting they PASS is the
  // fork-honesty check.
  it("star bullets `* ` normalize to `- ` but stay tree-equal -> true", () => {
    const starBullets = "* one\n* two\n* three";
    // Sanity: the SERIALIZED bytes DO change (normalization happened)...
    expect(roundTrip(starBullets)).not.toBe(starBullets);
    // ...yet the guard passes because the parsed tree is identical (informed consent).
    expect(isLosslessRoundTrip(starBullets)).toBe(true);
  });

  it("plus bullets `+ ` normalize but stay tree-equal -> true", () => {
    expect(isLosslessRoundTrip("+ a\n+ b")).toBe(true);
  });

  it("underscore emphasis `__b__` / `_i_` normalize but stay tree-equal -> true", () => {
    const underscoreEmphasis = "This is __bold__ and _italic_ text.";
    expect(roundTrip(underscoreEmphasis)).not.toBe(underscoreEmphasis);
    expect(isLosslessRoundTrip(underscoreEmphasis)).toBe(true);
  });
});

describe("isLosslessRoundTrip — genuine loss / throw falls back (guard, not toast — BLK-03)", () => {
  // A Markdown IMAGE (`![alt](url)`) is OUT of the bounded slice — StarterKit omits the
  // Image node and no preserved-tier atom captures it — so parse drops it to bare `alt`
  // text. roundTrip therefore loses the image (verified below), tree-equality is false,
  // and the guard returns false: TipTap is made UNAVAILABLE for such a page (warn + fall
  // back to CM6), never entered-and-silently-lost. This is a REAL in-scope genuine-loss
  // fixture, not merely the throw backstop.
  it("a markdown image (unmodeled) -> false — the URL is dropped AND the tree diverges", () => {
    const image = "![alt](https://example.com/x.png)";
    // The round-trip genuinely loses the image URL (drops to bare alt text) — content
    // loss — and parse(src) even yields a schema-invalid doc>text vs the reparsed
    // doc>paragraph>text, so tree-equality is false and the guard falls back to CM6.
    expect(roundTrip(image).includes("example.com")).toBe(false);
    expect(isLosslessRoundTrip(image)).toBe(false);
  });

  it("an image with a title (unmodeled) -> false", () => {
    expect(
      isLosslessRoundTrip('![alt](https://example.com/x.png "title")'),
    ).toBe(false);
  });

  // The backstop: a codec throw is treated as unsafe (false). We can't manufacture a
  // parse throw from a plain string in-scope, so we prove the try/catch contract directly
  // by forcing the input past the string contract — a non-string coerces .substring inside
  // marked to throw, and the guard MUST swallow it to false rather than propagate.
  it("a codec throw returns false, never propagates (backstop)", () => {
    // `parse`/`roundTrip` take a string; a non-string trips marked's internal string ops.
    // The guard's try/catch must convert that into a safe `false`.
    const notAString = { toString() { throw new Error("boom"); } } as unknown as string;
    expect(isLosslessRoundTrip(notAString)).toBe(false);
  });
});
