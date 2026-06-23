import { useEffect, useRef, useState } from "react";
import { ChevronDown, LogOut, User } from "lucide-react";
import "./UserMenu.css";

export interface UserMenuProps {
  displayName: string;
  onProfile: () => void;
  onLogout: () => void;
}

// UserMenu is the top-bar popover (display name -> Profile, Log out). It makes
// logout reachable from any authenticated page (AUTH-02). Closes on outside
// click and on Esc.
export default function UserMenu({ displayName, onProfile, onLogout }: UserMenuProps) {
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

  return (
    <div className="usermenu" ref={rootRef}>
      <button
        type="button"
        className="usermenu-trigger"
        aria-haspopup="menu"
        aria-expanded={open}
        onClick={() => setOpen((v) => !v)}
      >
        <span className="usermenu-name">{displayName}</span>
        <ChevronDown size={16} aria-hidden="true" />
      </button>
      {open && (
        <div className="usermenu-popover" role="menu">
          <button
            type="button"
            className="usermenu-item"
            role="menuitem"
            onClick={() => {
              setOpen(false);
              onProfile();
            }}
          >
            <User size={16} aria-hidden="true" />
            <span>Profile</span>
          </button>
          <button
            type="button"
            className="usermenu-item"
            role="menuitem"
            onClick={() => {
              setOpen(false);
              onLogout();
            }}
          >
            <LogOut size={16} aria-hidden="true" />
            <span>Log out</span>
          </button>
        </div>
      )}
    </div>
  );
}
