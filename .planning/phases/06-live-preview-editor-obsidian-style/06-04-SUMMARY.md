---
phase: 06-live-preview-editor-obsidian-style
plan: 04
subsystem: ui
tags: [codemirror6, lezer, decorations, statefield, read-mode, anchors, deep-link, security, vitest]

# Dependency graph
requires:
  - phase: 06-live-preview-editor-obsidian-style
    plan: 01
    provides: slug/dedupSlug/headingText github-slugger port (headingAnchors.ts); LivePreviewEditor surface + mode Compartment; cmCorpus fixtures
  - phase: 06-live-preview-editor-obsidian-style
    plan: 02
    provides: live-preview ViewPlugin (inline text-construct decorations + active-line reveal)
  - phase: 06-live-preview-editor-obsidian-style
    plan: 03
    provides: image/table widgets, linkNav internal .md SPA navigation, livePreviewExtension bundle
provides:
  - "headingAnchors() CM6 extension (StateField): stamps each ATX-heading line with Decoration.line({attributes:{id: slug}}), deduped (base, base-1, base-2) matching okf.ScanHeadings; ids rendered verbatim (never user-content-prefixed)"
  - "scrollToHash(view, hash?) helper: URL-decodes the #hash, looks up the matching heading-line id, scrolls it into view on mount (SRCH-06 deep-link preserved)"
  - "Read-only LivePreviewEditor mode (readOnly prop): non-editable Live surface (EditorState.readOnly + EditorView.editable.of(false)) + headingAnchors + scroll-to-#hash; selection/copy preserved; onChange never fires"
  - "PageView renders the read path through the unified read-only LivePreviewEditor (MarkdownProse retired from the read path)"
  - "@uiw/react-md-editor removed from web/package.json + lockfile (no remaining import)"
affects: []

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Heading-id line decorations come from a whole-document StateField (not a ViewPlugin) so every heading anchor exists regardless of viewport — required for scroll-to-#hash-on-mount to find off-screen headings"
    - "Read mode = the SAME live-preview surface in a read-only config (EditorState.readOnly + EditorView.editable.of(false), compartment forced to liveExtensions, no Source toggle) — pixel-identical to edit Live, single decoration pipeline (no second weaker read renderer)"
    - "location.hash is used ONLY as a lookup key against rendered heading ids (decodeURIComponent + CSS.escape on the selector), never written into the DOM or used to build markup"

key-files:
  created: []
  modified:
    - web/src/lib/cm/headingAnchors.ts
    - web/src/lib/cm/headingAnchors.test.ts
    - web/src/components/LivePreviewEditor.tsx
    - web/src/routes/PageView.tsx
    - web/src/routes/PageView.test.tsx
    - web/package.json
    - web/package-lock.json
    - CLAUDE.md

key-decisions:
  - "MarkdownProse.tsx is RETAINED on disk (not deleted) because HistoryPanel.tsx still imports it to render OLD page versions in the history-compare view. The plan scoped retirement to 'the read path' (PageView); HistoryPanel's historical-version preview is a separate, out-of-scope surface. MarkdownProse is fully removed from the READ path (PageView) as specified."
  - "headingAnchors is a StateField over the whole document (not viewport-bound) so scroll-to-#hash can target a heading below the fold on mount."
  - "Read mode forces liveExtensions in the mode Compartment and skips the mode-reconfigure effect, guaranteeing read and edit Live are byte-identical renders with no Source path."

patterns-established:
  - "A read-only CM6 surface is the canonical 'render Markdown for viewing' primitive — same decorations, widgets, link nav, and security gates as the editor, just non-editable"

requirements-completed: [EDIT-01, EDIT-04]

# Metrics
duration: 6min
completed: 2026-06-21
---

# Phase 6 Plan 04: Unified Read Mode + Heading Anchors Summary

