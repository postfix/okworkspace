// treeFold store — drives the tree's "collapse all / expand all" toggle. The
// toolbar button (in AppShell) flips `collapsed` and bumps `version`; every
// FolderRow watches `version` and snaps to !collapsed when it changes. A version
// counter (not just the boolean) lets a repeat toggle re-broadcast even folders a
// user re-opened manually in between. Not persisted — it's a transient action.
import { create } from "zustand";

interface TreeFoldState {
  // collapsed is the last broadcast target: true = "collapse all" was last run.
  collapsed: boolean;
  // version increments on every toggle so folder effects re-fire each press.
  version: number;
  toggle: () => void;
}

export const useTreeFold = create<TreeFoldState>((set) => ({
  collapsed: false,
  version: 0,
  toggle: () =>
    set((s) => ({ collapsed: !s.collapsed, version: s.version + 1 })),
}));
