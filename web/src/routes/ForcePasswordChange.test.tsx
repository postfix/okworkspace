/**
 * D-02 — ForcePasswordChange gates the app when must_change_password is set.
 * Validates ≥12 chars and password match before calling changePassword.
 */
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

// Mock the api/client module so no real fetch calls happen.
vi.mock("../api/client", () => ({
  changePassword: vi.fn(),
  me: vi.fn(),
}));

import * as client from "../api/client";
import ForcePasswordChange from "./ForcePasswordChange";

function renderFPC() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <ForcePasswordChange />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

// Helper to get inputs by their stable IDs.
const getCurrentInput = () => screen.getByLabelText(/temporary password/i);
const getNewInput = () => screen.getByLabelText(/^new password$/i);
const getConfirmInput = () => screen.getByLabelText(/confirm new password/i);

describe("ForcePasswordChange — D-02", () => {
  beforeEach(() => {
    vi.resetAllMocks();
  });

  it("renders the forced-change form with all three password fields", () => {
    renderFPC();
    expect(getCurrentInput()).toBeInTheDocument();
    expect(getNewInput()).toBeInTheDocument();
    expect(getConfirmInput()).toBeInTheDocument();
  });

  it("shows an error when the new password is shorter than 12 characters", async () => {
    renderFPC();
    const user = userEvent.setup();
    await user.type(getCurrentInput(), "tmp");
    await user.type(getNewInput(), "short");
    await user.type(getConfirmInput(), "short");
    await user.click(screen.getByRole("button", { name: /update password/i }));
    expect(await screen.findByRole("alert")).toHaveTextContent(/at least 12/i);
    expect(client.changePassword).not.toHaveBeenCalled();
  });

  it("shows an error when the new passwords do not match", async () => {
    renderFPC();
    const user = userEvent.setup();
    await user.type(getCurrentInput(), "tmp");
    await user.type(getNewInput(), "longenoughpass1");
    await user.type(getConfirmInput(), "longenoughpass2");
    await user.click(screen.getByRole("button", { name: /update password/i }));
    expect(await screen.findByRole("alert")).toHaveTextContent(/don't match/i);
    expect(client.changePassword).not.toHaveBeenCalled();
  });

  it("calls changePassword when validation passes", async () => {
    const changePasswordMock = vi.mocked(client.changePassword);
    const meMock = vi.mocked(client.me);
    changePasswordMock.mockResolvedValue(undefined);
    meMock.mockResolvedValue({
      username: "alice",
      display_name: "Alice",
      role: "editor",
      must_change_password: false,
    });

    renderFPC();
    const user = userEvent.setup();
    await user.type(getCurrentInput(), "tmpPassword!");
    await user.type(getNewInput(), "newStrongPass1!");
    await user.type(getConfirmInput(), "newStrongPass1!");
    await user.click(screen.getByRole("button", { name: /update password/i }));

    expect(changePasswordMock).toHaveBeenCalledWith(
      "tmpPassword!",
      "newStrongPass1!",
    );
  });
});
