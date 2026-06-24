import { useEffect, useRef, useState } from "react";
import ForceGraph2D, {
  type ForceGraphMethods,
  type NodeObject,
  type LinkObject,
} from "react-force-graph-2d";

// GraphCanvas is a THIN, prop-driven wrapper around the imperative ForceGraph2D
// (Canvas) instance — the same ref-lifecycle discipline LivePreviewEditor applies
// to its CodeMirror EditorView. It owns the imperative graph handle in a ref,
// auto-sizes the canvas to its host box, reads draw colors from CSS custom
// properties (so tokens.css drives the canvas), and ALWAYS tears the simulation
// down on unmount so React 19 StrictMode's mount→unmount→remount never leaks a
// running force engine.
//
// It deliberately does NOT compute classification / edge-filtering / highlight
// itself — the parent (GraphView / LocalGraphPanel in Wave 2) feeds already-
// shaped data + draw callbacks built from the pure helpers (model/filter/
// highlight). That keeps the canvas a dumb renderer and the logic unit-tested.
//
// Stored-XSS guard (T-10-01): node labels are drawn by the parent's
// nodeCanvasObject as canvas text — never via dangerouslySetInnerHTML. We default
// nodeLabel off (no DOM tooltip) so an untrusted label can't reach an HTML sink.

// GraphCanvasNode/Link are the lib-shaped data the parent hands in. The lib uses
// `links` (not `edges`) and resolves `source`/`target` to node refs after the
// first tick, so we keep these permissive (index signature) — the parent owns the
// concrete payload→lib mapping.
export type GraphCanvasNode = NodeObject<{ [k: string]: unknown }>;
export type GraphCanvasLink = LinkObject<{ [k: string]: unknown }>;

export interface GraphCanvasData {
  nodes: GraphCanvasNode[];
  links: GraphCanvasLink[];
}

interface GraphCanvasProps {
  data: GraphCanvasData;
  // Interaction — the parent wires click→navigate and hover→highlight-set.
  onNodeClick?: (node: GraphCanvasNode) => void;
  onNodeHover?: (node: GraphCanvasNode | null) => void;
  // Draw callbacks (parent supplies, built from the pure helpers).
  nodeCanvasObject?: (
    node: GraphCanvasNode,
    ctx: CanvasRenderingContext2D,
    globalScale: number,
  ) => void;
  linkColor?: (link: GraphCanvasLink) => string;
  linkWidth?: (link: GraphCanvasLink) => number;
  // Force-tuning knobs with sensible Obsidian-like defaults (CONTEXT "Claude's
  // Discretion"). cooldownTicks lets the layout settle then idle (no perpetual
  // animation, GRAPH-01). Parents may override after a tuning pass.
  cooldownTicks?: number;
  nodeRelSize?: number;
  // backgroundCssVar names the CSS custom property to read for the canvas
  // backdrop (default --color-bg). Read via getComputedStyle so tokens drive it.
  backgroundCssVar?: string;
}

// readCssVar resolves a CSS custom property off :root (documentElement). Returns
// a fallback when unavailable (SSR/tests). Pure read — no DOM mutation.
function readCssVar(name: string, fallback: string): string {
  if (typeof window === "undefined" || !window.getComputedStyle) return fallback;
  const v = getComputedStyle(document.documentElement)
    .getPropertyValue(name)
    .trim();
  return v || fallback;
}

export default function GraphCanvas({
  data,
  onNodeClick,
  onNodeHover,
  nodeCanvasObject,
  linkColor,
  linkWidth,
  cooldownTicks = 120,
  nodeRelSize = 4,
  backgroundCssVar = "--color-bg",
}: GraphCanvasProps) {
  // Own the imperative ForceGraph2D handle (LivePreviewEditor precedent): the lib
  // forwards a methods object here; we hold it so cleanup can stop the engine.
  const fgRef = useRef<ForceGraphMethods | undefined>(undefined);
  const hostRef = useRef<HTMLDivElement>(null);
  const [size, setSize] = useState<{ w: number; h: number }>({ w: 0, h: 0 });

  // Auto-size the canvas to the host box. ForceGraph2D needs explicit width/height
  // (it does not flex), so we observe the host and feed measured pixels. Guarded
  // for environments without ResizeObserver (jsdom) so the component still mounts.
  useEffect(() => {
    const el = hostRef.current;
    if (!el) return;
    const measure = () =>
      setSize({ w: el.clientWidth, h: el.clientHeight });
    measure();
    if (typeof ResizeObserver === "undefined") return;
    const ro = new ResizeObserver(measure);
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  // ALWAYS stop the simulation on unmount. The ForceGraph2D React component
  // destroys its own kapsule internally, but we additionally pause the animation
  // through our owned ref so a StrictMode double-mount never leaves a ticking
  // engine attached to a detached canvas (mirrors LivePreviewEditor's v.destroy()
  // discipline). Nulling the ref drops our handle.
  useEffect(() => {
    return () => {
      try {
        fgRef.current?.pauseAnimation();
      } catch {
        // The instance may already be torn down by the lib's own unmount; ignore.
      }
      fgRef.current = undefined;
    };
  }, []);

  const background = readCssVar(backgroundCssVar, "#0b0d12");

  return (
    <div ref={hostRef} className="graphcanvas-host" style={{ width: "100%", height: "100%" }}>
      <ForceGraph2D
        ref={fgRef}
        graphData={data}
        width={size.w || undefined}
        height={size.h || undefined}
        backgroundColor={background}
        cooldownTicks={cooldownTicks}
        nodeRelSize={nodeRelSize}
        // No DOM tooltip: labels are drawn on the canvas by nodeCanvasObject
        // (stored-XSS guard — an untrusted label never reaches an HTML sink).
        nodeLabel={() => ""}
        nodeCanvasObject={nodeCanvasObject}
        linkColor={linkColor}
        linkWidth={linkWidth}
        onNodeClick={(n) => onNodeClick?.(n as GraphCanvasNode)}
        onNodeHover={(n) => onNodeHover?.((n as GraphCanvasNode) ?? null)}
      />
    </div>
  );
}
