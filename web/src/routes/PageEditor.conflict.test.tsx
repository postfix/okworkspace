/**
 * COLL-04 — PageEditor conflict flow. A stale-save 409 opens the conflict dialog
 * (a REAL diff, the in-flight edit intact) instead of a silent overwrite; autosave
 * is gated while the dialog is open (no thrash, no dropped edit); each resolution
 * choice routes through the EXISTING revision-checked save path:
 *
 *   (a) 409 opens the conflict dialog with a real diff and my body intact.
 *   (b) while the dialog is open, the autosave debounce does NOT fire another save.
 *   (c) Save as copy calls createPage then savePage(NEW path) and navigates — and
 *       NEVER calls savePage(original path).
 *   (d) Overwrite calls getPage (fresh rev) then savePage(original path) at that
 *       fresh revision.
 */
import { describe, it, expect, vi, beforeEach } from "vitest";
import {
  render,
  screen,
  fireEvent,
  waitFor,
  act,
} from "@testing-library/react";
import { MemoryRouter, Routes, Route } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const navigateSpy = vi.fn();
vi.mock("react-router-dom", async (importOriginal) => {
  const actual = await importOriginal<typeof import("react-router-dom")>();
  return { ...actual, useNavigate: () => navigateSpy };
});

vi.mock("../api/client", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../api/client")>();
  return {
    ...actual,
    getPage: vi.fn(),
    savePage: vi.fn(),
    createPage: vi.fn(),
    getTree: vi.fn().mockResolvedValue([]),
    acquireLock: vi.fn().mockResolvedValue({ result: "acquired" }),
    releaseLock: vi.fn().mockResolvedValue(undefined),
    forceLock: vi.fn().mockResolvedValue(undefined),
  };
});

// Plain-textarea stand-in for the CM6 editor (same seam as PageEditor.test).
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

// Presence subscribes over SSE — stub it so the conflict test never opens a stream.
vi.mock("../components/PresenceIndicator", () => ({ default: () => null }));

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

const STALE_409 = (() => {
  const err = new Error("conflict") as Error & { status?: number };
  err.status = 409;
  return err;
})();

