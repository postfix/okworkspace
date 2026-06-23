/**
 * PAGE-04/05/08 — the read-mode PageActionMenu exposes Edit/Rename/Move/Version
 * history/Delete with an accessible "Page actions" trigger; mutating items are
 * RBAC-gated (readers do not see them); the popover closes on Esc.
 * Also covers the LinkPicker emitting a relative `.md` link (PAGE-08).
 */
import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import PageActionMenu from "./PageActionMenu";
import { relativeMdLink } from "../api/client";

function renderMenu(canEdit: boolean) {
  const handlers = {
    onEdit: vi.fn(),
    onRename: vi.fn(),
    onMove: vi.fn(),
    onHistory: vi.fn(),
    onDelete: vi.fn(),
  };
  render(<PageActionMenu canEdit={canEdit} {...handlers} />);
  return handlers;
}

describe("PageActionMenu", () => {
  it("trigger has the accessible name 'Page actions'", () => {
    renderMenu(true);
    expect(
      screen.getByRole("button", { name: /page actions/i }),
    ).toBeInTheDocument();
  });

  it("does not show menu items before the trigger is clicked", () => {
    renderMenu(true);
    expect(screen.queryByRole("menuitem", { name: /rename/i })).toBeNull();
  });

  it("shows Edit, Rename, Move, Version history, Delete for an editor", async () => {
    renderMenu(true);
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: /page actions/i }));
    expect(screen.getByRole("menuitem", { name: /^edit$/i })).toBeInTheDocument();
    expect(screen.getByRole("menuitem", { name: /rename/i })).toBeInTheDocument();
    expect(screen.getByRole("menuitem", { name: /move/i })).toBeInTheDocument();
    expect(
      screen.getByRole("menuitem", { name: /version history/i }),
    ).toBeInTheDocument();
    expect(screen.getByRole("menuitem", { name: /delete/i })).toBeInTheDocument();
  });

  it("hides the mutating items for a reader (RBAC)", async () => {
    renderMenu(false);
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: /page actions/i }));
    // Read-only items remain.
    expect(screen.getByRole("menuitem", { name: /^edit$/i })).toBeInTheDocument();
    expect(
      screen.getByRole("menuitem", { name: /version history/i }),
    ).toBeInTheDocument();
    // Mutating items are absent.
    expect(screen.queryByRole("menuitem", { name: /rename/i })).toBeNull();
    expect(screen.queryByRole("menuitem", { name: /move/i })).toBeNull();
    expect(screen.queryByRole("menuitem", { name: /delete/i })).toBeNull();
  });

  it("invokes the matching handler and closes the popover", async () => {
    const handlers = renderMenu(true);
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: /page actions/i }));
    await user.click(screen.getByRole("menuitem", { name: /rename/i }));
    expect(handlers.onRename).toHaveBeenCalledOnce();
    expect(screen.queryByRole("menu")).toBeNull();
  });

  it("closes on Esc", async () => {
    renderMenu(true);
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: /page actions/i }));
    expect(screen.getByRole("menu")).toBeInTheDocument();
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("menu")).toBeNull();
  });
});

describe("relativeMdLink (PAGE-08 link picker emits a relative .md path)", () => {
  it("computes a sibling link", () => {
    expect(relativeMdLink("runbooks/a.md", "runbooks/b.md")).toBe("b.md");
  });
  it("computes an across-folder link with ../", () => {
    expect(relativeMdLink("architecture/x.md", "runbooks/deploy.md")).toBe(
      "../runbooks/deploy.md",
    );
  });
  it("computes a nested-down link from root", () => {
    expect(relativeMdLink("index.md", "runbooks/deploy.md")).toBe(
      "runbooks/deploy.md",
    );
  });
  it("never emits a wiki-style [[...]] or ID link", () => {
    const dest = relativeMdLink("a/x.md", "b/y.md");
    expect(dest).not.toContain("[[");
    expect(dest).toMatch(/\.md$/);
  });
});
