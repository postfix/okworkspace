/**
 * DocStatusBar — the Obsidian footer strip: "N backlinks · N words · N,NNN characters".
 * Contracts under test:
 *   1. word count = whitespace-split of the TRIMMED body (empty/blank body → 0 words).
 *   2. char count = raw body.length (NOT trimmed), rendered with locale grouping.
 *   3. backlink count comes from useBacklinks(path) — seeded via the getBacklinks seam.
 *   4. singular vs plural copy: "1 backlink"/"2 backlinks", "1 word", "1 character".
 * The backlinks query is driven through the mocked api/client seam (no network),
 * mirroring BacklinksPanel.test.tsx.
 */
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

vi.mock("../api/client", async (importOriginal) => {
  const actual = await importOriginal<typeof client>();
  return {
    ...actual,
    getBacklinks: vi.fn(),
  };
});

import * as client from "../api/client";
import type { Backlink } from "../api/client";
import DocStatusBar from "./DocStatusBar";

function renderBar(
  body: string,
  { path = "notes/current.md", backlinks = [] as Backlink[] } = {},
) {
  vi.mocked(client.getBacklinks).mockResolvedValue(backlinks);
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  // Seed the query cache so the backlink count is available synchronously and no
  // fetch is needed for the render assertions (staleTime keeps it fresh).
  qc.setQueryData(["backlinks", path], backlinks);
  return render(
    <QueryClientProvider client={qc}>
      <DocStatusBar path={path} body={body} />
    </QueryClientProvider>,
  );
}

describe("DocStatusBar", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("counts words by whitespace split of the trimmed body", () => {
    renderBar("  one two three   four  ");
    expect(screen.getByText("4 words")).toBeInTheDocument();
  });

  it("reports 0 words for an empty body and for a whitespace-only body", () => {
    const { unmount } = renderBar("");
    expect(screen.getByText("0 words")).toBeInTheDocument();
    unmount();

    renderBar("   \n\t ");
    expect(screen.getByText("0 words")).toBeInTheDocument();
  });

  it("uses the singular 'word' when there is exactly one", () => {
    renderBar("solo");
    expect(screen.getByText("1 word")).toBeInTheDocument();
  });

  it("counts characters as the RAW body length (whitespace included, not trimmed)", () => {
    // Two chars of content + surrounding whitespace → length 6, not the trimmed 2.
    renderBar("  ab  ");
    expect(screen.getByText(/^6 characters$/)).toBeInTheDocument();
  });

  it("uses the singular 'character' for a one-character body", () => {
    renderBar("x");
    expect(screen.getByText("1 character")).toBeInTheDocument();
  });

  it("groups a large character count with a locale thousands separator", () => {
    renderBar("z".repeat(1500));
    // Locale-agnostic: assert a grouping separator sits between the digit groups
    // (the guard against a raw String(chars) that forgot toLocaleString).
    const chars = screen.getByText(/ characters$/);
    expect(chars).toHaveTextContent(/^1\D500 characters$/);
  });

  it("shows the seeded backlink count with plural copy", () => {
    renderBar("body", {
      backlinks: [
        { path: "a.md", title: "A" },
        { path: "b.md", title: "B" },
        { path: "c.md", title: "C" },
      ],
    });
    expect(screen.getByText("3 backlinks")).toBeInTheDocument();
  });

  it("uses the singular 'backlink' for exactly one", () => {
    renderBar("body", { backlinks: [{ path: "a.md", title: "A" }] });
    expect(screen.getByText("1 backlink")).toBeInTheDocument();
  });

  it("shows 0 backlinks when there are none", () => {
    renderBar("body", { backlinks: [] });
    expect(screen.getByText("0 backlinks")).toBeInTheDocument();
  });
});
