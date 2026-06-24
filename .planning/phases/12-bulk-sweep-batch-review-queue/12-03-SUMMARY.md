---
phase: 12-bulk-sweep-batch-review-queue
plan: 03
subsystem: frontend
status: complete
tags: [tagging, bulk-sweep, review-queue, admin-ui, lazy-route, reuse, trust-gate, stored-xss-guard, hidden-infra]
requires:
  - "POST /api/v1/admin/tags/sweep (admin, 202 {ok,queued:N}) — bulk-sweep start (12-01)"
  - "GET /api/v1/admin/tags/suggestions (admin) — pending review-queue read (12-01)"
  - "POST /api/v1/admin/tags/approve (admin) — batched approve, per-page [{page_path,status}] (12-02)"
  - "TagSuggestList approval surface + TagSuggestion {tag,existing} type (11-03)"
  - "Admin.tsx .admin-section rebuild-control pattern (reindexGraphMut) + Admin.css"
  - "App.tsx GraphView lazy-route + RequireAdmin pattern; AppShell.tsx navrow + isAdmin gate (Phase 10)"
  - "react-query mutate()/CSRF + err.status client conventions"
provides:
  - "startTagSweep / listTagSuggestions / approveTagSuggestions api client fns + TagSuggestionEntry / TagApproveResult types"
  - "Admin 'Tag suggestions' sweep-start section (untagged-default + 'include tagged pages' toggle, started/zero-target/busy/error states)"
  - "exported TagSuggestList (cancelLabel/applyLabel overridable) consumed by BOTH PageEditor's TagSuggest and TagReviewView"
  - "lazy /app/tag-review route + admin-gated 'Tag review' nav row"
  - "TagReviewView — backlog list + one-page-at-a-time review (reusing TagSuggestList) + batched Apply approved + aria-live progress + empty/loading/error/stale states"
affects:
  - web/src/api/client.ts
  - web/src/api/client.test.ts
  - web/src/routes/Admin.tsx
  - web/src/routes/Admin.test.tsx
  - web/src/routes/Admin.css
  - web/src/components/TagSuggest.tsx
  - web/src/components/TagReviewView.tsx
  - web/src/components/TagReviewView.css
  - web/src/components/TagReviewView.test.tsx
  - web/src/routes/AppShell.tsx
  - web/src/routes/AppShell.test.tsx
  - web/src/App.tsx
tech-stack:
  added: []
  patterns:
    - "reuse-don't-reimplement: TagSuggestList exported with overridable cancel/apply labels; PageEditor's per-page surface unchanged — both consume ONE approval surface (trust-gate focus inversion, new-default-unchecked, stale state inherited verbatim)"
    - "sweep-start mutation clones reindexGraphMut (async 202, muted role=status confirmation, generic field-error) with a scope-appropriate queued-count confirmation + queued===0 zero-target line"
    - "lazy admin route: React.lazy(() => import) + RequireAdmin + Suspense (Phase-10 GraphView pattern); admin-gated navrow with navrail-row-active + aria-current keyed on location.pathname"
    - "per-page 409 stale comes back as status='stale' in the approve result array (never a whole-call 409); the open page flips to the inherited stale state without invalidating/affecting the rest of the backlog"
    - "stored-XSS guard: page paths/tags render as React text children only — NO dangerouslySetInnerHTML (T-12-11)"
    - "token-only CSS reusing .navrow/.role-badge/.admin-empty/.backlinks-status recipes — no new token, no new dependency"
key-files:
  created:
    - web/src/components/TagReviewView.tsx
    - web/src/components/TagReviewView.css
    - web/src/components/TagReviewView.test.tsx
    - web/src/routes/Admin.test.tsx
  modified:
    - web/src/api/client.ts
    - web/src/api/client.test.ts
    - web/src/routes/Admin.tsx
    - web/src/routes/Admin.css
    - web/src/components/TagSuggest.tsx
    - web/src/routes/AppShell.tsx
    - web/src/routes/AppShell.test.tsx
    - web/src/App.tsx
