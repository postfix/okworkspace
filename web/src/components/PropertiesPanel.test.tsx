/**
 * PropertiesPanel — the Obsidian metadata grid (tags / description / arbitrary scalar
 * keys). Contracts under test:
 *   1. tag chips = frontmatter tags ∪ inline body #tags, de-duped case-insensitively.
 *      Inline-origin tags are body-managed → NO ✕; non-inline tags → ✕ calls onRemoveTag.
 *   2. scalar rows come from listScalarFields MINUS the reserved keys (title/tags/
 *      description), which have their own rows.
 *   3. edit mode: scalar values are inputs (onChange → onSetField), each with a ✕
 *      (→ onRemoveField); an add-property row (two inputs + Add) calls onSetField;
 *      the ✨ slots render.
 *   4. read mode: values are plain text — no ✕, no inputs, no add-row, no slots — and
 *      the whole panel renders NOTHING when there are no tags/description/scalars.
 * PropertiesPanel is pure/presentational (no router, no query), so no wrappers needed.
 */
import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import PropertiesPanel from "./PropertiesPanel";

describe("PropertiesPanel — tag chips (inline vs non-inline origin)", () => {
  it("gives non-inline (frontmatter-only) tags a ✕ and inline-origin tags none", async () => {
    const onRemoveTag = vi.fn();
    const user = userEvent.setup();
    render(
      <PropertiesPanel
        body="a note referencing #deploy"
        frontmatter={"tags:\n  - deploy\n  - release\n"}
        readOnly={false}
        onRemoveTag={onRemoveTag}
      />,
    );

    // deploy is also inline in the body → body-managed → no ✕.
    expect(screen.queryByRole("button", { name: "Remove tag deploy" })).toBeNull();
    // release is frontmatter-only → removable.
    const removeRelease = screen.getByRole("button", { name: "Remove tag release" });
    await user.click(removeRelease);
    expect(onRemoveTag).toHaveBeenCalledTimes(1);
    expect(onRemoveTag).toHaveBeenCalledWith("release");
  });

  it("renders an inline-only body tag as a chip with no ✕", () => {
    render(
      <PropertiesPanel
        body="cooking log #cooking"
        frontmatter=""
        readOnly={false}
        onRemoveTag={vi.fn()}
      />,
    );
    expect(screen.getByText("#cooking")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Remove tag/ })).toBeNull();
  });

  it("de-dupes a frontmatter tag that is also inline (case-insensitive) into one chip with no ✕", () => {
    render(
      <PropertiesPanel
        body="see #cooking"
        frontmatter={"tags:\n  - Cooking\n"}
        readOnly={false}
        onRemoveTag={vi.fn()}
      />,
    );
    // One chip only, preserving the frontmatter's original casing, and no remove
    // control because it is inline-managed.
    expect(screen.getByText("#Cooking")).toBeInTheDocument();
    expect(screen.queryByText("#cooking")).toBeNull();
    expect(screen.queryByRole("button", { name: /Remove tag/ })).toBeNull();
  });
});

describe("PropertiesPanel — scalar rows", () => {
  const fm =
    "title: My Page\ntags:\n  - a\ndescription: A quiet page\nauthor: Ada\nstatus: draft\n";

  it("renders one row per NON-reserved scalar key and excludes title/tags/description", () => {
    render(
      <PropertiesPanel body="" frontmatter={fm} readOnly onRemoveTag={vi.fn()} />,
    );
    // Reserved keys are shown through their dedicated rows, not as scalar keys.
    expect(screen.getByText("author")).toBeInTheDocument();
    expect(screen.getByText("status")).toBeInTheDocument();
    // The description VALUE shows in its own row (stripped), not as a scalar key row.
    expect(screen.getByText("A quiet page")).toBeInTheDocument();
    // No scalar key labelled "title"/"tags"/"description".
    expect(screen.queryByText("My Page")).toBeNull();
  });

  it("edit mode: scalar values are inputs whose change calls onSetField(key, value)", () => {
    const onSetField = vi.fn();
    render(
      <PropertiesPanel
        body=""
        frontmatter={fm}
        readOnly={false}
        onSetField={onSetField}
      />,
    );
    const authorInput = screen.getByLabelText("author value") as HTMLInputElement;
    expect(authorInput.value).toBe("Ada");
    // The input is controlled by the prop; a change event carries the new value to
    // the callback (the host owns the frontmatter, so the value only echoes props).
    fireEvent.change(authorInput, { target: { value: "Grace" } });
    expect(onSetField).toHaveBeenCalledWith("author", "Grace");
  });

  it("edit mode: each scalar row's ✕ calls onRemoveField(key)", async () => {
    const onRemoveField = vi.fn();
    const user = userEvent.setup();
    render(
      <PropertiesPanel
        body=""
        frontmatter={fm}
        readOnly={false}
        onRemoveField={onRemoveField}
      />,
    );
    await user.click(screen.getByRole("button", { name: "Remove property status" }));
    expect(onRemoveField).toHaveBeenCalledTimes(1);
    expect(onRemoveField).toHaveBeenCalledWith("status");
  });

  it("read mode: scalar values are plain text — no inputs, no per-row ✕", () => {
    render(
      <PropertiesPanel body="" frontmatter={fm} readOnly onRemoveField={vi.fn()} />,
    );
    expect(screen.getByText("Ada")).toBeInTheDocument();
    expect(screen.queryByLabelText("author value")).toBeNull();
    expect(screen.queryByRole("button", { name: /Remove property/ })).toBeNull();
  });
});

