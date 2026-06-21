/**
 * NAV-01/02/04 — LeftTree renders the live tree, toggles folders, and highlights
 * the active page row.
 */
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Routes, Route } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

vi.mock("../api/client", () => ({
  getTree: vi.fn(),
  me: vi.fn(),
  movePage: vi.fn(),
  createPage: vi.fn(),
  createFolder: vi.fn(),
  renamePage: vi.fn(),
  deletePage: vi.fn(),
  getHistory: vi.fn(),
  viewVersion: vi.fn(),
  // 07-01/07-02 folder client functions. The rebuilt LeftTree/RenameModal/
  // MoveDialog import these (folder branch wired in Plan 04); an incomplete
  // factory would throw "X is not a function" at import time, so list them now.
  renameFolder: vi.fn(),
  moveFolder: vi.fn(),
  deleteFolder: vi.fn(),
  restoreFolderGroup: vi.fn(),
}));

import * as client from "../api/client";
import type { Me, TreeNode } from "../api/client";
import LeftTree from "./LeftTree";

const EDITOR: Me = {
  username: "ed",
  display_name: "Ed",
  role: "editor",
  must_change_password: false,
};

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
  beforeEach(() => {
    vi.clearAllMocks();
    // Default to an editor so the context menu / DnD affordances are present.
    vi.mocked(client.me).mockResolvedValue(EDITOR);
  });

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

  it("right-clicking a folder opens a create-here menu (context menu)", async () => {
    vi.mocked(client.getTree).mockResolvedValue(TREE);
    renderTree();
    const folderRow = (await screen.findByText("runbooks")).closest(
      ".navrow-folder",
    ) as HTMLElement;
    fireEvent.contextMenu(folderRow);
    expect(
      await screen.findByRole("menuitem", { name: "New page here" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("menuitem", { name: "New folder here" }),
    ).toBeInTheDocument();
    // Folders have no rename/move/delete (no backend) — create-only.
    expect(screen.queryByRole("menuitem", { name: /rename/i })).toBeNull();
  });

  it("folder 'New page here' opens CreatePageModal scoped to that folder", async () => {
    vi.mocked(client.getTree).mockResolvedValue(TREE);
    const user = userEvent.setup();
    renderTree();
    const folderRow = (await screen.findByText("runbooks")).closest(
      ".navrow-folder",
    ) as HTMLElement;
    fireEvent.contextMenu(folderRow);
    await user.click(screen.getByRole("menuitem", { name: "New page here" }));
    // The create-page dialog opens and tells the user it lands in the folder.
    expect(await screen.findByRole("dialog")).toBeInTheDocument();
    expect(screen.getByText(/We'll create it in runbooks\./)).toBeInTheDocument();
  });

  it("right-clicking a page opens rename/move/delete/history (reusing modals)", async () => {
    vi.mocked(client.getTree).mockResolvedValue(TREE);
    renderTree();
    const pageRow = await screen.findByRole("button", { name: /Home/i });
    fireEvent.contextMenu(pageRow);
    expect(
      await screen.findByRole("menuitem", { name: /rename/i }),
    ).toBeInTheDocument();
    expect(screen.getByRole("menuitem", { name: /move/i })).toBeInTheDocument();
    expect(
      screen.getByRole("menuitem", { name: /version history/i }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("menuitem", { name: /delete/i }),
    ).toHaveClass("treemenu-item-danger");
  });

  it("a reader's page menu shows history only (RBAC)", async () => {
    vi.mocked(client.getTree).mockResolvedValue(TREE);
    vi.mocked(client.me).mockResolvedValue({ ...EDITOR, role: "reader" });
    renderTree();
    const pageRow = await screen.findByRole("button", { name: /Home/i });
    fireEvent.contextMenu(pageRow);
    expect(
      await screen.findByRole("menuitem", { name: /version history/i }),
    ).toBeInTheDocument();
    expect(screen.queryByRole("menuitem", { name: /rename/i })).toBeNull();
    expect(screen.queryByRole("menuitem", { name: /delete/i })).toBeNull();
  });

  it("dropping a page onto a folder calls movePage(page, folder)", async () => {
    vi.mocked(client.getTree).mockResolvedValue(TREE);
    vi.mocked(client.movePage).mockResolvedValue({ path: "runbooks/home.md" });
    renderTree();
    const folderRow = (await screen.findByText("runbooks")).closest(
      ".navrow-folder",
    ) as HTMLElement;
    const dt = makeDataTransfer({ "application/x-okf-page": "home.md" });
    fireEvent.drop(folderRow, { dataTransfer: dt });
    // movePage runs through react-query's mutationFn (a microtask), so wait.
    await waitFor(() =>
      expect(client.movePage).toHaveBeenCalledWith("home.md", "runbooks"),
    );
  });

  it("does not move a page dropped onto its current parent (no-op guard)", async () => {
    vi.mocked(client.getTree).mockResolvedValue(TREE);
    renderTree();
    const folderRow = (await screen.findByText("runbooks")).closest(
      ".navrow-folder",
    ) as HTMLElement;
    // The deploy page already lives in runbooks; dropping it there is a no-op.
    const dt = makeDataTransfer({ "application/x-okf-page": "runbooks/deploy.md" });
    fireEvent.drop(folderRow, { dataTransfer: dt });
    expect(client.movePage).not.toHaveBeenCalled();
  });
});

// makeDataTransfer fakes the parts of DataTransfer the component reads: a typed
// getData and a `types` list (jsdom's DataTransfer doesn't carry custom types
// through fireEvent).
function makeDataTransfer(data: Record<string, string>) {
  return {
    getData: (type: string) => data[type] ?? "",
    setData: vi.fn(),
    types: Object.keys(data),
    effectAllowed: "move",
  } as unknown as DataTransfer;
}
