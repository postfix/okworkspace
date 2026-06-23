// livePreview — the EDIT-01 core. A ViewPlugin that walks the Lezer markdown
// syntax tree over the editor's visible ranges and emits a DecorationSet that
// renders Markdown inline (Obsidian "Live Preview" feel):
//
//   • Decoration.mark  → styling: cm-strong (bold), cm-em (italic),
//                        cm-inline-code, cm-code-block, cm-md-link.
//   • Decoration.line  → per-level heading style (cm-heading-1..6) on the line.
//   • Decoration.replace({}) → ZERO-WIDTH hides for the syntax markers
//                        (** , * , the leading "# ", inline backticks, link
//                        brackets/URL) so they disappear in Live mode.
//
// ACTIVE-LINE REVEAL (RESEARCH option (b)): rather than emit every hide and then
// filter the ones under the selection, we compute the set of lines the selection
// touches up-front and simply SKIP emitting a hide for any marker on an active
// line. The styling marks are always emitted (so a revealed line keeps its
// bold/italic/heading look while showing its raw markers). This is cleaner than a
// post-filter and avoids the cursor-trap footgun.
//
// INVARIANTS (security + correctness):
//   • Decorations are VIEW-ONLY. The plugin issues no document transactions and
//     never serializes the tree back to text — `doc.toString()` is always the
//     verbatim Markdown (EDIT-02 / EDIT-03 hold by construction). T-06-04/T-06-05.
//   • NO atomicRanges on the hides (Anti-Pattern / Pitfall 1 — cursor trap).
//   • Hides are zero-width Decoration.replace({}) and the marks never change
//     line-height, so reveal is LAYOUT-NEUTRAL (Pitfall 2).
//   • Reveal is selection-driven (recompute on selectionSet), never timed.
//
// Image/table WIDGET decorations (06-03):
//   • Image  → Decoration.replace({ widget: ImageWidget })             (inline,
//              from the ViewPlugin — single-line replace, allowed from a plugin)
//   • Table  → Decoration.replace({ widget: TableWidget, block: true }) (BLOCK,
//              from a StateField — CM6 disallows block / line-break-spanning
//              replace decorations from a ViewPlugin's `decorations` provider, so
//              the multi-line table widget lives in `tableField` below)
// Both DROP their widget (revealing the raw Markdown source for editing) when the
// selection intersects the widget's line(s) — the SAME active-line reveal used for
// the text-construct hides. atomicRanges is applied to the block table / inline
// image widgets ONLY (atomic deletion is intentional there); the inline text-marker
// hides stay NON-atomic (Pitfall 1 — never trap the cursor on a marker run).
//
// Rendered links additionally carry a `data-href` attribute (the original Markdown
// href) so the 06-03 linkNav DOM handler can resolve+route internal `.md` clicks
// through react-router (D-06). The href is stored as DATA, never as an executed
// action — the scheme allowlist lives in resolveRelativeMdLink (reused by linkNav).
import {
  Decoration,
  ViewPlugin,
  EditorView,
} from "@codemirror/view";
import type { DecorationSet, ViewUpdate } from "@codemirror/view";
import { syntaxTree } from "@codemirror/language";
import { StateField } from "@codemirror/state";
import type { Extension, Range } from "@codemirror/state";
import type { EditorState } from "@codemirror/state";
import type { SyntaxNode } from "@lezer/common";

import { ImageWidget, TableWidget } from "./widgets";
import type { TableData } from "./widgets";

// hideMark hides a syntax marker. Zero-width + NON-atomic (no atomicRanges) so the
// cursor is never trapped and revealing it does not reflow neighbors (Pitfall 1/2).
const hideMark = Decoration.replace({});

// Inline styling marks — class-only so the theme (theme.ts) owns all sizing; the
// marks themselves never set width/height (layout-neutral reveal, Pitfall 2).
const strongMark = Decoration.mark({ class: "cm-strong" });
const emMark = Decoration.mark({ class: "cm-em" });
const inlineCodeMark = Decoration.mark({ class: "cm-inline-code" });
const codeBlockMark = Decoration.mark({ class: "cm-code-block" });
// (link marks are built per-node so each carries its own data-href — see the Link
// case in buildDecorations.)

