/**
 * PAGE-02 — PageEditor saves on click and surfaces the 409 conflict banner.
 */
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { MemoryRouter, Routes, Route } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

vi.mock("../api/client", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../api/client")>();
  return {
    ...actual,
    getPage: vi.fn(),
    savePage: vi.fn(),
    getTree: vi.fn().mockResolvedValue([]),
  };
});

// Mock the heavy markdown editor with a plain textarea so the test stays fast
// and deterministic (the editor's exact rendering is not under test here).
vi.mock("@uiw/react-md-editor", () => ({
  default: ({
    value,
    onChange,
  }: {
    value: string;
    onChange: (v?: string) => void;
  }) => (
    <textarea
      aria-label="body"
      value={value}
      onChange={(e) => onChange(e.target.value)}
    />
  ),
}));

import * as client from "../api/client";
import PageEditor from "./PageEditor";

function renderEditor(path: string) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={[`/app/edit/${path}`]}>
        <Routes>
          <Route path="/app/edit/*" element={<PageEditor />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("PageEditor", () => {
  beforeEach(() => {
    vi.mocked(client.getPage).mockResolvedValue({
      frontmatter: "type: Page\ntitle: Notes\n",
      body: "original",
      revision: "rev-1",
    });
  });

  it("issues a save PUT on Save page click", async () => {
    vi.mocked(client.savePage).mockResolvedValue(undefined);
    // After save, doSave refetches the page for the new revision.
    renderEditor("notes.md");
    const body = await screen.findByLabelText("body");
    fireEvent.change(body, { target: { value: "edited body" } });

    fireEvent.click(screen.getByRole("button", { name: "Save page" }));

    await waitFor(() =>
      expect(client.savePage).toHaveBeenCalledWith(
        "notes.md",
        expect.objectContaining({ body: "edited body", base_revision: "rev-1" }),
      ),
    );
  });

  it("surfaces the conflict banner on a 409", async () => {
    const err = new Error("conflict") as Error & { status?: number };
    err.status = 409;
    vi.mocked(client.savePage).mockRejectedValue(err);

    renderEditor("notes.md");
    await screen.findByLabelText("body");
    fireEvent.click(screen.getByRole("button", { name: "Save page" }));

    expect(
      await screen.findByText(/changed somewhere else since you opened it/i),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Reload page" })).toBeInTheDocument();
  });
});
