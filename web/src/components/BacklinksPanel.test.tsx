/**
 * LINK-02 — BacklinksPanel renders the collapsible "Referenced by (N)" section at
 * the bottom of the page read view: a populated list of click-to-navigate entries,
 * plus the three quiet muted states (empty / loading / error) with the EXACT
 * UI-SPEC copy, and a working collapse toggle (aria-expanded). Click-navigate is
 * asserted via a location-probe route that renders the current pathname.
 */
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Routes, Route, useLocation } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

vi.mock("../api/client", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../api/client")>();
  return {
    ...actual,
    getBacklinks: vi.fn(),
  };
});

import * as client from "../api/client";
import BacklinksPanel from "./BacklinksPanel";

// LocationProbe surfaces the current pathname so a click-navigate can be asserted
// without spying on the router internals (mirrors the MemoryRouter+Routes harness
// used by the other component/route tests).
function LocationProbe() {
  const loc = useLocation();
  return <div data-testid="location">{loc.pathname}</div>;
}

function renderPanel(path = "notes/current.md") {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={[`/app/page/${path}`]}>
        <BacklinksPanel path={path} />
        <LocationProbe />
        <Routes>
          <Route path="/app/page/*" element={null} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("BacklinksPanel", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders a populated list and click-navigates to the linking page", async () => {
    vi.mocked(client.getBacklinks).mockResolvedValue([
      { path: "notes/a.md", title: "Note A" },
      { path: "notes/b.md", title: "Note B" },
    ]);
    const user = userEvent.setup();
    renderPanel();

    expect(
      await screen.findByRole("button", { name: /Referenced by \(2\)/ }),
    ).toBeInTheDocument();
    expect(await screen.findByText("Note A")).toBeInTheDocument();
    expect(screen.getByText("Note B")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /Note A/ }));
    await waitFor(() =>
      expect(screen.getByTestId("location")).toHaveTextContent(
        "/app/page/notes/a.md",
      ),
    );
  });

  it("shows the (0) header and the quiet empty line when there are no backlinks", async () => {
    vi.mocked(client.getBacklinks).mockResolvedValue([]);
    renderPanel();

    expect(
      await screen.findByRole("button", { name: /Referenced by \(0\)/ }),
    ).toBeInTheDocument();
    expect(await screen.findByText("No backlinks yet")).toBeInTheDocument();
  });

  it("shows the quiet loading line while the query is in flight", async () => {
    // A never-resolving promise keeps the query in its loading state.
    vi.mocked(client.getBacklinks).mockReturnValue(new Promise(() => {}));
    renderPanel();

    expect(await screen.findByText("Loading backlinks…")).toBeInTheDocument();
  });

  it("shows the quiet error line when the fetch rejects", async () => {
    vi.mocked(client.getBacklinks).mockRejectedValue(
      new Error("Couldn't load backlinks."),
    );
    renderPanel();

    expect(
      await screen.findByText("Couldn't load backlinks. Refresh to try again."),
    ).toBeInTheDocument();
  });

  it("collapses the body region when the toggle is clicked", async () => {
    vi.mocked(client.getBacklinks).mockResolvedValue([
      { path: "notes/a.md", title: "Note A" },
    ]);
    const user = userEvent.setup();
    renderPanel();

    const toggle = await screen.findByRole("button", {
      name: /Referenced by \(1\)/,
    });
    expect(toggle).toHaveAttribute("aria-expanded", "true");
    expect(await screen.findByText("Note A")).toBeInTheDocument();

    await user.click(toggle);
    expect(toggle).toHaveAttribute("aria-expanded", "false");
    expect(screen.queryByText("Note A")).not.toBeInTheDocument();
  });
});
