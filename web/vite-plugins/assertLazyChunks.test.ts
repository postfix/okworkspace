// ED-04a Task 4 — unit tests for the assertLazyChunks build guard, run against SYNTHETIC
// bundle objects (no real build needed → fast + deterministic). The plugin's
// generateBundle hook calls this.error on a violation; we drive it with a fake
// PluginContext whose error() throws (mirroring Rollup's real never-returning error()).
import { describe, it, expect } from "vitest";
import { assertLazyChunks } from "./assertLazyChunks";

// Synthetic bundles are plain records of chunk/asset-shaped objects — only the fields the
// plugin reads matter (type/isEntry/moduleIds/code/fileName). Typed loosely + cast at the
// call boundary so the test doesn't depend on the exact rolldown/rollup bundle types.
type SyntheticBundle = Record<string, Record<string, unknown>>;

// A minimal fake PluginContext: error() throws, matching Rollup/rolldown's contract.
function fakeCtx(): { error: (e: string | { message: string }) => never } {
  return {
    error(e: string | { message: string }): never {
      throw new Error(typeof e === "string" ? e : e.message);
    },
  };
}

// Invoke the plugin's generateBundle hook (which may be a function or { handler }).
function runGenerateBundle(
  plugin: ReturnType<typeof assertLazyChunks>,
  bundle: SyntheticBundle,
): void {
  const hook = plugin.generateBundle;
  const fn = typeof hook === "function" ? hook : hook!.handler;
  // The hook only uses `this.error`, `chunk.type/isEntry/moduleIds/code/fileName`.
  (fn as (...a: unknown[]) => void).call(fakeCtx(), {}, bundle, false);
}

// Synthetic entry chunk.
function entryChunk(fileName: string, moduleIds: string[], code = ""): SyntheticBundle {
  return {
    [fileName]: { type: "chunk", isEntry: true, fileName, moduleIds, code },
  };
}

// Mirrors vite.config.ts: the four CORE grammars (markdown + its transitive html/css/js)
// are excluded; every genuinely-lazy grammar stays forbidden.
const FORBIDDEN = [
  /node_modules\/@codemirror\/lang-(?!(?:markdown|html|css|javascript)\/)/,
  /node_modules\/@codemirror\/legacy-modes\//,
];

describe("assertLazyChunks", () => {
  it("errors when a forbidden grammar lands in an entry chunk", () => {
    const bundle = entryChunk("index.js", [
      "/app/src/main.ts",
      "/app/node_modules/@codemirror/lang-go/dist/index.js",
    ]);
    expect(() =>
      runGenerateBundle(assertLazyChunks({ forbidden: FORBIDDEN }), bundle),
    ).toThrow(/lang-go.*leaked.*entry chunk/);
  });

  it("errors when a legacy-modes grammar lands in an entry chunk", () => {
    const bundle = entryChunk("index.js", [
      "/app/node_modules/@codemirror/legacy-modes/mode/shell.js",
    ]);
    expect(() =>
      runGenerateBundle(assertLazyChunks({ forbidden: FORBIDDEN }), bundle),
    ).toThrow(/legacy-modes/);
  });

  it("does NOT error when grammars are only in NON-entry (lazy) chunks", () => {
    const bundle: SyntheticBundle = {
      "index.js": {
        type: "chunk",
        isEntry: true,
        fileName: "index.js",
        moduleIds: ["/app/src/main.ts", "/app/src/lib/cm/highlight.ts"],
        code: "",
      },
      "lang-go.js": {
        type: "chunk",
        isEntry: false, // lazy chunk — grammars are SUPPOSED to live here
        fileName: "lang-go.js",
        moduleIds: ["/app/node_modules/@codemirror/lang-go/dist/index.js"],
        code: "",
      },
    };
    expect(() =>
      runGenerateBundle(assertLazyChunks({ forbidden: FORBIDDEN }), bundle),
    ).not.toThrow();
  });

  it("does NOT forbid the four CORE grammars (markdown + transitive html/css/javascript)", () => {
    // lang-markdown is the editor's base language (statically loaded); it hard-depends on
    // lang-html, which pulls lang-css + lang-javascript. All four are unavoidably in the
    // entry chunk (and were pre-phase) — the negative-lookahead regex must allow them.
    for (const core of ["lang-markdown", "lang-html", "lang-css", "lang-javascript"]) {
      const bundle = entryChunk("index.js", [
        `/app/node_modules/@codemirror/${core}/dist/index.js`,
      ]);
      expect(() =>
        runGenerateBundle(assertLazyChunks({ forbidden: FORBIDDEN }), bundle),
      ).not.toThrow();
    }
  });

  it("DOES forbid the genuinely-lazy grammars (json shares no prefix with javascript)", () => {
    // Guard the lookahead boundary: lang-json must stay forbidden even though it starts
    // with "j" like javascript.
    for (const lazy of ["lang-json", "lang-python", "lang-go", "lang-rust", "lang-yaml", "lang-sql"]) {
      const bundle = entryChunk("index.js", [
        `/app/node_modules/@codemirror/${lazy}/dist/index.js`,
      ]);
      expect(() =>
        runGenerateBundle(assertLazyChunks({ forbidden: FORBIDDEN }), bundle),
      ).toThrow(new RegExp(lazy));
    }
  });

  it("errors when the entry chunk exceeds maxEntryBytes", () => {
    const bundle = entryChunk("index.js", ["/app/src/main.ts"], "x".repeat(2000));
    expect(() =>
      runGenerateBundle(
        assertLazyChunks({ forbidden: FORBIDDEN, maxEntryBytes: 1000 }),
        bundle,
      ),
    ).toThrow(/over the 1000-byte budget/);
  });

  it("passes when the entry chunk is under maxEntryBytes", () => {
    const bundle = entryChunk("index.js", ["/app/src/main.ts"], "x".repeat(500));
    expect(() =>
      runGenerateBundle(
        assertLazyChunks({ forbidden: FORBIDDEN, maxEntryBytes: 1000 }),
        bundle,
      ),
    ).not.toThrow();
  });

  it("ignores non-entry chunks and assets", () => {
    const bundle: SyntheticBundle = {
      "lazy.js": {
        type: "chunk",
        isEntry: false,
        fileName: "lazy.js",
        moduleIds: ["/app/node_modules/@codemirror/lang-go/dist/index.js"],
        code: "x".repeat(99999),
      },
      "style.css": {
        type: "asset",
        fileName: "style.css",
        source: "body{}",
      },
    };
    expect(() =>
      runGenerateBundle(
        assertLazyChunks({ forbidden: FORBIDDEN, maxEntryBytes: 10 }),
        bundle,
      ),
    ).not.toThrow();
  });

  it("anchors the exclusion to the full name — a hypothetical lang-cssinjs stays forbidden", () => {
    // The trailing-`/` anchor means only the exact core names (css/html/javascript/
    // markdown) are excluded; a prefix-colliding future grammar is NOT silently allowed.
    const bundle = entryChunk("index.js", [
      "/app/node_modules/@codemirror/lang-cssinjs/dist/index.js",
    ]);
    expect(() =>
      runGenerateBundle(assertLazyChunks({ forbidden: FORBIDDEN }), bundle),
    ).toThrow(/lang-cssinjs/);
  });

  it("is reusable: a custom forbidden list (e.g. mermaid) is honored", () => {
    const bundle = entryChunk("index.js", [
      "/app/node_modules/mermaid/dist/mermaid.core.mjs",
    ]);
    expect(() =>
      runGenerateBundle(assertLazyChunks({ forbidden: [/mermaid/] }), bundle),
    ).toThrow(/mermaid/);
  });
});
