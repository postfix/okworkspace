import { create } from "zustand";

// searchStore is the first zustand store in the codebase (locked stack:
// zustand 5.0.14). It holds ONLY the palette's open/closed state so the global
// ⌘K listener and the top-bar trigger can toggle it from anywhere. The active
// query text and keyboard-active row index live in the palette component's local
// state, not here — the store stays minimal.
interface SearchStore {
  open: boolean;
  setOpen: (open: boolean) => void;
}

export const useSearchStore = create<SearchStore>((set) => ({
  open: false,
  setOpen: (open) => set({ open }),
}));
