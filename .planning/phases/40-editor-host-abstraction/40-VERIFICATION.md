---
author: qa
phase: 40
requirements: [RE-05]
verdict: PASS
status: Verified (independent qa)
---

# Phase 40: Editor-Host Abstraction — Independent QA Verification

**VERDICT: PASS** — behavior byte-identical. The refactor changes STRUCTURE only:
acceptance suites are byte-unchanged, `runSaver` is untouched, no test assertion was
weakened, and every RE-05 deliverable is real. Verified against code reality, not the
SUMMARY.

## Evidence

### 1. Acceptance suites pass (unchanged)
`cd web && npx vitest run LivePreviewEditor.test.tsx mode.test.ts PageEditor.test.tsx
PageEditor.conflict.test.tsx PageEditor.reconcile.adversarial.test.tsx`
→ **5 files, 47 tests passed.**
All five test files are **0 diff lines vs HEAD** (`git diff HEAD` per file) — no
`expect()` altered, no assertion relaxed. The only modified *.test.ts in the tree is
`web/src/lib/tiptap/codec.test.ts` — Phase 39's file, out of scope here.

### 2. runSaver untouched (the load-bearing check)
`git diff HEAD web/src/routes/PageEditor.tsx` hunk headers land ONLY at:
`@@ -2` (drop `import type EditorView`), `@@ -27` (import `EditorSurfaceHandle`),
`@@ -77` (`editorSurfaceRef`→`surfaceRef` decl), `@@ -516`/`@@ -534`
(insertTemplateAtCursor comment + reach-in replacement), `@@ -733` (renderEditorSurface
seam), `@@ -913` (mount replacement). The runSaver useCallback (**:246-376**) plus the
conflict / soft-lock / 409 / reconcile state sit entirely in the untouched
`@@ -89 … @@ -516` gap — **zero edits inside the save loop.** The only `runSaver` token
in the diff is a doc comment on the new seam.

### 3. Behavior preserved, tests not weakened
- `toggle()` body byte-identical (`diff` of HEAD vs working = identical).
- `mode.ts setMode` dispatch body identical; the only change is the param type rename
  `"live"|"source"` → `CmMode` (which *is* `"live"|"source"` — pure compile-time
  narrowing, runtime-identical). EDIT-02 byte-identity test (`mode.test.ts`
  `toBe(before)`) unchanged and green.
- `toggleKeymap` (Mod-e) narrows the 3-way store value at the call site
  (`next === "source" ? "source" : "live"`) — dispatches only live⇄source, byte-stable.
- The PageEditor suites mock LivePreviewEditor as a plain `<textarea>` (default export,
  no ref) and never exercise insert-at-cursor, so the `vi.mock` seam needed no re-point
  (MIN-10 allowance unused). `surfaceRef.current` is null in those suites; harmless.

### 4. RE-05 deliverables real
- `EditorSurfaceHandle { insertAtCursor(text) }` exported; `LivePreviewEditor` converted
  to `forwardRef<EditorSurfaceHandle, …>` + `useImperativeHandle(ref, …, [])` dispatching
  `v.state.replaceSelection(text)` on its OWN view — grep confirms
  forwardRef/useImperativeHandle/insertAtCursor present.
- `.cm-editor` querySelector reach-in **removed** — `grep 'querySelector(".cm-editor")'
  PageEditor.tsx` = NONE. `insertTemplateAtCursor` now calls
  `surfaceRef.current?.insertAtCursor(substituted)`.
- `EditorMode = "live" | "source" | "tiptap"` extended in the EXISTING store; **no
  new file** (`ls src/lib/editorMode.ts` = none), key `okf.editor.mode`, default `live`.
- `mode.ts` `setMode(view, mode: CmMode)` so `"tiptap"` can NEVER reach the CM6
  compartment config (compile-time guard).
- **No `@tiptap` import** in PageEditor (or any of the 4 files) — bundle unchanged.
- `cmView` stamp KEPT (LivePreviewEditor.tsx:230) — test-visible.

### 5. Cross-boundary (widened union)
Every `mode` consumer safely handles the new `"tiptap"` value: `renderEditorSurface`
(`cmMode = mode === "source" ? "source" : "live"`) and the toolbar toggle fall back to
CM6; `mode.ts setMode`/`toggleKeymap` are narrowed to `CmMode`. No exhaustive switch
silently drops or crashes on `"tiptap"`.

### 6. Build green
`npm run build` → exit **0** (`tsc -b && vite build`). The `assertLazyChunks` guard
(vite.config.ts:69, `maxEntryBytes: 1_420_000`) is active and did not abort — no
forbidden module leaked, no TipTap added, entry budget intact.

## GAPS
None.
