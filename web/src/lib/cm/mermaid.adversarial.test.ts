// ADVERSARIAL suite for the ED-05a mermaid render path — MermaidWidget (widgets.ts)
// + its renderMermaid collaborator (mermaid.ts) + the livePreview lazy boundary.
//
// This is the INDEPENDENT adversarial complement to widgets.test.ts /
// mermaid.test.ts — it attacks the four invariants the feature claims, at the
// boundaries the author's own happy-path tests don't reach. It does NOT modify the
// existing suites.
//
// mermaid's REAL renderer cannot run under jsdom (no getBBox), so — exactly like
// widgets.test.ts — we MOCK `./mermaid` and feed canned / hostile svg strings (or
// rejections) into MermaidWidget to attack OUR DOM-handling: the DOMParser +
// importNode + appendChild inject path that is the widget's last line of defense.
// (We also import the REAL renderMermaid source as ?raw for the structural lazy
// assertion — that needs the file text, not the mock.)
//
// FOUR INVARIANTS UNDER ATTACK
//   (a) NO-innerHTML / hostile-svg inject — a malicious svg (embedded <script>,
//       onload/onerror, foreignObject <img onerror>, javascript:/xlink:href, CDATA
//       script, entity-encoded payload, parsererror-inducing/non-svg-root) must
//       inject ONLY via real parsed nodes (never innerHTML of page content), must
//       NEVER execute a handler under our harness, and a parsererror / non-svg root
//       must keep the raw-code fallback. (See FINDINGS at the bottom: the widget is
//       NOT itself a sanitizer — its no-script guarantee is the UPSTREAM DOMPurify in
//       renderMermaid; this suite pins the boundary the widget actually owns.)
//   (b) Error / invalid diagram — a renderMermaid REJECTION, or a resolved
//       empty/blank/garbage svg, keeps the raw-code fallback (verbatim code via
//       textContent), never blank, no throw escaping toDOM, no unhandled rejection,
//       and no stray temp node left on document.body.
//   (c) Teardown / stale — destroy() before/after resolve, a detached wrapper at
//       resolve, the eq()-driven instance churn, and TWO toDOM()s on one instance
//       must never throw, never write into a stale/detached node, never cross-write.
//   (d) Lazy guarantee — mermaid.ts carries NO static `import … "mermaid"` (only the
//       dynamic `import("mermaid")`); a static import would defeat assertLazyChunks.
//
// ORACLES: structural only (tagName / namespaceURI / childNodes / textContent), the
// minimum bar (no throw, no unhandled rejection), and a global execution sentinel
// that must stay UNSET — never an innerHTML string comparison.
import { beforeEach, describe, expect, it, vi } from "vitest";
import { MermaidWidget } from "./widgets";
import { renderMermaid } from "./mermaid";

// Mock the wrapper exactly as widgets.test.ts does — we drive the widget's DOM
// handling with canned svg strings / rejections, never the real (jsdom-incapable)
// mermaid renderer.
vi.mock("./mermaid", () => ({ renderMermaid: vi.fn() }));
const renderMermaidMock = vi.mocked(renderMermaid);

const SVG_NS = "http://www.w3.org/2000/svg";
const NS = `xmlns="${SVG_NS}"`;
const CODE = "graph TD; A-->B;";

// A global the test points hostile handler attributes at. If ANY inline handler ever
// executed (onload/onerror/javascript:), this flips — the security sentinel.
declare global {
  var __mermaidPwn: number | undefined;
}

beforeEach(() => {
  renderMermaidMock.mockReset();
  delete globalThis.__mermaidPwn;
});

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// Mount a widget and (by default) attach the wrapper so the async swap is allowed to
// write (the guard requires wrapper.isConnected).
function mount(
  code = CODE,
  attach = true,
): { wrapper: HTMLElement; w: MermaidWidget } {
  const w = new MermaidWidget(code);
  const wrapper = w.toDOM();
  if (attach) document.body.appendChild(wrapper);
  return { wrapper, w };
}

