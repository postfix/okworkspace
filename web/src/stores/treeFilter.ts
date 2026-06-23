// treeFilter store — the file-panel filter query. Lifted out of LeftTree so the
// input can live in the navrail's fixed header (which doesn't scroll) while the
// tree it filters lives in the scrolling body below. Transient (not persisted).
import { create } from "zustand";

interface TreeFilterState {
  query: string;
  setQuery: (q: string) => void;
}

export const useTreeFilter = create<TreeFilterState>((set) => ({
  query: "",
  setQuery: (query) => set({ query }),
}));
