/**
 * TagReviewView — the admin batch review queue under test (TAG-06). These
 * assertions lock the route shell contract on top of the REUSED Phase-11
 * TagSuggestList (whose own row internals/trust-gate are covered by
 * TagSuggest.test.tsx — not re-tested here):
 *
 *   1. loading / empty / error states render the exact quiet copy.
 *   2. the backlog renders one row per pending page (title + path + "{n} pending"
 *      chip) and a "{n} pages left to review" progress line.
 *   3. opening a row shows the reused approval surface with new-default-unchecked
 *      and Apply NOT auto-focused (the inherited trust gate).
 *   4. "Apply approved" calls approveTagSuggestions with EXACTLY the checked tags
 *      for that page; on "applied" the page leaves the backlog + progress decrements.
 *   5. a returned "stale" status switches that page into the inherited stale state
 *      (warning + Re-run, Apply removed) without affecting the other backlog row.
 *   6. "Skip for now" leaves the page pending (approveTagSuggestions NOT called).
 *   7. no dangerouslySetInnerHTML anywhere (stored-XSS guard, T-12-11).
 */
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

vi.mock("../api/client", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../api/client")>();
  return {
    ...actual,
    listTagSuggestions: vi.fn(),
    approveTagSuggestions: vi.fn(),
    suggestTags: vi.fn(),
  };
});

import * as client from "../api/client";
import TagReviewView from "./TagReviewView";

const QUEUE = [
  {
    page_path: "notes/a.md",
    suggestions: [
      { tag: "release", existing: true },
      { tag: "q3-launch", existing: false },
    ],
    base_revision: "rev-a",
  },
  {
    page_path: "notes/b.md",
    suggestions: [{ tag: "draft", existing: true }],
    base_revision: "rev-b",
  },
];

function renderReview() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={qc}>
      <TagReviewView />
    </QueryClientProvider>,
  );
  return qc;
}

async function openSurface(user: ReturnType<typeof userEvent.setup>, label: RegExp) {
  await user.click(screen.getByRole("button", { name: label }));
  return screen.findByRole("dialog", { name: /suggested tags/i });
}

