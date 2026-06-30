// ED-05a — the lazy mermaid render wrapper. mermaid's real renderer CANNOT run under
// jsdom (no getBBox), so we MOCK `import("mermaid")` and assert only OUR contract:
//   • initialize is called ONCE (guarded), with the strict/dark/suppressErrorRendering
//     config (the security-critical flags — D-04);
//   • parse runs before render (the pre-validation gate);
//   • render's resolved svg string is returned verbatim;
//   • a parse OR render rejection PROPAGATES (the caller turns it into the fallback);
//   • each render gets a fresh unique id.
// A structural assertion (?raw) pins that this module has NO static mermaid import —
// the whole point of the lazy chunk (assertLazyChunks proves the build side).
import { describe, it, expect, vi, beforeEach } from "vitest";

// The mock default export. Hoisted via vi.mock's factory; the spies are captured on a
// shared object so each test can read call args / control resolution.
const mock = vi.hoisted(() => {
  return {
    initialize: vi.fn(),
    parse: vi.fn(async () => true),
    render: vi.fn(async (id: string) => ({ svg: `<svg data-id="${id}"></svg>` })),
  };
});

vi.mock("mermaid", () => ({ default: mock }));

// renderMermaid holds module-level `initialized`/`idCounter` state. resetModules gives
// each test a fresh module instance so the once-guard + counter start clean.
beforeEach(() => {
  vi.clearAllMocks();
  mock.parse.mockImplementation(async () => true);
  mock.render.mockImplementation(async (id: string) => ({
    svg: `<svg data-id="${id}"></svg>`,
  }));
  vi.resetModules();
});

async function load() {
  return (await import("./mermaid")).renderMermaid;
}

describe("renderMermaid (ED-05a lazy strict wrapper)", () => {
  it("initializes ONCE with strict + dark + suppressErrorRendering, then returns the svg", async () => {
    const renderMermaid = await load();
    const svg = await renderMermaid("graph TD; A-->B;");
    expect(svg).toBe('<svg data-id="mmd-0"></svg>');

    expect(mock.initialize).toHaveBeenCalledTimes(1);
    const cfg = mock.initialize.mock.calls[0][0];
    // The security-critical flags (D-04) — assert each explicitly.
    expect(cfg.securityLevel).toBe("strict");
    expect(cfg.theme).toBe("dark");
    expect(cfg.suppressErrorRendering).toBe(true);
    expect(cfg.startOnLoad).toBe(false);

    // parse ran BEFORE render (pre-validation gate).
    expect(mock.parse).toHaveBeenCalledWith("graph TD; A-->B;");
    expect(mock.render).toHaveBeenCalledTimes(1);
  });

  it("guards initialize across multiple renders (called exactly once)", async () => {
    const renderMermaid = await load();
    await renderMermaid("a");
    await renderMermaid("b");
    await renderMermaid("c");
    expect(mock.initialize).toHaveBeenCalledTimes(1);
    expect(mock.render).toHaveBeenCalledTimes(3);
  });

  it("uses a FRESH unique id per render (mmd-0, mmd-1, …)", async () => {
    const renderMermaid = await load();
    await renderMermaid("a");
    await renderMermaid("b");
    const id0 = mock.render.mock.calls[0][0];
    const id1 = mock.render.mock.calls[1][0];
    expect(id0).toBe("mmd-0");
    expect(id1).toBe("mmd-1");
    expect(id0).not.toBe(id1);
  });

  it("PROPAGATES a parse rejection (invalid diagram → caller falls back) and never renders", async () => {
    mock.parse.mockRejectedValueOnce(new Error("Parse error on line 1"));
    const renderMermaid = await load();
    await expect(renderMermaid("not a diagram")).rejects.toThrow(/Parse error/);
    // parse gates render: a failed parse must short-circuit before render runs.
    expect(mock.render).not.toHaveBeenCalled();
  });

  it("PROPAGATES a render rejection (suppressErrorRendering rethrow → caller falls back)", async () => {
    mock.render.mockRejectedValueOnce(new Error("render blew up"));
    const renderMermaid = await load();
    await expect(renderMermaid("graph TD; A-->B;")).rejects.toThrow(/render blew up/);
  });

  it("has NO static top-level mermaid import — only the dynamic import() (D-03)", async () => {
    const src = (await import("./mermaid.ts?raw")).default;
    // A static `import … from "mermaid"` would pull mermaid (+ d3/dagre/…) into the
    // entry chunk — exactly what assertLazyChunks forbids. Only `import("mermaid")` is allowed.
    const staticMermaidImport = /^import\s+[^;]*\bfrom\s+["']mermaid["']/m;
    expect(staticMermaidImport.test(src)).toBe(false);
    expect(/import\(["']mermaid["']\)/.test(src)).toBe(true);
  });
});
