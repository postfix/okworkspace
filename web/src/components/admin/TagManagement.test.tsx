/**
 * TagManagement — the workspace-wide tag management surface under test
 * (TM-04 / v1.3). These assertions lock the contract:
 *
 *   1. loading / empty / error states render the exact quiet copy.
 *   2. the tag table renders one row per tag (name + page count + 3 actions).
 *   3. the search filter narrows the table by case-insensitive substring.
 *   4. a filter with no matches shows the "No tags match" empty row.
 *   5. rename: preview then apply — calls renameTag twice (preview, then real).
 *   6. merge: preview then apply — calls mergeTag twice.
 *   7. delete: auto-previews on open, then apply calls deleteTag.
 *   8. a successful apply invalidates the tag list query (refresh).
 */
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

vi.mock("../../api/client", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../../api/client")>();
  return {
    ...actual,
    listTags: vi.fn(),
    renameTag: vi.fn(),
    mergeTag: vi.fn(),
    deleteTag: vi.fn(),
  };
});

import * as client from "../../api/client";
import TagManagement from "./TagManagement";

const TAGS = [
  { tag: "ops", count: 5 },
  { tag: "devops", count: 3 },
  { tag: "release-notes", count: 1 },
];

function renderTagManagement() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={qc}>
      <TagManagement />
    </QueryClientProvider>,
  );
  return qc;
}

