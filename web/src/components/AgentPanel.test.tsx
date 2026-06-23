/**
 * AgentPanel — the right-side Assistant chat column. Contracts: it is a labeled
 * "Assistant" landmark, shows the empty state before a prompt, renders the streamed
 * answer after, docks the prompt at the bottom, and surfaces quick-action
 * suggestions (summarize / rewrite / draft / propose) inside the window.
 */
import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";

import AgentPanel from "./AgentPanel";

function baseProps() {
  return {
    open: true,
    onClose: vi.fn(),
    mode: "ask" as const,
    scopeLabel: "This page",
    status: "idle" as const,
    answer: "",
    currentPath: "guides/welcome.md",
    submitted: false,
    suggestions: [],
    promptBar: <div data-testid="dock-prompt">prompt</div>,
  };
}

function renderPanel(ui: React.ReactElement) {
  return render(<MemoryRouter>{ui}</MemoryRouter>);
}

describe("AgentPanel", () => {
  it("renders nothing when collapsed", () => {
    const { container } = renderPanel(
      <AgentPanel {...baseProps()} open={false} />,
    );
    expect(container.querySelector(".agentpanel")).toBeNull();
  });

  it("is a labeled Assistant landmark with the empty state + docked prompt", () => {
    renderPanel(<AgentPanel {...baseProps()} />);
    expect(screen.getByRole("complementary", { name: "Assistant" })).toBeTruthy();
    expect(screen.getByText("Ask the assistant anything")).toBeTruthy();
    // The chat input is docked inside the panel (not a full-width app footer).
    expect(screen.getByTestId("dock-prompt")).toBeInTheDocument();
  });

  it("renders the streamed answer and the mode·scope meta after submit", () => {
    renderPanel(
      <AgentPanel {...baseProps()} submitted answer="The streamed answer." />,
    );
    expect(screen.getByText("The streamed answer.")).toBeTruthy();
    expect(screen.getByText("Ask · This page")).toBeTruthy();
  });

  it("surfaces quick-action suggestions inside the window and runs them", async () => {
    const user = userEvent.setup();
    const run = vi.fn();
    renderPanel(
      <AgentPanel
        {...baseProps()}
        suggestions={[
          { key: "summarize", label: "Summarize this page", run },
          { key: "draft", label: "Draft a page", run: vi.fn() },
        ]}
      />,
    );
    const chip = screen.getByRole("button", { name: "Summarize this page" });
    await user.click(chip);
    expect(run).toHaveBeenCalledTimes(1);
    expect(
      screen.getByRole("button", { name: "Draft a page" }),
    ).toBeInTheDocument();
  });

  it("renders no suggestions row when there are none (readers with nothing actionable)", () => {
    const { container } = renderPanel(
      <AgentPanel {...baseProps()} suggestions={[]} />,
    );
    expect(container.querySelector(".agentpanel-suggestions")).toBeNull();
  });

  it("collapses via the header toggle", async () => {
    const user = userEvent.setup();
    const props = baseProps();
    renderPanel(<AgentPanel {...props} />);
    await user.click(screen.getByRole("button", { name: "Hide assistant" }));
    expect(props.onClose).toHaveBeenCalledTimes(1);
  });
});
