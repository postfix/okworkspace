import { useEffect, useRef, useState } from "react";
import {
  MoreHorizontal,
  Pencil,
  Type,
  FolderInput,
  History,
  Trash2,
} from "lucide-react";
import "./PageActionMenu.css";

export interface PageActionMenuProps {
  // canEdit gates the mutating items (Rename/Move/Delete). When false, the menu
  // shows the read-only items and the mutating items are absent (RBAC: readers
  // view, editors mutate). UI-SPEC: readers see read-only.
  canEdit: boolean;
  onEdit: () => void;
  onRename: () => void;
  onMove: () => void;
  onHistory: () => void;
  onDelete: () => void;
}

// PageActionMenu is the read-mode action popover (UI-SPEC): Edit, Rename, Move,
// Version history, Delete. The trigger is an icon-only overflow button with the
// accessible name "Page actions". Mutating actions are RBAC-gated. Closes on
// outside click and Esc. NEVER surfaces Git vocabulary.
export default function PageActionMenu({
  canEdit,
  onEdit,
  onRename,
  onMove,
  onHistory,
  onDelete,
}: PageActionMenuProps) {
  const [open, setOpen] = useState(false);
  const rootRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    function onDocClick(e: MouseEvent) {
      if (rootRef.current && !rootRef.current.contains(e.target as Node)) {
        setOpen(false);
      }
    }
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") setOpen(false);
    }
    document.addEventListener("mousedown", onDocClick);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("mousedown", onDocClick);
      document.removeEventListener("keydown", onKey);
    };
  }, [open]);

  function run(action: () => void) {
    setOpen(false);
    action();
  }

  return (
    <div className="pageactions" ref={rootRef}>
      <button
        type="button"
        className="pageactions-trigger"
        aria-label="Page actions"
        aria-haspopup="menu"
        aria-expanded={open}
        onClick={() => setOpen((v) => !v)}
      >
        <MoreHorizontal size={18} aria-hidden="true" />
      </button>
      {open && (
        <div className="pageactions-popover" role="menu">
          <button
            type="button"
            className="pageactions-item"
            role="menuitem"
            onClick={() => run(onEdit)}
          >
            <Pencil size={16} aria-hidden="true" />
            <span>Edit</span>
          </button>
          {canEdit && (
            <button
              type="button"
              className="pageactions-item"
              role="menuitem"
              onClick={() => run(onRename)}
            >
              <Type size={16} aria-hidden="true" />
              <span>Rename</span>
            </button>
          )}
          {canEdit && (
            <button
              type="button"
              className="pageactions-item"
              role="menuitem"
              onClick={() => run(onMove)}
            >
              <FolderInput size={16} aria-hidden="true" />
              <span>Move</span>
            </button>
          )}
          <button
            type="button"
            className="pageactions-item"
            role="menuitem"
            onClick={() => run(onHistory)}
          >
            <History size={16} aria-hidden="true" />
            <span>Version history</span>
          </button>
          {canEdit && (
            <button
              type="button"
              className="pageactions-item pageactions-item-destructive"
              role="menuitem"
              onClick={() => run(onDelete)}
            >
              <Trash2 size={16} aria-hidden="true" />
              <span>Delete</span>
            </button>
          )}
        </div>
      )}
    </div>
  );
}
