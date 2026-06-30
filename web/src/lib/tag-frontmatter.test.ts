// tag-frontmatter — readTags/setTags must be INDENT-AGNOSTIC and QUOTE-AWARE. The
// app emits a 2-space block, but the Go server re-marshals frontmatter via yaml.v3 at
// 4-SPACE indent whenever Repair adds a missing required field (internal/okf/emit.go),
// and a data-open / hand-authored file may use any valid indent or quote a tag. Two
// regressions are pinned here:
//   • a 4-space block read as EMPTY → Phase-22's reconcile corrupts it on first save;
//   • a quoted block tag read WITH quotes → never equals its inline form → churn + dup.
import { describe, it, expect } from "vitest";
import { readTags, setTags, unionTags } from "./tag-frontmatter";

describe("readTags — indent-agnostic block sequence", () => {
  it("reads a 2-space block (the app's own emit form)", () => {
    expect(readTags("title: T\ntags:\n  - deploy\n  - llm\n")).toEqual([
      "deploy",
      "llm",
    ]);
  });

  it("reads a 4-space block (server yaml.v3 re-marshal — the BLOCKER)", () => {
    expect(readTags("title: T\ntags:\n    - deploy\n    - llm\n")).toEqual([
      "deploy",
      "llm",
    ]);
  });

  it("reads a tab-indented block", () => {
    expect(readTags("tags:\n\t- deploy\n")).toEqual(["deploy"]);
  });
});

describe("readTags — quote-aware (block form strips quotes like flow form)", () => {
  it("strips double quotes from a block tag", () => {
    expect(readTags('tags:\n  - "deploy"\n')).toEqual(["deploy"]);
  });
  it("strips single quotes from a block tag", () => {
    expect(readTags("tags:\n  - 'deploy'\n")).toEqual(["deploy"]);
  });
  it("strips quotes from a 4-space block tag too", () => {
    expect(readTags('tags:\n    - "true"\n')).toEqual(["true"]);
  });
  it("flow sequence still strips quotes", () => {
    expect(readTags('tags: ["a", "b"]\n')).toEqual(["a", "b"]);
  });
});

describe("setTags — replaces a block at ANY indent without orphaning lines", () => {
  it("rewrites a 2-space block to canonical 2-space", () => {
    expect(setTags("title: T\ntags:\n  - old\n", ["a", "b"])).toBe(
      "title: T\ntags:\n  - a\n  - b\n",
    );
  });

  it("rewrites a 4-space block CLEANLY — no orphaned 4-space lines (the corruption)", () => {
    // Before the fix this produced `tags:\n  - a\n  - b\n    - old\n` — the orphaned
    // `    - old` then folded into a corrupt scalar by yaml.v3 on the server.
    expect(setTags("title: T\ntags:\n    - old\n    - x\n", ["a", "b"])).toBe(
      "title: T\ntags:\n  - a\n  - b\n",
    );
  });

  it("preserves following keys when replacing a 4-space block", () => {
    expect(
      setTags("tags:\n    - old\ndescription: hi\n", ["new"]),
    ).toBe("tags:\n  - new\ndescription: hi\n");
  });

  it("still appends a block when no tags key exists", () => {
    expect(setTags("title: T\n", ["a"])).toBe("title: T\ntags:\n  - a\n");
  });
});

describe("readTags ∘ setTags round-trip is indent-stable", () => {
  it("a 4-space block read then re-read through setTags yields the same set", () => {
    const fm = "title: T\ntags:\n    - deploy\n    - llm\n";
    const tags = readTags(fm); // ["deploy","llm"]
    const rewritten = setTags(fm, [...tags, "extra"]);
    expect(readTags(rewritten)).toEqual(["deploy", "llm", "extra"]);
    // No orphaned 4-space line survived the rewrite.
    expect(rewritten).not.toContain("    - ");
  });
});

// unionTags is the apply-tags reconcile merge (Phase 23): existing local tags first,
// then NEW applied tags, normalized (trim+lowercase) + de-duped case-insensitively.
// Matching reconcileFrontmatterTags's `norm` is what keeps an apply + the next
// body-save autosave from churning each other's bytes.
describe("unionTags — case-insensitive, order-preserving, normalized", () => {
  it("appends only NEW tags, preserving existing order", () => {
    expect(unionTags(["deploy", "llm"], ["fresh"])).toEqual([
      "deploy",
      "llm",
      "fresh",
    ]);
  });

  it("de-dupes case-insensitively (a re-applied tag is not duplicated)", () => {
    expect(unionTags(["Deploy"], ["deploy", "fresh"])).toEqual([
      "deploy",
      "fresh",
    ]);
  });

  it("normalizes to trimmed + lower-case (matches the body-save reconcile)", () => {
    expect(unionTags(["  Deploy  "], [" Fresh "])).toEqual(["deploy", "fresh"]);
  });

  it("drops empties and de-dupes within the applied set", () => {
    expect(unionTags([], ["a", "", "A", "b"])).toEqual(["a", "b"]);
  });
});
