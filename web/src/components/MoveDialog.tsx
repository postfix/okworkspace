import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "react-router-dom";
import Dialog from "./Dialog";
import { getTree, movePage, type TreeNode } from "../api/client";

export interface MoveDialogProps {
  open: boolean;
  path: string;
  onClose: () => void;
}

// collectFolders flattens the tree into a list of folder paths (plus the root)
// so the user can choose a destination. The root is represented by "" with a
// human label.
function collectFolders(nodes: TreeNode[]): string[] {
  const out: string[] = [];
  function walk(ns: TreeNode[]) {
    for (const n of ns) {
      if (n.type === "folder") {
        out[out.length] = n.path;
        if (n.children) walk(n.children);
      }
    }
  }
  walk(nodes);
  return out;
}

// MoveDialog moves a page into another folder (PAGE-05). Move is non-destructive
// (links keep working, history continuous), so the confirm uses the accent
// primary — never the destructive color. On success the SPA navigates to the new
// page path.
export default function MoveDialog({ open, path, onClose }: MoveDialogProps) {
  const [parent, setParent] = useState("");
  const [error, setError] = useState<string | null>(null);
  const queryClient = useQueryClient();
  const navigate = useNavigate();

  const { data: tree } = useQuery<TreeNode[]>({
    queryKey: ["tree"],
    queryFn: getTree,
    enabled: open,
  });
  const folders = useMemo(() => collectFolders(tree ?? []), [tree]);

  const moveMut = useMutation({
    mutationFn: () => movePage(path, parent),
    onSuccess: (res) => {
      queryClient.invalidateQueries({ queryKey: ["tree"] });
      queryClient.invalidateQueries({ queryKey: ["page"] });
      onClose();
      navigate(`/app/page/${res.path}`);
    },
    onError: () => {
      setError("We couldn't move your page just now. Try again.");
    },
  });

  return (
    <Dialog
      open={open}
      title="Move page"
      onCancel={onClose}
      onConfirm={() => {
        setError(null);
        moveMut.mutate();
      }}
      confirmLabel="Move page"
      busy={moveMut.isPending}
    >
      <p className="field-help">Choose where this page should live.</p>
      <div className="field">
        <label className="field-label" htmlFor="move-parent">
          Folder
        </label>
        <select
          id="move-parent"
          className="input"
          value={parent}
          onChange={(e) => setParent(e.target.value)}
        >
          <option value="">Top level</option>
          {folders.map((f) => (
            <option key={f} value={f}>
              {f}
            </option>
          ))}
        </select>
        <p className="field-help">Links to this page will keep working.</p>
        {error && (
          <p className="field-help" role="alert">
            {error}
          </p>
        )}
      </div>
    </Dialog>
  );
}
