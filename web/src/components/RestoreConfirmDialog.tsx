import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import Dialog from "./Dialog";
import { restoreVersion } from "../api/client";

export interface RestoreConfirmDialogProps {
  open: boolean;
  path: string;
  // version is the OPAQUE token of the version to restore (never shown to the
  // user). dateLabel is the friendly date the version is from (e.g. "2 hours
  // ago"), shown in the reassuring copy.
  version: string;
  dateLabel: string;
  onClose: () => void;
  // onRestored fires after a successful restore so the parent panel can close.
  onRestored?: () => void;
}

// RestoreConfirmDialog confirms restoring an old version as a NEW forward version
// (VER-03). Restore is NON-destructive — the current version is kept in history,
// so this is an accent (primary) confirm, never a destructive one. Per the Dialog
// contract a backdrop click never confirms. NEVER surfaces Git vocabulary: the
// copy says "version" only, never any version-control jargon.
export default function RestoreConfirmDialog({
  open,
  path,
  version,
  dateLabel,
  onClose,
  onRestored,
}: RestoreConfirmDialogProps) {
  const [error, setError] = useState<string | null>(null);
  const queryClient = useQueryClient();

  const restoreMut = useMutation({
    mutationFn: () => restoreVersion(path, version),
    onSuccess: () => {
      // The page now has a new forward version — refresh the page + its history.
      queryClient.invalidateQueries({ queryKey: ["page", path] });
      queryClient.invalidateQueries({ queryKey: ["history", path] });
      onClose();
      onRestored?.();
    },
    onError: () => {
      setError("We couldn't restore that version just now. Try again.");
    },
  });

  return (
    <Dialog
      open={open}
      title="Restore this version?"
      onCancel={onClose}
      onConfirm={() => {
        setError(null);
        restoreMut.mutate();
      }}
      confirmLabel="Restore this version"
      cancelLabel="Keep current version"
      busy={restoreMut.isPending}
    >
      <p className="dialog-message">
        This brings the page back to how it looked {dateLabel}. Your current
        version is kept in history, so nothing is lost.
      </p>
      {error && (
        <p className="field-help" role="alert">
          {error}
        </p>
      )}
    </Dialog>
  );
}
