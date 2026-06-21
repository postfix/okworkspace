// EDIT-02 — toggling Live⇄Source⇄Live never mutates the document bytes. This is a
// STATE-level test (no DOM): build an EditorState per okf corpus fixture, flip the
// modeCompartment via setMode, and assert state.doc.toString() is byte-identical
// before and after. Byte-identity is by construction (the toggle dispatch carries
// `effects` only, never `changes`) — this test is the guard that it stays so.
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
        const before = view.state.doc.toString();
        expect(before).toBe(fixture.text);

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
