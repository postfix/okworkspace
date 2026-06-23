# Phase 6: Live-Preview Editor (Obsidian-style) - Pattern Map

**Mapped:** 2026-06-21
**Files analyzed:** 13 (10 new, 3 modified — plus `web/package.json`)
**Analogs found:** 13 / 13 (every file has at least a role-match analog; the CM6 `lib/cm/*` modules are pure-util-pattern matches, not behavioral analogs)

## Zustand Convention Resolution (canonical store dir)

The codebase has BOTH `web/src/stores/` and `web/src/store/`:

| Dir | File | Persist? | Verdict |
|-----|------|----------|---------|
| `web/src/stores/` | `recent.ts` + `recent.test.ts` | YES — `persist(...)` middleware, `{ name: "okf-recent-pages" }`, localStorage | **CANONICAL for a persisted UI-pref store** |
| `web/src/store/` | `searchStore.ts` (no test) | NO — plain `create(...)`, ephemeral open/closed only | one-off, non-persisted, singular-named dir |

**Decision for the new `editorMode` store:** the Live/Source mode preference MUST persist to `localStorage` (CONTEXT "persist last-used mode"). That is exactly the `stores/recent.ts` shape (`persist` + named key). **Place it at `web/src/stores/editorMode.ts`** (plural `stores/`, mirroring `recent.ts`), NOT in `store/`. Persistence key: `okf.editor.mode` (RESEARCH structure) — note `recent.ts` uses a hyphen key `okf-recent-pages`; either is fine, follow RESEARCH's `okf.editor.mode`. Co-locate a `stores/editorMode.test.ts` mirroring `recent.test.ts`.

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|-------------------|------|-----------|----------------|---------------|
| `web/src/components/LivePreviewEditor.tsx` | component (editor wrapper) | event-driven (value/onChange) | `web/src/components/MarkdownProse.tsx` (render + nav + sanitize bar) + `PageEditor.tsx` (value/onChange host) | role-match |
| `web/src/components/LivePreviewEditor.css` | config (per-component CSS) | — | `web/src/components/MarkdownProse.css` | exact |
| `web/src/lib/cm/markdown.ts` | utility (pure config module) | transform | `web/src/lib/mdlink.ts` (pure lib module convention) | role-match |
| `web/src/lib/cm/livePreview.ts` | utility (CM6 ViewPlugin) | transform | `web/src/lib/mdlink.ts` (pure module) — behavior is net-new | role-match |
| `web/src/lib/cm/widgets.ts` | utility (WidgetType subclasses + src guard) | transform | `web/src/lib/mdlink.ts` (pure module) | role-match |
| `web/src/lib/cm/theme.ts` | config (EditorView.theme over tokens) | — | `web/src/components/MarkdownProse.css` (token-var styling) | role-match |
| `web/src/lib/cm/linkNav.ts` | utility (click→navigate handler) | event-driven | `MarkdownProse.tsx` `a` component (resolveRelativeMdLink→navigate) | exact (same logic, CM6 surface) |
| `web/src/lib/cm/mode.ts` | utility (Compartment + keymap) | event-driven | `web/src/lib/mdlink.ts` (pure module) | role-match |
| `web/src/lib/cm/sanitizeSrc.ts` | utility (allowlist) | transform | `web/src/lib/mdlink.ts` (pure, unit-tested gate; reuse its scheme-regex idea) | exact (pure gate) |
| `web/src/lib/cm/headingAnchors.ts` | utility (slug + Decoration.line ids) | transform | `internal/okf/headings.go` `slug`/`dedupSlug` (algorithm to mirror) + `MarkdownProse` headingIdSchema | role-match (port Go→TS) |
| `web/src/stores/editorMode.ts` | store (zustand+persist) | pub-sub | `web/src/stores/recent.ts` | exact |
| `web/src/routes/PageEditor.tsx` (MOD) | route | event-driven | itself — swap only `<MDEditor>` | self |
| `web/src/routes/PageView.tsx` (MOD) | route | request-response | itself — swap `MarkdownProse` for read-only unified surface | self |
| `web/src/routes/PageEditor.test.tsx` (MOD) | test | — | itself + `web/src/stores/recent.test.ts` (vitest convention) | self |
| `web/package.json` (MOD) | config | — | — | n/a |

