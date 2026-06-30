// tag-reconcile — ADVERSARIAL complement (independently designed). The author's
// tag-reconcile.test.ts pins the golden add+remove+persist path, fenced-code safety,
// and a few byte-stability cases. This file attacks the partition the author's tests
// share a blind spot on: the BYTE-STABILITY contract under YAML forms the app does
// NOT itself emit but a data-open file (or yaml.v3 on the backend) WILL — quoted
// block tags, scalars, empty/no-key — plus the code-region boundary corpus, the
// diff-window across saves, and the universally-quantified properties the autosave
// loop depends on (idempotence / loop-termination, LLM-tag persistence,
// changed⇒rewrite, order-independence).
//
// ORACLES used here:
//   • byte-stability: the stated contract — !changed ⇒ frontmatter returned VERBATIM.
//   • code-region: the negative corpus (PITFALLS.md / hashtag.ts header) — these `#`
//     are NOT tags; the reconcile must therefore see the set as unchanged.
//   • idempotence/order: metamorphic relations the loop relies on (no oracle for the
//     "right" tag set, but reconcile∘reconcile and permuted-input must agree).
import { describe, it, expect } from "vitest";
import { reconcileFrontmatterTags } from "./tag-reconcile";
import { readTags } from "./tag-frontmatter";

const rec = (lastSavedBody: string, currentBody: string, frontmatter: string) =>
  reconcileFrontmatterTags({ lastSavedBody, currentBody, frontmatter });
// A "settled" reconcile: the body has not changed since the last save, so a correct
// reconcile is a pure no-op (the autosave/loop case the byte-stability bar protects).
const settled = (body: string, frontmatter: string) => rec(body, body, frontmatter);

describe("byte-stability — QUOTED block tags (DEFECT regression: readTags block form must strip quotes like the flow form)", () => {
  // ROOT CAUSE this pins: readTags' block branch returned `"deploy"` (quotes kept)
  // while the flow branch strips them, so a quoted block tag never equalled its inline
  // `#deploy` form → the reconcile fired changed=true and rewrote the frontmatter,
  // appending a DUPLICATE `- deploy` on every save (byte-stability + 409-thrash bug).
  it("double-quoted block tag matching an inline tag → no churn, byte-identical", () => {
    const fm = 'tags:\n  - "deploy"\n';
    const { frontmatter: out, changed } = settled("see #deploy", fm);
    expect(changed).toBe(false);
    expect(out).toBe(fm); // quotes preserved on disk, no duplicate appended
  });

  it("single-quoted block tag matching an inline tag → no churn", () => {
    const fm = "tags:\n  - 'deploy'\n";
    const { frontmatter: out, changed } = settled("see #deploy", fm);
    expect(changed).toBe(false);
    expect(out).toBe(fm);
  });

  it("reserved-word tag quoted by yaml.v3 (`#true`) → no churn (would have emitted a boolean-coercing `- true` duplicate)", () => {
    const fm = 'tags:\n  - "true"\n';
    const { frontmatter: out, changed } = settled("status #true", fm);
    expect(changed).toBe(false);
    expect(out).toBe(fm);
  });

  it("a quoted LLM-only tag (never inline) survives and stays byte-identical", () => {
    const fm = 'title: N\ntags:\n  - "llm-only"\n';
    const { frontmatter: out, changed } = settled("prose with no tags", fm);
    expect(changed).toBe(false);
    expect(out).toBe(fm);
    expect(readTags(out)).toContain("llm-only"); // read without the surrounding quotes
  });

  it("quoted block tag DOES still persist when a different tag is genuinely added", () => {
    const fm = 'tags:\n  - "deploy"\n';
    const { frontmatter: out, changed } = rec("see #deploy", "see #deploy #feature", fm);
    expect(changed).toBe(true);
    const tags = readTags(out);
    expect(tags).toContain("deploy");
    expect(tags).toContain("feature");
    // exactly one `deploy` — no quoted/unquoted duplicate
    expect(tags.filter((t) => t === "deploy")).toHaveLength(1);
  });
});

describe("byte-stability — scalar / empty / no-key forms must not churn", () => {
  it("scalar `tags: single` matching an inline tag → byte-identical", () => {
    const fm = "title: T\ntags: single\n";
    const { frontmatter: out, changed } = settled("a #single tag", fm);
    expect(changed).toBe(false);
    expect(out).toBe(fm);
  });

  it("no `tags:` key and no inline tags → byte-identical (no spurious append)", () => {
    const fm = "title: Fresh\ndescription: hi\n";
    const { frontmatter: out, changed } = settled("just prose", fm);
    expect(changed).toBe(false);
    expect(out).toBe(fm);
  });

  it("empty flow `tags: []` with no inline tags → byte-identical", () => {
    const fm = "tags: []\n";
    const { frontmatter: out, changed } = settled("no tags here", fm);
    expect(changed).toBe(false);
    expect(out).toBe(fm);
  });

  it("`tags:` present but null (no items) and no inline tags → byte-identical", () => {
    const fm = "tags:\ntitle: T\n";
    const { frontmatter: out, changed } = settled("prose", fm);
    expect(changed).toBe(false);
    expect(out).toBe(fm);
  });
});

