import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link2 } from "lucide-react";
import { getTree, relativeMdLink, type TreeNode } from "../api/client";
import "./LinkPicker.css";

export interface LinkPickerProps {
  // fromPath is the repo-relative path of the page being edited; the emitted
  // link destination is computed relative to it (D-05).
  fromPath: string;
  // onInsert receives the Markdown link text `[title](relative.md)` to splice
  // into the editor body at the cursor (D-06).
  onInsert: (markdown: string) => void;
}

interface PageEntry {
  path: string;
  title: string;
}

// flattenPages collects every page (leaf) in the tree with its title and path.
function flattenPages(nodes: TreeNode[]): PageEntry[] {
  const out: PageEntry[] = [];
  function walk(ns: TreeNode[]) {
    for (const n of ns) {
      if (n.type === "page") {
        out.push({ path: n.path, title: n.title });
      } else if (n.children) {
        walk(n.children);
      }
    }
  }
  walk(nodes);
  return out;
}

// LinkPicker is the editor "Link to page" affordance (PAGE-08). It opens a
// searchable list of pages; selecting one inserts a relative `.md` Markdown link
// (D-05) at the cursor. The link is a portable relative path — never a wiki-style
// double-bracket link or app-only ID link — so content stays agent-readable
// off-server. In read mode such links navigate within the app (D-06).
export default function LinkPicker({ fromPath, onInsert }: LinkPickerProps) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");

  const { data: tree } = useQuery<TreeNode[]>({
    queryKey: ["tree"],
    queryFn: getTree,
    enabled: open,
  });

  const pages = useMemo(() => flattenPages(tree ?? []), [tree]);
  const matches = useMemo(() => {
    const q = query.trim().toLowerCase();
    const candidates = pages.filter((p) => p.path !== fromPath);
    if (q === "") return candidates;
    return candidates.filter(
      (p) =>
        p.title.toLowerCase().includes(q) || p.path.toLowerCase().includes(q),
    );
  }, [pages, query, fromPath]);

  function select(p: PageEntry) {
    const dest = relativeMdLink(fromPath, p.path);
    onInsert(`[${p.title}](${dest})`);
    setOpen(false);
    setQuery("");
  }

  return (
    <div className="linkpicker">
      <button
        type="button"
        className="btn btn-secondary"
        onClick={() => setOpen((v) => !v)}
        aria-expanded={open}
        aria-haspopup="listbox"
      >
        <Link2 size={16} aria-hidden="true" />
        <span>Link to page</span>
      </button>
      {open && (
        <div className="linkpicker-popover">
          <input
            className="input"
            type="text"
            placeholder="Search pages…"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            aria-label="Search pages"
            autoFocus
          />
          {matches.length === 0 ? (
            <p className="linkpicker-empty">No pages match your search.</p>
          ) : (
            <ul className="linkpicker-list" role="listbox">
              {matches.map((p) => (
                <li key={p.path}>
                  <button
                    type="button"
                    className="linkpicker-item"
                    role="option"
                    aria-selected="false"
                    onClick={() => select(p)}
                  >
                    {p.title}
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
    </div>
  );
}
