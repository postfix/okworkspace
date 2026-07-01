// navSearch store — the navrail search field's query text. The field lives in the
// navrail's fixed header (which never scrolls) while the results render in the
// scrolling body below, so the query is lifted into a store both can read.
// Transient (not persisted). When the query is empty the navrail shows the file
// tree; when non-empty it shows inline full-text search results (NavSearchResults).
import { create } from "zustand";

interface NavSearchState {
  query: string;
  setQuery: (q: string) => void;
}

export const useNavSearch = create<NavSearchState>((set) => ({
  query: "",
  setQuery: (query) => set({ query }),
}));
