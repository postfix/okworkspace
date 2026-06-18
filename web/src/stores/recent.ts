// Recent-pages store (NAV-05). Client-side only — recents are an ephemeral,
// per-device convenience, not server state (D: recents stay client-side for a
// 5-user tool). Persisted to localStorage so they survive a reload; capped so
// the list stays short and most-recent-first.
import { create } from "zustand";
import { persist } from "zustand/middleware";

// RecentPage is one visited page: its route path (token) and human title.
export interface RecentPage {
  path: string;
  title: string;
}

// MAX_RECENT caps the list length (oldest entries drop off).
const MAX_RECENT = 8;

interface RecentState {
  recents: RecentPage[];
  // visit records that a page was opened, moving it to the front (de-duped by
  // path) and trimming to MAX_RECENT.
  visit: (page: RecentPage) => void;
  clear: () => void;
}

export const useRecent = create<RecentState>()(
  persist(
    (set) => ({
      recents: [],
      visit: (page) =>
        set((state) => {
          const withoutDup = state.recents.filter((r) => r.path !== page.path);
          return { recents: [page, ...withoutDup].slice(0, MAX_RECENT) };
        }),
      clear: () => set({ recents: [] }),
    }),
    { name: "okf-recent-pages" },
  ),
);
