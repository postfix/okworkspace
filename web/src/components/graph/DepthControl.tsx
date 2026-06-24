import { useId } from "react";

import {
  useLocalGraphPanel,
  DEPTH_MIN,
  DEPTH_MAX,
} from "../../stores/localGraphPanel";
import "./DepthControl.css";

// DepthControl is the Local-graph panel's hop-depth selector (GRAPH-03). It is a
// small labelled <select className="select"> offering 1 / 2 / 3 hops (default 1),
// bound to the localGraphPanel zustand slice. Changing it calls setDepth (which
// clamps to the endpoint range [1,3]); the chosen value is the single source for
// the useLocalGraph(path, depth) arg, so changing depth re-keys the react-query
// fetch and the neighborhood re-loads (no manual invalidation).
//
// Token-only CSS (the .select primitive + --hit-min-height target); no hard-coded
// color. Label copy is `Depth` per the UI-SPEC Copywriting Contract.

// OPTIONS is the integer hop range [DEPTH_MIN, DEPTH_MAX] rendered as <option>s.
const OPTIONS: number[] = Array.from(
  { length: DEPTH_MAX - DEPTH_MIN + 1 },
  (_, i) => DEPTH_MIN + i,
);

export default function DepthControl() {
  const depth = useLocalGraphPanel((s) => s.depth);
  const setDepth = useLocalGraphPanel((s) => s.setDepth);
  const id = useId();

  return (
    <div className="depthcontrol">
      <label className="depthcontrol-label" htmlFor={id}>
        Depth
      </label>
      <select
        id={id}
        className="select depthcontrol-select"
        value={depth}
        onChange={(e) => setDepth(Number(e.target.value))}
      >
        {OPTIONS.map((d) => (
          <option key={d} value={d}>
            {d}
          </option>
        ))}
      </select>
    </div>
  );
}
