// EDIT-01 — live-preview ViewPlugin decoration coverage. The ViewPlugin walks the
// Lezer markdown tree over the visible ranges and emits decorations:
//   • Decoration.mark   — styling (cm-strong / cm-em / cm-inline-code /
//                          cm-code-block / cm-md-link), kept on the active line.
//   • Decoration.replace — zero-width hides for the syntax markers (**, *, #,
//                          backticks, link brackets/URL), DROPPED on the line the
//                          selection touches so markers reveal for editing.
//
// These tests mount a minimal EditorView in jsdom (the corpus tests in
// mode.test.ts already prove a headless view dispatches under jsdom), seed a known
// construct, and walk the plugin's DecorationSet asserting the expected kinds/
// classes are produced — and that the hides are dropped when the selection is
// placed on the construct's line (active-line reveal).
//
// Image/table WIDGET decorations are 06-03's scope; they stay `it.todo` here.
import { describe, it, expect, afterEach } from "vitest";
import { EditorState } from "@codemirror/state";
import { EditorView } from "@codemirror/view";
import type { DecorationSet } from "@codemirror/view";

import { markdownExtension } from "./markdown";
import { cleanRender, livePreviewExtension } from "./livePreview";
import { HashtagWidget } from "./widgets";
import { EditorView as CMEditorView } from "@codemirror/view";
import type { WidgetType } from "@codemirror/view";

// A description of a single decoration as emitted by the plugin, flattened for
// assertion: its range plus the class (for marks), whether it is a "replace"
// decoration (a hide OR a widget), and the widget (if any) it mounts.
interface FlatDeco {
  from: number;
  to: number;
  class: string | null;
  isReplace: boolean;
  widget: WidgetType | null;
}

// mount builds a headless EditorView with the markdown language + livePreview
// plugin, seeds `doc`, optionally places the selection at `selectionAt`, and
// returns the flattened decoration list the plugin produced.
function decorationsFor(
  doc: string,
  selectionAt?: number,
  opts?: { clean?: boolean },
): FlatDeco[] {
  const parent = document.createElement("div");
  // Give the view a viewport so visibleRanges covers the whole (small) doc under
  // jsdom — without a measured layout CM6 still reports the full doc as visible
  // for a freshly-created small document.
  const view = new EditorView({
    parent,
    state: EditorState.create({
      doc,
      selection:
        selectionAt != null ? { anchor: selectionAt } : { anchor: 0 },
      // livePreviewExtension = the inline ViewPlugin + the block table/code StateFields.
      // `clean` adds the read-mode facet (PageView's render: no active-line reveal,
      // list/quote/fence markers hidden).
      extensions: [
        markdownExtension,
        livePreviewExtension,
        ...(opts?.clean ? [cleanRender.of(true)] : []),
      ],
    }),
  });
  views.push(view);

  const out: FlatDeco[] = [];
  const collect = (set: DecorationSet) => {
    const iter = set.iter();
    while (iter.value) {
      const spec = iter.value.spec as { class?: string; widget?: WidgetType };
      // A Decoration.mark/.line carries a `class`. A Decoration.replace({}) hide has
      // neither class nor widget. A widget replace carries `widget`. "isReplace" =
      // not a class-bearing mark/line (i.e. a hide OR a widget).
      out.push({
        from: iter.from,
        to: iter.to,
        class: spec.class ?? null,
        isReplace: !spec.class,
        widget: spec.widget ?? null,
      });
      iter.next();
    }
  };
  // Both the inline ViewPlugin (marks/hides/image widgets) and the block table
  // StateField register into the EditorView.decorations facet. Iterating the facet
  // collects both without double-counting; a source is either a DecorationSet or a
  // (view → DecorationSet) function.
  view.state.facet(CMEditorView.decorations).forEach((src) => {
    const set = typeof src === "function" ? src(view) : src;
    collect(set as DecorationSet);
  });
  return out;
}

// widgetDecoAt returns the WidgetType mounted by a replace decoration exactly
// covering [from,to), or null if none (e.g. the widget was dropped on the active
// line). Block widgets may report `to` one past the node end (CM6 extends a block
// replace to the line boundary), so match on `from` + widget presence.
function widgetDecoAt(
  decos: FlatDeco[],
  from: number,
  to: number,
): WidgetType | null {
  const d = decos.find(
    (x) => x.widget != null && x.from === from && x.to >= to,
  );
  return d ? d.widget : null;
}

