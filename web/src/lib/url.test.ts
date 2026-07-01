import { describe, it, expect } from "vitest";
import { encodePagePath } from "./url";

describe("encodePagePath", () => {
  it("leaves a plain single-segment filename unchanged", () => {
    expect(encodePagePath("hello.md")).toBe("hello.md");
  });

  it("keeps the slashes literal in a nested path", () => {
    expect(encodePagePath("a/b/c.md")).toBe("a/b/c.md");
  });

  it("encodes spaces per segment (not the slashes)", () => {
    expect(encodePagePath("Folder/Sub/My Page.md")).toBe(
      "Folder/Sub/My%20Page.md",
    );
  });

  it("encodes a `#` so it never starts a URL fragment", () => {
    expect(encodePagePath("notes/C# Guide.md")).toBe("notes/C%23%20Guide.md");
  });

  it("encodes a `?` so it never starts a query string", () => {
    expect(encodePagePath("faq/What? Now.md")).toBe("faq/What%3F%20Now.md");
  });

  it("encodes a literal `%` so it never mis-decodes server-side", () => {
    expect(encodePagePath("stats/100%.md")).toBe("stats/100%25.md");
  });

  it("encodes unicode characters", () => {
    expect(encodePagePath("café/résumé.md")).toBe(
      "caf%C3%A9/r%C3%A9sum%C3%A9.md",
    );
  });

  it("does not double-encode an already-plain path", () => {
    const once = encodePagePath("docs/guide.md");
    expect(encodePagePath(once)).toBe(once);
  });

  it("handles an empty string", () => {
    expect(encodePagePath("")).toBe("");
  });

  it("preserves leading/trailing separators as empty segments", () => {
    expect(encodePagePath("a/")).toBe("a/");
    expect(encodePagePath("/a")).toBe("/a");
  });

  it("encodes each segment of a deeply nested path with mixed specials", () => {
    expect(encodePagePath("A B/C#D/E%F/g.md")).toBe("A%20B/C%23D/E%25F/g.md");
  });
});
