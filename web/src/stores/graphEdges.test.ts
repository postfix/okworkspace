// GRAPH-04 — the persisted edge-toggle store. Mirrors editorMode.test.ts:
// default values, toggle(kind), and persistence under the named localStorage key.
// Critical default: Shared tags is OFF (success criterion — first view is not a
// hairball); Links and Backlinks default ON.
import { describe, it, expect, beforeEach } from "vitest";

import { useGraphEdges } from "./graphEdges";

describe("graphEdges store (GRAPH-04)", () => {
  beforeEach(() => {
    localStorage.clear();
    useGraphEdges.setState({ links: true, backlinks: true, sharedTags: false });
  });

  it("defaults Links ON, Backlinks ON, Shared tags OFF", () => {
    const s = useGraphEdges.getState();
    expect(s.links).toBe(true);
    expect(s.backlinks).toBe(true);
    expect(s.sharedTags).toBe(false);
  });

  it("toggle('sharedTags') flips shared tags on then off", () => {
    useGraphEdges.getState().toggle("sharedTags");
    expect(useGraphEdges.getState().sharedTags).toBe(true);
    useGraphEdges.getState().toggle("sharedTags");
    expect(useGraphEdges.getState().sharedTags).toBe(false);
  });

  it("toggle('links') flips links", () => {
    useGraphEdges.getState().toggle("links");
    expect(useGraphEdges.getState().links).toBe(false);
    useGraphEdges.getState().toggle("links");
    expect(useGraphEdges.getState().links).toBe(true);
  });

  it("toggle('backlinks') flips backlinks independently of links", () => {
    useGraphEdges.getState().toggle("backlinks");
    expect(useGraphEdges.getState().backlinks).toBe(false);
    expect(useGraphEdges.getState().links).toBe(true);
  });

  it("persists under the okf.graph.edges key", () => {
    useGraphEdges.getState().toggle("sharedTags");
    const raw = localStorage.getItem("okf.graph.edges");
    expect(raw).toBeTruthy();
    expect(raw).toContain("sharedTags");
  });
});