describe("PropertiesPanel — add-property row (edit only)", () => {
  it("typing a key+value and clicking Add calls onSetField(newKey, newValue)", async () => {
    const onSetField = vi.fn();
    const user = userEvent.setup();
    render(
      <PropertiesPanel
        body=""
        frontmatter=""
        readOnly={false}
        onSetField={onSetField}
      />,
    );
    await user.type(screen.getByLabelText("New property name"), "priority");
    await user.type(screen.getByLabelText("New property value"), "high");
    await user.click(screen.getByRole("button", { name: "Add property" }));
    expect(onSetField).toHaveBeenCalledWith("priority", "high");
  });

  it("the Add button is disabled until a key is entered", async () => {
    const user = userEvent.setup();
    render(
      <PropertiesPanel body="" frontmatter="" readOnly={false} onSetField={vi.fn()} />,
    );
    const add = screen.getByRole("button", { name: "Add property" });
    expect(add).toBeDisabled();
    await user.type(screen.getByLabelText("New property name"), "priority");
    expect(add).toBeEnabled();
  });

  it("does not render the add-property row in read mode", () => {
    render(
      <PropertiesPanel
        body=""
        frontmatter={"author: Ada\n"}
        readOnly
      />,
    );
    expect(screen.queryByLabelText("New property name")).toBeNull();
    expect(screen.queryByRole("button", { name: "Add property" })).toBeNull();
  });
});

describe("PropertiesPanel — read-mode emptiness + slot gating", () => {
  it("renders nothing in read mode when there are no tags, description, or scalars", () => {
    const { container } = render(
      <PropertiesPanel body="plain body, no tags" frontmatter="" readOnly />,
    );
    expect(container.firstChild).toBeNull();
    expect(screen.queryByRole("region", { name: "Properties" })).toBeNull();
  });

  it("still renders the panel in edit mode even when empty (so a user can add)", () => {
    render(
      <PropertiesPanel body="" frontmatter="" readOnly={false} onSetField={vi.fn()} />,
    );
    expect(screen.getByRole("region", { name: "Properties" })).toBeInTheDocument();
    expect(screen.getByLabelText("New property name")).toBeInTheDocument();
  });

  it("renders the ✨ slots only in edit mode", () => {
    const { rerender } = render(
      <PropertiesPanel
        body=""
        frontmatter={"description: hi\n"}
        readOnly={false}
        tagSuggestSlot={<span data-testid="tag-slot">suggest</span>}
        descriptionGenerateSlot={<span data-testid="desc-slot">generate</span>}
      />,
    );
    expect(screen.getByTestId("tag-slot")).toBeInTheDocument();
    expect(screen.getByTestId("desc-slot")).toBeInTheDocument();

    // Same content in read mode: the slots are gone (no editing affordances).
    rerender(
      <PropertiesPanel
        body=""
        frontmatter={"description: hi\n"}
        readOnly
        tagSuggestSlot={<span data-testid="tag-slot">suggest</span>}
        descriptionGenerateSlot={<span data-testid="desc-slot">generate</span>}
      />,
    );
    expect(screen.queryByTestId("tag-slot")).toBeNull();
    expect(screen.queryByTestId("desc-slot")).toBeNull();
  });
});
