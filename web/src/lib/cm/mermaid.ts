// mermaid — the lazy, strict-sandboxed render wrapper for ```mermaid fenced blocks
// (ED-05a). mermaid is LARGE (d3/dagre/cytoscape/dompurify/…), so it is loaded ONLY
// via a dynamic `import("mermaid")` inside renderMermaid — there is NO static
// top-level mermaid import in this module. That single dynamic import is what keeps
// mermaid + its heavy transitive deps out of every entry chunk; the assertLazyChunks
// build guard (vite.config.ts) PROVES it (D-03 / criterion 2).
//
// SECURITY (D-04): rendering runs under `securityLevel:'strict'` ⇒ mermaid pipes its
// output SVG through DOMPurify before returning it, AND disables click/interactivity.
// The diagram source is untrusted page content, so the strict sanitize + the caller's
// DOMParser injection (widgets.ts) together mean untrusted bytes can never become live
// script. `suppressErrorRendering:true` is REQUIRED: without it, `render` does NOT
// throw on an invalid diagram — it returns a "Syntax error" SVG and LEAVES a temp
// `<div id="d…">` on document.body. With it, render removes the temp node and rethrows,
// giving the caller a clean throw → the raw-code fallback (D-05), no leftover DOM.

// `initialize` overwrites the global mermaid config on every call (it is not usefully
// idempotent), so we guard it to run exactly once per page. App is dark-only today
// (D-07); `as const` keeps the literal types (e.g. 'strict') assignable to mermaid's
// config enums without a static `import type` (which the no-static-import test forbids).
// ponytail: theme locked to 'dark' at first render by the once-guard. A future light
// theme is a value swap here, plus re-init plumbing if runtime theme switching is added
// (the MermaidWidget already keys eq() on theme so a swap rebuilds the widget).
const MERMAID_CONFIG = {
  startOnLoad: false,
  securityLevel: "strict",
  theme: "dark",
  suppressErrorRendering: true,
} as const;

let initialized = false;
// A fresh, valid CSS id per render — mermaid requires `id` to be a unique element id
// (it mounts a temp node keyed on it). A monotonic counter is collision-free per page.
let idCounter = 0;

// renderMermaid lazy-loads mermaid, initializes once, pre-validates with `parse`
// (DOM-free, throws on invalid), then renders to an SVG STRING. It THROWS on any
// parse/render failure (suppressErrorRendering ensures render rethrows + cleans up its
// temp node) — the caller turns a throw into the raw-code fallback. The returned SVG is
// DOMPurify-sanitized (strict) but the caller STILL injects it via DOMParser (never
// innerHTML) as defense-in-depth.
export async function renderMermaid(code: string): Promise<string> {
  const mermaid = (await import("mermaid")).default;
  if (!initialized) {
    mermaid.initialize(MERMAID_CONFIG);
    initialized = true;
  }
  // Cheap pre-validation gate (throws on invalid, builds no DOM). render also throws
  // under suppressErrorRendering, but parse fails fast without touching the document.
  await mermaid.parse(code);
  const { svg } = await mermaid.render("mmd-" + idCounter++, code);
  return svg;
}
