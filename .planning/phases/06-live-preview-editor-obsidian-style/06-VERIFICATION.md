---
phase: 06-live-preview-editor-obsidian-style
verified: 2026-06-21T14:25:00Z
status: human_needed
score: 6/6 must-haves verified
overrides_applied: 0
human_verification:
  - test: "Open a page in edit mode (/app/edit/<page>). Type Markdown — headings, bold, italic, links, inline code, a fenced code block. Confirm each renders inline as you type without flicker."
    expected: "Headings render at heading size, bold renders bold, italic renders italic, links show styled link text, inline code and fenced code render in mono with a code background — all inline with no separate preview pane."
    why_human: "jsdom has no layout engine; decoration visual rendering cannot be asserted in vitest (only decoration kinds and classes are asserted, not pixels)."
  - test: "In the same editor, move the cursor through bold/heading/link lines. Observe the active line."
    expected: "Syntax markers (**, #, *, backticks, link brackets/URL) reveal on the line containing the cursor; lines above and below do NOT visibly jump (layout-neutral reveal)."
    why_human: "Reflow detection requires a rendered layout engine (the Pitfall 2 constraint is code-enforced by using only class marks and zero-width hides, but the absence of reflow can only be confirmed visually)."
  - test: "Edit a page containing an image (e.g. ![alt](/api/attachments/foo.png)) and a GFM table. Switch to Live mode."
    expected: "The image renders as an actual <img> widget. The GFM table renders as a styled grid. Moving the cursor onto their line reveals the raw Markdown source for editing."
    why_human: "Image layout and table visual fidelity require a real browser (jsdom renders the elements but not their visual presentation)."
  - test: "With an image whose src is javascript:alert(1), observe the Live render."
    expected: "The raw markdown text ![x](javascript:alert(1)) is shown as plain text, not a broken-image element and no alert fires."
    why_human: "Security-behavior spot-check — the automated widgets.test.ts asserts tagName=SPAN and textContent matches, but a browser smoke-test confirms no side effect."
  - test: "Toggle Live/Source via the header toggle and via Cmd/Ctrl-E. Reload the page."
    expected: "Content is byte-identical in both modes; reloading restores the last-used mode (persisted in localStorage)."
    why_human: "Toggle byte-stability is fully automated (mode.test.ts over corpus), but the visual appearance of Source mode (monospace document) and the persistence UX need a real browser session."
  - test: "Open a page in read mode (/app/page/<page>). Compare it to the same page in edit Live mode."
    expected: "Read mode looks pixel-identical to edit Live mode. Internal .md links navigate within the SPA. The page is non-editable (no cursor on click, but text can be selected/copied)."
    why_human: "Pixel-identical visual parity between read and edit is a perceptual quality check that jsdom cannot measure."
  - test: "From the search palette, click a HEADING result. Or visit /app/page/<path>#<heading-slug> directly."
    expected: "The read surface scrolls to the matching heading on mount. The browser URL fragment matches the visible heading."
    why_human: "scrollToHash calls el.scrollIntoView which is a no-op in jsdom; the automated headingAnchors.test.ts asserts id correctness but not that scrolling fires in a real browser."
  - test: "Type text, wait 1 second for autosave. Then check AutosaveStatus shows 'Saved'. Force a 409 conflict (open the same page in two tabs, edit both, save the second)."
    expected: "AutosaveStatus transitions idle → saving → saved. The conflict tab shows the 409 ConflictBanner with a Reload action."
    why_human: "The automated PageEditor.test.tsx mocks the API; confirming the autosave/conflict behavior against a live backend requires a running server."
---

# Phase 6: Live-Preview Editor (Obsidian-style) — Verification Report

