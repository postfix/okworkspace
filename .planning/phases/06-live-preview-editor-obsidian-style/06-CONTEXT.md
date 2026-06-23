# Phase 6: Live-Preview Editor (Obsidian-style) - Context

**Gathered:** 2026-06-21
**Status:** Ready for planning
**Mode:** Smart discuss (autonomous)

<domain>
## Phase Boundary

Replace the MVP `@uiw/react-md-editor` split-pane editor with a CodeMirror 6
live-preview surface that renders Markdown formatting inline as the user types,
with a Live/Source toggle. Storage and sync are OUT of scope and unchanged: the
okf raw-Markdown bytes stay the system of record, Git stays hidden behind the
backend, and live multi-user co-editing remains a Phase 5 concern (CRDT→Git, not
a store swap). The phase must preserve the byte-stable round-trip (raw Markdown
in/out, no lossy block model) and the stored-XSS guard (no raw HTML rendered from
page content).

</domain>

<decisions>
## Implementation Decisions

### Editor Engine & Library
- Build on raw `@codemirror/{state,view,commands}` + `@codemirror/lang-markdown`
  (Lezer markdown tree); NO React wrapper (`@uiw/react-codemirror` rejected) — we
  need full control over the live-preview decoration layer.
- Live preview is a custom `ViewPlugin` that walks the Lezer markdown syntax tree
  and applies `Decoration.mark` / `Decoration.replace` / widget decorations — the
  same approach Obsidian's Live Preview uses.
- The CM6 document IS the raw Markdown string. No block model, no reparse-to-bytes
  ever — the byte-stable golden-corpus round-trip holds by construction.
- New component `web/src/components/LivePreviewEditor.tsx` wraps `EditorView` via
  `useRef`/`useEffect`; CM6 extensions live in `web/src/lib/cm/`.

### Rendering Scope (v1)
- Inline-render: headings, bold/italic, lists, links, inline code, and code blocks
  (the success-criteria set).
- **Inline images** `![alt](src)` render as actual `<img>` widget decorations
  (user override — include in v1, not deferred).
- **GFM tables** render as a styled grid (user override — include in v1, not
  deferred). Must agree with the server's Goldmark GFM and read-mode remark-gfm.
- Hide syntax markers (`**`, `#`, link brackets/URLs, etc.) EXCEPT on the active
  line / selection — the true Obsidian Live Preview feel.
- Image widgets load via the existing attachments URL convention; the `src` MUST
  be sanitized/validated (reject `javascript:`/exec vectors; only allow the app's
  attachment/relative paths and safe schemes) — security constraint.

### Source/Raw Toggle & Mode Behavior
- Header toggle button with two modes, "Live" and "Source"; default to **Live**.
- Persist the last-used mode in `localStorage` (client UI preference; via the
  existing zustand store pattern).
- `Cmd/Ctrl-E` keyboard shortcut toggles the mode (Obsidian parity).
- Both modes share ONE `EditorState` document; the toggle swaps only a decoration
  `Compartment`. There is no serialization/reparse on toggle, so switching modes
  is byte-identical by construction (satisfies success criterion 2).

### Integration & Preserved Guarantees
- Keep PageEditor's save machinery UNTOUCHED — `runSaver`, the body/frontmatter
  refs, `baseRevision`, the in-flight `saving` guard, autosave debounce, and the
  409 ConflictBanner. The new editor exposes the SAME `value` / `onChange(string)`
  contract as the current `<MDEditor>`.
- Live preview injects NO raw HTML from page content — only controlled mark/widget
  decorations. Raw-HTML blocks in content stay styled text, never executed DOM.
  Preserves the Phase 1/2/3 stored-XSS guard (rehype-raw OFF equivalent).
- **Unify edit + read rendering (user override):** read mode (`PageView`) and edit
  mode share a single live-preview rendering surface (read mode = a read-only CM6
  live-preview), so reading and editing look identical (true Obsidian behavior).
  The unified renderer MUST preserve everything `MarkdownProse` provides today:
  internal `.md` link navigation within the SPA (D-06), GitHub-style heading anchor
  `id`s so search results deep-link to the right heading (SRCH-06 / T-03-15), and
  sanitization (no raw HTML). `MarkdownProse`'s fate (retire vs retain) is resolved
  at plan/research time, constrained by these requirements.
