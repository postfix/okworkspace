// widgets — the EDIT-01 inline WIDGETS (images + GFM tables) for Live mode, plus
// the load-bearing image-src security control (EDIT-04 / T-06-06..T-06-09).
//
// A `WidgetType` produces the DOM that `Decoration.replace({ widget })` mounts in
// place of the raw Markdown source span. Two widgets ship here:
//
//   • ImageWidget — replaces an `Image` (`![alt](src)`) node with a real <img>,
//     but ONLY after `sanitizeImageSrc(src)` (06-01) clears the src. A blocked
//     src (javascript:, vbscript:, executable data:, protocol-relative, …) falls
//     back to rendering the RAW `![alt](src)` Markdown text in a <span> — never a
//     broken-image element and never an executed/loaded URL. The widget therefore
//     never implies the page bytes changed.
//
//   • TableWidget — replaces a GFM `Table` block with a styled <table> grid built
//     from the parsed header + body cells, matching MarkdownProse table styling.
//
// SECURITY INVARIANT (T-06-08, RESEARCH Anti-Pattern): all widget DOM is built via
// document.createElement + textContent / explicit attributes ONLY. No `innerHTML`
// of page content is ever assigned — alt text, cell text, and the raw fallback are
// all set through `textContent`, so untrusted page bytes can never become live
// markup. The tests assert on element structure (tagName/children/textContent),
// never on HTML strings, to lock this in.
import { WidgetType } from "@codemirror/view";
import type { EditorView } from "@codemirror/view";
import { LanguageDescription } from "@codemirror/language";
import { highlightCode } from "@lezer/highlight";
import { sanitizeImageSrc } from "./sanitizeSrc";
import { codeHighlightStyle, codeLanguages } from "./highlight";
import { renderMermaid } from "./mermaid";

// ImageWidget renders an inline image. Constructed with the parsed src/alt and the
// RAW `![alt](src)` markdown so the blocked-src path can fall back to plain text.
export class ImageWidget extends WidgetType {
  constructor(
    readonly src: string,
    readonly alt: string,
    readonly raw: string,
  ) {
    super();
  }

  // eq compares src + alt (the raw is derived from them); identical images reuse
  // the same DOM and CM6 skips a re-render.
  eq(other: ImageWidget): boolean {
    return other.src === this.src && other.alt === this.alt;
  }

  toDOM(): HTMLElement {
    // THE gate: an image src is untrusted page content. sanitizeImageSrc allows
    // only http(s) + app-relative paths; everything else (javascript:, vbscript:,
    // executable data:, protocol-relative //) returns null.
    const safe = sanitizeImageSrc(this.src);
    if (safe == null) {
      // Blocked src → render the RAW markdown as plain text (textContent, never
      // innerHTML; never an <img> that would load/execute the blocked URL). The
      // user sees exactly what they typed — the bytes are unchanged.
      const span = document.createElement("span");
      span.className = "cm-md-image-raw";
      span.textContent = this.raw;
      return span;
    }
    // Safe src → a real <img>, attributes set explicitly (no innerHTML).
    const img = document.createElement("img");
    img.className = "cm-md-image";
    img.setAttribute("src", safe);
    img.setAttribute("alt", this.alt);
    return img;
  }

  // Let pointer events reach the widget so a click on the image can place the
  // cursor (reveal-to-source on the active line is driven by the selection).
  ignoreEvent(): boolean {
    return false;
  }
}

// BulletWidget replaces an unordered list's `ListMark` (-, *, +) with a typographic
// bullet "•" in the READ surface only (clean render). It is an inline widget mounted
// at the marker's own position, so nested-list indentation is preserved naturally
// (the widget sits exactly where the dash was). Edit mode never mounts it — the raw
// dash stays editable there.
export class BulletWidget extends WidgetType {
  eq(): boolean {
    // All bullets are identical — let CM6 reuse one DOM node for every bullet.
    return true;
  }

  toDOM(): HTMLElement {
    const span = document.createElement("span");
    span.className = "cm-md-bullet";
    span.textContent = "•";
    return span;
  }

