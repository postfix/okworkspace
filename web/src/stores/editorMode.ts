// editorMode store (EDIT-02). The Live/Source editor mode is a per-device UI
// preference (not server state), persisted to localStorage so the last-used mode
// survives a reload. Default is "live" (the Obsidian-style live-preview surface).
// Mirrors the recent.ts zustand+persist pattern; the persisted key is
// "okf.editor.mode". The Cmd/Ctrl-E keymap (web/src/lib/cm/mode.ts) and the
// PageEditor toolbar toggle both drive this single source of truth.
import { create } from "zustand";
import { persist } from "zustand/middleware";

// EditorMode is the editor-surface mode. "live" renders Markdown inline and
// "source" shows raw monospace text — the two CM6 live-preview modes that share
// ONE document (see mode.ts). "tiptap" is the future WYSIWYG surface (Phase 41
// mounts it, Phase 42 exposes it); it is NOT user-selectable yet, so the store
// carries the value but no UI sets it this phase (RE-05/MED-05).
export type EditorMode = "live" | "source" | "tiptap";

interface EditorModeState {
  mode: EditorMode;
  // setMode sets the mode explicitly (e.g. the toolbar segmented control). It is
  // the toolbar's source of truth and accepts all three modes.
  setMode: (mode: EditorMode) => void;
  // toggle flips live⇄source ONLY (the Cmd/Ctrl-E shortcut, a CM6-only affordance).
  // A "tiptap" current value toggles back to "live" — the keymap never toggles
  // INTO tiptap (that is a future React-toolbar-only selection, Phase 42).
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
