import { useCallback, useMemo, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import { Loader2 } from "lucide-react";

import { useGraph } from "../../hooks/useGraph";
import { useGraphEdges } from "../../stores/graphEdges";
import { useGraphFilters } from "../../stores/graphFilters";
import { computeDegrees, isOrphan } from "../../lib/graph/model";
import { filterEdges, filterNodes, applyOrphanFilter } from "../../lib/graph/filter";
import type { NodeFilters } from "../../lib/graph/filter";
import { edgeKey, neighborHighlight } from "../../lib/graph/highlight";
import type { GraphEdge, GraphNode } from "../../api/client";
import GraphCanvas, {
  type GraphCanvasLink,
  type GraphCanvasNode,
} from "./GraphCanvas";
import EdgeToggles from "./EdgeToggles";
import FilterControls from "./FilterControls";
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
// ("dim the rest" — GRAPH-05). MIN_R/MAX_R bound the degree→radius scale,
// kept small for an Obsidian-style dot-graph look. GLOW_SPREAD is the number
// of extra pixels the outer bloom ring extends beyond the filled node.
const DIM_ALPHA = 0.12;
const MIN_R = 2.5;
const MAX_R = 8;
const GLOW_SPREAD = 4;
// labelZoomThreshold computes a zoom cutoff for label visibility from the
// textFadeThreshold slider (0–1). At 0 labels always appear, at 0.5 they
// appear at ~2.5x zoom, at 1 they only appear at ~4.5x zoom.
function labelZoomThreshold(v: number): number {
  return 0.5 + v * 4;
}

export default function GraphView() {
  const navigate = useNavigate();
  const { data, isLoading, isError } = useGraph();

  const links = useGraphEdges((s) => s.links);
  const backlinks = useGraphEdges((s) => s.backlinks);
  const sharedTags = useGraphEdges((s) => s.sharedTags);

  // Filter state (GF-01).
  const searchQuery = useGraphFilters((s) => s.searchQuery);
  const showTags = useGraphFilters((s) => s.showTags);
  const showAttachments = useGraphFilters((s) => s.showAttachments);
  const existingFilesOnly = useGraphFilters((s) => s.existingFilesOnly);
  const showOrphans = useGraphFilters((s) => s.showOrphans);
  const showArrows = useGraphFilters((s) => s.showArrows);
  const textFadeThreshold = useGraphFilters((s) => s.textFadeThreshold);
  const filterNodeSize = useGraphFilters((s) => s.nodeSize);
  const filterLinkThickness = useGraphFilters((s) => s.linkThickness);

  const [hoverId, setHoverId] = useState<string | null>(null);
  // Keep the latest hover in a ref so the draw callbacks (stable identities) read
  // the current value without being re-created each hover (avoids canvas churn).
  const hoverRef = useRef<string | null>(null);
  hoverRef.current = hoverId;

  // --- GF-01 filtering pipeline: node filter → edge filter → orphan filter ---

  // 1. Node visibility filter (search + toggles).
  const nodeFilters: NodeFilters = useMemo(
    () => ({ searchQuery, showTags, showAttachments, existingFilesOnly }),
    [searchQuery, showTags, showAttachments, existingFilesOnly],
  );

  const visibleNodes = useMemo(() => {
    if (!data) return [];
    return filterNodes(data.nodes, nodeFilters);
  }, [data, nodeFilters]);

  // 2. Edge filter over the toggle slice, keeping only edges whose both
  // endpoints survived the node filter.
  const visibleNodeIds = useMemo(
    () => new Set(visibleNodes.map((n) => n.id)),
    [visibleNodes],
  );

  const visibleEdges: GraphEdge[] = useMemo(() => {
    if (!data) return [];
    const edges = filterEdges(data.edges, { links, backlinks, sharedTags });
    return edges.filter(
      (e) => visibleNodeIds.has(e.source) && visibleNodeIds.has(e.target),
    );
  }, [data, links, backlinks, sharedTags, visibleNodeIds]);

  // 3. Orphan filter — when orphans hidden, drop nodes with 0 visible edges.
  const orphanFiltered = useMemo(
    () => applyOrphanFilter(visibleNodes, visibleEdges, showOrphans),
    [visibleNodes, visibleEdges, showOrphans],
  );

  // Degree map over the ORPHAN-FILTERED edges so node sizing reflects what is
  // shown (an always-hidden orphan has degree 0 no matter what the payload says).
  const degrees = useMemo(() => {
    if (!data) return new Map<string, number>();
    return computeDegrees(orphanFiltered.nodes, orphanFiltered.edges);
  }, [data, orphanFiltered]);

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

  // The lib-shaped data: uses orphan-filtered nodes/edges.
  const canvasData = useMemo(() => {
    if (!data) return { nodes: [], links: [] };
    return {
      nodes: orphanFiltered.nodes.map((n) => ({
        id: n.id,
        label: n.label,
        type: n.type,
      })),
      links: orphanFiltered.edges.map((e) => ({
        source: e.source,
        target: e.target,
        type: e.type,
      })),
    };
  }, [data, orphanFiltered]);

  const radiusFor = useCallback(
    (id: string): number => {
      if (maxDegree <= 0) return MIN_R;
      const d = degrees.get(id) ?? 0;
      return MIN_R + (MAX_R - MIN_R) * (d / maxDegree);
    },
    [degrees, maxDegree],
  );

  // The hover highlight set, recomputed only when hover or the RENDERED edges
  // change (neighborHighlight is pure — GRAPH-05). Uses orphanFiltered.edges
  // so the highlight set keys always match what the canvas draws.
  const highlight = useMemo(
    () => neighborHighlight(hoverId, orphanFiltered.edges),
    [hoverId, orphanFiltered.edges],
  );

  // Draw a single node Obsidian-style: small colored dot with an outer glow ring
  // (the bloom signature of the dark-theme graph). Orphans use the same hue at low
  // alpha + a subtle outline so they read as distinct. Tags stay faint diamonds.
  // Dim everything outside the hover set on pointer-in.
  const nodeCanvasObject = useCallback(
    (
      node: GraphCanvasNode,
      ctx: CanvasRenderingContext2D,
      globalScale: number,
    ) => {
      const id = nodeId(node);
      const meta = nodeMeta.get(id);
      const type = meta?.type ?? "page";
      // Scale visual radius in lockstep with nodeRelSize so the slider
      // perceptibly grows/shrinks nodes AND the force simulation stays
      // consistent. At default filterNodeSize=0.2 the scale is 1.0.
      const visualScale = (2 + filterNodeSize * 8) / 3.6;
      const r = radiusFor(id) * visualScale;

      const nodeColor = readVar("--graph-node-color", "#4cc9a0");
      const glowColor = readVar("--graph-node-glow", "rgba(76,201,160,0.18)");
      const faint = readVar("--color-faint", "#626b7d");
      const borderSubtle = readVar("--color-border", "rgba(255,255,255,0.06)");
      const text = readVar("--color-text", "#eef1f7");

      const orphan = isOrphan(id, degrees, type);
      const hovered = hoverRef.current;
      const dim = hovered != null && !highlight.nodes.has(id);

      // Step 1: outer glow ring (the Obsidian bloom). Only for page nodes at
      // sufficient size; skips tags and min-radius orphans so the glow doesn't
      // overwhelm the canvas.
      if (type !== "tag" && r > (MIN_R * visualScale) + 0.5) {
        ctx.beginPath();
        ctx.arc(node.x ?? 0, node.y ?? 0, r + GLOW_SPREAD / globalScale, 0, 2 * Math.PI);
        ctx.fillStyle = dim ? withAlpha(glowColor, DIM_ALPHA * 2) : glowColor;
        ctx.fill();
      }

      // Step 2: solid node dot. Page nodes use the teal --graph-node-color;
      // orphans are the same hue at ~30% alpha; tags stay faint diamonds.
      let fill: string;
      if (type === "tag") {
        fill = faint;
      } else if (orphan) {
        fill = withAlpha(nodeColor, 0.3);
      } else {
        fill = nodeColor;
      }
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

      // Step 3: orphan outline so a lone page reads at min radius.
      if (orphan && !dim) {
        ctx.lineWidth = 0.8 / globalScale;
        ctx.strokeStyle = borderSubtle;
        ctx.stroke();
      }

      // Step 4: labels above the zoom threshold (driven by textFadeThreshold),
      // or always on hovered+neighbors.
      const inHover = hovered != null && highlight.nodes.has(id);
      const showLabel = globalScale > labelZoomThreshold(textFadeThreshold) || inHover;
      if (showLabel && meta) {
        const fontPx = 11 / globalScale;
        ctx.font = `${fontPx}px var(--font-family-ui), system-ui, sans-serif`;
        ctx.textAlign = "center";
        ctx.textBaseline = "top";
        ctx.fillStyle = dim ? withAlpha(text, DIM_ALPHA * 2) : text;
        ctx.fillText(meta.label, node.x ?? 0, (node.y ?? 0) + r + 1.5 / globalScale);
      }
    },
    [nodeMeta, radiusFor, degrees, highlight, textFadeThreshold, filterNodeSize],
  );

  // Edge color by type: link edges are blue (references), tag edges are green,
  // both subtle at rest and brightening on hover.
  const linkColor = useCallback(
    (link: GraphCanvasLink): string => {
      const edgeType = (link as { type?: "link" | "tag" }).type ?? "link";
      const isTag = edgeType === "tag";
      const base = isTag
        ? readVar("--graph-edge-tag", "#22c55e")
        : readVar("--graph-edge-backlink", "#60a5fa");
      const hovered = hoverRef.current;
      if (hovered == null) return base;
      const key = edgeKey({
        source: endId((link as { source?: unknown }).source),
        target: endId((link as { target?: unknown }).target),
        type: edgeType,
      });
      if (highlight.edges.has(key)) return isTag
        ? readVar("--graph-edge-tag", "#22c55e")
        : readVar("--graph-edge-backlink", "#60a5fa");
      return withAlpha(base, DIM_ALPHA);
    },
    [highlight],
  );

  const linkWidth = useCallback(
    (link: GraphCanvasLink): number => {
      // Base width from the thickness slider: maps 0→0.5, 0.2→1.4 (default),
      // 1→5.0. Multiplied by highlight state.
      const base = 0.5 + filterLinkThickness * 4.5;
      const hovered = hoverRef.current;
      if (hovered == null) return base;
      const key = edgeKey({
        source: endId((link as { source?: unknown }).source),
        target: endId((link as { target?: unknown }).target),
        type: (link as { type?: "link" | "tag" }).type ?? "link",
      });
      return highlight.edges.has(key) ? base * 3 : base;
    },
    [highlight, filterLinkThickness],
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
        <FilterControls />
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
            nodeRelSize={2 + filterNodeSize * 8}
            linkDirectionalArrowLength={showArrows ? 4 : 0}
          />
        )}
      </div>
    </section>
  );
}
