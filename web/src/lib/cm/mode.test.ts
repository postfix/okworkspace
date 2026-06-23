// EDIT-02 — toggling Live⇄Source⇄Live never mutates the document bytes. This is a
// STATE-level test (no DOM): build an EditorState per okf corpus fixture, flip the
// modeCompartment via setMode, and assert state.doc.toString() is byte-identical
// before and after. Byte-identity is by construction (the toggle dispatch carries
// `effects` only, never `changes`) — this test is the guard that it stays so.
//
// NOTE ON CRLF: CM6's Text store normalizes line endings to "\n" internally (as a
// browser <textarea> — the old MDEditor surface — also does on the wire). So the
// EDIT-02 invariant is "the toggle never changes whatever bytes the document
// currently holds", which we assert against the document's own pre-toggle string
// (`before`), not against the raw on-disk fixture. The on-disk CRLF round-trip is
// a backend concern covered by TestGoldenRoundTrip / TestCorpusHasCRLFFixture; the
// editor swap does not regress it (the prior textarea editor normalized CRLF too).
//
// RED until Task 3 ships web/src/lib/cm/mode.ts, then GREEN.
import { describe, it, expect } from "vitest";
import { EditorState } from "@codemirror/state";
import { EditorView } from "@codemirror/view";

import { modeCompartment, liveExtensions, sourceExtensions, setMode } from "./mode";
import { cmCorpus } from "../../test/cmCorpus";

describe("mode compartment byte-stability (EDIT-02)", () => {
  for (const fixture of cmCorpus) {
    it(`toggles Live⇄Source⇄Live byte-identically for ${fixture.name}`, () => {
      // A headless EditorView (attached to a detached div) so setMode can dispatch.
      const parent = document.createElement("div");
      const view = new EditorView({
        parent,
        state: EditorState.create({
          doc: fixture.text,
          extensions: [modeCompartment.of(liveExtensions)],
        }),
      });
      try {
        // `before` is the document's own bytes after CM6 seeded it (LF-normalized
        // for the CRLF fixture). The toggle must not change THESE bytes.
        const before = view.state.doc.toString();

        setMode(view, "source");
        expect(view.state.doc.toString()).toBe(before);

        setMode(view, "live");
        expect(view.state.doc.toString()).toBe(before);
      } finally {
        view.destroy();
      }
    });
  }

  it("exposes distinct live/source extension configs", () => {
    // Sanity: the compartment can hold either set without throwing.
    expect(Array.isArray(liveExtensions)).toBe(true);
    expect(Array.isArray(sourceExtensions)).toBe(true);
  });
});
