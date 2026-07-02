// codec — the HEADLESS TipTap <-> Markdown codec (RE-01 crit-1, D-01).
//
// Built directly on @tiptap/markdown's MarkdownManager — NO Editor, NO DOM. The
// MarkdownManager runs fully headless: `.parse(md)` lexes Markdown (MarkedJS-backed)
// into a ProseMirror/TipTap JSON document, `.serialize(json)` walks that JSON back to
// a Markdown string using each extension's renderMarkdown handler. Constructing an
// Editor would require a DOM; we deliberately never do that here.
//
// SECURITY (D-04, MAJ-04-b): MarkdownManager only turns embedded raw HTML into DOM
// nodes when `window.DOMParser` exists. In a pure-node environment (no window) it
// returns embedded HTML as entity-escaped LITERAL TEXT — no live DOM, no executable
// attributes. The codec tests therefore MUST run under a `node` vitest environment,
// NOT the app's global jsdom (which would provide window.DOMParser and re-open the
// HTML->DOM path). The gate asserts this; see codec.test.ts.
//
// IMPORT-ISOLATION (RE-04): nothing in the app entry statically imports this module
// this phase — it is exercised only by the codec/parity tests — so `marked` stays out
// of the entry chunk. The code-split guard itself lands in Phase 42.
import { MarkdownManager } from "@tiptap/markdown";
import type { JSONContent } from "@tiptap/core";
import { buildExtensions } from "./extensions";

// parse lexes a Markdown string into a TipTap/ProseMirror JSON document. Headless. A
// fresh MarkdownManager per call keeps parse/serialize stateless (no cross-call token/
// mark state) and cheap — the corpus is small and this runs only in tests.
export function parse(md: string): JSONContent {
  return new MarkdownManager({ extensions: buildExtensions() }).parse(md);
}

// serialize walks a TipTap JSON document back to a Markdown string. Headless, pure
// JSON walk — no DOM. Marker-canonicalizing (bullets */+ -> -, emphasis __/_ -> **/*,
// loose lists tightened): this is allowed Tier-1 normalization (D-02), and it is
// stable/idempotent, which the corpus gate proves.
export function serialize(doc: JSONContent): string {
  return new MarkdownManager({ extensions: buildExtensions() }).serialize(doc);
}

// roundTrip is parse-then-serialize: it canonicalizes a Markdown string through the
// codec. The gate compares parse(roundTrip(src)) tree-equal to parse(src) and asserts
// roundTrip is idempotent (roundTrip(roundTrip(x)) === roundTrip(x)).
export function roundTrip(md: string): string {
  return serialize(parse(md));
}
