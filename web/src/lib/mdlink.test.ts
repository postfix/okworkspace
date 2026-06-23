import { describe, expect, it } from "vitest";

import { resolveRelativeMdLink } from "./mdlink";

describe("resolveRelativeMdLink — WR-02 relative .md resolution", () => {
  it.each([
    // same-dir bare link
    ["docs/guide.md", "page2.md", "docs/page2.md"],
    // explicit same-dir
    ["docs/guide.md", "./page2.md", "docs/page2.md"],
    // one up
    ["docs/guide.md", "../runbooks/deploy.md", "runbooks/deploy.md"],
    // two up from depth 2
    ["a/b/guide.md", "../../runbooks/deploy.md", "runbooks/deploy.md"],
    // root page same-dir
    ["guide.md", "other.md", "other.md"],
    // absolute workspace path resolves from root
    ["docs/deep/guide.md", "/runbooks/deploy.md", "runbooks/deploy.md"],
    // nested down from same dir
    ["docs/guide.md", "./sub/child.md", "docs/sub/child.md"],
  ])("resolve(%j, %j) === %j", (from, href, expected) => {
    expect(resolveRelativeMdLink(from, href)).toBe(expected);
  });

  it("clamps `..` at root and never leaves a leading ../", () => {
    const out = resolveRelativeMdLink("guide.md", "../../x.md");
    expect(out).toBe("x.md");
    expect(out).not.toMatch(/^\.\.\//);
  });

  it("clamps excess `..` from a nested page", () => {
    const out = resolveRelativeMdLink("a/b/guide.md", "../../../../x.md");
    expect(out).toBe("x.md");
    expect(out).not.toMatch(/^\.\.\//);
  });

  it.each([
    ["docs/guide.md", "http://example.com/x.md"],
    ["docs/guide.md", "https://example.com/x.md"],
    ["docs/guide.md", "mailto:a@b.com"],
    ["docs/guide.md", "//cdn.example.com/x.md"],
  ])("returns null for external link %j -> %j", (from, href) => {
    expect(resolveRelativeMdLink(from, href)).toBeNull();
  });

  it("returns null for non-.md targets", () => {
    expect(resolveRelativeMdLink("docs/guide.md", "image.png")).toBeNull();
    expect(resolveRelativeMdLink("docs/guide.md", "../assets/file.pdf")).toBeNull();
  });

  it("returns null for empty/undefined href", () => {
    expect(resolveRelativeMdLink("docs/guide.md", undefined)).toBeNull();
    expect(resolveRelativeMdLink("docs/guide.md", "")).toBeNull();
  });

  it("carries a trailing #fragment through to the resolved path", () => {
    expect(resolveRelativeMdLink("docs/guide.md", "notes.md#section")).toBe(
      "docs/notes.md#section",
    );
    expect(
      resolveRelativeMdLink("a/b/guide.md", "../../runbooks/deploy.md#step-2"),
    ).toBe("runbooks/deploy.md#step-2");
  });
});
