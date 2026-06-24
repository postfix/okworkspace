---
phase: 09-graph-endpoints-backlinks-panel
plan: 02
subsystem: frontend-read-view
tags: [backlinks, link-02, react-query, collapsible-panel, token-only, stored-xss-guard, pageview]

# Dependency graph
requires:
  - phase: 09-graph-endpoints-backlinks-panel
    provides: "GET /api/v1/graph/backlinks?path= returning [{path,title}] (plan 09-01)"
provides:
  - "web/src/api/client.ts — getBacklinks(path) GET fn + Backlink {path,title} type"
  - "web/src/hooks/useBacklinks.ts — useBacklinks(path) react-query hook (queryKey [\"backlinks\",path])"
  - "web/src/components/BacklinksPanel.tsx + .css — collapsible Referenced by (N) panel"
  - "<BacklinksPanel path={path}/> mounted at the bottom of PageView's success branch"
affects: [phase-10-graph-ui]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Backlinks panel reuses the existing nav-row contract (.navtree/.navrow/.navrow-page/.tree-icon/.tree-label from LeftTree.css) so entries are visually identical to the left tree / recent rows — BacklinksPanel.css adds only container/header/status styles"
    - "Disclosure toggle mirrors the LeftTree .tree-caret pattern (ChevronDown open / ChevronRight collapsed, size 16, aria-hidden) with aria-expanded on a full-width <button>; visible label is the accessible name"
    - "react-query read hook convention (queryKey [\"backlinks\",path], enabled path !== \"\", staleTime 30_000) mirrors useSearch minus the debounce — backlinks fetch once on page load, not per keystroke"
    - "Quiet muted single-line empty/loading/error states (.backlinks-status) per the UI-SPEC copy contract — no spinner, no retry button, never blocks the page body (additive panel)"
    - "Click-navigate asserted in tests via a LocationProbe route (useLocation pathname) inside MemoryRouter — no router-internal spying"

key-files:
  created:
    - "web/src/hooks/useBacklinks.ts"
    - "web/src/components/BacklinksPanel.tsx"
    - "web/src/components/BacklinksPanel.css"
    - "web/src/components/BacklinksPanel.test.tsx"
  modified:
    - "web/src/api/client.ts"
    - "web/src/routes/PageView.tsx"
    - "web/src/routes/PageView.test.tsx"

key-decisions:
  - "Default expanded (local useState(true)); user can collapse, toggle persistence not implemented this phase (UI-SPEC says not required)"
  - "Panel renders in PageView's success branch for BOTH empty-body and non-empty-body cases (a body-less page can still have backlinks), after the body ternary and before the Dialog block; never on the 404/error early-returns"
  - "getBacklinks throws generic 'Couldn't load backlinks.' copy on !res.ok (hidden-Git safe); the panel surfaces the UI-SPEC error line regardless of the thrown message"
  - "1px in border-top: 1px solid var(--color-border) is the repo-wide hairline-border convention (identical to RecentList.css) and is explicitly prescribed by the plan/UI-SPEC; the COLOR is tokenized — no hard-coded design value introduced"

requirements-completed: [LINK-02]

# Metrics
duration: 9min
completed: 2026-06-24
status: complete
---

# Phase 9 Plan 02: Referenced-by Backlinks Panel Summary

**A collapsible "Referenced by (N)" backlinks panel mounted at the bottom of the page read view (LINK-02): a `getBacklinks` GET fn + `Backlink` type, a `useBacklinks` react-query hook, and a token-only `BacklinksPanel` that reuses the existing nav-row contract for click-to-navigate entries and renders quiet muted empty/loading/error states per the UI-SPEC copy — additive (the CM6 read surface is untouched) and adds no new frontend dependency.**

## Performance
- **Duration:** ~9 min
- **Tasks:** 2
- **Files created:** 4; modified: 3

