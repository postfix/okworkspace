import { useEffect, useLayoutEffect, useRef, useState } from "react";
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

// TreeContextMenu is a reusable cursor-anchored popup menu for the file tree. It
// renders a passed list of items at (x, y), closes on outside-click / Escape /
// scroll / resize, and is fully keyboard-navigable: arrow keys move focus
// (wrapping), Home/End jump, Enter/Space select, Escape closes. Focus is trapped
// within the menu while open and restored to the previously focused element on
// close. Accessible: role="menu" / role="menuitem".
export default function TreeContextMenu({
  x,
  y,
  items,
  onClose,
}: TreeContextMenuProps) {
  const menuRef = useRef<HTMLDivElement>(null);
  const itemRefs = useRef<(HTMLButtonElement | null)[]>([]);
  const previouslyFocused = useRef<Element | null>(null);
  const [pos, setPos] = useState({ x, y });

  // Keep the menu fully on-screen: after the first paint, measure it and clamp
  // the position so it never overflows the viewport (Obsidian-style behaviour
  // when right-clicking near an edge).
  useLayoutEffect(() => {
    const node = menuRef.current;
    if (!node) return;
    const rect = node.getBoundingClientRect();
    const margin = 4;
    let nx = x;
    let ny = y;
    if (x + rect.width + margin > window.innerWidth) {
      nx = Math.max(margin, window.innerWidth - rect.width - margin);
    }
    if (y + rect.height + margin > window.innerHeight) {
      ny = Math.max(margin, window.innerHeight - rect.height - margin);
    }
    setPos({ x: nx, y: ny });
  }, [x, y]);

  // On open, remember focus and move it to the first item.
  useEffect(() => {
    previouslyFocused.current = document.activeElement;
    itemRefs.current[0]?.focus();
    return () => {
      (previouslyFocused.current as HTMLElement | null)?.focus?.();
    };
  }, []);

  // Close on any outside interaction: outside-click, scroll, resize.
  useEffect(() => {
    function onDocPointer(e: MouseEvent) {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
        onClose();
      }
    }
    function onScrollOrResize() {
      onClose();
    }
    document.addEventListener("mousedown", onDocPointer);
    // Capture scroll from any scroll container (the nav rail scrolls), not just
    // the window — hence the capture phase.
    window.addEventListener("scroll", onScrollOrResize, true);
    window.addEventListener("resize", onScrollOrResize);
    return () => {
      document.removeEventListener("mousedown", onDocPointer);
      window.removeEventListener("scroll", onScrollOrResize, true);
      window.removeEventListener("resize", onScrollOrResize);
    };
  }, [onClose]);

  function focusItem(index: number) {
    const count = items.length;
    if (count === 0) return;
    const next = ((index % count) + count) % count;
    itemRefs.current[next]?.focus();
  }

  function currentIndex(): number {
    return itemRefs.current.findIndex((el) => el === document.activeElement);
  }

  function onKeyDown(e: React.KeyboardEvent) {
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
          }}
          className={`treemenu-item${item.danger ? " treemenu-item-danger" : ""}`}
          onClick={() => run(item)}
        >
          {item.label}
        </button>
      ))}
    </div>
  );
}
