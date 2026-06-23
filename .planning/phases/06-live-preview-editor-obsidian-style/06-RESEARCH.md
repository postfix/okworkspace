# Phase 6: Live-Preview Editor (Obsidian-style) - Research

**Researched:** 2026-06-21
**Domain:** CodeMirror 6 + Lezer Markdown — custom live-preview decoration layer, unified read/edit rendering, byte-stable round-trip
**Confidence:** HIGH (stack + APIs verified against npm registry and official CodeMirror docs; one MEDIUM area: the heading-deep-link unification strategy)

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

**Editor Engine & Library**
- Build on raw `@codemirror/{state,view,commands}` + `@codemirror/lang-markdown` (Lezer markdown tree); NO React wrapper (`@uiw/react-codemirror` rejected) — full control over the live-preview decoration layer is required.
- Live preview is a custom `ViewPlugin` that walks the Lezer markdown syntax tree and applies `Decoration.mark` / `Decoration.replace` / widget decorations — the Obsidian Live Preview approach.
- The CM6 document IS the raw Markdown string. No block model, no reparse-to-bytes ever — the byte-stable golden-corpus round-trip holds by construction.
- New component `web/src/components/LivePreviewEditor.tsx` wraps `EditorView` via `useRef`/`useEffect`; CM6 extensions live in `web/src/lib/cm/`.

