// ED-02a — the pure `#tag` grammar, unit-tested WITHOUT an EditorView. The render
// path (Hashtag node → HashtagWidget) is corpus-pinned in livePreview.test.ts; here
// we lock the *grammar* in isolation through `parseInlineTags`, the pure string
// scanner that mirrors the Lezer inline parser's boundary/body rules byte-for-byte
// (so the Phase-22 client materialization can never drift from what renders).
//
// The NEGATIVE corpus is as load-bearing as the positive (CONTEXT D-02): every case
// that must NOT be a tag is asserted to extract nothing.
import { describe, it, expect } from "vitest";
import { parseInlineTags } from "./hashtag";

describe("parseInlineTags — positive corpus (ED-02a)", () => {
  it("extracts a tag after whitespace", () => {
    expect(parseInlineTags("see #deploy")).toEqual(["deploy"]);
  });

  it("extracts a tag at start-of-string", () => {
    expect(parseInlineTags("#deploy here")).toEqual(["deploy"]);
  });

  it("allows a nested #a/b tag (slash separates segments)", () => {
    expect(parseInlineTags("#a/b")).toEqual(["a/b"]);
  });

  it("extracts multiple tags, de-duped + lower-cased", () => {
    expect(parseInlineTags("#abc #fff")).toEqual(["abc", "fff"]);
    expect(parseInlineTags("#Deploy and #deploy")).toEqual(["deploy"]); // dedupe, fold case
  });

  it("treats #-hex-like tokens as tags (no hex special-casing — Obsidian parity)", () => {
    expect(parseInlineTags("#abc #fff")).toEqual(["abc", "fff"]);
  });

  it("keeps _ and - in the body", () => {
    expect(parseInlineTags("#a_b-c")).toEqual(["a_b-c"]);
  });
});

