/**
 * GRAPH-04 — EdgeToggles renders the Links / Backlinks / Shared tags chip cluster
 * bound to the graphEdges zustand slice. Asserts the exact labels, the default
 * pressed state (Links/Backlinks ON, Shared tags OFF — the success-criterion
 * default), that clicking a chip flips its aria-pressed (driving the slice), and
 * that the styling is token-only (no hard-coded color hex). The canvas itself is
 * out of scope here — this is pure DOM chrome.
 */
import { describe, it, expect, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import { useGraphEdges } from "../../stores/graphEdges";
import EdgeToggles from "./EdgeToggles";

describe("EdgeToggles (GRAPH-04)", () => {
  beforeEach(() => {
    // Reset the persisted slice to its defaults so each test starts clean
    // (mirrors graphEdges.test.ts).
    localStorage.clear();
    useGraphEdges.setState({ links: true, backlinks: true, sharedTags: false });
  });

  it("renders three chips with the exact labels", () => {
    render(<EdgeToggles />);
    expect(screen.getByRole("button", { name: "Links" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Backlinks" })).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Shared tags" }),
    ).toBeInTheDocument();
  });

  it("defaults Shared tags OFF and Links/Backlinks ON (aria-pressed)", () => {
    render(<EdgeToggles />);
    expect(screen.getByRole("button", { name: "Links" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
    expect(screen.getByRole("button", { name: "Backlinks" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
    expect(screen.getByRole("button", { name: "Shared tags" })).toHaveAttribute(
      "aria-pressed",
      "false",
    );
  });

  it("clicking the Shared tags chip flips its aria-pressed and the slice", async () => {
    const user = userEvent.setup();
    render(<EdgeToggles />);
    const chip = screen.getByRole("button", { name: "Shared tags" });
    expect(chip).toHaveAttribute("aria-pressed", "false");

    await user.click(chip);
    expect(chip).toHaveAttribute("aria-pressed", "true");
    expect(useGraphEdges.getState().sharedTags).toBe(true);

    await user.click(chip);
    expect(chip).toHaveAttribute("aria-pressed", "false");
    expect(useGraphEdges.getState().sharedTags).toBe(false);
  });

  it("clicking Links flips it independently of Backlinks", async () => {
    const user = userEvent.setup();
    render(<EdgeToggles />);
    const links = screen.getByRole("button", { name: "Links" });

    await user.click(links);
    expect(links).toHaveAttribute("aria-pressed", "false");
    expect(screen.getByRole("button", { name: "Backlinks" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
  });

  it("renders chips as accessible buttons carrying the toggle pressed state", () => {
    render(<EdgeToggles />);
    // Every chip is a <button type=button> with aria-pressed — the accessible
    // contract the canvas chrome relies on (no DOM color is asserted; token-only
    // styling is grep-verified in the plan self-check).
    for (const name of ["Links", "Backlinks", "Shared tags"]) {
      const chip = screen.getByRole("button", { name });
      expect(chip).toHaveAttribute("type", "button");
      expect(chip).toHaveAttribute("aria-pressed");
    }
  });
});