  ignoreEvent(): boolean {
    return false;
  }
}

// CodeBlockWidget replaces a whole fenced code block with a clean <pre><code> in the
// READ surface only — the ``` fences and the info string disappear, leaving just the
// code on the code background (the language is exposed as a data-lang label via CSS).
//
// SYNTAX HIGHLIGHTING (ED-04a): toDOM renders the plain <code> FIRST (synchronous,
// textContent — today's behavior), THEN, if `lang` matches a curated codeLanguage,
// loads that grammar's lazy chunk and re-emits token <span>s via highlightCode using the
// ONE shared codeHighlightStyle (identical to the live editor). The plain→highlighted
// swap is color-only (zero layout shift) + a CSS fade (D-08).
//
// SECURITY (T-06-08, extended): EVERY node — plain code AND every token span — is built
// with createElement + textContent ONLY. highlightCode hands us (text, classes) per run;
// `text` always goes through textContent / createTextNode, NEVER innerHTML, so untrusted
// code bytes (even markup-looking ones) can never become live markup. Tests assert on
// element structure (tagName/className/textContent), never on HTML strings.
//
// TEARDOWN SAFETY (D-04): the grammar load is async, but the widget may be destroyed or
// its DOM replaced (eq mismatch) before it resolves. The re-highlight is guarded by a
// per-instance `destroyed` flag AND a `codeEl.isConnected` liveness check, so it never
// writes into a stale/detached node. Unknown/absent lang OR `mermaid` (not in the curated
// set → no match) skip the load and stay plain — no error, no throw.
export class CodeBlockWidget extends WidgetType {
  private destroyed = false;

  constructor(
    readonly code: string,
    readonly lang: string,
  ) {
    super();
  }

  eq(other: CodeBlockWidget): boolean {
    return other.code === this.code && other.lang === this.lang;
  }

  toDOM(): HTMLElement {
    const pre = document.createElement("pre");
    pre.className = "cm-md-codeblock";
    if (this.lang) pre.setAttribute("data-lang", this.lang);
    const code = document.createElement("code");
    // Plain render FIRST — synchronous, textContent-only (the safe baseline that ships
    // even if the grammar never loads).
    code.textContent = this.code;
    pre.appendChild(code);
    // Copy-to-clipboard button (EP-02): built with createElement + textContent
    // ONLY (no innerHTML — the no-innerHTML invariant). The button is a SIBLING
    // of <code> inside <pre>; highlight() only touches <code>'s children and
    // <pre>'s classList, so the button never perturbs the async highlight swap
    // or the destroyed/isConnected teardown (those guard codeEl, not the button).
    const btn = document.createElement("button");
    btn.className = "cm-copy-btn";
    btn.type = "button";
    btn.textContent = "Copy";
    btn.setAttribute("aria-label", "Copy code to clipboard");
    btn.addEventListener("click", (e) => {
      e.stopPropagation();
      const write = navigator.clipboard?.writeText?.bind(navigator.clipboard);
      if (!write) {
        btn.textContent = "Failed";
        setTimeout(() => { btn.textContent = "Copy"; }, 1500);
        return;
      }
      write(this.code).then(() => {
        btn.textContent = "Copied";
        setTimeout(() => { btn.textContent = "Copy"; }, 1500);
      }).catch(() => {
        btn.textContent = "Failed";
        setTimeout(() => { btn.textContent = "Copy"; }, 1500);
      });
    });
    pre.appendChild(btn);
    // Then kick off the async re-highlight (fire-and-forget; failures leave plain text).
    void this.highlight(pre, code);
    return pre;
  }

