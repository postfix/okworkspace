---
phase: 7
slug: obsidian-style-file-tree-folder-operations-tree-ux
audited: 2026-06-21
baseline: 07-UI-SPEC.md (approved)
screenshots: not captured (no dev server detected during audit; code-only audit)
---

# Phase 7 — UI Review

**Audited:** 2026-06-21
**Baseline:** 07-UI-SPEC.md (approved design contract)
**Screenshots:** not captured (code-only audit)

---

## Pillar Scores

| Pillar | Score | Key Finding |
|--------|-------|-------------|
| 1. Copywriting | 3/4 | Rename-folder help text diverges from spec; empty tree state missing entirely |
| 2. Visuals | 2/4 | Folder rows have no hover background; no empty-tree visual state rendered |
| 3. Color | 3/4 | Dialog backdrop uses hardcoded `rgba(16, 24, 40, 0.45)` — no CSS variable |
| 4. Typography | 4/4 | All four sizes and both weights via tokens; no violations |
| 5. Spacing | 3/4 | `gap: 2px` in `.trashview-row-main` violates the 4px floor; not a token |
| 6. Experience Design | 3/4 | Folder-row hover feedback absent; empty tree state never rendered |

**Overall: 18/24**

---

## Top 3 Priority Fixes

1. **Folder row hover state missing** — Users hover a folder and see zero visual feedback before right-clicking or dragging. Add `.navrow-folder:hover { background: var(--color-tree-hover); }` to LeftTree.css (5 minutes; matches the page-row treatment). BLOCKER-quality for Obsidian feel because Obsidian highlights every row on hover.

2. **Empty tree state never rendered** — When `nodes.length === 0` the component renders an empty `<ul aria-label="Pages">` with no guidance. The spec mandates the string "No pages yet. Right-click here or use New page to start." in a `lefttree-status` element. Add a conditional block after the `<ul>` (or inside it): `{nodes.length === 0 && <li className="lefttree-status">No pages yet. Right-click here or use New page to start.</li>}` — new users land on a blank nav rail with no instruction.

3. **Rename-folder help text diverges from spec** — Spec: `"Pages inside this folder, and links to them, will keep working."` Implemented (RenameModal.tsx:35): `"Links to pages in this folder will keep working."` The spec phrasing emphasises the folder's pages survive; the implemented string subtly changes the emphasis and omits the second clause. Fix: update the `folder.help` string in `RenameModal.tsx` to match the spec verbatim.

---

## Detailed Findings

### Pillar 1: Copywriting (3/4)

**WARNING — two deviations from the Copywriting Contract.**

**Finding 1-A (WARNING): Rename-folder help text not verbatim.**
- Spec (07-UI-SPEC.md): `"Pages inside this folder, and links to them, will keep working."`
- Implemented (`RenameModal.tsx:35`): `"Links to pages in this folder will keep working."`
- The meaning is similar but not identical; the spec copy is the user-tested string. Change the `folder.help` constant to the spec string.

**Finding 1-B (WARNING): Empty tree state copy never rendered.**
- Spec: `"No pages yet. Right-click here or use New page to start."` in `.lefttree-status` style.
- Implemented: `LeftTree.tsx` has no conditional block for `nodes.length === 0`. The `<ul>` renders with zero children — a silent blank. New users see an empty nav rail with no hint of what to do.

**Passing items (representative):**
- Folder context menu items: `New page here`, `New folder here`, `Rename`, `Move`, `Delete` — exact match (LeftTree.tsx:159-182).
- Page context menu: `Rename`, `Move`, `Version history`, `Delete` — exact match.
- Root menu: `New page`, `New folder` — exact match.
- Delete folder dialog title `Delete '{title}'?`, body with `pagesLabel` (`"its 1 page"` / `"its {N} pages"`), confirm `Delete folder`, cancel `Keep folder` — all verbatim (DeleteFolderDialog.tsx:62-81).
- Optimistic rollback copy `ROLLBACK_COPY` — exact spec string (useTreeMutations.ts:29-31).
- Folder collision copy — exact spec string (RenameModal.tsx:47, MoveDialog.tsx:62).
- Trash group label renders as `Folder '{name}' · {N} pages` — exact (TrashView.tsx:207-208).
- Restore folder button label `Restore folder` — exact (TrashView.tsx:219).
- Loading `Loading…` and error `Couldn't load your pages — try again.` — exact (LeftTree.tsx:227-238).

---

### Pillar 2: Visuals (2/4)

**WARNING — folder rows have no hover treatment; empty state leaves a blank void.**

