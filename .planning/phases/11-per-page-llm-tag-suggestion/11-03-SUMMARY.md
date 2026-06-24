---
phase: 11-per-page-llm-tag-suggestion
plan: 03
subsystem: frontend
status: complete
tags: [react, react-query, trust-gate, tagging, csrf, optimistic-concurrency, accessibility, token-css, stored-xss-guard]
requires:
  - "POST /agent/suggest-tags → { page_path, suggestions:[{tag, existing}], base_revision } (11-02)"
  - "POST /agent/apply-tags (editor + CSRF) → 204; 409 on stale revision (11-02)"
  - "mutate() CSRF helper + err.status attach (web/src/api/client.ts)"
  - "DiffReviewDialog trust-gate contract (focus inversion, Esc/backdrop cancel, stale state)"
  - "Cached [\"me\"] session query for the editor gate (App.tsx)"
provides:
  - "suggestTags(pagePath) → SuggestTagsResult + applyTags({page_path, tags, base_revision}) api client fns (409 via err.status)"
  - "TagSuggestion / SuggestTagsResult exported types"
  - "TagSuggest component: editor-gated 'Suggest tags' trigger + per-tag approval surface (checkbox rows, 'new' badge + new-default-unchecked, Apply/Cancel, loading/empty/suggest-error/apply-error/stale states)"
  - "TagSuggest mounted in PageEditor .pageeditor-frontmatter (editor-gated)"
affects:
  - web/src/api/client.ts
  - web/src/components/TagSuggest.tsx
  - web/src/components/TagSuggest.css
  - web/src/components/TagSuggest.test.tsx
  - web/src/routes/PageEditor.tsx
tech-stack:
  added: []
  patterns:
    - "react-query useMutation for suggest (read) + apply (CSRF write), mirroring the agent mode mutation usage"
    - "trust-gate focus inversion cloned verbatim from DiffReviewDialog (Cancel DOM-first + focused; Apply accent + NEVER auto-focused; Esc/backdrop = Cancel; focus trap + restore-focus)"
    - "tag names as React text children only — NO dangerouslySetInnerHTML (locked stored-XSS guard, identical to BacklinksPanel)"
    - "token-only CSS importing the Dialog + DiffReviewDialog shells; no new token, no new dependency"
    - "editor gate reads the cached [\"me\"] role (convenience); the apply endpoint is the real editor+CSRF boundary server-side"
key-files:
  created:
    - web/src/components/TagSuggest.tsx
    - web/src/components/TagSuggest.css
    - web/src/components/TagSuggest.test.tsx
  modified:
    - web/src/api/client.ts
    - web/src/api/client.test.ts
    - web/src/routes/PageEditor.tsx
decisions:
  - "Loading state lives on the trigger button ('Suggesting tags…' + spinner) per the UI-SPEC; the approval surface only opens AFTER a suggest succeeds, so there is no separate in-surface loading spinner — the empty/error/stale states are the only in-surface non-review states. This matches the SPEC copy contract (the loading label is the trigger's in-flight label)."
  - "The editor gate reads the cached [\"me\"] query via useQuery({queryKey:[\"me\"]}); in tests the role is pre-seeded with qc.setQueryData([\"me\"], …) so canEdit is synchronous. App.tsx already seeds this query on every authed route, so the trigger never flashes for an editor."
  - "applyError vs stale are distinct local flags: a 409 sets stale (Apply removed, Re-run offered); any other apply rejection sets applyError (a quiet line, Apply stays present for a retry). This mirrors the applyPatch/DiffReviewDialog split between the stale gate and a transient failure."
metrics:
  duration: ~20m
  completed: 2026-06-24
  tasks: 2
  files: 6
---

# Phase 11 Plan 03: Per-Page Tag Suggestion Approval UI Summary

