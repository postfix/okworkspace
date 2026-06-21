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
import { sanitizeImageSrc } from "./sanitizeSrc";

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
