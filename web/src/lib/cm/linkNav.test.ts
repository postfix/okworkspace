// EDIT-01 / D-06 / T-06-07 — internal `.md` link click-navigation on the CM6
// surface. Mount a real EditorView (markdown + livePreview so links render as
// `.cm-md-link[data-href]` marks) with the linkNav handler, dispatch a mousedown on
// the rendered link element, and assert:
//   • an internal `.md` href → navigate('/app/page/<target>') + preventDefault.
//   • an external/non-`.md` href → NO navigate (default behavior preserved).
//   • a `javascript:` href → NEVER navigated (resolveRelativeMdLink scheme gate).
import { describe, it, expect, afterEach, vi } from "vitest";
import { EditorState } from "@codemirror/state";
import { EditorView } from "@codemirror/view";

import { markdownExtension } from "./markdown";
import { livePreviewExtension } from "./livePreview";
import { linkNav } from "./linkNav";

const views: EditorView[] = [];
afterEach(() => {
  while (views.length) views.pop()!.destroy();
});

// mount builds an EditorView with the live-preview surface + a linkNav handler and
// returns the view plus the navigate spy. The selection is parked away from the
// link line so the link mark renders (not revealed-to-source).
function mount(doc: string, currentPath: string) {
  const navigate = vi.fn();
  const parent = document.createElement("div");
  const view = new EditorView({
    parent,
    state: EditorState.create({
      doc,
      selection: { anchor: 0 },
      extensions: [
        markdownExtension,
        livePreviewExtension,
        linkNav(currentPath, navigate),
      ],
    }),
  });
  views.push(view);
  return { view, navigate };
}

// fireLinkMousedown finds the rendered `.cm-md-link` element and dispatches a
// cancelable mousedown bubbling up to the editor (where the handler is attached).
// Returns whether preventDefault was called.
function fireLinkMousedown(view: EditorView): {
  found: boolean;
  defaultPrevented: boolean;
} {
  const linkEl = view.dom.querySelector(".cm-md-link") as HTMLElement | null;
  if (!linkEl) return { found: false, defaultPrevented: false };
  const ev = new MouseEvent("mousedown", { bubbles: true, cancelable: true });
  linkEl.dispatchEvent(ev);
  return { found: true, defaultPrevented: ev.defaultPrevented };
}

describe("linkNav (internal .md → SPA navigate)", () => {
  it("internal .md link click → navigate('/app/page/<target>') + preventDefault", () => {
    const { view, navigate } = mount(
      "para line\n\n[guide](sub/guide.md) word\n",
      "docs/index.md",
    );
    const r = fireLinkMousedown(view);
    expect(r.found).toBe(true);
    // Resolved against docs/ (the current page's dir): docs/sub/guide.md.
    expect(navigate).toHaveBeenCalledWith("/app/page/docs/sub/guide.md");
    expect(r.defaultPrevented).toBe(true);
  });

  it("external http(s) link click → NO SPA navigation (default behavior)", () => {
    const { view, navigate } = mount(
      "para line\n\n[ext](http://x.test/) word\n",
      "docs/index.md",
    );
    const r = fireLinkMousedown(view);
    expect(r.found).toBe(true);
    // The load-bearing assertion: an external link triggers NO SPA navigation. (We
    // don't assert on defaultPrevented here — CM6 itself may preventDefault a
    // mousedown for its own selection handling; our handler returns false and does
    // not navigate, which is the contract.)
    expect(navigate).not.toHaveBeenCalled();
  });

  it("non-.md internal link → NO SPA navigation", () => {
    const { view, navigate } = mount(
      "para line\n\n[img](pic.png) word\n",
      "docs/index.md",
    );
    fireLinkMousedown(view);
    expect(navigate).not.toHaveBeenCalled();
  });

  it("javascript: href is NEVER navigated (scheme gate)", () => {
    const { view, navigate } = mount(
      "para line\n\n[x](javascript:alert(1)) word\n",
      "docs/index.md",
    );
    fireLinkMousedown(view);
    expect(navigate).not.toHaveBeenCalled();
  });
});