## Pattern Assignments

### `web/src/stores/editorMode.ts` (store, zustand+persist) — ANALOG: `web/src/stores/recent.ts`

Mirror `recent.ts` exactly: `create<...>()(persist((set) => ({...}), { name: ... }))`.

```typescript
// recent.ts lines 5-6, 25-38 — the persist pattern to copy
import { create } from "zustand";
import { persist } from "zustand/middleware";

export const useRecent = create<RecentState>()(
  persist(
    (set) => ({
      recents: [],
      visit: (page) => set((state) => { /* ... */ }),
      clear: () => set({ recents: [] }),
    }),
    { name: "okf-recent-pages" },
  ),
);
```

**Mirror to:** `useEditorMode` with `mode: "live" | "source"` (default `"live"`), `setMode`/`toggle`, persisted under `{ name: "okf.editor.mode" }`. Add `stores/editorMode.test.ts` (see test analog below).

---

### `web/src/components/LivePreviewEditor.tsx` (component) — ANALOG: `MarkdownProse.tsx` + `PageEditor.tsx`

**value/onChange contract to preserve** (from `PageEditor.tsx` line 248 — the surface being swapped):
```tsx
<MDEditor value={body} onChange={onBodyChange} height={480} />
```
The new component MUST expose the identical `value: string` / `onChange(value: string)` contract so `PageEditor`'s `onBodyChange` (lines 156-162) and save machinery are untouched.

**EditorView lifecycle + value-sync skeleton:** copy the RESEARCH §"React wrapper skeleton" (06-RESEARCH.md lines 443-494) verbatim as the starting point — it already encodes the StrictMode-safe `view.destroy()` cleanup (Pitfall 5) and the feedback-loop guard (`if (value !== cur) dispatch(...)`, Pitfall 6).

**Internal `.md` nav + sanitize bar to clear (the quality bar):** these come from `MarkdownProse.tsx` and MUST carry over (see linkNav + headingAnchors modules below):
- `resolveRelativeMdLink(currentPath, href)` → `navigate('/app/page/<target>')` (MarkdownProse.tsx lines 45-70).
- NO raw HTML — controlled decorations only (MarkdownProse.tsx lines 19-22 / 77-83 keep rehype-raw OFF; the CM6 equivalent is "build widget DOM with createElement+textContent, never innerHTML").

---

### `web/src/components/LivePreviewEditor.css` (config) — ANALOG: `MarkdownProse.css`

Copy the token-var-only convention. Every value is a `var(--…)`, never a literal.

```css
/* MarkdownProse.css lines 1-19 — the token convention to mirror */
.markdown-prose {
  max-width: var(--prose-max-width);
  color: var(--color-text);
  font-size: var(--font-size-body);
  line-height: var(--line-height-body);
}
.markdown-prose h1 {
  font-size: var(--font-size-display);
  font-weight: var(--font-weight-semibold);
  line-height: var(--line-height-display);
  margin: var(--space-xl) 0 var(--space-md);
}
```
**Mirror to:** `.cm-strong`, `.cm-em`, `.cm-md-image`, code/table classes, and the `--editor-min-height`/`--prose-max-width` surface. The CM6 `theme.ts` (`EditorView.theme`) and this CSS file MUST agree visually with `MarkdownProse.css` (UI-SPEC Typography table) so read and edit look identical.

---

### `web/src/lib/cm/linkNav.ts` (utility, event-driven) — ANALOG: `MarkdownProse.tsx` `a` component