**Finding 2-A (BLOCKER): No hover background on folder rows.**
- `.navrow-page:hover` sets `background: var(--color-tree-hover)` (LeftTree.css:57-59).
- `.navrow-folder` has no `:hover` rule in LeftTree.css. The base `.navrow` in AppShell.css also lacks a generic `:hover` rule (only `.navrow-action:hover` covers action buttons, which are distinct elements).
- Result: hovering a folder row shows no background change. This breaks the Obsidian-feel bar explicitly named in the UI-SPEC ("the tree feels like Obsidian's file explorer") since Obsidian uniformly highlights all rows on hover.
- Fix: `LeftTree.css` — add `.navrow-folder:hover { background: var(--color-tree-hover); }`

**Finding 2-B (WARNING): Empty tree state leaves a blank visual area.**
- Spec: a muted status text inside `.lefttree-status` provides a clear focal point and instruction for empty repos.
- Implemented: a bare `<ul>` with no children. Visually, just whitespace — no focal point, no affordance.

**Passing items:**
- Active row visual treatment exact: `color-tree-active-bg` fill + `inset 2px 0 0 var(--color-accent)` left bar + accent text (LeftTree.css:61-65). Matches spec to the pixel.
- Drop-target highlight: `.lefttree-droptarget` applies `color-tree-active-bg` fill + `inset 0 0 0 1px var(--color-accent)` ring — exactly the spec's 1px full-ring accent treatment (LeftTree.css:21-25).
- Icons (`ChevronRight`, `ChevronDown`, `FileText`, `Folder`, `Undo2`, `Trash2`) all from lucide-react; `aria-hidden="true"` on all decorative uses — correct.
- Context menu `role="menu"` + `role="menuitem"`, `aria-orientation="vertical"`, focus-visible treatment — correct.
- Dialog title hierarchy (`h2.dialog-title`) with correct heading size — correct.
- Trash view title uses heading size + icon + semibold — matches spec.
- Caret has accessible `aria-label` + `aria-expanded` — correct.

---

### Pillar 3: Color (3/4)

**WARNING — one hardcoded rgba value in Dialog.css; all tree/menu surfaces clean.**

**Finding 3-A (WARNING): Hardcoded backdrop color in `Dialog.css:4`.**
- `background: rgba(16, 24, 40, 0.45);` — a raw hex-based rgba with no CSS variable.
- The spec states: "The rebuilt `LeftTree.css`, `TreeContextMenu.css`, `Dialog.css`, and `TrashView.css` MUST reference these CSS variables only — never hard-coded hex/px."
- `tokens.css` defines `--shadow-card` and `--shadow-popover` using the same `rgba(16, 24, 40, ...)` base but does not expose a `--color-overlay` or `--color-backdrop` variable. The fix is to add `--color-overlay: rgba(16, 24, 40, 0.45)` to `tokens.css` and reference it in Dialog.css — or to extract the value into a new token consistent with the 60/30/10 system.
- Note: the `2px` and `1px` border/box-shadow dimension values (e.g. `border: 1px solid`) are structural/outline values explicitly noted in the spec as "not spacing-token exceptions" — they are not violations.

**Passing items:**
- Accent used only for: active row bg + left bar, drop-target ring, primary CTA buttons (`.btn-primary`), focus-visible ring via global rule. Not applied to decorative elements or muted text.
- Destructive (`--color-destructive`) exclusively on `.treemenu-item-danger` (Delete menu item) and `.btn-destructive` (Delete folder confirm button). No other usages found.
- Invalid-drop: no color applied — `dragover` without `preventDefault()` so no droptarget class is set; native `cursor: not-allowed` is the sole affordance. Matches spec exactly.
- Hover/focus states use `--color-surface` (30% secondary) — correct 60/30/10 application.
- `--color-text-muted` used for icons, carets, muted helpers — correct.

---

### Pillar 4: Typography (4/4)

All font sizes and weights are applied via CSS variables. No hardcoded size or weight values found across any of the four audited CSS files.

**Distribution observed:**
- `--font-size-label` (14px): `.lefttree-status`, `.trashview-row-meta`
- `--font-size-body` (16px): `.treemenu-item`, `.dialog-body`, `.trashview-row-title`, `.trashview-notice`, `.trashview-empty-body`
- `--font-size-heading` (20px): `.dialog-title`, `.trashview-title`, `.trashview-empty-heading`
- `--font-size-display` (28px): unused on this surface — correct per spec
- `--font-weight-regular` (400): inherited default on tree rows, menu items
- `--font-weight-semibold` (600): dialog titles, trash row titles, trash view title, empty-state heading — all match spec

The spec's weight discipline is followed: active page row label is NOT bolded (accent color + inset bar conveys active state, not weight). No third weight introduced.

---

### Pillar 5: Spacing (3/4)

**WARNING — one sub-token gap value in TrashView.css.**