// Per-level heading LINE decorations (cm-heading-1..6); the leading HeaderMark is
// hidden separately so "# " disappears while the line renders at heading size.
const headingLine = [1, 2, 3, 4, 5, 6].map((n) =>
  Decoration.line({ class: `cm-heading-${n}` }),
);

// ATXHeading1..6 → its level (1..6); anything else → 0.
function headingLevel(name: string): number {
  const m = /^ATXHeading([1-6])$/.exec(name);
  return m ? Number(m[1]) : 0;
}

// parseImage extracts (src, alt) from an `Image` node (`![alt](src)`). The Lezer
// `Image` carries a child `URL` (the src) and the alt text is the doc slice between
// the opening `![` LinkMark and the closing `]` LinkMark. Both are read from the
// doc bytes — no innerHTML, no tree-to-text re-serialization of the whole node.
function parseImage(
  doc: { sliceString(from: number, to: number): string },
  node: SyntaxNode,
): { src: string; alt: string } {
  let src = "";
  let altFrom = -1;
  let altTo = -1;
  for (let c = node.firstChild; c; c = c.nextSibling) {
    if (c.name === "URL") {
      src = doc.sliceString(c.from, c.to);
    } else if (c.name === "LinkMark") {
      const mark = doc.sliceString(c.from, c.to);
      if (mark === "![") altFrom = c.to;
      else if (mark === "]" && altTo === -1) altTo = c.from;
    }
  }
  const alt =
    altFrom >= 0 && altTo >= altFrom ? doc.sliceString(altFrom, altTo) : "";
  return { src, alt };
}

// parseTable walks a `Table` node into header + body cell texts. The Lezer GFM
// grammar nests cells as `TableCell` under a `TableHeader` (the header row) and
// under each `TableRow` (body rows); the `|---|` delimiter row is a bare
// `TableDelimiter` with no cells (correctly contributing nothing). Cell text is the
// raw doc slice (trimmed) — set later via textContent in the widget (never HTML).
function parseTable(
  doc: { sliceString(from: number, to: number): string },
  node: SyntaxNode,
): TableData {
  const header: string[] = [];
  const rows: string[][] = [];
  const cellsOf = (row: SyntaxNode): string[] => {
    const cells: string[] = [];
    for (let c = row.firstChild; c; c = c.nextSibling) {
      if (c.name === "TableCell") {
        cells.push(doc.sliceString(c.from, c.to).trim());
      }
    }
    return cells;
  };
  for (let c = node.firstChild; c; c = c.nextSibling) {
    if (c.name === "TableHeader") {
      header.push(...cellsOf(c));
    } else if (c.name === "TableRow") {
      rows.push(cellsOf(c));
    }
  }
  return { header, rows };
}

// BuildResult carries the view decorations AND the widget-only atomic ranges
// (kept separate so atomicRanges covers ONLY the block/inline widgets, never the
// non-atomic text-marker hides — Pitfall 1).
interface BuildResult {
  decorations: DecorationSet;
  atomic: DecorationSet;
}