Port the exact resolve→navigate logic to the CM6 DOM-event surface. Reuse `resolveRelativeMdLink` unchanged.

```tsx
// MarkdownProse.tsx lines 49-69 — the behavior to replicate at the editor DOM level
const target = resolveRelativeMdLink(currentPath, href);
if (target != null) {
  // intercept, preventDefault, navigate(`/app/page/${target}`)
}
// else: external/non-.md link rendered unchanged
```
CM6 form: `EditorView.domEventHandlers({ mousedown(e, view){...} })` (06-RESEARCH.md lines 336-346). Same null-means-external gate; reuse `web/src/lib/mdlink.ts` verbatim.

---

### `web/src/lib/cm/sanitizeSrc.ts` (utility) — ANALOG: `web/src/lib/mdlink.ts`

Mirror `mdlink.ts`'s pure, single-chokepoint, unit-tested gate style. Reuse its scheme-detection regex idea for the allowlist.

```typescript
// mdlink.ts lines 19-23 — the external/unsafe-scheme gate to reuse for img src
// External / protocol links: matches scheme: and protocol-relative //
if (/^[a-z][a-z0-9+.-]*:/i.test(href) || href.startsWith("//")) { /* ... */ }
```
**Mirror to:** `sanitizeImageSrc(src): string | null` — allow `http:`/`https:` + app-relative/attachment paths; return `null` for `javascript:`/`vbscript:`/exec `data:` (Security Domain, 06-RESEARCH.md lines 599-602). `null` → widget falls back to raw markdown text (06-RESEARCH.md lines 312-318). Pure → unit-test like `mdlink.test.ts`.

---

### `web/src/lib/cm/headingAnchors.ts` (utility) — ANALOG: `internal/okf/headings.go` `slug`/`dedupSlug` + `MarkdownProse` headingIdSchema

This is the load-bearing deep-link preservation (SRCH-06 / T-03-15). Port the Go slug algorithm to TS byte-for-byte; the rendered heading `id` MUST equal `okf.ScanHeadings`'s anchor (and rehype-slug's id) so search deep-links land.

```go
// headings.go lines 124-141 — slug() algorithm to port to TS EXACTLY:
// lowercase; keep Unicode letter/number/'-'/'_'; space→'-'; NO collapse, NO trim
func slug(text string) string { /* ... */ }
// headings.go lines 147-160 — dedupSlug: base, then base-1, base-2 on repeats
```

```typescript
// MarkdownProse.tsx lines 19-22 — WHY ids must be un-prefixed (must equal slug, never user-content-)
const headingIdSchema = {
  ...defaultSchema,
  clobber: (defaultSchema.clobber ?? []).filter((attr) => attr !== "id"),
};
```
**Apply via:** `Decoration.line({ attributes: { id: slug(headingText) } })` on heading lines (06-CONTEXT.md lines 84-86), plus scroll-to-`#hash`-on-mount. A vitest MUST assert rendered id === `slug(text)` for the okf corpus headings.

---

### `web/src/lib/cm/{markdown,livePreview,widgets,theme,mode}.ts` (utilities) — ANALOG: `web/src/lib/mdlink.ts` (module convention only)

These are net-new CM6 behavior with NO behavioral codebase analog (no prior CodeMirror integration). Mirror only the **module conventions** of `web/src/lib/`:
- Pure, focused, well-commented modules (see `mdlink.ts` header doc style, lines 1-11).
- Named exports; co-located `*.test.ts` (vitest) for the pure ones (`livePreview`, `mode`, `markdown` config).

