import { useEffect, useRef } from "react";
import { useNavigate } from "react-router-dom";
import { Annotation, EditorState } from "@codemirror/state";
import { EditorView, keymap, placeholder } from "@codemirror/view";
import { defaultKeymap, history, historyKeymap } from "@codemirror/commands";

import {
  modeCompartment,
  liveExtensions,
  sourceExtensions,
  toggleKeymap,
} from "../lib/cm/mode";
import { linkNav } from "../lib/cm/linkNav";
import { headingAnchors, scrollToHash } from "../lib/cm/headingAnchors";
import "./LivePreviewEditor.css";

// LivePreviewEditor is the CodeMirror 6 editing surface that replaces
// <MDEditor>. It exposes the SAME contract as the old editor — `value: string`
// in, `onChange(value: string)` out — so PageEditor's save machinery (autosave,
// the single-flight runSaver, baseRevision, the 409 ConflictBanner, the
// frontmatter form) is untouched. The CM6 document IS the raw Markdown string: the
// updateListener ships `doc.toString()` verbatim on every doc change (no block
// model, no reparse-to-bytes — protects the byte-stable round-trip, EDIT-03).
//
// This plan keeps the surface minimal: "Live" mode (liveExtensions) is the
// markdown language + theme seam that 06-02/06-03 extend with inline-preview
// decorations and image/table widgets. "Source" mode is raw monospace text. The
// Live/Source choice is a `mode` prop driven by the persisted editorMode store;
// switching modes reconfigures a Compartment WITHOUT touching the document, so the
// toggle is byte-identical by construction (EDIT-02).
// When `readOnly` is true the surface is the UNIFIED read view (PageView): the
// document is non-editable (no caret, no edits) but selection/copy still work, the
// render is ALWAYS the Live decoration set (read mode never offers a Source toggle),
// each heading line carries its github-slugger id (headingAnchors), and on mount the
// surface scrolls to `#hash` if present (SRCH-06 deep-link). `onChange` is never
// fired in read mode (the doc cannot change). Edit mode (readOnly falsy) is unchanged.
interface LivePreviewEditorProps {
  value: string;
  onChange: (value: string) => void;
  currentPath: string;
  mode: "live" | "source";
  readOnly?: boolean;
}

// CmEditorEl is the .cm-editor host element with the EditorView exposed for tests
// (and potential imperative callers). CM6 does not expose the view as a stable
// public DOM property across versions, so the component stamps it explicitly.
type CmEditorEl = HTMLElement & { cmView?: { view: EditorView } };

// externalSeed annotates a transaction that re-seeds the document from the `value`
// prop (an initial load or a programmatic reset) so the updateListener can skip
// firing onChange for it — a seed is not a user edit and must not echo back through
// the controlled value (RESEARCH Pitfall 6).
const externalSeed = Annotation.define<boolean>();