// Capture unhandled promise rejections: the widget fires `void this.render(...)`, so
// any throw it does NOT swallow surfaces as an unhandled rejection — exactly what an
// adversary wants and what these traps detect.
function installRejectionTrap(): { reasons: unknown[]; uninstall: () => void } {
  const reasons: unknown[] = [];
  const onProc = (r: unknown) => reasons.push(r);
  const onWin = (e: PromiseRejectionEvent) => reasons.push(e.reason);
  process.on("unhandledRejection", onProc);
  window.addEventListener("unhandledrejection", onWin);
  return {
    reasons,
    uninstall: () => {
      process.off("unhandledRejection", onProc);
      window.removeEventListener("unhandledrejection", onWin);
    },
  };
}

// The raw-code fallback is a CodeBlockWidget <pre.cm-md-codeblock> whose <code> holds
// the verbatim fenced code via textContent. "Fallback intact" === that <pre> present
// AND no <svg> injected.
function assertFallback(wrapper: HTMLElement, code = CODE): void {
  const pre = wrapper.querySelector("pre.cm-md-codeblock");
  expect(pre).not.toBeNull();
  expect(pre!.querySelector("code")!.textContent).toBe(code);
  expect(wrapper.querySelector("svg")).toBeNull();
  expect(wrapper.classList.contains("cm-md-mermaid--rendered")).toBe(false);
}

// Let microtasks + the post-await guards settle (the render is a 1-await pass).
const settle = () => new Promise((r) => setTimeout(r, 30));