Copy the concrete CM6 patterns from RESEARCH (authoritative for these):
- `markdown.ts` ← 06-RESEARCH.md lines 100-101, 115 (`markdown({ extensions: [Table, Strikethrough, TaskList, Autolink] })` GFM-only for server parity).
- `livePreview.ts` ← 06-RESEARCH.md Pattern 1 (lines 214-272) + node-name map (lines 423-441).
- `widgets.ts` ← 06-RESEARCH.md Pattern 3 (lines 301-326) — ImageWidget calls `sanitizeImageSrc`, builds DOM via `createElement`/explicit attrs only (never innerHTML).
- `theme.ts` ← `EditorView.theme(...)` reading `tokens.css` vars (06-RESEARCH.md line 63; UI-SPEC §Typography/Color).
- `mode.ts` ← 06-RESEARCH.md Pattern 2 (lines 274-299) — `Compartment` + `Mod-e` keymap; reads/flips the `useEditorMode` store.

---

### `web/src/routes/PageEditor.tsx` (MODIFY) — swap ONLY the editor surface

Replace ONLY line 248 (`<MDEditor … />`) with `<LivePreviewEditor value={body} onChange={onBodyChange} currentPath={path} mode={mode} />`. Add the Live/Source toggle into the existing toolbar (lines 243-245, next to `<LinkPicker>`), reading `useEditorMode`. **DO NOT TOUCH** lines 36-189: `runSaver`, `bodyRef`/`frontmatterRef`, `baseRevision`, the `saving` guard, `DRAFT_DEBOUNCE_MS`/`scheduleAutosave`, the 409 ConflictBanner (lines 197-209), `onBodyChange`/`insertLink`/`onFieldChange`. `LinkPicker` stays wired (line 244). Remove the `import MDEditor` (line 4) and the `data-color-mode` wrapper if no longer needed.

---

### `web/src/routes/PageView.tsx` (MODIFY) — unified read-only surface

Per 06-CONTEXT.md lines 77-89 (RESOLVED): **fully unify** — replace `<MarkdownProse body={body} currentPath={path} />` (line 104) with the read-only live-preview surface (`EditorState.readOnly` / `EditorView.editable.of(false)`), pixel-identical to edit Live mode. Keep everything else: recents `visit` (lines 44-50), `canEdit` gate, empty-state copy (line 102), error states (lines 55-72). `MarkdownProse.tsx`/`.css` are retired from the read path once this ships (keep only if a concrete blocker emerges). Preserve internal `.md` nav + heading anchor ids + no-raw-HTML on the new surface (see linkNav + headingAnchors).

---

### `web/src/routes/PageEditor.test.tsx` (MODIFY) — ANALOG: itself + `stores/recent.test.ts`

Retarget the editor mock (lines 21-35) from `@uiw/react-md-editor` to the new `LivePreviewEditor` (mock it to a plain `<textarea aria-label="body">` exposing value/onChange) while preserving ALL autosave/conflict/in-flight assertions (lines 66-198) unchanged. New CM6 tests (`stores/editorMode.test.ts`, `lib/cm/*.test.ts`) follow the `recent.test.ts` vitest convention:

```typescript
// recent.test.ts lines 1-9 — vitest + localStorage reset convention to mirror
import { describe, it, expect, beforeEach } from "vitest";
beforeEach(() => { localStorage.clear(); /* store.getState().clear() */ });
```
For byte-stability (EDIT-02/03) tests, prefer state-level `EditorState` construction (no DOM) per 06-RESEARCH.md line 546.

---

### `web/package.json` (MODIFY)

Add: `@codemirror/state@6.6.0`, `@codemirror/view@6.43.1`, `@codemirror/commands@6.10.3`, `@codemirror/language@6.12.3`, `@codemirror/lang-markdown@6.5.0`, `@lezer/markdown@1.6.4` (06-RESEARCH.md lines 96-101). Remove `@uiw/react-md-editor` after PageEditor no longer imports it.

## Shared Patterns