  // highlight resolves `lang` to a curated grammar, loads its lazy chunk, and replaces the
  // <code> children with token spans. Returns early (stays plain) for an unknown/absent
  // lang or `mermaid`. The load is guarded on both sides of the await: a rejected import
  // is swallowed (.catch) and a resolved import is dropped if the widget was torn down or
  // detached meanwhile.
  private async highlight(pre: HTMLElement, codeEl: HTMLElement): Promise<void> {
    if (!this.lang) return;
    // Exact (case-insensitive) name/alias match only — `fuzzy=false` so an UNLISTED info
    // string (e.g. `jsonl`, `diffstat`, `mermaid`) never substring-matches a curated
    // language and is left plain, honoring the documented "unknown → plain" contract.
    const desc = LanguageDescription.matchLanguageName(codeLanguages, this.lang, false);
    if (!desc || !desc.load) return;

    // The ENTIRE async pass — load, parse, and token emit — is guarded: any failure
    // leaves the synchronous plain render untouched (no throw, no unhandled rejection),
    // honoring the "failures leave plain text" contract for the whole pass, not just load.
    try {
      const support = await desc.load();
      // Teardown guard: the widget may have been destroyed or its DOM detached/replaced
      // while the chunk was loading. Never write into a stale node.
      if (this.destroyed || !codeEl.isConnected) return;

      // Build the token DOM into a DETACHED fragment FIRST, then swap atomically. If
      // parse/emit throws, the live <code> is never cleared → the plain baseline survives
      // (no blanking) and the catch below swallows it.
      const tree = support.language.parser.parse(this.code);
      const frag = document.createDocumentFragment();
      highlightCode(
        this.code,
        tree,
        codeHighlightStyle,
        (text, classes) => {
          if (classes) {
            const span = document.createElement("span");
            span.className = classes;
            // textContent ONLY — never innerHTML (T-06-08).
            span.textContent = text;
            frag.appendChild(span);
          } else {
            // Unstyled run → an inert text node (still no innerHTML).
            frag.appendChild(document.createTextNode(text));
          }
        },
        () => {
          // putBreak fires per newline.
          frag.appendChild(document.createTextNode("\n"));
        },
      );
      // Atomic swap: clear the plain text and mount the prebuilt tokens in one step.
      while (codeEl.firstChild) codeEl.removeChild(codeEl.firstChild);
      codeEl.appendChild(frag);
      // Toggle the fade-in class (color-only transition; zero layout shift).
      pre.classList.add("cm-md-codeblock--highlighted");
    } catch {
      // Chunk load, parse, or emit failed → leave the plain render untouched.
      return;
    }
  }

  // CM6 calls destroy when the widget's DOM is removed/replaced — flip the flag so a
  // still-in-flight grammar load won't write into the dead node.
  destroy(): void {
    this.destroyed = true;
  }

  ignoreEvent(): boolean {
    return false;
  }
}

// MermaidWidget replaces a ```mermaid fenced block with the rendered diagram as inline
// SVG, on BOTH CM6 surfaces (read mode + the live editor; livePreview.ts gates the
// emission). It mirrors the Phase-25 CodeBlockWidget async-widget pattern (D-06):
//
//   1. toDOM builds a SYNCHRONOUS raw-code fallback FIRST (the exact CodeBlockWidget
//      <pre>, so a slow/failed/invalid render shows the fenced code the user typed —
//      never a blank widget). mermaid is not a curated grammar, so the reused
//      CodeBlockWidget stays plain (its highlight() returns early) → no nested async.
//   2. Then it lazy-loads + renders mermaid (renderMermaid, its own dynamic-import
//      chunk) and, on success, ATOMICALLY swaps the parsed <svg> in.
//
// SECURITY (D-04, T-06-08): the diagram source is untrusted page content. The script-
// stripping boundary is UPSTREAM: renderMermaid runs under securityLevel:'strict', so
// mermaid pipes the svg through DOMPurify (removing <script>/event-handler attributes)
// AND disables click/interactivity before the string ever reaches this widget — that is
// what guarantees no live script, not this widget's own logic. The widget's job is the
// INJECTION hygiene on top of it: parse via `new DOMParser().parseFromString(svg,
// "image/svg+xml")` + a parsererror check + a non-<svg>-root check + appendChild, NEVER
// innerHTML of page content (so we never re-parse bytes into live markup; and a
// DOMParser-created <script>, having its "already started" flag set, would not execute on
// insertion even if one slipped past DOMPurify). bindFunctions is deliberately NOT called
// (strict disables interactivity). A malformed svg (parsererror, or a non-<svg> root)
// keeps the raw-code fallback. This widget is NOT a second sanitizer — re-sanitizing here
// would be redundant with DOMPurify and a footgun to hand-roll.
//
// FALLBACK (D-05): a parse/render throw (invalid diagram) — or any DOM step failing — is
// swallowed by the whole-pass try/catch, leaving the raw-code fallback. No error escapes
// toDOM; suppressErrorRendering (mermaid.ts) means no leftover temp <div> on the body.
//
// TEARDOWN / STALE GUARD (D-06, Task 3): the render is async, but the widget may be
// destroyed (cursor enters the block → reveal drops the decoration; or the block text
// changed → eq() mismatch → a NEW instance replaces this one) or its DOM detached before
// it resolves. The swap is guarded by a per-instance `destroyed` flag AND
// `wrapper.isConnected`, so a resolved svg is discarded rather than written into a stale
// node. eq() keyed on (code, theme) → an unchanged diagram is reused (not re-rendered)
// when the cursor leaves, so live mode is not per-keystroke (the reveal model + eq).
export class MermaidWidget extends WidgetType {
  private destroyed = false;

