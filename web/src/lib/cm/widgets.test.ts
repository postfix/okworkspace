// EDIT-01 / EDIT-04 — the inline WIDGETS (ImageWidget + TableWidget) and the
// load-bearing image-src security control (T-06-06..T-06-09).
//
// These tests mount each WidgetType's toDOM() output directly (no EditorView needed
// — a WidgetType.toDOM is a pure DOM factory) and assert on ELEMENT STRUCTURE
// (tagName / children / textContent / attributes), never on HTML strings. Asserting
// structure rather than innerHTML is itself the T-06-08 guard: the widget is proven
// to build DOM via createElement + textContent, never by assigning page content to
// innerHTML.
//
// The security cases are explicit: a `javascript:`/executable-`data:`/protocol-
// relative image src must yield the RAW-text fallback <span>, NOT an <img> (which
// would load/execute the blocked URL).
import { describe, it, expect } from "vitest";
import { ImageWidget, TableWidget } from "./widgets";
import type { TableData } from "./widgets";

describe("ImageWidget (EDIT-01 inline image + EDIT-04 src gate)", () => {
  it("renders an <img> for a safe http(s) src", () => {
    const w = new ImageWidget(
      "https://cdn.test/a.png",
      "a pic",
      "![a pic](https://cdn.test/a.png)",
    );
    const dom = w.toDOM();
    expect(dom.tagName).toBe("IMG");
    expect(dom.getAttribute("src")).toBe("https://cdn.test/a.png");
    expect(dom.getAttribute("alt")).toBe("a pic");
    expect(dom.className).toBe("cm-md-image");
  });

  it("renders an <img> for a safe app-relative src", () => {
    const w = new ImageWidget("attachments/x.png", "x", "![x](attachments/x.png)");
    const dom = w.toDOM();
    expect(dom.tagName).toBe("IMG");
    expect(dom.getAttribute("src")).toBe("attachments/x.png");
  });

  it("BLOCKS a javascript: src → raw-text <span>, never an <img>", () => {
    const raw = "![x](javascript:alert(1))";
    const w = new ImageWidget("javascript:alert(1)", "x", raw);
    const dom = w.toDOM();
    // Must NOT be an executable/loading <img>.
    expect(dom.tagName).not.toBe("IMG");
    expect(dom.tagName).toBe("SPAN");
    // Falls back to the RAW markdown text, set via textContent (no innerHTML).
    expect(dom.textContent).toBe(raw);
    // And carries no live src attribute anywhere in the produced DOM.
    expect((dom as HTMLElement).querySelector("img")).toBeNull();
  });

  it("BLOCKS an executable data: src → raw-text <span>", () => {
    const raw = "![x](data:text/html,<script>alert(1)</script>)";
    const w = new ImageWidget(
      "data:text/html,<script>alert(1)</script>",
      "x",
      raw,
    );
    const dom = w.toDOM();
    expect(dom.tagName).toBe("SPAN");
    expect(dom.textContent).toBe(raw);
  });

  it("BLOCKS a protocol-relative //host src → raw-text <span>", () => {
    const raw = "![x](//evil.test/x.png)";
    const w = new ImageWidget("//evil.test/x.png", "x", raw);
    const dom = w.toDOM();
    expect(dom.tagName).toBe("SPAN");
    expect(dom.textContent).toBe(raw);
  });

  it("never assigns page content to innerHTML — alt with markup stays inert", () => {
    // An alt carrying angle brackets must surface as text, not parsed markup.
    const w = new ImageWidget(
      "https://cdn.test/a.png",
      "<b>x</b>",
      "![<b>x</b>](https://cdn.test/a.png)",
    );
    const dom = w.toDOM();
    expect(dom.tagName).toBe("IMG");
    // The alt is an attribute string, never injected as HTML — the <img> has no
    // child elements (innerHTML would have produced a <b> child somewhere).
    expect(dom.getAttribute("alt")).toBe("<b>x</b>");
    expect((dom as HTMLElement).children.length).toBe(0);
  });

  it("eq() compares src + alt", () => {
    const a = new ImageWidget("s", "alt", "raw1");
    const b = new ImageWidget("s", "alt", "raw2-different");
    const c = new ImageWidget("s", "other", "raw1");
    expect(a.eq(b)).toBe(true); // raw differs but src+alt match
    expect(a.eq(c)).toBe(false);
  });
});

describe("TableWidget (EDIT-01 GFM table grid)", () => {
  const data: TableData = {
    header: ["A", "B"],
    rows: [
      ["1", "2"],
      ["3", "4"],
    ],
  };

  it("builds a <table>/<thead>/<tbody> grid from parsed cells", () => {
    const dom = new TableWidget(data).toDOM();
    expect(dom.tagName).toBe("TABLE");
    expect(dom.className).toBe("cm-md-table");

    const thead = dom.querySelector("thead")!;
    const ths = thead.querySelectorAll("th");
    expect(Array.from(ths).map((th) => th.textContent)).toEqual(["A", "B"]);

    const tbody = dom.querySelector("tbody")!;
    const trs = tbody.querySelectorAll("tr");
    expect(trs.length).toBe(2);
    expect(Array.from(trs[0].querySelectorAll("td")).map((td) => td.textContent)).toEqual([
      "1",
      "2",
    ]);
    expect(Array.from(trs[1].querySelectorAll("td")).map((td) => td.textContent)).toEqual([
      "3",
      "4",
    ]);
  });

  it("cell text is set via textContent — markup in a cell stays inert", () => {
    const dom = new TableWidget({
      header: ["<b>H</b>"],
      rows: [["<script>x</script>"]],
    }).toDOM();
    const th = dom.querySelector("th")!;
    const td = dom.querySelector("td")!;
    // textContent round-trips the literal string; no child elements were parsed in.
    expect(th.textContent).toBe("<b>H</b>");
    expect(th.children.length).toBe(0);
    expect(td.textContent).toBe("<script>x</script>");
    expect(td.querySelector("script")).toBeNull();
  });

  it("eq() compares the structured cell data", () => {
    const a = new TableWidget(data);
    const b = new TableWidget({ header: ["A", "B"], rows: [["1", "2"], ["3", "4"]] });
    const c = new TableWidget({ header: ["A", "B"], rows: [["1", "X"]] });
    expect(a.eq(b)).toBe(true);
    expect(a.eq(c)).toBe(false);
  });
});
