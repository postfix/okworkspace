/**
 * TREE-03/06 — pure helpers behind the optimistic tree mutations.
 *
 * applyMove MUST mirror the server's literal prefix swap (Pitfall 6) so the
 * optimistic tree equals the eventual refetch; dropAllowed is the client-side
 * self/descendant/same-parent guard (TREE-06). These are pure functions, tested
 * without rendering.
 */
import { describe, it, expect, vi } from "vitest";
import { renderHook, act, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";

import {
  applyMove,
  dropAllowed,
  parentOf,
  basename,
  destDir,
  countFolderPages,
  useTreeMove,
} from "./useTreeMutations";
import type { TreeNode } from "../../api/client";

vi.mock("../../api/client", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../../api/client")>();
  return {
    ...actual,
    movePage: vi.fn(),
    moveFolder: vi.fn(),
    deleteFolder: vi.fn(),
    renameFolder: vi.fn(),
  };
});

import { moveFolder } from "../../api/client";

const TREE: TreeNode[] = [
  { type: "page", path: "home.md", title: "Home" },
  {
    type: "folder",
    path: "runbooks",
    title: "runbooks",
    children: [
      { type: "page", path: "runbooks/deploy.md", title: "Deploy" },
      {
        type: "folder",
        path: "runbooks/aws",
        title: "aws",
        children: [
          { type: "page", path: "runbooks/aws/ec2.md", title: "EC2" },
        ],
      },
    ],
  },
  {
    type: "folder",
    path: "archive",
    title: "archive",
    children: [],
  },
];

describe("path helpers", () => {
  it("parentOf returns the containing folder ('' = root)", () => {
    expect(parentOf("home.md")).toBe("");
    expect(parentOf("a/b.md")).toBe("a");
    expect(parentOf("a/b/c")).toBe("a/b");
  });
  it("basename returns the last segment", () => {
    expect(basename("home.md")).toBe("home.md");
    expect(basename("a/b/c.md")).toBe("c.md");
    expect(basename("a/b")).toBe("b");
  });
  it("destDir computes the moved path ('' = root)", () => {
    expect(destDir("home.md", "runbooks")).toBe("runbooks/home.md");
    expect(destDir("runbooks/deploy.md", "")).toBe("deploy.md");
    expect(destDir("runbooks/aws", "archive")).toBe("archive/aws");
    expect(destDir("runbooks/aws", "")).toBe("aws");
  });
});

describe("dropAllowed (TREE-06 guard)", () => {
  it("rejects a folder onto itself", () => {
    expect(dropAllowed("folder", "runbooks", "runbooks")).toBe(false);
  });
  it("rejects a folder onto its own descendant", () => {
    expect(dropAllowed("folder", "runbooks", "runbooks/aws")).toBe(false);
  });
  it("rejects a same-parent no-op for a folder", () => {
    // runbooks/aws lives in runbooks; dropping it back onto runbooks is a no-op.
    expect(dropAllowed("folder", "runbooks/aws", "runbooks")).toBe(false);
  });
  it("rejects a same-parent no-op for a page", () => {
    expect(dropAllowed("page", "runbooks/deploy.md", "runbooks")).toBe(false);
  });
  it("allows a valid folder move", () => {
    expect(dropAllowed("folder", "runbooks/aws", "archive")).toBe(true);
    expect(dropAllowed("folder", "runbooks", "")).toBe(false); // runbooks already at root
    expect(dropAllowed("folder", "archive", "runbooks")).toBe(true);
  });
  it("allows a valid page move", () => {
    expect(dropAllowed("page", "home.md", "runbooks")).toBe(true);
  });
});

