// GRAPH-02 pure model helpers: degree counting + orphan classification.
// DOM-free, data-in/data-out — these back the canvas node sizing/orphan fill
// without the canvas needing to recompute adjacency.
import { describe, it, expect } from "vitest";

import { computeDegrees, isOrphan } from "./model";
import type { GraphEdge, GraphNode } from "../../api/client";

const node = (id: string, type: GraphNode["type"] = "page"): GraphNode => ({
  id,
  label: id,
  type,
});
const edge = (
  source: string,
  target: string,
  type: GraphEdge["type"] = "link",
): GraphEdge => ({ source, target, type });

describe("computeDegrees (GRAPH-02)", () => {
  it("counts edges touching a node as source or target", () => {
    // A-B-C chain: B is in the middle (degree 2), A and C are ends (degree 1).
    const nodes = [node("A"), node("B"), node("C")];
    const edges = [edge("A", "B"), edge("B", "C")];
    const degrees = computeDegrees(nodes, edges);
    expect(degrees.get("A")).toBe(1);
    expect(degrees.get("B")).toBe(2);
    expect(degrees.get("C")).toBe(1);
  });

  it("gives a node with no edges degree 0", () => {
    const degrees = computeDegrees([node("X")], []);
    expect(degrees.get("X")).toBe(0);
  });

  it("counts every node present even when not referenced by any edge", () => {
    const degrees = computeDegrees([node("A"), node("B"), node("solo")], [
      edge("A", "B"),
    ]);
    expect(degrees.get("solo")).toBe(0);
    expect(degrees.size).toBe(3);
  });
});

describe("isOrphan (GRAPH-02)", () => {
  it("a page node with degree 0 is an orphan", () => {
    const degrees = new Map<string, number>([["p", 0]]);
    expect(isOrphan("p", degrees, "page")).toBe(true);
  });

  it("a page node with >= 1 edge is not an orphan", () => {
    const degrees = new Map<string, number>([["p", 2]]);
    expect(isOrphan("p", degrees, "page")).toBe(false);
  });

  it("a tag node is never an orphan, even at degree 0", () => {
    const degrees = new Map<string, number>([["tag:foo", 0]]);
    expect(isOrphan("tag:foo", degrees, "tag")).toBe(false);
  });

  it("an unknown node id (missing from the degree map) treats degree as 0", () => {
    expect(isOrphan("ghost", new Map(), "page")).toBe(true);
  });
});