**Unifies read mode onto the CodeMirror 6 live-preview surface (a read-only `LivePreviewEditor`, pixel-identical to edit Live) while preserving the search→heading deep-link via `headingAnchors` line decorations that stamp each heading with its github-slugger id (matching `okf.ScanHeadings` byte-for-byte) plus a scroll-to-`#hash`-on-mount effect — and removes the legacy `@uiw/react-md-editor` dependency.**

## Performance

- **Duration:** ~6 min
- **Completed:** 2026-06-21
- **Tasks:** 3 implementation tasks complete + automated coverage; Task 4 (perceptual human-verify) deferred to phase-level human verification.
- **Files modified:** 8 (0 created, 8 modified)

## Accomplishments
- **`headingAnchors()` extension (`web/src/lib/cm/headingAnchors.ts`).** A whole-document `StateField` walks the Lezer tree for `ATXHeading1..6` nodes and emits `Decoration.line({ attributes: { id: <slug> } })` on each heading's start line. The slug is computed by the existing 06-01 `slug()` port and deduped across the document with a shared `occurrences` map (`base`, `base-1`, `base-2`, …) exactly as `okf.ScanHeadings` does — so the rendered id equals the backend search anchor byte-for-byte. Ids are rendered **verbatim** (never `user-content-`-prefixed). A `scrollToHash(view, hash?)` helper URL-decodes the hash, `CSS.escape`s it for a safe selector, looks up the matching heading-line id, and scrolls it into view on mount.
- **Read-only `LivePreviewEditor` mode.** A new `readOnly?: boolean` prop turns the editor into the unified read surface: `EditorState.readOnly.of(true)` + `EditorView.editable.of(false)` (no caret/edits, selection/copy still work), the mode Compartment is forced to `liveExtensions` (read mode never toggles to Source — pixel-identical to edit Live), `headingAnchors` is added, and `scrollToHash(view)` runs once on mount. `onChange` never fires in read mode. Edit mode (readOnly falsy) is byte-for-byte unchanged from 06-03.
- **`PageView` swapped to the unified surface.** The read route renders `<LivePreviewEditor value={body} onChange={()=>{}} currentPath={path} mode="live" readOnly />` in place of `<MarkdownProse>`. Everything else is preserved verbatim: the recents `visit` effect, the `canEdit` Edit affordance + action menu, the empty-state copy ("This page is empty. Select Edit to start writing."), and the 404 / generic error states.
- **`@uiw/react-md-editor` removed.** No source file imported it (06-01 already swapped PageEditor); `npm uninstall @uiw/react-md-editor` removed it from `package.json` + the lockfile. `grep -rn "@uiw/react-md-editor" web/src web/package.json` returns nothing.
- **CLAUDE.md editor stack updated.** The editor "Pick" row now documents the hand-built CM6 live-preview surface (raw `@codemirror/*` + `@lezer/markdown`, no React wrapper); the Alternatives and What-NOT-to-Use editor rows note `@uiw/react-md-editor` was removed in Phase 6. The locked core-stack table is untouched.

## Task Commits

1. **Task 1: headingAnchors line decorations (id == okf slug) + scroll-to-#hash** — `dffdef4` (feat)
2. **Task 2: read-only LivePreviewEditor; swap PageView; remove @uiw/react-md-editor** — `6ed0d16` (feat)
3. **Task 3: update CLAUDE.md editor-stack row to the raw CM6 path** — `8c4ea9d` (docs)