**Phase Goal:** As an editor accustomed to Obsidian, I want a live-preview Markdown editor that renders formatting inline as I type (with a source/raw toggle), so that editing in the web app feels like Obsidian rather than a split source+preview pane.
**Verified:** 2026-06-21T14:25:00Z
**Status:** human_needed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths (from ROADMAP Success Criteria + PLAN must_haves)

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | While editing, Markdown formatting (headings, bold/italic, lists, links, inline code, code blocks, inline images, GFM tables) renders inline as the user types | VERIFIED | `livePreview.ts` ViewPlugin walks the Lezer tree over visibleRanges, emitting Decoration.mark/replace for each construct; `livePreview.test.ts` 13 tests GREEN asserting decoration kinds for all constructs including image and table widgets |
| 2 | Toggling Live/Source never alters the underlying Markdown bytes | VERIFIED | `mode.ts` setMode dispatches `effects: modeCompartment.reconfigure(...)` with NO `changes`; `mode.test.ts` 9 tests GREEN assert `doc.toString()` byte-identical across Live⇄Source⇄Live for all 8 okf corpus fixtures |
| 3 | Saving produces byte-identical Markdown; okf golden-corpus gate still holds | VERIFIED | CM6 `updateListener` ships `u.state.doc.toString()` verbatim on docChanged only; `LivePreviewEditor.test.tsx` 5 tests GREEN for verbatim-bytes onChange contract; `go test ./internal/okf/ -run TestGoldenRoundTrip` passes (0.002s) |
| 4 | Autosave drafts, optimistic-concurrency save, and sanitized rendering preserved | VERIFIED | PageEditor save machinery (runSaver, baseRevision, DRAFT_DEBOUNCE_MS, saving guard, 409 ConflictBanner) untouched at lines 85-146; `PageEditor.test.tsx` 4 tests GREEN against LivePreviewEditor; no raw HTML: controlled decorations only, no innerHTML assignments in widgets.ts/livePreview.ts |
| 5 | Read mode renders on the unified live-preview surface; SRCH-06 heading deep-link preserved | VERIFIED | `PageView.tsx` renders `<LivePreviewEditor value={body} onChange={()=>{}} currentPath={path} mode="live" readOnly />`; headingAnchors StateField stamps `Decoration.line({attributes:{id: slug}})` on every ATX heading; `headingAnchors.test.ts` 15 tests GREEN asserting id===slug(text) over corpus + dedup parity with okf.ScanHeadings; scrollToHash wired on mount |
| 6 | @uiw/react-md-editor removed; CLAUDE.md editor row updated | VERIFIED | `grep -rn "@uiw/react-md-editor" web/src web/package.json` returns nothing; `@codemirror/*` deps present in package.json; CLAUDE.md lines 72/112/125 updated to document CM6 path with note that md-editor was removed in Phase 6 |

**Score: 6/6 truths verified**

### Deferred Items

None.

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `web/src/components/LivePreviewEditor.tsx` | CM6 EditorView wrapper, value/onChange drop-in | VERIFIED | 185 lines; EditorView created in useEffect; externalSeed annotation; StrictMode-safe destroy; readOnly prop |
| `web/src/lib/cm/livePreview.ts` | ViewPlugin: Lezer tree walk → DecorationSet + active-line reveal | VERIFIED | 397 lines; ViewPlugin.fromClass; buildDecorations iterates visibleRanges; imageWidget/tableField included |
| `web/src/lib/cm/mode.ts` | modeCompartment + Live/Source extension sets + Mod-e keymap | VERIFIED | Compartment exported; liveExtensions includes livePreviewExtension; toggleKeymap binds Mod-e |
| `web/src/lib/cm/sanitizeSrc.ts` | sanitizeImageSrc allowlist gate | VERIFIED | 51 lines; SCHEME_RE + ALLOWED_SCHEMES; http/https + no-scheme allowed; all else null |
| `web/src/lib/cm/headingAnchors.ts` | slug() + dedupSlug + headingAnchors StateField + scrollToHash | VERIFIED | StateField.define over whole document; Decoration.line({attributes:{id}}); scrollToHash uses CSS.escape |
| `web/src/lib/cm/widgets.ts` | ImageWidget + TableWidget WidgetType subclasses | VERIFIED | Both extend WidgetType; ImageWidget calls sanitizeImageSrc; null → raw text span; all DOM via createElement+textContent |
| `web/src/lib/cm/linkNav.ts` | domEventHandlers click → resolveRelativeMdLink → navigate | VERIFIED | EditorView.domEventHandlers on mousedown; resolveRelativeMdLink rejects unsafe schemes |
| `web/src/routes/PageView.tsx` | read mode via read-only LivePreviewEditor (MarkdownProse retired from read path) | VERIFIED | Line 107-113: `<LivePreviewEditor value={body} onChange={()=>{}} currentPath={path} mode="live" readOnly />`; no MarkdownProse import |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| `PageEditor.tsx` | `LivePreviewEditor.tsx` | `<LivePreviewEditor value={body} onChange={onBodyChange} currentPath={path} mode={mode} />` | WIRED | Line 279; MDEditor import removed |
| `LivePreviewEditor.tsx` | `mode.ts` | `modeCompartment.of(...)` + mode-reconfigure effect | WIRED | Lines 10-12, 110, 118, 177-181 |
| `mode.ts` | `livePreview.ts` | `liveExtensions` includes `livePreviewExtension` | WIRED | Line 30: `livePreviewExtension` in liveExtensions array |
| `livePreview.ts` | `widgets.ts` | Image/Table nodes → `ImageWidget`/`TableWidget` | WIRED | Line 57: `import { ImageWidget, TableWidget }` |
| `widgets.ts` | `sanitizeSrc.ts` | `ImageWidget.toDOM()` calls `sanitizeImageSrc(src)` | WIRED | Line 24: import; line 47: `sanitizeImageSrc(this.src)` |
| `linkNav.ts` | `mdlink.ts` | `resolveRelativeMdLink(pathOf(), href)` | WIRED | Line 17: import; line 43: call |
| `LivePreviewEditor.tsx` | `headingAnchors.ts` | `headingAnchors` + `scrollToHash` in readOnly config | WIRED | Lines 14, 107, 144 |
| `PageView.tsx` | `LivePreviewEditor.tsx` | read-only LivePreviewEditor replaces MarkdownProse | WIRED | Lines 9, 107-113; no MarkdownProse import |
| `headingAnchors.ts` | `internal/okf/headings.go` slug algorithm | `slug()` ported byte-for-byte; headingAnchors.test.ts asserts equality over corpus | VERIFIED by test | 15 tests GREEN including dedup parity |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|---------------|--------|-------------------|--------|
| `LivePreviewEditor.tsx` | `value: string` | Passed as prop from PageEditor (`body` state) / PageView (`data.body`) | Yes — PageEditor seeds from TanStack Query `data.body`; onChange updates `body` state which CM6 updateListener reads via `u.state.doc.toString()` | FLOWING |
| `headingAnchors.ts` | `id` on `.cm-line` | `slug(headingText(line.text))` on each ATX heading parsed by Lezer from the CM6 document | Yes — computed from actual document content; StateField recomputes on docChanged | FLOWING |
| `sanitizeSrc.ts` | return value | `this.src` passed by livePreview.ts after parsing from the CM6 document via `parseImage()` | Yes — reads from actual Markdown bytes in the CM6 state | FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| All vitest tests (191 total) pass | `cd web && npx vitest run` | 191 passed (24 files), 0 failed | PASS |
| mode.test.ts: byte-stable toggle over all corpus fixtures | `cd web && npx vitest run src/lib/cm/mode.test.ts` | 9 tests passed | PASS |
| livePreview.test.ts: decorations for all constructs + reveal | `cd web && npx vitest run src/lib/cm/livePreview.test.ts` | 13 tests passed | PASS |
| sanitizeSrc.test.ts: allowlist gate | `cd web && npx vitest run src/lib/cm/sanitizeSrc.test.ts` | 8 tests passed | PASS |
| headingAnchors.test.ts: id==slug(text) over corpus + dedup | `cd web && npx vitest run src/lib/cm/headingAnchors.test.ts` | 15 tests passed | PASS |
| LivePreviewEditor.test.tsx: verbatim-bytes onChange contract | `cd web && npx vitest run src/components/LivePreviewEditor.test.tsx` | 5 tests passed | PASS |
| PageEditor.test.tsx: autosave/conflict against new surface | `cd web && npx vitest run src/routes/PageEditor.test.tsx` | 4 tests passed | PASS |
| TypeScript compilation | `cd web && npx tsc --noEmit` | Exit 0, clean | PASS |
| Backend golden round-trip | `go test ./internal/okf/ -run TestGoldenRoundTrip` | ok 0.002s | PASS |
| No remaining react-md-editor imports | `grep -rn "@uiw/react-md-editor" web/src web/package.json` | No output | PASS |

