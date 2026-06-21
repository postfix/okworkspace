---
phase: 06-live-preview-editor-obsidian-style
plan: 03
subsystem: ui
tags: [codemirror6, lezer, widgets, decorations, statefield, security, navigation, vitest]

# Dependency graph
requires:
  - phase: 06-live-preview-editor-obsidian-style
    plan: 01
    provides: sanitizeImageSrc allowlist gate, resolveRelativeMdLink (mdlink.ts), LivePreviewEditor surface + pathRef seam
  - phase: 06-live-preview-editor-obsidian-style
    plan: 02
    provides: live-preview ViewPlugin (livePreview.ts) tree walk + active-line reveal; the cm-md-link mark; livePreview.test.ts image/table RED todos
provides:
  - "ImageWidget + TableWidget WidgetType subclasses (web/src/lib/cm/widgets.ts): sanitized inline <img> with raw-text fallback; GFM table grid; all DOM via createElement+textContent (no innerHTML)"
  - "Image inline-widget branch in the livePreview ViewPlugin (Decoration.replace({widget})) + active-line reveal-to-source"
  - "Block GFM table widget via a StateField (tableField) — CM6 forbids block decorations from a ViewPlugin; livePreviewExtension bundles plugin + field"
  - "linkNav (web/src/lib/cm/linkNav.ts): EditorView.domEventHandlers click → resolveRelativeMdLink → react-router navigate; external/unsafe → default"
  - "cm-md-link marks now carry data-href so the handler can resolve the original href"
  - "EDIT-04 image-src + link-href scheme allowlists enforced on the widget render path"
affects: [06-04-unified-read-mode]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Block/multi-line replace widgets MUST come from a StateField (EditorView.decorations.from + EditorView.atomicRanges.of); inline single-line widgets MAY come from a ViewPlugin's decorations provider"
    - "WidgetType.toDOM builds DOM via document.createElement + textContent / explicit attributes ONLY — untrusted page content (src/alt/cell text) is never assigned to innerHTML (T-06-08)"
    - "ImageWidget calls the sanitizeImageSrc chokepoint before mounting <img>; a blocked src renders the RAW markdown text span (never a loading/executing element)"
    - "CM6 DOM-level click nav: rendered links are Decoration.mark spans carrying data-href; a domEventHandlers mousedown reads the href as DATA and routes internal .md through react-router (href is never an executed action)"
    - "A once-created EditorView reads live currentPath/navigate via () => ref.current getters so the handler stays correct across prop changes without recreating the view (StrictMode-safe)"
    - "atomicRanges applied to block/inline WIDGET ranges only (atomic deletion intentional); text-marker hides stay non-atomic (Pitfall 1)"

key-files:
  created:
    - web/src/lib/cm/widgets.ts
    - web/src/lib/cm/widgets.test.ts
    - web/src/lib/cm/linkNav.ts
    - web/src/lib/cm/linkNav.test.ts
  modified:
    - web/src/lib/cm/livePreview.ts
    - web/src/lib/cm/livePreview.test.ts
    - web/src/lib/cm/mode.ts
    - web/src/components/LivePreviewEditor.tsx
    - web/src/components/LivePreviewEditor.test.tsx
    - web/src/components/LivePreviewEditor.css

key-decisions:
  - "GFM table block widget moved to a StateField (tableField), NOT the ViewPlugin — CM6 throws 'Block decorations may not be specified via plugins'. RESEARCH Pattern 3's Decoration.replace({widget, block:true}) is correct, but only from a StateField. Inline image widgets stay on the plugin (single-line replace, allowed)."
  - "Link marks carry the href in a data-href attribute (read from the Lezer Link's child URL node). linkNav reads it as DATA; resolveRelativeMdLink's scheme allowlist is the only gate, so a javascript:/data: href can never reach navigate (T-06-07)."
  - "linkNav's currentPath accepts a () => string getter so the once-created EditorView resolves against the live linking page via pathRef without recreating the view."
  - "Blocked-src ImageWidget renders the RAW ![alt](src) text in a <span>, never a broken-image <img> — the bytes are never implied to have changed (CONTEXT data-openness)."