describe("PageEditor conflict flow (COLL-04)", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    navigateSpy.mockReset();
    // Initial load + the server version fetched on 409 both come from getPage; the
    // first call seeds the editor, later calls return the (different) server body.
    vi.mocked(client.getPage).mockResolvedValue({
      frontmatter: "type: Page\ntitle: Notes\n",
      body: "their server version",
      revision: "rev-server",
    });
    vi.mocked(client.getTree).mockResolvedValue([]);
  });

  it("opens the conflict dialog on a 409 with a real diff and the edit intact", async () => {
    // First getPage = initial load (my base); subsequent = server version on 409.
    vi.mocked(client.getPage)
      .mockResolvedValueOnce({
        frontmatter: "type: Page\ntitle: Notes\n",
        body: "original",
        revision: "rev-1",
      })
      .mockResolvedValue({
        frontmatter: "type: Page\ntitle: Notes\n",
        body: "their server version",
        revision: "rev-2",
      });
    vi.mocked(client.savePage).mockRejectedValue(STALE_409);

    renderEditor("notes.md");
    const body = await screen.findByLabelText("body");
    fireEvent.change(body, { target: { value: "MY EDIT" } });
    fireEvent.click(screen.getByRole("button", { name: "Save page" }));

    // The dialog opens with the conflict title and a real diff (a diff <table>).
    expect(
      await screen.findByText(/this page was changed somewhere else/i),
    ).toBeInTheDocument();
    const region = screen.getByLabelText("Proposed changes");
    expect(region.querySelector("table")).toBeTruthy();

    // My in-flight edit is NOT dropped — the textarea still holds it.
    expect((screen.getByLabelText("body") as HTMLTextAreaElement).value).toBe(
      "MY EDIT",
    );
  });

  it("gates autosave while the conflict dialog is open (no re-save / thrash)", async () => {
    vi.useFakeTimers();
    const flush = async () => {
      for (let i = 0; i < 8; i++) {
        await act(async () => {
          await vi.advanceTimersByTimeAsync(0);
        });
      }
    };
    try {
      vi.mocked(client.getPage)
        .mockResolvedValueOnce({
          frontmatter: "type: Page\ntitle: Notes\n",
          body: "original",
          revision: "rev-1",
        })
        .mockResolvedValue({
          frontmatter: "type: Page\ntitle: Notes\n",
          body: "their server version",
          revision: "rev-2",
        });
      vi.mocked(client.savePage).mockRejectedValue(STALE_409);

      renderEditor("notes.md");
      await flush();
      const body = screen.getByLabelText("body");

      // Edit + click Save → 409 → dialog opens; savePage called once so far.
      fireEvent.change(body, { target: { value: "A" } });
      fireEvent.click(screen.getByRole("button", { name: "Save page" }));
      await flush();
      expect(client.savePage).toHaveBeenCalledTimes(1);

      // Type again while the dialog is open and let the debounce window pass.
      // The autosave must NOT re-arm → savePage is still called only once.
      fireEvent.change(body, { target: { value: "AB" } });
      await act(async () => {
        await vi.advanceTimersByTimeAsync(2000);
      });
      await flush();
      expect(client.savePage).toHaveBeenCalledTimes(1);
    } finally {
      vi.useRealTimers();
    }
  });

  it("Save as copy creates+saves a NEW page and navigates — never the original", async () => {
    vi.mocked(client.getPage)
      .mockResolvedValueOnce({
        frontmatter: "type: Page\ntitle: Notes\n",
        body: "original",
        revision: "rev-1",
      })
      // server version on the 409
      .mockResolvedValueOnce({
        frontmatter: "type: Page\ntitle: Notes\n",
        body: "their server version",
        revision: "rev-2",
      })
      // fresh revision of the newly-created copy
      .mockResolvedValue({
        frontmatter: "type: Page\ntitle: Notes (Copy)\n",
        body: "",
        revision: "rev-copy",
      });
    vi.mocked(client.savePage)
      .mockRejectedValueOnce(STALE_409) // the original-path save 409s → dialog
      .mockResolvedValue(undefined); // the copy save succeeds
    vi.mocked(client.createPage).mockResolvedValue({ path: "notes-copy.md" });

    renderEditor("notes.md");
    const body = await screen.findByLabelText("body");
    fireEvent.change(body, { target: { value: "MY EDIT" } });
    fireEvent.click(screen.getByRole("button", { name: "Save page" }));

    await screen.findByText(/this page was changed somewhere else/i);
    fireEvent.click(
      screen.getByRole("button", { name: /save my version as a new page/i }),
    );

    // createPage is called for the copy, then savePage(NEW path) — and the copy
    // navigation fires. savePage is NEVER called for the original path after the
    // initial 409.
    await waitFor(() => expect(client.createPage).toHaveBeenCalledTimes(1));
    expect(vi.mocked(client.createPage).mock.calls[0][1]).toMatch(/\(Copy\)/);
    await waitFor(() =>
      expect(client.savePage).toHaveBeenCalledWith(
        "notes-copy.md",
        expect.objectContaining({ body: "MY EDIT", base_revision: "rev-copy" }),
      ),
    );
    // No savePage to the original path beyond the first (409'd) attempt.
    const savesToOriginal = vi
      .mocked(client.savePage)
      .mock.calls.filter((c) => c[0] === "notes.md");
    expect(savesToOriginal).toHaveLength(1); // only the initial 409 attempt
    await waitFor(() =>
      expect(navigateSpy).toHaveBeenCalledWith("/app/edit/notes-copy.md"),
    );
  });

  it("Overwrite fetches the fresh revision then saves the original at it", async () => {
    vi.mocked(client.getPage)
      .mockResolvedValueOnce({
        frontmatter: "type: Page\ntitle: Notes\n",
        body: "original",
        revision: "rev-1",
      })
      // server version on the 409
      .mockResolvedValueOnce({
        frontmatter: "type: Page\ntitle: Notes\n",
        body: "their server version",
        revision: "rev-2",
      })
      // Overwrite re-fetches the current revision, then again after the save lands.
      .mockResolvedValue({
        frontmatter: "type: Page\ntitle: Notes\n",
        body: "their server version",
        revision: "rev-2",
      });
    vi.mocked(client.savePage)
      .mockRejectedValueOnce(STALE_409) // initial save 409s → dialog
      .mockResolvedValue(undefined); // overwrite save succeeds

    renderEditor("notes.md");
    const body = await screen.findByLabelText("body");
    fireEvent.change(body, { target: { value: "MY EDIT" } });
    fireEvent.click(screen.getByRole("button", { name: "Save page" }));

    await screen.findByText(/this page was changed somewhere else/i);
    fireEvent.click(
      screen.getByRole("button", { name: /overwrite with my version/i }),
    );

    // Overwrite saves the ORIGINAL path at the freshly-fetched revision (rev-2).
    await waitFor(() =>
      expect(client.savePage).toHaveBeenCalledWith(
        "notes.md",
        expect.objectContaining({ body: "MY EDIT", base_revision: "rev-2" }),
      ),
    );
  });
});
