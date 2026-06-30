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

// ED-05a / D-03: the mermaid dep rules (mirrors vite.config.ts MERMAID_FORBIDDEN). These
// prove mermaid + its heavy MERMAID-UNIQUE deps are kept out of every entry chunk.
// `stylis` (also @emotion/css via react-diff-viewer-continued) and `d3-*` (also
// react-force-graph-2d via force-graph) are intentionally absent — both legitimately ride
// the entry chunk via OTHER static deps, so they are not mermaid-leak signals (see the
// vite.config.ts comment for the `npm ls` provenance).
const MERMAID_FORBIDDEN = [
  /node_modules[\\/]mermaid[\\/]/,
  /node_modules[\\/]dagre-d3-es[\\/]/,
  /node_modules[\\/]cytoscape(-[a-z-]+)?[\\/]/,
  /node_modules[\\/]khroma[\\/]/,
  /node_modules[\\/]dompurify[\\/]/,
  /node_modules[\\/]katex[\\/]/,
  /node_modules[\\/]marked[\\/]/,
  /node_modules[\\/]dayjs[\\/]/,
  /node_modules[\\/]roughjs[\\/]/,
  /node_modules[\\/]@mermaid-js[\\/]parser[\\/]/,
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

  // ─── ED-05a (Phase 26): the mermaid forbidden rules have teeth ───
  describe("mermaid forbidden rules (ED-05a)", () => {
    it("errors when mermaid itself leaks into an entry chunk", () => {
      const bundle = entryChunk("index.js", [
        "/app/src/main.tsx",
        "/app/node_modules/mermaid/dist/mermaid.core.mjs",
      ]);
      expect(() =>
        runGenerateBundle(assertLazyChunks({ forbidden: MERMAID_FORBIDDEN }), bundle),
      ).toThrow(/mermaid.*leaked.*entry chunk/);
    });

    it("errors when a mermaid-unique transitive dep (dagre-d3-es) leaks into an entry chunk", () => {
      const bundle = entryChunk("index.js", [
        "/app/node_modules/dagre-d3-es/src/dagre/index.js",
      ]);
      expect(() =>
        runGenerateBundle(assertLazyChunks({ forbidden: MERMAID_FORBIDDEN }), bundle),
      ).toThrow(/dagre-d3-es.*leaked/);
    });

    it("does NOT forbid the SHARED deps (stylis via emotion, d3-* via force-graph)", () => {
      // These legitimately ride the entry chunk via OTHER static deps — the mermaid rules
      // must NOT flag them (else the build fails on a pre-existing, non-mermaid fact).
      for (const id of [
        "/app/node_modules/stylis/src/Enum.js",
        "/app/node_modules/d3-selection/src/namespaces.js",
        "/app/node_modules/d3-shape/src/index.js",
      ]) {
        const bundle = entryChunk("index.js", ["/app/src/main.tsx", id]);
        expect(() =>
          runGenerateBundle(assertLazyChunks({ forbidden: MERMAID_FORBIDDEN }), bundle),
        ).not.toThrow();
      }
    });

    it("errors for each heavy mermaid dep family in an entry chunk", () => {
      const leaks = [
        "/app/node_modules/dagre-d3-es/src/dagre/index.js",
        "/app/node_modules/cytoscape-cose-bilkent/index.js",
        "/app/node_modules/dompurify/dist/purify.js",
        "/app/node_modules/katex/dist/katex.mjs",
        "/app/node_modules/@mermaid-js/parser/dist/index.js",
        "/app/node_modules/khroma/dist/index.js",
      ];
      for (const id of leaks) {
        const bundle = entryChunk("index.js", [id]);
        expect(() =>
          runGenerateBundle(assertLazyChunks({ forbidden: MERMAID_FORBIDDEN }), bundle),
        ).toThrow(/leaked into entry chunk/);
      }
    });

    it("does NOT error when mermaid + deps live only in NON-entry (lazy) chunks", () => {
      const bundle: SyntheticBundle = {
        "index.js": {
          type: "chunk",
          isEntry: true,
          fileName: "index.js",
          moduleIds: ["/app/src/main.tsx", "/app/src/lib/cm/mermaid.ts"],
          code: "",
        },
        "mermaid.lazy.js": {
          type: "chunk",
          isEntry: false, // the dynamic import() chunk — mermaid is SUPPOSED to live here
          fileName: "mermaid.lazy.js",
          moduleIds: [
            "/app/node_modules/mermaid/dist/mermaid.core.mjs",
            "/app/node_modules/d3-shape/src/index.js",
            "/app/node_modules/dompurify/dist/purify.js",
          ],
          code: "",
        },
      };
      expect(() =>
        runGenerateBundle(assertLazyChunks({ forbidden: MERMAID_FORBIDDEN }), bundle),
      ).not.toThrow();
    });
  });
});