patterns-established:
  - "WidgetType subclasses are the content→DOM boundary; the sanitizer + textContent-only construction make them the single audited XSS chokepoint for inline-rendered page content"
  - "livePreviewExtension = [livePreview (plugin), tableField (StateField)] — the canonical seam mode.ts/liveExtensions composes"

requirements-completed: [EDIT-01, EDIT-04]

# Metrics
duration: 11min
completed: 2026-06-21
---

# Phase 6 Plan 03: Image & Table Widgets + Link Navigation Summary

**Completes EDIT-01's render set with sanitized inline image widgets and a GFM-table block grid, plus internal `.md` link click-navigation on the CodeMirror 6 surface — every widget built from createElement+textContent (no innerHTML) and every image src / link href passed through the 06-01 scheme allowlists (EDIT-04).**

## Performance

- **Duration:** ~11 min
- **Completed:** 2026-06-21
- **Tasks:** 2
- **Files modified:** 10 (4 created, 6 modified)

## Accomplishments
- **ImageWidget + TableWidget shipped (`web/src/lib/cm/widgets.ts`).** `ImageWidget.toDOM()` calls `sanitizeImageSrc(src)` first: a safe http(s)/app-relative src → a real `<img class="cm-md-image">` with `src`/`alt` set as explicit attributes; a blocked src (`javascript:`, executable `data:`, protocol-relative `//`) → a `<span class="cm-md-image-raw">` whose `textContent` is the RAW `![alt](src)` markdown — never a loading/executing element, never implying the bytes changed. `TableWidget.toDOM()` builds a `<table>/<thead>/<tbody>` grid from parsed cells, every cell set via `textContent`. No `innerHTML` of page content anywhere.
- **livePreview image branch + active-line reveal.** The ViewPlugin now replaces an `Image` node with an inline `Decoration.replace({ widget: ImageWidget })`, dropping the widget (revealing raw source) when the selection touches the image line.
- **GFM table as a block widget via a StateField.** CM6 forbids block/line-break-spanning replace decorations from a ViewPlugin, so the multi-line table grid is produced by a new `tableField` `StateField` (`Decoration.replace({ widget, block: true })`), revealed when the selection intersects any table line. `livePreviewExtension` bundles the plugin + field and is what `mode.ts`/`liveExtensions` now composes.
- **Internal `.md` link navigation (`web/src/lib/cm/linkNav.ts`).** Rendered links are `.cm-md-link` mark spans now carrying a `data-href` attribute; an `EditorView.domEventHandlers` `mousedown` reads that href, runs it through `resolveRelativeMdLink`, and on a non-null internal `.md` target `preventDefault()` + `navigate('/app/page/<target>')`. External / non-`.md` / unsafe-scheme hrefs fall through to default behavior. Wired into `LivePreviewEditor` via `useNavigate` + ref-reading getters so the once-created view stays correct across prop changes.
- **Security locked by tests.** `widgets.test.ts` asserts the `javascript:`/`data:`/protocol-relative cases yield the raw-text `<span>` (not `<img>`) and that alt/cell markup stays inert (structural asserts, never HTML strings — the T-06-08 proof). `linkNav.test.ts` asserts a `javascript:` href is never navigated. The two `it.todo` widget gates in `livePreview.test.ts` are flipped to real GREEN assertions.

## Task Commits

1. **Task 1: ImageWidget + TableWidget (sanitized, no-innerHTML) + livePreview image/table branches** — `1f46080` (feat)
2. **Task 2: linkNav DOM handler (internal .md → SPA navigate) wired into LivePreviewEditor** — `24aaaae` (feat)