// ===========================================================================
// INVARIANT (a) — hostile-svg inject: parsed nodes only, never executes,
//                 parsererror / non-svg root → fallback
// ===========================================================================
describe("INV-(a) hostile svg inject — parsed-node mechanism, no execution, safe fallback", () => {
  // Payloads that DO parse as a well-formed <svg> root (so the widget injects them):
  // each carries a handler/script aimed at the __mermaidPwn sentinel. None may execute
  // under our harness, and the injection must be via a REAL parsed svg Element.
  const HOSTILE_BUT_VALID_ROOT: Array<{ name: string; svg: string }> = [
    {
      name: "embedded <script> in svg",
      svg: `<svg ${NS}><script>globalThis.__mermaidPwn=1</script><g class="node"/></svg>`,
    },
    {
      name: "svg root onload attribute",
      svg: `<svg ${NS} onload="globalThis.__mermaidPwn=1"><g class="node"/></svg>`,
    },
    {
      name: "foreignObject with <img onerror>",
      svg: `<svg ${NS}><foreignObject><img src="x" onerror="globalThis.__mermaidPwn=1"/></foreignObject></svg>`,
    },
    {
      name: "anchor xlink:href javascript:",
      svg: `<svg ${NS} xmlns:xlink="http://www.w3.org/1999/xlink"><a xlink:href="javascript:globalThis.__mermaidPwn=1"><text>x</text></a></svg>`,
    },
    {
      name: "anchor href javascript: (svg2)",
      svg: `<svg ${NS}><a href="javascript:globalThis.__mermaidPwn=1"><text>x</text></a></svg>`,
    },
    {
      name: "image with onload + javascript href",
      svg: `<svg ${NS}><image href="javascript:globalThis.__mermaidPwn=1" onload="globalThis.__mermaidPwn=1"/></svg>`,
    },
    {
      name: "entity-encoded &lt;script&gt; stays inert text",
      svg: `<svg ${NS}><text>&lt;script&gt;globalThis.__mermaidPwn=1&lt;/script&gt;</text></svg>`,
    },
    {
      name: "style with css expression-ish payload",
      svg: `<svg ${NS}><style>* { background: url("javascript:globalThis.__mermaidPwn=1") }</style><g/></svg>`,
    },
  ];

  it.each(HOSTILE_BUT_VALID_ROOT)(
    "$name → injected as a parsed <svg> node, NO handler executes",
    async ({ svg }) => {
      const trap = installRejectionTrap();
      renderMermaidMock.mockResolvedValue(svg);
      const { wrapper } = mount();
      try {
        await vi.waitFor(() =>
          expect(wrapper.querySelector("svg")).not.toBeNull(),
        );
        const el = wrapper.querySelector("svg")!;
        // MECHANISM: a real parsed SVG element (namespace + tagName), proving the
        // inject went through DOMParser + importNode, NOT innerHTML of a string blob.
        expect(el.tagName.toLowerCase()).toBe("svg");
        expect(el.namespaceURI).toBe(SVG_NS);
        // EXECUTION: no inline handler / script / javascript: URL ever ran.
        expect(globalThis.__mermaidPwn).toBeUndefined();
        // No throw leaked out of the fire-and-forget render.
        expect(trap.reasons).toEqual([]);
      } finally {
        trap.uninstall();
        wrapper.remove();
      }
    },
  );

  // CDATA-wrapped script: DOMParser(image/svg+xml) yields a CDATASection node; HTML
  // documents (jsdom / the live page) CANNOT hold a CDATA section, so importNode THROWS
  // `NotSupportedError`. This is a SYNCHRONOUS DOM-step throw — distinct from the
  // awaited renderMermaid rejection the existing tests cover. The widget's whole-pass
  // try/catch must swallow it and keep the fallback (no throw, no unhandled rejection,
  // no half-injected DOM). Previously-untested gap.
  it("CDATA-wrapped script makes importNode throw → caught, fallback intact, no leak", async () => {
    const trap = installRejectionTrap();
    renderMermaidMock.mockResolvedValue(
      `<svg ${NS}><script><![CDATA[globalThis.__mermaidPwn=1]]></script></svg>`,
    );
    const { wrapper } = mount();
    try {
      await settle();
      // The importNode throw was swallowed: no svg, the <pre> fallback survives.
      assertFallback(wrapper);
      expect(globalThis.__mermaidPwn).toBeUndefined();
      expect(trap.reasons).toEqual([]);
    } finally {
      trap.uninstall();
      wrapper.remove();
    }
  });

  // Inputs that DON'T yield a well-formed <svg> documentElement → the parsererror /
  // non-svg-root guard must keep the fallback. Every byte of these is attacker-chosen.
  const REJECTED_ROOTS: Array<{ name: string; svg: string }> = [
    { name: "mismatched tags (parsererror)", svg: "<svg><g></svg>" },
    { name: "unterminated tag (parsererror)", svg: `<svg ${NS}><g` },
    { name: "undefined entity (parsererror)", svg: `<svg ${NS}>&nope;</svg>` },
    { name: "non-svg root: xhtml div", svg: `<div xmlns="http://www.w3.org/1999/xhtml">x</div>` },
    { name: "non-svg root: html", svg: `<html xmlns="http://www.w3.org/1999/xhtml"><body/></html>` },
    { name: "non-svg root: arbitrary element", svg: `<evil ${NS.replace("svg", "evil")}>x</evil>` },
    { name: "empty string", svg: "" },
    { name: "whitespace only", svg: "   \n\t " },
    { name: "comment only (no documentElement)", svg: "<!-- just a comment -->" },
    { name: "bare text, no element", svg: "not xml at all" },
    { name: "processing instruction only", svg: `<?xml-stylesheet href="x"?>` },
    {
      name: "<script> as the ROOT (no svg wrapper)",
      svg: `<script xmlns="${SVG_NS}">globalThis.__mermaidPwn=1</script>`,
    },
  ];

  it.each(REJECTED_ROOTS)(
    "$name → keeps the raw-code fallback (no inject, no throw, no execution)",
    async ({ svg }) => {
      const trap = installRejectionTrap();
      renderMermaidMock.mockResolvedValue(svg);
      const { wrapper } = mount();
      try {
        await settle();
        assertFallback(wrapper);
        expect(globalThis.__mermaidPwn).toBeUndefined();
        expect(trap.reasons).toEqual([]);
      } finally {
        trap.uninstall();
        wrapper.remove();
      }
    },
  );

  // A DOCTYPE-with-entities (billion-laughs-shaped) svg must NOT hang or exponentially
  // expand. jsdom's XML parser does not expand the nested entities — the diagram either
  // falls back or renders a tiny tree, but the call returns promptly. Bounds the "no
  // input hangs" hard bar for the inject path.
  it("billion-laughs DOCTYPE svg does not hang or exponentially expand", async () => {
    const trap = installRejectionTrap();
    const lol =
      `<!DOCTYPE svg [` +
      `<!ENTITY a "aaaaaaaaaa">` +
      `<!ENTITY b "&a;&a;&a;&a;&a;&a;&a;&a;&a;&a;">` +
      `<!ENTITY c "&b;&b;&b;&b;&b;&b;&b;&b;&b;&b;">` +
      `<!ENTITY d "&c;&c;&c;&c;&c;&c;&c;&c;&c;&c;">` +
      `]><svg ${NS}><text>&d;</text></svg>`;
    renderMermaidMock.mockResolvedValue(lol);
    const { wrapper } = mount();
    try {
      const t0 = Date.now();
      await settle();
      // Whatever the disposition (fallback or a tiny tree), the text never blew up to
      // millions of chars and the pass returned fast.
      expect(Date.now() - t0).toBeLessThan(2000);
      expect((wrapper.textContent ?? "").length).toBeLessThan(10000);
      expect(trap.reasons).toEqual([]);
    } finally {
      trap.uninstall();
      wrapper.remove();
    }
  });

  // FINDING pin (reproduced, dispositioned below): when the upstream DOMPurify guarantee
  // is removed (simulated here by the mock returning a raw <script> svg), the widget DOES
  // append the parsed <script> node — it is NOT itself a script-stripping sanitizer. The
  // security-relevant property that HOLDS is that the script never EXECUTES (sentinel
  // unset). This test documents the real boundary so a regression (e.g. someone switching
  // to innerHTML, or removing the upstream sanitize) is noticed.
  it("DOCUMENTS the boundary: the widget passes a parsed <script> node through (does NOT strip), but it never executes", async () => {
    renderMermaidMock.mockResolvedValue(
      `<svg ${NS}><script>globalThis.__mermaidPwn=1</script></svg>`,
    );
    const { wrapper } = mount();
    try {
      await vi.waitFor(() => expect(wrapper.querySelector("svg")).not.toBeNull());
      // The node IS present — the widget relies on the UPSTREAM DOMPurify (strict) to
      // have stripped it in production; see FINDINGS at the file foot.
      expect(wrapper.querySelector("script")).not.toBeNull();
      // …but it never executed (DOMParser-parsed + no runScripts; the property the
      // app's safety actually rests on at this layer).
      expect(globalThis.__mermaidPwn).toBeUndefined();
    } finally {
      wrapper.remove();
    }
  });
});

