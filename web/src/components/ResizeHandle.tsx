import {
  useRef,
  type KeyboardEvent as ReactKeyboardEvent,
  type PointerEvent as ReactPointerEvent,
} from "react";

import "./ResizeHandle.css";

// ResizeHandle is a thin vertical drag bar placed between two flex columns. It
// reports the horizontal drag delta (px since the last tick) via onResize; the
// caller applies it to the adjacent column's width. Pointer capture keeps the
// drag alive while the cursor leaves the 6px bar. Keyboard-resizable for a11y
// (Arrow keys nudge by STEP).
const STEP = 16;

export interface ResizeHandleProps {
  // onResize receives the signed px delta for one drag tick / key press.
  onResize: (deltaX: number) => void;
  // ariaLabel describes what is being resized ("Resize sidebar").
  ariaLabel: string;
}

export default function ResizeHandle({ onResize, ariaLabel }: ResizeHandleProps) {
  // lastX holds the previous pointer X so each move reports an incremental delta
  // (null when not dragging).
  const lastX = useRef<number | null>(null);

  function onPointerDown(e: ReactPointerEvent<HTMLDivElement>) {
    e.preventDefault();
    lastX.current = e.clientX;
    e.currentTarget.setPointerCapture(e.pointerId);
  }

  function onPointerMove(e: ReactPointerEvent<HTMLDivElement>) {
    if (lastX.current === null) return;
    const dx = e.clientX - lastX.current;
    lastX.current = e.clientX;
    if (dx !== 0) onResize(dx);
  }

  function endDrag(e: ReactPointerEvent<HTMLDivElement>) {
    lastX.current = null;
    if (e.currentTarget.hasPointerCapture(e.pointerId)) {
      e.currentTarget.releasePointerCapture(e.pointerId);
    }
  }

  function onKeyDown(e: ReactKeyboardEvent<HTMLDivElement>) {
    if (e.key === "ArrowLeft") {
      e.preventDefault();
      onResize(-STEP);
    } else if (e.key === "ArrowRight") {
      e.preventDefault();
      onResize(STEP);
    }
  }

  return (
    <div
      className="resize-handle"
      role="separator"
      aria-orientation="vertical"
      aria-label={ariaLabel}
      tabIndex={0}
      onPointerDown={onPointerDown}
      onPointerMove={onPointerMove}
      onPointerUp={endDrag}
      onPointerCancel={endDrag}
      onKeyDown={onKeyDown}
    >
      <span className="resize-handle-grip" aria-hidden="true" />
    </div>
  );
}