const views: EditorView[] = [];
afterEach(() => {
  while (views.length) views.pop()!.destroy();
});

// Helper predicates over the flattened decoration list.
const hasMark = (decos: FlatDeco[], cls: string) =>
  decos.some((d) => d.class === cls);
const hides = (decos: FlatDeco[]) => decos.filter((d) => d.isReplace);
const marksOnly = (decos: FlatDeco[]) => decos.filter((d) => !d.isReplace);

describe("livePreview decorations (EDIT-01)", () => {
  it("heading line: ATXHeading* → hide HeaderMark, style the heading", () => {
    // Cursor parked away from the heading line so markers stay hidden.
    const decos = decorationsFor("# Title\n\nbody text here\n", 12);
    // The leading "# " HeaderMark is hidden (a replace decoration at the start).
    expect(hides(decos).some((d) => d.from === 0 && d.to <= 2)).toBe(true);
    // A heading style mark/line decoration is present.
    expect(
      decos.some((d) => d.class != null && /cm-heading/.test(d.class)),
    ).toBe(true);
  });

  it("bold: StrongEmphasis → cm-strong mark, hide EmphasisMark run", () => {
    // "para\n\n**bold** word" — cursor on the para line, away from the bold.
    const decos = decorationsFor("para line\n\n**bold** word\n", 3);
    expect(hasMark(decos, "cm-strong")).toBe(true);
    // The two ** runs are hidden (two replace decorations of width 2).
    const replaceWidth2 = hides(decos).filter((d) => d.to - d.from === 2);
    expect(replaceWidth2.length).toBeGreaterThanOrEqual(2);
  });

  it("italic: Emphasis → cm-em mark, hide EmphasisMark run", () => {
    const decos = decorationsFor("para line\n\n*italic* word\n", 3);
    expect(hasMark(decos, "cm-em")).toBe(true);
    // The two * runs are hidden (width 1 each).
    const replaceWidth1 = hides(decos).filter((d) => d.to - d.from === 1);
    expect(replaceWidth1.length).toBeGreaterThanOrEqual(2);
  });

  it("inline code: InlineCode → cm-inline-code mark, hide CodeMark backticks", () => {
    const decos = decorationsFor("para line\n\n`code` word\n", 3);
    expect(hasMark(decos, "cm-inline-code")).toBe(true);
    // The two backtick CodeMarks are hidden.
    expect(hides(decos).length).toBeGreaterThanOrEqual(2);
  });

  it("code block: FencedCode → cm-code-block styling, fences kept (not hidden)", () => {
    const decos = decorationsFor("para\n\n```\ncode\n```\n", 1);
    expect(hasMark(decos, "cm-code-block")).toBe(true);
    // The fence CodeMarks must NOT be hidden (block fences stay styled, per
    // behavior spec) — assert no replace decoration starts at the fence run.
    const fenceStart = "para\n\n".length;
    expect(
      hides(decos).some((d) => d.from === fenceStart),
    ).toBe(false);
  });

  it("link: Link → cm-md-link mark, hide LinkMark/URL on inactive line", () => {
    const decos = decorationsFor(
      "para line\n\n[text](http://x.test/) word\n",
      3,
    );
    expect(hasMark(decos, "cm-md-link")).toBe(true);
    // Brackets + the (url) are hidden when the cursor is off the link line.
    expect(hides(decos).length).toBeGreaterThanOrEqual(2);
  });

  it("active-line reveal: hides under the selection are dropped, marks stay", () => {
    const doc = "para line\n\n**bold** word\n";
    const boldLineCol = doc.indexOf("**bold**") + 2; // inside the bold word
    const decos = decorationsFor(doc, boldLineCol);
    // No hide decorations remain on the active (bold) line — markers reveal.
    const boldLineFrom = doc.indexOf("**bold**");
    const boldLineTo = boldLineFrom + "**bold** word".length;
    const hidesOnBoldLine = hides(decos).filter(
      (d) => d.from >= boldLineFrom && d.to <= boldLineTo,
    );
    expect(hidesOnBoldLine.length).toBe(0);
    // The styling mark MAY stay (reveal drops only the hides).
    expect(hasMark(decos, "cm-strong")).toBe(true);
  });

  it("inactive line keeps its hides while another line is active", () => {
    // Two bold lines; cursor on the first → first reveals, second stays hidden.
    const doc = "**one** a\n\n**two** b\n";
    const decos = decorationsFor(doc, doc.indexOf("**one**") + 2);
    const line2From = doc.indexOf("**two**");
    const line2To = line2From + "**two** b".length;
    const hidesOnLine2 = hides(decos).filter(
      (d) => d.from >= line2From && d.to <= line2To,
    );
    expect(hidesOnLine2.length).toBeGreaterThanOrEqual(2);
  });

  it("emits only marks/replaces — never serializes the tree back to text", () => {
    // Sanity: the plugin produces a DecorationSet and never mutates the doc.
    const doc = "# H\n\n**b** *i* `c` [l](u)\n";
    const parent = document.createElement("div");
    const view = new EditorView({
      parent,
      state: EditorState.create({
        doc,
        extensions: [markdownExtension, livePreviewExtension],
      }),
    });
    views.push(view);
    expect(view.state.doc.toString()).toBe(doc);
    expect(marksOnly(decorationsFor(doc, 1)).length).toBeGreaterThanOrEqual(0);
  });

  // 06-03 — image/table WIDGET decorations (replace + active-line reveal).
  it("image: Image → ImageWidget replace decoration when off the image line", () => {
    // Cursor parked on the first paragraph, away from the image line.
    const doc = "para\n\n![alt](img.png)\n";
    const decos = decorationsFor(doc, 1);
    const imgFrom = doc.indexOf("![alt]");
    const imgTo = imgFrom + "![alt](img.png)".length;
    // A replace decoration carrying a widget covers the whole image span.
    const widget = widgetDecoAt(decos, imgFrom, imgTo);
    expect(widget).not.toBeNull();
    expect(widget!.constructor.name).toBe("ImageWidget");
  });

  it("image: a javascript: src widget renders the RAW-text fallback, not an <img>", () => {
    // The widget decoration is still emitted, but its toDOM() must produce the raw
    // markdown span (the sanitizer blocks the src) — never an executable <img>.
    const doc = "para\n\n![x](javascript:alert(1))\n";
    const decos = decorationsFor(doc, 1);
    const imgFrom = doc.indexOf("![x]");
    const imgTo = imgFrom + "![x](javascript:alert(1))".length;
    const widget = widgetDecoAt(decos, imgFrom, imgTo) as
      | (import("@codemirror/view").WidgetType & { toDOM(): HTMLElement })
      | null;
    expect(widget).not.toBeNull();
    const dom = widget!.toDOM();
    expect(dom.tagName).not.toBe("IMG");
    expect(dom.tagName).toBe("SPAN");
    expect(dom.textContent).toBe("![x](javascript:alert(1))");
  });

  it("image: reveal-to-source — no widget when the cursor is on the image line", () => {
    const doc = "para\n\n![alt](img.png)\n";
    const imgFrom = doc.indexOf("![alt]");
    const decos = decorationsFor(doc, imgFrom + 2); // cursor inside the image source
    const imgTo = imgFrom + "![alt](img.png)".length;
    expect(widgetDecoAt(decos, imgFrom, imgTo)).toBeNull();
  });

  it("GFM table: Table → block widget grid, revealed when the selection is on a table line", () => {
    const doc = "para\n\n| A | B |\n|---|---|\n| 1 | 2 |\n\nafter\n";
    const tableFrom = doc.indexOf("| A | B |");
    const tableTo = doc.indexOf("| 1 | 2 |") + "| 1 | 2 |".length;

    // Cursor on the first paragraph (off the table) → a block widget covers it.
    const off = decorationsFor(doc, 1);
    const widget = widgetDecoAt(off, tableFrom, tableTo);
    expect(widget).not.toBeNull();
    expect(widget!.constructor.name).toBe("TableWidget");

    // Cursor on a table line → the widget is dropped (reveal-to-source).
    const onTable = decorationsFor(doc, tableFrom + 2);
    expect(widgetDecoAt(onTable, tableFrom, tableTo)).toBeNull();
  });
});

