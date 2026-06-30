// TagBadgeBar — ED-02c: a read-only effective-tag bar. Inline-origin tags (in the
// body) carry NO ✕; non-inline (LLM/imported) tags get an ✕ that calls onRemoveTag.
import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import TagBadgeBar from "./TagBadgeBar";

describe("TagBadgeBar", () => {
  it("renders effective tags; only NON-inline tags have a Remove ✕", () => {
    const onRemoveTag = vi.fn();
    render(
      <TagBadgeBar
        body="see #inline"
        frontmatter={"tags:\n  - inline\n  - llm\n"}
        onRemoveTag={onRemoveTag}
      />,
    );

    // Both badges render.
    expect(screen.getByText("#inline")).toBeInTheDocument();
    expect(screen.getByText("#llm")).toBeInTheDocument();

    // Only `llm` (non-inline) is removable; the inline-origin `inline` is not.
    expect(
      screen.getByRole("button", { name: "Remove tag llm" }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Remove tag inline" }),
    ).toBeNull();

    // Clicking the ✕ removes only `llm`.
    fireEvent.click(screen.getByRole("button", { name: "Remove tag llm" }));
    expect(onRemoveTag).toHaveBeenCalledTimes(1);
    expect(onRemoveTag).toHaveBeenCalledWith("llm");
  });

  it("de-dupes a body #Deploy against a frontmatter `deploy` into ONE inline badge", () => {
    render(
      <TagBadgeBar
        body="ship #Deploy"
        frontmatter={"tags:\n  - deploy\n"}
        onRemoveTag={() => {}}
      />,
    );
    // One badge, inline-origin (no ✕) — the frontmatter casing is shown.
    expect(screen.getByText("#deploy")).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /Remove tag/i }),
    ).toBeNull();
  });

  it("shows an inline tag typed into the body before any save (live body)", () => {
    render(
      <TagBadgeBar
        body="fresh #idea"
        frontmatter={"title: T\n"}
        onRemoveTag={() => {}}
      />,
    );
    expect(screen.getByText("#idea")).toBeInTheDocument();
    // Inline-origin ⇒ no removal control.
    expect(
      screen.queryByRole("button", { name: "Remove tag idea" }),
    ).toBeNull();
  });

  it("renders nothing when there are no effective tags", () => {
    const { container } = render(
      <TagBadgeBar
        body="no tags here"
        frontmatter={"title: T\n"}
        onRemoveTag={() => {}}
      />,
    );
    expect(container.querySelector(".tag-badge")).toBeNull();
  });
});
