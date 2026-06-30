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
import { describe, it, expect, vi } from "vitest";
import { CodeBlockWidget, HashtagWidget, ImageWidget, TableWidget } from "./widgets";
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

describe("HashtagWidget (ED-02a badge — no-innerHTML invariant)", () => {
  it("renders a <span class=cm-hashtag> displaying '#' + tag", () => {
    const dom = new HashtagWidget("deploy").toDOM();
    expect(dom.tagName).toBe("SPAN");
    expect(dom.className).toBe("cm-hashtag");
    // Obsidian-parity: the badge shows the leading '#' (a literal) + the label.
    expect(dom.textContent).toBe("#deploy");
  });

  it("builds DOM via createElement + textContent ONLY (no innerHTML) — markup stays inert", () => {
    // A tag carrying angle brackets must surface as a single inert text node, never
    // parsed markup (the security invariant: untrusted tag bytes can't become live
    // markup). Asserting a single text child + zero element children is the guard.
    const dom = new HashtagWidget("a<b>c").toDOM();
    expect(dom.tagName).toBe("SPAN");
    expect(dom.textContent).toBe("#a<b>c");
    expect(dom.children.length).toBe(0); // innerHTML would have parsed a <b> child
    expect(dom.childNodes.length).toBe(1); // exactly one node: the text node
    expect(dom.childNodes[0].nodeType).toBe(3); // Node.TEXT_NODE
  });

  it("eq() compares the tag label", () => {
    expect(new HashtagWidget("deploy").eq(new HashtagWidget("deploy"))).toBe(true);
    expect(new HashtagWidget("deploy").eq(new HashtagWidget("ops"))).toBe(false);
  });

  it("ignoreEvent() returns false so a click can place the cursor (reveal-to-source)", () => {
    expect(new HashtagWidget("deploy").ignoreEvent()).toBe(false);
  });
});

describe("CodeBlockWidget (ED-04a async syntax highlight — no-innerHTML + teardown-safe)", () => {
  // toDOM kicks off a real lazy grammar load; the re-highlight only writes when the
  // <code> is connected. So tests that exercise the async path attach the <pre> to
  // document.body and poll with vi.waitFor.
  function mount(code: string, lang: string): { pre: HTMLElement; w: CodeBlockWidget } {
    const w = new CodeBlockWidget(code, lang);
    const pre = w.toDOM();
    return { pre, w };
  }

  it("renders plain <pre><code> with textContent === code SYNCHRONOUSLY (no spans yet)", () => {
    const { pre } = mount("func main() {}", "go");
    expect(pre.tagName).toBe("PRE");
    expect(pre.className).toBe("cm-md-codeblock");
    expect(pre.getAttribute("data-lang")).toBe("go");
    const code = pre.querySelector("code")!;
    // Synchronous render is the plain baseline — one text node, no token spans.
    expect(code.textContent).toBe("func main() {}");
    expect(code.querySelectorAll("span").length).toBe(0);
  });

  it("re-highlights into tok-* <span>s once the grammar resolves (element structure, never innerHTML)", async () => {
    const code = "func main() { return }";
    const { pre } = mount(code, "go");
    document.body.appendChild(pre); // connected → re-highlight is allowed to write
    try {
      const codeEl = pre.querySelector("code")!;
      await vi.waitFor(() => {
        expect(codeEl.querySelectorAll("span.tok-keyword").length).toBeGreaterThan(0);
      });
      // Built via createElement + textContent: every child is a SPAN or a TEXT node,
      // never parsed markup. (Asserting node types is the T-06-08 guard.)
      for (const node of Array.from(codeEl.childNodes)) {
        expect([1, 3]).toContain(node.nodeType); // ELEMENT or TEXT only
        if (node.nodeType === 1) {
          expect((node as HTMLElement).tagName).toBe("SPAN");
          // a token span has no element children — its content is one inert text node
          expect((node as HTMLElement).children.length).toBe(0);
        }
      }
      // The concatenation of all token text round-trips the exact source bytes.
      expect(codeEl.textContent).toBe(code);
      // `go` is a keyword in the source → at least one tok-keyword span exists.
      expect(codeEl.querySelector("span.tok-keyword")).not.toBeNull();
      expect(pre.classList.contains("cm-md-codeblock--highlighted")).toBe(true);
    } finally {
      pre.remove();
    }
  });

  it("markup-looking code never becomes live DOM (textContent-only)", async () => {
    // Code that LOOKS like HTML must surface as text, never as parsed elements.
    const code = 'const x = "<script>alert(1)</script>";';
    const { pre } = mount(code, "javascript");
    document.body.appendChild(pre);
    try {
      const codeEl = pre.querySelector("code")!;
      await vi.waitFor(() => {
        expect(codeEl.querySelectorAll("span").length).toBeGreaterThan(0);
      });
      // No <script> element was ever created from the code bytes.
      expect(codeEl.querySelector("script")).toBeNull();
      // The literal text round-trips intact.
      expect(codeEl.textContent).toBe(code);
    } finally {
      pre.remove();
    }
  });

  it("an UNKNOWN language stays plain (no spans, no throw)", async () => {
    const { pre } = mount("some text", "totally-not-a-language");
    document.body.appendChild(pre);
    try {
      const codeEl = pre.querySelector("code")!;
      // Give any (non-existent) async path a chance — it must never add spans.
      await new Promise((r) => setTimeout(r, 30));
      expect(codeEl.querySelectorAll("span").length).toBe(0);
      expect(codeEl.textContent).toBe("some text");
      expect(pre.classList.contains("cm-md-codeblock--highlighted")).toBe(false);
    } finally {
      pre.remove();
    }
  });

  it("a ```mermaid fence is NOT highlighted (left plain — Phase 26)", async () => {
    const { pre } = mount("graph TD; A-->B;", "mermaid");
    document.body.appendChild(pre);
    try {
      const codeEl = pre.querySelector("code")!;
      await new Promise((r) => setTimeout(r, 30));
      expect(codeEl.querySelectorAll("span").length).toBe(0);
      expect(codeEl.textContent).toBe("graph TD; A-->B;");
    } finally {
      pre.remove();
    }
  });

  it("teardown-safe: destroy() before the grammar resolves → no write, no throw", async () => {
    const code = "func main() {}";
    const { pre, w } = mount(code, "go");
    document.body.appendChild(pre);
    try {
      const codeEl = pre.querySelector("code")!;
      // Tear down IMMEDIATELY — before the async grammar import can resolve.
      w.destroy();
      // Wait well past when the load would have resolved.
      await new Promise((r) => setTimeout(r, 60));
      // The guard must have skipped the write: still plain text, no spans.
      expect(codeEl.querySelectorAll("span").length).toBe(0);
      expect(codeEl.textContent).toBe(code);
    } finally {
      pre.remove();
    }
  });

  it("teardown-safe: a detached <code> (isConnected false) is never written into", async () => {
    const code = "def f(): pass";
    const { pre } = mount(code, "python");
    // Never attached to document → codeEl.isConnected stays false through the load.
    const codeEl = pre.querySelector("code")!;
    await new Promise((r) => setTimeout(r, 60));
    expect(codeEl.querySelectorAll("span").length).toBe(0);
    expect(codeEl.textContent).toBe(code);
  });

  it("eq() compares code + lang (constructor unchanged)", () => {
    const a = new CodeBlockWidget("x", "go");
    const b = new CodeBlockWidget("x", "go");
    const c = new CodeBlockWidget("x", "rust");
    expect(a.eq(b)).toBe(true);
    expect(a.eq(c)).toBe(false);
  });
});
