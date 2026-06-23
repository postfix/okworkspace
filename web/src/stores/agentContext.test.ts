/**
 * agentContext store (AGNT-02 / AGNT-03 / AGNT-06 / AGNT-07 UI wiring). This is
 * the ephemeral cross-component channel that carries the live editor selection
 * (text + length) and the chosen attachment from route children up to the
 * AppShell agent session. Unlike agentPanel/editorMode it is NOT persisted —
 * each session starts with no selection and no attachment.
 */
import { describe, it, expect, beforeEach } from "vitest";

import { useAgentContext } from "./agentContext";

beforeEach(() => {
  // Reset to defaults between tests (the store is module-singleton).
  useAgentContext.getState().clearSelection();
  useAgentContext.getState().clearAttachment();
});

describe("agentContext store", () => {
  it("defaults to an empty selection and no attachment", () => {
    const s = useAgentContext.getState();
    expect(s.selection).toBe("");
    expect(s.selectionLength).toBe(0);
    expect(s.attachment).toBeNull();
  });

  it("setSelection stores the verbatim text and a non-zero length", () => {
    useAgentContext.getState().setSelection("hello world");
    const s = useAgentContext.getState();
    expect(s.selection).toBe("hello world");
    // Raw character count (not trimmed) drives the "Selection (N chars)" chip.
    expect(s.selectionLength).toBe("hello world".length);
  });

  it("setSelection counts the RAW length (leading/trailing whitespace included)", () => {
    useAgentContext.getState().setSelection("  ab  ");
    expect(useAgentContext.getState().selectionLength).toBe(6);
  });

  it("clearSelection resets the selection back to empty / 0", () => {
    useAgentContext.getState().setSelection("something");
    useAgentContext.getState().clearSelection();
    const s = useAgentContext.getState();
    expect(s.selection).toBe("");
    expect(s.selectionLength).toBe(0);
  });

  it("setSelection('') clears the selection back to empty / 0", () => {
    useAgentContext.getState().setSelection("x");
    useAgentContext.getState().setSelection("");
    const s = useAgentContext.getState();
    expect(s.selection).toBe("");
    expect(s.selectionLength).toBe(0);
  });

  it("setAttachment sets the attachment context; clearAttachment resets it to null", () => {
    useAgentContext.getState().setAttachment({ id: "att-1", name: "notes.pdf" });
    expect(useAgentContext.getState().attachment).toEqual({
      id: "att-1",
      name: "notes.pdf",
    });
    useAgentContext.getState().clearAttachment();
    expect(useAgentContext.getState().attachment).toBeNull();
  });

  it("keeps selection and attachment independent (setting one preserves the other)", () => {
    useAgentContext.getState().setAttachment({ id: "att-2", name: "spec.docx" });
    useAgentContext.getState().setSelection("a non-empty selection");
    const s = useAgentContext.getState();
    expect(s.selection).toBe("a non-empty selection");
    expect(s.attachment).toEqual({ id: "att-2", name: "spec.docx" });

    // Clearing the selection must not disturb the attachment.
    useAgentContext.getState().clearSelection();
    expect(useAgentContext.getState().selection).toBe("");
    expect(useAgentContext.getState().attachment).toEqual({
      id: "att-2",
      name: "spec.docx",
    });
  });

  it("is NOT persisted (no localStorage key written for it)", () => {
    useAgentContext.getState().setSelection("ephemeral");
    useAgentContext.getState().setAttachment({ id: "x", name: "y" });
    // The persisted stores use okf.* keys; the ephemeral context store must not.
    const keys = Object.keys(localStorage);
    expect(keys.some((k) => k.includes("agentContext"))).toBe(false);
    expect(keys.some((k) => k.includes("agent.context"))).toBe(false);
  });
});
