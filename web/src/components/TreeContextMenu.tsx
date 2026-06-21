import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type KeyboardEvent,
} from "react";
import "./TreeContextMenu.css";

// TreeContextMenuItem is a single actionable row. `danger` styles the item as
// destructive (e.g. Delete). `onSelect` runs when the item is chosen; the menu
// closes itself first.
export interface TreeContextMenuItem {
  label: string;
  onSelect: () => void;
  danger?: boolean;
}

export interface TreeContextMenuProps {
  // Viewport coordinates (clientX/clientY of the triggering right-click).
  x: number;
  y: number;
  items: TreeContextMenuItem[];
  onClose: () => void;
}

// VIEWPORT_MARGIN keeps the menu this many px clear of every viewport edge when
// clamped (Obsidian-style behaviour when right-clicking near a corner).
const VIEWPORT_MARGIN = 4;

// useViewportClamp positions the menu at (x, y), then — after the first paint —
// measures it and nudges it back on-screen so it never overflows the viewport.
function useViewportClamp(
  ref: React.RefObject<HTMLElement | null>,
  x: number,
  y: number,
) {
  const [pos, setPos] = useState({ x, y });
  useLayoutEffect(() => {
    const node = ref.current;
    if (!node) return;
    const rect = node.getBoundingClientRect();
    let nx = x;
    let ny = y;
    if (x + rect.width + VIEWPORT_MARGIN > window.innerWidth) {
      nx = Math.max(VIEWPORT_MARGIN, window.innerWidth - rect.width - VIEWPORT_MARGIN);
    }
    if (y + rect.height + VIEWPORT_MARGIN > window.innerHeight) {
      ny = Math.max(VIEWPORT_MARGIN, window.innerHeight - rect.height - VIEWPORT_MARGIN);
    }
    setPos({ x: nx, y: ny });
  }, [ref, x, y]);
  return pos;
}

// useFocusOnOpen remembers the previously focused element on mount, moves focus
// to the first menu item, and restores focus to the previous element on unmount.
function useFocusOnOpen(firstItem: React.RefObject<HTMLElement | null>) {
  useEffect(() => {
    const previouslyFocused = document.activeElement as HTMLElement | null;
    firstItem.current?.focus();
    return () => {
      previouslyFocused?.focus?.();
    };
  }, [firstItem]);
}

// useDismissOnOutside closes the menu on any outside interaction: outside-click,
// scroll (capture, so the scrolling nav rail counts), or resize.
function useDismissOnOutside(
  ref: React.RefObject<HTMLElement | null>,
  onClose: () => void,
) {
  useEffect(() => {
    function onDocPointer(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) onClose();
    }
    function onScrollOrResize() {
      onClose();
    }
    document.addEventListener("mousedown", onDocPointer);
    window.addEventListener("scroll", onScrollOrResize, true);
    window.addEventListener("resize", onScrollOrResize);
    return () => {
      document.removeEventListener("mousedown", onDocPointer);
      window.removeEventListener("scroll", onScrollOrResize, true);
      window.removeEventListener("resize", onScrollOrResize);
    };
  }, [ref, onClose]);
}

// TreeContextMenu is a reusable cursor-anchored popup menu for the file tree. It
// renders a passed list of items at (x, y), closes on outside-click / Escape /
// scroll / resize, and is fully keyboard-navigable: arrow keys move focus
// (wrapping), Home/End jump, Enter/Space select, Tab is trapped (wraps). Focus
// moves to the first item on open and is restored on close. Accessible:
// role="menu" / role="menuitem".
export default function TreeContextMenu({
  x,
  y,
  items,
  onClose,
}: TreeContextMenuProps) {
  const menuRef = useRef<HTMLDivElement>(null);
  const itemRefs = useRef<(HTMLButtonElement | null)[]>([]);
  const firstItemRef = useRef<HTMLButtonElement | null>(null);

  const pos = useViewportClamp(menuRef, x, y);
  useFocusOnOpen(firstItemRef);
  useDismissOnOutside(menuRef, onClose);

  // focusItem moves focus to the item at `index`, wrapping past either end.
  const focusItem = useCallback(
    (index: number) => {
      const count = items.length;
      if (count === 0) return;
      const next = ((index % count) + count) % count;
      itemRefs.current[next]?.focus();
    },
    [items.length],
  );

  const currentIndex = useCallback(
    () => itemRefs.current.findIndex((el) => el === document.activeElement),
    [],
  );

  function onKeyDown(e: KeyboardEvent) {
    switch (e.key) {
      case "Escape":
        e.preventDefault();
        onClose();
        break;
      case "ArrowDown":
        e.preventDefault();
        focusItem(currentIndex() + 1);
        break;
      case "ArrowUp":
        e.preventDefault();
        focusItem(currentIndex() - 1);
        break;
      case "Home":
        e.preventDefault();
        focusItem(0);
        break;
      case "End":
        e.preventDefault();
        focusItem(items.length - 1);
        break;
      case "Tab":
        // Trap focus inside the menu (wrap with arrow-key semantics).
        e.preventDefault();
        focusItem(currentIndex() + (e.shiftKey ? -1 : 1));
        break;
      default:
        break;
    }
  }

  function run(item: TreeContextMenuItem) {
    onClose();
    item.onSelect();
  }

  return (
    <div
      ref={menuRef}
      className="treemenu"
      role="menu"
      aria-orientation="vertical"
      style={{ top: pos.y, left: pos.x }}
      onKeyDown={onKeyDown}
    >
      {items.map((item, i) => (
        <button
          key={item.label}
          type="button"
          role="menuitem"
          tabIndex={i === 0 ? 0 : -1}
          ref={(el) => {
            itemRefs.current[i] = el;
            if (i === 0) firstItemRef.current = el;
          }}
          className={`treemenu-item${
            item.danger ? " treemenu-item-danger" : ""
          }`}
          onClick={() => run(item)}
        >
          {item.label}
        </button>
      ))}
    </div>
  );
}
