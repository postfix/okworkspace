/**
 * VER-02/03 — HistoryPanel renders version rows as "{Action} by {name} ·
 * {relative time}" with ZERO Git vocabulary and NO version token shown,
 * "Restore this version" opens the (non-destructive) RestoreConfirmDialog, and a
 * single-version page shows the empty-state copy.
 */
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router-dom";
import type { ReactNode } from "react";

import HistoryPanel from "./HistoryPanel";
import {
  getHistory,
  viewVersion,
  restoreVersion,
  type HistoryEntry,
} from "../api/client";

vi.mock("../api/client", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../api/client")>();
  return {
    ...actual,
    getHistory: vi.fn(),
    viewVersion: vi.fn(),
    restoreVersion: vi.fn(),
  };
});

const mockGetHistory = vi.mocked(getHistory);
const mockViewVersion = vi.mocked(viewVersion);
const mockRestore = vi.mocked(restoreVersion);

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

const TWO_HOURS_AGO = new Date(Date.now() - 2 * 3600 * 1000).toISOString();
const YESTERDAY = new Date(Date.now() - 25 * 3600 * 1000).toISOString();

const TWO_VERSIONS: HistoryEntry[] = [
  { version: "tok-newest", action: "edit", who: "Sam", when: TWO_HOURS_AGO },
  { version: "tok-oldest", action: "create", who: "Sam", when: YESTERDAY },
];

beforeEach(() => {
  vi.clearAllMocks();
});

describe("HistoryPanel", () => {
  it("renders version rows as '{Action} by {name} · {relative time}' with no SHA", async () => {
    mockGetHistory.mockResolvedValue(TWO_VERSIONS);
    render(<HistoryPanel open path="notes/page.md" onClose={() => {}} />, {
      wrapper,
    });

    expect(await screen.findByText(/Edited by Sam · 2 hours ago/)).toBeInTheDocument();
    expect(screen.getByText(/Created by Sam · yesterday/)).toBeInTheDocument();

    // The opaque version token must NOT be rendered anywhere as text.
    expect(screen.queryByText(/tok-newest/)).not.toBeInTheDocument();
    expect(screen.queryByText(/tok-oldest/)).not.toBeInTheDocument();
  });

  it("never surfaces Git vocabulary", async () => {
    mockGetHistory.mockResolvedValue(TWO_VERSIONS);
    const { container } = render(
      <HistoryPanel open path="notes/page.md" onClose={() => {}} />,
      { wrapper },
    );
    await screen.findByText(/Edited by Sam/);
    expect(container.textContent ?? "").not.toMatch(
      /\b(commit|branch|SHA|HEAD|hash|push|merge|revert|reset)\b/i,
    );
  });

  it("opens the RestoreConfirmDialog from 'Restore this version'", async () => {
    mockGetHistory.mockResolvedValue(TWO_VERSIONS);
    render(<HistoryPanel open path="notes/page.md" onClose={() => {}} />, {
      wrapper,
    });

    const restoreButtons = await screen.findAllByRole("button", {
      name: /restore this version/i,
    });
    const user = userEvent.setup();
    await user.click(restoreButtons[1]); // restore the oldest version

    // The confirm dialog appears with its non-destructive copy.
    expect(
      await screen.findByText(/Restore this version\?/),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/Your current version is kept in history/),
    ).toBeInTheDocument();
  });

  it("calls restoreVersion with the opaque token on confirm", async () => {
    mockGetHistory.mockResolvedValue(TWO_VERSIONS);
    mockRestore.mockResolvedValue({ path: "notes/page.md" });
    render(<HistoryPanel open path="notes/page.md" onClose={() => {}} />, {
      wrapper,
    });

    const user = userEvent.setup();
    const restoreButtons = await screen.findAllByRole("button", {
      name: /restore this version/i,
    });
    await user.click(restoreButtons[1]);
    // Confirm inside the RestoreConfirmDialog (scoped by its aria-label so we do
    // not match the row buttons, which share the "Restore this version" label).
    const confirmDialog = await screen.findByRole("dialog", {
      name: /restore this version\?/i,
    });
    const confirm = within(confirmDialog).getByRole("button", {
      name: /^restore this version$/i,
    });
    await user.click(confirm);
    await waitFor(() =>
      expect(mockRestore).toHaveBeenCalledWith("notes/page.md", "tok-oldest"),
    );
  });

  it("previews an old version via 'View this version'", async () => {
    mockGetHistory.mockResolvedValue(TWO_VERSIONS);
    mockViewVersion.mockResolvedValue({
      frontmatter: "type: Page\n",
      body: "# the old body content\n",
      revision: "r1",
    });
    render(<HistoryPanel open path="notes/page.md" onClose={() => {}} />, {
      wrapper,
    });

    const user = userEvent.setup();
    const viewButtons = await screen.findAllByRole("button", {
      name: /view this version/i,
    });
    await user.click(viewButtons[1]);
    expect(
      await screen.findByText(/the old body content/),
    ).toBeInTheDocument();
    await waitFor(() =>
      expect(mockViewVersion).toHaveBeenCalledWith("notes/page.md", "tok-oldest"),
    );
  });

  it("shows the single-version empty state when only one version exists", async () => {
    mockGetHistory.mockResolvedValue([
      { version: "only", action: "create", who: "Sam", when: TWO_HOURS_AGO },
    ]);
    render(<HistoryPanel open path="notes/page.md" onClose={() => {}} />, {
      wrapper,
    });
    expect(
      await screen.findByText(/only its first version so far/),
    ).toBeInTheDocument();
  });
});
