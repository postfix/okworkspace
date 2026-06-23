import { createElement, Fragment, type ReactNode } from "react";

// renderHighlight is the XSS chokepoint for search snippets (T-03-08).
//
// The 03-01 server formatter emits ONLY weight-only highlight markers around
// matched terms: `<strong>…</strong>` (and, defensively, `<span class="search-hl">…
// </span>`). This function maps those known markers to React <strong> elements
// and renders EVERYTHING else as escaped plain text. It NEVER uses
// dangerouslySetInnerHTML of raw server HTML, so an unexpected tag (e.g. an
// injected <img onerror> or <script>) renders as literal text, not live DOM —
// consistent with the Phase 1 stored-XSS guard (rehype-raw OFF).
//
// Kept small and total on purpose: any tag we do not explicitly allow is treated
// as plain text. There is no general HTML parser here — only the two known
// weight-only wrappers are recognized.

// Matches an opening <strong>/<span class="search-hl"> or its closing tag. The
// attribute on the span is fixed (class="search-hl"); any other attribute or tag
// falls through and is rendered as escaped text.
const MARKER = /(<strong>|<\/strong>|<span class="search-hl">|<\/span>)/g;

// The server formatter HTML-escapes the surrounding fragment text (the XSS guard
// — T-03-01), so a snippet arrives as e.g. `config &lt;value&gt;`. React renders
// text nodes verbatim, so without decoding the user would see the literal
// characters `&lt;value&gt;` instead of `<value>` (WR-03 double-escape). We decode
// the known named entities BACK to their characters before pushing the text node;
// React then re-escapes them safely on render, so the snippet reads correctly AND
// stays safe (no dangerouslySetInnerHTML, no raw-HTML parsing). Only the fixed set
// the server's html.EscapeString can emit is decoded — &amp; LAST so an escaped
// `&amp;lt;` does not collapse to `<`.
function decodeEntities(text: string): string {
  return text
    .replace(/&lt;/g, "<")
    .replace(/&gt;/g, ">")
    .replace(/&#34;/g, '"')
    .replace(/&quot;/g, '"')
    .replace(/&#39;/g, "'")
    .replace(/&#x27;/g, "'")
    .replace(/&amp;/g, "&");
}

export function renderHighlight(fragment: string): ReactNode {
  if (!fragment) return null;

  const parts = fragment.split(MARKER);
  const nodes: ReactNode[] = [];

  // Depth of currently-open highlight markers. While > 0, plain text segments are
  // wrapped in <strong>. We never emit nested <strong> elements — a single
  // weight bump is enough and matches the weight-only rule.
  let depth = 0;
  let key = 0;

  for (const part of parts) {
    if (part === "") continue;
    if (part === "<strong>" || part === '<span class="search-hl">') {
      depth++;
      continue;
    }
    if (part === "</strong>" || part === "</span>") {
      if (depth > 0) depth--;
      continue;
    }
    // Plain text segment. Decode the server's HTML entities back to characters so
    // the snippet reads correctly (WR-03); React then re-escapes the resulting
    // string on render, so any stray angle-bracket content is still inserted as
    // literal text, never parsed HTML.
    const text = decodeEntities(part);
    if (depth > 0) {
      nodes.push(createElement("strong", { key: key++ }, text));
    } else {
      nodes.push(text);
    }
  }

  return createElement(Fragment, null, ...nodes);
}