decisions:
  - "TagSuggestList was exported (named export) with two optional label props (cancelLabel default 'Cancel', applyLabel default 'Apply tags') rather than extracting a separate headless component. The INTERACTION contract is unchanged by a label — cancel stays the DOM-first focused safe default, apply stays accent + never auto-focused. PageEditor's TagSuggest keeps the Phase-11 copy verbatim (zero behavior change, its test still green); TagReviewView passes 'Skip for now' / 'Apply approved'. The stale-state secondary button reads 'Close' when cancelLabel is the default and the override otherwise, so the review route shows 'Skip for now' consistently."
  - "The backlog row title uses the page_path as the label: the GET /admin/tags/suggestions payload (TagSuggestionEntry) carries page_path + suggestions + base_revision but NO page title field, so path IS the stable display label (rendered as a React text child). This matches the 12-01/12-02 handler contract; if a title field is added later the row title falls back cleanly."
  - "approveTagSuggestions returns per-page [{page_path,status}] and the whole call only rejects on a transport/server failure (per the 12-02 contract). TagReviewView inspects the result row for the open page: 'stale' → inherited stale state (no clobber, backlog untouched); 'applied'/'notfound' → invalidate the queue + close (the row is gone server-side, progress decrements). A thrown error (not a per-page status) shows the inherited apply-error line and keeps the surface open."
  - "Re-run from the stale state calls the existing per-page suggestTags (11-02) to get fresh suggestions, then invalidates the queue so the open entry carries the fresh base_revision. It never writes — the user must Apply again explicitly (the no-auto-write safety model)."
metrics:
  duration: ~35m
  completed: 2026-06-24
  tasks: 2
  files: 12
---

# Phase 12 Plan 03: Bulk Sweep + Batch Review Queue Frontend Summary

Built the Wave-3 frontend for the bulk tagging sweep + batch review queue
(TAG-05 sweep-start UI + TAG-06 review UI) per 12-UI-SPEC, composing the three
already-shipped patterns the spec locks: the Admin `.admin-section` rebuild
control, the Phase-10 lazy-route + admin-gated navrow, and the Phase-11
`TagSuggestList` approval surface — REUSED, not re-implemented. An admin starts a
sweep from the Admin Settings page (untagged-by-default with an "include pages
that already have tags" toggle, started / zero-target / busy / error states) and
reviews the resulting backlog on a lazy `/app/tag-review` route, one page at a
time through the reused approval surface, with a batched **Apply approved**, an
`aria-live` progress line, and the empty / loading / error / per-page stale
states. No new dependency, no new token, no `dangerouslySetInnerHTML`. This is
the FINAL plan of Phase 12 and of milestone v1.0.

## What Was Built

### Task 1 — api fns + admin sweep-start section — commit 6de8354
- `web/src/api/client.ts`: `startTagSweep({all})` (awaited `mutate` POST
  `/admin/tags/sweep` → `{queued}`), `listTagSuggestions()` (GET
  `/admin/tags/suggestions` → `TagSuggestionEntry[]`, generic hidden-infra-safe
  error), `approveTagSuggestions(approvals)` (`mutate` POST `/admin/tags/approve`
  → `TagApproveResult[]`). New types `TagSuggestionEntry {page_path, suggestions,
  base_revision}` + `TagApproveResult {page_path, status:"applied"|"stale"|"notfound"}`,
  reusing the Phase-11 `TagSuggestion {tag, existing}` shape.
- `web/src/routes/Admin.tsx`: a new `<section className="admin-section">` "Tag
  suggestions" cloning the `reindexGraphMut` pattern — `sweepAll` boolean (default
  false), `startSweepMut` with a scope-appropriate `role="status"` confirmation
  (`queued===0` → "Every page already has tags — nothing to suggest.", untagged vs
  all variants carry the queued count) and a generic `.field-error` error. The
  `.admin-sweep-toggle` checkbox (token-only, `accent-color: var(--color-accent)`)
  drives the scope; exact UI-SPEC copy with `&rsquo;`.
- `web/src/api/client.test.ts`: each fn hits the right route with the right body
  and returns the typed result (sweep `{all}`/`{queued}` both scopes; suggestions
  list shape + hidden-infra error; approve `{approvals}` + per-page status array).
- `web/src/routes/Admin.test.tsx` (new): the sweep section renders heading + toggle
  + button; untagged start → `startTagSweep({all:false})` + queued-count line;
  `queued===0` → "every page already has tags" (no alert); toggle on →
  `startTagSweep({all:true})` + all-scope copy; rejected mutation → the generic
  error line with zero infra vocabulary.

### Task 2 — lazy route + admin nav + TagReviewView — commit babb841
- `web/src/components/TagSuggest.tsx`: `TagSuggestList` exported as a named export
  with optional `cancelLabel` / `applyLabel` props (defaults preserve the Phase-11
  copy verbatim). PageEditor's `TagSuggest` consumes the same export unchanged.
