/**
 * WR-03 — renderHighlight decodes the server's HTML entities so snippets read
 * correctly (no visible &lt;/&amp;), while staying XSS-safe (React text nodes,
 * no dangerouslySetInnerHTML).
 */
import { describe, it, expect } from "vitest";
import { render } from "@testing-library/react";
import { createElement } from "react";

import { renderHighlight } from "./highlight";

function renderNode(fragment: string) {
  return render(createElement("div", { "data-testid": "hl" }, renderHighlight(fragment)));
}

describe("renderHighlight", () => {
  it("decodes server-escaped entities to literal characters (WR-03)", () => {
    // The server escapes surrounding text: `config <value> & "q"` arrives as
    // `config &lt;value&gt; &amp; &#34;q&#34;`.
    const { getByTestId } = renderNode(
      'config &lt;value&gt; &amp; &#34;q&#34;',
    );
    expect(getByTestId("hl").textContent).toBe('config <value> & "q"');
  });

  it("bolds the matched term while decoding surrounding entities", () => {
    const { getByTestId } = renderNode(
      "set &lt;x&gt; <strong>match</strong> &amp; go",
    );
    const el = getByTestId("hl");
    expect(el.textContent).toBe("set <x> match & go");
    const strongs = el.querySelectorAll("strong");
    expect(strongs).toHaveLength(1);
    expect(strongs[0].textContent).toBe("match");
  });

  it("does not collapse a double-escaped ampersand into a tag (&amp;lt; stays &lt;)", () => {
    // &amp;lt; means a literal "&lt;" in the source text — must NOT become "<".
    const { getByTestId } = renderNode("a &amp;lt; b");
    expect(getByTestId("hl").textContent).toBe("a &lt; b");
  });

  it("keeps an injected escaped <script> as literal text, never a live node (XSS guard)", () => {
    // An injected tag in page content arrives escaped from the server. After
    // decoding it is pushed as a React TEXT node, so React re-escapes it — it
    // shows as literal characters and never becomes a DOM <script>.
    const { getByTestId } = renderNode(
      "before &lt;script&gt;alert(1)&lt;/script&gt; after",
    );
    const el = getByTestId("hl");
    expect(el.textContent).toBe("before <script>alert(1)</script> after");
    // No live <script> element was created.
    expect(el.querySelector("script")).toBeNull();
  });
});
