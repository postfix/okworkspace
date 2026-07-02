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
import { describe, it, expect, vi, beforeEach } from "vitest";
import {
  CodeBlockWidget,
  HashtagWidget,
  ImageWidget,
  MermaidWidget,
  TableWidget,
} from "./widgets";
import type { TableData } from "./widgets";
import { renderMermaid } from "./mermaid";

// MermaidWidget exercises ONLY our DOM handling — mermaid's real renderer cannot run
// under jsdom (no getBBox), so the lazy wrapper is mocked. The widget then sees a
// resolved svg STRING / a rejection, and we assert the inject-vs-fallback behaviour.
vi.mock("./mermaid", () => ({ renderMermaid: vi.fn() }));
const renderMermaidMock = vi.mocked(renderMermaid);
beforeEach(() => {
  renderMermaidMock.mockReset();
});

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

  it("has a copy button (EP-02) — createElement only, no innerHTML, copies this.code", async () => {
    const code = "const x = 1;";
    const { pre } = mount(code, "go");
    const btn = pre.querySelector("button.cm-copy-btn")!;
    expect(btn).not.toBeNull();
    expect(btn.textContent).toBe("Copy");
    // The button is a sibling of <code>, not inside it — <code> stays clean.
    const codeEl = pre.querySelector("code")!;
    expect(codeEl.contains(btn)).toBe(false);
    // Mock clipboard.writeText and click.
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(globalThis.navigator, "clipboard", {
      value: { writeText },
      configurable: true,
    });
    btn.click();
    // The clipboard call received the code.
    expect(writeText).toHaveBeenCalledWith(code);
    // Wait for the "Copied" confirmation + reset (vi.waitFor polls).
    await vi.waitFor(() => {
      expect(btn.textContent).toBe("Copied");
    });
    await vi.waitFor(() => {
      expect(btn.textContent).toBe("Copy");
    }, { timeout: 3000 });
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

describe("MermaidWidget (ED-05a — strict SVG inject + raw-code fallback, teardown-safe)", () => {
  const CODE = "graph TD; A-->B;";
  // A well-formed SVG string (xmlns so it parses as image/svg+xml cleanly), carrying a
  // child so we can assert structure was preserved through DOMParser + importNode.
  const CANNED_SVG =
    '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><g class="node"></g></svg>';

  it("renders the raw-code fallback (a CodeBlockWidget <pre>) SYNCHRONOUSLY — no svg yet", () => {
    // Never resolves → the synchronous fallback is all that exists at toDOM time.
    renderMermaidMock.mockReturnValue(new Promise<string>(() => {}));
    const wrapper = new MermaidWidget(CODE).toDOM();
    expect(wrapper.tagName).toBe("DIV");
    expect(wrapper.className).toBe("cm-md-mermaid");
    const pre = wrapper.querySelector("pre.cm-md-codeblock")!;
    expect(pre).not.toBeNull();
    // The fallback shows the exact fenced code the user typed, via textContent.
    expect(pre.querySelector("code")!.textContent).toBe(CODE);
    // No diagram has been injected synchronously.
    expect(wrapper.querySelector("svg")).toBeNull();
  });

  it("injects the parsed <svg> (element structure, never innerHTML) once render resolves", async () => {
    renderMermaidMock.mockResolvedValue(CANNED_SVG);
    const w = new MermaidWidget(CODE);
    const wrapper = w.toDOM();
    document.body.appendChild(wrapper); // connected → the async swap is allowed to write
    try {
      await vi.waitFor(() => {
        expect(wrapper.querySelector("svg")).not.toBeNull();
      });
      const svg = wrapper.querySelector("svg")!;
      // Parsed as a REAL svg element (namespace + tagName), not a text/innerHTML blob.
      expect(svg.tagName.toLowerCase()).toBe("svg");
      expect(svg.namespaceURI).toBe("http://www.w3.org/2000/svg");
      // Child structure survived DOMParser + importNode (proves a parsed tree, not a string).
      expect(svg.querySelector("g.node")).not.toBeNull();
      // Sizing constraint applied (blunts layout shift).
      expect(svg.getAttribute("style")).toContain("max-width:100%");
      // The raw-code fallback was atomically replaced — the <pre> is gone.
      expect(wrapper.querySelector("pre.cm-md-codeblock")).toBeNull();
      expect(wrapper.classList.contains("cm-md-mermaid--rendered")).toBe(true);
    } finally {
      wrapper.remove();
    }
  });

  it("a render REJECTION (invalid diagram) keeps the raw-code fallback — no throw, no svg", async () => {
    renderMermaidMock.mockRejectedValue(new Error("Parse error on line 1"));
    const wrapper = new MermaidWidget("not a diagram").toDOM();
    document.body.appendChild(wrapper);
    try {
      // Give the rejected promise time to settle.
      await new Promise((r) => setTimeout(r, 30));
      // Fallback intact, no diagram, no error-svg.
      expect(wrapper.querySelector("svg")).toBeNull();
      const pre = wrapper.querySelector("pre.cm-md-codeblock")!;
      expect(pre.querySelector("code")!.textContent).toBe("not a diagram");
    } finally {
      wrapper.remove();
    }
  });

  it("a parsererror svg (malformed XML) keeps the raw-code fallback — never injects broken markup", async () => {
    // Mismatched tags → DOMParser yields a <parsererror> document.
    renderMermaidMock.mockResolvedValue("<svg><g></svg>");
    const wrapper = new MermaidWidget(CODE).toDOM();
    document.body.appendChild(wrapper);
    try {
      await new Promise((r) => setTimeout(r, 30));
      // No <svg> injected; the fallback <pre> remains.
      expect(wrapper.querySelector("svg")).toBeNull();
      expect(wrapper.querySelector("pre.cm-md-codeblock")).not.toBeNull();
    } finally {
      wrapper.remove();
    }
  });

  it("a non-<svg> root (e.g. a <div>) keeps the fallback (defends against a mangled result)", async () => {
    renderMermaidMock.mockResolvedValue(
      '<div xmlns="http://www.w3.org/1999/xhtml">nope</div>',
    );
    const wrapper = new MermaidWidget(CODE).toDOM();
    document.body.appendChild(wrapper);
    try {
      await new Promise((r) => setTimeout(r, 30));
      expect(wrapper.querySelector("svg")).toBeNull();
      expect(wrapper.querySelector("pre.cm-md-codeblock")).not.toBeNull();
    } finally {
      wrapper.remove();
    }
  });

  it("teardown-safe: destroy() before render resolves → no write into the (detached) node", async () => {
    // Resolve only AFTER we have destroyed the widget.
    let resolve!: (svg: string) => void;
    renderMermaidMock.mockReturnValue(
      new Promise<string>((r) => {
        resolve = r;
      }),
    );
    const w = new MermaidWidget(CODE);
    const wrapper = w.toDOM();
    document.body.appendChild(wrapper);
    try {
      w.destroy(); // cursor entered the block / text changed mid-render
      resolve(CANNED_SVG); // the in-flight render now resolves
      await new Promise((r) => setTimeout(r, 30));
      // The destroyed guard discarded the result — fallback intact, no svg.
      expect(wrapper.querySelector("svg")).toBeNull();
      expect(wrapper.querySelector("pre.cm-md-codeblock")).not.toBeNull();
    } finally {
      wrapper.remove();
    }
  });

  it("stale-safe: a detached wrapper (isConnected false) is never written into", async () => {
    renderMermaidMock.mockResolvedValue(CANNED_SVG);
    // Never attached → wrapper.isConnected stays false through the resolve.
    const wrapper = new MermaidWidget(CODE).toDOM();
    await new Promise((r) => setTimeout(r, 30));
    expect(wrapper.querySelector("svg")).toBeNull();
    expect(wrapper.querySelector("pre.cm-md-codeblock")).not.toBeNull();
  });

  it("eq() compares (code, theme) so an unchanged diagram is not re-rendered", () => {
    const a = new MermaidWidget("graph TD; A-->B;", "dark");
    const b = new MermaidWidget("graph TD; A-->B;", "dark");
    const c = new MermaidWidget("graph TD; A-->C;", "dark");
    const d = new MermaidWidget("graph TD; A-->B;", "light");
    expect(a.eq(b)).toBe(true);
    expect(a.eq(c)).toBe(false); // code changed
    expect(a.eq(d)).toBe(false); // theme changed
  });

  it("ignoreEvent() returns false so a click can place the cursor (reveal-to-source)", () => {
    expect(new MermaidWidget(CODE).ignoreEvent()).toBe(false);
  });
});