describe("applyMove (mirrors server prefix swap, Pitfall 6)", () => {
  it("moves a page into a folder, rewriting its path", () => {
    const next = applyMove(TREE, "home.md", "runbooks", "page");
    // home.md is gone from root, present under runbooks as runbooks/home.md.
    expect(next.find((n) => n.path === "home.md")).toBeUndefined();
    const runbooks = next.find((n) => n.path === "runbooks");
    expect(
      runbooks?.children?.some((c) => c.path === "runbooks/home.md"),
    ).toBe(true);
  });

  it("moves a FOLDER as a unit, rewriting EVERY descendant path by prefix swap", () => {
    const next = applyMove(TREE, "runbooks/aws", "archive", "folder");
    const archive = next.find((n) => n.path === "archive");
    const aws = archive?.children?.find((c) => c.path === "archive/aws");
    expect(aws).toBeTruthy();
    // The descendant page path is rewritten by literal prefix swap — exactly what
    // the server relocateFolder refetch would return (no node "jump").
    expect(aws?.children?.[0].path).toBe("archive/aws/ec2.md");
    // It's gone from its old parent.
    const runbooks = next.find((n) => n.path === "runbooks");
    expect(
      runbooks?.children?.some((c) => c.path === "runbooks/aws"),
    ).toBe(false);
  });

  it("moves a folder to root, dropping the old parent prefix", () => {
    const next = applyMove(TREE, "runbooks/aws", "", "folder");
    const aws = next.find((n) => n.path === "aws");
    expect(aws).toBeTruthy();
    expect(aws?.children?.[0].path).toBe("aws/ec2.md");
  });

  it("is a no-op when the node is missing or already in the destination", () => {
    expect(applyMove(TREE, "nope.md", "runbooks", "page")).toBe(TREE);
    // runbooks/deploy.md already lives in runbooks.
    expect(applyMove(TREE, "runbooks/deploy.md", "runbooks", "page")).toBe(TREE);
  });

  it("does not mutate the input snapshot (rollback safety)", () => {
    const before = JSON.stringify(TREE);
    applyMove(TREE, "runbooks/aws", "archive", "folder");
    expect(JSON.stringify(TREE)).toBe(before);
  });
});

describe("countFolderPages", () => {
  it("counts every descendant .md file (pages + each subfolder's index.md)", () => {
    // runbooks → deploy.md (page) + aws (subfolder index.md) + aws/ec2.md (page)
    // = 3 files actually moved to trash (IN-01: subfolder index.md files count).
    expect(countFolderPages(TREE, "runbooks")).toBe(3);
    expect(countFolderPages(TREE, "runbooks/aws")).toBe(1);
    expect(countFolderPages(TREE, "archive")).toBe(0);
    expect(countFolderPages(TREE, "missing")).toBe(0);
  });
});

function wrapper(qc: QueryClient) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
  };
}

describe("useTreeMove optimistic apply + rollback", () => {
  it("applies the move to the ['tree'] cache immediately, then reconciles on success", async () => {
    const qc = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    });
    qc.setQueryData(["tree"], TREE);
    vi.mocked(moveFolder).mockResolvedValue({ path: "archive/aws" });

    const { result } = renderHook(() => useTreeMove(), { wrapper: wrapper(qc) });
    act(() => {
      result.current.mutate({ src: "runbooks/aws", dest: "archive", kind: "folder" });
    });

    // Optimistic: the cache reflects the move synchronously (before the network).
    await waitFor(() => {
      const tree = qc.getQueryData<TreeNode[]>(["tree"]) ?? [];
      const archive = tree.find((n) => n.path === "archive");
      expect(archive?.children?.some((c) => c.path === "archive/aws")).toBe(true);
    });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
  });

  it("rolls back to the snapshot on error and reports the rollback copy", async () => {
    const qc = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    });
    qc.setQueryData(["tree"], TREE);
    vi.mocked(moveFolder).mockRejectedValue(new Error("boom"));
    const onRollback = vi.fn();

    const { result } = renderHook(() => useTreeMove(onRollback), {
      wrapper: wrapper(qc),
    });
    act(() => {
      result.current.mutate({ src: "runbooks/aws", dest: "archive", kind: "folder" });
    });

    await waitFor(() => expect(result.current.isError).toBe(true));
    // The cache is restored to the original snapshot (runbooks/aws back home).
    const tree = qc.getQueryData<TreeNode[]>(["tree"]) ?? [];
    const runbooks = tree.find((n) => n.path === "runbooks");
    expect(runbooks?.children?.some((c) => c.path === "runbooks/aws")).toBe(true);
    expect(onRollback).toHaveBeenCalledWith(
      expect.stringContaining("back where it was"),
    );
  });
});
