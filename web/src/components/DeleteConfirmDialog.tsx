import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "react-router-dom";
import Dialog from "./Dialog";
import { deletePage } from "../api/client";

export interface DeleteConfirmDialogProps {
  open: boolean;
  path: string;
  title: string;
  onClose: () => void;
}

// DeleteConfirmDialog confirms moving a page to Trash (PAGE-06). Delete is a
// recoverable recycle-bin action, not a permanent destruction (D-08/D-09), so
// the copy is reassuring: the page moves to Trash and can be restored anytime.
// It uses the destructive confirm style, and — per the Dialog contract — a
// backdrop click NEVER confirms; only the explicit "Delete" button does. On
// success the tree + trash queries are invalidated and the user is returned to
// their workspace. NEVER surfaces Git vocabulary.
export default function DeleteConfirmDialog({
  open,
  path,
  title,
  onClose,
}: DeleteConfirmDialogProps) {
  const [error, setError] = useState<string | null>(null);
  const queryClient = useQueryClient();
  const navigate = useNavigate();

  const deleteMut = useMutation({
    mutationFn: () => deletePage(path),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["tree"] });
      queryClient.invalidateQueries({ queryKey: ["trash"] });
      onClose();
      navigate("/app");
    },
    onError: () => {
      setError("We couldn't delete your page just now. Try again.");
    },
  });

  return (
    <Dialog
      open={open}
      title={`Delete '${title}'?`}
      onCancel={onClose}
      onConfirm={() => {
        setError(null);
        deleteMut.mutate();
      }}
      confirmLabel="Delete"
      cancelLabel="Keep page"
      destructive
      busy={deleteMut.isPending}
    >
      <p className="dialog-message">
        It will move to Trash. You can restore it anytime — nothing is
        permanently removed.
      </p>
      {error && (
        <p className="field-help" role="alert">
          {error}
        </p>
      )}
    </Dialog>
  );
}
