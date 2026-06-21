/**
 * PAGE-06/07 — TrashView lists deleted pages (title + deleted-by + relative
 * time, NO Git vocabulary), each row has a "Restore page" action, and the empty
 * state shows the UI-SPEC empty-trash copy. Also covers the relativeTime helper.
 */
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router-dom";
import type { ReactNode } from "react";

import TrashView, { relativeTime } from "./TrashView";
import {
  listTrash,
  restoreFromTrash,
  restoreFolderGroup,
  type TrashEntry,
} from "../api/client";

vi.mock("../api/client", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../api/client")>();
  return {
    ...actual,
    listTrash: vi.fn(),
    restoreFromTrash: vi.fn(),
    restoreFolderGroup: vi.fn(),
  };
});

const mockListTrash = vi.mocked(listTrash);
const mockRestore = vi.mocked(restoreFromTrash);
const mockRestoreGroup = vi.mocked(restoreFolderGroup);

function wrapper({ children }: { children: ReactNode }) {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return (
    <QueryClientProvider client={qc}>
      <MemoryRouter>{children}</MemoryRouter>
    </QueryClientProvider>
  );
}

const SAMPLE: TrashEntry[] = [
  {
    id: 1,
    title: "Deploy",
    original_path: "runbooks/deploy.md",
    deleted_by: "Sam",
    deleted_at: new Date(Date.now() - 2 * 3600 * 1000).toISOString(),
    // A solo per-page delete carries no group id (07-02 added this required field).
    delete_group_id: "",
  },
];

beforeEach(() => {
  vi.clearAllMocks();
});

describe("TrashView", () => {
  it("lists deleted pages with title, deleted-by, and relative time", async () => {
    mockListTrash.mockResolvedValue(SAMPLE);
    render(<TrashView />, { wrapper });

    expect(await screen.findByText("Deploy")).toBeInTheDocument();
    expect(screen.getByText(/Deleted by Sam/)).toBeInTheDocument();
    expect(screen.getByText(/hours ago/)).toBeInTheDocument();
  });

  it("shows a 'Restore page' action per row and restores on click", async () => {
    mockListTrash.mockResolvedValue(SAMPLE);
    mockRestore.mockResolvedValue({ path: "runbooks/deploy.md" });
    render(<TrashView />, { wrapper });

    const restoreBtn = await screen.findByRole("button", {
      name: /restore page/i,
    });
    const user = userEvent.setup();
    await user.click(restoreBtn);
    await waitFor(() => expect(mockRestore).toHaveBeenCalledWith(1));
  });

  it("shows the UI-SPEC empty-trash copy when there are no entries", async () => {
    mockListTrash.mockResolvedValue([]);
    render(<TrashView />, { wrapper });

    expect(await screen.findByText("Trash is empty")).toBeInTheDocument();
    expect(
      screen.getByText(/Pages you delete will appear here/),
    ).toBeInTheDocument();
  });

  it("surfaces the collision-suffix notice when the restored path differs", async () => {
    mockListTrash.mockResolvedValue(SAMPLE);
    // Backend auto-suffixed: restored path differs from the original.
    mockRestore.mockResolvedValue({ path: "runbooks/deploy-2.md" });
    render(<TrashView />, { wrapper });

    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("button", { name: /restore page/i }),
    );
    expect(
      await screen.findByText(/restored as 'Deploy \(restored\)'/),
    ).toBeInTheDocument();
  });

  it("never surfaces Git vocabulary", async () => {
    mockListTrash.mockResolvedValue(SAMPLE);
    const { container } = render(<TrashView />, { wrapper });
    await screen.findByText("Deploy");
    expect(container.textContent ?? "").not.toMatch(
      /\b(commit|branch|SHA|HEAD|push|merge)\b/i,
    );
  });

  // TREE-05 — grouped folder restore.
  const GROUPED: TrashEntry[] = [
    // Two pages trashed together by one folder-delete (shared group id).
    {
      id: 10,
      title: "runbooks",
      original_path: "runbooks/index.md",
      deleted_by: "Sam",
      deleted_at: new Date(Date.now() - 3600 * 1000).toISOString(),
      delete_group_id: "grp-1",
    },
    {
      id: 11,
      title: "Deploy",
      original_path: "runbooks/deploy.md",
      deleted_by: "Sam",
      deleted_at: new Date(Date.now() - 3600 * 1000).toISOString(),
      delete_group_id: "grp-1",
    },
    // One individually-trashed page (no group).
    {
      id: 12,
      title: "Notes",
      original_path: "notes.md",
      deleted_by: "Ada",
      deleted_at: new Date(Date.now() - 7200 * 1000).toISOString(),
      delete_group_id: "",
    },
  ];

  it("renders ONE grouped 'Restore folder' row + a per-page row and restores the group", async () => {
    mockListTrash.mockResolvedValue(GROUPED);
    mockRestoreGroup.mockResolvedValue({
      paths: ["runbooks/index.md", "runbooks/deploy.md"],
    });
    render(<TrashView />, { wrapper });

    // The grouped row labels the folder + page count and offers Restore folder.
    expect(
      await screen.findByText(/Folder 'runbooks' · 2 pages/),
    ).toBeInTheDocument();
    const groupBtn = screen.getByRole("button", { name: /restore folder/i });
    // The solo page keeps its per-page row.
    expect(screen.getByText("Notes")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /restore page/i }),
    ).toBeInTheDocument();
    // Exactly one grouped row (not one per grouped entry).
    expect(
      screen.getAllByRole("button", { name: /restore folder/i }),
    ).toHaveLength(1);

    const user = userEvent.setup();
    await user.click(groupBtn);
    await waitFor(() =>
      expect(mockRestoreGroup).toHaveBeenCalledWith("grp-1"),
    );
  });

  it("surfaces the batched collision notice when a grouped page was auto-suffixed", async () => {
    mockListTrash.mockResolvedValue(GROUPED);
    // One restored path differs from the original → a collision was suffixed.
    mockRestoreGroup.mockResolvedValue({
      paths: ["runbooks/index.md", "runbooks/deploy-2.md"],
    });
    render(<TrashView />, { wrapper });

    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("button", { name: /restore folder/i }),
    );
    expect(
      await screen.findByText(
        /Some pages already existed, so they were restored with a '\(restored\)' suffix\./,
      ),
    ).toBeInTheDocument();
  });
});

describe("relativeTime", () => {
  it("renders recent times as 'just now'", () => {
    expect(relativeTime(new Date().toISOString())).toBe("just now");
  });
  it("renders hours ago", () => {
    const t = new Date(Date.now() - 3 * 3600 * 1000).toISOString();
    expect(relativeTime(t)).toBe("3 hours ago");
  });
  it("renders 'yesterday'", () => {
    const t = new Date(Date.now() - 25 * 3600 * 1000).toISOString();
    expect(relativeTime(t)).toBe("yesterday");
  });
});
