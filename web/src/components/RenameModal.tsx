import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "react-router-dom";
import Dialog from "./Dialog";
import { renamePage } from "../api/client";

export interface RenameModalProps {
  open: boolean;
  path: string;
  currentTitle: string;
  onClose: () => void;
}

// RenameModal renames a page to a new title (PAGE-04). Rename is non-destructive
// (links keep working), so the confirm uses the accent primary — never the
// destructive color. On success the SPA navigates to the new page path and the
// tree + page queries are invalidated.
export default function RenameModal({
  open,
  path,
  currentTitle,
  onClose,
}: RenameModalProps) {
  const [title, setTitle] = useState(currentTitle);
  const [error, setError] = useState<string | null>(null);
  const queryClient = useQueryClient();
  const navigate = useNavigate();

  const renameMut = useMutation({
    mutationFn: () => renamePage(path, title.trim()),
    onSuccess: (res) => {
      queryClient.invalidateQueries({ queryKey: ["tree"] });
      queryClient.invalidateQueries({ queryKey: ["page"] });
      onClose();
      navigate(`/app/page/${res.path}`);
    },
    onError: () => {
      setError("We couldn't rename your page just now. Try again.");
    },
  });

  function onConfirm() {
    if (title.trim() === "") {
      setError("Give your page a title.");
      return;
    }
    setError(null);
    renameMut.mutate();
  }

  return (
    <Dialog
      open={open}
      title="Rename page"
      onCancel={onClose}
      onConfirm={onConfirm}
      confirmLabel="Rename"
      busy={renameMut.isPending}
    >
      <div className="field">
        <label className="field-label" htmlFor="rename-title">
          New title
        </label>
        <input
          id="rename-title"
          className="input"
          type="text"
          value={title}
          onChange={(e) => setTitle(e.target.value)}
          autoFocus
        />
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
