import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "react-router-dom";
import Dialog from "./Dialog";
import { renameFolder, renamePage } from "../api/client";

// NodeKind parameterizes the shared rename/move dialogs over what is being acted
// on. The page path is fully wired today; the folder branch is implemented but
// stays UNREACHED from LeftTree until Plan 04 opens these dialogs for folders.
export type NodeKind = "page" | "folder";

export interface RenameModalProps {
  open: boolean;
  path: string;
  currentTitle: string;
  // kind defaults to "page" so every existing page call site is unchanged.
  kind?: NodeKind;
  onClose: () => void;
}

// Per-kind copy. Page strings are byte-for-byte the shipped copy; folder strings
// are the net-new copy Plan 04 surfaces (UI-SPEC Copywriting Contract).
const COPY = {
  page: {
    dialogTitle: "Rename page",
    fieldLabel: "New title",
    help: "Links to this page will keep working.",
    emptyError: "Give your page a title.",
    failError: "We couldn't rename your page just now. Try again.",
    confirm: "Rename",
  },
  folder: {
    dialogTitle: "Rename folder",
    fieldLabel: "New name",
    help: "Links to pages in this folder will keep working.",
    emptyError: "Give your folder a name.",
    failError: "We couldn't rename your folder just now. Try again.",
    confirm: "Rename",
  },
} as const;

// COLLISION_COPY is the non-fatal 409 message shown when a folder of that name
// already exists at the destination (TREE-06; never silently merge). The dialog
// stays open so the user can pick a different name. Page rename auto-suffixes
// server-side, so it never reaches this branch.
const COLLISION_COPY =
  "A folder with that name already exists there. Pick a different name or destination.";

// RenameModal renames a page (PAGE-04) or a folder (TREE-02). Rename is
// non-destructive (links keep working), so the confirm uses the accent primary —
// never the destructive color. On success the SPA navigates to the new path and
// the tree + page queries are invalidated.
export default function RenameModal({
  open,
  path,
  currentTitle,
  kind = "page",
  onClose,
}: RenameModalProps) {
  const [title, setTitle] = useState(currentTitle);
  const [error, setError] = useState<string | null>(null);
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const copy = COPY[kind];

  const renameMut = useMutation({
    mutationFn: () =>
      kind === "folder"
        ? renameFolder(path, title.trim())
        : renamePage(path, title.trim()),
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

  function onConfirm() {
    if (title.trim() === "") {
      setError(copy.emptyError);
      return;
    }
    setError(null);
    renameMut.mutate();
  }

  return (
    <Dialog
      open={open}
      title={copy.dialogTitle}
      onCancel={onClose}
      onConfirm={onConfirm}
      confirmLabel={copy.confirm}
      busy={renameMut.isPending}
    >
      <div className="field">
        <label className="field-label" htmlFor="rename-title">
          {copy.fieldLabel}
        </label>
        <input
          id="rename-title"
          className="input"
          type="text"
          value={title}
          onChange={(e) => setTitle(e.target.value)}
          autoFocus
        />
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
