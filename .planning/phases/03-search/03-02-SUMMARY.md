---
phase: 03-search
plan: 02
subsystem: web-ui
tags: [search, command-palette, quick-switcher, zustand, react-query, weight-only-highlight, xss-guard, keyboard-nav, focus-trap, obsidian-ux]

# Dependency graph
requires:
  - phase: 03-01 (backend search foundation)
    provides: GET /api/v1/search (authed) returning the typed Result DTO (kind/title/path/snippet/anchor?/page_title?) with weight-only <strong> highlight fragments + type facet; empty-query fast path
  - phase: 01/02 (web shell)
    provides: Dialog focus-trap/restore contract, .navrow + .role-badge + .dialog-backdrop + .empty-state + .spinner + .banner-warning primitives, tokens.css design system, AppShell topbar, api/client GET pattern, vitest+RTL harness
provides:
  - web/src/api/client.search(q) + SearchResult/SearchResultKind types (the SPA half of the first search vertical slice)
  - first zustand store in the codebase (searchStore: open/setOpen) for global ⌘K toggle
  - debounced react-query useSearch hook (key ["search", q]) + useDebouncedValue helper
  - Obsidian-style ⌘K SearchPalette overlay (focus-trap, ↑/↓/Enter/Esc/hover nav, 5 states, grouped typed results, in-app navigation)
  - typed SearchResultRow (icon + highlighted title + type badge + clamped snippet + "in {page}" sub-line)
  - highlight.ts XSS chokepoint — maps only weight-only markers to React <strong>, everything else escaped plain text (no dangerouslySetInnerHTML)
  - AppShell top-bar Search trigger + global ⌘K/Ctrl K keydown listener
affects: [03-03 (heading/attachment result kinds render with no UI change once the endpoint returns them), 03-04 (admin reindex UI lives in the same shell)]

# Tech tracking
tech-stack:
  added: []  # all libs already locked in CLAUDE.md (zustand 5.0.14, @tanstack/react-query, react-router-dom, lucide-react)
  patterns:
    - "First zustand store: minimal create<T>((set)=>({...})) holding ONLY open/closed; component-local state for query text + active row"
    - "Palette mounts a fresh PaletteInner via render gating (early null when closed) so transient state resets on each open — no reset-on-close effect (satisfies React 19 set-state-in-effect rule)"
    - "Search snippet highlight = weight-only: a small total mapper turns <strong>/<span class=search-hl> into React <strong>; all other text is React-escaped — the single XSS chokepoint"
    - "Debounced react-query: useDebouncedValue(raw,200) feeds queryKey [\"search\",q], enabled on non-empty q, placeholderData keep-prev to avoid layout jump"
    - "Palette replicates Dialog's focus-trap/restore + Tab-trap + Esc but renders bespoke input-on-top chrome (no Dialog component reuse, only its contract)"

key-files:
  created:
    - web/src/store/searchStore.ts
    - web/src/hooks/useSearch.ts
    - web/src/components/search/SearchPalette.tsx
    - web/src/components/search/SearchPalette.css
    - web/src/components/search/SearchResultRow.tsx
    - web/src/components/search/SearchResultRow.css
    - web/src/components/search/highlight.ts
    - web/src/components/search/SearchPalette.test.tsx
  modified:
    - web/src/api/client.ts
    - web/src/routes/AppShell.tsx
    - web/src/routes/AppShell.css

decisions:
  - "Split SearchPalette into a thin store-gated wrapper + a PaletteInner that only mounts while open. This resets query/active-row/loading on each open WITHOUT a reset-on-close effect, which the React 19 eslint rule (react-hooks/set-state-in-effect) forbids. Cleaner than a prev-open ref guard."
  - "activeIndex is clamped at read time (const active = min(activeIndex, len-1)) instead of via a clamp effect — same rule. Keyboard handlers compute the next index from the clamped value."
  - "Delayed (150ms) loading flag uses setState only inside the timer callback + cleanup, never synchronously in the effect body, to stay within the set-state-in-effect rule."
  - "Result-row type icon is returned as a JSX element from a helper (kindIcon) rather than as a Component reference rendered as <Icon/>, to satisfy react-hooks/static-components (no component created during render)."
  - "No new design tokens. Added one .keycap chrome class (top-bar ⌘K hint) and palette/result CSS, all from existing var(--…). The only non-token values are the three documented palette media dimensions (640px width, 15vh top offset, min(60vh,480px) list height), exactly as the UI-SPEC sanctions."

