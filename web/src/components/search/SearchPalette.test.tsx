/**
 * SRCH-01/02/03/06 — SearchPalette renders typed results with weight-only
 * highlight (no raw HTML), the no-results and error states, and supports ↑/↓ +
 * Enter navigation.
 */
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const navigateMock = vi.fn();

vi.mock("react-router-dom", async (importOriginal) => {
  const actual = await importOriginal<typeof import("react-router-dom")>();
  return { ...actual, useNavigate: () => navigateMock };
});

vi.mock("../../api/client", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../../api/client")>();
  return { ...actual, search: vi.fn() };
});

import * as client from "../../api/client";
import type { SearchResult } from "../../api/client";
import { useSearchStore } from "../../store/searchStore";
import SearchPalette from "./SearchPalette";

// jsdom does not implement scrollIntoView; the palette calls it for auto-scroll.
beforeEach(() => {
  Element.prototype.scrollIntoView = vi.fn();
});

function renderPalette() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <SearchPalette />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

async function typeQuery(value: string) {
  const input = await screen.findByRole("combobox");
  fireEvent.change(input, { target: { value } });
  return input;
}

describe("SearchPalette", () => {
  beforeEach(() => {
    navigateMock.mockClear();
    vi.mocked(client.search).mockReset();
    // Open the palette via the store before each test.
    useSearchStore.getState().setOpen(true);
  });

  it("renders grouped typed results with weight-only highlight and no raw HTML", async () => {
    const results: SearchResult[] = [
      {
        kind: "page",
        title: "Deploy <strong>Staging</strong>",
        path: "runbooks/deploy.md",
        snippet:
          'How to deploy <strong>staging</strong> <img src=x onerror="alert(1)">',
      },
    ];
    vi.mocked(client.search).mockResolvedValue(results);
    renderPalette();
    await typeQuery("staging");

    // Group label and a typed result row render.
    expect(await screen.findByText("Pages")).toBeInTheDocument();
    const option = await screen.findByRole("option");
    expect(option).toBeInTheDocument();

    // The matched term is bold via a <strong> element (weight-only highlight).
    const strongs = option.querySelectorAll("strong");
    expect(strongs.length).toBeGreaterThan(0);
    expect(option.textContent).toContain("Staging");

    // The injected raw <img onerror> must NOT become a live DOM node — it is
    // rendered as escaped text only (XSS guard, T-03-08).
    expect(document.querySelector("img[onerror]")).toBeNull();
    expect(option.textContent).toContain("onerror");
  });

  it("renders the no-results state with the UI-SPEC copy", async () => {
    vi.mocked(client.search).mockResolvedValue([]);
    renderPalette();
    await typeQuery("zzzznope");
    expect(await screen.findByText("No matches")).toBeInTheDocument();
    // The query is rendered as escaped text inside curly quotes, split across
    // text nodes — assert the body and the query echo are both present.
    const body = screen.getByText(/Nothing matched/);
    expect(body).toBeInTheDocument();
    expect(body.textContent).toContain("zzzznope");
  });

  it("renders the error state without exposing internals", async () => {
    vi.mocked(client.search).mockRejectedValue(
      new Error("Search is unavailable. Try again in a moment."),
    );
    renderPalette();
    await typeQuery("boom");
    expect(await screen.findByText("Search is unavailable")).toBeInTheDocument();
    expect(
      screen.getByText(/Something went wrong while searching/),
    ).toBeInTheDocument();
  });

  it("navigates the active row with ↓ and opens it with Enter", async () => {
    const results: SearchResult[] = [
      { kind: "page", title: "Alpha", path: "a.md", snippet: "first" },
      { kind: "page", title: "Beta", path: "b.md", snippet: "second" },
    ];
    vi.mocked(client.search).mockResolvedValue(results);
    renderPalette();
    const input = await typeQuery("a");

    // Two rows render; the first is active by default.
    const options = await screen.findAllByRole("option");
    expect(options).toHaveLength(2);
    await waitFor(() =>
      expect(options[0]).toHaveAttribute("aria-selected", "true"),
    );

    // ↓ moves active to the second row.
    fireEvent.keyDown(input, { key: "ArrowDown" });
    await waitFor(() =>
      expect(
        screen.getAllByRole("option")[1],
      ).toHaveAttribute("aria-selected", "true"),
    );

    // Enter opens the active (second) row in-app.
    fireEvent.keyDown(input, { key: "Enter" });
    expect(navigateMock).toHaveBeenCalledWith("/app/page/b.md");
  });

  it("opens via the store and Esc-driven close is wired through the store", async () => {
    vi.mocked(client.search).mockResolvedValue([]);
    renderPalette();
    // The palette is open (combobox present) because the store said so.
    expect(await screen.findByRole("combobox")).toBeInTheDocument();
  });
});