describe("byte-stability — order / duplicate / case insensitivity must not churn", () => {
  it("frontmatter tags in a DIFFERENT order than reconciled produces → no churn", () => {
    const fm = "tags:\n  - zebra\n  - apple\n";
    const { frontmatter: out, changed } = settled("see #apple and #zebra", fm);
    expect(changed).toBe(false); // reconciled would be [zebra, apple] (fm order kept) — same set
    expect(out).toBe(fm);
  });

  it("a tag already DUPLICATED in frontmatter stays byte-identical (set is unchanged)", () => {
    const fm = "tags:\n  - dup\n  - dup\n";
    const { frontmatter: out, changed } = settled("see #dup", fm);
    expect(changed).toBe(false);
    expect(out).toBe(fm);
  });

  it("flow-sequence with quotes + spacing, set unchanged → byte-identical", () => {
    const fm = 'tags: ["alpha", "beta"]\n';
    const { frontmatter: out, changed } = settled("#alpha and #beta", fm);
    expect(changed).toBe(false);
    expect(out).toBe(fm);
  });

  it("body case differs from fm case (#Deploy vs deploy) → one tag, no churn", () => {
    const fm = "tags:\n  - deploy\n";
    const { frontmatter: out, changed } = settled("ship #Deploy now", fm);
    expect(changed).toBe(false);
    expect(out).toBe(fm);
    expect(readTags(out)).toEqual(["deploy"]);
  });
});

describe("code-region awareness — these `#` are NOT tags, so a body of only them must not change the set", () => {
  // Each row: a body that contains a `#…` that must be EXCLUDED. With a seeded `keep`
  // tag and no real inline tag, a correct parse leaves the set unchanged (byte-stable).
  const fm = "tags:\n  - keep\n";
  const negatives: Array<[string, string]> = [
    ["fenced code (```)", "```\n#secret\n```"],
    ["fenced code (~~~)", "~~~\n#secret\n~~~"],
    ["inline code span", "use `#define` here"],
    ["indented code block (blank-line preceded)", "intro\n\n    #codeword\nback"],
    ["all-digit run #1984", "year #1984 ended"],
    ["digits+slashes #12/34", "ratio #12/34 here"],
    ["backslash-escaped \\#esc", "an \\#esc here"],
    ["mid-word foo#bar", "foo#bar baz"],
    ["after a slash /#frag", "path/#frag end"],
    ["bare # then space", "a # b"],
  ];
  it.each(negatives)("%s does not materialize → byte-identical", (_label, body) => {
    const { frontmatter: out, changed } = settled(body, fm);
    expect(changed).toBe(false);
    expect(out).toBe(fm);
    expect(readTags(out)).toEqual(["keep"]);
  });
});

describe("positive boundary tags — these SHOULD materialize", () => {
  it("nested `#a/b` materializes as a/b; a trailing slash is trimmed", () => {
    const { frontmatter: out, changed } = rec("seed", "#a/b and #c/", "tags:\n  - keep\n");
    expect(changed).toBe(true);
    const tags = readTags(out);
    expect(tags).toContain("a/b");
    expect(tags).toContain("c");
  });

  it("`#`-color short tag (#fff) materializes (locked decision — indistinguishable from a tag)", () => {
    const { frontmatter: out } = rec("seed", "color #fff", "tags:\n  - keep\n");
    expect(readTags(out)).toContain("fff");
  });

  it("a `#tag` at the very start of a blockquote inline section materializes", () => {
    const { frontmatter: out } = rec("seed", "> #quoted", "tags:\n  - keep\n");
    expect(readTags(out)).toContain("quoted");
  });
});