### Internal `.md` link navigation (D-06)
**Source:** `web/src/components/MarkdownProse.tsx` lines 45-70 + `web/src/lib/mdlink.ts` (reused verbatim).
**Apply to:** `lib/cm/linkNav.ts`, `LivePreviewEditor.tsx`, the unified read surface in `PageView.tsx`.
```tsx
const target = resolveRelativeMdLink(currentPath, href);
if (target != null) { e.preventDefault(); navigate(`/app/page/${target}`); }
// null → external/non-.md → render/behave unchanged
```

### No-raw-HTML / stored-XSS guard (project invariant)
**Source:** `MarkdownProse.tsx` lines 19-22, 77-83 (rehype-raw OFF; `id` un-clobbered but no new attrs).
**Apply to:** every CM6 widget (`widgets.ts`) — build DOM with `document.createElement` + `textContent`/explicit attributes, NEVER `innerHTML` of page content. Image/link `src`/`href` pass through `sanitizeSrc` allowlist first.

### Token-driven per-component CSS (no literals)
**Source:** `MarkdownProse.css` lines 1-19; `web/src/styles/tokens.css` `:root` vars.
**Apply to:** `LivePreviewEditor.css` and `lib/cm/theme.ts` — reference `var(--…)` only (UI-SPEC §Design System).

### GitHub-slugger heading anchors (SRCH-06 / T-03-15)
**Source:** `internal/okf/headings.go` `slug` (lines 124-141) + `dedupSlug` (lines 147-160); read-view bar `MarkdownProse.tsx` lines 19-22.
**Apply to:** `lib/cm/headingAnchors.ts` (port to TS) → `Decoration.line({attributes:{id}})` on the unified read surface. Rendered id MUST equal `slug(text)`, un-prefixed.

### Pure, unit-tested lib modules
**Source:** `web/src/lib/mdlink.ts` (+ `mdlink.test.ts`) — focused pure function, documented header, co-located vitest.
**Apply to:** `lib/cm/sanitizeSrc.ts`, `lib/cm/headingAnchors.ts`, `lib/cm/mode.ts`, `lib/cm/markdown.ts`.

### zustand + persist UI-preference store
**Source:** `web/src/stores/recent.ts` (+ `recent.test.ts`).
**Apply to:** `web/src/stores/editorMode.ts` (+ `editorMode.test.ts`). Persist key `okf.editor.mode`.

## No Behavioral Analog Found

These files have a *convention* analog but no prior *behavioral* analog (no existing CodeMirror integration). Planner should use 06-RESEARCH.md patterns as the authoritative source for their internals:

| File | Role | Data Flow | Reason | Use Instead |
|------|------|-----------|--------|-------------|
| `web/src/lib/cm/livePreview.ts` | utility (ViewPlugin) | transform | First CM6 ViewPlugin in the codebase | 06-RESEARCH.md Pattern 1 (lines 214-272) + node map (423-441) |
| `web/src/lib/cm/widgets.ts` | utility (WidgetType) | transform | First CM6 widgets | 06-RESEARCH.md Pattern 3 (lines 301-326) |
| `web/src/lib/cm/mode.ts` | utility (Compartment) | event-driven | First CM6 Compartment toggle | 06-RESEARCH.md Pattern 2 (lines 274-299) |
| `web/src/lib/cm/markdown.ts` | utility (config) | transform | First lezer-markdown config | 06-RESEARCH.md lines 100-101, 115 |
| `web/src/lib/cm/theme.ts` | config | — | First `EditorView.theme` | 06-RESEARCH.md line 63 + UI-SPEC tokens; visual parity with `MarkdownProse.css` |

## Metadata

**Analog search scope:** `web/src/{components,routes,stores,store,lib,styles}/`, `internal/okf/`
**Files scanned:** `recent.ts`, `recent.test.ts`, `searchStore.ts`, `MarkdownProse.tsx`, `MarkdownProse.css`, `PageEditor.tsx`, `PageEditor.test.tsx`, `PageView.tsx`, `LinkPicker.tsx`, `mdlink.ts`, `headings.go`, `tokens.css`
**Pattern extraction date:** 2026-06-21
