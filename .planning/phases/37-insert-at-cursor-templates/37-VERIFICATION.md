---
author: qa
responsible: qa
phase: 37
phase_type: implementation
status: complete
---

# Phase 37: Insert-at-Cursor Templates — Verification

## Verdict: PASSED

**gate_result:** `pass`

## Acceptance handshake

- **Inputs complete:** PLAN (37-01-PLAN.md) and SUMMARY (37-SUMMARY.md) both
  present. Requirement EP-03 (REQUIREMENTS.md:220-232) is real and verifiable.
- **PLAN ↔ SUMMARY consistency:** SUMMARY claims (a) a TemplatePicker component
  mirroring LinkPicker, (b) `insertTemplateAtCursor` in PageEditor dispatching a
  CM6 `replaceSelection` transaction, (c) `substitutePlaceholders` reused from
  `lib/templates.ts`, (d) `editorSurfaceRef` resolving the EditorView via
  `.cm-editor` → `cmView.view`. All four claims confirmed against committed code
  (see evidence below).
- **Goal verifiability:** "Insert a template snippet at the cursor via a normal
  CM6 transaction" is concretely verifiable — the dispatch call, the
  updateListener's onChange-on-docChanged path, and the absence of externalSeed
  are all structurally checkable.

## EP-03 criteria — evidence against code reality

### [✓] EP-03-1: Editor affordance lists templates via listTemplates; inserts substituted body at cursor

**TemplatePicker** (`web/src/components/TemplatePicker.tsx`):
- Lines 4, 30-34: fetches templates via `useQuery({ queryKey: ["templates"],
  queryFn: listTemplates, enabled: open })` — on-demand react-query fetch,
  mirrors LinkPicker pattern.
- Line 46: `select(tpl)` calls `onSelect(tpl)` → wired to
  `insertTemplateAtCursor` in PageEditor.

**PageEditor** (`web/src/routes/PageEditor.tsx`):
- Line 13: `import { … listTemplates … }` — the endpoint is wired.
- Lines 747-752: `<TemplatePicker open={templatePickerOpen}
  onOpenChange={setTemplatePickerOpen} disabled={readOnly}
  onSelect={(tpl) => void insertTemplateAtCursor(tpl)} />` — the affordance is
  rendered in the toolbar, disabled under the held-by-other soft lock.
- Lines 529-545: `insertTemplateAtCursor(tpl)`:
  - Line 532: `const page = await getPage(tpl.path);` — fetches body via the
    existing safe-path resolver endpoint.
  - Line 534: `substitutePlaceholders(page.body, { title, now: new Date() })` —
    client-side `{{title}}`/`{{date}}`/`{{time}}` substitution.
  - Line 542: `view.dispatch(view.state.replaceSelection(substituted))` — CM6
    transaction inserting at cursor.

### [✓] EP-03-2: Normal CM6 transaction; onChange fires on docChanged; externalSeed NOT used

- **Insert is a plain dispatch:** `view.dispatch(view.state.replaceSelection(
  substituted))` (PageEditor.tsx:542) carries NO `externalSeed` annotation —
  it is an unannotated user-edit transaction.
- **onChange path fires:** LivePreviewEditor.tsx:184-196 — the
  `EditorView.updateListener` fires `onChangeRef.current(u.state.doc.toString())`
  when `u.docChanged && !u.transactions.some((t) => t.annotation(externalSeed))`.
  The insert changes the doc (`docChanged` true) and carries no `externalSeed`,
  so onChange fires verbatim → autosave picks it up like a keystroke.
- **externalSeed absent from PageEditor:** `grep 'externalSeed'
  web/src/routes/PageEditor.tsx` → 0 matches. The reset annotation lives only in
  LivePreviewEditor.tsx:56 (its definition) and is used solely for prop-driven
  seeds — never for the insert path.

### [✓] EP-03-3: Safe-path resolver; no placeholder executes code; no surrounding-text corruption

- **Safe-path resolver:** Template body fetched via `getPage(tpl.path)`
  (PageEditor.tsx:532), which calls `/api/v1/pages/${encodePagePath(path)}`
  (client.ts:295-296) — the existing SEC-01 safe-path resolver endpoint. The
  `tpl.path` values come from `listTemplates` (server-validated
  `Templates/*.md` paths), so no path escape.
- **No placeholder executes code:** `substitutePlaceholders`
  (`web/src/lib/templates.ts:47-58`) is a pure `String.replaceAll` — `{{title}}`,
  `{{date}}`, `{{time}}` only. No eval, no templating engine. Any other `{{…}}`
  is left literal (templates.ts:4-5).
- **No surrounding-text corruption:** `replaceSelection` (CM6 standard) replaces
  the current selection or inserts at the cursor if no selection — it does not
  touch text outside the selection range. Comment at PageEditor.tsx:526-528
  documents this; it is the CM6 primitive's defined behavior.

## Build verification (run by qa)

| Command | Result |
|---|---|
| `cd web && npx tsc --noEmit` | exit 0 |
| `cd web && npx vite build` | exit 0 (4238 modules transformed) |

## Files verified

- `web/src/routes/PageEditor.tsx` — insert affordance, `insertTemplateAtCursor`,
  `editorSurfaceRef`, `TemplatePicker` wiring (lines 12-13, 23, 30, 78-85,
  517-552, 747-752, 916-920).
- `web/src/components/TemplatePicker.tsx` — picker component, `listTemplates`
  fetch (lines 4, 30-34, 46).
- `web/src/lib/templates.ts` — `substitutePlaceholders`, pure string replace
  (lines 47-58).
- `web/src/components/LivePreviewEditor.tsx` — updateListener / onChange path,
  `externalSeed` definition + skip logic (lines 50, 56, 184-196, 205).
- `web/src/api/client.ts` — `getPage` (safe-path), `listTemplates`,
  `TemplateInfo` (lines 294-324).

## Notes

- The SUMMARY's note about `go build ./...` failing on a pre-existing
  `internal/server/handlers_tags_admin.go:761` syntax error is a sibling phase's
  in-progress backend work — Phase 37 is frontend-only, no backend changes
  expected or made. This does not affect the EP-03 verdict.
- `substitutePlaceholders` is reused verbatim from the create-from-template flow
  (Phase 27 / CreatePageModal) — no duplicate substitution logic, consistent
  with the plan's "same substitution" directive.

## Conclusion

All three EP-03 acceptance criteria are satisfied against committed code
(commit 9ba01fc). The insert is a normal CM6 transaction that fires onChange
verbatim, externalSeed is not used, the template body is read via the safe-path
resolver, placeholders are pure string substitution (no code execution), and
`replaceSelection` preserves surrounding text. Both `tsc --noEmit` and
`vite build` exit 0.

**gate_result:** `pass`