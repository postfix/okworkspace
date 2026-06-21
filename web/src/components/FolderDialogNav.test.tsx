/**
 * CR-01 — folder rename/move must navigate to the folder's index.md, NOT the
 * bare folder directory. The server returns the raw folder dir (e.g. "guides")
 * for a folder rename/move; navigating there hits getPage on a directory →
 * HTTP 500. The folder's addressable home page is "<dir>/index.md".
 */
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";

const navigateMock = vi.fn();

vi.mock("react-router-dom", async (importOriginal) => {
  const actual = await importOriginal<typeof import("react-router-dom")>();
  return { ...actual, useNavigate: () => navigateMock };
});

vi.mock("../api/client", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../api/client")>();
  return {
    ...actual,
    renameFolder: vi.fn(async () => ({ path: "guides" })),
    renamePage: vi.fn(async () => ({ path: "guides/welcome.md" })),
    moveFolder: vi.fn(async () => ({ path: "newparent/guides" })),
    movePage: vi.fn(async () => ({ path: "newparent/welcome.md" })),
    getTree: vi.fn(async () => []),
  };
});

import RenameModal from "./RenameModal";
import MoveDialog from "./MoveDialog";

function wrapper({ children }: { children: ReactNode }) {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
}

beforeEach(() => {
  navigateMock.mockReset();
});

describe("RenameModal folder navigation (CR-01)", () => {
  it("navigates to <newdir>/index.md after a folder rename, not the bare dir", async () => {
    const user = userEvent.setup();
    render(
      <RenameModal
        open
        path="guides"
        currentTitle="Guides"
        kind="folder"
        onClose={() => {}}
      />,
      { wrapper },
    );
    await user.click(screen.getByRole("button", { name: "Rename" }));
    await waitFor(() => {
      expect(navigateMock).toHaveBeenCalledWith("/app/page/guides/index.md");
    });
  });

  it("navigates to the page path (no index.md) after a page rename", async () => {
    const user = userEvent.setup();
    render(
      <RenameModal
        open
        path="guides/welcome.md"
        currentTitle="Welcome"
        kind="page"
        onClose={() => {}}
      />,
      { wrapper },
    );
    await user.click(screen.getByRole("button", { name: "Rename" }));
    await waitFor(() => {
      expect(navigateMock).toHaveBeenCalledWith(
        "/app/page/guides/welcome.md",
      );
    });
  });
});

describe("MoveDialog folder navigation (CR-01)", () => {
  it("navigates to <newdir>/index.md after a folder move, not the bare dir", async () => {
    const user = userEvent.setup();
    render(
      <MoveDialog open path="guides" kind="folder" onClose={() => {}} />,
      { wrapper },
    );
    await user.click(screen.getByRole("button", { name: "Move folder" }));
    await waitFor(() => {
      expect(navigateMock).toHaveBeenCalledWith(
        "/app/page/newparent/guides/index.md",
      );
    });
  });

  it("navigates to the page path (no index.md) after a page move", async () => {
    const user = userEvent.setup();
    render(
      <MoveDialog open path="welcome.md" kind="page" onClose={() => {}} />,
      { wrapper },
    );
    await user.click(screen.getByRole("button", { name: "Move page" }));
    await waitFor(() => {
      expect(navigateMock).toHaveBeenCalledWith(
        "/app/page/newparent/welcome.md",
      );
    });
  });
});