- Remove the `@uiw/react-md-editor` dependency once the CM6 editor lands (smaller
  bundle). `LinkPicker` stays wired into edit mode (relative `.md` link insert, D-05).

### Read-Mode Unification — RESOLVED (2026-06-21, post-research)
- Decision: **fully unify** read + edit onto one live-preview surface. Read mode is a
  read-only CodeMirror live-preview view (`EditorState.readOnly` / `EditorView.editable.of(false)`),
  pixel-identical to edit mode's Live view.
- The search→heading deep-link (SRCH-06 / T-03-15) MUST be preserved on the unified
  read surface via a heading-anchor mechanism: stamp each heading line with a DOM
  `id` (`Decoration.line({attributes:{id: slug}})`) whose slug is computed with the
  SAME github-slugger algorithm `okf.ScanHeadings` uses on the backend, plus a
  scroll-to-`#hash`-on-mount effect. A vitest MUST assert the rendered heading id
  equals `slug(text)` for the okf corpus headings.
- `MarkdownProse` is retired from the read path once the unified CM6 read surface
  ships (kept only if a concrete blocker emerges during execution). Internal `.md`
  link SPA navigation and the no-raw-HTML guard carry over to the unified surface.

### Claude's Discretion
- Exact CM6 extension composition, decoration CSS class names, and theme tokens.
- How the read-only Live view shares extensions/modules with the editable one (one
  component in a read-only config vs a thin shared `web/src/lib/cm/` module).
- How inline-image and table widgets coexist with active-line marker reveal.

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- `web/src/routes/PageEditor.tsx` — Edit mode. Holds the autosave/optimistic-
  concurrency machinery (`runSaver`, refs, `baseRevision`, 409 ConflictBanner,
  `DRAFT_DEBOUNCE_MS`) and the frontmatter form. Only the `<MDEditor>` surface is
  swapped; everything else stays.
- `web/src/routes/PageView.tsx` + `web/src/components/MarkdownProse.tsx` — Read
  mode. `MarkdownProse` uses react-markdown + remark-gfm + rehype-slug +
  rehype-sanitize (raw HTML OFF), a `headingIdSchema` that keeps github-slugger
  ids un-clobbered for deep-links, and an `a` component that intercepts internal
  `.md` links for SPA navigation. These behaviors are the bar the unified renderer
  must clear.
- `web/src/components/LinkPicker.tsx` — emits relative `.md` Markdown links into
  the body (D-05/D-06).
- `web/src/lib/mdlink.ts` (`resolveRelativeMdLink`) and `web/src/lib/frontmatter.ts`
  (`readField`/`setField`) — reused as-is.

### Established Patterns
- State: zustand for ephemeral UI state; TanStack Query for server state (`["page", path]`, `["tree"]`).
- Styling: per-component CSS files (`PageEditor.css`, `MarkdownProse.css`).
- Frontend builds into `internal/web/dist` and is embedded into the Go binary.
- `data-color-mode="light"` wraps the current editor surface today.

### Integration Points
- Route surfaces: `PageEditor` (edit) and `PageView` (read) under `/app/page/*`.
- Save path: `savePage(path, {body, frontmatter, base_revision})` → backend cuts a
  hidden Git version per write (Phase 1). The new editor must not change this.
- Deps to add: `@codemirror/state`, `@codemirror/view`, `@codemirror/commands`,
  `@codemirror/language`, `@codemirror/lang-markdown`, `@lezer/markdown` (as needed).
  Dep to remove after: `@uiw/react-md-editor`.

</code_context>

<specifics>
## Specific Ideas

- Reference UX is Obsidian "Live Preview" — the team are ex-Obsidian users; the web
  app stays the client. Inline formatting, active-line marker reveal, and a
  visually consistent read view are the felt-quality targets.

</specifics>

<deferred>
## Deferred Ideas

- Sibling Obsidian-feel items tracked separately (per ROADMAP notes): quick switcher
  (Ctrl-O), command palette (Ctrl-P), `[[wikilink]]` autocomplete, backlinks panel,
  denser file tree, dark theme.
- Live multi-user co-editing (CRDT→Git) stays Phase 5.
- DOCX/PDF in-browser editing remains out of scope (originals immutable).

</deferred>