**Rendering Scope (v1)**
- Inline-render: headings, bold/italic, lists, links, inline code, fenced code blocks.
- **Inline images** `![alt](src)` render as real `<img>` widget decorations (user override — IN v1).
- **GFM tables** render as a styled grid (user override — IN v1). Must agree with the server's Goldmark GFM and read-mode remark-gfm.
- Hide syntax markers (`**`, `#`, link brackets/URLs, etc.) EXCEPT on the active line / selection.
- Image widget `src` MUST be sanitized/validated (reject `javascript:`/exec vectors; only allow the app's attachment/relative paths and safe schemes).

**Source/Raw Toggle & Mode Behavior**
- Header toggle, two modes "Live" and "Source"; default **Live**.
- Persist last-used mode in `localStorage` (zustand store pattern).
- `Cmd/Ctrl-E` toggles the mode (Obsidian parity).
- Both modes share ONE `EditorState`; toggle swaps only a decoration `Compartment`. No serialization/reparse on toggle → byte-identical by construction.

**Integration & Preserved Guarantees**
- Keep PageEditor's save machinery UNTOUCHED — `runSaver`, body/frontmatter refs, `baseRevision`, the `saving` guard, autosave debounce, the 409 ConflictBanner. New editor exposes the SAME `value: string` / `onChange(string)` contract as `<MDEditor>`.
- Live preview injects NO raw HTML from page content — only controlled mark/widget decorations. Preserves the stored-XSS guard (rehype-raw OFF equivalent).
- **Unify edit + read rendering:** read mode (`PageView`) and edit mode share a single live-preview rendering surface (read mode = a read-only CM6 live-preview). The unified renderer MUST preserve everything `MarkdownProse` provides: internal `.md` link SPA navigation (D-06), GitHub-style heading anchor ids for search deep-links (SRCH-06 / T-03-15), and sanitization (no raw HTML). `MarkdownProse`'s retire-vs-retain is resolved at plan time, bounded by these requirements.
- Remove `@uiw/react-md-editor` once the CM6 editor lands. `LinkPicker` stays wired into edit mode (relative `.md` insert, D-05).

### Claude's Discretion
- Exact CM6 extension composition, decoration CSS class names, and theme tokens.
- Whether read mode reuses `LivePreviewEditor` in a read-only configuration or a thin shared rendering module — resolve in planning, bounded by the deep-link / internal-nav / sanitize constraints.
- How inline-image and table widgets coexist with active-line marker reveal.

### Deferred Ideas (OUT OF SCOPE)
- Sibling Obsidian-feel items tracked separately: quick switcher (Ctrl-O), command palette (Ctrl-P), `[[wikilink]]` autocomplete, backlinks panel, denser file tree, dark theme.
- Live multi-user co-editing (CRDT→Git) stays Phase 5.
- DOCX/PDF in-browser editing remains out of scope.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| EDIT-01 | While editing, Markdown formatting (headings, bold/italic, lists, links, inline code, code blocks, inline images, GFM tables) renders inline as the user types | Pattern 1 (live-preview ViewPlugin) + Pattern 3 (image/table widgets); Lezer node-name map in Architecture Patterns |
| EDIT-02 | Toggle live-preview ⇄ raw-source; switching never alters underlying Markdown bytes | Pattern 2 (Compartment-swapped decoration set over one shared EditorState — zero serialization on toggle, byte-identical by construction); test strategy §"Byte-Stability Test Strategy" |
| EDIT-03 | Saving from live-preview produces byte-identical Markdown to the source-mode round-trip; okf golden-corpus gate holds | "Decorations are view-only" invariant — the CM6 doc IS the bytes; no AST re-emit. Backend golden corpus at `internal/okf/testdata/corpus/` already proves server round-trip; frontend asserts `view.state.doc.toString()` invariance |
| EDIT-04 | Existing guarantees preserved — autosave drafts, optimistic-concurrency save, sanitized rendering | Integration plan: swap only the `<MDEditor>` surface; preserve `runSaver`/refs/`baseRevision`/ConflictBanner; controlled-decorations-only XSS guard (Security Domain) |
</phase_requirements>

## Project Constraints (from CLAUDE.md)

- **Frontend stack is locked:** React 19.2.7, react-dom 19.2.7, Vite 8.0.16, TypeScript 6.0.3, `@vitejs/plugin-react` 6.0.2, Node 20.19+. New CM6 packages must be compatible with these (they are — CM6 is framework-agnostic, ships ESM + types, no peer-dep on React).
- **Note — CLAUDE.md prescribes a different editor path than CONTEXT:** CLAUDE.md's stack table recommends `@uiw/react-md-editor` (MVP) and lists `@uiw/react-codemirror` + `@codemirror/lang-markdown` as the alternative. **CONTEXT.md supersedes this** — it locks the *raw* CM6 package path with NO React wrapper. This research follows CONTEXT (the later, phase-specific decision). The planner should treat the CLAUDE.md editor row as superseded for Phase 6; consider updating the CLAUDE.md stack table after this phase ships.
- **Styling:** token-driven per-component CSS only — never hard-coded hex/px. The CM6 theme (`EditorView.theme(...)`) must reference `web/src/styles/tokens.css` CSS variables (per UI-SPEC). No shadcn, no component library.
- **Stored-XSS guard is a project-wide invariant** (CLAUDE.md "What NOT to Use": `rehype-raw` without sanitize is forbidden). The CM6 surface must never inject raw page HTML.
- **Single-binary deploy:** frontend builds to `internal/web/dist` and is `//go:embed`-ed. Pure client-side, no SSR — CM6 fits this cleanly (no SSR story needed).
- **GSD workflow enforcement:** edits go through a GSD command, not direct repo edits.

## Summary

This phase replaces the `@uiw/react-md-editor` split-pane surface with a hand-built CodeMirror 6 live-preview editor that renders Markdown formatting inline (Obsidian "Live Preview" feel) and unifies read + edit onto one rendering surface. The locked architecture is sound and matches a well-trodden path: a `ViewPlugin` walks the `@lezer/markdown` syntax tree over the visible ranges, emits a `DecorationSet` of `Decoration.mark` (styling), `Decoration.replace` (hide syntax markers), and `Decoration.replace({widget})` / `Decoration.widget` (images, tables), then *removes* the hide-decorations on any line/range the selection overlaps so markers reveal for editing. Because every decoration is **view-only**, `view.state.doc.toString()` is always the verbatim Markdown — the byte-stable round-trip (EDIT-03) and the zero-mutation toggle (EDIT-02) hold *by construction*, not by test luck.

The single hardest constraint is **unifying read mode while preserving GitHub-style heading deep-links** (SRCH-06 / T-03-15). Search results currently jump to `MarkdownProse`'s github-slugger DOM `id`s; a CM6 surface has no such DOM `id`s natively. The recommendation (MEDIUM confidence) is a **hybrid: retain `MarkdownProse` for read mode** and use `LivePreviewEditor` only for edit mode in v1 — OR, if true visual unification is required, add a `Decoration.line` that stamps `data-heading-id`/`id` attributes on heading lines plus a custom scroll-to-anchor effect. The hybrid is lower-risk and fully preserves all three `MarkdownProse` guarantees; the unified-CM read surface is achievable but carries the deep-link risk and more surface area. The planner should pick one explicitly with the user, because "unify read+edit" (a user override) and "preserve heading deep-links" (a shipped requirement) are in mild tension.

All six core packages are verified on the npm registry, authored by Marijn Haverbeke (CodeMirror's creator), with 2.7M–10.7M weekly downloads. Two (`@codemirror/view`, `@lezer/markdown`) trip the legitimacy gate's "too-new" heuristic *only* because they were republished within the lookback window — both are flagship CM6 packages, not slopsquats. The bundle impact is favorable: modular CM6 (~hundreds of KB, tree-shaken) replaces `@uiw/react-md-editor` (which bundles a full CodeMirror + its own preview pipeline), so the net change is roughly neutral-to-smaller while giving full control.

**Primary recommendation:** Build a `web/src/lib/cm/` extension set (live-preview ViewPlugin + theme + markdown-with-GFM language + image/table widgets + link-click handler), wrap it in `LivePreviewEditor.tsx` exposing `value`/`onChange`, swap it into `PageEditor.tsx` in place of `<MDEditor>` with **zero changes to the save machinery**, gate the Live/Source decoration set behind a `Compartment`, and **keep `MarkdownProse` for read mode in v1** (hybrid unification) unless the user explicitly accepts the heading-deep-link work to fully unify the read surface.

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Live-preview rendering (decorations) | Browser / Client (CM6 ViewPlugin) | — | Pure presentation over the in-memory doc; no server involvement. |
| Markdown parsing for decoration | Browser / Client (`@lezer/markdown`) | — | Parser runs in the editor only; it never re-emits bytes (round-trip is by construction). |
| Byte-stable storage / round-trip | API / Backend (`internal/okf`) | Browser (verbatim doc) | The okf golden corpus is the system-of-record gate; the client just ships the raw string back unchanged. |
| Image `src` sanitization | Browser / Client (widget guard) | API (attachment URL convention) | The widget mounts client-side, so the src must be validated client-side before `<img>` mounts. |
| Heading anchor ids / deep-link | Browser / Client (read renderer) | API (`okf.ScanHeadings` produces matching anchors) | Anchors must agree byte-for-byte between server (search results) and client (rendered ids). |
| Internal `.md` link navigation | Browser / Client (react-router) | — | SPA navigation via `resolveRelativeMdLink` → `navigate()`. |
| Save / optimistic concurrency | API / Backend (existing) | Browser (unchanged `runSaver`) | Untouched by this phase. |
| Mode preference persistence | Browser / Client (`localStorage` via zustand) | — | Per-device UI preference, never server state. |

## Standard Stack

### Core (packages to ADD)
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `@codemirror/state` | 6.6.0 | EditorState, Transaction, `Compartment`, `RangeSet` | Core CM6 state model. `Compartment` is the locked toggle mechanism. `[VERIFIED: npm registry]` |
| `@codemirror/view` | 6.43.1 | EditorView, `ViewPlugin`, `Decoration`, `WidgetType`, `EditorView.theme/baseTheme`, `EditorView.editable`, `EditorView.atomicRanges` | The decoration + view layer — the heart of the live-preview plugin. `[VERIFIED: npm registry]` (flagged "too-new" — false positive, see audit) |
| `@codemirror/commands` | 6.10.3 | `defaultKeymap`, `history`, base editing commands | Standard editing behavior + undo/redo. `[VERIFIED: npm registry]` |
| `@codemirror/language` | 6.12.3 | `syntaxTree`, `Language`, `LanguageSupport`, `syntaxHighlighting` | Provides `syntaxTree(view.state)` — the tree the ViewPlugin walks. `[VERIFIED: npm registry]` |
| `@codemirror/lang-markdown` | 6.5.0 | `markdown()`, `markdownLanguage`, `commonmarkLanguage` | The CM6 Markdown language integration; `markdownLanguage` includes GFM by default. `[VERIFIED: npm registry]` |
| `@lezer/markdown` | 1.6.4 | `Table`, `TaskList`, `Strikethrough`, `Autolink`, `GFM` bundle parser extensions | Source of the GFM table/strikethrough/tasklist parsing — passed to `markdown({ extensions: [...] })`. `[VERIFIED: npm registry]` (flagged "too-new" — false positive) |

### Supporting (transitive — likely auto-installed, pin if needed)
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `@lezer/common` | 1.5.2 | `SyntaxNode`/tree cursor types, `iterate` | Pulled in transitively by `@codemirror/language`; import its types when walking the tree. `[VERIFIED: npm registry]` |
| `@lezer/highlight` | 1.2.3 | `tags`, `HighlightStyle` for token classes | Optional — only if you use `syntaxHighlighting(HighlightStyle.define(...))` for Source-mode token colors. `[VERIFIED: npm registry]` |
| `@codemirror/search` | 6.7.1 | In-editor find (Cmd-F) | OPTIONAL, not required by any phase requirement. Skip in v1 unless desired. `[VERIFIED: npm registry]` |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Raw `@codemirror/*` packages | `codemirror` 6.0.2 meta-package | The meta-package bundles `basicSetup` (a curated extension set). CONTEXT locked the raw modular path for full control; `basicSetup` pulls extensions you may not want (line numbers, fold gutter). Use the raw packages and assemble your own minimal extension list. |
| Raw CM6 | `@uiw/react-codemirror` | CLAUDE.md's listed alternative, but CONTEXT explicitly **rejected** the React wrapper — it abstracts away the `EditorView` lifecycle and decoration plumbing you need. Do not use. |
| `markdown({ base: markdownLanguage })` | `markdown({ extensions: [GFM] })` over commonmark base | `markdownLanguage` already includes GFM + subscript/superscript/emoji. Using it as `base` is simplest. If you want *only* GFM tables/strikethrough (not emoji/sub/sup) for exact parity with the server's Goldmark GFM, use `markdown({ extensions: [Table, Strikethrough, TaskList, Autolink] })` over the default commonmark base. Recommendation: prefer the explicit GFM-only extension list for closest parity with read-mode remark-gfm + Goldmark. |

**Installation:**
```bash
cd web
npm install @codemirror/state@6.6.0 @codemirror/view@6.43.1 @codemirror/commands@6.10.3 \
  @codemirror/language@6.12.3 @codemirror/lang-markdown@6.5.0 @lezer/markdown@1.6.4
# @lezer/common and @lezer/highlight arrive transitively; pin explicitly only if a type import needs them:
# npm install @lezer/common@1.5.2 @lezer/highlight@1.2.3

# After the CM6 editor lands and PageEditor no longer imports MDEditor:
npm uninstall @uiw/react-md-editor
```

**Version verification:** All versions confirmed via `npm view <pkg> version` on 2026-06-21. Publish/modified dates: `@codemirror/view` 2026-06-09, `@codemirror/state` 2026-04-13, `@codemirror/language` 2026-04-13, `@codemirror/commands` 2026-04-13, `@codemirror/lang-markdown` 2026-04-13 (published 2025-10-23), `@lezer/markdown` 2026-05-28. CM6 uses a stable `6.x` line — minor bumps are non-breaking.

## Package Legitimacy Audit

> Run via `gsd-tools query package-legitimacy check --ecosystem npm ...` on 2026-06-21.

| Package | Registry | Weekly Downloads | Source Repo | Verdict | Disposition |
|---------|----------|------------------|-------------|---------|-------------|
| `@codemirror/state` | npm | 8,836,177 | github.com/codemirror/state | OK | Approved |
| `@codemirror/view` | npm | 9,125,512 | code.haverbeke.berlin/codemirror/view (mirror of github.com/codemirror/view) | SUS (`too-new`) | Approved — false positive (see note) |
| `@codemirror/commands` | npm | 8,327,380 | github.com/codemirror/commands | OK | Approved |
| `@codemirror/language` | npm | 8,729,552 | github.com/codemirror/language | OK | Approved |
| `@codemirror/lang-markdown` | npm | 2,676,280 | github.com/codemirror/lang-markdown | OK | Approved |
| `@lezer/markdown` | npm | 2,743,433 | code.haverbeke.berlin/lezer/markdown (mirror of github.com/lezer-parser/markdown) | SUS (`too-new`) | Approved — false positive (see note) |
| `@lezer/common` | npm | 10,736,561 | github.com/lezer-parser/common | OK | Approved |
| `@lezer/highlight` | npm | 8,714,440 | github.com/lezer-parser/highlight | OK | Approved |

**Packages removed due to [SLOP] verdict:** none.
**Packages flagged as suspicious [SUS]:** `@codemirror/view`, `@lezer/markdown` — **both are false positives.** The `SUS` verdict is driven solely by the `too-new` reason: each was *republished* within the gate's lookback window (`@codemirror/view` on 2026-06-09, `@lezer/markdown` on 2026-05-28). The signals contradict slopsquatting: 9.1M and 2.7M weekly downloads respectively, no postinstall script, `deprecated: false`, and both are authored by `marijn <marijn@haverbeke.berlin>` (Marijn Haverbeke, the original author of CodeMirror and Lezer). The repo URLs point at `code.haverbeke.berlin` (the author's own git host) which the gate could not resolve to a GitHub repo, contributing to the `too-new`/no-resolved-repo signal. **Recommendation for the planner:** no `checkpoint:human-verify` task is needed for these two — they are the canonical CM6 packages. If the planner's policy mandates a checkpoint for any `SUS` verdict, a single lightweight checkpoint noting "verify @codemirror/view and @lezer/markdown are the official Marijn Haverbeke packages (they are)" suffices.

**Postinstall check:** `npm view <pkg> scripts.postinstall` returned `null` for all eight packages — no install-time script execution.

## Architecture Patterns

### System Architecture Diagram

```
                         RAW MARKDOWN STRING  (system of record — never re-emitted)
                                  │
              ┌───────────────────┴────────────────────┐
              │                                          │
     ┌────────▼─────────┐                       ┌────────▼─────────┐
     │  EDIT MODE        │                       │  READ MODE        │
     │  PageEditor.tsx   │                       │  PageView.tsx     │
     │  (save machinery  │                       │                   │
     │   UNCHANGED)      │                       │                   │
     └────────┬─────────┘                       └────────┬─────────┘
              │ value / onChange(string)                  │ body
     ┌────────▼──────────────────────┐          ┌─────────▼─────────────────────┐
     │ LivePreviewEditor.tsx          │          │  v1 RECOMMENDED: keep         │
     │  (EditorView via useRef/effect)│          │  MarkdownProse (react-markdown │
     └────────┬──────────────────────┘          │  + slug + sanitize) — preserves│
              │                                   │  heading deep-link ids        │
   ┌──────────▼───────────────────────────┐      └───────────────────────────────┘
   │ web/src/lib/cm/  EXTENSION SET        │        (Alt: read-only LivePreviewEditor
   │                                       │         — see "Unified read+edit" risk)
   │  ┌─────────────────────────────────┐ │
   │  │ markdown({extensions:[GFM…]})   │ │  ← @lezer/markdown parse → syntaxTree
   │  ├─────────────────────────────────┤ │
   │  │ livePreview ViewPlugin          │ │  ← walks tree over visibleRanges:
   │  │   • Decoration.mark  (style)    │ │      builds DecorationSet
   │  │   • Decoration.replace (hide)   │ │      then FILTERS OUT hides that
   │  │   • Decoration.replace{widget}  │ │      overlap selection (reveal)
   │  │       img / table widgets       │ │
   │  ├─────────────────────────────────┤ │
   │  │ modeCompartment (Live | Source) │ │  ← toggle swaps ONLY this compartment
   │  ├─────────────────────────────────┤ │      (no doc serialization → bytes ==)
   │  │ theme (tokens.css vars)         │ │
   │  │ linkClick handler → navigate()  │ │
   │  │ keymap: Cmd/Ctrl-E toggle       │ │
   │  └─────────────────────────────────┘ │
   └───────────────────────────────────────┘
              │
   view.state.doc.toString() === raw bytes  (EDIT-02 / EDIT-03 hold by construction)
```

### Recommended Project Structure
```
web/src/
├── components/
│   ├── LivePreviewEditor.tsx      # React wrapper: EditorView lifecycle (useRef/useEffect), value/onChange bridge
│   ├── LivePreviewEditor.css      # per-component CSS (token vars only)
│   └── MarkdownProse.tsx          # KEEP (v1 read mode) — preserves heading ids + .md nav + sanitize
├── lib/cm/
│   ├── markdown.ts                # markdown({extensions:[Table,Strikethrough,TaskList,Autolink]}) language config
│   ├── livePreview.ts             # the ViewPlugin: tree walk → DecorationSet + active-line/selection reveal
│   ├── widgets.ts                 # ImageWidget, TableWidget WidgetType subclasses (+ src sanitizer)
│   ├── theme.ts                   # EditorView.theme(...) reading tokens.css variables
│   ├── linkNav.ts                 # click handler: intercept rendered .md links → resolveRelativeMdLink → navigate
│   ├── mode.ts                    # modeCompartment + Live/Source decoration-enable, Cmd/Ctrl-E keymap
│   └── sanitizeSrc.ts             # image-src allowlist (safe schemes + app/relative paths)
└── stores/
    └── editorMode.ts              # zustand+persist store, key "okf.editor.mode" (mirror stores/recent.ts)
```

### Pattern 1: Live-Preview ViewPlugin (the core)
**What:** A `ViewPlugin` that builds a `DecorationSet` by walking the Lezer markdown tree over `view.visibleRanges`, then reveals (drops) hide-decorations the selection overlaps.
**When to use:** This is the heart of EDIT-01. One plugin, recomputed on `docChanged || viewportChanged || selectionSet`.
**Example:**
```typescript
// Source: codemirror.net/examples/decoration + discuss.codemirror.net/t/hide-markdown-syntax/7602
import { ViewPlugin, ViewUpdate, DecorationSet, Decoration, EditorView } from "@codemirror/view";
import { syntaxTree } from "@codemirror/language";

const hideMark = Decoration.replace({});            // hide a syntax marker (zero-width)
const boldMark = Decoration.mark({ class: "cm-strong" });

export const livePreview = ViewPlugin.fromClass(
  class {
    decorations: DecorationSet;
    constructor(view: EditorView) { this.decorations = this.build(view); }

    update(u: ViewUpdate) {
      // selectionSet matters: marker reveal is selection-driven, not timer-driven.
      if (u.docChanged || u.viewportChanged || u.selectionSet)
        this.decorations = this.build(u.view);
    }

    build(view: EditorView): DecorationSet {
      const widgets: any[] = [];
      for (const { from, to } of view.visibleRanges) {
        syntaxTree(view.state).iterate({
          from, to,
          enter: (node) => {
            switch (node.name) {
              case "EmphasisMark":          // the * or _ around italics/bold marks
              case "HeaderMark":            // the leading "# " run
              case "CodeMark":              // ` or ``` fences
              case "LinkMark":              // [ ] ( ) brackets
                widgets.push(hideMark.range(node.from, node.to));
                break;
              case "StrongEmphasis":
                widgets.push(boldMark.range(node.from, node.to));
                break;
              // …Image / Table handled with widgets (Pattern 3)
            }
          },
        });
      }
      let set = Decoration.set(widgets, /* sort */ true);
      // REVEAL: drop any hide-decoration the selection overlaps so markers show for editing.
      for (const r of view.state.selection.ranges) {
        set = set.update({
          filter: (dFrom, dTo) => dTo < r.from || dFrom > r.to,
          filterFrom: r.from, filterTo: r.to,
        });
      }
      return set;
    }
  },
  { decorations: (v) => v.decorations },
);
```
> **Critical:** the `filter` step removes *only* the hide/replace decorations under the selection. To keep styling marks (bold/italic colour) on the active line while revealing their `*` markers, either (a) only apply the reveal filter to `replace`-type decorations (tag them and filter by tag), or (b) compute the active line up-front and skip emitting `hideMark` for nodes whose line equals the cursor line. Option (b) is cleaner and what most Obsidian-clones do.

### Pattern 2: Live/Source toggle via Compartment (EDIT-02)
**What:** One `EditorState`; a `Compartment` holds either the live-preview decorations or nothing (Source = raw). Toggling reconfigures the compartment with `view.dispatch({ effects: compartment.reconfigure(...) })` — **no document transaction**, so `doc.toString()` is byte-identical across the toggle.
**Example:**
```typescript
// Source: codemirror.net/examples/config (Compartment pattern)
import { Compartment } from "@codemirror/state";
import { keymap } from "@codemirror/view";

export const modeCompartment = new Compartment();

// Live: include the livePreview plugin + theme markers. Source: empty (raw text, mono).
export const liveExtensions = [livePreview /*, source-mode-off styling */];
export const sourceExtensions: [] = [];

export function setMode(view: EditorView, mode: "live" | "source") {
  view.dispatch({
    effects: modeCompartment.reconfigure(mode === "live" ? liveExtensions : sourceExtensions),
  });
  // NOTE: no `changes` in this dispatch → the document is untouched → bytes identical.
}

export const toggleKeymap = keymap.of([
  { key: "Mod-e", run: (view) => { /* read+flip zustand mode, call setMode */ return true; } },
]);
```
> `Mod-` in a CM6 keybinding maps to Cmd on macOS and Ctrl elsewhere — exactly the `Cmd/Ctrl-E` requirement, handled by CM6 natively.

### Pattern 3: Image & Table widgets (EDIT-01 v1 scope)
**What:** `Decoration.replace({ widget, block? })` swaps the markdown source span for a rendered widget via a `WidgetType` subclass.
**Example:**
```typescript
// Source: codemirror.net/examples/decoration (WidgetType)
import { WidgetType } from "@codemirror/view";
import { sanitizeImageSrc } from "./sanitizeSrc";

class ImageWidget extends WidgetType {
  constructor(readonly src: string, readonly alt: string, readonly raw: string) { super(); }
  eq(o: ImageWidget) { return o.src === this.src && o.alt === this.alt; }
  toDOM() {
    const safe = sanitizeImageSrc(this.src);
    if (safe == null) {                      // blocked src → fall back to RAW markdown text
      const span = document.createElement("span");
      span.textContent = this.raw;           // never imply the bytes changed
      return span;
    }
    const img = document.createElement("img");
    img.src = safe; img.alt = this.alt; img.className = "cm-md-image";
    return img;
  }
  ignoreEvent() { return false; }
}
```
**Active-line coexistence:** when the cursor is on the image/table line, DROP the replacing widget decoration (same selection-overlap filter) so the user sees and edits the raw `![alt](src)` / table source. Images are inline widgets; a GFM table is a multi-line block — use `Decoration.replace({ widget, block: true })` spanning the whole `Table` node range, and reveal (remove) it when the selection intersects any table line.

### Pattern 4: Internal `.md` link navigation in CM6 (D-06)
**What:** Rendered links are CM6 marks/widgets, not `<a onClick>`. Intercept clicks at the editor DOM level and route through react-router.
**Example:**
```typescript
// Reuse the existing resolveRelativeMdLink (web/src/lib/mdlink.ts)
import { EditorView } from "@codemirror/view";
import { resolveRelativeMdLink } from "../mdlink";

export function linkNav(currentPath: string, navigate: (to: string) => void) {
  return EditorView.domEventHandlers({
    mousedown(e, view) {
      const a = (e.target as HTMLElement).closest("a.cm-md-link") as HTMLAnchorElement | null;
      if (!a) return false;
      const target = resolveRelativeMdLink(currentPath, a.getAttribute("href") ?? undefined);
      if (target != null) { e.preventDefault(); navigate(`/app/page/${target}`); return true; }
      return false; // external link: let default happen
    },
  });
}
```
> Render link text as a `Decoration.mark` adding `class="cm-md-link"` and (in read mode / a widget) an `<a href>` carrying the original href so the handler can resolve it.

### Anti-Patterns to Avoid
- **Re-emitting the doc from the syntax tree.** Never call anything that serializes the Lezer tree back to text — that would reintroduce the lossy-block-model risk EDIT-03 forbids. Decorations are view-only; the doc string is the only source of bytes.
- **`atomicRanges` on hidden markers.** Making hide-`replace` ranges atomic causes backspace to delete the *entire* marked span (the `discuss.codemirror.net` thread documents this footgun). Prefer the selection-overlap reveal so the cursor is never trapped, and reserve `atomicRanges` only for block widgets (tables/images) where atomic deletion is actually desired.
- **Recomputing on a timer.** Marker reveal must be driven by `update.selectionSet`, not a debounce — otherwise reveal lags the cursor.
- **Injecting `innerHTML` from page content into a widget.** Build widget DOM with `document.createElement` + `textContent` / explicit attributes only. Never `el.innerHTML = userContent`.
- **Mutating the doc on toggle.** The toggle dispatch must carry `effects` only, never `changes`.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Markdown parsing for decoration | A regex/string scanner to find bold/headings/links | `@lezer/markdown` via `syntaxTree(view.state)` | Incremental, GFM-aware, correct around code fences/escapes; CM6 reparses incrementally on edit. A regex scanner re-corrupts on nested/edge constructs. |
| Hiding/revealing syntax markers | Manual DOM hide + scroll/measure | `Decoration.replace` + selection-overlap filter | CM6 handles position mapping through edits (`RangeSet.map`), viewport virtualization, and layout stability. |
| Mode toggle without reparse | Serializing to a second doc / two EditorStates | One `EditorState` + `Compartment.reconfigure` | Byte-identity by construction (EDIT-02); two states would require copying and risk divergence. |
| Undo/redo, default keybindings | Custom history stack | `history()` + `defaultKeymap` from `@codemirror/commands` | Battle-tested; integrates with transactions. |
| GFM tables/strikethrough/tasklists | Custom table tokenizer | `@lezer/markdown` `Table`/`Strikethrough`/`TaskList` extensions | Matches Goldmark GFM + remark-gfm semantics; hand-rolling diverges from server render. |
| `Cmd` vs `Ctrl` platform handling | `navigator.platform` sniffing | CM6 `Mod-e` keybinding | CM6 maps `Mod-` to the right modifier per-OS. |
| Image-src sanitization | Inline `if (src.startsWith("http"))` checks | A small dedicated allowlist module (still hand-written, but isolated + unit-tested) | One tested chokepoint; reuses the app's attachment-URL convention. (No npm dep needed — keep it minimal and audited.) |

**Key insight:** Almost everything that *looks* like custom work here is actually "compose CM6 primitives correctly." The only genuinely bespoke logic is (1) the node-name → decoration mapping, (2) the selection-overlap reveal, and (3) the src sanitizer. Keep those three small and unit-tested.

## Runtime State Inventory

> This is a UI-replacement phase (swap an editor surface), not a rename/migration. No stored data, service config, OS registration, secrets, or build-artifact renames are involved.

| Category | Items Found | Action Required |
|----------|-------------|------------------|
| Stored data | None — the okf raw-Markdown bytes are unchanged; no datastore key/collection is renamed. | None — verified: storage/sync explicitly OUT of scope (CONTEXT "Phase Boundary"). |
| Live service config | None — no external service config references the editor. | None. |
| OS-registered state | None. | None. |
| Secrets/env vars | None — no new secret or env var introduced. | None. |
| Build artifacts | `@uiw/react-md-editor` removed from `web/package.json` → `package-lock.json` regenerates; `internal/web/dist` rebuilds on next `vite build`. | Run `npm uninstall @uiw/react-md-editor` and rebuild the SPA; the embedded `dist` updates via the normal build. |

## Common Pitfalls

### Pitfall 1: Cursor trapped inside a replaced range
**What goes wrong:** Hidden markers via `Decoration.replace` + `atomicRanges` make backspace delete the whole span, or the cursor can't be placed to edit the marker.
**Why it happens:** Atomic replaced ranges are treated as single uneditable units; mark-based atomic ranges delete entirely on backspace.
**How to avoid:** Drive reveal by selection overlap (drop the hide-decoration when the selection touches its line/range); reserve `atomicRanges` for image/table block widgets only.
**Warning signs:** Backspace at end of a bold word deletes the whole word; cursor jumps past where a `*` should be.

### Pitfall 2: Layout reflow when markers reveal
**What goes wrong:** Lines shift vertically as the cursor enters/leaves a line (markers appearing/disappearing change width/height).
**Why it happens:** Replacing `**` with zero-width and back changes content width; if styling also changes line-height, neighbors reflow.
**How to avoid:** Keep revealed and hidden states layout-neutral — hide markers with `Decoration.replace({})` (zero-width) and ensure `cm-strong`/`cm-em` marks don't change line-height (UI-SPEC: bold = weight only, no size change on body text). The UI-SPEC explicitly makes this a hard constraint.
**Warning signs:** Visible "jump" of text below the active line on cursor move.

### Pitfall 3: Heading deep-links break under unified read mode
**What goes wrong:** Search results (`#anchor` deep-links, SRCH-06) land nowhere because a CM6 read surface has no DOM `id` on headings.
**Why it happens:** `MarkdownProse` produces github-slugger `id`s; CM6 renders lines, not anchored headings.
**How to avoid:** v1 — keep `MarkdownProse` for read mode (hybrid). If unifying, add a `Decoration.line({ attributes: { id: slug } })` on heading lines computed with the *same* slug algorithm as `okf.ScanHeadings` (lowercase, strip non-letter/number/`-`/`_`/space, space→`-`, `-N` dedup), plus a scroll-to-`#hash` effect on mount.
**Warning signs:** Clicking a heading search result scrolls to top / does nothing.

### Pitfall 4: GFM render divergence (editor vs server vs read view)
**What goes wrong:** A table or strikethrough renders in the live-preview editor but not in `MarkdownProse` (or vice-versa), or vice-versa with the server's Goldmark output.
**Why it happens:** `markdownLanguage` includes emoji/sub/sup that Goldmark + remark-gfm don't; or commonmark base without the Table extension parses a table as plain text.
**How to avoid:** Configure `markdown({ extensions: [Table, Strikethrough, TaskList, Autolink] })` (GFM-only) over the commonmark base for closest parity with the server (Goldmark GFM) and read view (remark-gfm). Avoid the emoji/sub/sup superset unless the server also supports it.
**Warning signs:** `:smile:` renders as an emoji in the editor but stays literal on read/server.

### Pitfall 5: React StrictMode double-mounts the EditorView
**What goes wrong:** In dev, `useEffect` runs twice; two `EditorView`s attach to the same DOM node, or the second leaks.
**Why it happens:** React 19 StrictMode mounts→unmounts→remounts effects.
**How to avoid:** Create the `EditorView` in `useEffect` and **always** `view.destroy()` in the cleanup; guard against re-creation; sync external `value` changes via `view.dispatch` (compare against `view.state.doc.toString()` to avoid feedback loops with `onChange`).
**Warning signs:** Duplicate editors in dev, doubled `onChange` fires, memory growth.

### Pitfall 6: onChange ⇄ controlled-value feedback loop
**What goes wrong:** `onChange` updates React state → re-render pushes `value` back into the editor → cursor resets / infinite churn.
**Why it happens:** Treating CM6 as a fully controlled input.
**How to avoid:** Make CM6 the source of truth while focused. Fire `onChange` from an `updateListener` only on `docChanged`. When the incoming `value` prop differs from `view.state.doc.toString()`, dispatch a replace transaction *only then* (e.g. external seed/reset). PageEditor already seeds once and reads via refs — this matches cleanly.
**Warning signs:** Cursor jumps to start on every keystroke.

## Code Examples

### Lezer markdown node-name map (what the tree calls things)
```
// Source: github.com/lezer-parser/markdown (node names)
ATXHeading1..ATXHeading6   // heading lines; child HeaderMark = the "# " run
HeaderMark                 // leading "#"s (hide these)
StrongEmphasis             // **bold** ; child EmphasisMark = the "**"
Emphasis                   // *italic* ; child EmphasisMark = the "*"
EmphasisMark               // the * / _ marker runs (hide)
InlineCode                 // `code` ; child CodeMark = the backticks
FencedCode                 // ```fenced``` ; children CodeMark, CodeInfo, CodeText
CodeMark                   // ` or ``` (hide on inline; keep block fences styled)
Link                       // [text](url) ; children LinkMark, URL, (LinkLabel for refs)
LinkMark                   // [ ] ( ) brackets (hide), URL (hide on inactive line)
Image                      // ![alt](src) → replace with ImageWidget
Table, TableHeader, TableRow, TableCell, TableDelimiter  // GFM table (Table ext)
BulletList, OrderedList, ListItem, ListMark  // ListMark = bullet/number marker
Strikethrough              // ~~x~~ (Strikethrough ext)
Task / TaskMarker          // [ ] / [x] (TaskList ext)
```

### React wrapper skeleton (value/onChange bridge — the MDEditor drop-in)
```typescript
// LivePreviewEditor.tsx — same contract as <MDEditor value onChange height>
import { useEffect, useRef } from "react";
import { EditorState, Compartment } from "@codemirror/state";
import { EditorView, keymap, placeholder } from "@codemirror/view";
import { defaultKeymap, history, historyKeymap } from "@codemirror/commands";
// ... markdown(), livePreview, theme, modeCompartment, linkNav from web/src/lib/cm/

export default function LivePreviewEditor({
  value, onChange, currentPath, mode,
}: { value: string; onChange: (v: string) => void; currentPath: string; mode: "live" | "source"; }) {
  const host = useRef<HTMLDivElement>(null);
  const view = useRef<EditorView | null>(null);

  useEffect(() => {
    const v = new EditorView({
      parent: host.current!,
      state: EditorState.create({
        doc: value,
        extensions: [
          history(), keymap.of([...defaultKeymap, ...historyKeymap]),
          /* markdown(...), theme, linkNav(...), toggleKeymap, */
          modeCompartment.of(mode === "live" ? liveExtensions : sourceExtensions),
          placeholder("Start writing in Markdown…"),
          EditorView.updateListener.of((u) => {
            if (u.docChanged) onChange(u.state.doc.toString()); // bytes verbatim
          }),
        ],
      }),
    });
    view.current = v;
    return () => v.destroy(); // StrictMode-safe cleanup
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // external value sync (seed/reset only — avoid feedback loop)
  useEffect(() => {
    const v = view.current; if (!v) return;
    const cur = v.state.doc.toString();
    if (value !== cur) v.dispatch({ changes: { from: 0, to: cur.length, insert: value } });
  }, [value]);

  // mode toggle without touching the doc
  useEffect(() => {
    const v = view.current; if (!v) return;
    v.dispatch({ effects: modeCompartment.reconfigure(mode === "live" ? liveExtensions : sourceExtensions) });
  }, [mode]);

  return <div ref={host} className="livepreview-editor" />;
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `@uiw/react-md-editor` split-pane (textarea + separate preview) | Single CM6 surface with inline live-preview decorations | This phase | True Obsidian feel; no separate preview pane; smaller, fully-controlled bundle. |
| CodeMirror 5 (monolithic, `.mode` system) | CodeMirror 6 (modular `@codemirror/*`, Lezer incremental parser, decoration facets) | CM6 GA (2022), now stable 6.x | Tree-sitter-style incremental parsing enables cheap per-edit decoration recompute. |
| `atomicRanges`-based marker hiding | selection-overlap filter reveal (no atomic on hides) | community-established (Obsidian-clones) | Avoids the cursor-trap/backspace-eats-span footgun. |

**Deprecated/outdated:**
- CodeMirror 5 APIs (`CodeMirror.fromTextArea`, `.mode`): not applicable — CM6 only.
- `@uiw/react-md-editor`: to be removed this phase.

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | `@lezer/markdown` node names (`HeaderMark`, `EmphasisMark`, `StrongEmphasis`, `LinkMark`, `Image`, `Table`, `CodeMark`, `ListMark`, etc.) are exactly as listed | Code Examples / Pattern 1 | LOW–MEDIUM — names verified against the official lezer-parser/markdown docs, but a v1.6.x minor could rename a node. Mitigation: log the tree once at dev time (`syntaxTree(state).iterate({enter: n => console.log(n.name)})`) before wiring decorations. `[CITED: github.com/lezer-parser/markdown]` |
| A2 | `markdownLanguage` includes GFM by default; GFM-only parity is achieved via `markdown({ extensions: [Table, Strikethrough, TaskList, Autolink] })` | Standard Stack / Pitfall 4 | LOW — confirmed via official lang-markdown README, but exact import surface for the GFM bundle vs individual extensions should be confirmed at build time. `[CITED: codemirror/lang-markdown README]` |
| A3 | Net bundle size is neutral-to-smaller after the swap | Summary / Bundle section | LOW — directionally certain (modular tree-shaken CM6 vs a bundled full editor), but exact KB unmeasured. Mitigation: measure `vite build` output before/after. `[ASSUMED]` |
| A4 | Keeping `MarkdownProse` for read mode (hybrid) satisfies the "unify read+edit" user override "well enough" for v1 | Open Questions Q1 | MEDIUM — this is a UX/scope judgment the user must confirm; the override says *unify*, the requirement says *preserve deep-links*. **Planner must surface this to the user.** `[ASSUMED]` |
| A5 | The okf golden corpus (`internal/okf/testdata/corpus/`, 8 fixtures incl. CRLF, no-trailing-newline, quirky frontmatter, table/links/images) is the authoritative EDIT-03 gate and is unaffected by a frontend-only change | Byte-Stability Test Strategy | LOW — verified by reading `roundtrip_test.go`; the gate is backend-only and stays green because the client ships bytes verbatim. `[VERIFIED: codebase grep]` |

## Open Questions

1. **Read-mode unification: hybrid (keep MarkdownProse) vs fully-unified CM6 read surface?**
   - What we know: The user override says read + edit should "look identical" (one live-preview surface). The shipped requirement (SRCH-06 / T-03-15) needs github-slugger DOM `id`s on headings for search deep-links, which `MarkdownProse` provides today via `rehypeSlug` + the `headingIdSchema`. A read-only CM6 view (`EditorView.editable.of(false)` / `EditorState.readOnly.of(true)`) is technically the right vehicle for a unified surface, but it has no native heading DOM ids.
   - What's unclear: whether the user values *visual unification* over the (working, tested) deep-link path enough to fund the extra CM6 work.
   - Recommendation: **v1 = hybrid.** Keep `MarkdownProse` for read mode; ship `LivePreviewEditor` for edit mode only. This preserves all three `MarkdownProse` guarantees with zero risk and still delivers the headline EDIT-01..EDIT-04. If the user insists on a fully-unified read surface, scope a follow-up task: read-only `LivePreviewEditor` + `Decoration.line({attributes:{id}})` heading anchors (slug algorithm mirrored from `okf.ScanHeadings`) + a scroll-to-`#hash`-on-mount effect, and add a vitest asserting the rendered heading id equals `slug(text)` for the corpus headings. **The planner MUST raise this with the user before locking the read-mode approach.**

2. **GFM table reveal granularity.**
   - What we know: a table is a multi-line `Table` block; the grid widget should reveal to raw source when edited.
   - What's unclear: reveal the *whole* table or just the edited row when the cursor is inside.
   - Recommendation: v1 — reveal the whole table (simplest, matches Obsidian's "edit the source block" behavior). Refine later if desired.

3. **List bullets — hide or keep?**
   - What we know: Obsidian keeps list bullets visible (renders `-`/`1.` as a styled bullet, doesn't hide them).
   - Recommendation: Do NOT hide `ListMark` in v1 — style it (or leave it). Hiding list markers is the most reflow-prone and least-expected behavior. Keep scope to the success-criteria set.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Node | `npm install`, `vite build`, `vitest` | ✓ (env states Node 20.19.6; Vite 8 needs 20.19+) | 20.19.6 | — |
| npm registry access | adding CM6 packages | ✓ (verified via `npm view`) | — | — |
| Vite 8 / TS 6 / React 19 | building the SPA | ✓ (already in `web/package.json`) | 8.0.16 / 6.0.3 / 19.2.7 | — |
| vitest + jsdom | byte-stability + component tests | ✓ (`vitest` 3.2.4, `jsdom` 26.1.0, `@testing-library/react` 16.3.0 present) | — | — |

**Missing dependencies with no fallback:** none.
**Missing dependencies with fallback:** none. All tooling is already present; this phase only adds runtime npm packages.

> Note: CM6 renders to real DOM. Some `view.dispatch`/layout paths assume a layout engine jsdom doesn't fully provide. For tests asserting `doc.toString()` (the byte-stability tests), construct `EditorState` (not a mounted `EditorView`) where possible — state-level tests don't need a DOM and are fully reliable under jsdom. Reserve mounted-`EditorView` tests for interaction smoke tests and tolerate jsdom layout gaps.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework (frontend) | vitest 3.2.4 + jsdom 26.1.0 + @testing-library/react 16.3.0 |
| Framework (backend) | Go `testing` (the okf golden corpus gate) |
| Config file | `web/vitest.config.ts` (jsdom env, `src/test/setup.ts`, globals) |
| Quick run command (frontend) | `cd web && npx vitest run src/lib/cm src/components/LivePreviewEditor.test.tsx` |
| Full suite command (frontend) | `cd web && npm test` (`vitest run`) |
| Backend round-trip gate | `go test ./internal/okf/ -run TestGoldenRoundTrip` |

### Phase Requirements → Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| EDIT-01 | live-preview decorations render for each construct | unit (state+jsdom) | `cd web && npx vitest run src/lib/cm/livePreview.test.ts` | ❌ Wave 0 |
| EDIT-02 | toggling Live⇄Source never mutates `doc.toString()` | unit (EditorState) | `cd web && npx vitest run src/lib/cm/mode.test.ts` | ❌ Wave 0 |
| EDIT-03 | type→save cycle ships verbatim bytes; backend corpus stays green | unit + go | `cd web && npx vitest run src/components/LivePreviewEditor.test.tsx` && `go test ./internal/okf/ -run TestGoldenRoundTrip` | ❌ Wave 0 (frontend) / ✅ (backend gate exists) |
| EDIT-04 | save machinery untouched; sanitize (no raw HTML); image-src allowlist | unit | `cd web && npx vitest run src/lib/cm/sanitizeSrc.test.ts src/routes/PageEditor.test.tsx` | ⚠️ PageEditor.test.tsx exists (must be updated for the new surface); sanitizeSrc test ❌ Wave 0 |

### Sampling Rate
- **Per task commit:** `cd web && npx vitest run <touched cm test>` (sub-30s).
- **Per wave merge:** `cd web && npm test` + `go test ./internal/okf/...`.
- **Phase gate:** full `npm test` green AND `go test ./internal/okf/ -run TestGoldenRoundTrip` green before `/gsd-verify-work`.

### Wave 0 Gaps
- [ ] `web/src/lib/cm/mode.test.ts` — EDIT-02: build an `EditorState` with a known doc, toggle the mode compartment, assert `state.doc.toString()` identical before/after (state-level, no DOM). Drive it over each corpus fixture string for breadth.
- [ ] `web/src/lib/cm/livePreview.test.ts` — EDIT-01: assert the ViewPlugin emits the expected decoration kinds for bold/heading/link/code/image/table inputs (mount a minimal `EditorView` in jsdom; assert decoration ranges/classes or rendered DOM).
- [ ] `web/src/lib/cm/sanitizeSrc.test.ts` — EDIT-04: assert `javascript:`, `data:` (exec), and path-escape srcs return null/blocked; safe `http(s)`/app-relative pass.
- [ ] `web/src/components/LivePreviewEditor.test.tsx` — EDIT-03/04: type into the editor, assert `onChange` receives verbatim bytes; assert value/onChange contract parity with the old MDEditor.
- [ ] **Update** `web/src/routes/PageEditor.test.tsx` — it currently asserts against `<MDEditor>`; retarget to `LivePreviewEditor` while preserving the autosave/conflict assertions.
- [ ] Optional shared corpus import: load `internal/okf/testdata/corpus/*.md` into the frontend test (or copy the fixture strings) so EDIT-02/03 tests exercise the same byte-stability fixtures the backend gate uses.

## Security Domain

> `security_enforcement: true`, `security_asvs_level: 1`, `security_block_on: high` (from `.planning/config.json`).

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no | unchanged this phase (Phase 0 owns auth). |
| V3 Session Management | no | unchanged. |
| V4 Access Control | no | edit affordance already gated by role in PageView (`canEdit`); unchanged. |
| V5 Input Validation / Output Encoding | **yes** | (a) image-`src` allowlist before `<img>` mounts; (b) widget DOM built via `createElement`+`textContent`/explicit attrs — never `innerHTML`; (c) no raw page HTML rendered (controlled decorations only). This is the load-bearing control for EDIT-04. |
| V6 Cryptography | no | none. |

### Known Threat Patterns for {CM6 widget rendering of untrusted Markdown}

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Stored XSS via `![alt](javascript:alert(1))` image src | Tampering / Elevation | `sanitizeImageSrc` allowlist: permit only `http:`/`https:` and app-relative/attachment paths; reject `javascript:`, `vbscript:`, and executable `data:` schemes. Blocked src falls back to rendering the raw markdown text (never executes). |
| Stored XSS via raw HTML block in page content rendered into a widget | Tampering | Never `innerHTML` page content into a widget; raw-HTML blocks stay styled-as-text (rehype-raw-OFF equivalent). Mirrors the existing `MarkdownProse` guard. |
| Stored XSS via `[text](javascript:...)` link in a rendered link mark/widget | Tampering | Apply the same scheme allowlist to link hrefs; the `linkNav` handler only navigates internal `.md` targets and lets safe external schemes through — never executes a `javascript:` href. Reuse `resolveRelativeMdLink`'s existing external-scheme gate. |
| `data:` image exfil/abuse | Information Disclosure | Reject `data:` for images by default (CONTEXT lists data-exec vectors as out); only allow if a future requirement explicitly needs inline data images. |
| Open-redirect / path escape via crafted relative `.md` link | Tampering | `resolveRelativeMdLink` already clamps `..` at the workspace root (verified in `mdlink.ts`); reuse it unchanged. |

## Sources

### Primary (HIGH confidence)
- npm registry (`npm view <pkg> version` / `time.modified` / `maintainers` / `repository.url` / `scripts.postinstall`, 2026-06-21) — all 8 CM6/Lezer package versions, authorship (Marijn Haverbeke), no postinstall scripts.
- `gsd-tools query package-legitimacy check --ecosystem npm ...` — download counts + verdicts.
- codemirror.net/examples/decoration — `ViewPlugin`/`DecorationSet`/`Decoration.mark|replace|widget|line`, `WidgetType` (`toDOM`/`eq`/`ignoreEvent`), `atomicRanges`, `provide`, mapping through transactions.
- codemirror/lang-markdown README (raw.githubusercontent.com) — `markdown(config)` signature + `base`/`extensions`/`codeLanguages`/`addKeymap`, `markdownLanguage`/`commonmarkLanguage`.
- github.com/lezer-parser/markdown — GFM extensions (`Table`, `TaskList`, `Strikethrough`, `Autolink`, `GFM` bundle) + `configure()`.
- Codebase (read directly): `PageEditor.tsx`, `MarkdownProse.tsx`, `PageView.tsx`, `mdlink.ts`, `frontmatter.ts`, `LinkPicker.tsx`, `stores/recent.ts`, `internal/okf/headings.go`, `internal/okf/roundtrip_test.go`, `internal/okf/testdata/corpus/`, `web/vitest.config.ts`, `web/package.json`, `.planning/config.json`.

### Secondary (MEDIUM confidence)
- discuss.codemirror.net/t/hide-markdown-syntax/7602 — recommended marker-hide approach + the `atomicRanges`-backspace footgun + selection-overlap filter pattern.
- github.com/kenforthewin/atomic-editor — "CodeMirror 6 markdown editor with Obsidian-style inline live preview"; confirms the view-only-decorations + selection-overlap-reveal + stable-line-height architecture and dependency set.

### Tertiary (LOW confidence)
- WebSearch result summaries (Obsidian-clone references: nothingislost/obsidian-cm6-attributes, obsidian forum) — corroborate the general approach; not independently version-verified.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — all versions verified on npm; packages authored by CodeMirror's creator; stable 6.x line.
- Architecture (ViewPlugin/decorations/Compartment/widgets): HIGH — patterns from official CodeMirror examples + a directly-matching reference implementation.
- Read-mode unification strategy: MEDIUM — technically clear, but the hybrid-vs-fully-unified call is a user UX/scope decision (A4) that the planner must surface.
- Lezer node names: MEDIUM — cited from official docs; confirm by logging the tree once at dev time before wiring decorations.
- Pitfalls / security: HIGH — drawn from official docs, the CM6 forum, and the existing codebase's own XSS/round-trip invariants.

**Research date:** 2026-06-21
**Valid until:** 2026-07-21 (CM6 is stable but actively released — re-check `@codemirror/view` / `@lezer/markdown` minors at build time; both moved within the last month).