### Probe Execution

Step 7c: No probe scripts declared or present for this phase. SKIPPED.

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|------------|-------------|--------|---------|
| EDIT-01 | 06-02-PLAN, 06-03-PLAN | Markdown formatting renders inline as user types (headings, bold/italic, lists, links, inline code, code blocks, inline images, GFM tables) | SATISFIED | livePreview.ts ViewPlugin + imageWidget + tableField; livePreview.test.ts 13 tests GREEN |
| EDIT-02 | 06-01-PLAN | Toggle never alters Markdown bytes | SATISFIED | Compartment effects-only dispatch; mode.test.ts 9 tests GREEN over 8 corpus fixtures |
| EDIT-03 | 06-01-PLAN | Byte-identical save; backend corpus gate holds | SATISFIED | updateListener ships doc.toString() verbatim; TestGoldenRoundTrip GREEN |
| EDIT-04 | 06-01-PLAN, 06-03-PLAN | Save machinery preserved; no raw HTML; image-src allowlist | SATISFIED | runSaver/baseRevision/saving untouched; no innerHTML in widgets; sanitizeImageSrc blocks javascript:/vbscript:/data: |
| SRCH-06 (preserve) | 06-04-PLAN | Heading deep-link preserved on unified read surface | SATISFIED | headingAnchors StateField; id==slug(text); scrollToHash on mount; 15 tests GREEN |

Note: SRCH-06 in REQUIREMENTS.md tracks "search returns typed results" (Phase 3 scope, pending). The Phase 6 obligation is narrower: preserve the existing heading-anchor deep-link capability when unifying the read surface. That narrower obligation is verified above.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| `web/src/components/LivePreviewEditor.tsx` | 151 | `// eslint-disable-next-line react-hooks/exhaustive-deps` on the creation effect | Info | Intentional — the creation effect intentionally omits `value`/`mode` from deps (synced by separate effects); well-documented in comment |

