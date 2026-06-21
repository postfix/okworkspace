import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "react-router-dom";
import Dialog from "./Dialog";
import { getTree, moveFolder, movePage, type TreeNode } from "../api/client";
import type { NodeKind } from "./RenameModal";

export interface MoveDialogProps {
  open: boolean;
  path: string;
  // kind defaults to "page" so every existing page call site is unchanged. The
  // folder branch is implemented but stays UNREACHED from LeftTree until Plan 04.
  kind?: NodeKind;
  onClose: () => void;
}

// collectFolders flattens the tree into a list of folder paths (the root is the
// "" option the caller adds). When excludePrefix is set (a folder being moved),
// the folder itself and its descendants are omitted — you cannot move a folder
// into itself or its own subtree.
function collectFolders(nodes: TreeNode[], excludePrefix?: string): string[] {
  const out: string[] = [];
  function isExcluded(p: string): boolean {
    if (!excludePrefix) return false;
    return p === excludePrefix || p.startsWith(`${excludePrefix}/`);
  }
  function walk(ns: TreeNode[]) {
    for (const n of ns) {
      if (n.type === "folder") {
        if (!isExcluded(n.path)) out.push(n.path);
        if (n.children) walk(n.children);
      }
    }
  }
  walk(nodes);
  return out;
}

// Per-kind copy. Page strings are byte-for-byte the shipped copy; folder strings
// are the net-new copy Plan 04 surfaces (UI-SPEC Copywriting Contract).
const COPY = {
  page: {
    dialogTitle: "Move page",
    intro: "Choose where this page should live.",
    help: "Links to this page will keep working.",
    failError: "We couldn't move your page just now. Try again.",
    confirm: "Move page",
  },
  folder: {
    dialogTitle: "Move folder",
    intro: "Choose where this folder should live.",
    help: "Links to pages in this folder will keep working.",
    failError: "We couldn't move your folder just now. Try again.",
    confirm: "Move folder",
  },
} as const;

// COLLISION_COPY is the non-fatal 409 message when a folder of that name already
// exists at the destination (TREE-06; never silently merge). The dialog stays
// open. Page move auto-suffixes server-side, so it never reaches this branch.
const COLLISION_COPY =
  "A folder with that name already exists there. Pick a different name or destination.";

// MoveDialog moves a page (PAGE-05) or a folder (TREE-02) into another folder.
// Move is non-destructive (links keep working, history continuous), so the
// confirm uses the accent primary — never the destructive color. On success the
// SPA navigates to the new path.
export default function MoveDialog({
  open,
  path,
  kind = "page",
  onClose,
}: MoveDialogProps) {
  const [parent, setParent] = useState("");
  const [error, setError] = useState<string | null>(null);
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const copy = COPY[kind];

  const { data: tree } = useQuery<TreeNode[]>({
    queryKey: ["tree"],
    queryFn: getTree,
    enabled: open,
  });
  // For a folder move, omit the folder and its own subtree from the destinations.
  const folders = useMemo(
    () => collectFolders(tree ?? [], kind === "folder" ? path : undefined),
    [tree, kind, path],
  );

  const moveMut = useMutation({
    mutationFn: () =>
      kind === "folder" ? moveFolder(path, parent) : movePage(path, parent),
    onSuccess: (res) => {
      queryClient.invalidateQueries({ queryKey: ["tree"] });
      queryClient.invalidateQueries({ queryKey: ["page"] });
      onClose();
      navigate(`/app/page/${res.path}`);
    },
    onError: (err: Error & { status?: number }) => {
      // A folder collision (409) is recoverable: surface the collision copy and
      // keep the dialog open. Any other failure shows the generic per-kind error.
      setError(err.status === 409 ? COLLISION_COPY : copy.failError);
    },
  });

  return (
    <Dialog
      open={open}
      title={copy.dialogTitle}
      onCancel={onClose}
      onConfirm={() => {
        setError(null);
        moveMut.mutate();
      }}
      confirmLabel={copy.confirm}
      busy={moveMut.isPending}
    >
      <p className="field-help">{copy.intro}</p>
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
        <p className="field-help">{copy.help}</p>
        {error && (
          <p className="field-help" role="alert">
            {error}
          </p>
        )}
      </div>
    </Dialog>
  );
}