## Files Created/Modified
- `web/src/lib/cm/headingAnchors.ts` — **modified.** Added the `headingAnchors` `StateField` extension (`buildHeadingAnchors` walks ATX headings → deduped `Decoration.line` ids) and the `scrollToHash(view, hash?)` helper. The 06-01 `slug`/`dedupSlug`/`headingText` functions are unchanged and reused.
- `web/src/lib/cm/headingAnchors.test.ts` — **modified.** Added a `headingAnchors()` rendered-id suite (mounts a headless read-only EditorView, reads `.cm-line[id]`): id === `slug(text)`, dedup `base`/`base-1`/`base-2` parity with `okf.ScanHeadings` over the whole okf corpus, never `user-content-`-prefixed; plus a `scrollToHash` suite (matches/decodes the hash, returns false for empty/unknown). 15 tests GREEN.
- `web/src/components/LivePreviewEditor.tsx` — **modified.** Added the `readOnly` prop + read-only extension set (readOnly/non-editable + `headingAnchors` + forced `liveExtensions`), the on-mount `scrollToHash` call, and a guard so the mode-reconfigure effect is skipped in read mode. Edit-mode behavior unchanged.
- `web/src/routes/PageView.tsx` — **modified.** Replaced `<MarkdownProse>` with the read-only `<LivePreviewEditor>`; dropped the `MarkdownProse` import; all other states preserved.
- `web/src/routes/PageView.test.tsx` — **modified.** Retargeted the heading/body assertions to the CM6 read surface (heading id on a `.cm-line[id]` element, bold via `.cm-strong`); the XSS-safe and recents tests pass unchanged.
- `web/package.json` / `web/package-lock.json` — **modified.** `@uiw/react-md-editor` removed.
- `CLAUDE.md` — **modified.** Editor-stack rows updated to the raw CM6 path; note md-editor removed in Phase 6.

## Decisions Made
- **MarkdownProse retained (not deleted) due to a concrete blocker.** `HistoryPanel.tsx` still imports `MarkdownProse` to render OLD page versions in the history-compare view (lines 11/126). The plan scoped retirement to "the read path" (PageView), which is fully done — PageView no longer imports or renders MarkdownProse. Deleting the file would break HistoryPanel, an out-of-scope surface. So `MarkdownProse.tsx`/`.css` stay on disk; they are simply no longer on the PageView read path. (Migrating HistoryPanel onto the unified read-only surface is a clean follow-up, not required by this plan.)
- **headingAnchors is a whole-document StateField, not a viewport ViewPlugin.** A ViewPlugin only decorates visible ranges; a `#hash` deep-link must scroll to a heading that may be below the fold on mount, so every heading id must exist before the first measure. A StateField over the whole document gives that (and is cheap at this app's page sizes; CM6 still only paints visible decorations).
- **Read mode forces Live + skips the toggle.** Read mode pins the compartment to `liveExtensions` and the mode-reconfigure effect early-returns when `readOnly`, so read is guaranteed byte-identical to edit Live with no Source affordance.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] PageView.test.tsx asserted against the retired MarkdownProse render shape**
- **Found during:** Task 2 (full `npm test` gate)
- **Issue:** Two PageView tests used `findByRole("heading", …)` and expected react-markdown's semantic `<h*>` output. The unified CM6 read surface renders headings as styled `.cm-line` elements carrying the slug `id` (no `role="heading"`), so the assertions failed.
- **Fix:** Retargeted the two tests to the CM6 read surface: assert the heading text + `id` on a `.cm-line[id]` element (id === `okf` slug, un-prefixed) and bold via `.cm-strong`. The SRCH-06 guarantee (rendered id equals the backend anchor) is still asserted — just against the new surface. The XSS-safe and recents tests passed unchanged.
- **Files modified:** web/src/routes/PageView.test.tsx
- **Verification:** PageView.test.tsx 4/4 GREEN; full `npm test` 191/191 GREEN.
- **Committed in:** 6ed0d16 (Task 2 commit)

---

**Total deviations:** 1 auto-fixed (Rule 3 - blocking test-conformance to the new read surface). No scope creep, no architectural change.

## Threat Model Compliance
- **T-06-10 (Tampering — read surface rendering untrusted content):** Mitigated. Read mode reuses the SAME `livePreviewExtension` pipeline (sanitized image widgets, no-innerHTML widgets, link-scheme gate via `linkNav`/`resolveRelativeMdLink`) as edit Live — there is no separate, weaker read renderer.
- **T-06-11 (Tampering — heading id from heading text):** Mitigated. The id is `slug(text)` (lowercase, keep letters/numbers/`-`/`_`, space→`-`, drop everything else) — cannot contain quotes/markup; matches `okf.ScanHeadings`. `headingAnchors.test.ts` asserts the exact rendered ids over the okf corpus, including dedup.
- **T-06-12 (Tampering — location.hash → scroll lookup):** Mitigated. `scrollToHash` uses the hash ONLY to look up a matching heading-line id (`decodeURIComponent` + `CSS.escape` for a safe selector) and scroll; it is never written into the DOM or used to build markup.
- **T-06-SC (npm uninstall @uiw/react-md-editor):** Accepted — removal only, no new package; lockfile regenerated.