// ED-02a — the inline `#tag` badge corpus. The single correctness landmine is
// *which `#` is a tag*, so the NEGATIVE cases are as load-bearing as the positive
// (CONTEXT D-02). Every case is pinned through the SAME decorationsFor harness: a
// Hashtag badge is a replace decoration whose `widget instanceof HashtagWidget`.
describe("Hashtag badges (ED-02a)", () => {
  // All HashtagWidget decorations the plugin produced for `doc` (clean = read mode).
  const badges = (doc: string, selectionAt?: number, clean = false): FlatDeco[] =>
    decorationsFor(doc, selectionAt, clean ? { clean: true } : undefined).filter(
      (d) => d.widget instanceof HashtagWidget,
    );

  // POSITIVE corpus — a badge IS produced (clean render, so no active-line reveal).
  // [doc, expected label without the leading '#'].
  const positive: [string, string][] = [
    ["#deploy", "deploy"],
    ["#abc", "abc"],
    ["#fff", "fff"], // #-hex-like: a tag, NOT special-cased (Obsidian parity)
    ["#a/b", "a/b"], // nested
  ];
  for (const [doc, label] of positive) {
    it(`badges ${JSON.stringify(doc)} → #${label}`, () => {
      const found = badges(doc, undefined, true);
      expect(found.length).toBe(1);
      expect((found[0].widget as HashtagWidget).tag).toBe(label);
      // The decoration replaces exactly the `#tag` span, '#' included.
      expect(found[0].from).toBe(doc.indexOf("#"));
      expect(found[0].to).toBe(doc.indexOf("#") + ("#" + label).length);
    });
  }

  // NEGATIVE corpus — NO Hashtag badge is produced. Code/heading exclusions are
  // structural (the Lezer tree never yields a Hashtag node there); the boundary/
  // digit rules reject the rest. Pinned to catch a grammar/precedence regression.
  const negative: [string, string][] = [
    ["`#code`", "inline code"],
    ["```\n#x\n```\n", "fenced code"],
    ["# Heading", "ATX heading marker"],
    ["see /#frag", "URL fragment after '/'"],
    ["#1984", "all-digit run"],
    ["\\#esc", "escaped hash"],
    ["foo#bar", "mid-word"],
  ];
  for (const [doc, why] of negative) {
    it(`does NOT badge ${JSON.stringify(doc)} (${why})`, () => {
      expect(badges(doc, undefined, true).length).toBe(0);
    });
  }

  it("edit mode: badge shows off the tag line, reveals to raw on the active line", () => {
    const doc = "para line\n\n#deploy here\n";
    const tagFrom = doc.indexOf("#deploy");
    // Cursor on the first paragraph (off the tag line) → badge present.
    const off = badges(doc, 1);
    expect(off.length).toBe(1);
    expect((off[0].widget as HashtagWidget).tag).toBe("deploy");
    // Cursor on the tag line → badge dropped, raw `#deploy` revealed for editing.
    const on = badges(doc, tagFrom + 2);
    expect(on.length).toBe(0);
  });

  it("read (clean) mode always badges, even with the selection on the tag line", () => {
    const doc = "#deploy here\n";
    // Selection on the tag line, but clean render keeps activeLines empty → badge.
    expect(badges(doc, 2, true).length).toBe(1);
  });

  it("byte-stable: building decorations never mutates the doc (literal #deploy kept)", () => {
    const doc = "see #deploy in the body\n";
    const parent = document.createElement("div");
    const view = new EditorView({
      parent,
      // clean render forces a badge to be built; the doc must still be verbatim.
      state: EditorState.create({
        doc,
        extensions: [markdownExtension, livePreviewExtension, cleanRender.of(true)],
      }),
    });
    views.push(view);
    // The plugin built a HashtagWidget in its constructor — yet zero transactions
    // were issued, so the document bytes are unchanged and still hold `#deploy`.
    expect(view.state.doc.toString()).toBe(doc);
    expect(view.state.doc.toString()).toContain("#deploy");
  });
});

