import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "react-router-dom";
import Dialog from "./Dialog";
import { countFolderPages, useFolderDelete } from "./hooks/useTreeMutations";
import type { TreeNode } from "../api/client";

export interface DeleteFolderDialogProps {
  open: boolean;
  // dir is the folder's repo-relative path; title is its display name.
  dir: string;
  title: string;
  onClose: () => void;
}

// DeleteFolderDialog confirms moving a whole folder — its index.md + every
// descendant page — to Trash (TREE-04/05). Like the page delete it is a
// recoverable recycle-bin action (D-08/D-09), so the copy is reassuring and names
// the affected page count N (UI-SPEC: for N == 1 use "its 1 page"). It uses the
// destructive confirm style; per the Dialog contract a backdrop click NEVER
// confirms — only the explicit "Delete folder" button does. On success the
// tree+trash queries reconcile and, if the open page lived inside the deleted
// folder, the user is returned to their workspace. NEVER surfaces Git vocabulary.
export default function DeleteFolderDialog({
  open,
  dir,
  title,
  onClose,
}: DeleteFolderDialogProps) {
  const [error, setError] = useState<string | null>(null);
  const queryClient = useQueryClient();
  const navigate = useNavigate();

  // Count the folder's descendant pages from the cached tree for the copy. The
  // cache is the same ["tree"] the dialog's optimistic delete prunes.
  const tree = queryClient.getQueryData<TreeNode[]>(["tree"]) ?? [];
  const pageCount = countFolderPages(tree, dir);
  const pagesLabel = pageCount === 1 ? "its 1 page" : `its ${pageCount} pages`;

  const deleteMut = useFolderDelete((message) => setError(message));

  function onConfirm() {
    setError(null);
    deleteMut.mutate(
      { dir },
      {
        onSuccess: () => {
          onClose();
          // If the open page lived inside the deleted folder, return to the
          // workspace so the SPA never renders a now-trashed page.
          const open = currentPagePath();
          if (open === dir || open.startsWith(`${dir}/`)) {
            navigate("/app");
          }
        },
      },
    );
  }

  return (
    <Dialog
      open={open}
      title={`Delete '${title}'?`}
      onCancel={onClose}
      onConfirm={onConfirm}
      confirmLabel="Delete folder"
      cancelLabel="Keep folder"
      destructive
      busy={deleteMut.isPending}
    >
      <p className="dialog-message">
        This folder and {pagesLabel} will move to Trash. You can restore the
        whole folder anytime — nothing is permanently removed.
      </p>
      {error && (
        <p className="field-help" role="alert">
          {error}
        </p>
      )}
    </Dialog>
  );
}

// currentPagePath reads the open page's repo-relative path from the URL
// (/app/page/<path>), or "" when no page is open. A plain location read keeps
// this dialog decoupled from the router param shape.
function currentPagePath(): string {
  const prefix = "/app/page/";
  const { pathname } = window.location;
  return pathname.startsWith(prefix)
    ? decodeURIComponent(pathname.slice(prefix.length))
    : "";
}
