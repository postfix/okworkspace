// theme — the CM6 EditorView theme for the live-preview surface. Every value is a
// tokens.css `var(--…)` reference (NO hard-coded hex/px) so the editor inherits the
// Phase 0 design contract and a future dark theme needs no edits here. Visual
// parity target is MarkdownProse.css (UI-SPEC Typography/Color): the content column
// is capped at --prose-max-width, the surface has a minimum height, body text uses
// the UI font at body size, code/source uses the mono font, and the focus ring uses
// the accent color.
import { EditorView } from "@codemirror/view";

export const theme = EditorView.theme({
  "&": {
    color: "var(--color-text)",
    backgroundColor: "var(--color-bg)",
    fontSize: "var(--font-size-body)",
    minHeight: "var(--editor-min-height)",
  },
  ".cm-scroller": {
    fontFamily: "var(--font-family-ui)",
    lineHeight: "var(--line-height-body)",
  },
  ".cm-content": {
    maxWidth: "var(--prose-max-width)",
    caretColor: "var(--color-text)",
    minHeight: "var(--editor-min-height)",
  },
  "&.cm-focused": {
    outline: "2px solid var(--color-accent)",
    outlineOffset: "2px",
  },
  // Inline code + fenced code use the mono font and code background, matching
  // MarkdownProse's `code`/`pre` styling.
  ".cm-md-code, .cm-md-codeblock": {
    fontFamily: "var(--font-family-mono)",
    backgroundColor: "var(--color-code-bg)",
    borderRadius: "var(--radius-sm)",
  },
});

// sourceTheme renders the WHOLE document monospace (Source mode shows raw Markdown
// document-wide, per UI-SPEC "Source mode renders mono document-wide"). It is added
// to sourceExtensions only.
export const sourceTheme = EditorView.theme({
  ".cm-content": {
    fontFamily: "var(--font-family-mono)",
  },
});
