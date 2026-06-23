import { useEffect, useState } from "react";
import { AlertTriangle, Loader2, Pencil, Users, WifiOff } from "lucide-react";

import {
  subscribePresence,
  type PresenceSnapshot,
  type PresenceState,
} from "../api/client";
import "./PresenceIndicator.css";

// PresenceIndicator is the quiet "who else is editing" line in the editor toolbar
// (COLL-01). It is the awareness twin of AutosaveStatus: a single muted
// aria-live="polite" span that announces presence changes without stealing focus.
// It subscribes to the per-page presence SSE stream on mount and unsubscribes on
// unmount, deriving "other editors" by filtering out your own session — it NEVER
// shows yourself. Presence is ambient, never an alert: muted in every state except
// the transient "Reconnecting…" SSE-drop, which is warning-tinted to signal that
// your awareness is degraded.
//
// States (UI-SPEC §PresenceIndicator States, copy verbatim):
//   none         → empty live span (no icon, no weight)
//   one other    → Pencil + "{name} is editing"
//   many         → Users + "{name} and {N} others are editing"
//   connecting   → Loader2 (.spinner) + "Connecting…"
//   reconnecting → AlertTriangle (warning) + "Reconnecting…"
//   disconnected → WifiOff + "Presence unavailable" (optional terminal)
export default function PresenceIndicator({
  path,
  conn,
}: {
  path: string;
  conn: string;
}) {
  // The latest full-state snapshot (null until the first frame arrives) and the
  // connection lifecycle. Both are reset whenever the page path changes (the
  // subscription is re-created), so a stale page's presence never leaks across a
  // navigation.
  const [snapshot, setSnapshot] = useState<PresenceSnapshot | null>(null);
  const [state, setState] = useState<PresenceState>("connecting");

  useEffect(() => {
    setSnapshot(null);
    setState("connecting");
    const unsubscribe = subscribePresence(path, conn, setSnapshot, setState);
    return unsubscribe;
  }, [path, conn]);

  // Other editors = live holders minus your own session (never show yourself).
  // Usernames render as plain React text below (auto-escaped) — never a raw-HTML
  // sink (T-05-13).
  const others = (snapshot?.editors ?? []).filter((e) => !e.you);

  // Before the first snapshot settles, show the connecting/reconnecting state so
  // the line is not silently blank during the initial connect or a drop.
  if (snapshot === null) {
    if (state === "reconnecting") {
      return (
        <span
          className="presence-indicator presence-warning"
          aria-live="polite"
          aria-label="Who else is editing"
        >
          <AlertTriangle size={14} aria-hidden="true" />
          Reconnecting…
        </span>
      );
    }
    return (
      <span
        className="presence-indicator presence-muted"
        aria-live="polite"
        aria-label="Who else is editing"
      >
        <Loader2 size={14} aria-hidden="true" className="spinner" />
        Connecting…
      </span>
    );
  }

  // Streaming but dropped: surface the degraded warning state over the last-known
  // snapshot (the EventSource auto-reconnects).
  if (state === "reconnecting") {
    return (
      <span
        className="presence-indicator presence-warning"
        aria-live="polite"
        aria-label="Who else is editing"
      >
        <AlertTriangle size={14} aria-hidden="true" />
        Reconnecting…
      </span>
    );
  }

  // Nobody else is editing: an empty live span occupies no visual weight (mirrors
  // AutosaveStatus idle) while still carrying the aria-live region.
  if (others.length === 0) {
    return (
      <span
        className="presence-indicator"
        aria-live="polite"
        aria-label="Who else is editing"
      />
    );
  }

  // One other editor: Pencil + "{name} is editing".
  if (others.length === 1) {
    return (
      <span
        className="presence-indicator presence-muted"
        aria-live="polite"
        aria-label="Who else is editing"
      >
        <Pencil size={14} aria-hidden="true" />
        {others[0].username} is editing
      </span>
    );
  }

  // Many editors: Users + "{name} and {N} others are editing" (N = others minus
  // the one named).
  return (
    <span
      className="presence-indicator presence-muted"
      aria-live="polite"
      aria-label="Who else is editing"
    >
      <Users size={14} aria-hidden="true" />
      {others[0].username} and {others.length - 1} others are editing
    </span>
  );
}

// PresenceDisconnected is the optional terminal "gave up / offline" state. It is
// exported for completeness (UI-SPEC §PresenceIndicator States) but the live SSE
// path above keeps reconnecting rather than ever rendering this, matching the
// non-blocking "editing still works" contract.
export function PresenceDisconnected() {
  return (
    <span
      className="presence-indicator presence-muted"
      aria-live="polite"
      aria-label="Who else is editing"
    >
      <WifiOff size={14} aria-hidden="true" />
      Presence unavailable
    </span>
  );
}
