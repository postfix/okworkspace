// mode — the Live/Source toggle seam (EDIT-02). Both modes share ONE EditorState
// document; a Compartment holds either the live-preview extension set or the
// source-mode set, and toggling reconfigures that compartment with a dispatch that
// carries `effects` ONLY (never `changes`). Because the document transaction is
// never touched, `doc.toString()` is byte-identical across Live⇄Source⇄Live by
// construction — that is the EDIT-02 guarantee mode.test.ts pins over the corpus.
//
// This plan keeps `liveExtensions` minimal: "Live" mode is functionally raw text
// plus the markdown language + theme seam. The actual inline-render ViewPlugin and
// image/table widgets land in 06-02/06-03, which extend `liveExtensions` here.
import { Compartment } from "@codemirror/state";
import { EditorView, keymap } from "@codemirror/view";
import type { Extension } from "@codemirror/state";

import { markdownExtension } from "./markdown";
import { theme, sourceTheme } from "./theme";
import { useEditorMode } from "../../stores/editorMode";

// modeCompartment holds the mode-specific extension set; reconfiguring it is the
// whole toggle (no document edit).
export const modeCompartment = new Compartment();

// liveExtensions: the Live-preview configuration. 06-02 appends the livePreview
// ViewPlugin (inline decorations) and 06-03 the image/table widgets; this plan
// ships the markdown language + theme so the seam exists and the tree parses.
export const liveExtensions: Extension[] = [markdownExtension, theme];

// sourceExtensions: raw Markdown, monospace document-wide, no live-preview
// decorations. The markdown language stays loaded (syntax-aware editing) but no
// reveal/hide decorations are applied.
export const sourceExtensions: Extension[] = [markdownExtension, theme, sourceTheme];

// setMode flips the compartment WITHOUT touching the document. The dispatch carries
// `effects` only — this is the byte-identity guarantee (EDIT-02). Never add
// `changes` here.
export function setMode(view: EditorView, mode: "live" | "source"): void {
  view.dispatch({
    effects: modeCompartment.reconfigure(
      mode === "live" ? liveExtensions : sourceExtensions,
    ),
  });
}

// toggleKeymap binds Cmd/Ctrl-E (CM6 `Mod-e` maps Cmd on macOS, Ctrl elsewhere) to
// flip the persisted editor-mode store and reconfigure the compartment to match.
// Reading the store after toggle() (rather than from a stale closure) keeps the
// keymap and the React toolbar control on the same single source of truth.
export const toggleKeymap = keymap.of([
  {
    key: "Mod-e",
    run: (view) => {
      useEditorMode.getState().toggle();
      setMode(view, useEditorMode.getState().mode);
      return true;
    },
  },
]);
