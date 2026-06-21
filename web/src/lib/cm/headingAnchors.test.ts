// SRCH-06 — heading anchor slugs must equal the github-slugger ids the backend
// okf.ScanHeadings assigns, so search-result `#anchor` deep-links land correctly
// on the unified read surface. These cases mirror the backend's slug behavior
// (lowercase; keep letters/numbers/'-'/'_'; space→'-'; NO collapse, NO trim) and
// the -N dedup suffix.
import { describe, it, expect, afterEach } from "vitest";
import { EditorState } from "@codemirror/state";
import { EditorView } from "@codemirror/view";

import { markdownExtension } from "./markdown";
import {
  slug,
  dedupSlug,
  headingText,
  headingAnchors,
  scrollToHash,
} from "./headingAnchors";
import { cmCorpus } from "../../test/cmCorpus";

describe("slug (SRCH-06, mirrors okf.ScanHeadings)", () => {
  it("slugs the okf corpus headings", () => {
    // From internal/okf/testdata/corpus/* headings.
    expect(slug("Top Heading")).toBe("top-heading");
    expect(slug("Second Level")).toBe("second-level");
    expect(slug("Third Level")).toBe("third-level");
    expect(slug("Links and Tables")).toBe("links-and-tables");
    expect(slug("CRLF Body")).toBe("crlf-body");
    expect(slug("Tricky Body")).toBe("tricky-body");
    expect(slug("Body")).toBe("body");
  });

  it("lowercases and drops punctuation/symbols", () => {
    expect(slug("Hello, World!")).toBe("hello-world");
    expect(slug("API & SDK")).toBe("api--sdk");
    expect(slug("v1.2.3 Release")).toBe("v123-release");
  });

  it("keeps '-' and '_' but no other punctuation", () => {
    expect(slug("snake_case-and-kebab")).toBe("snake_case-and-kebab");
  });

  it("does NOT collapse whitespace or trim hyphens (github-slugger parity)", () => {
    // Two spaces → two hyphens; leading/trailing space → leading/trailing hyphen.
    expect(slug("a  b")).toBe("a--b");
    expect(slug(" lead and trail ")).toBe("-lead-and-trail-");
  });

  it("keeps Unicode letters and numbers", () => {
    expect(slug("Café Münü")).toBe("café-münü");
    expect(slug("Über 2 Dinge")).toBe("über-2-dinge");
  });
});

describe("dedupSlug (mirrors okf.dedupSlug)", () => {
  it("returns base first, then base-1, base-2 …", () => {
    const occ = new Map<string, number>();
    expect(dedupSlug(occ, "intro")).toBe("intro");
    expect(dedupSlug(occ, "intro")).toBe("intro-1");
    expect(dedupSlug(occ, "intro")).toBe("intro-2");
    expect(dedupSlug(occ, "other")).toBe("other");
  });
});

describe("headingText", () => {
  it("strips the leading '#' run and the single required space", () => {
    expect(headingText("# Top Heading")).toBe("Top Heading");
    expect(headingText("### Third Level")).toBe("Third Level");
    expect(headingText("#   Padded")).toBe("Padded");
  });

  it("returns non-heading lines unchanged", () => {
    expect(headingText("not a heading")).toBe("not a heading");
    expect(headingText("#NoSpace")).toBe("#NoSpace");
  });

  it("strips an ATX closing '#' run preceded by whitespace (CommonMark §4.2, okf.trimATXClosing parity)", () => {
    // A closing '#' run preceded by whitespace is removed along with that space.
    expect(headingText("## My Title ##")).toBe("My Title");
    expect(headingText("## Title ###")).toBe("Title");
    expect(headingText("# Foo #")).toBe("Foo");
    expect(headingText("## Baz ## ")).toBe("Baz");
    // No whitespace before the '#' run → NOT a closer; the '#'s stay in the text.
    expect(headingText("## foo#")).toBe("foo#");
    expect(headingText("### Bar###")).toBe("Bar###");
    // No closing marker → unchanged.
    expect(headingText("## Title")).toBe("Title");
  });

  it("slug(headingText(line)) matches okf.ScanHeadings for closing-marker headings", () => {
    // Backend: "## My Title ##" → trimATXClosing → "My Title" → slug "my-title".
    // Without trimATXClosing the trailing space would slug to "my-title-".
    expect(slug(headingText("## My Title ##"))).toBe("my-title");
    expect(slug(headingText("## Title ###"))).toBe("title");
    // "## foo#" is not a closer → "foo#" → slug drops '#' → "foo".
    expect(slug(headingText("## foo#"))).toBe("foo");
    expect(slug(headingText("## Title"))).toBe("title");
  });
});

