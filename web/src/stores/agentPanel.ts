// agentPanel store (AGNT-01..AGNT-10 UI). The right-side AgentPanel's
// open/collapse is a per-device UI preference (not server state), persisted to
// localStorage so the last-used state survives a reload. Default is collapsed
// (the panel opens on first submit / via the topbar toggle) — the editor keeps
// the full width until the user invokes the assistant.
//
// Mirrors the editorMode.ts zustand+persist pattern EXACTLY; the persisted key is
// "okf.agent.panelOpen". The topbar toggle, the panel header collapse button, and
// the PromptBar auto-open-on-first-submit all drive this single source of truth.
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
      open: false,
      setOpen: (open) => set({ open }),
      toggle: () => set((s) => ({ open: !s.open })),
    }),
    { name: "okf.agent.panelOpen" },
  ),
);
