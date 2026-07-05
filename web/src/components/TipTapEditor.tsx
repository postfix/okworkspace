// TipTapEditor — the LIVE @tiptap/react editing SURFACE (RE-03, D-04/D-06).
//
// A second implementation of the Phase-40 EditorSurfaceHandle seam (the CM6
// LivePreviewEditor is the first), so PageEditor can mount it interchangeably: same
// value:string in (content + contentType:"markdown") / onChange:string out
// (editor.getMarkdown() in onUpdate) / imperative insertAtCursor over its OWN editor.
//
// IMPORT-ISOLATION (D-06, Phase-42 prep): this module is the SINGLE boundary that
// pulls @tiptap/react + @tiptap/suggestion + the Markdown extension. Nothing on the
// app entry / CM6 path imports it statically — PageEditor mounts it via React.lazy —
// so @tiptap stays OUT of the entry chunk (assertLazyChunks stays green even now).
//
// SECURITY (MAJ-05, paste — the milestone's sharpest untrusted surface): pasted HTML
// is defended in BOTH layers.
//   • Layer 1 (structural): ProseMirror's clipboard parser is schema-bound, so an
//     unknown element (<script>/<iframe>/…) has no parseDOM rule and is DROPPED;
//     parsed content is re-rendered inert via the schema's renderHTML.
//   • Layer 2 (attributes): the schema does NOT scrub dangerous ATTRIBUTES on
//     elements that DO map, so editorProps.transformPastedHTML = sanitizePastedHtml
//     strips <script>/<iframe>/<object>/<embed>, on*= event-handler attrs, and
//     javascript: URLs from the raw HTML BEFORE parsing. Neither layer ever executes
//     the pasted markup. A paste-XSS fixture (TipTapEditor.test.tsx) proves it.
import {
  forwardRef,
  useEffect,
  useImperativeHandle,
  useMemo,
  useRef,
} from "react";
import { useEditor, EditorContent } from "@tiptap/react";
import { Markdown } from "@tiptap/markdown";
import type { AnyExtension } from "@tiptap/core";

import { buildExtensions } from "../lib/tiptap/extensions";
import { Tag } from "../lib/tiptap/tagNode";
import { TagSuggest, SlashCommands } from "../lib/tiptap/suggestion";
import { sanitizePastedHtml } from "../lib/tiptap/sanitizePaste";
import { getTagVocabulary } from "../api/client";
import type { EditorSurfaceHandle } from "./LivePreviewEditor";
import "./TipTapEditor.css";

// TipTapEditorProps mirrors LivePreviewEditorProps EXACTLY so the surface is a drop-in
// replacement PageEditor mounts through the same seam. `currentPath`/`mode` are part of
// the shared contract (used by the CM6 surface); the TipTap surface accepts them for
// interchangeability. `readOnly` gates editability (soft lock, COLL-02).
interface TipTapEditorProps {
  value: string;
  onChange: (value: string) => void;
  currentPath: string;
  mode: "live" | "source";
  readOnly?: boolean;
}

const TipTapEditor = forwardRef<EditorSurfaceHandle, TipTapEditorProps>(
  function TipTapEditor({ value, onChange, readOnly = false }, ref) {
    // onChangeRef keeps onUpdate stable: the editor is created once (deps []), but the
    // parent's onChange identity may change per render — read it through a ref so a
    // stale closure never ships bytes to a dead handler.
    const onChangeRef = useRef(onChange);
    useEffect(() => {
      onChangeRef.current = onChange;
    }, [onChange]);

    // vocabRef holds the fetched tag vocabulary; the `#`-suggest reads it via the
    // getVocabulary getter (stable closure) so a late fetch populates suggestions
    // without recreating the editor. A failed fetch leaves it empty (graceful).
    const vocabRef = useRef<string[]>([]);
    useEffect(() => {
      let alive = true;
      void getTagVocabulary()
        .then((vocab) => {
          if (alive) vocabRef.current = vocab;
        })
        .catch(() => {
          // Vocabulary is a convenience; on failure the `#`-suggest offers nothing.
        });
      return () => {
        alive = false;
      };
    }, []);

    // The extension array is built ONCE (stable identity) so useEditor never recreates
    // the editor on re-render. Tag + the suggestion extensions are registered ONLY here
    // (never in buildExtensions) so the headless codec stays tag-free. Markdown is added
    // for editor.getMarkdown()/contentType:"markdown" (the codec uses MarkdownManager
    // directly, so buildExtensions omits it).
    const extensions = useMemo<AnyExtension[]>(
      () => [
        ...buildExtensions(),
        Markdown,
        Tag,
        TagSuggest.configure({ getVocabulary: () => vocabRef.current }),
        SlashCommands,
      ],
      [],
    );

    const editor = useEditor(
      {
        extensions,
        content: value,
        contentType: "markdown",
        editable: !readOnly,
        editorProps: {
          // Layer 2 (MAJ-05): scrub dangerous nodes/attrs from the raw clipboard HTML
          // before ProseMirror's schema-bound parse. Layer 1 (the schema) drops unknown
          // NODES; this closes the ATTRIBUTE gap.
          transformPastedHTML: sanitizePastedHtml,
        },
        onUpdate: ({ editor: ed }) => onChangeRef.current(ed.getMarkdown()),
      },
      [],
    );

    // Reseed the document when the `value` prop changes EXTERNALLY (a programmatic reset
    // / conflict resolution), guarded so an echo of the surface's own onChange never
    // loops: only reseed when the incoming value differs from what the editor holds.
    // emitUpdate:false so the reseed is not mistaken for a user edit.
    useEffect(() => {
      if (!editor) return;
      if (value !== editor.getMarkdown()) {
        editor.commands.setContent(value, {
          contentType: "markdown",
          emitUpdate: false,
        });
      }
    }, [editor, value]);

    // A soft lock discovered AFTER mount (COLL-02) flips readOnly true; reconfigure
    // editability without recreating the editor so the buffer/selection are preserved.
    useEffect(() => {
      editor?.setEditable(!readOnly);
    }, [editor, readOnly]);

    useImperativeHandle(
      ref,
      () => ({
        insertAtCursor(text: string) {
          editor
            ?.chain()
            .focus()
            .insertContent(text, { contentType: "markdown" })
            .run();
        },
      }),
      [editor],
    );

    return <EditorContent editor={editor} className="tiptap-editor" />;
  },
);

export default TipTapEditor;