describe("diff-window correctness — removals diff against lastSavedBody, not current-vs-self", () => {
  it("3-step: save {a,b}; then remove #a, keep #b, add #c → {b,c} with the LLM tag kept", () => {
    // Step 1: a fresh body introduces #a #b; an LLM `llm` tag is already in frontmatter.
    const s1 = rec("seed prose", "x #a #b", "tags:\n  - llm\n");
    expect(s1.changed).toBe(true);
    expect(new Set(readTags(s1.frontmatter))).toEqual(new Set(["llm", "a", "b"]));

    // Step 2: lastSavedBody is now "x #a #b"; the edit removes #a, keeps #b, adds #c.
    const s2 = rec("x #a #b", "x #b #c", s1.frontmatter);
    expect(s2.changed).toBe(true);
    expect(new Set(readTags(s2.frontmatter))).toEqual(new Set(["llm", "b", "c"]));
  });

  it("a tag removed from one spot but still present elsewhere in the body is NOT removed", () => {
    // lastSaved had #a twice; the edit drops one occurrence but #a still appears.
    const { changed } = rec("#a foo #a", "#a foo", "tags:\n  - a\n");
    expect(changed).toBe(false); // #a is still inline ⇒ not a deletion ⇒ no change
  });

  it("an LLM-only tag that COLLIDES with a body-removed inline tag is still removed (documented diff semantics)", () => {
    // `deploy` is in frontmatter; it was inline last save and is now deleted. The diff
    // cannot tell an LLM-origin `deploy` from an inline-origin one — removal wins.
    const { frontmatter: out, changed } = rec("ship #deploy", "ship", "tags:\n  - deploy\n  - llm\n");
    expect(changed).toBe(true);
    expect(readTags(out)).toEqual(["llm"]);
  });
});

describe("PROPERTY — idempotence / loop-termination (reconcile∘reconcile = reconcile)", () => {
  // The autosave loop in PageEditor re-saves until the live content matches the last
  // save. If reconcile were not idempotent, a settled body would keep producing
  // changed=true and the editor would 409-thrash forever. For every case: after the
  // first reconcile, feeding the OUTPUT frontmatter back with a now-settled body must
  // yield changed=false AND a byte-identical frontmatter.
  const cases: Array<[string, string, string, string]> = [
    ["add", "seed", "x #deploy", "title: T\n"],
    ["add+remove+persist", "intro #old", "intro #deploy", "tags:\n  - old\n  - llm\n"],
    ["quoted fm tag + inline", "see #deploy", "see #deploy", 'tags:\n  - "deploy"\n'],
    ["flow fm → add", "seed", "#new", "tags: [keep]\n"],
    ["scalar fm → add", "seed", "#new", "tags: keep\n"],
    ["nested tag", "seed", "#area/sub", "title: T\n"],
    ["remove last inline", "had #gone", "had", "tags:\n  - gone\n  - llm\n"],
    ["case-folded", "seed", "#Deploy #DEPLOY", "title: T\n"],
  ];
  it.each(cases)("%s is idempotent", (_label, last, cur, fm) => {
    const first = rec(last, cur, fm);
    const second = settled(cur, first.frontmatter); // body settled at `cur`
    expect(second.changed).toBe(false);
    expect(second.frontmatter).toBe(first.frontmatter);
  });
});

describe("PROPERTY — LLM-only tag persistence (a tag never inline always survives)", () => {
  const bodies = ["", "no tags", "added #x", "#a #b #c", "```\n#fenced\n```", "removed #x"];
  it.each(bodies)("llm survives for body %j", (cur) => {
    // `llm` is in frontmatter and never appears inline in `cur` ⇒ it can never be in
    // removedInline ⇒ it must be in the reconciled set.
    const { frontmatter: out } = rec("seed #x", cur, "tags:\n  - llm\n");
    expect(readTags(out)).toContain("llm");
  });
});

describe("PROPERTY — !changed ⇒ output is byte-identical (the contract's hard guard)", () => {
  const frontmatters = [
    "title: T\n",
    "tags:\n  - a\n  - b\n",
    'tags:\n  - "a"\n',
    "tags: [a, b]\n",
    "tags: single\n",
    "tags: []\n",
    "title: T\ntags:\n  - a\n  - a\n",
  ];
  it.each(frontmatters)("settled no-tag body keeps %j verbatim", (fm) => {
    const { frontmatter: out, changed } = settled("prose with no hashtags at all", fm);
    expect(changed).toBe(false);
    expect(out).toBe(fm);
  });
});

describe("METAMORPHIC — inline order does not affect the reconciled SET", () => {
  it("permuting the inline tags in the body yields the same set", () => {
    const a = rec("seed", "#a #b #c", "tags:\n  - llm\n");
    const b = rec("seed", "#c #b #a", "tags:\n  - llm\n");
    expect(new Set(readTags(a.frontmatter))).toEqual(new Set(readTags(b.frontmatter)));
  });

  it("add-then-remove of the same tag across two saves nets back to the original set", () => {
    const base = "tags:\n  - keep\n";
    // save 1: add #temp
    const s1 = rec("body", "body #temp", base);
    expect(new Set(readTags(s1.frontmatter))).toEqual(new Set(["keep", "temp"]));
    // save 2: remove #temp again (diffed against "body #temp")
    const s2 = rec("body #temp", "body", s1.frontmatter);
    expect(new Set(readTags(s2.frontmatter))).toEqual(new Set(["keep"]));
  });
});