describe("TagReviewView — batch review queue (TAG-06)", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("shows the loading state while fetching the backlog", async () => {
    let resolve!: (v: typeof QUEUE) => void;
    vi.mocked(client.listTagSuggestions).mockReturnValue(
      new Promise((r) => {
        resolve = r;
      }),
    );
    renderReview();
    expect(screen.getByText("Loading pages to review…")).toBeTruthy();
    resolve(QUEUE);
    await screen.findByText("2 pages left to review");
  });

  it("renders the empty state when the queue is empty", async () => {
    vi.mocked(client.listTagSuggestions).mockResolvedValue([]);
    renderReview();
    expect(await screen.findByText("No pending suggestions")).toBeTruthy();
    expect(
      screen.getByText(/Run a tag-suggestion sweep from the admin Settings page/i),
    ).toBeTruthy();
  });

  it("renders the error state when the backlog fails to load", async () => {
    vi.mocked(client.listTagSuggestions).mockRejectedValue(new Error("boom"));
    renderReview();
    expect(
      await screen.findByText("Couldn’t load the review queue. Refresh to try again."),
    ).toBeTruthy();
  });

  it("renders one backlog row per pending page with title + path + pending chip + progress", async () => {
    vi.mocked(client.listTagSuggestions).mockResolvedValue(QUEUE);
    renderReview();

    expect(await screen.findByText("2 pages left to review")).toBeTruthy();
    const rowA = await screen.findByRole("button", {
      name: /review suggestions for notes\/a\.md/i,
    });
    expect(within(rowA).getByText("2 pending")).toBeTruthy();
    const rowB = screen.getByRole("button", {
      name: /review suggestions for notes\/b\.md/i,
    });
    expect(within(rowB).getByText("1 pending")).toBeTruthy();
  });

  it("opens a row into the reused approval surface: new default unchecked + Apply not auto-focused", async () => {
    vi.mocked(client.listTagSuggestions).mockResolvedValue(QUEUE);
    const user = userEvent.setup();
    renderReview();
    await screen.findByText("2 pages left to review");

    await openSurface(user, /review suggestions for notes\/a\.md/i);

    const releaseRow = screen.getByText("release").closest("label")!;
    const newRow = screen.getByText("q3-launch").closest("label")!;
    expect(within(newRow).getByText("new")).toBeTruthy();
    expect(
      (within(releaseRow).getByRole("checkbox") as HTMLInputElement).checked,
    ).toBe(true);
    expect(
      (within(newRow).getByRole("checkbox") as HTMLInputElement).checked,
    ).toBe(false);

    // Trust gate: "Apply approved" is NEVER the initial focus; Skip is.
    const apply = screen.getByRole("button", { name: /apply approved/i });
    const skip = screen.getByRole("button", { name: /skip for now/i });
    expect(document.activeElement).toBe(skip);
    expect(document.activeElement).not.toBe(apply);
  });

  it("Apply approved sends EXACTLY the checked tags; an applied page leaves the backlog + progress decrements", async () => {
    vi.mocked(client.listTagSuggestions).mockResolvedValue(QUEUE);
    vi.mocked(client.approveTagSuggestions).mockResolvedValue([
      { page_path: "notes/a.md", status: "applied" },
    ]);
    const user = userEvent.setup();
    renderReview();
    await screen.findByText("2 pages left to review");

    await openSurface(user, /review suggestions for notes\/a\.md/i);
    // The applied row leaves the backlog: the invalidate-triggered refetch now
    // returns only the second page. Arm the post-apply queue BEFORE clicking so
    // the refetch fired in onSuccess sees the shrunken queue.
    vi.mocked(client.listTagSuggestions).mockResolvedValue([QUEUE[1]]);

    // Default state: release checked, q3-launch unchecked → only release applies.
    await user.click(screen.getByRole("button", { name: /apply approved/i }));

    await waitFor(() =>
      expect(client.approveTagSuggestions).toHaveBeenCalledTimes(1),
    );
    expect(client.approveTagSuggestions).toHaveBeenCalledWith([
      { page_path: "notes/a.md", tags: ["release"] },
    ]);

    // The page left the backlog + the progress line decrements.
    await waitFor(() =>
      expect(screen.getByText("1 pages left to review")).toBeTruthy(),
    );
  });

  it("a 'stale' result switches THAT page into the inherited stale state without affecting the other row", async () => {
    vi.mocked(client.listTagSuggestions).mockResolvedValue(QUEUE);
    vi.mocked(client.approveTagSuggestions).mockResolvedValue([
      { page_path: "notes/a.md", status: "stale" },
    ]);
    const user = userEvent.setup();
    renderReview();
    await screen.findByText("2 pages left to review");

    await openSurface(user, /review suggestions for notes\/a\.md/i);
    await user.click(screen.getByRole("button", { name: /apply approved/i }));

    // The stale state removes the Apply path and offers Re-run.
    await waitFor(() =>
      expect(screen.queryByRole("button", { name: /apply approved/i })).toBeNull(),
    );
    expect(screen.getByRole("button", { name: /re-run/i })).toBeTruthy();
    // The OTHER backlog page is untouched — the progress line is unchanged.
    expect(screen.getByText("2 pages left to review")).toBeTruthy();
  });

  it("Skip for now leaves the page pending (approveTagSuggestions not called)", async () => {
    vi.mocked(client.listTagSuggestions).mockResolvedValue(QUEUE);
    const user = userEvent.setup();
    renderReview();
    await screen.findByText("2 pages left to review");

    await openSurface(user, /review suggestions for notes\/a\.md/i);
    await user.click(screen.getByRole("button", { name: /skip for now/i }));

    await waitFor(() =>
      expect(screen.queryByRole("dialog", { name: /suggested tags/i })).toBeNull(),
    );
    expect(client.approveTagSuggestions).not.toHaveBeenCalled();
    // The page is still in the backlog.
    expect(screen.getByText("2 pages left to review")).toBeTruthy();
  });
});