// ---------------------------------------------------------------------------
// headingAnchors() extension — the RENDERED heading-line id MUST equal slug(text)
// (deduped) for the okf corpus headings. This is the SRCH-06 preservation gate:
// the read surface's heading ids must agree byte-for-byte with okf.ScanHeadings'
// backend anchors so a search-result `#anchor` deep-link lands on the right line.
// ---------------------------------------------------------------------------

const views: EditorView[] = [];
afterEach(() => {
  for (const v of views) v.destroy();
  views.length = 0;
});

// mountRead builds a headless read-only EditorView with the markdown language +
// headingAnchors field, seeds `doc`, and returns it. (A small fresh doc reports as
// fully visible under jsdom, and the field is whole-document anyway.)
function mountRead(doc: string): EditorView {
  const parent = document.createElement("div");
  const view = new EditorView({
    parent,
    state: EditorState.create({
      doc,
      extensions: [
        markdownExtension,
        headingAnchors,
        EditorState.readOnly.of(true),
        EditorView.editable.of(false),
      ],
    }),
  });
  views.push(view);
  return view;
}

// renderedHeadingIds returns the id of every .cm-line element that carries one, in
// DOM (document) order — i.e. the rendered heading ids the field stamped.
function renderedHeadingIds(view: EditorView): string[] {
  return Array.from(view.dom.querySelectorAll<HTMLElement>(".cm-line[id]")).map(
    (el) => el.id,
  );
}

describe("headingAnchors() rendered ids (SRCH-06, mirror okf.ScanHeadings)", () => {
  it("stamps each heading line with id === slug(text)", () => {
    const view = mountRead(
      "# Top Heading\n\nintro\n\n## Second Level\n\n### Third Level\n",
    );
    expect(renderedHeadingIds(view)).toEqual([
      "top-heading",
      "second-level",
      "third-level",
    ]);
  });

  it("dedups duplicate headings as base, base-1, base-2 (okf.ScanHeadings parity)", () => {
    const view = mountRead("# Intro\n\n## Intro\n\n## Intro\n\n# Other\n");
    expect(renderedHeadingIds(view)).toEqual([
      "intro",
      "intro-1",
      "intro-2",
      "other",
    ]);
  });

  it("ids are NEVER user-content-prefixed", () => {
    const view = mountRead("# My Section\n");
    const ids = renderedHeadingIds(view);
    expect(ids).toEqual(["my-section"]);
    for (const id of ids) {
      expect(id.startsWith("user-content-")).toBe(false);
    }
  });

  it("matches slug(headingText(line)) for the okf corpus headings", () => {
    // Re-derive the expected ids the SAME way the backend okf.ScanHeadings does:
    // walk each corpus body line, slug+dedup its heading text, and assert the
    // rendered ids equal that sequence.
    for (const fixture of cmCorpus) {
      const occ = new Map<string, number>();
      const expected: string[] = [];
      for (const rawLine of fixture.text.split("\n")) {
        const line = rawLine.replace(/\r$/, "");
        if (/^ {0,3}#{1,6}([ \t]|$)/.test(line)) {
          expected.push(dedupSlug(occ, slug(headingText(line))));
        }
      }
      const view = mountRead(fixture.text);
      expect(renderedHeadingIds(view)).toEqual(expected);
    }
  });
});

describe("scrollToHash (deep-link on mount, T-06-12)", () => {
  it("scrolls to the heading line whose id matches the hash", () => {
    const view = mountRead("# One\n\n## Two\n\n## Two\n");
    const el = view.dom.querySelector<HTMLElement>("#two-1");
    expect(el).not.toBeNull();
    let scrolledTo: HTMLElement | null = null;
    el!.scrollIntoView = () => {
      scrolledTo = el;
    };
    expect(scrollToHash(view, "#two-1")).toBe(true);
    expect(scrolledTo).toBe(el);
  });

  it("returns false for an empty hash or an unknown id", () => {
    const view = mountRead("# Only\n");
    expect(scrollToHash(view, "")).toBe(false);
    expect(scrollToHash(view, "#")).toBe(false);
    expect(scrollToHash(view, "#nope")).toBe(false);
  });

  it("URL-decodes a percent-encoded Unicode anchor before matching", () => {
    const view = mountRead("# Café\n");
    const el = view.dom.querySelector<HTMLElement>("#café");
    expect(el).not.toBeNull();
    let scrolled = false;
    el!.scrollIntoView = () => {
      scrolled = true;
    };
    expect(scrollToHash(view, "#caf%C3%A9")).toBe(true);
    expect(scrolled).toBe(true);
  });
});
