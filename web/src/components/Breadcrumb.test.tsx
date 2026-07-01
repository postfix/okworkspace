/**
 * Breadcrumb — the folder trail above a page (Obsidian parity). The contracts under
 * test:
 *   1. ancestor FOLDER segments (all but the last path segment) render as clickable
 *      crumbs, in on-disk order, and click-navigate to `/app/page/<folder>/index.md`.
 *   2. the title renders as a NON-interactive tail (`.breadcrumb-current`, not a button).
 *   3. a single-segment path (a top-level page) has no parent trail → renders nothing.
 * Navigation is asserted through a location-probe route (no router-internal spying),
 * mirroring BacklinksPanel.test.tsx.
 */
import { describe, it, expect } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Routes, Route, useLocation } from "react-router-dom";

import Breadcrumb from "./Breadcrumb";

function LocationProbe() {
  const loc = useLocation();
  return <div data-testid="location">{loc.pathname}</div>;
}

function renderCrumb(path: string, title: string) {
  return render(
    <MemoryRouter initialEntries={[`/app/page/${path}`]}>
      <Breadcrumb path={path} title={title} />
      <LocationProbe />
      <Routes>
        <Route path="/app/page/*" element={null} />
      </Routes>
    </MemoryRouter>,
  );
}

describe("Breadcrumb", () => {
  it("renders ancestor folder crumbs in order, then the current title as a non-interactive tail", () => {
    renderCrumb("Cooking/Recipes/Salo.md", "Salo");

    // Only the ancestor FOLDERS become crumb buttons (the file segment does not).
    const crumbs = screen.getAllByRole("button");
    expect(crumbs.map((b) => b.textContent)).toEqual(["Cooking", "Recipes"]);

    // The title is present but NOT a button — it is the current, non-interactive tail.
    const current = screen.getByText("Salo");
    expect(current).toHaveClass("breadcrumb-current");
    expect(current.tagName).not.toBe("BUTTON");
    expect(screen.queryByRole("button", { name: "Salo" })).toBeNull();
  });

  it("navigates a top-level folder crumb to that folder's index page", async () => {
    const user = userEvent.setup();
    renderCrumb("Cooking/Recipes/Salo.md", "Salo");

    await user.click(screen.getByRole("button", { name: "Cooking" }));
    await waitFor(() =>
      expect(screen.getByTestId("location")).toHaveTextContent(
        "/app/page/Cooking/index.md",
      ),
    );
  });

  it("navigates a nested folder crumb to its CUMULATIVE folder path index", async () => {
    const user = userEvent.setup();
    renderCrumb("Cooking/Recipes/Salo.md", "Salo");

    // The second crumb resolves to the joined ancestor path, not just its own segment.
    await user.click(screen.getByRole("button", { name: "Recipes" }));
    await waitFor(() =>
      expect(screen.getByTestId("location")).toHaveTextContent(
        "/app/page/Cooking/Recipes/index.md",
      ),
    );
  });

  it("renders every ancestor folder for a deeply nested path, in order", () => {
    renderCrumb("a/b/c/d.md", "D");
    expect(screen.getAllByRole("button").map((b) => b.textContent)).toEqual([
      "a",
      "b",
      "c",
    ]);
  });

  it("renders nothing for a single-segment (top-level) path — no nav, no crumbs", () => {
    const { container } = render(
      <MemoryRouter>
        <Breadcrumb path="note.md" title="Note" />
      </MemoryRouter>,
    );
    expect(container.firstChild).toBeNull();
    expect(screen.queryByRole("navigation")).toBeNull();
    expect(screen.queryByRole("button")).toBeNull();
  });
});
