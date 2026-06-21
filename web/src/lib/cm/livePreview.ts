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
// Image/table WIDGET decorations and link click-navigation are 06-03's scope.
import {
  Decoration,
  ViewPlugin,
  EditorView,
} from "@codemirror/view";
import type { DecorationSet, ViewUpdate } from "@codemirror/view";
import { syntaxTree } from "@codemirror/language";
import type { Range } from "@codemirror/state";

// hideMark hides a syntax marker. Zero-width + NON-atomic (no atomicRanges) so the
// cursor is never trapped and revealing it does not reflow neighbors (Pitfall 1/2).
const hideMark = Decoration.replace({});

// Inline styling marks — class-only so the theme (theme.ts) owns all sizing; the
// marks themselves never set width/height (layout-neutral reveal, Pitfall 2).
const strongMark = Decoration.mark({ class: "cm-strong" });
const emMark = Decoration.mark({ class: "cm-em" });
const inlineCodeMark = Decoration.mark({ class: "cm-inline-code" });
const codeBlockMark = Decoration.mark({ class: "cm-code-block" });
const linkMark = Decoration.mark({ class: "cm-md-link" });

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

function buildDecorations(view: EditorView): DecorationSet {
  // Lines the selection touches → reveal (skip hides) on those lines.
  const activeLines = new Set<number>();
  for (const r of view.state.selection.ranges) {
    const fromLine = view.state.doc.lineAt(r.from).number;
    const toLine = view.state.doc.lineAt(r.to).number;
    for (let ln = fromLine; ln <= toLine; ln++) activeLines.add(ln);
  }
  const isActive = (pos: number) =>
    activeLines.has(view.state.doc.lineAt(pos).number);

  // Collect marks/lines and hides separately so we can sort lines before marks
  // (CM6 requires line decorations sorted ahead of inline decorations at the same
  // position — Decoration.set(..., true) sorts by from/startSide for us, but line
  // decos must sit at the line start).
  const deco: Range<Decoration>[] = [];

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
          case "Link":
            deco.push(linkMark.range(node.from, node.to));
            break;
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
  return Decoration.set(deco, true);
}

export const livePreview = ViewPlugin.fromClass(
  class {
    decorations: DecorationSet;

    constructor(view: EditorView) {
      this.decorations = buildDecorations(view);
    }

    update(u: ViewUpdate) {
      // selectionSet is load-bearing: marker reveal is selection-driven, never
      // timer-driven (Anti-Pattern: recomputing on a debounce lags the cursor).
      if (u.docChanged || u.viewportChanged || u.selectionSet) {
        this.decorations = buildDecorations(u.view);
      }
    }
  },
  {
    decorations: (v) => v.decorations,
  },
);
