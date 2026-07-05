// sanitizePaste — the Layer-2 clipboard-HTML attribute scrubber (RE-03, MAJ-05).
//
// Wired as the live surface's editorProps.transformPastedHTML (TipTapEditor.tsx), it runs
// on the raw clipboard HTML string BEFORE ProseMirror's schema-bound parse. The schema
// (Layer 1) drops unknown NODES structurally (a <script>/<iframe> has no parseDOM rule),
// but does NOT scrub dangerous ATTRIBUTES on elements that DO map — that gap is this
// module's job. It removes:
//   • <script>/<iframe>/<object>/<embed> elements (open+content+close AND any stray
//     open/self-closing/close form) — a belt for the schema's structural drop;
//   • event-handler attributes (on\w+=…, quoted or unquoted) on ANY element — the real
//     gap (a surviving onerror on a mapped element would be a stored-XSS vector);
//   • javascript: URL schemes anywhere — neutralized so a mapped <a>/<img> can't carry
//     an executable URL.
// A pure string→string function so the paste-XSS test can exercise it directly AND
// through the editor (TipTapEditor.test.tsx). Kept in its own module so nothing but the
// live surface + its test imports it (import-isolation, D-06).
export function sanitizePastedHtml(html: string): string {
  return html
    .replace(/<(script|iframe|object|embed)\b[^>]*>[\s\S]*?<\/\1\s*>/gi, "")
    .replace(/<\/?(script|iframe|object|embed)\b[^>]*>/gi, "")
    .replace(/\son\w+\s*=\s*("[^"]*"|'[^']*'|[^\s>]+)/gi, "")
    .replace(/javascript:/gi, "removed:");
}
