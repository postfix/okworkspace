/**
 * Admin — the "Tag suggestions" sweep-start section under test (TAG-05).
 * These assertions lock the sweep-start control's contract:
 *
 *   1. the section renders the heading + the scope toggle + the start button.
 *   2. clicking with the toggle OFF calls startTagSweep({all:false}) and shows the
 *      untagged confirmation carrying the queued count.
 *   3. queued===0 shows the "every page already has tags" line (no error).
 *   4. toggling ON calls startTagSweep({all:true}) and shows the all-scope copy.
 *   5. a rejected mutation shows the generic error line (zero infra vocabulary).
 */
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

vi.mock("../api/client", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../api/client")>();
  return {
    ...actual,
    me: vi.fn(),
    listUsers: vi.fn(),
    startTagSweep: vi.fn(),
  };
});

import * as client from "../api/client";
import Admin from "./Admin";

function renderAdmin() {
  vi.mocked(client.me).mockResolvedValue({
    username: "admin",
    display_name: "Admin",
    role: "admin",
    must_change_password: false,
  });
  vi.mocked(client.listUsers).mockResolvedValue([]);
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  qc.setQueryData(["me"], {
    username: "admin",
    display_name: "Admin",
    role: "admin",
    must_change_password: false,
  });
  render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <Admin />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  return qc;
}

describe("Admin — Tag suggestions sweep-start (TAG-05)", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders the sweep section heading + scope toggle + start button", async () => {
    renderAdmin();
    expect(
      await screen.findByRole("heading", { name: "Tag suggestions" }),
    ).toBeTruthy();
    expect(
      screen.getByRole("checkbox", {
        name: /include pages that already have tags/i,
      }),
    ).toBeTruthy();
    expect(
      screen.getByRole("button", { name: /suggest tags for pages/i }),
    ).toBeTruthy();
  });

  it("starts an untagged-scope sweep and shows the queued-count confirmation", async () => {
    vi.mocked(client.startTagSweep).mockResolvedValue({ queued: 7 });
    const user = userEvent.setup();
    renderAdmin();

    await user.click(
      await screen.findByRole("button", { name: /suggest tags for pages/i }),
    );

    await waitFor(() =>
      expect(client.startTagSweep).toHaveBeenCalledWith({ all: false }),
    );
    expect(
      await screen.findByText(
        "Started suggesting tags for untagged pages — 7 pages queued for review.",
      ),
    ).toBeTruthy();
  });

  it("shows the 'every page already has tags' line when queued===0", async () => {
    vi.mocked(client.startTagSweep).mockResolvedValue({ queued: 0 });
    const user = userEvent.setup();
    renderAdmin();

    await user.click(
      await screen.findByRole("button", { name: /suggest tags for pages/i }),
    );

    expect(
      await screen.findByText("Every page already has tags — nothing to suggest."),
    ).toBeTruthy();
    // queued===0 is NOT an error.
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("toggling the scope on starts an all-pages sweep with the all-scope copy", async () => {
    vi.mocked(client.startTagSweep).mockResolvedValue({ queued: 12 });
    const user = userEvent.setup();
    renderAdmin();

    await user.click(
      await screen.findByRole("checkbox", {
        name: /include pages that already have tags/i,
      }),
    );
    await user.click(
      screen.getByRole("button", { name: /suggest tags for pages/i }),
    );

    await waitFor(() =>
      expect(client.startTagSweep).toHaveBeenCalledWith({ all: true }),
    );
    expect(
      await screen.findByText(
        "Started suggesting tags — 12 pages queued for review.",
      ),
    ).toBeTruthy();
  });

  it("shows the generic error line on a rejected sweep (no infra vocabulary)", async () => {
    vi.mocked(client.startTagSweep).mockRejectedValue(new Error("boom"));
    const user = userEvent.setup();
    renderAdmin();

    await user.click(
      await screen.findByRole("button", { name: /suggest tags for pages/i }),
    );

    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toBe("Couldn’t start the sweep. Try again.");
    // Hidden-infra discipline: no job/queue/worker/index/LLM vocabulary.
    expect(alert.textContent).not.toMatch(/job|queue|worker|index|llm|sweep failed/i);
  });
});
