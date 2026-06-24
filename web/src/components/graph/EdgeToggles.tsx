import { useGraphEdges, type EdgeKind } from "../../stores/graphEdges";
import "./EdgeToggles.css";

// EdgeToggles is the Links / Backlinks / Shared tags chip cluster (GRAPH-04).
// The three edge-type toggles are per-device CLIENT state held in the graphEdges
// zustand slice (never a backend concern): GraphView filters the payload edges
// with filterEdges + useMemo over these booleans and NEVER refetches on a toggle.
//
// Each chip is an accessible <button type="button"> whose aria-pressed reflects
// the toggle boolean; clicking calls toggle(kind). Styling clones the
// .agentpanel-suggestion ghost chip (ON = --color-accent-soft bg +
// --color-accent-border; OFF = --color-border) — token-only, no hard-coded color.
// Shared tags reflects the slice default (OFF) so the first view is not a hairball.

interface ToggleSpec {
  kind: EdgeKind;
  label: string;
}

// Order + exact labels per the UI-SPEC Copywriting Contract.
const TOGGLES: ToggleSpec[] = [
  { kind: "links", label: "Links" },
  { kind: "backlinks", label: "Backlinks" },
  { kind: "sharedTags", label: "Shared tags" },
];

export default function EdgeToggles() {
  const links = useGraphEdges((s) => s.links);
  const backlinks = useGraphEdges((s) => s.backlinks);
  const sharedTags = useGraphEdges((s) => s.sharedTags);
  const toggle = useGraphEdges((s) => s.toggle);

  const value: Record<EdgeKind, boolean> = { links, backlinks, sharedTags };

  return (
    <div className="edgetoggles" role="group" aria-label="Edge types">
      {TOGGLES.map(({ kind, label }) => {
        const on = value[kind];
        return (
          <button
            key={kind}
            type="button"
            className={`edgetoggle-chip${on ? " edgetoggle-chip-on" : ""}`}
            aria-pressed={on}
            onClick={() => toggle(kind)}
          >
            {label}
          </button>
        );
      })}
    </div>
  );
}