metrics:
  duration: ~25m
  completed: 2026-06-21
  tasks: 3
  files_changed: 11
  commits: 4
---

# Phase 3 Plan 02: ⌘K SearchPalette UI Summary

Delivered the user-facing half of the first search vertical slice: an Obsidian-style ⌘K quick-switcher wired to the `GET /api/v1/search` endpoint from 03-01. A signed-in teammate presses ⌘K (or clicks the top-bar Search trigger) anywhere, types, and sees live, debounced, typed PAGE results grouped under "Pages" with matched terms bold (weight-only, never a highlight fill and never raw server HTML); ↑/↓ move the active row, Enter/click opens it in-app, and Esc closes and restores focus to the trigger. All five UI states (empty, loading, results, no-results, error) render with the exact UI-SPEC copy. Heading/attachment rows are fully implemented and will render the moment 03-03 starts returning those kinds — no further UI change needed.

## What Was Built

- **Data layer (Task 1)** — `client.search(q)` (blank → `[]` with no network call; generic internals-free error copy) plus `SearchResult`/`SearchResultKind` types; the first zustand store (`searchStore`, just `open`/`setOpen`); and a debounced `useSearch` hook (`useDebouncedValue` + `useQuery` key `["search", q]`, gated on non-empty q, `placeholderData` keep-prev).
- **Palette + rows + highlight (Task 2)** — `SearchPalette` (Dialog focus-trap/restore + Tab-trap + Esc, bespoke input-on-top chrome, top-anchored 640px panel, the 5 states, grouped typed results, ↑/↓/Enter/hover nav, in-app navigation including heading-anchor deep-link scaffold for 03-03); `SearchResultRow` (role=option `.navrow` row: type icon + highlighted title + `.role-badge` + clamped snippet + "in {page}" sub-line, `.navrow-active` treatment); and `highlight.ts`, the XSS chokepoint that maps only weight-only markers to React `<strong>` and escapes everything else.
- **Wiring + test (Task 3)** — `AppShell` mounts `<SearchPalette/>`, adds a ghost top-bar Search trigger with a `⌘K` keycap (left of `UserMenu`), and registers a global `⌘K`/`Ctrl K` keydown listener (with `preventDefault`); plus an RTL/Vitest test covering grouped highlighted results with no injected raw HTML, the no-results and error copy, and ↑/↓ + Enter navigation.

## Tasks Completed

| Task | Name | Commit | Key files |
| ---- | ---- | ------ | --------- |
| 1 | search() client + types, zustand store, debounced useSearch hook | 61f6405 | api/client.ts, store/searchStore.ts, hooks/useSearch.ts |
| 2 | SearchPalette overlay + typed SearchResultRow + weight-only highlight | 3d92f02 | components/search/{SearchPalette,SearchResultRow}.{tsx,css}, highlight.ts |
| 3 | AppShell top-bar trigger + global ⌘K listener; palette test | 689a565 | routes/AppShell.{tsx,css}, components/search/SearchPalette.test.tsx |

## Tests

`web/src/components/search/SearchPalette.test.tsx` (5 tests, all green):
- grouped typed results render with weight-only `<strong>` highlight AND no injected raw `<img onerror>` (T-03-08 XSS guard)
- no-results state renders the UI-SPEC copy with the escaped query echo
- error state renders "Search is unavailable" / "Something went wrong while searching" without internals (T-03-09)
- ↑/↓ moves the active row (aria-selected) and Enter triggers in-app `navigate("/app/page/b.md")`
- palette opens via the store

