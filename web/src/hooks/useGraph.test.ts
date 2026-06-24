// GRAPH-01/03 — thin react-query wrappers over getGraph / getLocalGraph.
// We assert the query WIRING (queryKey shape + the `enabled` gate) rather than
// the canvas: useLocalGraph must key on [path, depth] so it auto-refetches when
// either changes, and must be disabled for an empty path.
import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createElement, type ReactNode } from "react";

import * as client from "../api/client";
import { useGraph, useLocalGraph } from "./useGraph";

const sample: client.GraphData = {
  nodes: [{ id: "a", label: "A", type: "page" }],
  edges: [],
};

function wrapper() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  });
  return ({ children }: { children: ReactNode }) =>
    createElement(QueryClientProvider, { client: qc }, children);
}

describe("useGraph / useLocalGraph (GRAPH-01/03)", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it("useGraph calls getGraph and resolves the payload", async () => {
    const spy = vi.spyOn(client, "getGraph").mockResolvedValue(sample);
    const { result } = renderHook(() => useGraph(), { wrapper: wrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(spy).toHaveBeenCalledTimes(1);
    expect(result.current.data).toEqual(sample);
  });

  it("useLocalGraph is disabled (no fetch) when path is empty", async () => {
    const spy = vi.spyOn(client, "getLocalGraph").mockResolvedValue(sample);
    const { result } = renderHook(() => useLocalGraph("", 1), {
      wrapper: wrapper(),
    });
    // An empty path must not fire the query (enabled: path !== "").
    expect(result.current.fetchStatus).toBe("idle");
    expect(spy).not.toHaveBeenCalled();
  });

  it("useLocalGraph fetches with path + depth when path is non-empty", async () => {
    const spy = vi.spyOn(client, "getLocalGraph").mockResolvedValue(sample);
    const { result } = renderHook(() => useLocalGraph("notes/a.md", 2), {
      wrapper: wrapper(),
    });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(spy).toHaveBeenCalledWith("notes/a.md", 2);
  });

  it("useLocalGraph re-fetches when depth changes (queryKey carries depth)", async () => {
    const spy = vi.spyOn(client, "getLocalGraph").mockResolvedValue(sample);
    const { result, rerender } = renderHook(
      ({ p, d }: { p: string; d: number }) => useLocalGraph(p, d),
      { wrapper: wrapper(), initialProps: { p: "notes/a.md", d: 1 } },
    );
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    rerender({ p: "notes/a.md", d: 3 });
    await waitFor(() =>
      expect(spy).toHaveBeenCalledWith("notes/a.md", 3),
    );
    // Two distinct query keys → both depths were fetched.
    expect(spy.mock.calls).toContainEqual(["notes/a.md", 1]);
    expect(spy.mock.calls).toContainEqual(["notes/a.md", 3]);
  });
});
