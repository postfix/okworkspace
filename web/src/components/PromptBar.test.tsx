/**
 * PromptBar — the agent entry point. These assertions cover the load-bearing
 * UI-SPEC contracts: editor-gated modes (readers never reach Propose), the
 * fail-closed agent-off state (disabled + explanation, never a silent hang),
 * Enter-submits / Shift+Enter-newlines, and the in-flight Stop affordance.
 */
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import PromptBar from "./PromptBar";

function baseProps() {
  return {
    mode: "ask" as const,
    onModeChange: vi.fn(),
    canEdit: true,
    hasPage: true,
    pageTitle: "This page",
    workspace: false,
    onWorkspaceToggle: vi.fn(),
    status: "idle" as const,
    onSubmit: vi.fn(),
    onCancel: vi.fn(),
  };
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe("PromptBar", () => {
  it("submits the trimmed prompt on Enter (Shift+Enter inserts a newline)", async () => {
    const user = userEvent.setup();
    const props = baseProps();
    render(<PromptBar {...props} />);

    const input = screen.getByLabelText("Prompt");
    await user.click(input);
    // Shift+Enter must NOT submit (it inserts a newline).
    await user.keyboard("line one{Shift>}{Enter}{/Shift}line two");
    expect(props.onSubmit).not.toHaveBeenCalled();
    // A plain Enter submits the whole prompt.
    await user.keyboard("{Enter}");
    expect(props.onSubmit).toHaveBeenCalledTimes(1);
    expect(props.onSubmit.mock.calls[0][0]).toContain("line one");
    expect(props.onSubmit.mock.calls[0][0]).toContain("line two");
  });

  it("disables Propose for readers (editor-gated) and enables it for editors", () => {
    const reader = { ...baseProps(), canEdit: false };
    const { rerender } = render(<PromptBar {...reader} />);
    const proposeReader = screen.getByRole("option", {
      name: "Propose a patch",
    }) as HTMLOptionElement;
    expect(proposeReader.disabled).toBe(true);

    rerender(<PromptBar {...baseProps()} canEdit hasPage workspace={false} />);
    const proposeEditor = screen.getByRole("option", {
      name: "Propose a patch",
    }) as HTMLOptionElement;
    expect(proposeEditor.disabled).toBe(false);
  });

  it("fails closed when the agent is off: disabled controls + explanation", () => {
    render(
      <PromptBar
        {...baseProps()}
        disabledReason="The assistant is turned off."
      />,
    );

    expect(screen.getByText("The assistant is turned off.")).toBeTruthy();
    expect((screen.getByLabelText("Prompt") as HTMLTextAreaElement).disabled).toBe(
      true,
    );
    expect(
      (screen.getByLabelText("Assistant mode") as HTMLSelectElement).disabled,
    ).toBe(true);
  });

  it("shows a Stop affordance while in-flight and cancels on click", async () => {
    const user = userEvent.setup();
    const props = { ...baseProps(), status: "streaming" as const };
    render(<PromptBar {...props} />);

    const stop = screen.getByRole("button", { name: "Stop" });
    await user.click(stop);
    expect(props.onCancel).toHaveBeenCalledTimes(1);
    // The streaming status label is announced.
    expect(screen.getByText("Streaming…")).toBeTruthy();
  });

  it("reflects the Whole-workspace toggle in the read-only context chip", () => {
    const { rerender } = render(<PromptBar {...baseProps()} workspace={false} />);
    // Page scope by default.
    expect(screen.getByTitle("This page")).toBeTruthy();
    rerender(<PromptBar {...baseProps()} workspace />);
    expect(screen.getByTitle("Whole workspace")).toBeTruthy();
  });
});
