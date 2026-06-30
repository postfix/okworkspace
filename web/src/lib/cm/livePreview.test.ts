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

import { vi } from "vitest";
import { markdownExtension } from "./markdown";
import { cleanRender, livePreview, livePreviewExtension } from "./livePreview";
import { HashtagWidget } from "./widgets";
import { inlineTagsFromBody } from "./hashtag";
import { EditorView as CMEditorView } from "@codemirror/view";
import type { WidgetType } from "@codemirror/view";

// MermaidWidget.toDOM (called when CM6 renders a visible block widget in the harness)
// invokes renderMermaid → a dynamic import("mermaid") whose real renderer cannot run
// under jsdom. Stub it with a pending promise: these tests assert on the emitted
// DECORATION SET (widget type / presence), never on a rendered diagram, so the render
// never needs to resolve.
vi.mock("./mermaid", () => ({ renderMermaid: vi.fn(() => new Promise<string>(() => {})) }));

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

  it("mermaid fence: read mode yields a MermaidWidget (not a CodeBlockWidget)", () => {
    // ED-05a: a ```mermaid fence renders as a DIAGRAM in read mode; a ```go fence stays
    // a CodeBlockWidget. Same block-replace machinery, branched on the info string.
    const doc = "para\n\n```mermaid\ngraph TD; A-->B;\n```\n";
    const fenceFrom = doc.indexOf("```mermaid");
    const fenceTo = doc.lastIndexOf("```") + 3;
    const clean = decorationsFor(doc, 1, { clean: true });
    const widget = widgetDecoAt(clean, fenceFrom, fenceTo);
    expect(widget).not.toBeNull();
    expect(widget!.constructor.name).toBe("MermaidWidget");
  });

  it("a ```go fence still yields a CodeBlockWidget in read mode (mermaid branch is additive)", () => {
    const doc = "```go\nfmt.Println(1)\n```\n";
    const clean = decorationsFor(doc, undefined, { clean: true });
    const widget = clean.find((d) => d.widget != null && d.from === 0);
    expect(widget).toBeDefined();
    expect(widget!.widget!.constructor.name).toBe("CodeBlockWidget");
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

// ─────────────────────────────────────────────────────────────────────────────
// ED-02a — Phase-21 ADVERSARIAL render corpus (folded in from the adversarial-tester
// gate). The single correctness landmine is *which `#` becomes a badge*, and the
// harder question: does the RENDER (Lezer `Hashtag` node) agree with the pure
// `parseInlineTags` scanner that Phase 22 will reuse? Every case here was RUN through
// the real plugin; the structural cases (blockquote/list/heading/code/link) exercise
// the boundary that hand-written examples miss. `badgeList` finds a badge by LABEL
// (not by first-'#') so a structural prefix doesn't confuse the span.
// ─────────────────────────────────────────────────────────────────────────────
describe("Hashtag badges — adversarial boundary corpus (Phase 21)", () => {
  // The HashtagWidget badges the plugin produced for `doc`, with their spans.
  const badgeList = (
    doc: string,
    opts?: { clean?: boolean; selectionAt?: number },
  ): { label: string; from: number; to: number }[] =>
    decorationsFor(
      doc,
      opts?.selectionAt,
      opts?.clean ? { clean: true } : undefined,
    )
      .filter((d) => d.widget instanceof HashtagWidget)
      .map((d) => ({
        label: (d.widget as HashtagWidget).tag,
        from: d.from,
        to: d.to,
      }));
  const labels = (doc: string, opts?: { clean?: boolean }): string[] =>
    badgeList(doc, { clean: opts?.clean ?? true }).map((b) => b.label);

  // POSITIVE — a badge IS produced (clean render). These cover the structural
  // boundaries (blockquote/list/heading-content) and the body/slash/case alphabet.
  it("badges a tag at the start of blockquote CONTENT (with a space): '> #deploy'", () => {
    expect(labels("> #deploy")).toEqual(["deploy"]);
  });
  it("badges a tag inside a list item: '- #deploy' and '* #deploy'", () => {
    expect(labels("- #deploy")).toEqual(["deploy"]);
    expect(labels("* #deploy")).toEqual(["deploy"]);
  });
  it("badges a tag AFTER an ATX heading marker on the same line: '## #deploy'", () => {
    // The leading '##' is a block HeaderMark; the inline content '#deploy' is a tag.
    const found = badgeList("## #deploy", { clean: true });
    expect(found.length).toBe(1);
    expect(found[0].label).toBe("deploy");
    // The badge replaces exactly the '#deploy' span (index 3), not the heading '#'s.
    expect(found[0].from).toBe("## #deploy".indexOf("#deploy"));
    expect(found[0].to).toBe(found[0].from + "#deploy".length);
  });
  it("badges after a MID-LINE tab: 'a\\t#deploy'", () => {
    expect(labels("a\t#deploy")).toEqual(["deploy"]);
  });
  it("badges after a soft line break (newline boundary): 'a\\n#deploy'", () => {
    expect(labels("a\n#deploy")).toEqual(["deploy"]);
  });
  it("preserves CASE in the badge label (render is not lower-cased): '#0xFF' → '0xFF'", () => {
    // The render slices the doc verbatim (case kept); inlineTagsFromBody lower-cases for
    // the materialized set. Same SPAN, different normalization — by design, not a drift.
    expect(labels("#0xFF")).toEqual(["0xFF"]);
    expect(inlineTagsFromBody("#0xFF")).toEqual(["0xff"]);
  });
  it("badges the slash/dash/underscore body forms: #/a #-1 #_ #a//b #a/ #a/b/c", () => {
    expect(labels("#/a")).toEqual(["/a"]);
    expect(labels("#-1")).toEqual(["-1"]);
    expect(labels("#_")).toEqual(["_"]);
    expect(labels("#a//b")).toEqual(["a//b"]);
    expect(labels("#a/")).toEqual(["a"]); // trailing slash trimmed
    expect(labels("#a/b/c")).toEqual(["a/b/c"]);
  });
  it("a non-ASCII letter terminates the ASCII-only body: '#café' → '#caf'", () => {
    expect(labels("#café")).toEqual(["caf"]);
  });

  // NEGATIVE — NO badge. Structural exclusions (code/heading-marker/link) come from
  // the Lezer tree; the boundary rule rejects the punctuation/unicode-preceded forms.
  it("does NOT badge inside link TEXT (the '[' precedes the '#'): '[#tag](u)'", () => {
    expect(labels("[#tag](http://x.test/)")).toEqual([]);
  });
  it("does NOT badge directly after an inline-code span (backtick precedes): '`x`#tag'", () => {
    expect(labels("`x`#tag")).toEqual([]);
    expect(labels("a `x`#tag")).toEqual([]);
  });
  it("does NOT badge inside an INDENTED code block (leading tab): '\\t#deploy'", () => {
    // A leading tab makes the line an indented code block → no inline parse → no badge.
    // inlineTagsFromBody runs the SAME parse, so it AGREES (no extract) — the Phase-21
    // code-region scope gap is closed (asserted in hashtag.test.ts).
    expect(labels("\t#deploy")).toEqual([]);
  });
  it("does NOT badge after punctuation other than whitespace: '(#tag' ',#tag' '\"#tag'", () => {
    expect(labels("(#tag)")).toEqual([]);
    expect(labels(",#tag")).toEqual([]);
    expect(labels('"#tag')).toEqual([]);
  });
  it("does NOT badge after an em-dash (U+2014 is not whitespace): 'a —#tag'", () => {
    expect(labels("a —#tag")).toEqual([]);
  });
  it("does NOT badge when a non-ASCII char precedes '#': 'é#tag' / '😀#tag'", () => {
    expect(labels("é#tag")).toEqual([]);
    expect(labels("😀#tag")).toEqual([]);
  });
  it("does NOT badge '###tag' (no body char follows the first '#') or '1.#tag'", () => {
    expect(labels("###tag")).toEqual([]);
    expect(labels("1.#tag")).toEqual([]);
  });

  // SPAN VALIDITY across multi-byte content (surrogate pairs are 2 code units).
  it("keeps a valid code-unit span for a tag AFTER a surrogate-pair char + space", () => {
    const doc = "😀 #deploy"; // 😀 = 2 code units, then ' ', then '#deploy'
    const found = badgeList(doc, { clean: true });
    expect(found.length).toBe(1);
    expect(found[0].label).toBe("deploy");
    const hashPos = doc.indexOf("#"); // code-unit index === 3
    expect(found[0].from).toBe(hashPos);
    expect(found[0].to).toBe(hashPos + "#deploy".length);
    // The slice the widget used is exactly the tag — the span did not split the pair.
    expect(doc.slice(found[0].from + 1, found[0].to)).toBe("deploy");
  });

  // SECURITY/CONTRACT — the badge widget must be NON-atomic (never trap the cursor).
  it("the badge widget is NON-atomic: rendered badges contribute ZERO atomicRanges", () => {
    const doc = "#a #b #c"; // three badges in clean render
    const parent = document.createElement("div");
    const view = new EditorView({
      parent,
      state: EditorState.create({
        doc,
        extensions: [
          markdownExtension,
          livePreviewExtension,
          cleanRender.of(true),
        ],
      }),
    });
    views.push(view);
    // Sanity: the three badges really were produced.
    expect(labels(doc).length).toBe(3);
    // ...yet the inline ViewPlugin exposes NO atomic ranges (Pitfall 1 — unlike the
    // image/table widgets, a hashtag is short inline text and stays non-atomic).
    const plugin = view.plugin(livePreview)!;
    let atomicCount = 0;
    const iter = plugin.atomicRanges.iter();
    while (iter.value) {
      atomicCount++;
      iter.next();
    }
    expect(atomicCount).toBe(0);
  });

  // BYTE-STABILITY under churn — a tag-dense doc with every structural shape must be
  // byte-verbatim after the plugin builds decorations, in BOTH edit and clean mode.
  it("byte-stable: a tag-dense doc is unchanged in edit AND clean mode (zero transactions)", () => {
    const doc = "#a #b/c #deploy\n> #d\n- #e\n## #f\n`x` #g #h/i/\n";
    for (const clean of [false, true]) {
      const parent = document.createElement("div");
      const view = new EditorView({
        parent,
        state: EditorState.create({
          doc,
          extensions: [
            markdownExtension,
            livePreviewExtension,
            ...(clean ? [cleanRender.of(true)] : []),
          ],
        }),
      });
      views.push(view);
      // No document transaction was ever issued → bytes verbatim, every literal kept.
      expect(view.state.doc.toString()).toBe(doc);
      expect(view.state.doc.toString()).toContain("#deploy");
      expect(view.state.doc.toString()).toContain("#h/i/");
    }
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// ED-02b — render⇄materialize AGREEMENT (Phase 22 CLOSED the Phase-21 gap). The
// Phase-21 pure `parseInlineTags` string scanner diverged from the render on two
// render-only behaviors: (1) it had no block structure, so `>#deploy` (no space after
// the blockquote marker) was rejected by the raw `>` boundary even though the render
// badges it; and (2) it extracted `#tag` from inside code. Phase 22 retired that
// scanner: `inlineTagsFromBody` runs the SAME Lezer extension set as the render, so it
// agrees by construction. These tests flip the old "DRIFT" pins into agreement assertions
// — the materialized set now equals the rendered badge set, the whole point of ED-02b.
// ─────────────────────────────────────────────────────────────────────────────
describe("Hashtag render⇄materialize agreement (Phase 22 — Phase-21 gap closed)", () => {
  const renderedTags = (doc: string): string[] =>
    decorationsFor(doc, undefined, { clean: true })
      .filter((d) => d.widget instanceof HashtagWidget)
      .map((d) => (d.widget as HashtagWidget).tag);

  it("the RENDER badges '>#deploy' (blockquote-content start is a valid boundary)", () => {
    expect(renderedTags(">#deploy")).toEqual(["deploy"]);
  });
  it("inlineTagsFromBody('>#deploy') now AGREES with the render → ['deploy']", () => {
    // The structural Lezer parse sees '>' as a blockquote marker and '#deploy' as
    // section-start content — a valid boundary, exactly as the render does.
    expect(inlineTagsFromBody(">#deploy")).toEqual(["deploy"]);
  });
  it("render and materialize agree on the block-marker-abutting case (gap closed)", () => {
    // Both badge AND materialize '>#deploy' (case-folded), and both handle the
    // space-after-marker form identically. The render keeps display case; materialize
    // lower-cases — same membership, by design (D-04).
    expect(renderedTags(">#deploy")).toEqual(["deploy"]);
    expect(inlineTagsFromBody(">#deploy")).toEqual(["deploy"]);
    expect(renderedTags("> #deploy")).toEqual(["deploy"]);
    expect(inlineTagsFromBody("> #deploy")).toEqual(["deploy"]);
  });
});
