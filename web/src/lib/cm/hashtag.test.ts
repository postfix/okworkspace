// ED-02a/ED-02b — the `#tag` grammar, exercised through `inlineTagsFromBody` (the
// LIVE Phase-22 materialization function). It runs the SAME Lezer extension set the
// CM6 render uses, so testing the grammar through it also pins the render⇄materialize
// AGREEMENT: a case that does not badge on screen extracts nothing here, and vice
// versa. (Phase 21 tested the now-retired pure `parseInlineTags` string scanner; this
// suite supersedes it and CLOSES its two documented divergences — code regions and
// block-marker boundaries — by routing through the structural Lezer parse.)
//
// The NEGATIVE corpus is as load-bearing as the positive (CONTEXT D-02): every case
// that must NOT be a tag is asserted to extract nothing.
import { describe, it, expect } from "vitest";
import { inlineTagsFromBody } from "./hashtag";

describe("inlineTagsFromBody — positive corpus (ED-02a)", () => {
  it("extracts a tag after whitespace", () => {
    expect(inlineTagsFromBody("see #deploy")).toEqual(["deploy"]);
  });

  it("extracts a tag at start-of-string", () => {
    expect(inlineTagsFromBody("#deploy here")).toEqual(["deploy"]);
  });

  it("allows a nested #a/b tag (slash separates segments)", () => {
    expect(inlineTagsFromBody("#a/b")).toEqual(["a/b"]);
  });

  it("extracts multiple tags, de-duped + lower-cased", () => {
    expect(inlineTagsFromBody("#abc #fff")).toEqual(["abc", "fff"]);
    expect(inlineTagsFromBody("#Deploy and #deploy")).toEqual(["deploy"]); // dedupe, fold case
  });

  it("treats #-hex-like tokens as tags (no hex special-casing — Obsidian parity)", () => {
    expect(inlineTagsFromBody("#abc #fff")).toEqual(["abc", "fff"]);
  });

  it("keeps _ and - in the body", () => {
    expect(inlineTagsFromBody("#a_b-c")).toEqual(["a_b-c"]);
  });
});

