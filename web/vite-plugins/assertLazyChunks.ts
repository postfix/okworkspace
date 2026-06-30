// assertLazyChunks — a build-time structural guard (ED-04a / D-05).
//
// A Rollup `generateBundle` plugin that inspects the EMITTED bundle and FAILS the build
// (`this.error`) if either invariant is violated:
//
//   1. No forbidden module leaks into an ENTRY chunk. For every entry chunk, each module
//      id in `chunk.moduleIds` is matched against the `forbidden` regexes; a match means a
//      module that should be lazy-loaded got statically pulled into the entry/main chunk.
//      (moduleIds is the only module-level signal in the bundle — manifest.json is too
//      coarse.)
//   2. The entry chunk's byte size stays under `maxEntryBytes` (when provided) — a measured
//      budget so a future accidental static import is caught even if it dodges the regexes.
//
// Designed for REUSE: the `forbidden` list is a parameter. Phase 25 forbids the
// code-split syntax grammars; Phase 26 will reuse the same plugin with `forbidden: [/mermaid/]`.
import type { Plugin } from "vite";

export interface AssertLazyChunksOptions {
  // Module-id patterns that must NOT appear in any entry chunk (they must be code-split).
  forbidden: RegExp[];
  // Optional hard cap on an entry chunk's byte size (Buffer.byteLength of chunk.code).
  maxEntryBytes?: number;
}

export function assertLazyChunks(opts: AssertLazyChunksOptions): Plugin {
  const { forbidden, maxEntryBytes } = opts;
  return {
    name: "assert-lazy-chunks",
    generateBundle(_options, bundle) {
      for (const file of Object.values(bundle)) {
        // Only entry chunks are inspected — lazy/vendor chunks are exactly where the
        // forbidden modules are SUPPOSED to live. The `type === "chunk"` guard narrows
        // `file` to the chunk type (moduleIds / code / fileName available).
        if (file.type !== "chunk" || !file.isEntry) continue;

        for (const id of file.moduleIds) {
          for (const re of forbidden) {
            if (re.test(id)) {
              this.error(
                `assertLazyChunks: forbidden module ${id} (matched ${re}) leaked ` +
                  `into entry chunk ${file.fileName}. It must be code-split ` +
                  `(loaded via a dynamic import()), not statically imported.`,
              );
            }
          }
        }

        if (maxEntryBytes != null) {
          const bytes = Buffer.byteLength(file.code);
          if (bytes > maxEntryBytes) {
            this.error(
              `assertLazyChunks: entry chunk ${file.fileName} is ${bytes} bytes, ` +
                `over the ${maxEntryBytes}-byte budget. A heavy module likely landed ` +
                `in the entry chunk — keep grammars/large deps lazy.`,
            );
          }
        }
      }
    },
  };
}