Built the frontend trust surface for per-page LLM tag suggestion (TAG-01 trigger + TAG-02 per-tag approval): two api client fns (`suggestTags` read / `applyTags` CSRF write, the latter surfacing the 409 stale via `err.status`), and a `TagSuggest` component — an editor-gated "Suggest tags" trigger mounted in the page editor's frontmatter block that opens a focused per-tag approval list. The approval surface inherits the agent trust-gate interaction language VERBATIM from `DiffReviewDialog`: Apply is the single accent action but is NEVER auto-focused, Cancel is the DOM-first focused safe default, Esc/backdrop cancel without writing, and a 409 stale revision removes the apply path and offers Re-run rather than clobbering. New (invented) tags carry a "new" word-badge AND default unchecked; only checked tags are applied. No new dependency, no new CSS token, no `dangerouslySetInnerHTML`.

## What Was Built

### Task 1 — suggestTags + applyTags api client fns (web/src/api/client.ts) — commit 6c37834
- `TagSuggestion { tag, existing }` + `SuggestTagsResult { suggestions, base_revision }` interfaces mirroring the Wave-2 suggest-tags response field names exactly.
- `suggestTags(pagePath)`: an awaited `mutate<SuggestTagsResult>("/api/v1/agent/suggest-tags", { page_path })` read — goes through the existing `mutate()`/CSRF helper like `proposePatch`/`rewrite`; a fail-closed status surfaces the server's generic message. It NEVER writes.
- `applyTags({ page_path, tags, base_revision })`: a `mutate<void>("/api/v1/agent/apply-tags", payload)` CSRF write whose 409 surfaces via `err.status === 409` (set by `mutate()`), the exact `applyPatch` contract — so the approval surface shows its stale state and never clobbers. Documented that the server re-validates+normalizes the list (the client list is never authoritative).
- `client.test.ts`: suggest happy-path (route + typed result), suggest fail-closed reject (server message), apply 204 with exactly the checked tags + base_revision in the POST body, and the 409 stale signal via `err.status`.

### Task 2 — TagSuggest component + PageEditor mount (web/src/components, web/src/routes) — commit e5e81e9
- `TagSuggest.tsx`:
  - **Trigger** — a `.btn .btn-ghost` with a lucide `Tags` icon (label `Suggest tags`), min-height `--hit-min-height`, editor-gated (renders only when the cached `["me"]` role is editor/admin, mirroring AppShell `canEdit`). While the suggest mutation is in flight it is disabled and shows `Suggesting tags…` with the `.spinner` (lucide `Loader2`). A suggest error before the surface opens shows the quiet `Couldn't suggest tags. Try again.` line.
  - **Approval surface (`TagSuggestList`)** — clones the `.dialog-backdrop`/`.dialog`/`.dialog-title`/`.dialog-footer` shell and reproduces the trust-gate contract verbatim: `cancelRef` is the DOM-first deliberate initial-focus target; `Apply tags` is the single accent `.btn-primary` (with the `Check` icon) and is NEVER auto-focused; Esc + backdrop-click invoke Cancel; a full focus trap + restore-focus-on-close effect copied from DiffReviewDialog.
  - **Rows** — each is a `<label>` wrapping `<input type="checkbox">` + the tag name (a React text child — NO `dangerouslySetInnerHTML`) + the `new` badge (clones `.role-badge`) for `existing:false` rows. New tags default UNCHECKED; existing tags default checked. A header strip with `Select all` / `Clear all` and a live `aria-live="polite"` `{n} selected` count. Apply sends ONLY the checked tags + the captured `base_revision`.
  - **States** — empty (`No tag suggestions for this page.`), apply-error (`Couldn't apply the tags. Try again.`, Apply stays for retry), and the 409 **stale** state (clones the DiffReviewDialog stale footer: `.diff-stale` warning banner with `AlertTriangle` + heading + body, the Apply path removed, `Close` / `Re-run` — Re-run re-fires `suggestTags`). All quiet muted hidden-Git-safe single lines.
  - **Wiring** — react-query `useMutation` for suggest + apply; on a successful apply it invalidates `["page", pagePath]` so the editor reloads the freshly written frontmatter rather than racing autosave; a 409 sets `stale`, any other apply error sets `applyError`.