- `web/src/components/TagReviewView.tsx` (default export, lazy-loadable): the
  `/app/tag-review` shell. `useQuery(["tag-suggestions"], listTagSuggestions)` for
  the backlog; `openPath` selects the page under review; the reused `TagSuggestList`
  renders that page's suggestions with `cancelLabel="Skip for now"` /
  `applyLabel="Apply approved"`. Apply → `approveTagSuggestions([{page_path,tags}])`:
  `"applied"`/`"notfound"` invalidate the queue + close (progress decrements),
  `"stale"` flips the open page to the inherited stale state without touching the
  rest of the backlog; a thrown error shows the inherited apply-error line. Re-run
  calls `suggestTags` for fresh suggestions. Backlog rows (path title + muted path +
  `{n} pending` `.role-badge` chip + `ChevronRight`) and an `aria-live` "{n} pages
  left to review" line. Page paths/tags are React text children only — NO
  `dangerouslySetInnerHTML`.
- `web/src/components/TagReviewView.css`: token-only chrome (header band, backlog
  rows cloning `.navrow`, `.role-badge` chip, `.admin-empty`-style empty state,
  muted `.backlinks-status`-style state lines). No new token.
- `web/src/App.tsx`: `const TagReviewView = lazy(...)` + a `/app/tag-review` Route
  wrapped in `RequireAdmin` → `AppShell` → `Suspense` (cloning the `/app/graph` +
  `/admin` patterns).
- `web/src/routes/AppShell.tsx`: an `{isAdmin && ...}` "Tag review" navrow beside
  the Graph row (`ListChecks` icon, `navrail-row-active` + `aria-current` keyed on
  `/app/tag-review`).
- `web/src/components/TagReviewView.test.tsx` (new): loading/empty/error copy; two
  backlog rows with title+path+pending chip + progress line; opening a row →
  reused surface with new-default-unchecked + Apply NOT auto-focused; Apply approved
  → exactly the checked tags + page leaves backlog + progress decrements; a "stale"
  result → inherited stale state (Re-run, Apply removed) with the other row
  untouched; Skip for now → page stays pending (approve not called).
- `web/src/routes/AppShell.test.tsx`: the "Tag review" navrow renders + routes (with
  `aria-current=page`) for an admin, and is absent for an editor.

## Deviations from Plan

### Adjustments

**1. [Adaptation - real GET helper] listTagSuggestions uses a raw fetch, not a `get()` helper**
The plan referenced a `get()` helper for the suggestions GET. `client.ts` has no
generic `get<T>()` — its GET fns use a raw `fetch` (e.g. `getBacklinks`/`getGraph`).
`listTagSuggestions` mirrors that raw-fetch pattern with a generic hidden-infra-safe
error, exactly as the plan's behavior intended.

**2. [Adaptation - label props, not a new headless component] TagSuggestList reused via export + label overrides**
The plan allowed extracting a reusable approval-list surface "WITHOUT changing its
interaction contract." Rather than a new component, `TagSuggestList` was exported
in place with two optional label props; the per-page `TagSuggest` keeps its defaults
(zero behavior change, its full test suite still green) and `TagReviewView` overrides
to "Skip for now" / "Apply approved". The trust-gate focus inversion, new-default-
unchecked, select-all/clear, count line, and stale state are inherited verbatim.

**3. [Adaptation - path-as-title] backlog row title is the page path**
The `TagSuggestionEntry` payload (12-01/12-02) carries no title field, so the row
uses `page_path` as the stable React-text-child title (the plan noted "falls back to
path if no title"). The muted secondary line is the same path.

**4. [Adaptation - admin error copy is generic, not the server message]**
`startSweepMut.onError` sets a fixed "Couldn't start the sweep. Try again." rather
than surfacing `err.message`, guaranteeing the hidden-infra invariant even on an
unexpected server failure (the plan's UI-SPEC copy).

No new dependency, no new token, no `dangerouslySetInnerHTML`; the admin gates are
client convenience (the server `RequireRole(admin)` on every endpoint is the real
boundary).

## Self-Check: PASSED

Files created (all FOUND):
- web/src/components/TagReviewView.tsx, TagReviewView.css, TagReviewView.test.tsx
- web/src/routes/Admin.test.tsx

Commits (all FOUND in git log): 6de8354, babb841

Verification output (real, pasted):
- `cd web && npx tsc -b` → exit 0
- `cd web && npx vitest run` → 47 files, 388 tests passed (incl. new client tag-sweep
  fns, Admin.test.tsx sweep section, TagReviewView.test.tsx, AppShell navrow tests)
- `git diff --stat web/package.json web/package-lock.json` → empty (NO new dependency)
- `git diff --stat web/src/styles/tokens.css` → empty (NO new token)
- `grep dangerouslySetInnerHTML web/src/components/TagReviewView.tsx` → only 2 COMMENT
  lines documenting its deliberate absence (no JSX prop usage)
- `CGO_ENABLED=0 go build ./...` at repo root → exit 0 (the embedded SPA builds)
