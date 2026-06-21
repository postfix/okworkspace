/**
 * PAGE-03 — PageView renders committed Markdown safely and records a recent.
 *
 * As of Phase 6 (06-04) the read surface is the UNIFIED read-only LivePreviewEditor
 * (a CodeMirror 6 live-preview view), not react-markdown/MarkdownProse. Headings are
 * therefore styled `.cm-line` elements carrying a github-slugger `id` (NOT semantic
 * `<h*>` `role="heading"` nodes). The load-bearing guarantees are unchanged: the
 * heading text renders, the line id equals okf.ScanHeadings's anchor (SRCH-06,
 * un-prefixed), raw HTML never executes, and the open is recorded as a recent.
 */
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Routes, Route } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

vi.mock("../api/client", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../api/client")>();
  return {
    ...actual,
    getPage: vi.fn(),
    me: vi.fn(),
    getTree: vi.fn().mockResolvedValue([]),
    renamePage: vi.fn(),
    movePage: vi.fn(),
  };
});

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
    // The page title (from frontmatter) appears in the header.
    expect(await screen.findByText("Deploy Staging")).toBeInTheDocument();
    // The body renders on the read-only live-preview surface: the heading line and
    // the bold text are present in the CM document.
    await waitFor(() =>
      expect(document.querySelector(".cm-line[id='heading']")?.textContent).toContain(
        "Heading",
      ),
    );
    expect(document.querySelector(".cm-strong")?.textContent).toBe("bold");
  });

  it("assigns GitHub-style heading ids matching the search anchor (SRCH-06)", async () => {
    vi.mocked(client.getPage).mockResolvedValue({
      frontmatter: "title: Guide\n",
      body: "## Rollback Procedure\n\nsteps",
      revision: "r1",
    });
    renderView("guide.md");
    // The rendered heading line carries an id equal to okf.ScanHeadings's anchor
    // (no user-content- prefix) — the SRCH-06 deep-link target.
    const heading = await waitFor(() => {
      const el = document.querySelector<HTMLElement>(".cm-line[id]");
      expect(el).not.toBeNull();
      return el!;
    });
    expect(heading.id).toBe("rollback-procedure");
    expect(heading.textContent).toContain("Rollback Procedure");
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
