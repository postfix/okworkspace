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
const FORBIDDEN_IN_ENTRY = [
  /node_modules\/@codemirror\/lang-(?!(?:markdown|html|css|javascript)\/)/,
  /node_modules\/@codemirror\/legacy-modes\//,
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
