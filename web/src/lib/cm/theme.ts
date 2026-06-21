// theme — the CM6 EditorView theme for the live-preview surface. Every value is a
// tokens.css `var(--…)` reference (NO hard-coded hex/px) so the editor inherits the
// Phase 0 design contract and a future dark theme needs no edits here.
//
// Visual parity target is MarkdownProse.css (UI-SPEC Typography/Color): the content
// column is capped at --prose-max-width, body text uses the UI font at body size,
// the live-preview decoration classes emitted by livePreview.ts render at the EXACT
// UI-SPEC token values so Live mode looks byte-for-byte like read mode:
//
//   .cm-strong       → semibold, body size (WEIGHT ONLY — no size change, so the
//                      active-line marker reveal stays layout-neutral, Pitfall 2).
//   .cm-em           → italic, regular weight.
//   .cm-inline-code  → mono, code-bg, radius-sm, xs horizontal padding (mirrors
//                      `.markdown-prose code`).
//   .cm-code-block   → mono, code-bg block, radius-md, md padding (mirrors
//                      `.markdown-prose pre`).
//   .cm-heading-1    → display size/line-height, semibold (mirrors `h1`).
//   .cm-heading-2    → heading size/line-height, semibold (mirrors `h2`).
//   .cm-heading-3..6 → body size, semibold (mirrors `h3..h6`).
//   .cm-md-link      → accent text, underline-on-hover (mirrors `.prose-link`).
//
// CRITICAL (Pitfall 2 — layout-neutral reveal): none of these classes change
// line-height. Headings use the heading line-heights but a heading line is a
// heading line whether or not its "# " marker is revealed, so neighbors never
// reflow when the cursor enters/leaves. Bold is weight-only on body text.
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
  // NOTE: the accent focus ring lives in LivePreviewEditor.css (the project's
  // focus-ring convention is a literal accent outline in CSS — tokens.css
  // `:focus-visible`, controls.css, SearchPalette.css; there is no ring-width
  // token). Keeping it out of this JS theme object keeps every value here a pure
  // token reference, matching the tokens-only contract.

  // Inline emphasis — weight/style only, body size (layout-neutral reveal).
  ".cm-strong": {
    fontWeight: "var(--font-weight-semibold)",
  },
  ".cm-em": {
    fontStyle: "italic",
    fontWeight: "var(--font-weight-regular)",
  },

  // Inline code — mono span on the code background (mirrors `.markdown-prose code`).
  ".cm-inline-code": {
    fontFamily: "var(--font-family-mono)",
    backgroundColor: "var(--color-code-bg)",
    borderRadius: "var(--radius-sm)",
    padding: "0 var(--space-xs)",
  },
  // Fenced code block — mono block on the code background (mirrors `pre`,
  // which uses full md padding on all sides — not just horizontal).
  ".cm-code-block": {
    fontFamily: "var(--font-family-mono)",
    backgroundColor: "var(--color-code-bg)",
    borderRadius: "var(--radius-md)",
    padding: "var(--space-md)",
  },

  // Heading lines — sizes/weights/line-heights AND vertical rhythm mirror
  // `.markdown-prose h1..h6` so edit Live mode matches the read surface.
  ".cm-heading-1": {
    fontSize: "var(--font-size-display)",
    fontWeight: "var(--font-weight-semibold)",
    lineHeight: "var(--line-height-display)",
    margin: "var(--space-xl) 0 var(--space-md)",
  },
  ".cm-heading-2": {
    fontSize: "var(--font-size-heading)",
    fontWeight: "var(--font-weight-semibold)",
    lineHeight: "var(--line-height-heading)",
    margin: "var(--space-lg) 0 var(--space-sm)",
  },
  ".cm-heading-3, .cm-heading-4, .cm-heading-5, .cm-heading-6": {
    fontSize: "var(--font-size-body)",
    fontWeight: "var(--font-weight-semibold)",
    lineHeight: "var(--line-height-label)",
    margin: "var(--space-md) 0 var(--space-sm)",
  },

  // Rendered Markdown links — accent text, underline on hover (`.prose-link`).
  ".cm-md-link": {
    color: "var(--color-accent)",
    textDecoration: "none",
  },
  ".cm-md-link:hover": {
    textDecoration: "underline",
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
