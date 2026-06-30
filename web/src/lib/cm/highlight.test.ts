// ED-04a Task 1 — the shared HighlightStyle + the code-split codeLanguages.
//
// Two things are pinned here:
//   1. codeHighlightStyle is a real HighlightStyle whose `style(tags)` maps the curated
//      code tags → the explicit `tok-*` classes (the single source of truth both surfaces
//      consume).
//   2. codeLanguages carries the curated names + aliases, each `load()` resolves to a
//      LanguageSupport WITHOUT any static top-level grammar import (the code-split
//      contract, D-05) — asserted both behaviorally (load() works) and structurally (the
//      module source contains grammar package strings ONLY inside dynamic import()).
import { describe, it, expect } from "vitest";
import { LanguageSupport } from "@codemirror/language";
import { tags as t } from "@lezer/highlight";
import { codeHighlightStyle, codeLanguages } from "./highlight";
// Vite-native raw import of this module's source text — used by the structural
// "no static grammar import" assertion below.
import highlightSource from "./highlight.ts?raw";

describe("codeHighlightStyle (shared, token-driven HighlightStyle)", () => {
  it("is a HighlightStyle with a callable style(tags)", () => {
    expect(typeof codeHighlightStyle.style).toBe("function");
  });

  it("maps the curated code tags → explicit tok-* classes", () => {
    // style() returns the space-separated class list for a set of tags.
    expect(codeHighlightStyle.style([t.keyword])).toBe("tok-keyword");
    expect(codeHighlightStyle.style([t.string])).toBe("tok-string");
    expect(codeHighlightStyle.style([t.comment])).toBe("tok-comment");
    expect(codeHighlightStyle.style([t.number])).toBe("tok-number");
    expect(codeHighlightStyle.style([t.function(t.variableName)])).toBe(
      "tok-function",
    );
    expect(codeHighlightStyle.style([t.typeName])).toBe("tok-function");
  });

  it("does NOT map Markdown structural tags (heading/strong/emphasis own those)", () => {
    // The editor's .cm-heading/.cm-strong/.cm-em decorations must stay untouched, so this
    // code-scoped style returns null for those tags.
    expect(codeHighlightStyle.style([t.heading])).toBeNull();
    expect(codeHighlightStyle.style([t.strong])).toBeNull();
    expect(codeHighlightStyle.style([t.emphasis])).toBeNull();
  });
});

describe("codeLanguages (curated, code-split set — D-06)", () => {
  const byName = new Map(codeLanguages.map((d) => [d.name, d]));

  it("includes the curated v1.4 languages", () => {
    for (const name of [
      "javascript",
      "python",
      "go",
      "rust",
      "json",
      "yaml",
      "sql",
      "html",
      "css",
      "shell",
      "markdown",
      "diff",
    ]) {
      expect(byName.has(name)).toBe(true);
    }
  });

  it("does NOT include mermaid (left plain — Phase 26)", () => {
    expect(byName.has("mermaid")).toBe(false);
  });

  it("carries the expected aliases (js/ts → javascript, bash/sh → shell)", () => {
    expect(byName.get("javascript")!.alias).toEqual(
      expect.arrayContaining(["js", "ts", "jsx", "tsx", "typescript"]),
    );
    expect(byName.get("shell")!.alias).toEqual(
      expect.arrayContaining(["bash", "sh"]),
    );
    expect(byName.get("python")!.alias).toContain("py");
    expect(byName.get("go")!.alias).toContain("golang");
  });

  // The async load() must resolve to a usable LanguageSupport for a representative subset
  // spanning lang-* (go, python), legacy-modes (shell, diff), and js/ts.
  it.each(["go", "python", "shell", "diff", "javascript"])(
    "load() resolves %s to a LanguageSupport (dynamic import, no static dep)",
    async (name) => {
      const support = await byName.get(name)!.load!();
      expect(support).toBeInstanceOf(LanguageSupport);
      // A LanguageSupport carries a parsable language.
      expect(support.language.parser).toBeDefined();
    },
  );

  it("imports NO grammar at module top — only dynamic import() inside load (D-05)", () => {
    const src = highlightSource;
    // Every reference to a grammar package must be a dynamic `import("@codemirror/lang-…")`
    // or `import("@codemirror/legacy-modes/…")` — never a static top-level
    // `import x from "@codemirror/lang-…"`. A static import would put the grammar in the
    // entry chunk (the thing assertLazyChunks forbids).
    const staticGrammarImport =
      /^import\s+[^;]*\bfrom\s+["']@codemirror\/(lang-(?!markdown)|legacy-modes)/m;
    expect(staticGrammarImport.test(src)).toBe(false);
    // And there IS at least one dynamic import of a grammar (sanity: the loads are real).
    expect(/import\(["']@codemirror\/lang-go["']\)/.test(src)).toBe(true);
  });
});
