---
author: engineer
phase: 40
requirements: [RE-05]
status: Executed (awaiting independent qa verification)
---

# Phase 40: Editor-Host Abstraction — Execution Summary

**Verdict:** EXECUTE COMPLETE. The editor-surface coupling is abstracted behind a
stable `EditorSurfaceHandle`; `editorMode` is 3-way in the existing store; the CM6
path is byte-identical. `runSaver` and the save/lock/409/reconcile state are
untouched. All existing editor + PageEditor suites pass **unchanged** (no test
assertion was altered; the `vi.mock` seams were left as-is — no re-point was even
needed because PageEditor's suites never invoke insert-at-cursor).

## Files touched (ONLY these 4 — `web/src/lib/tiptap/*` NOT touched, Phase 39 owns it)

- `web/src/stores/editorMode.ts`
- `web/src/lib/cm/mode.ts`
- `web/src/components/LivePreviewEditor.tsx`
- `web/src/routes/PageEditor.tsx`

Diff: 4 files, +106 / −48.

## What was abstracted (the 3 tasks)

### Task 1 — 3-way editorMode + narrowed CM6 setMode (BLK-02 / MED-05)
- `editorMode.ts`: `EditorMode = "live" | "source" | "tiptap"` extended in the
  EXISTING store (persisted key `okf.editor.mode`, default `"live"`, **no new
  file**). `setMode` (store setter) accepts all three (the toolbar's source of
  truth). `toggle()` (Cmd/Ctrl-E) is **byte-identical** — its body was unchanged
  (`s.mode === "live" ? "source" : "live"`), so a `"tiptap"` current value already
  toggles back to `"live"` and never toggles INTO tiptap; only the doc comments were
  added to make that contract explicit.
- `mode.ts`: introduced `export type CmMode = "live" | "source"` and narrowed
  `setMode(view, mode: CmMode)` so `"tiptap"` can NEVER reach the CM6 compartment
  config (a TipTap surface isn't a CM6 view). `toggleKeymap` (Mod-e) now narrows the
  3-way store value at the call site — `next === "source" ? "source" : "live"` — so
  the keymap dispatches only `live⇄source`. The dispatch itself (effects-only
  compartment reconfigure) is byte-identical — the EDIT-02 guarantee `mode.test.ts`
  pins is untouched.

### Task 2 — EditorSurface interface + LivePreviewEditor imperative handle (MED-06)
- `LivePreviewEditor.tsx`: defined and exported
  `interface EditorSurfaceHandle { insertAtCursor(text: string): void }`.
- Converted `LivePreviewEditor` to
  `forwardRef<EditorSurfaceHandle, LivePreviewEditorProps>` and added a
  `useImperativeHandle` (deps `[]`) exposing `insertAtCursor(text)` that dispatches
  `v.dispatch(v.state.replaceSelection(text))` on its OWN `EditorView` (the existing
  `view` useRef). The default export stays default (`export default LivePreviewEditor`)
  so `vi.mock('../components/LivePreviewEditor')` still works. **The `cmView.view`
  stamp is KEPT** (`(v.dom as CmEditorEl).cmView = { view: v }`) — the
  `LivePreviewEditor.test.tsx` tests read it.
- `PageEditor.tsx`: added `const surfaceRef = useRef<EditorSurfaceHandle>(null)` and
  rewired `insertTemplateAtCursor` (formerly `:529-542`) to call
  `surfaceRef.current?.insertAtCursor(substituted)` — the `.cm-editor` /
  `querySelector` / `cmView.view` reach-in is **removed** (and its now-unused
  `import type { EditorView }` deleted; `grep 'querySelector(".cm-editor")'` = none).
  `insertLink` was left on its existing `setBody`-append path (the plan explicitly
  permits either — keeping it preserves today's exact behavior; zero regression).

### Task 3 — Mount seam + `"tiptap"` unreachable guard (D-02 / D-03)
- Added a minimal `renderEditorSurface()` seam in `PageEditor` (the recommended
  MINIMAL inline form — no `runSaver` extraction). It resolves the 3-way store `mode`
  to a CM mode (`const cmMode = mode === "source" ? "source" : "live"`), so
  `"tiptap"` (not user-selectable this phase) **falls back to CM6 "live"** — a
  defensive guard never reached in practice (no UI sets `"tiptap"`). It mounts
  `<LivePreviewEditor ref={surfaceRef} value onChange currentPath mode={cmMode}
  readOnly />`. The mount site now renders `{renderEditorSurface()}` inside the
  `.pageeditor-surface` div; the dead `editorSurfaceRef` on that div is removed.
- **No TipTap import** this phase (`grep '@tiptap' PageEditor.tsx` = none — the only
  mention was a comment, since reworded to "TipTap"). The 2-way toolbar toggle
  (`setMode(mode === "live" ? "source" : "live")`) is unchanged.

## runSaver untouched — confirmed
- The `runSaver` useCallback (`:246-376`) and the entire conflict/lock/reconcile
  state (`reconcileApplied`, `reconcileAppliedTags`, `reconcileAppliedDescription`,
  `scheduleAutosave`, the 409 dialog handlers) were NOT edited.
- `git diff HEAD` hunk headers land only at: imports (2 hunks), `surfaceRef` decl,
  `insertTemplateAtCursor` comment + reach-in replacement, the `renderEditorSurface`
  seam insert, and the mount replacement. NONE fall inside the save loop.
- The only `runSaver` token in the diff is my new doc comment on the seam.

## Zero behavior change — evidence
- **Target suites (acceptance) — all pass UNCHANGED:**
  `npx vitest run LivePreviewEditor.test.tsx mode.test.ts PageEditor.test.tsx
  PageEditor.conflict.test.tsx PageEditor.reconcile.adversarial.test.tsx`
  → **5 files, 47 tests passed** (identical to the pre-edit baseline of 47).
- **Full editor + PageEditor + shared suites (v1.6 lesson — shared-component change):**
  `editorMode.test.ts + PageView.test.tsx + PageEditor* + LivePreviewEditor.test.tsx
  + src/lib/cm/` → **19 files, 341 tests passed**, unchanged.
- **No test assertion was modified.** No `vi.mock` re-point was needed (MIN-10 permits
  one, but the PageEditor textarea mock never exercises insert-at-cursor, so it stays
  as-is). No test needed a real assertion change → no behavior change.
- `npx tsc --noEmit` → exit 0.
- `npm run build` → exit 0; `assertLazyChunks` (vite `generateBundle` guard) green —
  no forbidden module leaked, entry-chunk budget unchanged (zero imports added, no
  TipTap).
- `eslint` on the 4 files → the single diagnostic (`connId.current` accessed during
  render in `<PresenceIndicator>`) is **pre-existing on HEAD** (verified by stashing
  my edits and re-linting the committed file — identical diagnostic). My 4 files add
  **zero new lint diagnostics**.

## Acceptance-criteria grep results
- `grep -nE 'insertAtCursor|useImperativeHandle|forwardRef' LivePreviewEditor.tsx` → matches ✓
- `grep -n 'querySelector(".cm-editor")' PageEditor.tsx` → none ✓ (reach-in removed)
- `grep -n '"tiptap"' editorMode.ts` → matches ✓
- `ls web/src/lib/editorMode.ts` → nothing ✓ (BLK-02, no new file)
- `grep -n '@tiptap' PageEditor.tsx` → none ✓
- `grep -n 'cmView' LivePreviewEditor.tsx` → still matches ✓ (test-visible stamp kept)
- `git status --short` → exactly the 4 files; `src/lib/tiptap/` untouched ✓

*Independent qa writes VERIFICATION.md; this engineer did NOT mark the phase complete.*