describe("inlineTagsFromBody — negative corpus (ED-02a)", () => {
  it("rejects an all-digit run (#1984)", () => {
    expect(inlineTagsFromBody("#1984")).toEqual([]);
  });

  it("rejects mid-word (foo#bar)", () => {
    expect(inlineTagsFromBody("foo#bar")).toEqual([]);
  });

  it("rejects a #-after-slash URL fragment (a/#frag)", () => {
    expect(inlineTagsFromBody("a/#frag")).toEqual([]);
  });

  it("rejects an escaped \\#esc (Escape consumes the '#' before the tag parser)", () => {
    expect(inlineTagsFromBody("\\#esc")).toEqual([]);
  });

  it("rejects a bare '#'", () => {
    expect(inlineTagsFromBody("#")).toEqual([]);
  });

  it("rejects '# ' (hash then space — ATX-marker shape)", () => {
    expect(inlineTagsFromBody("# Heading")).toEqual([]);
  });

  it("rejects a digits-and-slashes-only run (#12/34)", () => {
    expect(inlineTagsFromBody("#12/34")).toEqual([]);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Phase-21 ADVERSARIAL corpus (folded in from the adversarial-tester gate), now run
// through the live Lezer materializer. Every case is cross-checked against the render
// (livePreview.test.ts) so the pinned value is observed truth, not a guess. The body/
// slash/boundary cases below are plain-paragraph strings, so the structural Lezer
// parse reduces to the same body alphabet + whitespace start rule the grammar defines.
// ─────────────────────────────────────────────────────────────────────────────

describe("inlineTagsFromBody — body/slash edge cases (Phase 21)", () => {
  // The body alphabet is [A-Za-z0-9_/-]; the >=1-non-digit rule treats '-' and '_'
  // as non-digits (so a bare `#-`/`#_` qualifies) but NOT '/' (a separator).
  it("a leading slash is body, kept: #/a → /a", () => {
    expect(inlineTagsFromBody("#/a")).toEqual(["/a"]);
  });
  it("a single '-' qualifies (dash is a non-digit body char): #-1 → -1", () => {
    expect(inlineTagsFromBody("#-1")).toEqual(["-1"]);
  });
  it("a bare underscore is a tag (underscore is a non-digit): #_ → _", () => {
    expect(inlineTagsFromBody("#_")).toEqual(["_"]);
  });
  it("a bare dash is a tag: #- → -", () => {
    expect(inlineTagsFromBody("#-")).toEqual(["-"]);
  });
  it("consecutive slashes are kept (only a TRAILING slash is trimmed): #a//b → a//b", () => {
    expect(inlineTagsFromBody("#a//b")).toEqual(["a//b"]);
  });
  it("a single trailing slash is trimmed: #a/ → a", () => {
    expect(inlineTagsFromBody("#a/")).toEqual(["a"]);
  });
  it("multiple trailing slashes are all trimmed, leaving a non-empty body: #A/B/ → a/b", () => {
    expect(inlineTagsFromBody("#A/B/")).toEqual(["a/b"]);
  });
  it("a deep nested path is kept: #a/b/c → a/b/c", () => {
    expect(inlineTagsFromBody("#a/b/c")).toEqual(["a/b/c"]);
  });
  it("an all-slash run has no non-digit AND trims to empty → rejected: #/// → []", () => {
    expect(inlineTagsFromBody("#///")).toEqual([]);
  });
  it("trailing ASCII punctuation is not body, so it terminates the tag: #tag. → tag", () => {
    expect(inlineTagsFromBody("#tag.")).toEqual(["tag"]);
    expect(inlineTagsFromBody("#tag,")).toEqual(["tag"]);
    expect(inlineTagsFromBody("#tag!")).toEqual(["tag"]);
  });
  it("a '.' inside terminates the body (not in the alphabet): #a.b → a", () => {
    expect(inlineTagsFromBody("#a.b")).toEqual(["a"]);
  });
  it("hex-like tokens are tags, lower-cased: #0xFF → 0xff", () => {
    expect(inlineTagsFromBody("#0xFF")).toEqual(["0xff"]);
  });
});

describe("inlineTagsFromBody — start-boundary edge cases (Phase 21)", () => {
  // The ONLY valid start boundary is start-of-section OR a whitespace char. Every
  // other preceding character (punctuation, a multi-byte char) rejects.
  it("rejects after an opening paren: (#tag → []", () => {
    expect(inlineTagsFromBody("(#tag)")).toEqual([]);
  });
  it("rejects after a comma: ,#tag → []", () => {
    expect(inlineTagsFromBody(",#tag")).toEqual([]);
  });
  it('rejects after a double-quote: "#tag → []', () => {
    expect(inlineTagsFromBody('"#tag')).toEqual([]);
  });
  it("rejects after an em-dash (U+2014 is not whitespace): a —#tag → []", () => {
    expect(inlineTagsFromBody("a —#tag")).toEqual([]);
    expect(inlineTagsFromBody("—#tag")).toEqual([]);
  });
  it("badges a block-marker-abutting '#' (>#deploy → deploy — now AGREES with the render)", () => {
    // CLOSED Phase-21 divergence: the render always treated blockquote-content start as
    // a valid '#' boundary (via cx.offset); the retired raw-string scanner saw the
    // literal '>' before '#' and rejected. inlineTagsFromBody runs the structural Lezer
    // parse, so '>' is a blockquote marker and '#deploy' is section-start content → a tag.
    expect(inlineTagsFromBody(">#deploy")).toEqual(["deploy"]);
  });
  it("rejects ### + word (no body char follows the first '#'): ###tag → []", () => {
    expect(inlineTagsFromBody("###tag")).toEqual([]);
  });
  it("rejects after '.' in a list-like prefix: 1.#tag → []", () => {
    expect(inlineTagsFromBody("1.#tag")).toEqual([]);
  });
  it("accepts after a MID-LINE tab (tab is whitespace): a\\t#deploy → deploy", () => {
    expect(inlineTagsFromBody("a\t#deploy")).toEqual(["deploy"]);
  });
  it("accepts after a soft line break (newline boundary): a\\n#deploy → deploy", () => {
    expect(inlineTagsFromBody("a\n#deploy")).toEqual(["deploy"]);
  });
});

describe("inlineTagsFromBody — Unicode boundary + body (Phase 21)", () => {
  // The body alphabet is ASCII-only, so any multi-byte char terminates the body; a
  // multi-byte char BEFORE '#' is not whitespace, so it rejects. Surrogate pairs must
  // not corrupt the index walk.
  it("a non-ASCII letter terminates the ASCII-only body: #café → caf", () => {
    expect(inlineTagsFromBody("#café")).toEqual(["caf"]);
  });
  it("rejects when a non-ASCII letter (single code unit) precedes '#': é#tag → []", () => {
    expect(inlineTagsFromBody("é#tag")).toEqual([]);
  });
  it("rejects when a surrogate-pair emoji (2 code units) precedes '#': 😀#tag → []", () => {
    expect(inlineTagsFromBody("😀#tag")).toEqual([]);
  });
  it("a valid tag AFTER a surrogate-pair char + space is still found (index walk intact): 😀 #deploy → deploy", () => {
    expect(inlineTagsFromBody("😀 #deploy")).toEqual(["deploy"]);
  });
});

describe("inlineTagsFromBody — code-region aware (Phase-22 closes the Phase-21 scope gap)", () => {
  // The retired pure scanner had no block structure and WRONGLY extracted a `#tag` from
  // inside code. inlineTagsFromBody runs the structural Lezer parse, which never offers
  // a '#' inside InlineCode/FencedCode/IndentedCode to the inline parser — so these now
  // agree with the render (no badge ⇒ no materialized tag). This is the ED-02b hard bar.
  it("does NOT extract from inside an indented code block (leading tab): \\t#deploy → []", () => {
    expect(inlineTagsFromBody("\t#deploy")).toEqual([]);
  });
  it("does NOT extract from inside a fenced block: ```\\n#x\\n``` → []", () => {
    expect(inlineTagsFromBody("```\n#x\n```")).toEqual([]);
  });
  it("does NOT extract from inside an inline-code span: `x #code` → []", () => {
    expect(inlineTagsFromBody("`x #code`")).toEqual([]);
  });
  it("does NOT extract directly after an inline-code span (backtick precedes): `x`#tag → []", () => {
    expect(inlineTagsFromBody("`x`#tag")).toEqual([]);
  });
});