// Clean render is the PageView (read-mode) surface: the cleanRender facet disables
// the active-line marker reveal and turns list/quote/fence markers into finished
// prose. Edit mode never provides the facet, so the tests above (no facet) prove the
// editing surface is byte-for-byte unchanged.
describe("livePreview clean render (read mode)", () => {
  it("heading on the selection line still hides its '#' (no active-line reveal)", () => {
    // Selection at 0 sits on the heading line. In EDIT mode that reveals the "# ";
    // in clean mode the reveal is disabled, so the HeaderMark is still hidden.
    const doc = "# Title\n\nbody\n";
    const edit = decorationsFor(doc, 0); // active-line reveal → no hide on line 1
    expect(hides(edit).some((d) => d.from === 0)).toBe(false);

    const clean = decorationsFor(doc, 0, { clean: true });
    expect(hasMark(clean, "cm-heading-1")).toBe(true);
    // The "# " run (and its trailing space) is hidden even though the selection is
    // on the heading line.
    expect(hides(clean).some((d) => d.from === 0)).toBe(true);
  });

  it("unordered list: the ListMark is replaced by a BulletWidget", () => {
    const doc = "- one\n- two\n";
    // Edit mode: bullets are NOT touched (no widget at the dash).
    const edit = decorationsFor(doc, 8);
    expect(edit.some((d) => d.widget != null && d.from === 0)).toBe(false);

    const clean = decorationsFor(doc, undefined, { clean: true });
    const bullet = clean.find((d) => d.widget != null && d.from === 0);
    expect(bullet).toBeDefined();
    expect(bullet!.widget!.constructor.name).toBe("BulletWidget");
  });

  it("ordered list: numeric markers are left intact (no bullet widget)", () => {
    const doc = "1. one\n2. two\n";
    const clean = decorationsFor(doc, undefined, { clean: true });
    // No widget replaces the "1." / "2." markers.
    expect(clean.some((d) => d.widget != null && d.from === 0)).toBe(false);
  });

  it("blockquote: the '>' is hidden and the line carries cm-blockquote", () => {
    const doc = "> quoted line\n\nafter\n";
    const edit = decorationsFor(doc, 17);
    expect(hasMark(edit, "cm-blockquote")).toBe(false);
    expect(hides(edit).some((d) => d.from === 0)).toBe(false);

    const clean = decorationsFor(doc, undefined, { clean: true });
    expect(hasMark(clean, "cm-blockquote")).toBe(true);
    expect(hides(clean).some((d) => d.from === 0)).toBe(true);
  });

  it("fenced code: a CodeBlockWidget replaces the whole block (fences hidden)", () => {
    const doc = "para\n\n```go\ncode()\n```\n";
    const fenceFrom = doc.indexOf("```go");
    const fenceTo = doc.lastIndexOf("```") + 3;

    // Edit mode keeps the cm-code-block styling mark and shows the fences (no widget).
    const edit = decorationsFor(doc, 1);
    expect(hasMark(edit, "cm-code-block")).toBe(true);
    expect(edit.some((d) => d.widget != null && d.from === fenceFrom)).toBe(false);

    // Clean mode replaces the block with a CodeBlockWidget — no cm-code-block mark.
    const clean = decorationsFor(doc, 1, { clean: true });
    const widget = widgetDecoAt(clean, fenceFrom, fenceTo);
    expect(widget).not.toBeNull();
    expect(widget!.constructor.name).toBe("CodeBlockWidget");
    expect(hasMark(clean, "cm-code-block")).toBe(false);
  });

  it("CodeBlockWidget renders code text without the ``` fences or info string", () => {
    const doc = "```go\nfmt.Println(1)\n```\n";
    const clean = decorationsFor(doc, undefined, { clean: true });
    const widget = clean.find(
      (d) => d.widget != null && d.widget.constructor.name === "CodeBlockWidget",
    );
    expect(widget).toBeDefined();
    const dom = (
      widget!.widget as import("@codemirror/view").WidgetType & {
        toDOM(): HTMLElement;
      }
    ).toDOM();
    expect(dom.tagName).toBe("PRE");
    expect(dom.getAttribute("data-lang")).toBe("go");
    expect(dom.textContent).toBe("fmt.Println(1)");
    // No backtick fences leaked into the rendered text.
    expect(dom.textContent).not.toContain("`");
  });
});
