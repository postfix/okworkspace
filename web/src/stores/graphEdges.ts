// graphEdges store (GRAPH-04). The three edge-type toggles are per-device UI
// state, not a backend concern: the UI filters the payload `edges` array client-
// side (filterEdges + useMemo) and NEVER refetches on a toggle.
//
// Defaults: Links ON + Backlinks ON, Shared tags OFF. Shared-tag-OFF-by-default
// is a success criterion — it keeps the first view from being a hairball (the
// reason Phase 9 used the bipartite popular-tag cap). Persisted to localStorage
// so the last-used toggle state survives a reload.
//
// Mirrors the agentPanel.ts / editorMode.ts zustand+persist pattern EXACTLY; the
// persisted key is "okf.graph.edges".
import { create } from "zustand";
import { persist } from "zustand/middleware";

// EdgeKind is which toggle to flip.
export type EdgeKind = "links" | "backlinks" | "sharedTags";

interface GraphEdgesState {
  links: boolean;
  backlinks: boolean;
  sharedTags: boolean;
  // toggle flips one edge-type toggle (the chip cluster drives this).
  toggle: (kind: EdgeKind) => void;
}

export const useGraphEdges = create<GraphEdgesState>()(
  persist(
    (set) => ({
      links: true,
      backlinks: true,
      sharedTags: false,
      toggle: (kind) => set((s) => ({ [kind]: !s[kind] })),
    }),
    { name: "okf.graph.edges" },
  ),
);
