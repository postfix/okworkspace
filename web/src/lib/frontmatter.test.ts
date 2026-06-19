import { describe, expect, it } from "vitest";

import { okfTitle, readField, setField } from "./frontmatter";

// extractTitleLine pulls the emitted `title:` line and returns the raw text
// after the colon+space, so we can assert the exact quoting shape setField
// produced.
function emittedTitle(value: string): string {
  const out = setField("", "title", value);
  const m = out.match(/^title:\s?(.*)$/m);
  if (!m) throw new Error(`no title line in:\n${out}`);
  return m[1];
}

describe("quoteIfNeeded (via setField) — CR-02 valid-YAML quoting", () => {
  // A plain, safe scalar must stay UNQUOTED (no over-quoting churn for the
  // common case).
  it("leaves a plain title unquoted", () => {
    expect(emittedTitle("My Page")).toBe("My Page");
    expect(emittedTitle("Deploy Staging 2")).toBe("Deploy Staging 2");
  });

  // Each of these would be INVALID or mis-read YAML if emitted unquoted, so the
  // value must be double-quoted.
  it.each([
    ["leading indicator [", "[Draft] Plan", '"[Draft] Plan"'],
    ["leading colon", ": leading colon", '": leading colon"'],
    ["trailing space", "trailing space ", '"trailing space "'],
    ["leading space", " leading space", '" leading space"'],
    ["reserved bool true", "true", '"true"'],
    ["reserved bool TRUE", "TRUE", '"TRUE"'],
    ["reserved null", "null", '"null"'],
    ["reserved yes", "yes", '"yes"'],
    ["numeric int", "123", '"123"'],
    ["numeric float", "1.5", '"1.5"'],
    ["leading dash", "- dash", '"- dash"'],
    ["colon+space map", "key: value", '"key: value"'],
    ["space+hash comment", "title # note", '"title # note"'],
    ["leading hash", "#tag", '"#tag"'],
    ["tilde null", "~", '"~"'],
    ["empty", "", '""'],
  ])("quotes %s", (_label, input, expected) => {
    expect(emittedTitle(input)).toBe(expected);
  });

  // Embedded quotes / backslashes are escaped so the emitted scalar stays a
  // valid double-quoted YAML string.
  it("escapes embedded double quotes", () => {
    expect(emittedTitle('say "hi"')).toBe('"say \\"hi\\""');
  });
  // A lone backslash is a legal plain YAML scalar, so it stays unquoted (valid
  // YAML); over-quoting is acceptable but not required here.
  it("leaves a lone backslash unquoted", () => {
    expect(emittedTitle("a\\b")).toBe("a\\b");
  });
  // When a backslash appears in a value that DOES need quoting, the backslash is
  // escaped so the double-quoted scalar stays valid.
  it("escapes backslashes inside a quoted value", () => {
    expect(emittedTitle('a\\b "c"')).toBe('"a\\\\b \\"c\\""');
  });
});

describe("setField / readField round-trip is preserved", () => {
  it("setField then readField returns the original plain value", () => {
    const fm = setField("type: Page\n", "title", "My Page");
    expect(readField(fm, "title")).toBe("My Page");
    expect(fm).toContain("type: Page");
  });

  it("readField strips quotes from a quoted scalar", () => {
    const fm = setField("", "title", "[Draft] Plan");
    // The on-disk shape is quoted; readField unwraps the single quote layer.
    expect(readField(fm, "title")).toBe("[Draft] Plan");
  });

  it("okfTitle reads a quoted title back", () => {
    const fm = setField("", "title", "key: value");
    expect(okfTitle(fm, "x/y.md")).toBe("key: value");
  });

  it("updating an existing field preserves other keys", () => {
    const fm = "type: Page\ntitle: Old\ntags: []\n";
    const out = setField(fm, "title", "New Title");
    expect(out).toContain("type: Page");
    expect(out).toContain("tags: []");
    expect(readField(out, "title")).toBe("New Title");
  });
});

describe("readField/setField escape the field name in RegExp (IN-02)", () => {
  // A field name containing regex metacharacters must be treated literally, not
  // compiled into a surprising pattern. These assertions would fail (or match
  // the wrong line) if the field were interpolated unescaped.
  it("reads a field whose name has regex metacharacters literally", () => {
    const fm = "a.b: hit\naxb: miss\n";
    // "." must match a literal dot, not any char — so "a.b" must not match "axb".
    expect(readField(fm, "a.b")).toBe("hit");
  });

  it("does not match an arbitrary line for a metacharacter field name", () => {
    const fm = "title: present\n";
    // ".*" as a field name must look for a literal ".*:" key, which is absent.
    expect(readField(fm, ".*")).toBe("");
  });

  it("setField updates the literal field, not a regex-matched one", () => {
    const fm = "a.b: old\naxb: keep\n";
    const out = setField(fm, "a.b", "new");
    expect(readField(out, "a.b")).toBe("new");
    expect(out).toContain("axb: keep");
  });
});