**Finding 5-A (WARNING): `gap: 2px` in `.trashview-row-main` (TrashView.css:55).**
- The spec declares `--space-xs: 4px` as the smallest token, and "declared values must be multiples of 4."
- `2px` is not a token and falls below the 4px floor.
- The intent is a tight visual separation between the row title and meta text (title above, meta below). Replace with either `gap: 0` (rely on line-height alone) or `gap: var(--space-xs)` (4px), whichever produces the intended density.

**Passing items:**
- Tree row indent formula `calc(${depth} * var(--tree-indent) + var(--space-sm))` — exact spec formula (LeftTree.tsx:481).
- Context-menu padding: `var(--space-xs)` container padding, `var(--space-sm) var(--space-md)` item padding — matches spec.
- Dialog padding `var(--space-lg)` inner, `gap: var(--space-md)` body, `gap: var(--space-sm)` footer — matches spec.
- Trash view `padding: var(--space-xl) var(--space-lg)` (top padding), `padding: var(--space-3xl)` on empty state — matches spec.
- `min-width: 180px` (context menu) and `max-width: 720px` (trash view) — layout dimensions noted in spec as not spacing-token exceptions; not a violation.
- `.lefttree-root-drop` `min-height: var(--space-xl)` — exact spec value.
- `min-height: var(--hit-min-height)` on menu items and dialog footer buttons — correct.

---

### Pillar 6: Experience Design (3/4)

**WARNING — folder-row hover feedback absent; empty tree state missing; all other states and interactions correct.**

**Finding 6-A (WARNING): Folder hover state missing.**
- Without a hover background, the folder row gives no pre-click affordance. Users cannot distinguish interactive rows from static section headers before acting. This degrades discoverability of the right-click menu on folders.

**Finding 6-B (WARNING): Empty tree state not rendered.**
- An empty workspace shows a blank nav rail. New users (or editors who haven't yet created pages) see no instruction and must rely on tribal knowledge. The spec's empty state copy ("No pages yet. Right-click here or use New page to start.") would provide the actionable cue.

**Passing items:**
- Context menu keyboard: Arrow Up/Down, Home/End, Enter/Space, Esc, Tab trap with wrapping — all implemented correctly (TreeContextMenu.tsx:129-158). Focus on first item on open, focus restored to prior element on close.
- Dismiss on outside-click, scroll (capture phase), and resize — correct (TreeContextMenu.tsx:76-91).
- Viewport clamp at 4px margin — correct (TreeContextMenu.tsx:34-55).
- Backdrop never confirms: `onMouseDown` on backdrop calls `onCancel` only; only explicit confirm button calls `onConfirm` (Dialog.tsx:103-105). Esc closes without confirming.
- Optimistic tree updates: `onMutate` snapshots + applies `applyMove`, `onError` rolls back + surfaces `ROLLBACK_COPY`, `onSettled` invalidates — complete (useTreeMutations.ts:220-242).
- Optimistic folder delete: same `onMutate`/`onError`/`onSettled` pattern with `removeNode` (useTreeMutations.ts:248-269).
- Invalid-drop guard during dragover (TREE-06): prefix-check via `dropAllowed`, no `preventDefault()` on invalid, no highlight applied — correct (LeftTree.tsx:393-398).
- Dragleave flicker suppression via `relatedTarget` check — correct (LeftTree.tsx:401-415).
- RBAC gate: readers get no context menu for folders/root; readers get version-history-only menu for pages — correct (LeftTree.tsx:147-156, 186-207).
- Loading state: `role="status"` with "Loading…" text — correct.
- Error state: `role="alert"` with copy — correct.
- Grouped trash restore: `restoreGroupMut` + collision notice via `setNotice` — complete (TrashView.tsx:139-165).
- Collision non-fatal dialog error: `role="alert"` `field-help` paragraph keeps dialog open — correct (RenameModal.tsx:119-121, MoveDialog.tsx:143-145).
- Delete folder navigates away if open page was inside deleted folder — correct (DeleteFolderDialog.tsx:47-54).
- `aria-current="page"` on active page row — correct (LeftTree.tsx:606).

---

## Registry Safety

Registry audit: 0 third-party blocks — shadcn not initialized, no third-party registries declared. Audit gate skipped per spec.

---

## Files Audited

- `/web/src/components/LeftTree.tsx`
- `/web/src/components/LeftTree.css`
- `/web/src/components/TreeContextMenu.tsx`
- `/web/src/components/TreeContextMenu.css`
- `/web/src/components/DeleteFolderDialog.tsx`
- `/web/src/components/MoveDialog.tsx`
- `/web/src/components/RenameModal.tsx`
- `/web/src/components/TrashView.tsx`
- `/web/src/components/TrashView.css`
- `/web/src/components/Dialog.tsx`
- `/web/src/components/Dialog.css`
- `/web/src/components/hooks/useTreeMutations.ts`
- `/web/src/styles/tokens.css`
- `/web/src/routes/AppShell.css` (for `.navrow` base class)
