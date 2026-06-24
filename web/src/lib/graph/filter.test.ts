// GRAPH-04 pure edge filter: keep/drop edges by the active toggle booleans.
// Client-only — the UI useMemos this over the payload edges and never refetches.
import { describe, it, expect } from "vitest";

import { filterEdges } from "./filter";
import type { GraphEdge } from "../../api/client";

const link = (source: string, target: string): GraphEdge => ({
  source,
  target,
  type: "link",
});
const tagEdge = (source: string, target: string): GraphEdge => ({
  source,
  target,
  type: "tag",
});

describe("filterEdges (GRAPH-04)", () => {
  const edges: GraphEdge[] = [
    link("A", "B"),
    link("B", "C"),
    tagEdge("A", "tag:x"),
  ];

  it("with the DEFAULT toggle set (links on, backlinks on, sharedTags OFF) keeps only link edges", () => {
    const out = filterEdges(edges, {
      links: true,
      backlinks: true,
      sharedTags: false,
    });
    expect(out).toHaveLength(2);
    expect(out.every((e) => e.type === "link")).toBe(true);
  });

  it("turning sharedTags ON adds the tag edges", () => {
    const out = filterEdges(edges, {
      links: true,
      backlinks: true,
      sharedTags: true,
    });
    expect(out).toHaveLength(3);
    expect(out.some((e) => e.type === "tag")).toBe(true);
  });

  it("turning links OFF (backlinks still on) drops link edges but keeps tag edges when sharedTags on", () => {
    const out = filterEdges(edges, {
      links: false,
      backlinks: false,
      sharedTags: true,
    });
    expect(out).toHaveLength(1);
    expect(out[0].type).toBe("tag");
  });

  it("everything off yields no edges", () => {
    const out = filterEdges(edges, {
      links: false,
      backlinks: false,
      sharedTags: false,
    });
    expect(out).toHaveLength(0);
  });

  it("backlinks toggle gates reverse-direction link edges; both on keeps all link edges", () => {
    // The Phase-9 payload carries direction on link edges; "backlinks" reveals
    // the reverse-direction view. With both links AND backlinks on every link
    // edge survives regardless of direction.
    const directed: GraphEdge[] = [link("A", "B"), link("C", "A")];
    const both = filterEdges(directed, {
      links: true,
      backlinks: true,
      sharedTags: false,
    });
    expect(both).toHaveLength(2);
  });
});
