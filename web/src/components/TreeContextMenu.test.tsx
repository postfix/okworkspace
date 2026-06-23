/**
 * TreeContextMenu — the reusable cursor-anchored tree menu renders its passed
 * items as role="menuitem", invokes the matching onSelect (and closes), is
 * keyboard-navigable (arrow keys + Enter), and closes on Escape / outside-click.
 */
import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import TreeContextMenu from "./TreeContextMenu";

describe("TreeContextMenu", () => {
  it("renders the passed items as menuitems", () => {
    render(
      <TreeContextMenu
        x={10}
        y={10}
        onClose={vi.fn()}
        items={[
          { label: "New page here", onSelect: vi.fn() },
          { label: "New folder here", onSelect: vi.fn() },
        ]}
      />,
    );
    expect(screen.getByRole("menu")).toBeInTheDocument();
    expect(
      screen.getByRole("menuitem", { name: "New page here" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("menuitem", { name: "New folder here" }),
    ).toBeInTheDocument();
  });

  it("invokes the item's onSelect and closes on click", async () => {
    const onSelect = vi.fn();
    const onClose = vi.fn();
    render(
      <TreeContextMenu
        x={10}
        y={10}
        onClose={onClose}
        items={[{ label: "Rename", onSelect }]}
      />,
    );
    const user = userEvent.setup();
    await user.click(screen.getByRole("menuitem", { name: "Rename" }));
    expect(onSelect).toHaveBeenCalledOnce();
    expect(onClose).toHaveBeenCalledOnce();
  });

  it("is keyboard-navigable: ArrowDown moves focus, Enter selects", async () => {
    const first = vi.fn();
    const second = vi.fn();
    render(
      <TreeContextMenu
        x={10}
        y={10}
        onClose={vi.fn()}
        items={[
          { label: "First", onSelect: first },
          { label: "Second", onSelect: second },
        ]}
      />,
    );
    const user = userEvent.setup();
    // First item is focused on open.
    expect(screen.getByRole("menuitem", { name: "First" })).toHaveFocus();
    await user.keyboard("{ArrowDown}");
    expect(screen.getByRole("menuitem", { name: "Second" })).toHaveFocus();
    await user.keyboard("{Enter}");
    expect(second).toHaveBeenCalledOnce();
    expect(first).not.toHaveBeenCalled();
  });

  it("closes on Escape", async () => {
    const onClose = vi.fn();
    render(
      <TreeContextMenu
        x={10}
        y={10}
        onClose={onClose}
        items={[{ label: "Delete", onSelect: vi.fn(), danger: true }]}
      />,
    );
    const user = userEvent.setup();
    await user.keyboard("{Escape}");
    expect(onClose).toHaveBeenCalledOnce();
  });

  it("closes on outside click", () => {
    const onClose = vi.fn();
    render(
      <div>
        <button type="button">outside</button>
        <TreeContextMenu
          x={10}
          y={10}
          onClose={onClose}
          items={[{ label: "Move", onSelect: vi.fn() }]}
        />
      </div>,
    );
    fireEvent.mouseDown(screen.getByRole("button", { name: "outside" }));
    expect(onClose).toHaveBeenCalledOnce();
  });

  it("renders a danger item with the destructive class", () => {
    render(
      <TreeContextMenu
        x={10}
        y={10}
        onClose={vi.fn()}
        items={[{ label: "Delete", onSelect: vi.fn(), danger: true }]}
      />,
    );
    expect(screen.getByRole("menuitem", { name: "Delete" })).toHaveClass(
      "treemenu-item-danger",
    );
  });
});
