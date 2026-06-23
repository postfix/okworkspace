/**
 * AgentAnswer — the streamed-answer render region. The load-bearing contracts:
 * answers render through the sanitized MarkdownProse surface (no raw HTML → no
 * stored XSS, T-04-21), the region is an aria-live region, workspace citations
 * render as deep-links, and the error state keeps the partial answer.
 */
import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";

import AgentAnswer from "./AgentAnswer";

function renderAnswer(ui: React.ReactElement) {
  return render(<MemoryRouter>{ui}</MemoryRouter>);
}

describe("AgentAnswer", () => {
  it("renders the streamed Markdown and is an aria-live region", () => {
    const { container } = renderAnswer(
      <AgentAnswer
        answer="Hello **world**"
        streaming
        currentPath="guides/welcome.md"
      />,
    );
    // Bold Markdown renders through MarkdownProse (a <strong>, not literal **).
    expect(screen.getByText("world").tagName.toLowerCase()).toBe("strong");
    // The answer region is a polite live region announcing as it streams.
    const live = container.querySelector('[aria-live="polite"]');
    expect(live).toBeTruthy();
    expect(live!.getAttribute("aria-busy")).toBe("true");
  });

  it("does NOT render raw HTML in the answer (stored-XSS guard, T-04-21)", () => {
    const { container } = renderAnswer(
      <AgentAnswer
        answer={'<img src=x onerror="alert(1)">done'}
        streaming={false}
        currentPath="guides/welcome.md"
      />,
    );
    // The sanitized surface (rehype-raw OFF) must not emit an <img> from raw HTML.
    expect(container.querySelector("img")).toBeNull();
  });

  it("renders workspace citations as deep-links", () => {
    renderAnswer(
      <AgentAnswer
        answer="An answer."
        streaming={false}
        citations={["guides/deploy.md", "runbooks/index.md"]}
        currentPath=""
      />,
    );
    expect(screen.getByText("Reasoned over:")).toBeTruthy();
    const deploy = screen.getByRole("link", { name: "deploy" });
    expect(deploy.getAttribute("href")).toBe("/app/page/guides/deploy.md");
    // index.md collapses to its folder name in the label.
    expect(screen.getByRole("link", { name: "runbooks" })).toBeTruthy();
  });

  it("keeps the partial answer and shows the error row on failure", () => {
    renderAnswer(
      <AgentAnswer
        answer="A partial answer so far."
        streaming={false}
        error="Something went wrong while answering. Your prompt is kept — try again."
        currentPath="guides/welcome.md"
      />,
    );
    // The partial answer is kept.
    expect(screen.getByText("A partial answer so far.")).toBeTruthy();
    // The error is surfaced as an alert.
    const alert = screen.getByRole("alert");
    expect(alert.textContent).toContain("Your prompt is kept");
  });
});
