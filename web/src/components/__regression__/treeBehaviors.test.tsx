/**
 * Clean-Rebuild Behavior Inventory — regression net (07-RESEARCH §).
 *
 * This file is the load-bearing safety contract for Phase 7's deliberate CLEAN
 * REBUILD of the tree UX (CONTEXT override). It pins — as black-box tests — every
 * currently-shipped behavior of LeftTree / TreeContextMenu / RenameModal /
 * MoveDialog so the rebuild can proceed with proof that nothing user-visible
 * regresses. It is GREEN against the CURRENT (un-rebuilt) components BEFORE any
 * rebuild edit, and stays GREEN after the rebuild. NO new folder feature is
 * asserted here — those land in Plan 04.
 */
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Routes, Route } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

// The mock factory MUST list the 07-01/07-02 folder client functions — the
// rebuilt components import them, and an incomplete factory throws on import
// (RESEARCH Runtime State). This is the same contract the LeftTree.test.tsx
// factory carries.
vi.mock("../../api/client", () => ({
  getTree: vi.fn(),
  me: vi.fn(),
  movePage: vi.fn(),
  createPage: vi.fn(),
  createFolder: vi.fn(),
  renamePage: vi.fn(),
  deletePage: vi.fn(),
  getHistory: vi.fn(),
  viewVersion: vi.fn(),
  renameFolder: vi.fn(),
  moveFolder: vi.fn(),
  deleteFolder: vi.fn(),
  restoreFolderGroup: vi.fn(),
}));

import * as client from "../../api/client";
import type { Me, TreeNode } from "../../api/client";
import LeftTree from "../LeftTree";
import TreeContextMenu from "../TreeContextMenu";
import RenameModal from "../RenameModal";
import MoveDialog from "../MoveDialog";

const EDITOR: Me = {
  username: "ed",
  display_name: "Ed",
  role: "editor",
  must_change_password: false,
};
const READER: Me = { ...EDITOR, role: "reader" };

