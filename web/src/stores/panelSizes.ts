// panelSizes store — the navrail (left) and assistant (right) column widths are
// a per-device UI preference, persisted to localStorage so a user's chosen
// layout survives a reload. The widths drive the --navrail-width /
// --agentpanel-width CSS vars (set inline on .appshell), which the columns
// already consume. Mirrors the editorMode/agentPanel zustand+persist pattern.
import { create } from "zustand";
import { persist } from "zustand/middleware";

// Clamp bounds keep a dragged column usable (never collapsed to nothing or wide
// enough to crowd out the editor). Defaults match the original fixed tokens.
export const NAV_MIN = 200;
export const NAV_MAX = 480;
export const AGENT_MIN = 300;
export const AGENT_MAX = 680;

const NAV_DEFAULT = 264;
const AGENT_DEFAULT = 360;

const clamp = (n: number, lo: number, hi: number) =>
  Math.min(hi, Math.max(lo, n));

interface PanelSizesState {
  navWidth: number;
  agentWidth: number;
  // nudge* apply a drag delta (px) and clamp — called per pointermove tick.
  nudgeNav: (dx: number) => void;
  nudgeAgent: (dx: number) => void;
  reset: () => void;
}

export const usePanelSizes = create<PanelSizesState>()(
  persist(
    (set) => ({
      navWidth: NAV_DEFAULT,
      agentWidth: AGENT_DEFAULT,
      nudgeNav: (dx) =>
        set((s) => ({ navWidth: clamp(s.navWidth + dx, NAV_MIN, NAV_MAX) })),
      nudgeAgent: (dx) =>
        set((s) => ({
          agentWidth: clamp(s.agentWidth + dx, AGENT_MIN, AGENT_MAX),
        })),
      reset: () => set({ navWidth: NAV_DEFAULT, agentWidth: AGENT_DEFAULT }),
    }),
    { name: "okf.panel.sizes" },
  ),
);