  constructor(
    readonly code: string,
    readonly theme: string = "dark",
  ) {
    super();
  }

  eq(other: MermaidWidget): boolean {
    return other.code === this.code && other.theme === this.theme;
  }

  toDOM(view?: EditorView): HTMLElement {
    const wrapper = document.createElement("div");
    wrapper.className = "cm-md-mermaid";
    // Synchronous raw-code fallback FIRST — the exact CodeBlockWidget render (so an
    // unrendered/failed diagram is indistinguishable from a normal fenced code block).
    const fallback = new CodeBlockWidget(this.code, "mermaid").toDOM();
    wrapper.appendChild(fallback);
    // Fire-and-forget async render; a failure leaves the fallback untouched.
    void this.render(wrapper, view);
    return wrapper;
  }

  // render lazy-loads mermaid, renders to an svg string, and (if still live) swaps the
  // parsed <svg> in for the fallback. The ENTIRE pass is guarded: any throw leaves the
  // synchronous fallback intact (no blank, no escaping error, no unhandled rejection).
  private async render(wrapper: HTMLElement, view?: EditorView): Promise<void> {
    try {
      const svg = await renderMermaid(this.code);
      // Teardown/stale guard (post-await): destroyed or detached → discard the result.
      if (this.destroyed || !wrapper.isConnected) return;
      // Parse the (strict-sanitized) svg string WITHOUT innerHTML. A parsererror node or
      // a non-<svg> root → keep the raw-code fallback rather than inject broken markup.
      const parsed = new DOMParser().parseFromString(svg, "image/svg+xml");
      if (parsed.querySelector("parsererror")) return;
      const root = parsed.documentElement;
      if (!root || root.tagName.toLowerCase() !== "svg") return;
      // importNode adopts the node into THIS document (a DOMParser doc is separate).
      const svgEl = document.importNode(root, true);
      // Constrain sizing so a large diagram fits the column (blunts layout shift; CSS
      // also sets a min-height). Set as a single attribute (no innerHTML).
      svgEl.setAttribute("style", "max-width:100%;height:auto");
      // Re-check liveness right before the write (defensive; nothing awaited since).
      if (this.destroyed || !wrapper.isConnected) return;
      // Atomic swap: drop the fallback and mount the <svg> in one step.
      while (wrapper.firstChild) wrapper.removeChild(wrapper.firstChild);
      wrapper.appendChild(svgEl);
      wrapper.classList.add("cm-md-mermaid--rendered");
      // The fallback→svg swap changes block height — ask CM6 to re-measure so cursor
      // coordinates / scroll stay correct. Guarded: tests call toDOM() with no view.
      view?.requestMeasure?.();
    } catch {
      // Invalid diagram (mermaid threw) or a DOM step failed → keep the raw-code fallback.
      return;
    }
  }

