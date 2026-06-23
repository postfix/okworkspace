// agentPanel store (AGNT-01..AGNT-10 UI). The right-side AgentPanel's
// open/collapse is a per-device UI preference (not server state), persisted to
// localStorage so the last-used state survives a reload. Default is OPEN — the
// panel IS the chat surface (the prompt is docked at its bottom, Cursor-style), so
// it shows by default; "Hide assistant" collapses it to reclaim the editor width.
//
// Mirrors the editorMode.ts zustand+persist pattern EXACTLY; the persisted key is
// "okf.agent.panelOpen". The topbar toggle and the panel header collapse button
// both drive this single source of truth.
import { create } from "zustand";
import { persist } from "zustand/middleware";

interface AgentPanelState {
  open: boolean;
  // setOpen sets the open state explicitly (e.g. auto-open on first submit).
  setOpen: (open: boolean) => void;
  // toggle flips open⇄collapsed (the topbar / header toggle).
  toggle: () => void;
}

export const useAgentPanel = create<AgentPanelState>()(
  persist(
    (set) => ({
      open: true,
      setOpen: (open) => set({ open }),
      toggle: () => set((s) => ({ open: !s.open })),
    }),
    { name: "okf.agent.panelOpen" },
  ),
);