function buildDecorations(view: EditorView): BuildResult {
  // Lines the selection touches → reveal (skip hides) on those lines.
  const activeLines = new Set<number>();
  for (const r of view.state.selection.ranges) {
    const fromLine = view.state.doc.lineAt(r.from).number;
    const toLine = view.state.doc.lineAt(r.to).number;
    for (let ln = fromLine; ln <= toLine; ln++) activeLines.add(ln);
  }
  const isActive = (pos: number) =>
    activeLines.has(view.state.doc.lineAt(pos).number);
  // rangeActive: does the selection touch ANY line the [from,to) range spans? Used
  // by the multi-line block table widget (and inline image) to reveal-to-source.
  const rangeActive = (from: number, to: number) => {
    const first = view.state.doc.lineAt(from).number;
    const last = view.state.doc.lineAt(to).number;
    for (let ln = first; ln <= last; ln++) {
      if (activeLines.has(ln)) return true;
    }
    return false;
  };

  // Collect marks/lines and hides separately so we can sort lines before marks
  // (CM6 requires line decorations sorted ahead of inline decorations at the same
  // position — Decoration.set(..., true) sorts by from/startSide for us, but line
  // decos must sit at the line start).
  const deco: Range<Decoration>[] = [];
  // Ranges occupied by a mounted block/inline WIDGET (image/table). These are the
  // ONLY ranges made atomic (atomic deletion of a rendered widget is intentional);
  // the text-marker hides are NEVER atomic (Pitfall 1). Collected here and handed to
  // the plugin's atomicRanges provider.
  const widgetRanges: Range<Decoration>[] = [];

  // Pushes a zero-width hide UNLESS the marker sits on an active (revealed) line.
  const pushHide = (from: number, to: number) => {
    if (from >= to) return;
    if (isActive(from)) return; // reveal: show the raw marker for editing
    deco.push(hideMark.range(from, to));
  };

  for (const { from, to } of view.visibleRanges) {
    syntaxTree(view.state).iterate({
      from,
      to,
      enter: (node) => {
        const level = headingLevel(node.name);
        if (level > 0) {
          // Stamp the heading-level class on the line; hide its leading "# " run.
          const line = view.state.doc.lineAt(node.from);
          deco.push(headingLine[level - 1].range(line.from));
          return;
        }
        switch (node.name) {
          case "StrongEmphasis":
            deco.push(strongMark.range(node.from, node.to));
            break;
          case "Emphasis":
            deco.push(emMark.range(node.from, node.to));
            break;
          case "InlineCode":
            deco.push(inlineCodeMark.range(node.from, node.to));
            break;
          case "FencedCode":
            // Style the block; its fence CodeMarks are KEPT (not hidden).
            deco.push(codeBlockMark.range(node.from, node.to));
            break;
          case "Link": {
            // Carry the original href as a data-attribute so the linkNav DOM handler
            // (06-03) can resolve+route internal `.md` clicks. The href is DATA, not
            // an executed action — resolveRelativeMdLink owns the scheme allowlist.
            let href = "";
            for (
              let c = node.node.firstChild;
              c;
              c = c.nextSibling
            ) {
              if (c.name === "URL") {
                href = view.state.doc.sliceString(c.from, c.to);
                break;
              }
            }
            deco.push(
              Decoration.mark({
                class: "cm-md-link",
                attributes: href ? { "data-href": href } : undefined,
              }).range(node.from, node.to),
            );
            break;
          }
          case "Image": {
            // Replace the whole `![alt](src)` span with an ImageWidget — UNLESS the
            // selection is on the image line, in which case DROP the widget so the
            // raw Markdown shows for editing (active-line reveal). Skip descending
            // into the Image children either way (their LinkMark/URL belong to the
            // widget, not to standalone hides).
            if (!rangeActive(node.from, node.to)) {
              const { src, alt } = parseImage(view.state.doc, node.node);
              const raw = view.state.doc.sliceString(node.from, node.to);
              const widget = Decoration.replace({
                widget: new ImageWidget(src, alt, raw),
              }).range(node.from, node.to);
              deco.push(widget);
              widgetRanges.push(widget);
            }
            return false; // never style/hide the image's inner markers
          }
          case "Table":
            // The block table widget is produced by `tableField` (a StateField) —
            // CM6 forbids block decorations from a ViewPlugin. Skip the subtree here
            // so the table's inner delimiters/cells get no stray hides.
            return false;
          case "HeaderMark":
            // The leading "#"s run (and any trailing space the parser folds in):
            // hide it plus the single following space so the heading reads clean.
            pushHide(
              node.from,
              Math.min(node.to + 1, view.state.doc.lineAt(node.from).to),
            );
            break;
          case "EmphasisMark":
          case "LinkMark":
            pushHide(node.from, node.to);
            break;
          case "CodeMark": {
            // Hide inline-code backticks; KEEP fenced-code fences (they live under
            // a FencedCode parent and stay styled, per the behavior spec).
            const parent = node.node.parent;
            if (parent && parent.name === "FencedCode") break;
            pushHide(node.from, node.to);
            break;
          }
          case "URL": {
            // Hide the (url) of an inline link on inactive lines (the bare link
            // text carries cm-md-link). Reference-style/autolink URLs stay.
            const parent = node.node.parent;
            if (parent && parent.name === "Link") pushHide(node.from, node.to);
            break;
          }
          // List bullets (ListMark) are intentionally NOT hidden in v1
          // (RESEARCH Open Q3) — they stay visible/styled by the theme.
        }
      },
    });
  }

  // sort=true: CM6 orders the set (line decorations ahead of inline at the same
  // position). The plugin NEVER returns a doc edit — purely a view layer.
  return {
    decorations: Decoration.set(deco, true),
    atomic: Decoration.set(widgetRanges, true),
  };
}

