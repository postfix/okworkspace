import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";

import Dialog from "../Dialog";
import { removeAttachment } from "../../api/client";

export interface RemoveAttachmentDialogProps {
  open: boolean;
  id: string;
  filename: string;
  pagePath: string;
  onClose: () => void;
}

// RemoveAttachmentDialog confirms removing an attachment's link from this page
// (ATT-06). When the removed link was the last reference anywhere, the file itself
// is deleted (ATT-07) — but the prior versions remain recoverable, so the copy is
// reassuring and carries NO Git vocabulary. Removing a link is a destructive action
// (the file may be deleted), so the confirm uses the destructive style. Per the
// Dialog contract a backdrop click / Esc CANCELS only; the explicit "Remove file"
// button is the only path that removes. On success the page's attachment list is
// refetched and the dialog closes.
export default function RemoveAttachmentDialog({
  open,
  id,
  filename,
  pagePath,
  onClose,
}: RemoveAttachmentDialogProps) {
  const [error, setError] = useState<string | null>(null);
  const queryClient = useQueryClient();

  const removeMut = useMutation({
    mutationFn: () => removeAttachment(id, pagePath),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["attachments", pagePath] });
      onClose();
    },
    onError: () => {
      setError("We couldn't remove that file just now. Try again.");
    },
  });

  return (
    <Dialog
      open={open}
      title="Remove attachment"
      onCancel={() => {
        setError(null);
        onClose();
      }}
      onConfirm={() => {
        setError(null);
        removeMut.mutate();
      }}
      confirmLabel="Remove file"
      cancelLabel="Cancel"
      destructive
      busy={removeMut.isPending}
    >
      <p className="dialog-message">
        Remove &ldquo;{filename}&rdquo; from this page? If no other page uses it,
        the file is deleted.
      </p>
      {error && (
        <p className="field-help" role="alert">
          {error}
        </p>
      )}
    </Dialog>
  );
}
