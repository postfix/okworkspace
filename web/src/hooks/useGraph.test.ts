// GRAPH-01 — thin react-query wrapper over getGraph. We assert the query WIRING
// (queryKey shape) rather than the canvas.
import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createElement, type ReactNode } from "react";

import * as client from "../api/client";
import { useGraph } from "./useGraph";

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

describe("useGraph (GRAPH-01)", () => {
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
});
