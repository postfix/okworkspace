// tag-reconcile — the ED-02b materialization rule, golden-tested without React. The
// load-bearing case (criterion 1/5) is the GOLDEN test: add + remove + LLM-tag-persists
// in ONE reconcile call. Byte-stability (no setTags churn on an unchanged set) and
// case-collision folding are pinned alongside.
import { describe, it, expect } from "vitest";
import { reconcileFrontmatterTags } from "./tag-reconcile";
import { readTags } from "./tag-frontmatter";

describe("reconcileFrontmatterTags — GOLDEN (criterion 1/5)", () => {
  it("add + remove + LLM-tag-persists in ONE reconcile call", () => {
    // lastSavedBody had `#old` inline; the new body removed it and added `#deploy`.
    // Frontmatter carries `old` (inline-origin) AND `llm` (an LLM tag never inline).
    const lastSavedBody = "intro #old and more";
    const currentBody = "intro and more\nshipping #deploy";
    const frontmatter = "title: Note\ntags:\n  - old\n  - llm\n";

    const { frontmatter: out, changed } = reconcileFrontmatterTags({
      lastSavedBody,
      currentBody,
      frontmatter,
    });

    expect(changed).toBe(true);
    const tags = readTags(out);
    expect(tags).toContain("deploy"); // (a) ADDED from the body
    expect(tags).not.toContain("old"); // (b) REMOVED (was inline, deleted from body)
    expect(tags).toContain("llm"); // (c) LLM-only tag PERSISTS
    // Deterministic order: surviving fm tag(s) first, then inline-only appended.
    expect(tags).toEqual(["llm", "deploy"]);
  });
});

describe("reconcileFrontmatterTags — byte-stability (criterion 4)", () => {
  it("unchanged set → changed=false and frontmatter is BYTE-IDENTICAL", () => {
    const body = "see #deploy";
    const frontmatter = "title: T\ntags:\n  - deploy\n";
    const { frontmatter: out, changed } = reconcileFrontmatterTags({
      lastSavedBody: body,
      currentBody: body,
      frontmatter,
    });
    expect(changed).toBe(false);
    expect(out).toBe(frontmatter); // no setTags churn ⇒ no 409 thrash
  });

  it("unchanged FLOW-sequence frontmatter stays byte-identical (no flow→block churn)", () => {
    const body = "#a #b";
    const frontmatter = "tags: [a, b]\n";
    const { frontmatter: out, changed } = reconcileFrontmatterTags({
      lastSavedBody: body,
      currentBody: body,
      frontmatter,
    });
    expect(changed).toBe(false);
    expect(out).toBe(frontmatter); // setTags would fold to block form — guard prevents it
  });

  it("an existing-case fm tag is NOT rewritten when the set is unchanged (#Deploy body vs deploy fm)", () => {
    const body = "see #Deploy";
    const frontmatter = "tags:\n  - deploy\n";
    const { frontmatter: out, changed } = reconcileFrontmatterTags({
      lastSavedBody: body,
      currentBody: body,
      frontmatter,
    });
    expect(changed).toBe(false); // deploy === deploy (case-insensitive) ⇒ one tag, no change
    expect(out).toBe(frontmatter);
    expect(readTags(out)).toEqual(["deploy"]);
  });
});

describe("reconcileFrontmatterTags — LLM-only tag persistence", () => {
  it("an LLM tag (never inline) survives a save with no inline tags and causes NO churn", () => {
    const body = "just prose, no tags here";
    const frontmatter = "tags:\n  - llm\n";
    const { frontmatter: out, changed } = reconcileFrontmatterTags({
      lastSavedBody: body,
      currentBody: body,
      frontmatter,
    });
    expect(changed).toBe(false);
    expect(out).toBe(frontmatter);
    expect(readTags(out)).toContain("llm");
  });

  it("an LLM tag survives WHILE an inline tag is added in the same save", () => {
    const frontmatter = "tags:\n  - llm\n";
    const { frontmatter: out, changed } = reconcileFrontmatterTags({
      lastSavedBody: "no tags",
      currentBody: "added #feature",
      frontmatter,
    });
    expect(changed).toBe(true);
    const tags = readTags(out);
    expect(tags).toContain("llm");
    expect(tags).toContain("feature");
  });
});

describe("reconcileFrontmatterTags — additions and code-region safety", () => {
  it("adding #deploy to a body with NO existing tags: key appends a tags block", () => {
    const frontmatter = "title: Fresh\n";
    const { frontmatter: out, changed } = reconcileFrontmatterTags({
      lastSavedBody: "title only",
      currentBody: "now with #deploy",
      frontmatter,
    });
    expect(changed).toBe(true);
    expect(readTags(out)).toEqual(["deploy"]);
  });

  it("a #tag inside a fenced code block does NOT materialize (code-region aware)", () => {
    const frontmatter = "tags:\n  - keep\n";
    const { frontmatter: out, changed } = reconcileFrontmatterTags({
      lastSavedBody: "title only",
      currentBody: "```\n#secret\n```",
      frontmatter,
    });
    expect(changed).toBe(false); // #secret is inside code ⇒ not a tag ⇒ set unchanged
    expect(out).toBe(frontmatter);
    expect(readTags(out)).toEqual(["keep"]);
  });

  it("removing the LAST inline tag empties only that tag, keeping LLM tags", () => {
    const { frontmatter: out, changed } = reconcileFrontmatterTags({
      lastSavedBody: "had #gone here",
      currentBody: "had here", // #gone deleted
      frontmatter: "tags:\n  - gone\n  - llm\n",
    });
    expect(changed).toBe(true);
    expect(readTags(out)).toEqual(["llm"]);
  });
});

describe("reconcileFrontmatterTags — server-repaired (4-space) frontmatter (BLOCKER)", () => {
  // A repaired page is re-marshaled by yaml.v3 at 4-space. The reconcile must read it
  // correctly: an unchanged set is byte-identical (no corruption), and a real change is
  // a CLEAN canonical rewrite (no orphaned 4-space line folded into a scalar).
  it("4-space block + matching inline tag → no churn, byte-identical (no corruption)", () => {
    const fm = "title: T\ntags:\n    - deploy\n";
    const { frontmatter: out, changed } = reconcileFrontmatterTags({
      lastSavedBody: "see #deploy",
      currentBody: "see #deploy",
      frontmatter: fm,
    });
    expect(changed).toBe(false);
    expect(out).toBe(fm); // 4-space preserved verbatim — NOT rewritten/corrupted
  });

  it("4-space block + a newly added inline tag → clean 2-space rewrite, no orphan", () => {
    const { frontmatter: out, changed } = reconcileFrontmatterTags({
      lastSavedBody: "see #deploy",
      currentBody: "see #deploy #feature",
      frontmatter: "title: T\ntags:\n    - deploy\n",
    });
    expect(changed).toBe(true);
    expect(readTags(out)).toEqual(["deploy", "feature"]);
    expect(out).not.toContain("    - "); // no orphaned 4-space item line
  });
});
