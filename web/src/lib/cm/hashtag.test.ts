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
