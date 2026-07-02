// @vitest-environment node
//
// parity.test.ts — CROSS-ENGINE parity gate (RE-01 crit-3, D-03, MAJ-02 the LOCKED
// invariant). Runs under `node` (same rationale as codec.test.ts).
//
// TipTap's own round-trip only proves TipTap-internal idempotence. The project's LOCKED
// invariant (markdown.ts:1-8) is that the editor's parse set MUST agree with the CM6
// read/live-preview path — and @tiptap/markdown is MarkedJS-backed (marked@17), a FOURTH
// Markdown engine beside Goldmark, CM6 @lezer/markdown, and read-mode remark-gfm. So we
// re-parse the codec's serialized output through the SAME @lezer/markdown GFM config the
// CM6 editor uses and assert the Lezer node-type structure matches the source's.
//
// The GFM extension set is [Table, Strikethrough, TaskList, Autolink] over the
// commonmark base — mirrored EXACTLY from markdown.ts / hashtag.ts (tagParserExtensions
// minus the app-specific hashtagExtension, which is not standard Markdown). We do NOT
// invent a different GFM set: a divergence must reflect a REAL structural change, not a
// parser-config mismatch.
//
// A benign marker normalization (e.g. `*` bullets -> `-`) passes because both parse to
// the same Lezer structure (ListMark). A structural divergence (TipTap serializes such
// that the app's OWN parser reads a different tree) is a real finding — asserted here,
// surfaced to the GO/NO-GO report, never swallowed.
import { describe, it, expect } from "vitest";
import { parser, Table, Strikethrough, TaskList, Autolink } from "@lezer/markdown";
import type { Tree } from "@lezer/common";
import { roundTrip } from "./codec";
import { TIER1_CORPUS } from "./codec.corpus";

// gfmParser mirrors the CM6 editor's Lezer config (markdown.ts:33-37 composes the same
// four GFM extensions over commonmark via @codemirror/lang-markdown, which wraps this
// very @lezer/markdown parser). commonmark is @lezer/markdown's default base, so
// parser.configure([...GFM]) === the base the CM6 `commonmarkLanguage` wraps.
const gfmParser = parser.configure([Table, Strikethrough, TaskList, Autolink]);

// nodeTypeSequence walks a Lezer tree in document order and returns the flat sequence of
// node-type names (positions ignored). Two Markdown strings that parse to the same
// sequence have the same structure under the CM6 parser — which is exactly the parity
// property we need (marker bytes may differ; structure must not).
function nodeTypeSequence(tree: Tree): string[] {
  const seq: string[] = [];
  const cursor = tree.cursor();
  do {
    seq.push(cursor.name);
  } while (cursor.next());
  return seq;
}

// lezerStructure parses a Markdown source with the CM6 GFM config and returns its
// node-type sequence — the comparable structural fingerprint. Reused for both the source
// and the round-trip output (a real shared comparator; Phases 39/41 extend the corpus).
function lezerStructure(md: string): string[] {
  return nodeTypeSequence(gfmParser.parse(md));
}

describe("tiptap codec — cross-engine parity vs CM6 @lezer/markdown (MAJ-02)", () => {
  for (const [id, src] of Object.entries(TIER1_CORPUS)) {
    it(`${id}: serialize(parse(src)) re-parses to the source's Lezer structure`, () => {
      const sourceStructure = lezerStructure(src);
      const roundTripStructure = lezerStructure(roundTrip(src));
      // The app's OWN Markdown parser sees the SAME structure for the codec's output as
      // it did for the source — TipTap did not normalize in a way that changes how the
      // rest of the app parses the bytes.
      expect(roundTripStructure).toEqual(sourceStructure);
    });
  }
});
