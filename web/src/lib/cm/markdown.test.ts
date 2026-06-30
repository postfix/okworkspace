// ED-04a Task 2 — wiring the live editor: markdown({ codeLanguages }) stays GFM-only
// (codeLanguages is additive) AND liveExtensions carries syntaxHighlighting(codeHighlightStyle)
// while sourceExtensions does NOT.
//
// Two guards:
//   1. Regression — markdownExtension still parses the four GFM constructs + the Hashtag
//      inline node, and a ```go fence still parses to FencedCode/CodeInfo (additive
//      codeLanguages did not perturb the construct set).
//   2. Wiring — highlightingFor(state, [tags.keyword]) returns "tok-keyword" for a state
//      built from liveExtensions (proving the shared style is active there) and null for
//      sourceExtensions (Source mode stays raw mono).
import { describe, it, expect } from "vitest";
import { EditorState } from "@codemirror/state";
import { highlightingFor } from "@codemirror/language";
import { tags as t } from "@lezer/highlight";
import type { Tree } from "@lezer/common";

import { markdownExtension } from "./markdown";
import { liveExtensions, sourceExtensions } from "./mode";

// Collect every node name present in a parse of `doc`.
function nodeNames(doc: string): Set<string> {
  const tree: Tree = markdownExtension.language.parser.parse(doc);
  const names = new Set<string>();
  tree.iterate({ enter: (n) => void names.add(n.name) });
  return names;
}

describe("markdownExtension — GFM-only contract preserved with codeLanguages (regression)", () => {
  it("still parses the four GFM constructs", () => {
    expect(nodeNames("| a | b |\n|---|---|\n| 1 | 2 |").has("Table")).toBe(true);
    expect(nodeNames("~~struck~~").has("Strikethrough")).toBe(true);
    expect(nodeNames("- [ ] todo").has("TaskList") || nodeNames("- [ ] todo").has("Task")).toBe(
      true,
    );
    expect(nodeNames("see https://x.test now").has("URL")).toBe(true);
  });

  it("still parses the ED-02a Hashtag inline node", () => {
    expect(nodeNames("see #deploy").has("Hashtag")).toBe(true);
  });

  it("does NOT promote to the markdownLanguage superset (no Emoji/Superscript)", () => {
    // The GFM-only contract: emoji/super/subscript must NOT parse as constructs.
    const n = nodeNames(":smile: ^sup^ ~sub~");
    expect(n.has("Emoji")).toBe(false);
    expect(n.has("Superscript")).toBe(false);
  });

  it("parses a ```go fence to FencedCode with CodeInfo 'go' + CodeText", () => {
    const names = nodeNames("```go\nfunc main() {}\n```");
    expect(names.has("FencedCode")).toBe(true);
    expect(names.has("CodeInfo")).toBe(true);
    expect(names.has("CodeText")).toBe(true);
  });
});

describe("syntaxHighlighting wiring (codeHighlightStyle active in liveExtensions only)", () => {
  it("liveExtensions resolves code tags to tok-* classes", () => {
    const state = EditorState.create({ doc: "x", extensions: liveExtensions });
    expect(highlightingFor(state, [t.keyword])).toBe("tok-keyword");
    expect(highlightingFor(state, [t.string])).toBe("tok-string");
    expect(highlightingFor(state, [t.function(t.variableName)])).toBe("tok-function");
  });

  it("sourceExtensions does NOT carry the code HighlightStyle (raw mono)", () => {
    const state = EditorState.create({ doc: "x", extensions: sourceExtensions });
    expect(highlightingFor(state, [t.keyword])).toBeNull();
  });
});
