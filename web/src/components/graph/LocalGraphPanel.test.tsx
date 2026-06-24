/**
 * GRAPH-03/04/05 — LocalGraphPanel chrome under jsdom (which cannot paint a
 * canvas). We mock react-force-graph-2d with a DOM stand-in (surfacing nodes +
 * click/hover handlers) and mock useLocalGraph to drive each state. Asserts:
 * the dock is collapsed by default (only a reopen affordance), opens to show the
 * title "Local graph" + EdgeToggles + DepthControl, renders the exact empty /
 * loading / error copy, the collapse toggle flips aria-pressed + state, a
 * page-node click navigates while a tag node does NOT, and that the DepthControl
 * value drives the useLocalGraph(path, depth) arg (depth re-keys the query).
 * Canvas pixels are out of scope (the plan's human_verification covers paint).
 */
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Routes, Route, useLocation } from "react-router-dom";

import type { GraphData } from "../../api/client";
import { useGraphEdges } from "../../stores/graphEdges";
import { useLocalGraphPanel } from "../../stores/localGraphPanel";

// Mock the canvas library: render nodes as buttons and wire click/hover so the
// chrome + navigation are assertable without a real canvas (ForceGraph2D throws
// under jsdom).
vi.mock("react-force-graph-2d", () => ({
  __esModule: true,
  default: (props: {
    graphData?: { nodes?: Array<{ id: string; label: string }> };
    onNodeClick?: (n: { id: string }) => void;
    onNodeHover?: (n: { id: string } | null) => void;
  }) => (
    <div data-testid="force-graph">
      {(props.graphData?.nodes ?? []).map((n) => (
        <button
          key={n.id}
          type="button"
          data-testid={`node-${n.id}`}
          onClick={() => props.onNodeClick?.({ id: n.id })}
          onMouseEnter={() => props.onNodeHover?.({ id: n.id })}
          onMouseLeave={() => props.onNodeHover?.(null)}
        >
          {n.label}
        </button>
      ))}
    </div>
  ),
}));

// Capture the (path, depth) args useLocalGraph is called with, and return a
// per-test result.
const useLocalGraphMock =
  vi.fn<(path: string, depth: number) => unknown>();
vi.mock("../../hooks/useGraph", () => ({
  useLocalGraph: (path: string, depth: number) =>
    useLocalGraphMock(path, depth),
}));

import LocalGraphPanel from "./LocalGraphPanel";

function LocationProbe() {
  const loc = useLocation();
  return <div data-testid="location">{loc.pathname}</div>;
}

const PATH = "notes/current.md";

function renderPanel() {
  return render(
    <MemoryRouter initialEntries={[`/app/page/${PATH}`]}>
      <LocalGraphPanel path={PATH} />
      <LocationProbe />
      <Routes>
        <Route path="/app/page/*" element={null} />
      </Routes>
    </MemoryRouter>,
  );
}

const POPULATED: GraphData = {
  nodes: [
    { id: PATH, label: "Current", type: "page" },
    { id: "notes/a.md", label: "Note A", type: "page" },
    { id: "tag:project", label: "project", type: "tag" },
  ],
  edges: [
    { source: PATH, target: "notes/a.md", type: "link" },
    { source: PATH, target: "tag:project", type: "tag" },
  ],
};