// ===========================================================================
// INVARIANT (b) — error / invalid diagram → fallback, never blank, no leak,
//                 no stray temp node on document.body
// ===========================================================================
describe("INV-(b) error / invalid → raw-code fallback, no leak, no stray DOM", () => {
  // Every rejection shape: Error, plain string, null, a non-Error object. All must be
  // swallowed → fallback intact, no unhandled rejection.
  const REJECTIONS: Array<{ name: string; reason: unknown }> = [
    { name: "Error('Parse error on line 1')", reason: new Error("Parse error on line 1") },
    { name: "Error('render blew up')", reason: new Error("render blew up") },
    { name: "thrown string", reason: "syntax error string" },
    { name: "thrown null", reason: null },
    { name: "thrown plain object", reason: { code: "EBADDIAGRAM" } },
  ];

  it.each(REJECTIONS)(
    "renderMermaid rejects with $name → fallback intact, no unhandled rejection",
    async ({ reason }) => {
      const trap = installRejectionTrap();
      renderMermaidMock.mockRejectedValue(reason);
      const { wrapper } = mount("not a diagram");
      try {
        await settle();
        assertFallback(wrapper, "not a diagram");
        expect(trap.reasons).toEqual([]);
      } finally {
        trap.uninstall();
        wrapper.remove();
      }
    },
  );

  // Resolved-but-useless svg strings (mermaid could in principle hand back junk).
  const JUNK_RESOLVES: Array<{ name: string; svg: unknown }> = [
    { name: "empty string", svg: "" },
    { name: "blank whitespace", svg: "  \n  " },
    { name: "the literal 'undefined'", svg: "undefined" },
    { name: "non-string null (off-contract)", svg: null },
    { name: "non-string undefined (off-contract)", svg: undefined },
    { name: "non-string number (off-contract)", svg: 42 },
  ];

  it.each(JUNK_RESOLVES)(
    "renderMermaid resolves $name → fallback intact, never blank, no throw",
    async ({ svg }) => {
      const trap = installRejectionTrap();
      // Cast: the off-contract non-string cases probe defensive robustness of the path.
      renderMermaidMock.mockResolvedValue(svg as unknown as string);
      const { wrapper } = mount();
      try {
        await settle();
        // Never blank: the fallback <pre> + verbatim code survive any junk resolve.
        assertFallback(wrapper);
        expect(trap.reasons).toEqual([]);
      } finally {
        trap.uninstall();
        wrapper.remove();
      }
    },
  );

  it("a render pass leaves NO stray temp node on document.body (only our wrapper)", async () => {
    renderMermaidMock.mockRejectedValue(new Error("boom"));
    const before = document.body.childNodes.length;
    const { wrapper } = mount();
    try {
      await settle();
      // The widget appended exactly ONE node (our wrapper); the failed render created
      // no orphan div on the body (suppressErrorRendering owns the real mermaid case,
      // but the widget itself must never leak either).
      expect(document.body.childNodes.length).toBe(before + 1);
      expect(document.body.contains(wrapper)).toBe(true);
    } finally {
      wrapper.remove();
    }
    expect(document.body.childNodes.length).toBe(before);
  });

  // The fallback must reproduce the EXACT fenced code (incl. markup-shaped, multi-line,
  // and very long) via textContent — never parsed as markup, never truncated.
  const FALLBACK_CODES: Array<{ name: string; code: string }> = [
    { name: "markup-shaped diagram source", code: "graph TD; A[\"</svg><img src=x onerror=alert(1)>\"]-->B" },
    { name: "multi-line with angle brackets", code: "sequenceDiagram\n  A->>B: <hi>\n  B-->>A: </hi>" },
    { name: "empty diagram body", code: "" },
    { name: "very long single line (~60k)", code: "graph LR;" + "A-->B;".repeat(10000) },
  ];

  it.each(FALLBACK_CODES)(
    "$name renders verbatim in the fallback <code> (textContent, never markup)",
    async ({ code }) => {
      renderMermaidMock.mockRejectedValue(new Error("invalid"));
      const { wrapper } = mount(code);
      try {
        await settle();
        const codeEl = wrapper.querySelector("pre.cm-md-codeblock code")!;
        expect(codeEl.textContent).toBe(code);
        // No markup ever parsed out of the diagram source.
        expect(wrapper.querySelector("img")).toBeNull();
        expect(wrapper.querySelector("svg")).toBeNull();
      } finally {
        wrapper.remove();
      }
    },
  );
});