## Files Created/Modified
- `web/src/lib/cm/widgets.ts` — **created.** `ImageWidget` (sanitize-then-`<img>`-or-raw-span, `eq` on src+alt) and `TableWidget` (header/body grid from `TableData`, `eq` on serialized cells); all DOM via `createElement`+`textContent`/explicit attrs.
- `web/src/lib/cm/widgets.test.ts` — **created.** 10 tests: safe http(s)/app-relative → `<img>`; `javascript:`/`data:`/`//` → raw `<span>`; inert alt/cell markup; `eq` semantics.
- `web/src/lib/cm/linkNav.ts` — **created.** The `domEventHandlers` mousedown→`resolveRelativeMdLink`→navigate handler; `currentPath` accepts a `() => string` getter.
- `web/src/lib/cm/linkNav.test.ts` — **created.** 4 tests: internal `.md` → navigate+preventDefault; external/non-`.md`/`javascript:` → no navigate.
- `web/src/lib/cm/livePreview.ts` — **modified.** Added `parseImage`/`parseTable`; the `Image` inline-widget branch; per-node link marks carrying `data-href`; a `tableField` `StateField` for the block table widget; `livePreviewExtension` export; widget-only `atomicRanges`.
- `web/src/lib/cm/livePreview.test.ts` — **modified.** Flatten helper now collects from the `EditorView.decorations` facet (plugin + field) and exposes widgets; the 2 image/table `it.todo` flipped to GREEN (emission + reveal + the `javascript:` raw-fallback case).
- `web/src/lib/cm/mode.ts` — **modified.** `liveExtensions` composes `livePreviewExtension` (was the bare `livePreview` plugin).
- `web/src/components/LivePreviewEditor.tsx` — **modified.** `useNavigate` + `navigateRef`/`pathRef`-reading `linkNav` extension in the EditorView.
- `web/src/components/LivePreviewEditor.test.tsx` — **modified.** Renders wrapped in `MemoryRouter` (the component now uses `useNavigate`).
- `web/src/components/LivePreviewEditor.css` — **modified.** Token-only `cm-md-image` / `cm-md-image-raw` / `cm-md-table` styling matching `MarkdownProse.css` (border-collapse, 1px `--color-border`, xs/sm cell padding).

## Decisions Made
- **Block table widget → StateField, not ViewPlugin.** This is the load-bearing structural decision. CM6 raises `RangeError: Block decorations may not be specified via plugins` when a `block:true` (or any line-break-spanning) replace decoration is provided from a ViewPlugin. The fix was to move ONLY the table widget into a `StateField` (`tableField`) which both provides its decorations (`EditorView.decorations.from`) and its atomicRanges; inline single-line image widgets remain on the plugin. `livePreviewExtension` is the new bundle so `mode.ts` composes both. See Deviations (Rule 3).
- **Href as data, never as action.** Link marks store the original href in `data-href`; `linkNav` reads it and the ONLY scheme gate is `resolveRelativeMdLink` (reused verbatim — it returns null for any `scheme:`/`//`/non-`.md`). A `javascript:`/`data:` href therefore can never reach `navigate`.
- **Getter-based currentPath for the once-created view.** `linkNav(() => pathRef.current, (to) => navigateRef.current(to))` keeps the resolve base + navigate live without recreating the EditorView (preserves StrictMode-safe single-view cleanup).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] CM6 forbids block decorations from a ViewPlugin — table widget moved to a StateField**
- **Found during:** Task 1 (the table-widget livePreview.test.ts assertion)
- **Issue:** PLAN/RESEARCH Pattern 3 specified the GFM table as `Decoration.replace({ widget: TableWidget, block: true })` emitted from the live-preview ViewPlugin. CM6 throws `RangeError: Block decorations may not be specified via plugins` (and a parallel error for replace decorations that span line breaks) at view-construction time — block/multi-line replace decorations are only permitted from a StateField.
- **Fix:** Added a `tableField` `StateField` that computes the block table decorations over the whole document (a field has no viewport) with the same active-line reveal, and provides both `EditorView.decorations.from(f)` and `EditorView.atomicRanges.of(...)`. The ViewPlugin's `Table` case now just skips the subtree. Exported `livePreviewExtension = [livePreview, tableField]`; `mode.ts` composes it. The inline image widget (single-line replace) stays on the plugin (allowed).
- **Files modified:** web/src/lib/cm/livePreview.ts, web/src/lib/cm/mode.ts, web/src/lib/cm/livePreview.test.ts
- **Verification:** livePreview.test.ts table emission + reveal GREEN; full `npm test` GREEN.
- **Committed in:** 1f46080 (Task 1 commit)