## Accomplishments
- `web/src/api/client.ts`: `interface Backlink { path: string; title: string }` + `getBacklinks(path)` GET fn mirroring `getTree` — `fetch(/api/v1/graph/backlinks?path=${encodeURIComponent(path)}, { credentials: "same-origin" })`, no CSRF (GET), generic error copy on `!res.ok`.
- `web/src/hooks/useBacklinks.ts`: `useBacklinks(path)` → `useQuery<Backlink[]>({ queryKey: ["backlinks", path], queryFn, enabled: path !== "", staleTime: 30_000 })`, mirroring the `useSearch` convention without the debounce.
- `web/src/components/BacklinksPanel.tsx`: `<section className="backlinks-panel">` with a full-width `aria-expanded` toggle (Chevron caret + `Referenced by ({count})`), and when open a `.navtree` of `navrow navrow-page` entries (`FileText` + `.tree-label`, click → `navigate(/app/page/${b.path})`). Titles render as React text children — NO `dangerouslySetInnerHTML` (stored-XSS guard T-09-06). Quiet muted `Loading backlinks…` / `Couldn't load backlinks. Refresh to try again.` / `No backlinks yet` states with the exact UI-SPEC copy.
- `web/src/components/BacklinksPanel.css`: token-only container/header/status rules (`--prose-max-width`, `--space-lg/md/sm`, `--color-border/-text-muted/-hover`, `--font-size-label`, `--font-weight-semibold`, `--hit-min-height`, `--radius-sm`); rows reuse the existing nav classes (not redefined).
- `web/src/routes/PageView.tsx`: `<BacklinksPanel path={path} />` mounted in the success branch immediately after the body-render ternary, before the `<Dialog>` block. The `LivePreviewEditor` read block and the 404/error early-returns are untouched (additive only).
- `web/src/components/BacklinksPanel.test.tsx`: vitest covering populated list + click-navigate (LocationProbe), empty `(0)` + "No backlinks yet", loading line, error line, and collapse (aria-expanded flips, list unmounts).
- `web/src/routes/PageView.test.tsx`: added `getBacklinks: vi.fn().mockResolvedValue([])` to the api mock so the existing PageView tests (which now mount BacklinksPanel) stay green.

## Task Commits
1. **Task 1: getBacklinks api fn + Backlink type + useBacklinks hook** — `34596f4` (feat)
2. **Task 2: collapsible Referenced by (N) panel + CSS + PageView mount + vitest** — `6d708be` (feat)

Plan metadata (STATE.md + ROADMAP.md + REQUIREMENTS.md) committed separately.

## Deviations from Plan
None of substance. Implementation followed the real codebase patterns exactly as the plan instructed:
- The api/hook/component/test signatures matched the plan's symbol assumptions (`getTree` GET shape, `useSearch` hook convention, RecentList row classes, LeftTree `.tree-caret`, PageView success-branch mount point, PageView.test mock shape).
- Click-navigate is asserted via a `LocationProbe` (`useLocation` pathname) route inside `MemoryRouter` — the plan said "assert via a route spy / a MemoryRouter location probe, matching how other component tests assert navigation"; the repo's existing tests assert navigation through router state, so the location-probe variant was chosen.

**Note on the token-only CSS check:** `BacklinksPanel.css` contains one `px` literal — `border-top: 1px solid var(--color-border)`. This is the repo-wide hairline-border convention (byte-identical to `RecentList.css`) and is the exact rule prescribed by the plan and UI-SPEC; the COLOR is tokenized. No hard-coded hex and no hard-coded design dimension was introduced — every spacing/size/color value is a `var(--…)` token.

## Threat Mitigations Applied
- **T-09-06 (Tampering/XSS):** backlink titles render as React text children (`<span className="tree-label">{b.title}</span>`) — never `dangerouslySetInnerHTML`. No raw-HTML path introduced.
- **T-09-07 (Info disclosure):** the fetch error path shows only the generic "Couldn't load backlinks. Refresh to try again." copy; no server internals surface.
- **T-09-08 (Tampering/injection):** `path` is URL-encoded via `encodeURIComponent` into the query string (server parameterizes the SQL per 09-01).
- **T-09-SC (npm install):** NO new frontend dependency — icons reuse the existing `lucide-react`; `git diff HEAD -- web/package.json web/package-lock.json` is empty.

## Self-Check Verification (actual command output)

```
### frontend type-check ###   cd web && npx tsc -b                         → tsc exit=0
### targeted tests ###        npx vitest run BacklinksPanel.test.tsx
                              PageView.test.tsx                            → 2 files, 9 tests passed
### full frontend suite ###   cd web && npx vitest run                     → 34 files, 295 tests passed (exit 0)
### token-only CSS ###        grep -E '#hex|[0-9]+px' BacklinksPanel.css   → only `1px solid var(--color-border)` (repo hairline convention; color tokenized)
### no new dependency ###     git diff HEAD -- web/package.json
                              web/package-lock.json                        → 0 lines (unchanged)
### backend build (CGO-free) ### CGO_ENABLED=0 go build ./...              → go build exit=0
```

## LINK-02 Satisfied
A user viewing a page sees a collapsible "Referenced by (N)" panel at the bottom of the read view listing every page that links to the current page; clicking an entry navigates to that linking page via the existing `/app/page/:path` route. Empty/loading/error states render the exact quiet UI-SPEC copy and never block the page body. The panel feels native (reuses tokens + nav-row classes), is additive (CM6 read surface untouched), and adds no new frontend dependency.

## Self-Check: PASSED

All 4 created files + 3 modified files exist; both task commits (`34596f4`, `6d708be`) are present in `git log`.

---
*Phase: 09-graph-endpoints-backlinks-panel*
*Completed: 2026-06-24*
