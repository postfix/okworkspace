// extensions — the SHARED bounded-slice TipTap extension array (RE-01 crit-1).
//
// buildExtensions() is the SINGLE source of truth for which Markdown constructs the
// TipTap codec understands. The headless codec (codec.ts) consumes it now via
// MarkdownManager; the live editor host (Phase 40+) will consume the SAME array, so
// parse/serialize behaviour is identical across both surfaces and the round-trip
// corpus that gates this milestone is testing exactly what the editor will do.
//
// THE BOUNDED SLICE (v1.7 Tier-1 — the constructs the codec must round-trip):
//   paragraphs, headings 1-6, bold/italic/strike/inline-code, fenced code block,
//   bullet/ordered/task lists, blockquote, link, hard break.
//
// StarterKit 3.27.1 already bundles Document, Paragraph, Text, Heading, Bold, Italic,
// Strike, Code, CodeBlock, Blockquote, HardBreak, Link, and the list nodes
// (BulletList/OrderedList/ListItem/ListKeymap via @tiptap/extension-list). We disable
// the six extensions that are OUT of the bounded slice (verified against the installed
// StarterKitOptions type — every key below is a real `false`-able option):
//   - horizontalRule  — thematic break, not in the slice
//   - underline       — no Markdown syntax; would diverge from Goldmark/read view
//   - dropcursor      — editor-only UI behaviour (irrelevant headless)
//   - gapcursor       — editor-only UI behaviour
//   - trailingNode    — editor-only affordance; would inject empty trailing paragraphs
//   - undoRedo        — history plugin; editor-only, no codec relevance
//
// TaskList + TaskItem are NOT part of StarterKit — they are imported from
// @tiptap/extension-list and appended so `- [ ]` / `- [x]` round-trips.
//
// IMPORT-ISOLATION (RE-04, wired in Phase 42): nothing reachable from the app entry
// statically imports web/src/lib/tiptap/* this phase — it is used ONLY by the codec
// tests — so `marked` (pulled transitively by @tiptap/markdown) stays out of the
// entry chunk by construction until the guard lands.
import { StarterKit } from "@tiptap/starter-kit";
import { TaskList, TaskItem } from "@tiptap/extension-list";
import type { AnyExtension } from "@tiptap/core";

// buildExtensions returns a fresh bounded-slice extension array. It is called per
// MarkdownManager instance (codec.ts) so no mutable extension state is shared between
// parse/serialize passes.
export function buildExtensions(): AnyExtension[] {
  return [
    StarterKit.configure({
      horizontalRule: false,
      underline: false,
      dropcursor: false,
      gapcursor: false,
      trailingNode: false,
      undoRedo: false,
    }),
    TaskList,
    TaskItem,
  ];
}
