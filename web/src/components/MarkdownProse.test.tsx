/**
 * remark-hashtag (EP-01) — the remark plugin that renders body `#tag` as a
 * safe badge `<span class="cm-hashtag">` in the react-markdown read surface.
 *
 * Security contract: the badge is a typed hast element (via data.hName), NOT
 * raw HTML. rehype-raw stays OFF; rehype-sanitize stays ON. The tag text flows
 * through as a hast text node — never parsed as markup. These tests assert the
 * rendered DOM structure (a span.cm-hashtag with the tag text) and that
 * false-`#` cases (inline code, escapes, headings, URLs) stay literal.
 */
import { describe, it, expect } from "vitest";
import { render } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";

import MarkdownProse from "./MarkdownProse";

function renderMd(body: string) {
  return render(
    <MemoryRouter>
      <MarkdownProse body={body} currentPath="test.md" />
    </MemoryRouter>,
  );
}

describe("remarkHashtag (EP-01 — #tag badge in MarkdownProse)", () => {
  it("renders a body #tag as a span.cm-hashtag badge", () => {
    const { container } = renderMd("Hello #deploy world");
    const badge = container.querySelector("span.cm-hashtag");
    expect(badge).not.toBeNull();
    expect(badge!.textContent).toBe("#deploy");
  });

  it("renders multiple #tags in one line", () => {
    const { container } = renderMd("#alpha and #beta/gamma");
    const badges = container.querySelectorAll("span.cm-hashtag");
    expect(badges.length).toBe(2);
    expect(badges[0].textContent).toBe("#alpha");
    expect(badges[1].textContent).toBe("#beta/gamma");
  });

  it("does NOT badge a # inside inline code (stays literal)", () => {
    const { container } = renderMd("Use `#tag` for tagging");
    // No badge — the # is inside a code node, never walked.
    expect(container.querySelectorAll("span.cm-hashtag").length).toBe(0);
    // The code element holds the literal text.
    const code = container.querySelector("code");
    expect(code).not.toBeNull();
    expect(code!.textContent).toContain("#tag");
  });

  it("does NOT badge an all-digit #run (e.g. #1984)", () => {
    const { container } = renderMd("The year #1984 was pivotal");
    expect(container.querySelectorAll("span.cm-hashtag").length).toBe(0);
  });

  it("does NOT badge a # inside a word (foo#bar)", () => {
    const { container } = renderMd("foo#bar is not a tag");
    expect(container.querySelectorAll("span.cm-hashtag").length).toBe(0);
  });

  it("does NOT badge an ATX heading marker (# Heading)", () => {
    const { container } = renderMd("# A Title\n\nbody text");
    // The heading's # is a block marker, not inline text → no badge.
    expect(container.querySelectorAll("span.cm-hashtag").length).toBe(0);
    // The heading renders as an h1.
    expect(container.querySelector("h1")).not.toBeNull();
  });

  it("does NOT badge a \\# escape (stays literal)", () => {
    const { container } = renderMd("This is \\#not-a-tag");
    expect(container.querySelectorAll("span.cm-hashtag").length).toBe(0);
  });

  it("does NOT badge a # in a URL fragment (http://x/#frag)", () => {
    const { container } = renderMd("See http://example.com/#frag");
    expect(container.querySelectorAll("span.cm-hashtag").length).toBe(0);
  });

  it("badges a # at the start of a line", () => {
    const { container } = renderMd("#deploy at line start");
    const badge = container.querySelector("span.cm-hashtag");
    expect(badge).not.toBeNull();
    expect(badge!.textContent).toBe("#deploy");
  });

  it("does NOT render raw HTML in the answer (stored-XSS guard holds, T-04-21)", () => {
    const { container } = renderMd('<img src=x onerror="alert(1)">#tag');
    // rehype-raw OFF → no <img> from raw HTML.
    expect(container.querySelector("img")).toBeNull();
    // The #tag still badges (it's a typed element, not raw HTML).
    const badge = container.querySelector("span.cm-hashtag");
    expect(badge).not.toBeNull();
    expect(badge!.textContent).toBe("#tag");
  });
});