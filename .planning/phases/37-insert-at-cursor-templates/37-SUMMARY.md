---
author: engineer
responsible: engineer
phase: 37
phase_type: implementation
status: complete
---

# Phase 37: Insert-at-Cursor Templates — Summary

## What was built

An "Insert template" affordance in the PageEditor toolbar that lists `Templates/*.md`
snippets and inserts the chosen template's substituted body at the cursor via a
normal CM6 transaction.

## Files changed

- **`web/src/components/TemplatePicker.tsx`** (new) — the picker component: a
  button + popover listing templates from `listTemplates()` (react-query,
  on-demand when open). Mirrors the LinkPicker pattern exactly.
- **`web/src/components/TemplatePicker.css`** (new) — picker styles, mirroring
  LinkPicker.css tokens/structure.
- **`web/src/routes/PageEditor.tsx`** (modified):
  - Imports: `FileText` icon, `EditorView` type, `listTemplates` + `TemplateInfo`
    from client, `substitutePlaceholders` from `lib/templates`, `TemplatePicker`.
  - State: `templatePickerOpen` (bool), `editorSurfaceRef` (ref on the
    `.pageeditor-surface` wrapper div).
  - `insertTemplateAtCursor(tpl)`: fetches body via `getPage(tpl.path)`,
    substitutes `{{title}}`/`{{date}}`/`{{time}}` via `substitutePlaceholders`,
    resolves the CM6 `EditorView` from `editorSurfaceRef → querySelector('.cm-editor')
    → cmView.view`, and dispatches `view.dispatch(view.state.replaceSelection(
    substituted))`. No `externalSeed` — the insert is a user edit that fires
    `onChange` verbatim on `docChanged`, so autosave picks it up exactly like a
    keystroke. No manual `setBody` (would race with the CM6-driven onChange).
  - Toolbar: `<TemplatePicker>` rendered beside `<LinkPicker>`, disabled when
    `readOnly` (held-by-other lock).
  - `ref={editorSurfaceRef}` on the `.pageeditor-surface` div.

## Key decisions

- **EditorView access via DOM ref, not a new prop.** The `LivePreviewEditor`
  stamps `cmView.view` on its `.cm-editor` host element (LivePreviewEditor.tsx:205).
  `insertTemplateAtCursor` resolves it via `editorSurfaceRef.current?
  .querySelector('.cm-editor')` → `cmView?.view`. This is zero-contract-change
  to the existing editor surface — no forwardRef, no callback prop, no new
  interface. The ref is on the wrapper div PageEditor already renders.
- **Reused `substitutePlaceholders` from `lib/templates.ts`.** The exact same
  helper the create-from-template flow (Phase 27 / CreatePageModal) uses. No
  duplicate substitution logic.
- **Normal CM6 transaction, not `externalSeed`.** `replaceSelection` is a
  standard user-edit transaction. The updateListener (LivePreviewEditor.tsx:184-
  196) fires `onChange(doc.toString())` on `docChanged` (the insert changes the
  doc), which calls `onBodyChange` → sets `bodyRef` + schedules autosave. The
  `externalSeed` annotation is for prop-driven resets only — an insert is a user
  edit that MUST fire onChange so it saves.
- **TemplatePicker mirrors LinkPicker.** Same button + popover pattern, same
  CSS token vocabulary, same on-demand react-query fetch (`enabled: open`).

## Verification

- `cd web && npx tsc --noEmit` → exit 0
- `cd web && npx vite build` → exit 0 (4237 modules transformed)
- `go build ./...` → fails on a pre-existing syntax error in
  `internal/server/handlers_tags_admin.go:761` (extra `}`), which is a sibling
  phase's in-progress backend work — Phase 37 is frontend-only, no backend
  changes expected or made.
- Acceptance greps:
  - `grep 'listTemplates' PageEditor.tsx` → match (import + comment)
  - `grep 'Insert template' PageEditor.tsx` → match (comment)
  - `grep 'replaceSelection' PageEditor.tsx` → match (comment + dispatch call)
  - `grep 'dispatch' PageEditor.tsx` → match (view.dispatch call)
  - `grep 'externalSeed' PageEditor.tsx` → 0 matches (not used for insert)
  - `grep 'substitutePlaceholders' PageEditor.tsx` → match (import + call)
  - `grep '{{title}}' PageEditor.tsx` → match (comment)

## Notes for QA

- The insert is a normal CM6 transaction — the byte-stable round-trip is
  preserved by construction (the doc IS the Markdown).
- The template body is read via `getPage` (the existing safe-path resolver
  endpoint) — no path escape.
- `replaceSelection` replaces the current selection or inserts at the cursor
  if no selection — inserting into a non-empty document does not corrupt
  surrounding text.
- The picker is disabled when `readOnly` (held-by-other soft lock), matching
  the Save button's disabled contract.