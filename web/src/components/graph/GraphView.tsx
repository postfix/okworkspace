import { useCallback, useMemo, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import { Loader2 } from "lucide-react";

import { useGraph } from "../../hooks/useGraph";
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
import "./GraphView.css";

// GraphView is the full-pane /app/graph component (GRAPH-01/02/04/05): a header
// (title "Graph" + the EdgeToggles cluster) over the GraphCanvas. It fetches the
// global payload with useGraph (react-query — server state), filters edges with
// filterEdges + useMemo over the graphEdges zustand slice (client-only; toggling
// NEVER refetches), and feeds the canvas degree-sized / orphan-distinct nodes,
// an active-page accent, hover-neighbor highlighting, and click-to-open.
//
// jsdom cannot paint a canvas, so the draw callbacks are only exercised when the
// canvas actually mounts (human verification). The chrome (title, toggles, the
// empty/loading/error states) is the unit-tested surface.
//
// Stored-XSS guard (T-10-03): node labels reach the screen only as canvas text
// drawn here — never via dangerouslySetInnerHTML.

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

// nodeId resolves the id of a lib node (the lib keeps `id` on the node object).
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

// DIM_ALPHA is the low alpha applied to non-highlighted elements on hover
// ("dim the rest" — GRAPH-05). MIN_R/MAX_R bound the degree→radius scale.
const DIM_ALPHA = 0.18;
const MIN_R = 3;
const MAX_R = 12;

interface GraphViewProps {
  // activePath, when supplied (the local-graph use later), accents that one page
  // node. The global /app/graph view has no single active page → undefined.
  activePath?: string;
}

export default function GraphView({ activePath }: GraphViewProps) {
  const navigate = useNavigate();
  const { data, isLoading, isError } = useGraph();

  const links = useGraphEdges((s) => s.links);
  const backlinks = useGraphEdges((s) => s.backlinks);
  const sharedTags = useGraphEdges((s) => s.sharedTags);

  const [hoverId, setHoverId] = useState<string | null>(null);
  // Keep the latest hover in a ref so the draw callbacks (stable identities) read
  // the current value without being re-created each hover (avoids canvas churn).
  const hoverRef = useRef<string | null>(null);
  hoverRef.current = hoverId;

  // Filter the payload edges client-side over the toggle slice (GRAPH-04). This
  // useMemo is the ONLY edge transform — toggling never refetches.
  const visibleEdges: GraphEdge[] = useMemo(() => {
    if (!data) return [];
    return filterEdges(data.edges, { links, backlinks, sharedTags });
  }, [data, links, backlinks, sharedTags]);

  // Degree map over the VISIBLE edges so node sizing reflects what is shown.
  const degrees = useMemo(() => {
    if (!data) return new Map<string, number>();
    return computeDegrees(data.nodes, visibleEdges);
  }, [data, visibleEdges]);

  // The maximum degree drives the radius scale denominator.
  const maxDegree = useMemo(() => {
    let m = 0;
    for (const v of degrees.values()) m = Math.max(m, v);
    return m;
  }, [degrees]);

  // Index node type + label by id for the draw callbacks.
  const nodeMeta = useMemo(() => {
    const map = new Map<string, GraphNode>();
    if (data) for (const n of data.nodes) map.set(n.id, n);
    return map;
  }, [data]);

  // The lib-shaped data: it wants `links` (not `edges`) with source/target ids.
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

  // The hover highlight set, recomputed only when hover or the visible edges
  // change (neighborHighlight is pure — GRAPH-05).
  const highlight = useMemo(
    () => neighborHighlight(hoverId, visibleEdges),
    [hoverId, visibleEdges],
  );

  // Draw a single node: degree-sized; orphan = dimmer fill + outline; active page
  // = accent; tag = faint diamond. Dim everything outside the hover set.
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

      const isActive = activePath != null && id === activePath;
      const orphan = isOrphan(id, degrees, type);

      let fill = muted;
      if (type === "tag") fill = faint;
      else if (isActive) fill = accent;
      else if (orphan) fill = orphanColor;

      // Dim non-highlighted elements when hovering (GRAPH-05).
      const hovered = hoverRef.current;
      const dim = hovered != null && !highlight.nodes.has(id);
      if (dim) fill = withAlpha(fill, DIM_ALPHA);

      ctx.beginPath();
      if (type === "tag") {
        // Diamond for tag nodes (distinct shape, NOT a rainbow color).
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

      // Orphan outline so an unlinked page reads as distinct even at min radius.
      if (orphan && !dim) {
        ctx.lineWidth = 1 / globalScale;
        ctx.strokeStyle = borderStrong;
        ctx.stroke();
      }

      // Labels: above a zoom threshold, or whenever this node is in the hover
      // highlight set (so a hovered node + neighbors always read).
      const inHover = hovered != null && highlight.nodes.has(id);
      const showLabel = globalScale > 1.6 || inHover;
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
    [nodeMeta, radiusFor, degrees, activePath, highlight],
  );

  // Edge color: connecting edges of the hovered node stay bright (accent-2),
  // everything else dims when hovering, else the default border stroke.
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

  // Click a PAGE node → open its page (reuse the BacklinksPanel navigate seam).
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

  return (
    <section className="graphview" aria-label="Graph">
      <header className="graphview-header">
        <h1 className="graphview-title">Graph</h1>
        <EdgeToggles />
      </header>

      <div className="graphview-canvas">
        {isLoading && (
          <p className="graphview-status" role="status">
            <Loader2 size={14} aria-hidden="true" className="spinner" /> Building
            the graph…
          </p>
        )}
        {isError && !isLoading && (
          <p className="graphview-status" role="alert">
            Couldn't load the graph. Refresh to try again.
          </p>
        )}
        {!isLoading && !isError && data && data.nodes.length === 0 && (
          <div className="graphview-empty">
            <h2 className="graphview-empty-heading">No pages to graph yet</h2>
            <p className="graphview-empty-body">
              Create and link a few pages, then come back to see how your
              workspace connects.
            </p>
          </div>
        )}
        {!isLoading && !isError && data && data.nodes.length > 0 && (
          <GraphCanvas
            data={canvasData}
            onNodeClick={onNodeClick}
            onNodeHover={onNodeHover}
            nodeCanvasObject={nodeCanvasObject}
            linkColor={linkColor}
            linkWidth={linkWidth}
          />
        )}
      </div>
    </section>
  );
}
