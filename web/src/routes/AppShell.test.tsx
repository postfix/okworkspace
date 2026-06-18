/**
 * AUTH-06 — AppShell renders the authenticated user's display_name in the top bar.
 */
import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

// Mock api/client so no real fetch calls happen. AppShell now mounts LeftTree
// (getTree) and the create modals (createPage/createFolder) in the nav rail.
vi.mock("../api/client", () => ({
  me: vi.fn(),
  health: vi.fn(),
  logout: vi.fn(),
  getTree: vi.fn().mockResolvedValue([]),
  createPage: vi.fn(),
  createFolder: vi.fn(),
}));

import * as client from "../api/client";
import AppShell from "./AppShell";

function renderAppShell() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <AppShell />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  return qc;
}

describe("AppShell — AUTH-06", () => {
  it("renders the authenticated user's display_name in the top bar", async () => {
    vi.mocked(client.me).mockResolvedValue({
      username: "alice",
      display_name: "Alice Wonderland",
      role: "editor",
      must_change_password: false,
    });
    vi.mocked(client.health).mockResolvedValue({
      ok: true,
      diverged: false,
      self_healed: false,
      detail: "",
    });

    renderAppShell();

    // The display_name should appear inside the UserMenu trigger in the topbar.
    expect(
      await screen.findByRole("button", { name: /Alice Wonderland/i }),
    ).toBeInTheDocument();
  });

  it("renders OKF Workspace wordmark in the top bar", async () => {
    vi.mocked(client.me).mockResolvedValue({
      username: "bob",
      display_name: "Bob",
      role: "reader",
      must_change_password: false,
    });
    vi.mocked(client.health).mockResolvedValue({
      ok: true,
      diverged: false,
      self_healed: false,
      detail: "",
    });

    renderAppShell();

    expect(
      await screen.findByRole("button", { name: /OKF Workspace/i }),
    ).toBeInTheDocument();
  });
});
