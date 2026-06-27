// graphFilters store (GF-01). Carries all filter + display state for the graph
// view: search query, visibility toggles, display sliders, and graph groups.
// Persisted to localStorage so the last-used filter state survives a reload.
//
// Mirrors the graphEdges.ts zustand+persist pattern EXACTLY; the persisted key
// is "okf.graph.filters".
import { create } from "zustand";
import { persist } from "zustand/middleware";

export interface GraphGroup {
  id: string;
  name: string;
  query: string;
  color: string;
  enabled: boolean;
}

interface GraphFilterState {
  searchQuery: string;
  showTags: boolean;
  showAttachments: boolean;
  existingFilesOnly: boolean;
  showOrphans: boolean;
  showArrows: boolean;
  textFadeThreshold: number;
  nodeSize: number;
  linkThickness: number;
  groups: GraphGroup[];
  // Actions
  setSearchQuery: (q: string) => void;
  toggleTag: () => void;
  toggleAttachments: () => void;
  toggleExistingFilesOnly: () => void;
  toggleOrphans: () => void;
  toggleArrows: () => void;
  setTextFadeThreshold: (v: number) => void;
  setNodeSize: (v: number) => void;
  setLinkThickness: (v: number) => void;
  addGroup: () => void;
  reset: () => void;
}

const DEFAULTS = {
  searchQuery: "",
  showTags: true,
  showAttachments: false,
  existingFilesOnly: false,
  showOrphans: true,
  showArrows: false,
  textFadeThreshold: 0.5,
  nodeSize: 0.2,
  linkThickness: 0.2,
};

export const useGraphFilters = create<GraphFilterState>()(
  persist(
    (set) => ({
      ...DEFAULTS,
      groups: [],
      setSearchQuery: (searchQuery) => set({ searchQuery }),
      toggleTag: () => set((s) => ({ showTags: !s.showTags })),
      toggleAttachments: () => set((s) => ({ showAttachments: !s.showAttachments })),
      toggleExistingFilesOnly: () => set((s) => ({ existingFilesOnly: !s.existingFilesOnly })),
      toggleOrphans: () => set((s) => ({ showOrphans: !s.showOrphans })),
      toggleArrows: () => set((s) => ({ showArrows: !s.showArrows })),
      setTextFadeThreshold: (textFadeThreshold) => set({ textFadeThreshold }),
      setNodeSize: (nodeSize) => set({ nodeSize }),
      setLinkThickness: (linkThickness) => set({ linkThickness }),
      addGroup: () => set((s) => ({
        groups: [
          ...s.groups,
          {
            id: crypto.randomUUID(),
            name: "New group",
            query: "",
            color: "#8b5cf6",
            enabled: true,
          },
        ],
      })),
      reset: () => set({ ...DEFAULTS, groups: [] }),
    }),
    { name: "okf.graph.filters" },
  ),
);
