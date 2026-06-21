# Phase 6 — UI Review

**Audited:** 2026-06-21
**Baseline:** 06-UI-SPEC.md (approved design contract)
**Screenshots:** Not captured — port 3000 was occupied by an unrelated process ("Open Presentation Preview in Obsidian first!"); the app dev server was not running. Audit is code-only (CSS, TSX, theme.ts analysis).

---

## Pillar Scores

| Pillar | Score | Key Finding |
|--------|-------|-------------|
| 1. Copywriting | 3/4 | All spec-mandated strings present and verbatim; "Edit page" CTA absent from spec copy table but is pre-existing carry-over, not net-new invention |
| 2. Visuals | 3/4 | Heading vertical rhythm in edit mode diverges from read mode — CM6 heading classes carry no margin, MarkdownProse.css carries top-margin on h1/h2/h3–h6 |
| 3. Color | 2/4 | Active toggle segment uses a full accent background fill, violating the spec's explicit "accent text or 2px underline only — do NOT fill the whole toggle with accent" |
| 4. Typography | 3/4 | Code block vertical padding is missing (horizontal only), breaking visual parity with MarkdownProse `pre { padding: var(--space-md) }` |
| 5. Spacing | 3/4 | All spacing values use token vars; one off-spec value: code block `padding: "0 var(--space-md)"` applies 0 vertical padding vs the spec's `--space-md` all-sides |
| 6. Experience Design | 4/4 | Loading, error (409/generic), empty, conflict, and save-error states all handled; keyboard shortcut wired; mode persisted; autosave guard present |

**Overall: 18/24**

---

## Top 3 Priority Fixes

1. **Active toggle segment accent fill** (PageEditor.css:40-42) — Breaks the spec's 60/30/10 color discipline: the entire `.pageeditor-mode-btn[aria-pressed="true"]` is painted with `--color-accent` background. The UI-SPEC §Color states: "do NOT fill the whole toggle with accent — use accent text or a 2px accent underline on the active mode label." Fix: replace `background: var(--color-accent); color: var(--color-accent-fg); border-color: var(--color-accent)` with `color: var(--color-accent); font-weight: var(--font-weight-semibold); border-bottom: 2px solid var(--color-accent)` (or equivalent low-emphasis active indicator).

2. **Code block vertical padding missing in CM6 theme** (theme.ts:71) — `.cm-code-block` sets `padding: "0 var(--space-md)"`, which gives 0px top and bottom. MarkdownProse's `pre` uses `padding: var(--space-md)` (all four sides = 16px). This breaks the "byte-for-byte visual parity" guarantee between edit Live mode and read mode for fenced code blocks. Fix: change to `padding: "var(--space-md)"` (four sides).

3. **Heading vertical rhythm absent in CM6** (theme.ts lines 75-88) — `.cm-heading-1`, `.cm-heading-2`, `.cm-heading-3..6` set only `fontSize`, `fontWeight`, and `lineHeight`. They have no `margin` rules. MarkdownProse sets `margin: var(--space-xl) 0 var(--space-md)` on h1, `margin: var(--space-lg) 0 var(--space-sm)` on h2, and `margin: var(--space-md) 0 var(--space-sm)` on h3–h6. In edit Live mode, headings render without spacing between sections, visually diverging from read mode and breaking the unified-surface parity promise. Fix: add corresponding margin rules to each `.cm-heading-N` selector in theme.ts.

---

## Detailed Findings

### Pillar 1: Copywriting (3/4)

**Spec-mandated strings — all present verbatim:**
- "Save page" — PageEditor.tsx:305. PASS.
- "Live" / "Source" — PageEditor.tsx:280, 288. PASS.
- `aria-label="Editor mode"` — PageEditor.tsx:271. PASS.
- `aria-pressed` wired on each button — PageEditor.tsx:277, 285. PASS.
- "Start writing in Markdown…" placeholder — LivePreviewEditor.tsx:116. PASS.
- "This page is empty. Select Edit to start writing." — PageView.tsx:105. PASS.
- "Loading…" — PageEditor.tsx:215, PageView.tsx:56. PASS.
- Save error verbatim — PageEditor.tsx:123-126. PASS.
- Conflict banner verbatim — PageEditor.tsx:222-230. PASS.
- 404 state — PageView.tsx:63-71. PASS.
- Generic read error — PageView.tsx:74. PASS.

**WARNING — Keyboard shortcut hint delivery:**
- UI-SPEC: "shortcut hinted: Toggle live preview (⌘E / Ctrl+E)". Implemented via `title="Toggle live preview (⌘E / Ctrl+E)"` on the `role="group"` container (PageEditor.tsx:272). The `title` attribute is not reliably surfaced to screen readers (NVDA/VoiceOver omit it on non-interactive elements) and never shows on touch devices. The hint should be on a visible `<kbd>` element, an `aria-describedby` association on the buttons, or a visible inline label. This does not break any user flow but degrades keyboard-user discovery.

