/**
 * NavSearchResults — the inline navrail full-text surface that REPLACED the ⌘K
 * modal palette (RS-02). Contracts under test:
 *   1. GROUPING: results render grouped Pages → Headings → Attachments in that
 *      order; a kind with zero results omits its group label.
 *   2. OPEN + CLEAR: opening a heading result navigates to /app/page/<path>#<id>
 *      (the server anchor's leading '#' stripped for the DOM id, kept in the route
 *      hash) and clears the navSearch query so the tree returns; a plain page
 *      result navigates to /app/page/<path> (no hash) and also clears.
 *   3. STATES: isError → role="alert" "unavailable"; empty data → "No matches";
 *      loading (no data) → the delayed "Searching…" indicator after ~150ms.
 *
 * useSearch is mocked so each test controls { data, isLoading, isError };
 * react-router-dom's useNavigate is spied. The parent (AppShell) only mounts this
 * when the query is non-empty, so a non-empty query is seeded in beforeEach.
 */
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, act } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import type * as RouterDom from "react-router-dom";

import type { SearchResult } from "../../api/client";

const navigateMock = vi.fn();

// Mutable per-test hook return; the factory reads .value at call time.
const searchState: {
  value: {
    data: SearchResult[] | undefined;
    isLoading: boolean;
    isError: boolean;
  };
} = { value: { data: undefined, isLoading: false, isError: false } };

vi.mock("react-router-dom", async (importOriginal) => {
  const actual = await importOriginal<typeof RouterDom>();
  return { ...actual, useNavigate: () => navigateMock };
});

vi.mock("../../hooks/useSearch", () => ({
  useSearch: () => searchState.value,
}));

import NavSearchResults from "./NavSearchResults";
import { useNavSearch } from "../../stores/navSearch";

function renderResults() {
  return render(
    <MemoryRouter>
      <NavSearchResults />
    </MemoryRouter>,
  );
}

const pageResult: SearchResult = {
  kind: "page",
  title: "Welcome",
  path: "welcome.md",
  snippet: "a welcome page",
};
const headingResult: SearchResult = {
  kind: "heading",
  title: "My Section",
  path: "notes.md",
  snippet: "",
  anchor: "#my-section",
  page_title: "Notes",
};
const attachmentResult: SearchResult = {
  kind: "attachment",
  title: "diagram.png",
  path: "notes.md",
  snippet: "",
  page_title: "Notes",
};

beforeEach(() => {
  navigateMock.mockReset();
  searchState.value = { data: undefined, isLoading: false, isError: false };
  // Parent precondition: this component only mounts on a non-empty query.
  useNavSearch.getState().setQuery("q");
});

afterEach(() => {
  useNavSearch.getState().setQuery("");
});

describe("NavSearchResults — grouping", () => {
  it("renders the three group labels Pages → Headings → Attachments in order", () => {
    searchState.value = {
      data: [pageResult, headingResult, attachmentResult],
      isLoading: false,
      isError: false,
    };
    const { container } = renderResults();

    const labels = Array.from(
      container.querySelectorAll(".navsearch-group-label"),
    ).map((n) => n.textContent);
    expect(labels).toEqual(["Pages", "Headings", "Attachments"]);
  });

  it("omits a group whose kind has zero results (no attachments → no Attachments label)", () => {
    searchState.value = {
      data: [pageResult, headingResult],
      isLoading: false,
      isError: false,
    };
    renderResults();

    expect(screen.getByText("Pages")).toBeInTheDocument();
    expect(screen.getByText("Headings")).toBeInTheDocument();
    expect(screen.queryByText("Attachments")).toBeNull();
  });
});

describe("NavSearchResults — open + query clear", () => {
  it("opening a heading result navigates to /app/page/<path>#<id-without-leading-hash> and clears the query", async () => {
    const user = userEvent.setup();
    searchState.value = {
      data: [headingResult],
      isLoading: false,
      isError: false,
    };
    renderResults();

    await user.click(screen.getByRole("option", { name: /My Section/ }));

    expect(navigateMock).toHaveBeenCalledTimes(1);
    expect(navigateMock).toHaveBeenCalledWith("/app/page/notes.md#my-section");
    expect(useNavSearch.getState().query).toBe("");
  });

  it("opening a plain page result navigates to /app/page/<path> with no hash and clears the query", async () => {
    const user = userEvent.setup();
    searchState.value = {
      data: [pageResult],
      isLoading: false,
      isError: false,
    };
    renderResults();

    await user.click(screen.getByRole("option", { name: /Welcome/ }));

    expect(navigateMock).toHaveBeenCalledTimes(1);
    expect(navigateMock).toHaveBeenCalledWith("/app/page/welcome.md");
    expect(useNavSearch.getState().query).toBe("");
  });
});

describe("NavSearchResults — states", () => {
  it("renders an alert when the search is unavailable (isError)", () => {
    searchState.value = { data: undefined, isLoading: false, isError: true };
    renderResults();

    const alert = screen.getByRole("alert");
    expect(alert).toHaveTextContent(/unavailable/i);
  });

  it("renders a No-matches message when the query returns nothing", () => {
    searchState.value = { data: [], isLoading: false, isError: false };
    renderResults();

    expect(screen.getByText(/No matches/i)).toBeInTheDocument();
  });

  it("shows the delayed Searching… indicator after ~150ms while loading with no data", () => {
    vi.useFakeTimers();
    try {
      searchState.value = { data: undefined, isLoading: true, isError: false };
      renderResults();

      // Before the delay elapses the loading indicator is not shown yet.
      expect(screen.queryByText("Searching…")).toBeNull();

      act(() => {
        vi.advanceTimersByTime(150);
      });

      expect(screen.getByText("Searching…")).toBeInTheDocument();
    } finally {
      vi.useRealTimers();
    }
  });
});
