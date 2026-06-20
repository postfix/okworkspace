---
slug: autosave-trailing-write
status: complete
created: 2026-06-21
completed: 2026-06-21
---

# Summary: Fix autosave lost-trailing-write

## What changed

`web/src/routes/PageEditor.tsx` — replaced the autosave engine:

- **Single serialized coalescing saver** (`runSaver`) replaces the old draft(1s)+
  idle(6s) two-timer + recursive-flush scheme. One single-flight loop saves until
  the server content matches the live editor, reading the freshest content and the
  advanced `base_revision` on each iteration. A trailing edit typed while a save is
  in flight is always picked up by the next loop iteration (never dropped); a stale
  snapshot can never clobber a newer one (no overlapping PUTs).
- **Seed editor state only once** (`seeded` ref) so the post-save `invalidateQueries`
  refetch can't reset `body`/`bodyRef` and wipe in-flight edits.
- **Accurate status**: "Saved" is only set once the loop has caught the server up to
  the editor; otherwise it stays "Saving…". No more false "Saved" with unsent content.
- WR-03 preserved: the in-flight guard still means two PUTs never race on a base
  revision; the explicit "Save page" forces one save even when unchanged.

`web/src/routes/PageEditor.test.tsx` — added a regression test: a trailing edit typed
during an in-flight save is flushed (savePage eventually called with the trailing
content). Existing WR-03 in-flight and 409-banner tests still pass.

## Root cause

The two-timer scheme fired overlapping/late saves: a stale idle version-save (reading
an old snapshot, or racing the async commit queue with a stale `base_revision`) could
commit AFTER a newer save and overwrite it. Confirmed via git history during UAT:
`...SECOND-B` was committed, then a stale `...FIRST-A` save clobbered it 3s later.

## Verification

- New regression test + full frontend suite: 110/110 pass.
- `npm run build` (tsc + vite): clean.
- **Live headless-browser (Playwright) re-test of the exact UAT repro:** type A →
  autosave → type B → idle → both A and B persist in git HEAD (was: B lost). Status
  reads "Saved" matching the editor. Network: 2 PUTs, both 204, no 409. Stress burst
  (type during save, multiple bursts) → committed body exactly matches the editor.

## Commit

- `7985857` fix(editor): autosave never drops or clobbers a trailing edit
