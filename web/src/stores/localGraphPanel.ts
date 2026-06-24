// localGraphPanel store (GRAPH-03..05 UI). The right-side per-page Local-graph
// dock's open/collapse + the depth-control value are per-device UI preferences
// (not server state), persisted to localStorage so the last-used state survives a
// reload. Mirrors the agentPanel.ts / graphEdges.ts zustand+persist pattern
// EXACTLY; the persisted key is "okf.graph.localPanel".
//
// Default is COLLAPSED (open: false) — unlike the AgentPanel (which IS the chat
// surface and shows by default), the local graph is an optional companion view, so
// it stays out of the way until the reader reveals it (CONTEXT: "collapsible/
// toggleable so it doesn't crowd the editor"). depth defaults to 1 hop (GRAPH-03)
// and setDepth clamps to the endpoint's integer range [1,3] (Phase-9 server clamp).
import { create } from "zustand";
import { persist } from "zustand/middleware";

// DEPTH_MIN/DEPTH_MAX bound the depth control to the local-graph endpoint clamp.
export const DEPTH_MIN = 1;
export const DEPTH_MAX = 3;

// clampDepth coerces an arbitrary number into the integer range [1,3]: floors to
// an integer (a fractional select value can never arise, but be defensive) then
// clamps. NaN falls back to the default 1.
export function clampDepth(d: number): number {
  if (!Number.isFinite(d)) return DEPTH_MIN;
  const n = Math.floor(d);
  if (n < DEPTH_MIN) return DEPTH_MIN;
  if (n > DEPTH_MAX) return DEPTH_MAX;
  return n;
}

interface LocalGraphPanelState {
  open: boolean;
  depth: number;
  // setOpen sets the open state explicitly.
  setOpen: (open: boolean) => void;
  // toggle flips open⇄collapsed (the header collapse button / the reopen tab).
  toggle: () => void;
  // setDepth sets the depth, clamped to [1,3] (the DepthControl drives this).
  setDepth: (depth: number) => void;
}

export const useLocalGraphPanel = create<LocalGraphPanelState>()(
  persist(
    (set) => ({
      open: false,
      depth: 1,
      setOpen: (open) => set({ open }),
      toggle: () => set((s) => ({ open: !s.open })),
      setDepth: (depth) => set({ depth: clampDepth(depth) }),
    }),
    { name: "okf.graph.localPanel" },
  ),
);