export const livePreview = ViewPlugin.fromClass(
  class {
    decorations: DecorationSet;
    // Widget-only atomic ranges — exposed via provide() so backspace over a
    // rendered image/table deletes it as a unit (intended), while text-marker
    // hides remain non-atomic.
    atomicRanges: DecorationSet;

    constructor(view: EditorView) {
      const r = buildDecorations(view);
      this.decorations = r.decorations;
      this.atomicRanges = r.atomic;
    }

    update(u: ViewUpdate) {
      // selectionSet is load-bearing: marker reveal is selection-driven, never
      // timer-driven (Anti-Pattern: recomputing on a debounce lags the cursor).
      if (u.docChanged || u.viewportChanged || u.selectionSet) {
        const r = buildDecorations(u.view);
        this.decorations = r.decorations;
        this.atomicRanges = r.atomic;
      }
    }
  },
  {
    decorations: (v) => v.decorations,
    // atomicRanges only covers the inline image widgets (never the text hides),
    // so cursor motion/backspace treats a rendered widget as one unit.
    provide: (plugin) =>
      EditorView.atomicRanges.of(
        (view) => view.plugin(plugin)?.atomicRanges ?? Decoration.none,
      ),
  },
);

// buildTableDecorations: the BLOCK table widgets. CM6 requires block /
// line-break-spanning replace decorations to come from a StateField (not a
// ViewPlugin), so the multi-line GFM table grid is produced here. The table is
// replaced when off its lines and REVEALED (dropped) when the selection intersects
// any table line — the same active-line reveal as the rest of the surface. Computed
// over the WHOLE document (a StateField has no viewport) — fine at this app's page
// sizes, and CM6 only renders the visible decorations anyway.
function buildTableDecorations(state: EditorState): DecorationSet {
  const activeLines = new Set<number>();
  for (const r of state.selection.ranges) {
    const fromLine = state.doc.lineAt(r.from).number;
    const toLine = state.doc.lineAt(r.to).number;
    for (let ln = fromLine; ln <= toLine; ln++) activeLines.add(ln);
  }
  const rangeActive = (from: number, to: number) => {
    const first = state.doc.lineAt(from).number;
    const last = state.doc.lineAt(to).number;
    for (let ln = first; ln <= last; ln++) {
      if (activeLines.has(ln)) return true;
    }
    return false;
  };

  const deco: Range<Decoration>[] = [];
  syntaxTree(state).iterate({
    enter: (node) => {
      if (node.name !== "Table") return;
      if (!rangeActive(node.from, node.to)) {
        const data = parseTable(state.doc, node.node);
        deco.push(
          Decoration.replace({
            widget: new TableWidget(data),
            block: true,
          }).range(node.from, node.to),
        );
      }
      return false; // never descend into the table's cells/delimiters
    },
  });
  return Decoration.set(deco, true);
}

// tableField holds the block table decorations and recomputes on any doc/selection
// change (selectionSet drives the reveal). The decorations + atomicRanges are both
// provided so a rendered table deletes as a unit (intentional block-widget atomicity).
const tableField = StateField.define<DecorationSet>({
  create: (state) => buildTableDecorations(state),
  update(value, tr) {
    if (tr.docChanged || tr.selection) {
      return buildTableDecorations(tr.state);
    }
    return value;
  },
  provide: (f) => [
    EditorView.decorations.from(f),
    EditorView.atomicRanges.of((view) => view.state.field(f)),
  ],
});

// livePreviewExtension bundles the inline ViewPlugin and the block table StateField
// — the full EDIT-01 live-render surface. mode.ts adds this to liveExtensions.
export const livePreviewExtension: Extension = [livePreview, tableField];
