// editorMode store (EDIT-02). The Live/Source editor mode is a per-device UI
// preference (not server state), persisted to localStorage so the last-used mode
// survives a reload. Default is "live" (the Obsidian-style live-preview surface).
// Mirrors the recent.ts zustand+persist pattern; the persisted key is
// "okf.editor.mode". The Cmd/Ctrl-E keymap (web/src/lib/cm/mode.ts) and the
// PageEditor toolbar toggle both drive this single source of truth.
import { create } from "zustand";
import { persist } from "zustand/middleware";

// EditorMode is the live-preview surface mode: "live" renders Markdown inline,
// "source" shows raw monospace text. Both share ONE document — see mode.ts.
export type EditorMode = "live" | "source";

interface EditorModeState {
  mode: EditorMode;
  // setMode sets the mode explicitly (e.g. the toolbar segmented control).
  setMode: (mode: EditorMode) => void;
  // toggle flips live⇄source (the Cmd/Ctrl-E shortcut).
  toggle: () => void;
}

export const useEditorMode = create<EditorModeState>()(
  persist(
    (set) => ({
      mode: "live",
      setMode: (mode) => set({ mode }),
      toggle: () => set((s) => ({ mode: s.mode === "live" ? "source" : "live" })),
    }),
    { name: "okf.editor.mode" },
  ),
);
