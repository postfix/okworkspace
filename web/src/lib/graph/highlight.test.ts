// GRAPH-05 pure hover-highlight helper: the focused node + its immediate
// neighbors + the connecting edges. The canvas dims everything NOT in these sets.
import { describe, it, expect } from "vitest";

import { neighborHighlight } from "./highlight";
import type { GraphEdge } from "../../api/client";

const edge = (
  source: string,
  target: string,
  type: GraphEdge["type"] = "link",
): GraphEdge => ({ source, target, type });

describe("neighborHighlight (GRAPH-05)", () => {
  it("focusing the middle of A-B-C returns all three nodes and both edges", () => {
    const edges = [edge("A", "B"), edge("B", "C")];
    const { nodes, edges: hl } = neighborHighlight("B", edges);
    expect(nodes).toEqual(new Set(["A", "B", "C"]));
    expect(hl.size).toBe(2);
  });

  it("focusing an end node returns only itself + its one neighbor + the one edge", () => {
    const edges = [edge("A", "B"), edge("B", "C")];
    const { nodes, edges: hl } = neighborHighlight("A", edges);
    expect(nodes).toEqual(new Set(["A", "B"]));
    expect(hl.size).toBe(1);
  });

  it("focusing an isolated node returns just itself and no edges", () => {
    const edges = [edge("A", "B")];
    const { nodes, edges: hl } = neighborHighlight("solo", edges);
    expect(nodes).toEqual(new Set(["solo"]));
    expect(hl.size).toBe(0);
  });

  it("a null focus returns empty sets", () => {
    const { nodes, edges: hl } = neighborHighlight(null, [edge("A", "B")]);
    expect(nodes.size).toBe(0);
    expect(hl.size).toBe(0);
  });

  it("an undefined focus returns empty sets", () => {
    const { nodes, edges: hl } = neighborHighlight(undefined, [edge("A", "B")]);
    expect(nodes.size).toBe(0);
    expect(hl.size).toBe(0);
  });
});