// ===========================================================================
// INVARIANT (c) — teardown / stale: destroy, detach, churn, double-toDOM
// ===========================================================================
describe("INV-(c) teardown / stale — no throw, no write into a stale node, no cross-write", () => {
  const CANNED_SVG = `<svg ${NS} viewBox="0 0 10 10"><g class="node"></g></svg>`;

  it("destroy() AFTER a completed swap → svg remains, destroy() does not throw or mutate", async () => {
    renderMermaidMock.mockResolvedValue(CANNED_SVG);
    const { wrapper, w } = mount();
    try {
      await vi.waitFor(() => expect(wrapper.querySelector("svg")).not.toBeNull());
      expect(() => w.destroy()).not.toThrow();
      // destroy() only flips a flag — the already-rendered svg is untouched.
      expect(wrapper.querySelector("svg")).not.toBeNull();
      expect(wrapper.querySelector("pre.cm-md-codeblock")).toBeNull();
    } finally {
      wrapper.remove();
    }
  });

  it("attach → DETACH before resolve (isConnected false) → never written into, no throw", async () => {
    // Connected at toDOM time, then detached synchronously; the post-await isConnected
    // guard must drop the write. (Existing test never attaches; this attaches first.)
    const trap = installRejectionTrap();
    let resolve!: (svg: string) => void;
    renderMermaidMock.mockReturnValue(new Promise<string>((r) => (resolve = r)));
    const { wrapper } = mount();
    try {
      wrapper.remove(); // detach BEFORE the render resolves
      resolve(CANNED_SVG);
      await settle();
      // Detached → the guard skipped the swap; the wrapper still holds only the fallback.
      expect(wrapper.querySelector("svg")).toBeNull();
      expect(wrapper.querySelector("pre.cm-md-codeblock")).not.toBeNull();
      expect(trap.reasons).toEqual([]);
    } finally {
      trap.uninstall();
    }
  });

  it("destroy() THEN re-attach before resolve → destroyed flag still wins (no write)", async () => {
    // Even if the node is connected again at resolve time, the per-instance destroyed
    // flag must veto the write (mirrors the eq()-churn: the old instance is dead).
    const trap = installRejectionTrap();
    let resolve!: (svg: string) => void;
    renderMermaidMock.mockReturnValue(new Promise<string>((r) => (resolve = r)));
    const { wrapper, w } = mount();
    try {
      w.destroy();
      document.body.appendChild(wrapper); // re-connected, but instance is destroyed
      resolve(CANNED_SVG);
      await settle();
      expect(wrapper.querySelector("svg")).toBeNull();
      expect(wrapper.querySelector("pre.cm-md-codeblock")).not.toBeNull();
      expect(trap.reasons).toEqual([]);
    } finally {
      trap.uninstall();
      wrapper.remove();
    }
  });

  it("TWO toDOM()s on one instance, both resolve → independent wrappers, no cross-write", async () => {
    // CM6 may build a widget's DOM more than once; each toDOM() makes its own wrapper and
    // the render closure captures THAT wrapper. Two concurrent resolves must each land in
    // their own wrapper — never blank or double the other.
    renderMermaidMock.mockResolvedValue(CANNED_SVG);
    const w = new MermaidWidget(CODE);
    const wrapperA = w.toDOM();
    const wrapperB = w.toDOM();
    document.body.appendChild(wrapperA);
    document.body.appendChild(wrapperB);
    try {
      await vi.waitFor(() => {
        expect(wrapperA.querySelector("svg")).not.toBeNull();
        expect(wrapperB.querySelector("svg")).not.toBeNull();
      });
      // Each wrapper holds exactly ONE svg and no leftover fallback — no cross-write.
      expect(wrapperA.querySelectorAll("svg").length).toBe(1);
      expect(wrapperB.querySelectorAll("svg").length).toBe(1);
      expect(wrapperA.querySelector("pre.cm-md-codeblock")).toBeNull();
      expect(wrapperB.querySelector("pre.cm-md-codeblock")).toBeNull();
    } finally {
      wrapperA.remove();
      wrapperB.remove();
    }
  });

  it("destroy() on one of two toDOM() wrappers stops BOTH (shared instance flag) — documents the churn semantics", async () => {
    // destroyed is per-INSTANCE, shared across that instance's toDOM() outputs. So a
    // destroy() before resolve halts every in-flight render from the instance. (In CM6,
    // eq()-churn replaces the whole instance, so this is the intended unit of teardown.)
    let resolve!: (svg: string) => void;
    renderMermaidMock.mockReturnValue(new Promise<string>((r) => (resolve = r)));
    const w = new MermaidWidget(CODE);
    const wrapperA = w.toDOM();
    const wrapperB = w.toDOM();
    document.body.appendChild(wrapperA);
    document.body.appendChild(wrapperB);
    try {
      w.destroy();
      resolve(CANNED_SVG);
      await settle();
      expect(wrapperA.querySelector("svg")).toBeNull();
      expect(wrapperB.querySelector("svg")).toBeNull();
    } finally {
      wrapperA.remove();
      wrapperB.remove();
    }
  });

  it("a throwing view.requestMeasure is swallowed and the svg stays mounted", async () => {
    // requestMeasure runs AFTER the atomic swap, inside the try. If the host view throws
    // there, it must not escape and must not undo the already-mounted diagram.
    renderMermaidMock.mockResolvedValue(CANNED_SVG);
    const trap = installRejectionTrap();
    const view = {
      requestMeasure: () => {
        throw new Error("measure boom");
      },
    } as unknown as import("@codemirror/view").EditorView;
    const w = new MermaidWidget(CODE);
    const wrapper = w.toDOM(view);
    document.body.appendChild(wrapper);
    try {
      await vi.waitFor(() => expect(wrapper.querySelector("svg")).not.toBeNull());
      // Swap survived the throw; nothing leaked.
      expect(wrapper.querySelector("svg")).not.toBeNull();
      expect(trap.reasons).toEqual([]);
    } finally {
      trap.uninstall();
      wrapper.remove();
    }
  });

  it("view.requestMeasure IS called after a successful swap (re-measure contract)", async () => {
    renderMermaidMock.mockResolvedValue(CANNED_SVG);
    const requestMeasure = vi.fn();
    const view = { requestMeasure } as unknown as import("@codemirror/view").EditorView;
    const w = new MermaidWidget(CODE);
    const wrapper = w.toDOM(view);
    document.body.appendChild(wrapper);
    try {
      await vi.waitFor(() => expect(wrapper.querySelector("svg")).not.toBeNull());
      expect(requestMeasure).toHaveBeenCalled();
    } finally {
      wrapper.remove();
    }
  });

  // eq() adversarial table — the churn key. Whitespace and theme are SIGNIFICANT (a
  // one-byte edit must re-render); identical (code, theme) reuse the DOM.
  it("eq() is byte- and theme-exact (adversarial pairs)", () => {
    const pairs: Array<{ a: MermaidWidget; b: MermaidWidget; equal: boolean }> = [
      { a: new MermaidWidget("graph TD; A-->B;"), b: new MermaidWidget("graph TD; A-->B;"), equal: true },
      { a: new MermaidWidget("graph TD; A-->B;"), b: new MermaidWidget("graph TD; A-->B; "), equal: false }, // trailing space
      { a: new MermaidWidget("graph TD; A-->B;"), b: new MermaidWidget("graph TD;\nA-->B;"), equal: false }, // newline
      { a: new MermaidWidget("", "dark"), b: new MermaidWidget("", "dark"), equal: true }, // empty code
      { a: new MermaidWidget(CODE, "dark"), b: new MermaidWidget(CODE, "light"), equal: false }, // theme
      { a: new MermaidWidget(CODE), b: new MermaidWidget(CODE, "dark"), equal: true }, // default theme === "dark"
    ];
    for (const { a, b, equal } of pairs) {
      expect(a.eq(b)).toBe(equal);
      expect(b.eq(a)).toBe(equal); // symmetry
    }
  });
});