- `TagSuggest.css`: token-only; `@import`s the Dialog + DiffReviewDialog shells so the modal/stale styles ship. New badge clones `.role-badge`; checked checkbox uses `accent-color: var(--color-accent)` (the only row accent); footer is space-between; `prefers-reduced-motion` stops the trigger spinner. No new token.
- `PageEditor.tsx`: mounts `<TagSuggest pagePath={path} />` in the `.pageeditor-frontmatter` block (after Title/Description). The byte-stable save/autosave path is untouched — TagSuggest writes through its own apply-tags mutation (the server owns the `okf.SetTags` write).
- `TagSuggest.test.tsx` (vitest, QueryClientProvider + `vi.mock("../api/client")`): reader editor-gate (no trigger), new-default-unchecked + "new" badge + live count, only-checked-applied (`applyTags` called with exactly the checked tags + base_revision), Apply-not-autofocused / Cancel-focused, Esc cancel (applyTags zero calls), 409 → stale (Apply removed, warning + Re-run/Close), Re-run re-runs suggest, empty state, in-flight loading label, suggest-error line, apply-error line (non-stale, Apply stays).

## Deviations from Plan

### Adjustments

**1. [Adaptation - real seam] The loading state lives on the trigger, not as a separate in-surface spinner row**
- The plan's `<behavior>` listed a "loading (suggestTags in flight)" surface state alongside empty/error/stale. In practice the approval surface only opens AFTER a suggest succeeds (`onSuccess: setOpen(true)`), so the in-flight state is naturally the trigger's `Suggesting tags…` + spinner label (exactly the UI-SPEC "Trigger loading label" copy). Re-run from the stale state also shows `Suggesting…` on the Re-run button while re-fetching. This matches the SPEC Copywriting Contract (the loading label is the trigger's in-flight label) and avoids a flash of an empty modal. The loading-state test asserts the trigger's `Suggesting tags…` label.

**2. [Adaptation - role source] Editor gate reads the cached `["me"]` query directly**
- Per the plan's `read_first` note, the role lives in the cached `["me"]` session query. TagSuggest reads it via `useQuery({ queryKey: ["me"], queryFn: me })`; `canEdit = role === "editor" || "admin"`. Tests pre-seed it with `qc.setQueryData(["me"], …)` so `canEdit` is synchronous (no flash). App.tsx already seeds this query on every authed route.

No new dependency added; no new CSS token; no `dangerouslySetInnerHTML`; the byte-stable save path is untouched.

## Known Stubs

None. The component is fully wired to the live Wave-2 endpoints (suggest read + apply CSRF write); there are no hardcoded/placeholder data paths.

## Self-Check: PASSED

- `web/src/components/TagSuggest.tsx` — FOUND
- `web/src/components/TagSuggest.css` — FOUND
- `web/src/components/TagSuggest.test.tsx` — FOUND
- `web/src/api/client.ts` (suggestTags + applyTags) — FOUND
- `web/src/routes/PageEditor.tsx` (`<TagSuggest`) — FOUND
- Commits 6c37834, e5e81e9 — FOUND in git log

Verification output (real, pasted):
- `cd web && npx tsc -b` → exit 0
- `cd web && npx vitest run` → **45 test files passed, 368 tests passed** (includes the new `client.test.ts` tag fns + `TagSuggest.test.tsx`, full suite green)
- `git diff --stat HEAD~2 -- web/package.json web/package-lock.json` → empty (NO new dependency)
- `git diff --stat web/src/styles/tokens.css` → empty (NO new token)
- `CGO_ENABLED=0 go build ./...` (repo root) → exit 0