**Note:** "Edit page" (PageView.tsx:91) is absent from the spec copy table. It is a pre-existing string from PageView (Phase 1 carryover) and not a net-new invention introduced by Phase 6 — not scored against this phase.

### Pillar 2: Visuals (3/4)

**PASS items:**
- Focal point is clear on the editor surface; the toolbar with Live/Source toggle and Save CTA provides a legible hierarchy.
- Icon in "Edit page" (Pencil, `aria-hidden="true"`, with sibling `<span>`) is accessible.
- Content column correctly centered via `margin-left: auto; margin-right: auto` on `.livepreview-editor .cm-content`.
- Image widget fallback (`.cm-md-image-raw`) renders in mono text, never a broken-image chrome.

**WARNING — Heading vertical rhythm (edit vs read mode divergence):**
- In read mode (`PageView`), the old `MarkdownProse.css` heading rules provide vertical breathing room: h1 gets 32px top margin, h2 gets 24px top margin, h3–h6 get 16px top margin. In edit Live mode, the CM6 heading line decorations (`.cm-heading-1..6`) carry only font size/weight/line-height — no margin. A document with multiple heading levels in the editor will appear more compressed than in read mode, undermining the "pixel-identical" unified-surface claim and the Obsidian-parity feel objective.

**Note — shortcut tooltip (`title` attribute) invisible on touch:** Flagged under Copywriting but also affects visual UX; no on-screen indicator of ⌘E exists for non-hovering users.

### Pillar 3: Color (2/4)

**BLOCKER — Active toggle segment uses full accent background fill:**
- PageEditor.css:39-43 — `.pageeditor-mode-btn[aria-pressed="true"]` applies:
  ```
  background: var(--color-accent);    /* #2563eb fills the entire button */
  color: var(--color-accent-fg);      /* white text */
  border-color: var(--color-accent);
  ```
  The UI-SPEC §Color is explicit: "The active/selected segment of the Live/Source toggle (accent text or 2px accent underline on the active mode label) — low-emphasis; do NOT fill the whole toggle with accent." The current implementation is a primary-button-strength accent fill, not the contracted low-emphasis active indicator. This over-uses the 10% accent slot — every time the editor is open, a large solid-blue button dominates the toolbar.

**PASS items:**
- Links: `color: var(--color-accent)` — accent reserved for links only in theme.ts:93. PASS.
- Focus ring: `outline: 2px solid var(--color-accent)` in LivePreviewEditor.css:23 — matches spec. PASS.
- Borders/dividers: `var(--color-border)` throughout. PASS.
- Muted text: `var(--color-text-muted)` on inactive labels and empty states. PASS.
- No hardcoded hex values in any production CSS or theme files (test files excluded). PASS.
- Active-line background: not present (no accent used for reveal). PASS.
- "Save page" `.btn-primary` is the one primary CTA using accent background — this is correct per spec.

### Pillar 4: Typography (3/4)

**PASS items:**
- 4 sizes, 2 weights — tokens used exclusively, no additional sizes or weights introduced.
- `.cm-heading-1`: `--font-size-display` / `--font-weight-semibold` / `--line-height-display`. Matches spec.
- `.cm-heading-2`: `--font-size-heading` / `--font-weight-semibold` / `--line-height-heading`. Matches spec.
- `.cm-heading-3..6`: `--font-size-body` / `--font-weight-semibold` / `--line-height-label`. Matches both spec and MarkdownProse.css.
- `.cm-strong`: `--font-weight-semibold`, no size change (layout-neutral). PASS.
- `.cm-em`: `font-style: italic`, `--font-weight-regular`. PASS.
- `.cm-inline-code`: `--font-family-mono`, `--color-code-bg`, `--radius-sm`, `0 var(--space-xs)` padding. Matches MarkdownProse.css code. PASS.
- Source mode: `--font-family-mono` document-wide via `sourceTheme`. PASS.

**WARNING — Code block vertical padding:**
- theme.ts:71: `.cm-code-block { padding: "0 var(--space-md)" }` — shorthand `0 [right/left]` means 0px top/bottom padding.
- MarkdownProse.css:57: `pre { padding: var(--space-md) }` — 16px all sides.
- The vertical padding discrepancy means fenced code blocks in edit Live mode are visually tighter than in read mode, breaking the "byte-for-byte visual parity" contract. This is a typography and spacing failure combined.

### Pillar 5: Spacing (3/4)

