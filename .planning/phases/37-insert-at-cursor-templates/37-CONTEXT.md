---
author: architect
responsible: architect
phase: 37
phase_type: implementation
status: Ready for planning
hard_bar: false
security_relevant: false
design_fork: false
parent_artifacts:
  - .planning/ROADMAP.md
  - .planning/REQUIREMENTS.md
---

# Phase 37: Insert-at-Cursor Templates - Context

**Gathered:** 2026-07-02
**Status:** Ready for planning

<domain>
## Phase Boundary

Insert a `Templates/` snippet at the cursor inside the open `LivePreviewEditor` via a
normal CM6 transaction — distinct from Phase-27 new-page seeding (which creates a new
page from a template, not inserting into an existing one).

**Requirements:** EP-03

**Non-goals:** creating new pages from templates (Phase 27, already shipped); server-side
template expansion (substitution is client-side, same as Phase 27); the `externalSeed`
reset annotation (that is for prop-driven resets, not user edits).
</domain>

<decisions>
## Implementation Decisions

### Template listing (D-01) — reuse listTemplates
- **D-01:** The editor affordance lists templates via the existing `listTemplates`
  endpoint (`GET /api/v1/templates` → `listTemplates()` in `client.ts`). The user
  picks a template; the client fetches its body via `getPage(templatePath)` (the
  existing page-read endpoint); substitutes `{{title}}`/`{{date}}`/`{{time}}`
  client-side (same substitution as Phase 27's create-from-template); and inserts the
  substituted body at the cursor.

### Insertion mechanism (D-02) — normal CM6 transaction
- **D-02:** Insertion dispatches a **normal CM6 transaction** on the live `EditorView`:
  ```typescript
  const view = editorRef.current?.cmView?.view;
  if (view) {
    view.dispatch(view.state.replaceSelection(substitutedBody));
  }
  ```
  This is an ordinary user edit: the byte-stable `onChange` fires verbatim on
  `docChanged` (`LivePreviewEditor.tsx:24` — the updateListener ships `doc.toString()`
  on every doc change). The `externalSeed` reset annotation is NOT used (it's for
  prop-driven resets only, `:56`). The transaction replaces the current selection (or
  inserts at the cursor if no selection) — standard CM6 behavior.

### The affordance (D-03) — a button + picker in the editor
- **D-03:** A "Insert template" button in the editor toolbar (or near the
  `LivePreviewEditor`). Clicking opens a dropdown/dialog listing templates (from
  `listTemplates()`). Selecting one fetches the template body, substitutes placeholders,
  and inserts at the cursor. The picker is a simple list (reuse the existing Dialog
  component pattern from `TagManagement.tsx` or a lightweight dropdown).
- **D-03a (ref access):** The `LivePreviewEditor` exposes the `EditorView` via
  `CmEditorEl.cmView.view` (`:50`). The insert affordance needs a ref to the editor
  element. The planner wires this — either a forwardRef on `LivePreviewEditor` or a
  callback prop that hands the view to the parent (`PageEditor.tsx`).

### Placeholder substitution (D-04) — client-side, same as Phase 27
- **D-04:** Substitution replaces `{{title}}` with the current page title,
  `{{date}}` with today's date (YYYY-MM-DD), `{{time}}` with the current time
  (HH:MM). Same substitution as Phase 27's create-from-template (the client does it;
  the server re-parses but doesn't expand — `templates.go:99-98` "placeholders stored
  literally"). For insert-at-cursor, the substituted body is inserted directly (no
  server round-trip for the insert — it's a local CM6 transaction).

### The agent's Discretion
- The exact affordance shape (toolbar button + dropdown vs a dialog — recommend a
  lightweight dropdown/popover).
- How the editor view ref is accessed (forwardRef vs callback prop).
- Whether the template list is fetched on-demand or cached.
- Commit slicing.
</decisions>

<disciplines>
## Lenses surfaced for this phase

No disciplines surfaced — phase scope didn't call for any. The byte-stable
round-trip is a constraint (the insertion is a normal CM6 transaction, which
preserves it by construction), not a lens.

</disciplines>

<canonical_refs>
## Canonical References

### Phase spec & requirements
- `.planning/ROADMAP.md` §"Phase 37: Insert-at-Cursor Templates" — goal, success criteria.
- `.planning/REQUIREMENTS.md` §EP-03 (lines 220-232) — acceptance criteria.

### The editor surface
- `web/src/components/LivePreviewEditor.tsx` (the CM6 editor; the EditorView exposed
  via `CmEditorEl.cmView.view` at :50; the `onChange`/`docChanged` contract at :24;
  the `externalSeed` annotation at :56 — NOT used for insert).
- `web/src/routes/PageEditor.tsx` (the page editor host — the insert affordance lives
  here or in the LivePreviewEditor's toolbar).

### The template source
- `web/src/api/client.ts` (`listTemplates` — fetches the template list;
  `getPage` — fetches a template's body by path).
- `internal/server/handlers_pages.go:178-197` (`handleListTemplates` — the existing
  endpoint); `internal/pages/templates.go:42` (`ListTemplates` — the backend).

### No external specs
- No third-party specs/ADRs.
</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `listTemplates()` (`client.ts`) — the template list API.
- `getPage(path)` (`client.ts:295`) — fetch a template's body by path.
- `LivePreviewEditor` — the CM6 editor; exposes the `EditorView` for the transaction.
- The `Dialog` component pattern (`TagManagement.tsx`) — for the template picker.

### Established Patterns
- CM6 transaction dispatch (`view.dispatch(view.state.replaceSelection(text))`) —
  standard CM6, preserves the byte-stable round-trip (the doc IS the Markdown).
- Client-side placeholder substitution (Phase 27 pattern).
- `onChange` fires verbatim on `docChanged` — the insert is a normal edit.

### Landmines
- **Do NOT use `externalSeed`** — that annotation marks a prop-driven reset (the
  updateListener skips firing onChange for it). An insert is a user edit, not a
  reset — it MUST fire onChange so the edit is saved.
- **Insert at the cursor, not at the document start** — use
  `replaceSelection` (replaces the selection or inserts at cursor), not a full
  document replacement.
- **Template body is read via the safe-path resolver** — `getPage` routes through
  the server's SEC-01 resolver; no path escape.
</code_context>

<specifics>
## Specific Ideas

- The template picker can show the template name + a preview (first line of the body).
- The insert can be a single transaction (`view.dispatch(view.state.replaceSelection(substitutedBody))`)
  — one undo step, one `onChange` fire.
</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope.
</deferred>

---

*Phase: 37-insert-at-cursor-templates*
*Context gathered: 2026-07-02*