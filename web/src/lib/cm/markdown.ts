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
import { tagParserExtensions } from "./hashtag";
import { codeLanguages } from "./highlight";

// markdownExtension is the configured CM6 language extension. liveExtensions /
// sourceExtensions (mode.ts) and the LivePreviewEditor both compose this so the
// Lezer syntax tree the decoration plugin (06-02) walks is GFM-only.
//
// tagParserExtensions (hashtag.ts) is the shared [Table, Strikethrough, TaskList,
// Autolink, hashtagExtension] list — the four GFM extensions plus the ED-02a `Hashtag`
// inline node. The CM6 render AND the Phase-22 `inlineTagsFromBody` materializer both
// configure their parser with THIS exact array (CONTEXT D-01b), so what badges on
// screen and what gets written to frontmatter can never fork. hashtagExtension is a
// purely additive client-side parser (no server-side counterpart, so no client/server
// drift; the server's Goldmark never sees a `#tag`, and rendering is decoration-only so
// the stored bytes are unchanged either way).
// codeLanguages (ED-04a) is PURELY ADDITIVE: it only tells the parser which nested
// fenced-code grammars to lazy-load for syntax highlighting. `base` + `extensions` are
// UNCHANGED, so which Markdown constructs are parsed (the GFM-only contract: Table,
// Strikethrough, TaskList, Autolink + the Hashtag inline node) is identical — the
// editor⇄server-Goldmark⇄read-view parity the comment above guards is preserved. Each
// codeLanguage's grammar is code-split (dynamic import in highlight.ts), so the editor
// re-parses + re-highlights a ```go block automatically once lang-go's chunk arrives.
export const markdownExtension = markdown({
  base: commonmarkLanguage,
  extensions: tagParserExtensions,
  codeLanguages,
});