const TREE: TreeNode[] = [
  { type: "page", path: "home.md", title: "Home" },
  {
    type: "folder",
    path: "runbooks",
    title: "runbooks",
    children: [
      { type: "page", path: "runbooks/deploy.md", title: "Deploy Staging" },
    ],
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

// makeDataTransfer fakes the parts of DataTransfer the component reads (jsdom's
// DataTransfer doesn't carry custom types through fireEvent).
function makeDataTransfer(data: Record<string, string>) {
  return {
    getData: (type: string) => data[type] ?? "",
    setData: vi.fn(),
    types: Object.keys(data),
    effectAllowed: "move",
  } as unknown as DataTransfer;
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(client.me).mockResolvedValue(EDITOR);
  vi.mocked(client.getTree).mockResolvedValue(TREE);
});

describe("regression: LeftTree behavior inventory", () => {
  // Inventory #2
  it("coalesces a null tree to [] (no white-screen)", async () => {
    vi.mocked(client.getTree).mockResolvedValue(null as unknown as TreeNode[]);
    renderTree();
    const list = await screen.findByRole("list", { name: "Pages" });
    expect(list).toBeEmptyDOMElement();
  });

  // Inventory #1
  it("renders folders and pages from getTree", async () => {
    renderTree();
    expect(await screen.findByText("Home")).toBeInTheDocument();
    expect(await screen.findByText("runbooks")).toBeInTheDocument();
    expect(await screen.findByText("Deploy Staging")).toBeInTheDocument();
  });

  // Inventory #3
  it("expands/collapses a folder with aria-expanded", async () => {
    renderTree();
    const caret = await screen.findByRole("button", {
      name: /Collapse runbooks/i,
    });
    expect(caret).toHaveAttribute("aria-expanded", "true");
    fireEvent.click(caret);
    expect(screen.queryByText("Deploy Staging")).not.toBeInTheDocument();
    const reCaret = await screen.findByRole("button", {
      name: /Expand runbooks/i,
    });
    expect(reCaret).toHaveAttribute("aria-expanded", "false");
  });

  // Inventory #4
  it("marks the active page row navrow-active + aria-current=page", async () => {
    renderTree("/app/page/home.md");
    const homeRow = await screen.findByRole("button", { name: /Home/i });
    expect(homeRow).toHaveClass("navrow-active");
    expect(homeRow).toHaveAttribute("aria-current", "page");
  });

  // Inventory #7 (editor)
  it("right-click a page (editor) shows Rename/Move/Version history/Delete", async () => {
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
    expect(screen.getByRole("menuitem", { name: /delete/i })).toHaveClass(
      "treemenu-item-danger",
    );
  });

  // Inventory #7 (reader)
  it("right-click a page (reader) shows ONLY Version history", async () => {
    vi.mocked(client.me).mockResolvedValue(READER);
    renderTree();
    const pageRow = await screen.findByRole("button", { name: /Home/i });
    fireEvent.contextMenu(pageRow);
    expect(
      await screen.findByRole("menuitem", { name: /version history/i }),
    ).toBeInTheDocument();
    expect(screen.queryByRole("menuitem", { name: /rename/i })).toBeNull();
    expect(screen.queryByRole("menuitem", { name: /move/i })).toBeNull();
    expect(screen.queryByRole("menuitem", { name: /delete/i })).toBeNull();
  });

  // Inventory #6
  it("right-click a folder (editor) shows the create-here actions", async () => {
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
    // No folder rename/move/delete in this base (those land in Plan 04).
    expect(screen.queryByRole("menuitem", { name: /^rename$/i })).toBeNull();
  });

  // Inventory #8
  it("right-click blank nav space shows root create", async () => {
    renderTree();
    const list = await screen.findByRole("list", { name: "Pages" });
    fireEvent.contextMenu(list);
    expect(
      await screen.findByRole("menuitem", { name: "New page" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("menuitem", { name: "New folder" }),
    ).toBeInTheDocument();
  });

  // Inventory #9
  it("page rows are draggable (editor) and set application/x-okf-page on dragStart", async () => {
    renderTree();
    const pageRow = await screen.findByRole("button", { name: /Home/i });
    expect(pageRow).toHaveAttribute("draggable", "true");
    const dt = makeDataTransfer({});
    fireEvent.dragStart(pageRow, { dataTransfer: dt });
    expect(dt.setData).toHaveBeenCalledWith(
      "application/x-okf-page",
      "home.md",
    );
  });

  // Inventory #14 (reader: no DnD)
  it("page rows are NOT draggable for a reader (RBAC)", async () => {
    vi.mocked(client.me).mockResolvedValue(READER);
    renderTree();
    const pageRow = await screen.findByRole("button", { name: /Home/i });
    expect(pageRow).toHaveAttribute("draggable", "false");
  });

  // Inventory #10
  it("dropping a page onto a folder row moves it (movePage called)", async () => {
    vi.mocked(client.movePage).mockResolvedValue({ path: "runbooks/home.md" });
    renderTree();
    const folderRow = (await screen.findByText("runbooks")).closest(
      ".navrow-folder",
    ) as HTMLElement;
    const dt = makeDataTransfer({ "application/x-okf-page": "home.md" });
    fireEvent.drop(folderRow, { dataTransfer: dt });
    await waitFor(() =>
      expect(client.movePage).toHaveBeenCalledWith("home.md", "runbooks"),
    );
  });

  // Inventory #11
  it("same-parent drop is a no-op (movePage NOT called)", async () => {
    renderTree();
    const folderRow = (await screen.findByText("runbooks")).closest(
      ".navrow-folder",
    ) as HTMLElement;
    const dt = makeDataTransfer({
      "application/x-okf-page": "runbooks/deploy.md",
    });
    fireEvent.drop(folderRow, { dataTransfer: dt });
    expect(client.movePage).not.toHaveBeenCalled();
  });

  // Inventory #13 (loading)
  it("renders a role=status loading state while the tree loads", async () => {
    // A never-resolving getTree keeps the query pending so the status shows.
    vi.mocked(client.getTree).mockReturnValue(new Promise<TreeNode[]>(() => {}));
    renderTree();
    const status = await screen.findByRole("status");
    expect(status).toHaveTextContent(/loading/i);
  });

  // Inventory #13 (error)
  it("renders a role=alert error state when the tree fails to load", async () => {
    vi.mocked(client.getTree).mockRejectedValue(new Error("boom"));
    renderTree();
    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent(/Couldn't load your pages/i);
  });
});

describe("regression: TreeContextMenu a11y", () => {
  it("renders role=menu with role=menuitem rows", () => {
    render(
      <TreeContextMenu
        x={10}
        y={10}
        onClose={vi.fn()}
        items={[
          { label: "Rename", onSelect: vi.fn() },
          { label: "Delete", onSelect: vi.fn(), danger: true },
        ]}
      />,
    );
    expect(screen.getByRole("menu")).toBeInTheDocument();
    expect(screen.getAllByRole("menuitem")).toHaveLength(2);
    expect(screen.getByRole("menuitem", { name: "Delete" })).toHaveClass(
      "treemenu-item-danger",
    );
  });

  it("focuses the first item on open and ArrowDown wraps focus", async () => {
    const user = userEvent.setup();
    render(
      <TreeContextMenu
        x={10}
        y={10}
        onClose={vi.fn()}
        items={[
          { label: "First", onSelect: vi.fn() },
          { label: "Second", onSelect: vi.fn() },
        ]}
      />,
    );
    expect(screen.getByRole("menuitem", { name: "First" })).toHaveFocus();
    await user.keyboard("{ArrowDown}");
    expect(screen.getByRole("menuitem", { name: "Second" })).toHaveFocus();
    await user.keyboard("{ArrowDown}");
    expect(screen.getByRole("menuitem", { name: "First" })).toHaveFocus();
  });

  it("Enter selects the focused item (and closes); Escape closes", async () => {
    const onSelect = vi.fn();
    const onClose = vi.fn();
    const user = userEvent.setup();
    const { rerender } = render(
      <TreeContextMenu
        x={10}
        y={10}
        onClose={onClose}
        items={[{ label: "Rename", onSelect }]}
      />,
    );
    await user.keyboard("{Enter}");
    expect(onSelect).toHaveBeenCalledOnce();
    expect(onClose).toHaveBeenCalledOnce();

    onClose.mockClear();
    rerender(
      <TreeContextMenu
        x={10}
        y={10}
        onClose={onClose}
        items={[{ label: "Rename", onSelect: vi.fn() }]}
      />,
    );
    await user.keyboard("{Escape}");
    expect(onClose).toHaveBeenCalledOnce();
  });

  it("closes on outside click", () => {
    const onClose = vi.fn();
    render(
      <div>
        <button type="button">outside</button>
        <TreeContextMenu
          x={10}
          y={10}
          onClose={onClose}
          items={[{ label: "Move", onSelect: vi.fn() }]}
        />
      </div>,
    );
    fireEvent.mouseDown(screen.getByRole("button", { name: "outside" }));
    expect(onClose).toHaveBeenCalledOnce();
  });
});

describe("regression: RenameModal (page path)", () => {
  function renderRename() {
    const qc = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    render(
      <QueryClientProvider client={qc}>
        <MemoryRouter>
          <RenameModal
            open
            path="home.md"
            currentTitle="Home"
            onClose={vi.fn()}
          />
        </MemoryRouter>
      </QueryClientProvider>,
    );
  }

  it("rejects an empty title with the page validation copy (no API call)", async () => {
    const user = userEvent.setup();
    renderRename();
    const input = screen.getByLabelText(/new title/i);
    await user.clear(input);
    await user.click(screen.getByRole("button", { name: "Rename" }));
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Give your page a title.",
    );
    expect(client.renamePage).not.toHaveBeenCalled();
  });

  it("calls renamePage with the trimmed title and shows the link-safety help", async () => {
    vi.mocked(client.renamePage).mockResolvedValue({ path: "renamed.md" });
    const user = userEvent.setup();
    renderRename();
    expect(
      screen.getByText("Links to this page will keep working."),
    ).toBeInTheDocument();
    const input = screen.getByLabelText(/new title/i);
    await user.clear(input);
    await user.type(input, "  Runbook  ");
    await user.click(screen.getByRole("button", { name: "Rename" }));
    await waitFor(() =>
      expect(client.renamePage).toHaveBeenCalledWith("home.md", "Runbook"),
    );
  });
});

describe("regression: MoveDialog (page path)", () => {
  function renderMove() {
    const qc = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    render(
      <QueryClientProvider client={qc}>
        <MemoryRouter>
          <MoveDialog open path="home.md" onClose={vi.fn()} />
        </MemoryRouter>
      </QueryClientProvider>,
    );
  }

  it("flattens the tree to a folder <select> with a Top level option", async () => {
    renderMove();
    expect(
      await screen.findByRole("option", { name: "Top level" }),
    ).toBeInTheDocument();
    await waitFor(() =>
      expect(
        screen.getByRole("option", { name: "runbooks" }),
      ).toBeInTheDocument(),
    );
    expect(
      screen.getByText("Choose where this page should live."),
    ).toBeInTheDocument();
  });

  it("calls movePage(path, destination) on confirm", async () => {
    vi.mocked(client.movePage).mockResolvedValue({ path: "runbooks/home.md" });
    const user = userEvent.setup();
    renderMove();
    // Wait for the tree query to populate the destination options before
    // selecting (the <select> exists immediately, but "runbooks" arrives async).
    const option = await screen.findByRole("option", { name: "runbooks" });
    const select = screen.getByLabelText(/folder/i);
    await user.selectOptions(select, option);
    await user.click(screen.getByRole("button", { name: "Move page" }));
    await waitFor(() =>
      expect(client.movePage).toHaveBeenCalledWith("home.md", "runbooks"),
    );
  });
});
