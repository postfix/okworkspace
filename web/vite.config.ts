import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import { assertLazyChunks } from "./vite-plugins/assertLazyChunks";

// ED-04a / D-05: the syntax-highlight grammars added by this phase MUST stay code-split —
// never statically pulled into an entry chunk.
//
// FOUR grammars are EXCLUDED because they are CORE, not code-fence additions: the editor's
// base language @codemirror/lang-markdown (statically imported in markdown.ts) has a hard
// dependency on @codemirror/lang-html (embedded-HTML support), which transitively pulls
// @codemirror/lang-css + @codemirror/lang-javascript. Those four are therefore unavoidably
// in the entry chunk and were ALREADY there before this phase — forbidding them would
// flag a pre-existing core fact, not a regression. The negative lookahead excludes exactly
// those four; every genuinely-lazy grammar (python/go/rust/json/yaml/sql + legacy-modes
// shell/diff) stays forbidden. maxEntryBytes is the MEASURED entry size + ~20% headroom
// (see 25-01-PLAN-SUMMARY.md) — empirical, re-measured if the entry legitimately grows.
// The excluded names are anchored with a trailing `/` (the path-segment boundary in a
// module id like `.../@codemirror/lang-css/dist/index.js`) so the lookahead matches the
// WHOLE name, not a prefix — a future `lang-cssinjs` / `lang-htmlx` would NOT be silently
// allowed (it stays forbidden because `cssinjs/` ≠ `css/`).
// ED-05a / D-03: mermaid is LARGE and loaded only via a dynamic import("mermaid")
// (lib/cm/mermaid.ts) → its own lazy chunk. These rules PROVE mermaid AND its heavy
// transitive deps (d3 / dagre / cytoscape / dompurify / katex / …) never get statically
// pulled into an entry chunk. A single dynamic import splits cleanly under Vite/Rollup;
// the ONLY thing that breaks laziness is a stray static `import "mermaid"` reachable from
// entry — these regexes catch exactly that, failing `npm run build`. The entry budget is
// NOT raised: mermaid must not inflate the entry chunk. `[\\/]` matches POSIX + Windows
// module-id separators.
//
// `stylis` and `d3-*` are DELIBERATELY EXCLUDED (research_facts listed them, but
// `npm ls` shows both are SHARED with other STATICALLY-imported app deps, so they
// legitimately ride the entry chunk INDEPENDENT of mermaid — the Phase-25 lang-css
// situation):
//   • stylis  → also a dep of react-diff-viewer-continued → @emotion/css.
//   • d3-*    → also a dep of react-force-graph-2d → force-graph (d3-selection/zoom/…).
// Forbidding them would flag a pre-existing, non-mermaid fact. Every rule KEPT below is
// mermaid-UNIQUE per `npm ls` (mermaid is the SOLE root that pulls it), so each remains a
// reliable signal: a stray static mermaid import would drag ALL of these (mermaid itself
// + its unique renderers dagre-d3-es / cytoscape / @mermaid-js/parser / roughjs / katex /
// marked / dayjs / dompurify / khroma) into the entry chunk, and any one match fails the build.
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

const FORBIDDEN_IN_ENTRY = [
  /node_modules\/@codemirror\/lang-(?!(?:markdown|html|css|javascript)\/)/,
  /node_modules\/@codemirror\/legacy-modes\//,
  ...MERMAID_FORBIDDEN,
];

// Build output goes to internal/web/dist, which the Go binary embeds via
// embed.FS. Go's //go:embed cannot traverse ".." and resolves relative to the
// embedding source file (internal/web/embed.go), so the SPA must build there.
// During dev, /api requests are proxied to the Go backend on :8080.
export default defineConfig({
  plugins: [
    react(),
    // Measured entry chunk = 1,180,566 bytes (clean build, grammars lazy); budget set to
    // 1,420,000 (~20% headroom). Re-measure + raise here if the entry legitimately grows.
    assertLazyChunks({
      forbidden: FORBIDDEN_IN_ENTRY,
      maxEntryBytes: 1_420_000,
    }),
  ],
  build: {
    outDir: "../internal/web/dist",
    emptyOutDir: true,
  },
  server: {
    proxy: {
      "/api": "http://localhost:8080",
    },
  },
});
