/**
 * NAV-01/02/04 — LeftTree renders the live tree, toggles folders, and highlights
 * the active page row.
 */
import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { MemoryRouter, Routes, Route } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

vi.mock("../api/client", () => ({
  getTree: vi.fn(),
}));

import * as client from "../api/client";
import type { TreeNode } from "../api/client";
import LeftTree from "./LeftTree";

const TREE: TreeNode[] = [
  { type: "page", path: "home.md", title: "Home" },
  {
    type: "folder",
    path: "runbooks",
    title: "runbooks",
    children: [{ type: "page", path: "runbooks/deploy.md", title: "Deploy Staging" }],
  },
];

function renderTree(route = "/app") {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={[route]}>
        <Routes>
          <Route path="/app" element={<LeftTree />} />
          <Route path="/app/page/*" element={<LeftTree />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("LeftTree", () => {
  it("renders folders and pages from getTree", async () => {
    vi.mocked(client.getTree).mockResolvedValue(TREE);
    renderTree();
    expect(await screen.findByText("Home")).toBeInTheDocument();
    expect(await screen.findByText("runbooks")).toBeInTheDocument();
    // Folder is expanded by default, so the child page shows.
    expect(await screen.findByText("Deploy Staging")).toBeInTheDocument();
  });

  it("toggles a folder's children via the caret (NAV-02)", async () => {
    vi.mocked(client.getTree).mockResolvedValue(TREE);
    renderTree();
    const caret = await screen.findByRole("button", { name: /Collapse runbooks/i });
    expect(caret).toHaveAttribute("aria-expanded", "true");
    fireEvent.click(caret);
    // After collapse the child disappears and the caret flips its label.
    expect(screen.queryByText("Deploy Staging")).not.toBeInTheDocument();
    expect(
      await screen.findByRole("button", { name: /Expand runbooks/i }),
    ).toHaveAttribute("aria-expanded", "false");
  });

  it("renders an empty tree without crashing when getTree resolves null (UAT blocker)", async () => {
    // The tree endpoint can serialize an empty repo to JSON `null`; the
    // component must coalesce to [] and not throw on nodes.map.
    vi.mocked(client.getTree).mockResolvedValue(null as unknown as TreeNode[]);
    renderTree();
    const list = await screen.findByRole("list", { name: "Pages" });
    expect(list).toBeEmptyDOMElement();
  });

  it("highlights the active page row (NAV-04)", async () => {
    vi.mocked(client.getTree).mockResolvedValue(TREE);
    renderTree("/app/page/home.md");
    const homeRow = await screen.findByRole("button", { name: /Home/i });
    expect(homeRow).toHaveClass("navrow-active");
    expect(homeRow).toHaveAttribute("aria-current", "page");
  });
});
