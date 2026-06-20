---
slug: autosave-trailing-write
status: in-progress
created: 2026-06-21
source: Phase 1 UAT gap (.planning/phases/01-okf-pages-navigation-hidden-git/01-UAT.md → "autosave-drops-trailing-edit")
---

# Quick Task: Fix autosave lost-trailing-write

## Problem

In the page editor, an edit made shortly after a prior autosave is silently never
sent to the server, while the status shows "Saved". Found in Phase 1 UAT, confirmed
at the network layer (trailing edit never PUT; git never receives it; explicit
"Save page" recovers it). Not the WR-03 conflict — no 409 involved.

## Root cause (web/src/routes/PageEditor.tsx)

1. `doSave`: `if (saving.current) return;` drops any save scheduled while one is in
   flight and never retries it — so the trailing edit's save is discarded.
2. The seeding `useEffect([data])` re-runs on every `data` change, including the
   `invalidateQueries(["page", path])` refetch after each save, which can reset
   `body`/`bodyRef` to server content and clobber edits typed during the save.

## Change

1. After a save settles successfully, compare current `bodyRef`/`frontmatterRef`
   against the snapshot that was just sent (`lastSavedBody`/`lastSavedFrontmatter`).
   If they differ, immediately re-save (draft) — a coalescing single-flight saver
   that never drops a trailing edit. The in-flight guard (`saving.current`) still
   prevents two concurrent PUTs (WR-03 preserved).
2. Seed editor state only ONCE (a `seeded` ref) so post-save refetches never
   clobber in-flight edits.
3. "Saved"/"Draft saved" status is only set when the sent snapshot still matches
   the current content; otherwise the flush keeps it in "Saving…". So the status
   can no longer falsely read "Saved" with unsent content.

## Verification

- New regression test: a trailing edit typed during an in-flight save is flushed
  (savePage eventually called with the trailing content).
- Existing tests stay green, especially "does not start an overlapping save while
  one is in flight (WR-03)" and the 409 conflict banner.
- `npm run build` passes.

## Out of scope

- No backend changes.
