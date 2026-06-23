/**
 * PromptBar — the chat input docked in the Assistant panel. These assertions
 * cover the load-bearing contracts of the Cursor-style entry point: Enter submits
 * (Shift+Enter inserts a newline), the input clears after a send, Esc cancels, the
 * fail-closed agent-off state (disabled + explanation, never a silent hang), and
 * the muted context/error hints. There are no mode/scope/submit buttons.
 */
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import PromptBar from "./PromptBar";

function baseProps() {
  return {
    placeholder: "Ask anything…",
    contextLabel: "This page",
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

  it("clears the input after a successful send", async () => {
    const user = userEvent.setup();
    const props = baseProps();
    render(<PromptBar {...props} />);

    const input = screen.getByLabelText("Prompt") as HTMLTextAreaElement;
    await user.click(input);
    await user.keyboard("what is this page about?{Enter}");
    expect(props.onSubmit).toHaveBeenCalledWith("what is this page about?");
    expect(input.value).toBe("");
  });

  it("does not submit a blank/whitespace-only prompt", async () => {
    const user = userEvent.setup();
    const props = baseProps();
    render(<PromptBar {...props} />);

    const input = screen.getByLabelText("Prompt");
    await user.click(input);
    await user.keyboard("   {Enter}");
    expect(props.onSubmit).not.toHaveBeenCalled();
  });

  it("cancels on Escape (stop stream / clear primed action)", async () => {
    const user = userEvent.setup();
    const props = { ...baseProps(), status: "streaming" as const };
    render(<PromptBar {...props} />);

    const input = screen.getByLabelText("Prompt");
    await user.click(input);
    await user.keyboard("{Escape}");
    expect(props.onCancel).toHaveBeenCalledTimes(1);
  });

  it("fails closed when the agent is off: disabled input + explanation", () => {
    const props = {
      ...baseProps(),
      disabledReason: "The assistant is turned off.",
    };
    render(<PromptBar {...props} />);

    expect(screen.getByLabelText("Prompt")).toBeDisabled();
    expect(screen.getByText("The assistant is turned off.")).toBeInTheDocument();
  });

  it("shows a transient error note when a request fails", () => {
    const props = { ...baseProps(), error: "The assistant couldn't answer. Try again." };
    render(<PromptBar {...props} />);

    expect(
      screen.getByText("The assistant couldn't answer. Try again."),
    ).toBeInTheDocument();
  });

  it("shows the muted context label and a streaming hint", () => {
    const props = {
      ...baseProps(),
      contextLabel: "Whole workspace",
      status: "streaming" as const,
    };
    render(<PromptBar {...props} />);

    expect(screen.getByText("Whole workspace")).toBeInTheDocument();
    expect(screen.getByText(/Streaming/)).toBeInTheDocument();
  });
});