## Known Stubs
None. `headingAnchors.test.ts` is fully GREEN (no `it.todo`); PageView renders real page bytes on the unified surface; `onChange={()=>{}}` in read mode is intentional (a read-only document never changes), not an unwired data source.

## Manual Visual-Verification Steps (deferred to phase-level human verification)

Task 4 of the plan is a `checkpoint:human-verify` covering perceptual checks that jsdom cannot assert. Per the orchestrator's checkpoint guidance these are NOT blocking this plan (all implementation + automated coverage is GREEN) — they are routed to the user once at the phase-level verification. To verify, `cd web && npm run build` then `npm run dev` (or run the built binary) and confirm:

1. **Live-preview, layout-neutral reveal (edit).** Open `/app/edit/<page>`; formatting renders inline as you type; moving the cursor through bold/heading/link lines reveals the raw markers on the active line with NO vertical jump of the lines below.
2. **Inline image + GFM table reveal (edit).** Edit a page with an image and a table — both render inline in Live; clicking onto their line shows the raw Markdown source.
3. **Live/Source toggle persistence.** Toggle via the header control and via Cmd/Ctrl-E; content is unchanged and a reload restores the last-used mode.
4. **Autosave / conflict untouched.** Type, wait for autosave (AutosaveStatus), and (forcing a stale edit) confirm the 409 ConflictBanner still behaves as before.
5. **Read = edit Live (unified surface).** Open `/app/page/<page>` — it looks pixel-identical to edit Live mode, and internal `.md` links navigate within the app.
6. **SRCH-06 heading deep-link.** From search, click a HEADING result (or visit `/app/page/<path>#<heading-slug>` directly) and confirm the read surface scrolls to that heading on mount.
7. **Script-scheme image src is inert.** A page with `![x](javascript:alert(1))` shows the raw text, not an executed/broken-image element.

**Automated coverage status:** GREEN. The slug/dedup/id correctness (6), the no-raw-HTML guard (7, partial — CM6 never parses HTML), the byte-stable toggle (3), and verbatim-bytes autosave contract (4) are all covered by vitest; only the *perceptual* aspects (reveal smoothness, pixel-identical look, smooth scroll) remain for the human pass.

## Verification Run
- `cd web && npm test` → **191 passed (24 files)**.
- `cd web && npx tsc --noEmit` → clean (exit 0).
- `cd web && npm run build` → succeeds (chunk-size advisory only, pre-existing).
- `npx eslint` on all touched `.ts`/`.tsx` → clean.
- `go test ./internal/okf/ -run TestGoldenRoundTrip` → `ok` (EDIT-03 backend gate).
- `grep -rn "@uiw/react-md-editor" web/src web/package.json` → nothing.

## Self-Check: PASSED

- All 8 declared modified files exist on disk with the changes (headingAnchors `StateField`/`scrollToHash`, LivePreviewEditor `readOnly`, PageView swap, package.json/lockfile md-editor removal, CLAUDE.md rows).
- All 3 task commits present in git history: `dffdef4`, `6ed0d16`, `8c4ea9d`.
- Full `cd web && npm test` GREEN (191 passed); `npx tsc --noEmit` clean; `eslint` clean; `npm run build` succeeds; `go test ./internal/okf/ -run TestGoldenRoundTrip` GREEN; no remaining `@uiw/react-md-editor` reference.

---
*Phase: 06-live-preview-editor-obsidian-style*
*Completed: 2026-06-21*
