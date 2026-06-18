/**
 * PAGE-03 — PageView renders committed Markdown safely and records a recent.
 */
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import { MemoryRouter, Routes, Route } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

vi.mock("../api/client", () => ({
  getPage: vi.fn(),
  me: vi.fn(),
}));

import * as client from "../api/client";
import { useRecent } from "../stores/recent";
import PageView from "./PageView";

function renderView(path: string) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={[`/app/page/${path}`]}>
        <Routes>
          <Route path="/app/page/*" element={<PageView />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("PageView", () => {
  beforeEach(() => {
    localStorage.clear();
    useRecent.getState().clear();
    vi.mocked(client.me).mockResolvedValue({
      username: "ed",
      display_name: "Ed",
      role: "editor",
      must_change_password: false,
    });
  });

  it("renders Markdown body and the title from frontmatter", async () => {
    vi.mocked(client.getPage).mockResolvedValue({
      frontmatter: "type: Page\ntitle: Deploy Staging\n",
      body: "# Heading\n\nSome **bold** text.",
      revision: "abc123",
    });
    renderView("runbooks/deploy.md");
    expect(await screen.findByRole("heading", { name: "Heading" })).toBeInTheDocument();
    expect(screen.getByText("bold")).toBeInTheDocument();
    // The page title (from frontmatter) appears.
    expect(screen.getByText("Deploy Staging")).toBeInTheDocument();
  });

  it("does not render raw HTML (XSS-safe, rehype-raw off)", async () => {
    vi.mocked(client.getPage).mockResolvedValue({
      frontmatter: "title: X\n",
      body: 'Hello <img src=x onerror="alert(1)"> world',
      revision: "r1",
    });
    renderView("x.md");
    await screen.findByText(/Hello/);
    // The injected onerror image must not be present in the DOM.
    expect(document.querySelector('img[onerror]')).toBeNull();
  });

  it("records the opened page in the recent store (NAV-05)", async () => {
    vi.mocked(client.getPage).mockResolvedValue({
      frontmatter: "title: Notes\n",
      body: "hi",
      revision: "r1",
    });
    renderView("notes.md");
    await screen.findByText("hi");
    const recents = useRecent.getState().recents;
    expect(recents[0]).toEqual({ path: "notes.md", title: "Notes" });
  });
});
