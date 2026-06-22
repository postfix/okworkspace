// EDIT-03/04 — the LivePreviewEditor exposes the SAME value/onChange(string)
// contract as the old <MDEditor>, and typing ships VERBATIM bytes to onChange (no
// block-model rewrite). Real CM6 EditorView under jsdom: render, type into the
// contenteditable, assert onChange fires with exactly the bytes the document now
// holds, and assert an external `value` change reseeds the document without an
// onChange feedback loop.
//
// RED until Task 4 ships web/src/components/LivePreviewEditor.tsx, then GREEN.
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, act } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";

import LivePreviewEditor from "./LivePreviewEditor";

// LivePreviewEditor uses react-router's useNavigate (for internal `.md` link
// click-navigation, D-06), so every render must sit inside a Router. `withRouter`
// wraps the element; the test asserts the SAME value/onChange contract regardless.
const withRouter = (ui: React.ReactElement) => (
  <MemoryRouter>{ui}</MemoryRouter>
);

// Helper: drive a programmatic insertion through the real CM6 view by dispatching
// against the EditorView the component mounts. We locate it via the .cm-content DOM
// and use document.execCommand-free insertion by simulating user input is brittle
// in jsdom; instead we assert the contract through the component's onChange when the
// external value changes and through a direct doc replace. To exercise "typing"
// deterministically we replace the document via the value prop round-trip below.

describe("LivePreviewEditor value/onChange contract (EDIT-03/04)", () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it("mounts a single CM6 editor surface", () => {
    const onChange = vi.fn();
    render(
      withRouter(
        <LivePreviewEditor
          value="hello"
          onChange={onChange}
          currentPath="notes.md"
          mode="live"
        />,
      ),
    );
    // The CM6 view renders a .cm-editor / .cm-content into the host.
    expect(document.querySelector(".cm-editor")).not.toBeNull();
    expect(document.querySelector(".cm-content")).not.toBeNull();
  });

  it("renders the initial value as the document text", () => {
    render(
      withRouter(
        <LivePreviewEditor
          value="# Title\nbody"
          onChange={vi.fn()}
          currentPath="notes.md"
          mode="source"
        />,
      ),
    );
    const content = document.querySelector(".cm-content");
    expect(content).not.toBeNull();
    // CM6 splits the doc into .cm-line nodes; their joined text is the document.
    expect(content!.textContent).toContain("# Title");
  });

  it("fires onChange with verbatim bytes when the document changes", () => {
    const onChange = vi.fn();
    const { container } = render(
      withRouter(
        <LivePreviewEditor
          value=""
          onChange={onChange}
          currentPath="notes.md"
          mode="live"
        />,
      ),
    );
    // Reach into the mounted EditorView and dispatch a real insert transaction,
    // exactly what a keystroke produces. The updateListener must then call onChange
    // with the new doc string verbatim.
    const cmEditor = container.querySelector(".cm-editor") as HTMLElement & {
      cmView?: { view: import("@codemirror/view").EditorView };
    };
    const view = cmEditor?.cmView?.view;
    expect(view).toBeDefined();
    act(() => {
      view!.dispatch({ changes: { from: 0, insert: "**bold** and `code`" } });
    });
    expect(onChange).toHaveBeenCalledWith("**bold** and `code`");
  });

  it("reseeds on external value change without an onChange feedback loop", () => {
    const onChange = vi.fn();
    const { rerender } = render(
      withRouter(
        <LivePreviewEditor
          value="first"
          onChange={onChange}
          currentPath="notes.md"
          mode="live"
        />,
      ),
    );
    onChange.mockClear();
    rerender(
      withRouter(
        <LivePreviewEditor
          value="second"
          onChange={onChange}
          currentPath="notes.md"
          mode="live"
        />,
      ),
    );
    const content = document.querySelector(".cm-content");
    expect(content!.textContent).toContain("second");
    // The external seed must NOT echo back through onChange (no feedback loop).
    expect(onChange).not.toHaveBeenCalled();
  });

  it("toggling mode does not change the document bytes", () => {
    const onChange = vi.fn();
    const { rerender, container } = render(
      withRouter(
        <LivePreviewEditor
          value="# H\n**b**"
          onChange={onChange}
          currentPath="notes.md"
          mode="live"
        />,
      ),
    );
    const cmEditor = container.querySelector(".cm-editor") as HTMLElement & {
      cmView?: { view: import("@codemirror/view").EditorView };
    };
    const before = cmEditor.cmView!.view.state.doc.toString();
    rerender(
      withRouter(
        <LivePreviewEditor
          value="# H\n**b**"
          onChange={onChange}
          currentPath="notes.md"
          mode="source"
        />,
      ),
    );
    expect(cmEditor.cmView!.view.state.doc.toString()).toBe(before);
  });

  // COLL-02 regression: a soft lock is almost always discovered AFTER the editor
  // mounts (enter Edit → an acquire heartbeat returns held-by-other). The editor must
  // become genuinely non-editable when `readOnly` flips true post-mount — not stay
  // editable behind a "View only" banner (the original bug: readOnly was baked into
  // the initial state and never reconfigured, so keystrokes still landed and autosaved).
  it("flips to genuinely non-editable when readOnly turns true after mount (COLL-02)", () => {
    const onChange = vi.fn();
    const { rerender, container } = render(
      withRouter(
        <LivePreviewEditor
          value="hello"
          onChange={onChange}
          currentPath="notes.md"
          mode="live"
          readOnly={false}
        />,
      ),
    );
    const cmEditor = container.querySelector(".cm-editor") as HTMLElement & {
      cmView?: { view: import("@codemirror/view").EditorView };
    };
    const view = cmEditor.cmView!.view;
    // Entered Edit editable (the lock is not known yet).
    expect(view.state.readOnly).toBe(false);
    expect(view.contentDOM.getAttribute("contenteditable")).toBe("true");

    // A held-by-other lock arrives → readOnly flips true.
    rerender(
      withRouter(
        <LivePreviewEditor
          value="hello"
          onChange={onChange}
          currentPath="notes.md"
          mode="live"
          readOnly={true}
        />,
      ),
    );
    // SAME view instance — reconfigured, not rebuilt, so the buffer is preserved …
    expect(cmEditor.cmView!.view).toBe(view);
    // … and it is now genuinely non-editable: the browser blocks the caret/keystrokes
    // (contentEditable=false) and CM commands refuse edits (state.readOnly=true).
    expect(view.state.readOnly).toBe(true);
    expect(view.contentDOM.getAttribute("contenteditable")).toBe("false");

    // Force edit clears the lock → editing is restored on the same view.
    rerender(
      withRouter(
        <LivePreviewEditor
          value="hello"
          onChange={onChange}
          currentPath="notes.md"
          mode="live"
          readOnly={false}
        />,
      ),
    );
    expect(cmEditor.cmView!.view).toBe(view);
    expect(view.state.readOnly).toBe(false);
    expect(view.contentDOM.getAttribute("contenteditable")).toBe("true");
  });
});

// Keep this referenced so the unused-import guard does not complain in the stub
// phase even if a test above is adjusted.
void screen;