describe("TagManagement (TM-04)", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders loading state, then the tag table", async () => {
    vi.mocked(client.listTags).mockResolvedValue(TAGS);
    renderTagManagement();

    // Initially loading (query is pending).
    expect(screen.getByText(/loading tags/i)).toBeTruthy();

    // Then the table appears.
    expect(await screen.findByRole("table")).toBeTruthy();
    expect(screen.getByText("ops")).toBeTruthy();
    expect(screen.getByText("devops")).toBeTruthy();
    expect(screen.getByText("release-notes")).toBeTruthy();
  });

  it("renders the empty-tags copy when no tags exist", async () => {
    vi.mocked(client.listTags).mockResolvedValue([]);
    renderTagManagement();

    expect(
      await screen.findByText(/no tags yet/i),
    ).toBeTruthy();
  });

  it("renders the error copy when listTags rejects", async () => {
    vi.mocked(client.listTags).mockRejectedValue(new Error("boom"));
    renderTagManagement();

    expect(
      await screen.findByText(/couldn't load tags/i),
    ).toBeTruthy();
  });

  it("renders one row per tag with name + count + 3 action buttons", async () => {
    vi.mocked(client.listTags).mockResolvedValue(TAGS);
    renderTagManagement();

    const table = await screen.findByRole("table");
    const rows = within(table).getAllByRole("row");
    // 1 header row + 3 tag rows.
    expect(rows).toHaveLength(4);

    // Each tag row has 3 action buttons (rename, merge, delete).
    const renameBtns = screen.getAllByRole("button", { name: /rename/i });
    const mergeBtns = screen.getAllByRole("button", { name: /merge/i });
    const deleteBtns = screen.getAllByRole("button", { name: /delete/i });
    expect(renameBtns).toHaveLength(3);
    expect(mergeBtns).toHaveLength(3);
    expect(deleteBtns).toHaveLength(3);

    // Page counts render.
    expect(screen.getByText("5")).toBeTruthy();
    expect(screen.getByText("3")).toBeTruthy();
    expect(screen.getByText("1")).toBeTruthy();
  });

  it("filter narrows the table by case-insensitive substring", async () => {
    vi.mocked(client.listTags).mockResolvedValue(TAGS);
    const user = userEvent.setup();
    renderTagManagement();

    await screen.findByRole("table");
    const search = screen.getByRole("textbox", { name: /filter tags/i });

    // "dev" matches only "devops".
    await user.type(search, "dev");
    const rows = within(screen.getByRole("table")).getAllByRole("row");
    expect(rows).toHaveLength(2); // header + 1 match
    expect(screen.getByText("devops")).toBeTruthy();
    expect(screen.queryByText("ops")).toBeNull();
    expect(screen.queryByText("release-notes")).toBeNull();

    // Clear filter restores all.
    await user.clear(search);
    expect(
      within(screen.getByRole("table")).getAllByRole("row"),
    ).toHaveLength(4);
  });

  it("filter with no matches shows the empty row", async () => {
    vi.mocked(client.listTags).mockResolvedValue(TAGS);
    const user = userEvent.setup();
    renderTagManagement();

    await screen.findByRole("table");
    await user.type(
      screen.getByRole("textbox", { name: /filter tags/i }),
      "zzz",
    );

    expect(
      await screen.findByText(/no tags match/i),
    ).toBeTruthy();
  });

  it("rename: preview then apply calls renameTag twice and refreshes", async () => {
    vi.mocked(client.listTags).mockResolvedValue(TAGS);
    vi.mocked(client.renameTag).mockResolvedValue({
      tag: "ops",
      affected_pages: [
        { page_path: "a.md", current_tags: ["ops"], new_tags: ["devops"] },
      ],
      total_affected: 1,
    });
    const user = userEvent.setup();
    renderTagManagement();

    await screen.findByRole("table");

    // Open rename for "ops".
    await user.click(screen.getAllByRole("button", { name: /rename/i })[0]);

    // Type new name and click Preview.
    const dialog = await screen.findByRole("dialog");
    const input = within(dialog).getByPlaceholderText(/new-tag-name/i);
    await user.type(input, "devops");
    await user.click(
      within(dialog).getByRole("button", { name: /preview/i }),
    );

    // Preview fetched.
    await waitFor(() =>
      expect(client.renameTag).toHaveBeenCalledWith({
        old_name: "ops",
        new_name: "devops",
        preview: true,
      }),
    );

    // Now the confirm button says "Apply rename".
    await user.click(
      within(dialog).getByRole("button", { name: /apply rename/i }),
    );

    await waitFor(() =>
      expect(client.renameTag).toHaveBeenCalledWith({
        old_name: "ops",
        new_name: "devops",
      }),
    );

    // listTags was called twice (initial load + refresh after apply).
    await waitFor(() => expect(client.listTags).toHaveBeenCalledTimes(2));
  });

  it("merge: preview then apply calls mergeTag twice and refreshes", async () => {
    vi.mocked(client.listTags).mockResolvedValue(TAGS);
    vi.mocked(client.mergeTag).mockResolvedValue({
      tag: "ops",
      affected_pages: [
        { page_path: "a.md", current_tags: ["ops"], new_tags: ["devops"] },
      ],
      total_affected: 1,
    });
    const user = userEvent.setup();
    renderTagManagement();

    await screen.findByRole("table");

    // Open merge for "ops".
    await user.click(screen.getAllByRole("button", { name: /merge/i })[0]);

    // Select target and click Preview.
    const dialog = await screen.findByRole("dialog");
    const select = within(dialog).getByRole("combobox");
    await user.selectOptions(select, "devops");
    await user.click(
      within(dialog).getByRole("button", { name: /preview/i }),
    );

    await waitFor(() =>
      expect(client.mergeTag).toHaveBeenCalledWith({
        source: "ops",
        target: "devops",
        preview: true,
      }),
    );

    // Apply.
    await user.click(
      within(dialog).getByRole("button", { name: /apply merge/i }),
    );

    await waitFor(() =>
      expect(client.mergeTag).toHaveBeenCalledWith({
        source: "ops",
        target: "devops",
      }),
    );

    await waitFor(() => expect(client.listTags).toHaveBeenCalledTimes(2));
  });

  it("delete: auto-previews on open, then apply calls deleteTag", async () => {
    vi.mocked(client.listTags).mockResolvedValue(TAGS);
    vi.mocked(client.deleteTag).mockResolvedValue({
      tag: "ops",
      affected_pages: [
        { page_path: "a.md", current_tags: ["ops"], new_tags: [] },
      ],
      total_affected: 1,
    });
    const user = userEvent.setup();
    renderTagManagement();

    await screen.findByRole("table");

    // Open delete for "ops".
    await user.click(screen.getAllByRole("button", { name: /delete/i })[0]);

    // Auto-preview fires on open.
    await waitFor(() =>
      expect(client.deleteTag).toHaveBeenCalledWith({
        tag: "ops",
        preview: true,
      }),
    );

    // Confirm deletion.
    const dialog = await screen.findByRole("dialog");
    await user.click(
      within(dialog).getByRole("button", { name: /^delete$/i }),
    );

    await waitFor(() =>
      expect(client.deleteTag).toHaveBeenCalledWith({ tag: "ops" }),
    );

    await waitFor(() => expect(client.listTags).toHaveBeenCalledTimes(2));
  });

  it("delete: preview with a body_affected page renders the body #tags marker (TR-05)", async () => {
    vi.mocked(client.listTags).mockResolvedValue(TAGS);
    vi.mocked(client.deleteTag).mockResolvedValue({
      tag: "ops",
      affected_pages: [
        { page_path: "a.md", current_tags: ["ops"], new_tags: [], body_affected: true },
      ],
      total_affected: 1,
    });
    const user = userEvent.setup();
    renderTagManagement();

    await screen.findByRole("table");

    // Open delete for "ops" — auto-preview fires and returns a body_affected page.
    await user.click(screen.getAllByRole("button", { name: /delete/i })[0]);

    // The delete dialog must surface the SAME body-note marker rename/merge show.
    const dialog = await screen.findByRole("dialog");
    expect(
      await within(dialog).findByText(/body #tags that will be rewritten/i),
    ).toBeTruthy();
  });
});