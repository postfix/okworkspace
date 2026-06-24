/**
 * GRAPH-01/02/04/05 — GraphView chrome under jsdom (which cannot paint a canvas).
 * We mock react-force-graph-2d with a light DOM stand-in that surfaces the node
 * list + exposes the click/hover handlers, and mock useGraph to drive each state.
 * Asserts: the title "Graph" + the EdgeToggles cluster render; the empty / error /
 * loading copy is exact; a page-node click navigates to /app/page/<id> (via a
 * LocationProbe) while a tag node does NOT. Canvas pixels are out of scope (the
 * plan's human_verification covers the actual paint).
 */
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Routes, Route, useLocation } from "react-router-dom";

import type { GraphData } from "../../api/client";
import { useGraphEdges } from "../../stores/graphEdges";

// Mock the canvas library: render the nodes as buttons and wire the click/hover
// callbacks so the surrounding chrome + navigation are assertable without a real
// canvas (ForceGraph2D throws under jsdom).
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

// Mock useGraph so each test sets the query result directly.
const useGraphMock = vi.fn();
vi.mock("../../hooks/useGraph", () => ({
  useGraph: () => useGraphMock(),
}));

import GraphView from "./GraphView";

function LocationProbe() {
  const loc = useLocation();
  return <div data-testid="location">{loc.pathname}</div>;
}

function renderView() {
  return render(
    <MemoryRouter initialEntries={["/app/graph"]}>
      <GraphView />
      <LocationProbe />
      <Routes>
        <Route path="/app/page/*" element={null} />
        <Route path="/app/graph" element={null} />
      </Routes>
    </MemoryRouter>,
  );
}

const EMPTY: GraphData = { nodes: [], edges: [] };

describe("GraphView (GRAPH-01/02/04/05)", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    useGraphEdges.setState({ links: true, backlinks: true, sharedTags: false });
  });

  it("renders the title and the EdgeToggles cluster", () => {
    useGraphMock.mockReturnValue({ data: EMPTY, isLoading: false, isError: false });
    renderView();
    expect(screen.getByRole("heading", { name: "Graph" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Links" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Backlinks" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Shared tags" })).toBeInTheDocument();
  });

  it("shows the empty state when there are no nodes", () => {
    useGraphMock.mockReturnValue({ data: EMPTY, isLoading: false, isError: false });
    renderView();
    expect(screen.getByText("No pages to graph yet")).toBeInTheDocument();
    expect(
      screen.getByText(/Create and link a few pages/),
    ).toBeInTheDocument();
  });

  it("shows the quiet loading line while the query is in flight", () => {
    useGraphMock.mockReturnValue({
      data: undefined,
      isLoading: true,
      isError: false,
    });
    renderView();
    expect(screen.getByText(/Building the graph…/)).toBeInTheDocument();
  });

  it("shows the generic hidden-Git-safe error line on failure", () => {
    useGraphMock.mockReturnValue({
      data: undefined,
      isLoading: false,
      isError: true,
    });
    renderView();
    expect(
      screen.getByText("Couldn't load the graph. Refresh to try again."),
    ).toBeInTheDocument();
  });

  it("renders the canvas with nodes and navigates on a page-node click", async () => {
    useGraphMock.mockReturnValue({
      data: {
        nodes: [
          { id: "notes/a.md", label: "Note A", type: "page" },
          { id: "tag:project", label: "project", type: "tag" },
        ],
        edges: [{ source: "notes/a.md", target: "tag:project", type: "tag" }],
      } as GraphData,
      isLoading: false,
      isError: false,
    });
    const user = userEvent.setup();
    renderView();

    expect(screen.getByTestId("force-graph")).toBeInTheDocument();

    // Page node → navigates to /app/page/<id>.
    await user.click(screen.getByTestId("node-notes/a.md"));
    await waitFor(() =>
      expect(screen.getByTestId("location")).toHaveTextContent(
        "/app/page/notes/a.md",
      ),
    );
  });

  it("does NOT navigate when a tag node is clicked", async () => {
    useGraphMock.mockReturnValue({
      data: {
        nodes: [{ id: "tag:project", label: "project", type: "tag" }],
        edges: [],
      } as GraphData,
      isLoading: false,
      isError: false,
    });
    const user = userEvent.setup();
    renderView();

    await user.click(screen.getByTestId("node-tag:project"));
    // Stays on /app/graph (tag nodes are non-navigable).
    expect(screen.getByTestId("location")).toHaveTextContent("/app/graph");
  });
});