// ===========================================================================
// INVARIANT (d) — the lazy guarantee, asserted structurally on the REAL source
// ===========================================================================
describe("INV-(d) lazy boundary — mermaid.ts has NO static mermaid import", () => {
  it("mermaid.ts: only the dynamic import('mermaid'), never a static import/import type", async () => {
    const src = (await import("./mermaid.ts?raw")).default;
    // A static `import … from "mermaid"` (value OR type-only) would pull mermaid + its
    // heavy transitive deps (d3/dagre/cytoscape/dompurify/…) into the entry chunk —
    // exactly what assertLazyChunks (vite.config.ts) forbids.
    const staticImport = /^import\s+[^;]*\bfrom\s+["']mermaid["']/m;
    const staticTypeImport = /^import\s+type\s+[^;]*\bfrom\s+["']mermaid["']/m;
    expect(staticImport.test(src)).toBe(false);
    expect(staticTypeImport.test(src)).toBe(false);
    // The dynamic import IS present, and EVERY bare-"mermaid" reference in real code is
    // a dynamic import() argument — i.e. none is an import/require pulling the module
    // statically. (We strip line comments first so the header's prose `import("mermaid")`
    // mention doesn't count.)
    expect(/import\(["']mermaid["']\)/.test(src)).toBe(true);
    const codeOnly = src.replace(/\/\/.*$/gm, "");
    const bareMermaidRefs = codeOnly.match(/["']mermaid["']/g) ?? [];
    expect(bareMermaidRefs.length).toBe(1); // only the dynamic import()'s argument
    expect(/\bimport\(["']mermaid["']\)/.test(codeOnly)).toBe(true);
    // No `require("mermaid")` / static specifier slipped in either.
    expect(/require\(\s*["']mermaid["']\s*\)/.test(codeOnly)).toBe(false);
  });

  it("widgets.ts reaches mermaid ONLY through the ./mermaid wrapper (never the bare module)", async () => {
    const src = (await import("./widgets.ts?raw")).default;
    // The widget must import renderMermaid from the local wrapper, not pull "mermaid"
    // statically (which would re-introduce the heavy dep into the widgets chunk).
    expect(/from\s+["']\.\/mermaid["']/.test(src)).toBe(true);
    expect(/from\s+["']mermaid["']/.test(src)).toBe(false);
  });
});
