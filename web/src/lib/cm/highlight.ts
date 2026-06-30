// highlight — the ONE shared, token-driven syntax-highlight engine for fenced code
// on BOTH CM6 surfaces (ED-04a). Two exports drive everything:
//
//   • codeHighlightStyle — a single HighlightStyle.define([...]) mapping @lezer/highlight
//     `tags` → explicit `class:` names (`tok-keyword`, `tok-string`, `tok-comment`,
//     `tok-number`, `tok-function`). The live editor consumes it via
//     syntaxHighlighting(codeHighlightStyle) (mode.ts); the read CodeBlockWidget passes
//     the SAME object to highlightCode (widgets.ts). One source of truth → live and read
//     render identically (D-02). Using explicit `class:` names means the emitted class is
//     that fixed name with NO StyleModule.mount — the colors live in LivePreviewEditor.css
//     as `.tok-*{color:var(--syntax-*)}` (tokens-only contract, no inline hex; D-01).
//
//     SCOPE: only CODE-token tags are mapped (keyword/string/comment/number/function-type).
//     Heading/emphasis/strong are deliberately NOT mapped — the live editor's existing
//     .cm-heading/.cm-strong/.cm-em decorations (theme.ts) own those, and the read view's
//     Markdown is rendered by its own decorations, never by this style.
//
//   • codeLanguages — the curated LanguageDescription[] (D-06) handed to both
//     markdown({ codeLanguages }) (editor) and LanguageDescription.matchLanguageName
//     (read widget). EVERY grammar loads via a dynamic `import()` inside `load` so Vite
//     code-splits each into its own lazy chunk — there is NO static top-level import of
//     any grammar in this module (verified by the assertLazyChunks build assertion, D-05).
import {
  HighlightStyle,
  LanguageDescription,
  LanguageSupport,
  StreamLanguage,
} from "@codemirror/language";
import { tags as t } from "@lezer/highlight";

// codeHighlightStyle — ~5 desaturated roles (D-01). Each spec uses an explicit `class:`
// so the emitted class is the fixed `tok-*` name; the actual color is `var(--syntax-*)`
// declared in LivePreviewEditor.css (no color value lives here — tokens-only contract).
export const codeHighlightStyle = HighlightStyle.define([
  {
    tag: [
      t.keyword,
      t.modifier,
      t.operatorKeyword,
      t.controlKeyword,
      t.definitionKeyword,
      t.moduleKeyword,
      t.self,
      t.operator,
    ],
    class: "tok-keyword",
  },
  {
    tag: [
      t.string,
      t.docString,
      t.character,
      t.regexp,
      t.escape,
      t.special(t.string),
    ],
    class: "tok-string",
  },
  {
    tag: [t.comment, t.lineComment, t.blockComment, t.docComment],
    class: "tok-comment",
  },
  {
    tag: [t.number, t.integer, t.float, t.bool, t.atom, t.literal, t.unit],
    class: "tok-number",
  },
  {
    tag: [
      t.function(t.variableName),
      t.function(t.propertyName),
      t.definition(t.function(t.variableName)),
      t.typeName,
      t.className,
      t.macroName,
    ],
    class: "tok-function",
  },
]);

// codeLanguages — the curated v1.4 set (D-06). Each entry's `load` is the ONLY place a
// grammar is imported, and always via dynamic import() so it lands in a lazy chunk.
// matchLanguageName matches a fence info string against `name` + `alias` (case-insensitive).
//
// Unlisted / unknown languages simply don't match → the read widget stays plain (no error);
// `mermaid` is deliberately ABSENT (left plain — routed to Phase 26 / ED-05).
export const codeLanguages: LanguageDescription[] = [
  LanguageDescription.of({
    name: "javascript",
    alias: ["js", "jsx", "ts", "tsx", "typescript"],
    load: () =>
      import("@codemirror/lang-javascript").then((m) =>
        m.javascript({ jsx: true, typescript: true }),
      ),
  }),
  LanguageDescription.of({
    name: "python",
    alias: ["py"],
    load: () => import("@codemirror/lang-python").then((m) => m.python()),
  }),
  LanguageDescription.of({
    name: "go",
    alias: ["golang"],
    load: () => import("@codemirror/lang-go").then((m) => m.go()),
  }),
  LanguageDescription.of({
    name: "rust",
    alias: ["rs"],
    load: () => import("@codemirror/lang-rust").then((m) => m.rust()),
  }),
  LanguageDescription.of({
    name: "json",
    load: () => import("@codemirror/lang-json").then((m) => m.json()),
  }),
  LanguageDescription.of({
    name: "yaml",
    alias: ["yml"],
    load: () => import("@codemirror/lang-yaml").then((m) => m.yaml()),
  }),
  LanguageDescription.of({
    name: "sql",
    load: () => import("@codemirror/lang-sql").then((m) => m.sql()),
  }),
  LanguageDescription.of({
    name: "html",
    alias: ["xml"],
    load: () => import("@codemirror/lang-html").then((m) => m.html()),
  }),
  LanguageDescription.of({
    name: "css",
    load: () => import("@codemirror/lang-css").then((m) => m.css()),
  }),
  // bash/shell via @codemirror/legacy-modes — there is NO "bash" stream mode; `shell`
  // IS bash/sh. Wrap the StreamParser in a StreamLanguage + LanguageSupport.
  LanguageDescription.of({
    name: "shell",
    alias: ["bash", "sh", "zsh", "shell-session", "console"],
    load: () =>
      import("@codemirror/legacy-modes/mode/shell").then(
        (m) => new LanguageSupport(StreamLanguage.define(m.shell)),
      ),
  }),
  LanguageDescription.of({
    name: "markdown",
    alias: ["md"],
    load: () => import("@codemirror/lang-markdown").then((m) => m.markdown()),
  }),
  // diff/patch via @codemirror/legacy-modes.
  LanguageDescription.of({
    name: "diff",
    alias: ["patch"],
    load: () =>
      import("@codemirror/legacy-modes/mode/diff").then(
        (m) => new LanguageSupport(StreamLanguage.define(m.diff)),
      ),
  }),
];