describe("parseInlineTags — negative corpus (ED-02a)", () => {
  it("rejects an all-digit run (#1984)", () => {
    expect(parseInlineTags("#1984")).toEqual([]);
  });

  it("rejects mid-word (foo#bar)", () => {
    expect(parseInlineTags("foo#bar")).toEqual([]);
  });

  it("rejects a #-after-slash URL fragment (a/#frag)", () => {
    expect(parseInlineTags("a/#frag")).toEqual([]);
  });

  it("rejects an escaped \\#esc (preceding char is a backslash, not whitespace)", () => {
    expect(parseInlineTags("\\#esc")).toEqual([]);
  });

  it("rejects a bare '#'", () => {
    expect(parseInlineTags("#")).toEqual([]);
  });

  it("rejects '# ' (hash then space — ATX-marker shape)", () => {
    expect(parseInlineTags("# Heading")).toEqual([]);
  });

  it("rejects a digits-and-slashes-only run (#12/34)", () => {
    expect(parseInlineTags("#12/34")).toEqual([]);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Phase-21 ADVERSARIAL corpus (folded in from the adversarial-tester gate). Every
// case was RUN against the real `parseInlineTags` AND cross-checked against the
// Lezer render path (livePreview.test.ts) so the pinned value is observed truth,
// not a guess. Cases are split: "surprising-but-correct" (per the stated grammar)
// get a passing pin + a why-comment; genuine render divergences are flagged in
// livePreview.test.ts (where both consumers are reachable).
// ─────────────────────────────────────────────────────────────────────────────

describe("parseInlineTags — body/slash edge cases (Phase 21)", () => {
  // The body alphabet is [A-Za-z0-9_/-]; the >=1-non-digit rule treats '-' and '_'
  // as non-digits (so a bare `#-`/`#_` qualifies) but NOT '/' (a separator). These
  // also pin the code-reviewer's flagged boundaries (#-, #_, #a/) as intended.
  it("a leading slash is body, kept: #/a → /a", () => {
    expect(parseInlineTags("#/a")).toEqual(["/a"]);
  });
  it("a single '-' qualifies (dash is a non-digit body char): #-1 → -1", () => {
    expect(parseInlineTags("#-1")).toEqual(["-1"]);
  });
  it("a bare underscore is a tag (underscore is a non-digit): #_ → _", () => {
    expect(parseInlineTags("#_")).toEqual(["_"]);
  });
  it("a bare dash is a tag: #- → -", () => {
    expect(parseInlineTags("#-")).toEqual(["-"]);
  });
  it("consecutive slashes are kept (only a TRAILING slash is trimmed): #a//b → a//b", () => {
    expect(parseInlineTags("#a//b")).toEqual(["a//b"]);
  });
  it("a single trailing slash is trimmed: #a/ → a", () => {
    expect(parseInlineTags("#a/")).toEqual(["a"]);
  });
  it("multiple trailing slashes are all trimmed, leaving a non-empty body: #A/B/ → a/b", () => {
    expect(parseInlineTags("#A/B/")).toEqual(["a/b"]);
  });
  it("a deep nested path is kept: #a/b/c → a/b/c", () => {
    expect(parseInlineTags("#a/b/c")).toEqual(["a/b/c"]);
  });
  it("an all-slash run has no non-digit AND trims to empty → rejected: #/// → []", () => {
    expect(parseInlineTags("#///")).toEqual([]);
  });
  it("trailing ASCII punctuation is not body, so it terminates the tag: #tag. → tag", () => {
    expect(parseInlineTags("#tag.")).toEqual(["tag"]);
    expect(parseInlineTags("#tag,")).toEqual(["tag"]);
    expect(parseInlineTags("#tag!")).toEqual(["tag"]);
  });
  it("a '.' inside terminates the body (not in the alphabet): #a.b → a", () => {
    expect(parseInlineTags("#a.b")).toEqual(["a"]);
  });
  it("hex-like tokens are tags, lower-cased: #0xFF → 0xff", () => {
    expect(parseInlineTags("#0xFF")).toEqual(["0xff"]);
  });
});

describe("parseInlineTags — start-boundary edge cases (Phase 21)", () => {
  // The ONLY valid start boundary is start-of-string OR a whitespace char. Every
  // other preceding character (punctuation, a multi-byte char, a block marker)
  // rejects — this is the load-bearing negative rule.
  it("rejects after an opening paren: (#tag → []", () => {
    expect(parseInlineTags("(#tag)")).toEqual([]);
  });
  it("rejects after a comma: ,#tag → []", () => {
    expect(parseInlineTags(",#tag")).toEqual([]);
  });
  it('rejects after a double-quote: "#tag → []', () => {
    expect(parseInlineTags('"#tag')).toEqual([]);
  });
  it("rejects after an em-dash (U+2014 is not whitespace): a —#tag → []", () => {
    expect(parseInlineTags("a —#tag")).toEqual([]);
    expect(parseInlineTags("—#tag")).toEqual([]);
  });
  it("rejects after a '>' in the RAW string (parseInlineTags has no block sense): >#deploy → []", () => {
    // NOTE: the RENDER badges this (the Lezer parser sees blockquote-content start as
    // a valid '#' boundary). parseInlineTags, scanning the raw string, sees '>' before
    // '#' and rejects. This documented divergence is pinned in livePreview.test.ts
    // ("Hashtag DRIFT") as a Phase-22 reconciliation item.
    expect(parseInlineTags(">#deploy")).toEqual([]);
  });
  it("rejects ### + word (no body char follows the first '#'): ###tag → []", () => {
    expect(parseInlineTags("###tag")).toEqual([]);
  });
  it("rejects after '.' in a list-like prefix: 1.#tag → []", () => {
    expect(parseInlineTags("1.#tag")).toEqual([]);
  });
  it("accepts after a TAB (tab is whitespace): a\\t#deploy → deploy", () => {
    expect(parseInlineTags("a\t#deploy")).toEqual(["deploy"]);
  });
  it("accepts after a newline (a soft break is whitespace): a\\n#deploy → deploy", () => {
    expect(parseInlineTags("a\n#deploy")).toEqual(["deploy"]);
  });
});

describe("parseInlineTags — Unicode boundary + body (Phase 21)", () => {
  // The scanner is code-UNIT based (charCodeAt). The body alphabet is ASCII-only, so
  // any multi-byte char terminates the body; a multi-byte char BEFORE '#' is not
  // whitespace, so it rejects. Surrogate pairs must not corrupt the index walk.
  it("a non-ASCII letter terminates the ASCII-only body: #café → caf", () => {
    expect(parseInlineTags("#café")).toEqual(["caf"]);
  });
  it("rejects when a non-ASCII letter (single code unit) precedes '#': é#tag → []", () => {
    expect(parseInlineTags("é#tag")).toEqual([]);
  });
  it("rejects when a surrogate-pair emoji (2 code units) precedes '#': 😀#tag → []", () => {
    // The low surrogate is the immediate predecessor code unit — not whitespace.
    expect(parseInlineTags("😀#tag")).toEqual([]);
  });
  it("a valid tag AFTER a surrogate-pair char + space is still found (index walk intact): 😀 #deploy → deploy", () => {
    expect(parseInlineTags("😀 #deploy")).toEqual(["deploy"]);
  });
});

describe("parseInlineTags — KNOWN scope gap: does NOT skip code regions (Phase 21, documented)", () => {
  // hashtag.ts SCOPE NOTE: parseInlineTags has no block structure to consult, so it
  // does NOT exclude code regions (the render does, structurally, via the Lezer tree).
  // This is documented + accepted for Phase 21 (the function is unused in the render
  // path) and deferred to Phase 22. Pinned here so the gap is visible, not silent.
  it("extracts a tag from inside an indented code block (render does NOT badge it): \\t#deploy → deploy", () => {
    expect(parseInlineTags("\t#deploy")).toEqual(["deploy"]);
  });
  it("extracts a tag from inside a fenced block: ```\\n#x\\n``` → x", () => {
    expect(parseInlineTags("```\n#x\n```")).toEqual(["x"]);
  });
  it("extracts a whitespace-preceded tag inside an inline-code span (render does NOT): `x #code` → code", () => {
    // The render leaves InlineCode untouched (no badge); parseInlineTags has no code
    // awareness and sees the space before '#', so it extracts. Documented scope gap.
    // (NB: `#code` with the backtick directly before '#' is rejected by the boundary
    // rule, so that particular shape happens to AGREE with the render — see the
    // negative corpus. The gap only shows when whitespace precedes the '#' in-span.)
    expect(parseInlineTags("`x #code`")).toEqual(["code"]);
  });
});
