/**
 * GRAPH-03 — DepthControl: the Local-graph panel's 1/2/3 hop selector. Asserts it
 * renders the `Depth` label, offers the three options, reflects the current slice
 * depth, and drives setDepth (clamped) on change. Mirrors the EdgeToggles.test.tsx
 * chrome-assertion shape (DOM + the zustand slice, no canvas).
 */
import { describe, it, expect, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import { useLocalGraphPanel } from "../../stores/localGraphPanel";
import DepthControl from "./DepthControl";

describe("DepthControl (GRAPH-03)", () => {
  beforeEach(() => {
    localStorage.clear();
    useLocalGraphPanel.setState({ open: false, depth: 1 });
  });

  it("renders the Depth label and the 1/2/3 options", () => {
    render(<DepthControl />);
    expect(screen.getByText("Depth")).toBeInTheDocument();
    const select = screen.getByRole("combobox", { name: "Depth" });
    expect(select).toBeInTheDocument();
    const options = screen.getAllByRole("option") as HTMLOptionElement[];
    expect(options.map((o) => o.value)).toEqual(["1", "2", "3"]);
  });

  it("reflects the current slice depth as the selected value", () => {
    useLocalGraphPanel.setState({ depth: 2 });
    render(<DepthControl />);
    const select = screen.getByRole("combobox", {
      name: "Depth",
    }) as HTMLSelectElement;
    expect(select.value).toBe("2");
  });

  it("calls setDepth on change (slice updates to the chosen hop)", async () => {
    const user = userEvent.setup();
    render(<DepthControl />);
    const select = screen.getByRole("combobox", { name: "Depth" });

    await user.selectOptions(select, "3");
    expect(useLocalGraphPanel.getState().depth).toBe(3);

    await user.selectOptions(select, "2");
    expect(useLocalGraphPanel.getState().depth).toBe(2);
  });
});
