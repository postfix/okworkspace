/**
 * GRAPH-03 — localGraphPanel slice: the persisted open/collapse + depth state for
 * the right-side per-page Local-graph dock. Asserts the defaults (collapsed,
 * depth 1), that setDepth clamps out-of-range values into [1,3], and that toggle
 * flips open. Mirrors the graphEdges.test.ts zustand-slice test shape.
 */
import { describe, it, expect, beforeEach } from "vitest";

import {
  useLocalGraphPanel,
  clampDepth,
  DEPTH_MIN,
  DEPTH_MAX,
} from "./localGraphPanel";

describe("localGraphPanel slice", () => {
  beforeEach(() => {
    localStorage.clear();
    // Reset to declared defaults between tests (the persist middleware would
    // otherwise carry mutations across cases).
    useLocalGraphPanel.setState({ open: false, depth: 1 });
  });

  it("defaults to collapsed (open=false) and depth 1", () => {
    const s = useLocalGraphPanel.getState();
    expect(s.open).toBe(false);
    expect(s.depth).toBe(1);
  });

  it("toggle() flips open", () => {
    expect(useLocalGraphPanel.getState().open).toBe(false);
    useLocalGraphPanel.getState().toggle();
    expect(useLocalGraphPanel.getState().open).toBe(true);
    useLocalGraphPanel.getState().toggle();
    expect(useLocalGraphPanel.getState().open).toBe(false);
  });

  it("setOpen sets open explicitly", () => {
    useLocalGraphPanel.getState().setOpen(true);
    expect(useLocalGraphPanel.getState().open).toBe(true);
    useLocalGraphPanel.getState().setOpen(false);
    expect(useLocalGraphPanel.getState().open).toBe(false);
  });

  it("setDepth accepts in-range values 1/2/3", () => {
    for (const d of [1, 2, 3]) {
      useLocalGraphPanel.getState().setDepth(d);
      expect(useLocalGraphPanel.getState().depth).toBe(d);
    }
  });

  it("setDepth clamps above-range values to 3", () => {
    useLocalGraphPanel.getState().setDepth(5);
    expect(useLocalGraphPanel.getState().depth).toBe(DEPTH_MAX);
  });

  it("setDepth clamps below-range values to 1", () => {
    useLocalGraphPanel.getState().setDepth(0);
    expect(useLocalGraphPanel.getState().depth).toBe(DEPTH_MIN);
    useLocalGraphPanel.getState().setDepth(-7);
    expect(useLocalGraphPanel.getState().depth).toBe(DEPTH_MIN);
  });

  it("persists under the okf.graph.localPanel key", () => {
    useLocalGraphPanel.getState().setOpen(true);
    useLocalGraphPanel.getState().setDepth(2);
    const raw = localStorage.getItem("okf.graph.localPanel");
    expect(raw).toBeTruthy();
    const parsed = JSON.parse(raw as string);
    expect(parsed.state.open).toBe(true);
    expect(parsed.state.depth).toBe(2);
  });
});

describe("clampDepth", () => {
  it("clamps into [1,3] and floors fractional input", () => {
    expect(clampDepth(0)).toBe(1);
    expect(clampDepth(1)).toBe(1);
    expect(clampDepth(2)).toBe(2);
    expect(clampDepth(3)).toBe(3);
    expect(clampDepth(4)).toBe(3);
    expect(clampDepth(2.9)).toBe(2);
    expect(clampDepth(Number.NaN)).toBe(1);
  });
});