No TBD, FIXME, or XXX markers found in any phase-modified file. No hardcoded hex/px literals in theme.ts. No innerHTML assignments in widget/livePreview code paths.

MarkdownProse.tsx is retained on disk (not deleted) because HistoryPanel.tsx still imports it for historical-version preview rendering. This is explicitly documented in 06-04-SUMMARY as a concrete blocker; it is not a stub or a gap — MarkdownProse is removed from the read path (PageView) as planned.

### Human Verification Required

All six automated correctness dimensions are GREEN. The following eight checks remain for a human running the live app. They are **advisory** (the correctness proofs exist in automated tests) — they confirm perceptual quality and live-server behavior only.

#### 1. Live-preview inline render (visual quality)

**Test:** Open `/app/edit/<page>`. Type: a heading (`# Heading`), bold (`**word**`), italic (`*word*`), a link (`[text](http://example.com)`), inline code (`` `code` ``), and a fenced code block.
**Expected:** Each construct renders inline (heading at heading size, bold rendered bold, etc.) without a separate preview pane.
**Why human:** jsdom has no layout engine; decoration classes are tested but not their visual rendering.

#### 2. Active-line marker reveal, layout-neutral

**Test:** In the same editor, move the cursor through bold, heading, and link lines.
**Expected:** Syntax markers reveal on the cursor's line; lines above and below do NOT jump vertically.
**Why human:** Reflow is a layout-engine concern that jsdom cannot detect. The code enforces layout-neutrality via zero-width hides and class-only marks, but a browser confirms it.

#### 3. Inline image and GFM table widget + active-line reveal

**Test:** Edit a page containing `![alt](/api/attachments/foo.png)` and a GFM `|table|`. View in Live mode.
**Expected:** Image renders as an `<img>`; table renders as a styled grid. Moving the cursor to their line shows the raw Markdown source.
**Why human:** Image layout and table grid visual fidelity require a real browser.

#### 4. Script-scheme image src is inert (live browser confirmation)

**Test:** Edit a page with `![x](javascript:alert(1))`. View in Live mode.
**Expected:** The raw text `![x](javascript:alert(1))` is shown as plain text. No alert fires. No broken-image element.
**Why human:** The automated test asserts tagName=SPAN and textContent correctness, but a browser smoke-test confirms no execution side-effect.

#### 5. Live/Source toggle persistence (UX)

**Test:** Toggle via the header toggle and via Cmd/Ctrl-E. Reload the page.
**Expected:** Content is unchanged in both modes. After reload, the last-used mode is restored (Source stays Source; Live stays Live).
**Why human:** The mode store persistence (zustand+persist, key `okf.editor.mode`) is tested in editorMode.test.ts but localStorage behaviour in a real browser session is the UX confirmation.

#### 6. Read mode = edit Live (unified surface, visual parity)

**Test:** Open the same page at `/app/page/<page>` and `/app/edit/<page>`. Compare visually.
**Expected:** Read mode looks pixel-identical to edit Live mode. Internal `.md` links navigate within the SPA from the read surface. The read surface is non-editable (clicking does not produce a caret) but text can be selected/copied.
**Why human:** Pixel-identical parity is a perceptual quality check. Read-only enforcement (no caret) is a browser behavior.

#### 7. SRCH-06 heading deep-link (live browser)

**Test:** Search for a page, click a heading result (or visit `/app/page/<path>#<heading-slug>` directly).
**Expected:** The read surface scrolls to the matching heading on mount. The heading is visible without manual scroll.
**Why human:** `scrollToHash` calls `el.scrollIntoView()`, which is a no-op in jsdom. The automated test confirms the element lookup (id correctness + CSS.escape), but the scroll action requires a real browser viewport.

#### 8. Autosave and 409 ConflictBanner (live server)

**Test:** Edit a page, wait ~1 second. Observe AutosaveStatus. Then open the same page in a second tab, edit both, save the second tab — return to the first tab and try to autosave.
**Expected:** AutosaveStatus shows idle → saving → saved. The first tab shows the 409 ConflictBanner with a "Reload" action.
**Why human:** PageEditor.test.tsx mocks the API; the save machinery integration against a live backend with real Git commits requires a running server.

---

## Gaps Summary

None. All 6 must-have truths are VERIFIED by code and automated test evidence. The `human_needed` status reflects eight perceptual/live-server checks that are advisory only — they confirm visual quality and browser UX, not correctness. The correctness layer (byte-stability, decoration kinds, heading id parity, XSS allowlist, save machinery) is fully covered by 191 automated tests, all GREEN.

---

_Verified: 2026-06-21T14:25:00Z_
_Verifier: Claude (gsd-verifier)_
