// guard — the TipTap LOAD GUARD (RE-04, D-02). isLosslessRoundTrip decides whether a
// Markdown buffer is SAFE to open in the TipTap surface: it is safe iff the codec's
// parse→serialize→parse cycle is TREE-STABLE, i.e. serializing the parsed document and
// re-parsing it yields an IDENTICAL TipTap/ProseMirror JSON tree.
//
// TREE-EQUALITY is the informed-consent semantics (D-02, the co-driver's locked
// decision — NOT byte-equality, which was the rejected fork option ii):
//   - Modeled-syntax normalization (`*`→`-` bullets, `__`→`**` emphasis, loose→tight
//     lists) is SEMANTICALLY lossless — parse maps both spellings to the SAME tree, so the
//     guard PASSES. The one-time consent notice (TipTapConsentNotice) tells the user those
//     bytes reflow for everyone viewing the page; the guard does NOT block it.
//   - GENUINE content loss — a construct the bounded slice + preserved tier cannot
//     round-trip tree-stably, or any parse/serialize throw — returns false: TipTap is made
//     UNAVAILABLE for that page (a guard, not a toast; PageEditor warns AND falls back to
//     CM6, on BOTH open-in-tiptap and switch-to-tiptap, against the CURRENT live buffer).
//
// It composes the exported HEADLESS codec (codec.ts) — no Editor, no DOM — so it runs
// cheaply and synchronously on the live buffer at guard time.
import type { JSONContent } from "@tiptap/core";
import { parse, roundTrip } from "./codec";

// deepEqual is a structural compare over the plain-JSON TipTap document (the output of
// MarkdownManager.parse — objects/arrays/primitives only, no class instances, no cycles).
// A hand-rolled recursive compare (NOT JSON.stringify) so key ORDER never affects the
// verdict: two trees are equal iff they have the same keys with deeply-equal values.
function deepEqual(a: unknown, b: unknown): boolean {
  if (a === b) return true;
  if (
    typeof a !== "object" ||
    typeof b !== "object" ||
    a === null ||
    b === null
  ) {
    return false;
  }
  const aArr = Array.isArray(a);
  const bArr = Array.isArray(b);
  if (aArr !== bArr) return false;
  if (aArr && bArr) {
    if (a.length !== b.length) return false;
    for (let i = 0; i < a.length; i++) {
      if (!deepEqual(a[i], b[i])) return false;
    }
    return true;
  }
  const ao = a as Record<string, unknown>;
  const bo = b as Record<string, unknown>;
  const aKeys = Object.keys(ao);
  const bKeys = Object.keys(bo);
  if (aKeys.length !== bKeys.length) return false;
  for (const k of aKeys) {
    if (!Object.prototype.hasOwnProperty.call(bo, k)) return false;
    if (!deepEqual(ao[k], bo[k])) return false;
  }
  return true;
}

// isLosslessRoundTrip returns true iff `md` round-trips TREE-STABLE through the codec
// (parse(roundTrip(md)) deep-equals parse(md)). A parse/serialize throw returns false —
// the guard is a backstop, so anything the codec cannot cleanly process is treated as
// unsafe to open in TipTap and PageEditor keeps the byte-exact CM6 surface.
export function isLosslessRoundTrip(md: string): boolean {
  try {
    const reparsed: JSONContent = parse(roundTrip(md));
    const direct: JSONContent = parse(md);
    return deepEqual(reparsed, direct);
  } catch {
    return false;
  }
}