export default function LivePreviewEditor({
  value,
  onChange,
  currentPath,
  mode,
  readOnly = false,
}: LivePreviewEditorProps) {
  const navigate = useNavigate();
  const host = useRef<HTMLDivElement>(null);
  const view = useRef<EditorView | null>(null);
  // onChange can change identity between renders; read the latest from a ref so
  // the updateListener (created once) never fires a stale callback. The ref is
  // synced in an effect (never mutated during render).
  const onChangeRef = useRef(onChange);
  // currentPath + navigate are captured for the linkNav handler; held in refs so
  // an internal-link click always resolves against the LIVE path and routes through
  // the current navigate fn even though the EditorView is created only once.
  const pathRef = useRef(currentPath);
  const navigateRef = useRef(navigate);
  useEffect(() => {
    onChangeRef.current = onChange;
    pathRef.current = currentPath;
    navigateRef.current = navigate;
  }, [onChange, currentPath, navigate]);

  // Create the EditorView once. ALWAYS destroy in cleanup so React 19 StrictMode's
  // mount→unmount→remount never leaves two views attached or a leaked instance.
  // The `value`/`mode` deps are intentionally omitted (synced by the effects
  // below) so a prop change never tears down and rebuilds the editor.
  useEffect(() => {
    // Shared extensions: internal `.md` link click-navigation (D-06), live in BOTH
    // read and edit modes. The handler reads the live currentPath/navigate from refs
    // so it stays correct across prop changes without recreating the view
    // (StrictMode-safe). resolveRelativeMdLink owns the scheme allowlist, so no
    // javascript:/data: href is ever navigated (T-06-07).
    const nav = linkNav(
      () => pathRef.current,
      (to) => navigateRef.current(to),
    );

    const extensions = readOnly
      ? [
          // Unified READ surface: non-editable (no caret/edits) but selection/copy
          // works. The render is ALWAYS Live (read mode never toggles to Source), and
          // each heading line gets its github-slugger id via headingAnchors so a
          // search `#hash` deep-link lands correctly (SRCH-06). T-06-10: read mode
          // reuses the SAME live-preview decoration pipeline as edit Live — there is
          // no second, weaker read renderer.
          EditorState.readOnly.of(true),
          EditorView.editable.of(false),
          nav,
          headingAnchors,
          // Force Live decorations through the same compartment seam edit mode uses,
          // so read and edit are pixel-identical. No toggleKeymap, no Source path.
          modeCompartment.of(liveExtensions),
        ]
      : [
          history(),
          keymap.of([...defaultKeymap, ...historyKeymap]),
          toggleKeymap,
          placeholder("Start writing in Markdown…"),
          nav,
          modeCompartment.of(mode === "live" ? liveExtensions : sourceExtensions),
          EditorView.updateListener.of((u) => {
            // Fire onChange ONLY on a document change, with the bytes verbatim —
            // never on selection/viewport updates (which would loop with the
            // controlled `value`). This is the EDIT-03 verbatim-bytes guarantee.
            // Skip seeds (externalSeed-annotated transactions) — only real user
            // edits report through onChange, with the bytes verbatim.
            if (
              u.docChanged &&
              !u.transactions.some((t) => t.annotation(externalSeed))
            ) {
              onChangeRef.current(u.state.doc.toString());
            }
          }),
        ];

    const v = new EditorView({
      parent: host.current!,
      state: EditorState.create({ doc: value, extensions }),
    });
    view.current = v;
    // Expose the view on the .cm-editor host for tests / imperative callers.
    (v.dom as CmEditorEl).cmView = { view: v };
    // Read mode: scroll to a #hash heading on mount (SRCH-06 deep-link). The hash is
    // only a lookup key against the rendered heading ids (T-06-12).
    if (readOnly) {
      scrollToHash(v);
    }
    return () => {
      v.destroy();
      view.current = null;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Sync an EXTERNAL value change (seed/reset) into the document — but only when it
  // actually differs from the current doc, so the onChange→state→value echo does
  // NOT loop back into a redundant dispatch (RESEARCH Pitfall 6). CM6 is the source
  // of truth while focused; this handles the initial seed and programmatic resets.
  useEffect(() => {
    const v = view.current;
    if (!v) return;
    const cur = v.state.doc.toString();
    if (value !== cur) {
      v.dispatch({
        changes: { from: 0, to: cur.length, insert: value },
        annotations: externalSeed.of(true),
      });
    }
  }, [value]);

  // Reconfigure the mode Compartment on a `mode` change WITHOUT touching the
  // document (effects-only dispatch) — byte-identical toggle (EDIT-02).
  useEffect(() => {
    // Read mode never toggles: the compartment stays on liveExtensions for the life
    // of the read view (pixel-identical to edit Live), so skip reconfiguration.
    if (readOnly) return;
    const v = view.current;
    if (!v) return;
    v.dispatch({
      effects: modeCompartment.reconfigure(
        mode === "live" ? liveExtensions : sourceExtensions,
      ),
    });
  }, [mode, readOnly]);

  return <div ref={host} className="livepreview-editor" />;
}
