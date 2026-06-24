import { useCallback, useMemo, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import { Loader2, PanelRightClose, PanelRightOpen } from "lucide-react";

import { useLocalGraph } from "../../hooks/useGraph";
import { useLocalGraphPanel } from "../../stores/localGraphPanel";
import { useGraphEdges } from "../../stores/graphEdges";
import { computeDegrees, isOrphan } from "../../lib/graph/model";
import { filterEdges } from "../../lib/graph/filter";
import { edgeKey, neighborHighlight } from "../../lib/graph/highlight";
import type { GraphEdge, GraphNode } from "../../api/client";
import GraphCanvas, {
  type GraphCanvasLink,
  type GraphCanvasNode,
} from "./GraphCanvas";
import EdgeToggles from "./EdgeToggles";
import DepthControl from "./DepthControl";
import "./LocalGraphPanel.css";

// LocalGraphPanel is the right-side collapsible per-page 'Local graph' dock
// (GRAPH-03/04/05). It clones the AgentPanel dock chrome (fixed-width column, left
// border, header + body, a collapse icon button) and hosts the SAME GraphCanvas
// the global GraphView uses, fed from useLocalGraph(path, depth): the current page
// plus its direct neighbors to `depth` hops. The query is keyed on [path, depth]
// (see useLocalGraph) so it auto-updates when the active page route changes OR the
// DepthControl changes — no manual invalidation (GRAPH-03 auto-update + depth).
//
// The open/collapse + depth live in the persisted localGraphPanel slice, so the
// dock stays collapsed by default (it never crowds the editor) and a reader who
// never opens it pays no canvas cost — the hook is gated to an empty path while
// collapsed, so /graph/local is not fetched until the panel is revealed.
//
// Edge filtering + hover-highlight reuse the SAME 10-01 helpers + the SHARED
// graphEdges slice as GraphView, so the global and local views stay in lock-step
// (toggling Links/Backlinks/Shared tags never refetches — GRAPH-04). The current
// (seed) page node is drawn in --color-accent (GRAPH-02); tag nodes are faint
// diamonds; orphans dim + outlined — identical visual language to the global view.
//
// Stored-XSS guard (T-10-06): node labels reach the screen ONLY as canvas text
// drawn by nodeCanvasObject — never via dangerouslySetInnerHTML.

// readVar resolves a CSS custom property off :root, with a fallback for jsdom.
function readVar(name: string, fallback: string): string {
  if (typeof window === "undefined" || !window.getComputedStyle) return fallback;
  const v = getComputedStyle(document.documentElement)
    .getPropertyValue(name)
    .trim();
  return v || fallback;
}

// withAlpha multiplies the alpha of a color for the dim treatment. Handles
// hex (#rrggbb) and rgb/rgba(); anything else is returned unchanged.
function withAlpha(color: string, alpha: number): string {
  const hex = color.match(/^#([0-9a-f]{6})$/i);
  if (hex) {
    const n = parseInt(hex[1], 16);
    const r = (n >> 16) & 255;
    const g = (n >> 8) & 255;
    const b = n & 255;
    return `rgba(${r}, ${g}, ${b}, ${alpha})`;
  }
  const rgb = color.match(/^rgba?\(([^)]+)\)$/i);
  if (rgb) {
    const parts = rgb[1].split(",").map((p) => p.trim());
    const [r, g, b] = parts;
    return `rgba(${r}, ${g}, ${b}, ${alpha})`;
  }
  return color;
}

function nodeId(n: GraphCanvasNode): string {
  return String((n as { id?: unknown }).id ?? "");
}

// endId resolves a link endpoint, which the lib mutates from a string id into a
// node ref after the first simulation tick.
function endId(end: unknown): string {
  if (end == null) return "";
  if (typeof end === "object") return String((end as { id?: unknown }).id ?? "");
  return String(end);
}

const DIM_ALPHA = 0.18;
const MIN_R = 3;
const MAX_R = 12;

interface LocalGraphPanelProps {
  // path is the active page route param (PageView's `*`). It becomes the seed of
  // the local neighborhood AND the accent-coloured node.
  path: string;
}

export default function LocalGraphPanel({ path }: LocalGraphPanelProps) {
  const navigate = useNavigate();

  const open = useLocalGraphPanel((s) => s.open);
  const toggle = useLocalGraphPanel((s) => s.toggle);
  const depth = useLocalGraphPanel((s) => s.depth);

  const links = useGraphEdges((s) => s.links);
  const backlinks = useGraphEdges((s) => s.backlinks);
  const sharedTags = useGraphEdges((s) => s.sharedTags);

  // Gate the fetch on open: while collapsed we pass an empty path so the hook's
  // `enabled: path !== ""` keeps the query idle (a reader who never opens the
  // panel makes no /graph/local request — no canvas cost). When open, the real
  // path drives the keyed query, which auto-updates on path/depth change.
  const queryPath = open ? path : "";
  const { data, isLoading, isError } = useLocalGraph(queryPath, depth);

  const [hoverId, setHoverId] = useState<string | null>(null);
  const hoverRef = useRef<string | null>(null);
  hoverRef.current = hoverId;

  // Filter the payload edges client-side over the shared toggle slice (GRAPH-04).
  const visibleEdges: GraphEdge[] = useMemo(() => {
    if (!data) return [];
    return filterEdges(data.edges, { links, backlinks, sharedTags });
  }, [data, links, backlinks, sharedTags]);

  const degrees = useMemo(() => {
    if (!data) return new Map<string, number>();
    return computeDegrees(data.nodes, visibleEdges);
  }, [data, visibleEdges]);

  const maxDegree = useMemo(() => {
    let m = 0;
    for (const v of degrees.values()) m = Math.max(m, v);
    return m;
  }, [degrees]);

  const nodeMeta = useMemo(() => {
    const map = new Map<string, GraphNode>();
    if (data) for (const n of data.nodes) map.set(n.id, n);
    return map;
  }, [data]);

  const canvasData = useMemo(() => {
    if (!data) return { nodes: [], links: [] };
    return {
      nodes: data.nodes.map((n) => ({ id: n.id, label: n.label, type: n.type })),
      links: visibleEdges.map((e) => ({
        source: e.source,
        target: e.target,
        type: e.type,
      })),
    };
  }, [data, visibleEdges]);

  const radiusFor = useCallback(
    (id: string): number => {
      if (maxDegree <= 0) return MIN_R;
      const d = degrees.get(id) ?? 0;
      return MIN_R + (MAX_R - MIN_R) * (d / maxDegree);
    },
    [degrees, maxDegree],
  );

  const highlight = useMemo(
    () => neighborHighlight(hoverId, visibleEdges),
    [hoverId, visibleEdges],
  );

  // Draw a node: degree-sized; orphan = dimmer fill + outline; the seed/active
  // page = accent (GRAPH-02); tag = faint diamond. Dim everything outside the
  // hover set (GRAPH-05). Identical to GraphView, with the seed path accented.
  const nodeCanvasObject = useCallback(
    (
      node: GraphCanvasNode,
      ctx: CanvasRenderingContext2D,
      globalScale: number,
    ) => {
      const id = nodeId(node);
      const meta = nodeMeta.get(id);
      const type = meta?.type ?? "page";
      const r = radiusFor(id);

      const accent = readVar("--color-accent", "#8b5cf6");
      const muted = readVar("--color-text-muted", "#8d96a8");
      const orphanColor = readVar("--graph-node-orphan", "rgba(141,150,168,0.45)");
      const faint = readVar("--color-faint", "#626b7d");
      const borderStrong = readVar("--color-border-strong", "rgba(255,255,255,0.13)");
      const text = readVar("--color-text", "#eef1f7");

      // The seed (current page) is the single accented node in the local view.
      const isActive = id === path;
      const orphan = isOrphan(id, degrees, type);

      let fill = muted;
      if (type === "tag") fill = faint;
      else if (isActive) fill = accent;
      else if (orphan) fill = orphanColor;

      const hovered = hoverRef.current;
      const dim = hovered != null && !highlight.nodes.has(id);
      if (dim) fill = withAlpha(fill, DIM_ALPHA);

      ctx.beginPath();
      if (type === "tag") {
        ctx.moveTo(node.x ?? 0, (node.y ?? 0) - r);
        ctx.lineTo((node.x ?? 0) + r, node.y ?? 0);
        ctx.lineTo(node.x ?? 0, (node.y ?? 0) + r);
        ctx.lineTo((node.x ?? 0) - r, node.y ?? 0);
        ctx.closePath();
      } else {
        ctx.arc(node.x ?? 0, node.y ?? 0, r, 0, 2 * Math.PI);
      }
      ctx.fillStyle = fill;
      ctx.fill();

      if (orphan && !dim) {
        ctx.lineWidth = 1 / globalScale;
        ctx.strokeStyle = borderStrong;
        ctx.stroke();
      }

      const inHover = hovered != null && highlight.nodes.has(id);
      const showLabel = globalScale > 1.6 || inHover || isActive;
      if (showLabel && meta) {
        const fontPx = 12 / globalScale;
        ctx.font = `${fontPx}px var(--font-family-ui), system-ui, sans-serif`;
        ctx.textAlign = "center";
        ctx.textBaseline = "top";
        ctx.fillStyle = dim ? withAlpha(text, DIM_ALPHA) : text;
        // Label is canvas text only (stored-XSS guard) — never an HTML sink.
        ctx.fillText(meta.label, node.x ?? 0, (node.y ?? 0) + r + 1 / globalScale);
      }
    },
    [nodeMeta, radiusFor, degrees, path, highlight],
  );

  const linkColor = useCallback(
    (link: GraphCanvasLink): string => {
      const base = readVar("--color-border-strong", "rgba(255,255,255,0.13)");
      const accent2 = readVar("--color-accent-2", "#a78bfa");
      const hovered = hoverRef.current;
      if (hovered == null) return base;
      const key = edgeKey({
        source: endId((link as { source?: unknown }).source),
        target: endId((link as { target?: unknown }).target),
        type: (link as { type?: "link" | "tag" }).type ?? "link",
      });
      if (highlight.edges.has(key)) return accent2;
      return withAlpha(base, DIM_ALPHA);
    },
    [highlight],
  );

  const linkWidth = useCallback(
    (link: GraphCanvasLink): number => {
      const hovered = hoverRef.current;
      if (hovered == null) return 1;
      const key = edgeKey({
        source: endId((link as { source?: unknown }).source),
        target: endId((link as { target?: unknown }).target),
        type: (link as { type?: "link" | "tag" }).type ?? "link",
      });
      return highlight.edges.has(key) ? 2 : 1;
    },
    [highlight],
  );

  // Click a PAGE node → open its page (reuse the BacklinksPanel/GraphView seam).
  // Tag nodes are non-navigable (no-op).
  const onNodeClick = useCallback(
    (node: GraphCanvasNode) => {
      const id = nodeId(node);
      const meta = nodeMeta.get(id);
      if (meta?.type === "tag") return;
      if (id) navigate(`/app/page/${id}`);
    },
    [navigate, nodeMeta],
  );

  const onNodeHover = useCallback((node: GraphCanvasNode | null) => {
    setHoverId(node ? nodeId(node) : null);
  }, []);

  // Collapsed: render only the slim reopen affordance (a vertical tab on the
  // right edge), mirroring the AgentPanel `if (!open) return null` discipline but
  // keeping a way back in (the topbar Assistant toggle is AppShell's; PageView has
  // no topbar entry for this dock, so the tab is the reopen control).
  if (!open) {
    return (
      <button
        type="button"
        className="btn btn-ghost localgraph-reopen"
        aria-label="Show local graph"
        aria-pressed={false}
        title="Show local graph"
        onClick={toggle}
      >
        <PanelRightOpen size={16} aria-hidden="true" />
      </button>
    );
  }

  // The seed-only (no neighbors) empty state: one page node and no edges. We treat
  // a single-node payload (or an explicitly empty one) as "no links yet".
  const isEmpty =
    !!data &&
    !isLoading &&
    !isError &&
    (data.nodes.length === 0 || data.edges.length === 0);

  return (
    <aside className="localgraph" aria-label="Local graph">
      <header className="localgraph-header">
        <div className="localgraph-heading">
          <h2 className="localgraph-title">Local graph</h2>
        </div>
        <button
          type="button"
          className="btn btn-ghost localgraph-collapse"
          aria-label="Hide local graph"
          aria-pressed={true}
          onClick={toggle}
        >
          <PanelRightClose size={16} aria-hidden="true" />
        </button>
      </header>

      <div className="localgraph-controls">
        <EdgeToggles />
        <DepthControl />
      </div>

      <div className="localgraph-body">
        {isLoading && (
          <p className="localgraph-status" role="status">
            <Loader2 size={14} aria-hidden="true" className="spinner" /> Loading
            local graph…
          </p>
        )}
        {isError && !isLoading && (
          <p className="localgraph-status" role="alert">
            Couldn't load the local graph. Refresh to try again.
          </p>
        )}
        {isEmpty && (
          <p className="localgraph-status">This page has no links yet</p>
        )}
        {!isLoading && !isError && data && !isEmpty && (
          <div className="localgraph-canvas">
            <GraphCanvas
              data={canvasData}
              onNodeClick={onNodeClick}
              onNodeHover={onNodeHover}
              nodeCanvasObject={nodeCanvasObject}
              linkColor={linkColor}
              linkWidth={linkWidth}
            />
          </div>
        )}
      </div>
    </aside>
  );
}
