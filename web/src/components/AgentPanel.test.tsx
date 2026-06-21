/**
 * AgentPanel — the right-side collapsible answer column. Contracts: it is a
 * labeled "Assistant" landmark, shows the empty state before a prompt, renders
 * the streamed answer after, and the editor-gated "Propose this as a patch"
 * footer appears ONLY for editors with a page in scope (readers never see it).
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
    canPropose: true,
    onProposeFromAnswer: vi.fn(),
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

  it("is a labeled Assistant landmark with the empty state before a prompt", () => {
    renderPanel(<AgentPanel {...baseProps()} />);
    expect(screen.getByRole("complementary", { name: "Assistant" })).toBeTruthy();
    expect(screen.getByText("Ask the assistant anything")).toBeTruthy();
  });

  it("renders the streamed answer and the mode·scope meta after submit", () => {
    renderPanel(
      <AgentPanel
        {...baseProps()}
        submitted
        answer="The streamed answer."
      />,
    );
    expect(screen.getByText("The streamed answer.")).toBeTruthy();
    expect(screen.getByText("Ask · This page")).toBeTruthy();
  });

  it("shows the propose footer for editors (page scope) and fires the handler", async () => {
    const user = userEvent.setup();
    const props = { ...baseProps(), submitted: true, answer: "Done." };
    renderPanel(<AgentPanel {...props} />);
    const propose = screen.getByRole("button", {
      name: /propose this as a patch/i,
    });
    await user.click(propose);
    expect(props.onProposeFromAnswer).toHaveBeenCalledTimes(1);
  });

  it("hides the propose footer for readers (canPropose=false)", () => {
    renderPanel(
      <AgentPanel
        {...baseProps()}
        submitted
        answer="Done."
        canPropose={false}
      />,
    );
    expect(
      screen.queryByRole("button", { name: /propose this as a patch/i }),
    ).toBeNull();
  });

  it("collapses via the header toggle", async () => {
    const user = userEvent.setup();
    const props = baseProps();
    renderPanel(<AgentPanel {...props} />);
    await user.click(screen.getByRole("button", { name: "Hide assistant" }));
    expect(props.onClose).toHaveBeenCalledTimes(1);
  });
});
