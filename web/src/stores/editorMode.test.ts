// EDIT-02 — the persisted Live/Source editor-mode store. Mirrors recent.test.ts:
// default value, setMode/toggle, and persistence under the named localStorage key.
import { describe, it, expect, beforeEach } from "vitest";

import { useEditorMode } from "./editorMode";

describe("editorMode store (EDIT-02)", () => {
  beforeEach(() => {
    localStorage.clear();
    useEditorMode.setState({ mode: "live" });
  });

  it("defaults to live mode", () => {
    expect(useEditorMode.getState().mode).toBe("live");
  });

  it("setMode persists the chosen mode", () => {
    useEditorMode.getState().setMode("source");
    expect(useEditorMode.getState().mode).toBe("source");
    useEditorMode.getState().setMode("live");
    expect(useEditorMode.getState().mode).toBe("live");
  });

  it("toggle flips live⇄source", () => {
    expect(useEditorMode.getState().mode).toBe("live");
    useEditorMode.getState().toggle();
    expect(useEditorMode.getState().mode).toBe("source");
    useEditorMode.getState().toggle();
    expect(useEditorMode.getState().mode).toBe("live");
  });

  it("persists under the okf.editor.mode key", () => {
    useEditorMode.getState().setMode("source");
    const raw = localStorage.getItem("okf.editor.mode");
    expect(raw).toBeTruthy();
    expect(raw).toContain("source");
  });
});
