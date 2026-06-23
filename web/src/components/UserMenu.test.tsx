/**
 * AUTH-02 — "Log out" is reachable from the UserMenu on any authenticated page.
 * Clicking it invokes the supplied onLogout callback.
 */
import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import UserMenu from "./UserMenu";

function renderMenu(overrides: Partial<Parameters<typeof UserMenu>[0]> = {}) {
  const onProfile = vi.fn();
  const onLogout = vi.fn();
  render(
    <UserMenu
      displayName="Alice"
      onProfile={onProfile}
      onLogout={onLogout}
      {...overrides}
    />,
  );
  return { onProfile, onLogout };
}

describe("UserMenu — AUTH-02", () => {
  it("shows the display name on the trigger button", () => {
    renderMenu({ displayName: "Bob" });
    expect(screen.getByRole("button", { name: /Bob/i })).toBeInTheDocument();
  });

  it("does not show Log out before the menu is opened", () => {
    renderMenu();
    expect(screen.queryByRole("menuitem", { name: /log out/i })).toBeNull();
  });

  it("reveals Log out after clicking the trigger", async () => {
    renderMenu();
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: /Alice/i }));
    expect(
      screen.getByRole("menuitem", { name: /log out/i }),
    ).toBeInTheDocument();
  });

  it("calls onLogout when Log out is clicked", async () => {
    const { onLogout } = renderMenu();
    const user = userEvent.setup();
    // Open the menu first.
    await user.click(screen.getByRole("button", { name: /Alice/i }));
    await user.click(screen.getByRole("menuitem", { name: /log out/i }));
    expect(onLogout).toHaveBeenCalledOnce();
  });

  it("closes the popover after Log out is clicked", async () => {
    renderMenu();
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: /Alice/i }));
    await user.click(screen.getByRole("menuitem", { name: /log out/i }));
    expect(screen.queryByRole("menu")).toBeNull();
  });
});