describe("LocalGraphPanel (GRAPH-03/04/05)", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    useGraphEdges.setState({ links: true, backlinks: true, sharedTags: false });
    useLocalGraphPanel.setState({ open: false, depth: 1 });
    useLocalGraphMock.mockReturnValue({
      data: POPULATED,
      isLoading: false,
      isError: false,
    });
  });

  it("is collapsed by default — only the reopen affordance shows", () => {
    renderPanel();
    expect(
      screen.getByRole("button", { name: "Show local graph" }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("heading", { name: "Local graph" }),
    ).not.toBeInTheDocument();
  });

  it("gates the fetch while collapsed (empty path → idle query)", () => {
    renderPanel();
    // Collapsed → the hook is called with an empty seed path so it never fetches.
    expect(useLocalGraphMock).toHaveBeenCalledWith("", 1);
  });

  it("opens to show the title, EdgeToggles, and DepthControl", async () => {
    const user = userEvent.setup();
    renderPanel();
    await user.click(screen.getByRole("button", { name: "Show local graph" }));

    expect(
      screen.getByRole("heading", { name: "Local graph" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Links" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Backlinks" })).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Shared tags" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("combobox", { name: "Depth" })).toBeInTheDocument();
  });

  it("collapse toggle flips aria-pressed and hides the dock", async () => {
    useLocalGraphPanel.setState({ open: true, depth: 1 });
    const user = userEvent.setup();
    renderPanel();

    const hide = screen.getByRole("button", { name: "Hide local graph" });
    expect(hide).toHaveAttribute("aria-pressed", "true");

    await user.click(hide);
    expect(
      screen.queryByRole("heading", { name: "Local graph" }),
    ).not.toBeInTheDocument();
    const show = screen.getByRole("button", { name: "Show local graph" });
    expect(show).toHaveAttribute("aria-pressed", "false");
  });

  it("calls useLocalGraph with the route path + slice depth when open", () => {
    useLocalGraphPanel.setState({ open: true, depth: 3 });
    renderPanel();
    expect(useLocalGraphMock).toHaveBeenCalledWith(PATH, 3);
  });

  it("re-keys the query when the depth changes (depth drives the hook arg)", async () => {
    useLocalGraphPanel.setState({ open: true, depth: 1 });
    const user = userEvent.setup();
    renderPanel();
    expect(useLocalGraphMock).toHaveBeenCalledWith(PATH, 1);

    await user.selectOptions(
      screen.getByRole("combobox", { name: "Depth" }),
      "2",
    );
    await waitFor(() =>
      expect(useLocalGraphMock).toHaveBeenCalledWith(PATH, 2),
    );
  });

  it("shows the quiet empty line when the page has no neighbors", () => {
    useLocalGraphPanel.setState({ open: true, depth: 1 });
    useLocalGraphMock.mockReturnValue({
      data: { nodes: [{ id: PATH, label: "Current", type: "page" }], edges: [] },
      isLoading: false,
      isError: false,
    });
    renderPanel();
    expect(screen.getByText("This page has no links yet")).toBeInTheDocument();
  });

  it("shows the quiet loading line while the query is in flight", () => {
    useLocalGraphPanel.setState({ open: true, depth: 1 });
    useLocalGraphMock.mockReturnValue({
      data: undefined,
      isLoading: true,
      isError: false,
    });
    renderPanel();
    expect(screen.getByText(/Loading local graph…/)).toBeInTheDocument();
  });

  it("shows the generic hidden-Git-safe error line on failure", () => {
    useLocalGraphPanel.setState({ open: true, depth: 1 });
    useLocalGraphMock.mockReturnValue({
      data: undefined,
      isLoading: false,
      isError: true,
    });
    renderPanel();
    expect(
      screen.getByText("Couldn't load the local graph. Refresh to try again."),
    ).toBeInTheDocument();
  });

  it("renders the canvas and navigates on a page-node click; tag node no-ops", async () => {
    useLocalGraphPanel.setState({ open: true, depth: 1 });
    const user = userEvent.setup();
    renderPanel();

    expect(screen.getByTestId("force-graph")).toBeInTheDocument();

    await user.click(screen.getByTestId("node-notes/a.md"));
    await waitFor(() =>
      expect(screen.getByTestId("location")).toHaveTextContent(
        "/app/page/notes/a.md",
      ),
    );

    // Tag node click does not navigate (stays on the last page route).
    await user.click(screen.getByTestId("node-tag:project"));
    expect(screen.getByTestId("location")).toHaveTextContent(
      "/app/page/notes/a.md",
    );
  });
});
