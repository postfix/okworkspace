// navrail store — the left file-tree panel's open/collapsed state is a per-device
// UI preference (Obsidian-style hideable sidebar), persisted to localStorage so it
// survives a reload. Default OPEN. Mirrors the agentPanel store pattern; the
// topbar panel-toggle drives this single source of truth.
import { create } from "zustand";
import { persist } from "zustand/middleware";

interface NavrailState {
  open: boolean;
  setOpen: (open: boolean) => void;
  toggle: () => void;
}

export const useNavrail = create<NavrailState>()(
  persist(
    (set) => ({
      open: true,
      setOpen: (open) => set({ open }),
      toggle: () => set((s) => ({ open: !s.open })),
    }),
    { name: "okf.navrail.open" },
  ),
);
