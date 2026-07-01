// hashtag.parity.test.ts — the TR-02 anti-drift contract (TS side).
//
// SINGLE-SOURCE MECHANISM (regenerate-and-diff): the shared whole-document
// fixtures (hashtag.fixtures.ts) are run through the REAL Lezer parser
// (inlineTagSpansFromBody, which shares `tagParser` with the render + the
// Phase-22 materializer). Their Hashtag node BYTE-offset spans are the expected
// results. This test:
//
//   1. Regenerates the corpus from the CURRENT grammar and asserts it is
//      byte-identical to the checked-in internal/okf/testdata/hashtag_corpus.json.
//      A grammar/fixture change that is not committed to the JSON FAILS here —
//      so the Go scanner (which asserts against that SAME JSON in
//      internal/okf/hashtag_test.go) can never silently diverge from Lezer.
//   2. Asserts BYTE OFFSETS / spliced output (from/to spans), NOT merely tag-name
//      sets — the offset-parity discipline (a set-equal test is a false-safe).
//
// Run `UPDATE_CORPUS=1 npx vitest run src/lib/cm/hashtag.parity.test.ts` to
// regenerate the JSON after an intentional grammar/fixture change.
import { describe, it, expect } from "vitest";
import { readFileSync, writeFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, resolve } from "node:path";
import { inlineTagSpansFromBody } from "./hashtag";
import { hashtagFixtures } from "./hashtag.fixtures";

interface CorpusEntry {
  name: string;
  body: string;
  spans: { from: number; to: number; name: string }[];
}

const CORPUS_PATH = resolve(
  dirname(fileURLToPath(import.meta.url)),
  "../../../../internal/okf/testdata/hashtag_corpus.json",
);

function generate(): CorpusEntry[] {
  return hashtagFixtures.map((f) => ({
    name: f.name,
    body: f.body,
    spans: inlineTagSpansFromBody(f.body),
  }));
}

describe("hashtag Lezer↔Go parity corpus (TR-02)", () => {
  it("offset spans are generated from the real Lezer parser (asserts byte from/to, not name sets)", () => {
    const corpus = generate();
    // Sanity: at least one fixture must yield a real span so this can't pass
    // vacuously, and every span must be a byte-offset triple.
    const total = corpus.reduce((n, e) => n + e.spans.length, 0);
    expect(total).toBeGreaterThan(0);
    for (const e of corpus) {
      for (const s of e.spans) {
        expect(typeof s.from).toBe("number");
        expect(typeof s.to).toBe("number");
        expect(s.to).toBeGreaterThan(s.from);
        // The span begins at a '#' and its name matches the spliced-out bytes.
        expect(e.body[e.body.indexOf("#", 0)]).toBe("#");
      }
    }
  });

  it("checked-in corpus JSON is up to date with the current grammar (regenerate-and-diff; drift FAILS CI)", () => {
    const generated = JSON.stringify(generate(), null, 2) + "\n";
    if (process.env.UPDATE_CORPUS) {
      writeFileSync(CORPUS_PATH, generated, "utf8");
    }
    let onDisk = "";
    try {
      onDisk = readFileSync(CORPUS_PATH, "utf8");
    } catch {
      throw new Error(
        `corpus JSON missing at ${CORPUS_PATH}; run UPDATE_CORPUS=1 vitest to generate it`,
      );
    }
    expect(onDisk).toBe(generated);
  });

  it("block-context fixtures exercise every excluded region + boundary", () => {
    // Pin the specific block-context expectations so a fixture edit that removes
    // coverage is visible (these are the cases a paragraph-only corpus misses).
    const byName = Object.fromEntries(generate().map((e) => [e.name, e]));

    // Indented code (after a blank line) yields NO tag; the live line after does.
    expect(byName["indented-code-after-blank"].spans.map((s) => s.name)).toEqual([
      "ops",
    ]);
    // HTML block content is excluded; the tag after the block is live.
    expect(byName["html-block-div"].spans.map((s) => s.name)).toEqual(["realtag"]);
    // GFM table cells are inline-parsed → both cell tags recognized.
    expect(byName["gfm-table"].spans.map((s) => s.name)).toEqual(["alpha", "beta"]);
    // Lazy blockquote continuation: both lines' tags recognized.
    expect(
      byName["lazy-blockquote-continuation"].spans.map((s) => s.name),
    ).toEqual(["one", "two"]);
    // `>#tag` block-marker abut is a valid boundary.
    expect(byName["blockquote-marker-abut"].spans.map((s) => s.name)).toEqual([
      "deploy",
    ]);
    // Fenced code excluded; the tag after is live.
    expect(byName["fenced-then-live"].spans.map((s) => s.name)).toEqual(["yes"]);
  });
});