  // CM6 calls destroy when the widget's DOM is removed/replaced (cursor-enter reveal, an
  // eq() mismatch on edited text, or unmount) — flip the flag so a still-in-flight render
  // never writes into the dead node.
  destroy(): void {
    this.destroyed = true;
  }

  ignoreEvent(): boolean {
    return false;
  }
}

// HashtagWidget renders a body `#tag` as a small inline badge (ED-02a). It mirrors
// the BulletWidget shape — a one-element <span> built with createElement + textContent
// ONLY. `tag` is the label WITHOUT the leading '#' (e.g. "deploy"); toDOM prepends a
// LITERAL '#' so the badge reads "#deploy" (Obsidian-parity). The tag bytes are
// untrusted page content, so they go through textContent and can never become live
// markup (the no-innerHTML security invariant; widgets.test.ts asserts a single text
// child). The widget is emitted as a NON-atomic inline replace (livePreview.ts) — a
// hashtag is short inline text, so it must not trap the cursor (Pitfall 1).
export class HashtagWidget extends WidgetType {
  constructor(readonly tag: string, readonly color?: string) {
    super();
  }

  // Identical labels + color reuse the same DOM (CM6 skips a re-render).
  eq(other: HashtagWidget): boolean {
    return other.tag === this.tag && other.color === this.color;
  }

  toDOM(): HTMLElement {
    const span = document.createElement("span");
    span.className = "cm-hashtag";
    // The '#' is a literal prefix; only `this.tag` is dynamic and it flows through
    // textContent (never innerHTML) — one inert text node, no markup parsing.
    span.textContent = "#" + this.tag;
    // Apply the tag's color via style.color (never innerHTML). The color is a
    // server-validated hex/palette token; an empty/undefined color falls back to
    // the .cm-hashtag CSS default (var(--color-accent)).
    if (this.color) {
      span.style.color = this.color;
    }
    return span;
  }

  // Let a click reach the widget so it can place the cursor → the active-line reveal
  // swaps the badge back to the raw `#tag` for editing.
  ignoreEvent(): boolean {
    return false;
  }
}

// A parsed GFM table: the header cell texts and the body rows (each a list of cell
// texts). Built by the caller from the Lezer Table subtree and handed in verbatim.
export interface TableData {
  header: string[];
  rows: string[][];
}

// TableWidget renders a GFM table as a styled <table>/<thead>/<tbody> grid. It is a
// BLOCK widget (Decoration.replace({ widget, block: true })) spanning the whole
// Table node range.
export class TableWidget extends WidgetType {
  // The serialized cell text is the identity — two tables with identical text reuse
  // the DOM. JSON of the structured cells is a stable, cheap key.
  private readonly key: string;

  constructor(readonly data: TableData) {
    super();
    this.key = JSON.stringify(data);
  }

  eq(other: TableWidget): boolean {
    return other.key === this.key;
  }

  toDOM(): HTMLElement {
    // Every node + cell text is created via createElement + textContent — NEVER
    // innerHTML — so untrusted cell content can never become live markup (T-06-08).
    const table = document.createElement("table");
    table.className = "cm-md-table";

    const thead = document.createElement("thead");
    const headRow = document.createElement("tr");
    for (const cell of this.data.header) {
      const th = document.createElement("th");
      th.textContent = cell;
      headRow.appendChild(th);
    }
    thead.appendChild(headRow);
    table.appendChild(thead);

    const tbody = document.createElement("tbody");
    for (const row of this.data.rows) {
      const tr = document.createElement("tr");
      for (const cell of row) {
        const td = document.createElement("td");
        td.textContent = cell;
        tr.appendChild(td);
      }
      tbody.appendChild(tr);
    }
    table.appendChild(tbody);

    return table;
  }

  ignoreEvent(): boolean {
    return false;
  }
}