**2. [Rule 3 - Blocking] LivePreviewEditor.test.tsx needed a Router (component now uses useNavigate)**
- **Found during:** Task 2 (LivePreviewEditor.test.tsx GREEN gate)
- **Issue:** Wiring `linkNav` required `useNavigate`, which throws `useNavigate() may be used only in the context of a <Router>` when the component is rendered bare in the existing tests.
- **Fix:** Added a `withRouter` helper wrapping every `render`/`rerender` in `MemoryRouter`. The value/onChange/mode-toggle contract assertions are unchanged. (In the app, `LivePreviewEditor` already renders inside the SPA's router via PageEditor, so no app-side change was needed.)
- **Files modified:** web/src/components/LivePreviewEditor.test.tsx
- **Verification:** LivePreviewEditor.test.tsx 5/5 GREEN; full `npm test` GREEN.
- **Committed in:** 24aaaae (Task 2 commit)

---

**Total deviations:** 2 auto-fixed (both Rule 3 - blocking). Both are CM6/react-router API-conformance fixes within the planned task scope — the table widget renders exactly as specified (a block grid revealing to source), just sourced from the correct CM6 primitive. No scope creep, no architectural change, no behavior change versus the plan's intent.

## Threat Model Compliance
- **T-06-06 (Tampering — `javascript:` image src):** Mitigated. `ImageWidget.toDOM()` calls `sanitizeImageSrc` before any `<img>` is built; a blocked src renders the raw-text span. `widgets.test.ts` asserts the `javascript:`/`data:`/`//` cases produce a `<span>`, never an `<img>`. `livePreview.test.ts` asserts the in-plugin `![x](javascript:alert(1))` widget's `toDOM()` is the raw span.
- **T-06-07 (Tampering — `javascript:` link href):** Mitigated. `linkNav` reads the href from `data-href` (data, not an action) and only navigates when `resolveRelativeMdLink` returns non-null; that function rejects any `scheme:`/`//`. `linkNav.test.ts` asserts a `javascript:` href is never navigated.
- **T-06-08 (Tampering — widget DOM from untrusted text):** Mitigated. All widget DOM is `createElement` + `textContent`/explicit attributes; no `innerHTML` of page content. Tests assert on element structure (tagName/children/textContent), and explicitly that markup in alt/cell text stays inert (no parsed child elements).
- **T-06-09 (Information Disclosure — `data:` image):** Mitigated. `sanitizeImageSrc` rejects schemed `data:` (only http/https allowed when schemed); `widgets.test.ts` covers an executable `data:text/html` src → raw-text fallback.
- **T-06-SC (npm installs):** Accepted — no new packages this plan.

## Known Stubs
None. The two prior `it.todo` widget gates in `livePreview.test.ts` are now real GREEN assertions; no placeholder data or unwired components remain. EDIT-01's full v1 render set (headings, bold/italic, lists, links, inline code, code blocks, inline images, GFM tables) is live.

## Issues Encountered
- None beyond the two Rule 3 blocking deviations above.

## Next Phase Readiness
- **06-04 (unified read mode):** the themed decoration classes + the image/table widgets are the shared visual surface a read-only CM6 config can reuse via `liveExtensions`/`livePreviewExtension` + `theme`. The `slug`/`dedupSlug` anchor port (06-01) plus heading line decorations are still pending for the read-mode anchor parity (RESEARCH Pitfall 3). `@uiw/react-md-editor` can be removed once the read path stops importing it.

## Self-Check: PASSED

- `web/src/lib/cm/widgets.ts`, `web/src/lib/cm/widgets.test.ts`, `web/src/lib/cm/linkNav.ts`, `web/src/lib/cm/linkNav.test.ts` all exist on disk (created).
- Both task commits present in git history: `1f46080`, `24aaaae`.
- Full `cd web && npm test` GREEN (184 passed, 0 todo across 24 files); `npx tsc --noEmit` clean (exit 0); `eslint` clean on all touched .ts/.tsx files.

---
*Phase: 06-live-preview-editor-obsidian-style*
*Completed: 2026-06-21*
