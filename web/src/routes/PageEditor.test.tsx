/**
 * PAGE-02 — PageEditor saves on click and surfaces the 409 conflict banner.
 */
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor, act } from "@testing-library/react";
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

// Mock the CM6 LivePreviewEditor with a plain textarea so the test stays fast and
// deterministic (the editor's exact rendering is covered by LivePreviewEditor.test;
// here we only exercise PageEditor's save machinery via the value/onChange seam).
vi.mock("../components/LivePreviewEditor", () => ({
  default: ({
    value,
    onChange,
  }: {
    value: string;
    onChange: (v: string) => void;
    currentPath: string;
    mode: "live" | "source";
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
    // Reset call history between tests so per-test call-count assertions are not
    // polluted by a prior test's savePage/getPage calls.
    vi.clearAllMocks();
    vi.mocked(client.getPage).mockResolvedValue({
      frontmatter: "type: Page\ntitle: Notes\n",
      body: "original",
      revision: "rev-1",
    });
    vi.mocked(client.getTree).mockResolvedValue([]);
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

  it("does not start an overlapping save while one is in flight (WR-03)", async () => {
    // savePage resolves on a deferred promise so we can hold the first save
    // in-flight and prove a burst of Save clicks during that window is dropped.
    let releaseSave: () => void = () => {};
    let savePromise = new Promise<undefined>((resolve) => {
      releaseSave = () => resolve(undefined);
    });
    vi.mocked(client.savePage).mockImplementation(() => savePromise);

    renderEditor("notes.md");
    await screen.findByLabelText("body");

    // Fire a burst of Save clicks synchronously. The first starts a save (the
    // returned promise is held unresolved); every subsequent click while it is
    // in flight must be dropped by the in-flight guard, so savePage is called
    // exactly once — two PUTs never race on the same base revision (WR-03). No
    // fireEvent.change is used, so no autosave draft timer can perturb the count.
    const saveBtn = screen.getByRole("button", { name: "Save page" });
    fireEvent.click(saveBtn);
    fireEvent.click(saveBtn);
    fireEvent.click(saveBtn);
    fireEvent.click(saveBtn);
    expect(client.savePage).toHaveBeenCalledTimes(1);

    // Release the in-flight save (resolve immediately); the getPage refetch
    // advances the revision and clears the guard.
    savePromise = Promise.resolve(undefined);
    releaseSave();
    await waitFor(() => expect(client.getPage).toHaveBeenCalled());

    // A fresh save after the in-flight one settled is allowed again.
    fireEvent.click(saveBtn);
    await waitFor(() => expect(client.savePage).toHaveBeenCalledTimes(2));
  });

  it("flushes a trailing edit typed during an in-flight save (no lost write)", async () => {
    vi.useFakeTimers();
    // Drain timers + microtasks repeatedly so a chain of awaited promises settles.
    const flush = async () => {
      for (let i = 0; i < 8; i++) {
        await act(async () => {
          await vi.advanceTimersByTimeAsync(0);
        });
      }
    };
    try {
      // The first autosave is held in flight; later saves resolve immediately.
      let release: () => void = () => {};
      const held = new Promise<undefined>((resolve) => {
        release = () => resolve(undefined);
      });
      vi.mocked(client.savePage)
        .mockImplementationOnce(() => held)
        .mockResolvedValue(undefined);
      // Initial load returns rev-1; every post-save refetch returns a fresh rev.
      vi.mocked(client.getPage)
        .mockResolvedValueOnce({
          frontmatter: "type: Page\ntitle: Notes\n",
          body: "original",
          revision: "rev-1",
        })
        .mockResolvedValue({
          frontmatter: "type: Page\ntitle: Notes\n",
          body: "original",
          revision: "rev-2",
        });

      renderEditor("notes.md");
      await flush();
      const body = screen.getByLabelText("body");

      // First edit "A": the 1s draft timer fires the first save (held in flight).
      fireEvent.change(body, { target: { value: "A" } });
      await act(async () => {
        await vi.advanceTimersByTimeAsync(1000);
      });
      expect(client.savePage).toHaveBeenCalledTimes(1);
      expect(vi.mocked(client.savePage).mock.calls[0][1]).toEqual(
        expect.objectContaining({ body: "A" }),
      );

      // Trailing edit "AB" typed WHILE the first save is in flight. Its draft
      // timer fires but is dropped by the in-flight guard — it must not be lost.
      fireEvent.change(body, { target: { value: "AB" } });
      await act(async () => {
        await vi.advanceTimersByTimeAsync(1000);
      });
      expect(client.savePage).toHaveBeenCalledTimes(1); // dropped while in flight

      // Release the in-flight save; the success path must flush the trailing edit.
      release();
      await flush();

      expect(client.savePage).toHaveBeenCalledTimes(2);
      expect(vi.mocked(client.savePage).mock.calls[1][1]).toEqual(
        expect.objectContaining({ body: "AB" }),
      );
    } finally {
      vi.useRealTimers();
    }
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
