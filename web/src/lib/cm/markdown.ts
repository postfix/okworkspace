// markdown — the CM6 Markdown language extension configured for GFM-only parsing,
// over the commonmark base. This is deliberately NOT the `markdownLanguage`
// superset (which adds emoji/subscript/superscript that the server's Goldmark and
// the read view's remark-gfm do NOT support): if the editor parsed `:smile:` or
// `^sup^` as a construct, the live-preview render would diverge from what the
// server stores and what read mode shows (RESEARCH Pitfall 4). Restricting to the
// four GFM extensions — Table, Strikethrough, TaskList, Autolink — keeps the
// editor, server, and read view in agreement.
import { markdown } from "@codemirror/lang-markdown";
import { commonmarkLanguage } from "@codemirror/lang-markdown";
import { Table, Strikethrough, TaskList, Autolink } from "@lezer/markdown";

// markdownExtension is the configured CM6 language extension. liveExtensions /
// sourceExtensions (mode.ts) and the LivePreviewEditor both compose this so the
// Lezer syntax tree the decoration plugin (06-02) walks is GFM-only.
export const markdownExtension = markdown({
  base: commonmarkLanguage,
  extensions: [Table, Strikethrough, TaskList, Autolink],
});
