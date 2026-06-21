// SRCH-06 — heading anchor slugs must equal the github-slugger ids the backend
// okf.ScanHeadings assigns, so search-result `#anchor` deep-links land correctly
// on the unified read surface. These cases mirror the backend's slug behavior
// (lowercase; keep letters/numbers/'-'/'_'; space→'-'; NO collapse, NO trim) and
// the -N dedup suffix.
import { describe, it, expect } from "vitest";

import { slug, dedupSlug, headingText } from "./headingAnchors";

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
});