Gates: `npm run build` green; `npx tsc --noEmit` clean; `npx eslint src/components/search/ src/store/ src/hooks/ src/routes/AppShell.tsx` clean; `npx vitest run` green (115/115 across the whole suite); `CGO_ENABLED=0 go build ./...` green; `CGO_ENABLED=0 go test ./...` green (unchanged — this plan is web-only).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] React 19 eslint rules (`set-state-in-effect`, `static-components`) failed the per-task verify**
- **Found during:** Task 2 (`npx eslint src/components/search/`).
- **Issue:** The project's eslint config enforces React 19's `react-hooks/set-state-in-effect` (synchronous `setState` in an effect body) and `react-hooks/static-components` (a component created during render). The plan's literal sketch — a reset-on-close effect, a clamp effect, a synchronous `setShowLoading(false)` branch, and a `const Icon = kindIcon(...)` rendered as `<Icon/>` — tripped all four.
- **Fix:** (a) Split `SearchPalette` into a store-gated wrapper + a `PaletteInner` that only mounts while open, so transient state resets on each open with NO reset effect; (b) clamp `activeIndex` at read time instead of via an effect; (c) move the 150ms loading `setState` into the timer callback + cleanup only; (d) make `kindIcon` return a JSX element instead of a component reference. All four are behavior-preserving and stay within the UI-SPEC contract.
- **Files modified:** web/src/components/search/SearchPalette.tsx, web/src/components/search/SearchResultRow.tsx
- **Commit:** 3d92f02

### Minor scope notes (within plan)

- The plan listed `aria-live="polite"` on the empty/no-results/error panels; the error panel additionally carries `role="alert"` (it reuses the `.banner-warning` alert pattern) — strictly additive for screen-reader announcement.
- The query echo in the no-results copy is rendered with typographic curly quotes (`&ldquo;`/`&rdquo;`); the test asserts the body text and the escaped query substring rather than straight ASCII quotes.

## Threat Model Coverage

| Threat ID | Disposition | How addressed |
|-----------|-------------|---------------|
| T-03-08 (highlight snippet XSS) | mitigated | `highlight.ts` maps ONLY `<strong>`/`<span class="search-hl">` to React `<strong>`; all other content (including an injected `<img onerror>`) is React-escaped plain text. No `dangerouslySetInnerHTML` anywhere in the search UI; rehype-raw stays OFF. Test asserts no `img[onerror]` node exists while the literal "onerror" text is present. |
| T-03-09 (error info disclosure) | mitigated | The error state shows only "Search is unavailable" / "Something went wrong while searching. Try again in a moment." `client.search` throws a fixed generic message on any non-ok response; server internals never reach the UI. Test asserts the copy. |
| T-03-10 (search() access) | accept (per plan) | GET /search is authed server-side (03-01); the SPA relies on the existing same-origin session cookie. No new client-side auth surface added. |

## Hidden-Git Compliance

No version-control vocabulary anywhere in the palette, result rows, copy, or error messages — no "index/commit/repo/Git/Bleve/HEAD". The user sees only search, results, pages, headings, and attachments.

## Known Stubs

None that block the plan goal. Page results are fully wired end-to-end (input → debounced hook → `search()` → typed rows → in-app navigation). Two intentional forward-deferrals to 03-03, both render-ready today:
- Heading/attachment result rows are fully implemented (icon, badge, "in {page}" sub-line, navigation to owning page); they simply have no data until 03-01's empty-but-declared heading/attachment mappings get populated in 03-03 — no SPA change required.
- The heading-anchor deep-link (`scrollIntoView` on `r.anchor` after navigation) is a no-op until 03-03 adds heading ids to the renderer; the page still opens correctly in the meantime.

## Self-Check: PASSED
- web/src/api/client.ts (search + types): FOUND
- web/src/store/searchStore.ts: FOUND
- web/src/hooks/useSearch.ts: FOUND
- web/src/components/search/{SearchPalette,SearchResultRow}.{tsx,css}: FOUND
- web/src/components/search/highlight.ts: FOUND
- web/src/components/search/SearchPalette.test.tsx: FOUND
- web/src/routes/AppShell.tsx + AppShell.css (modified): FOUND
- Commits 61f6405, 3d92f02, 689a565: FOUND in git log