**PASS items:**
- All spacing uses token vars: `--space-xs`, `--space-sm`, `--space-md`, `--space-lg` — never arbitrary pixel values or bare numbers.
- No `[Npx]` or `[Nrem]` Tailwind arbitrary values (N/A — project uses plain CSS, confirmed).
- Table cell padding `var(--space-xs) var(--space-sm)` — matches MarkdownProse.css table exactly. PASS.
- Table margin `var(--space-md) 0` — matches MarkdownProse.css. PASS.
- `.pageeditor` gap between sections: `var(--space-md)`. PASS.
- `.pageeditor-frontmatter` gap: `var(--space-sm)`. PASS.
- Toggle group gap: `var(--space-xs)`. PASS.

**WARNING — Code block vertical padding (same defect as Typography Pillar 4):**
- `padding: "0 var(--space-md)"` on `.cm-code-block` produces 0px top/bottom vs the spec's `--space-md` (16px) on all sides. Off-scale in the vertical axis.

**Minor — Heading top margins absent (also flagged in Visuals):**
- No `margin` in `.cm-heading-1..6` means heading-to-heading and paragraph-to-heading vertical spacing is whatever the editor line-height provides, rather than the spec's declared scale values (xl/lg/md). Not an arbitrary value violation, but a missing application of the declared spacing scale.

### Pillar 6: Experience Design (4/4)

All five state categories are handled correctly and no new gaps were introduced:

- **Loading:** `<p className="pageeditor-status">Loading…</p>` (PageEditor.tsx:215), `<p className="pageview-status">Loading…</p>` (PageView.tsx:56). Both render before data arrives.
- **Error (save):** `saveError` banner with verbatim spec copy (PageEditor.tsx:233-236); triggered on any non-409 save failure.
- **Conflict (409):** `ConflictBanner` with "Reload page" button (PageEditor.tsx:220-231). Correctly gates further saves.
- **Empty page (read):** `.pageview-empty` paragraph with spec copy (PageView.tsx:105). Shown when `body.trim() === ""`.
- **Empty editor (edit):** CM6 `placeholder("Start writing in Markdown…")` (LivePreviewEditor.tsx:116). Non-blocking.
- **Mode persistence:** `useEditorMode` zustand store with `persist` middleware, key `okf.editor.mode` (editorMode.ts). Default "live". PASS.
- **Keyboard shortcut:** `toggleKeymap` wired in edit mode (LivePreviewEditor.tsx:115); Cmd/Ctrl-E toggles mode via store. PASS.
- **Byte-stable toggle:** Mode change dispatches `modeCompartment.reconfigure` only — no document mutation (LivePreviewEditor.tsx:176-181). PASS.
- **Read-only surface:** `EditorState.readOnly.of(true)` + `EditorView.editable.of(false)` in read mode; `onChange` is never called (LivePreviewEditor.tsx:104-105). PASS.
- **Heading deep-links:** `headingAnchors` extension stamps github-slugger ids on heading lines; `scrollToHash` runs on mount (LivePreviewEditor.tsx:143-145). PASS.
- **Security:** `sanitizeSrc` referenced in widgets.ts for image widget; `resolveRelativeMdLink` scheme allowlist for link navigation. No raw HTML from content. PASS.

---

## Registry Safety

Registry audit: 0 third-party blocks checked — `components.json` not present (shadcn not initialized). Skipped per protocol.

---

## Files Audited

- `/home/john/go/src/github.com/postfix/okworkspace/.planning/phases/06-live-preview-editor-obsidian-style/06-UI-SPEC.md`
- `/home/john/go/src/github.com/postfix/okworkspace/.planning/phases/06-live-preview-editor-obsidian-style/06-CONTEXT.md`
- `/home/john/go/src/github.com/postfix/okworkspace/web/src/lib/cm/theme.ts`
- `/home/john/go/src/github.com/postfix/okworkspace/web/src/lib/cm/livePreview.ts`
- `/home/john/go/src/github.com/postfix/okworkspace/web/src/lib/cm/mode.ts` (directory listing confirmed; content reviewed via store/integration analysis)
- `/home/john/go/src/github.com/postfix/okworkspace/web/src/components/LivePreviewEditor.tsx`
- `/home/john/go/src/github.com/postfix/okworkspace/web/src/components/LivePreviewEditor.css`
- `/home/john/go/src/github.com/postfix/okworkspace/web/src/components/MarkdownProse.css`
- `/home/john/go/src/github.com/postfix/okworkspace/web/src/routes/PageEditor.tsx`
- `/home/john/go/src/github.com/postfix/okworkspace/web/src/routes/PageEditor.css`
- `/home/john/go/src/github.com/postfix/okworkspace/web/src/routes/PageView.tsx`
- `/home/john/go/src/github.com/postfix/okworkspace/web/src/routes/PageView.css`
- `/home/john/go/src/github.com/postfix/okworkspace/web/src/styles/tokens.css`
- `/home/john/go/src/github.com/postfix/okworkspace/web/src/stores/editorMode.ts`
