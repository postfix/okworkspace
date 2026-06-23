import { useRef, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";

import Dialog from "../Dialog";
import { replaceAttachment } from "../../api/client";

export interface ReplaceAttachmentDialogProps {
  open: boolean;
  id: string;
  filename: string;
  pagePath: string;
  onClose: () => void;
}

// ReplaceAttachmentDialog confirms swapping an attachment's bytes for a newly
// picked file, reusing the SAME id (ATT-05). Replace is NON-destructive (the prior
// version is retained and can be restored), so the confirm uses the primary
// (.btn-primary) style — never the destructive style. Per the Dialog contract a
// backdrop click / Esc CANCELS only; the explicit confirm button is the only path
// that replaces. On success the page's attachment list is refetched and the dialog
// closes. The copy conveys version retention WITHOUT any Git vocabulary
// ("kept in history and can be restored").
export default function ReplaceAttachmentDialog({
  open,
  id,
  filename,
  pagePath,
  onClose,
}: ReplaceAttachmentDialogProps) {
  const [error, setError] = useState<string | null>(null);
  const [file, setFile] = useState<File | null>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const queryClient = useQueryClient();

  const replaceMut = useMutation({
    mutationFn: () => {
      if (!file) {
        return Promise.reject(new Error("Choose a file to replace it with."));
      }
      return replaceAttachment(id, file);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["attachments", pagePath] });
      setFile(null);
      onClose();
    },
    onError: (e: unknown) => {
      setError(
        e instanceof Error
          ? e.message
          : "We couldn't replace that file just now. Try again.",
      );
    },
  });

  return (
    <Dialog
      open={open}
      title="Replace attachment"
      onCancel={() => {
        setError(null);
        setFile(null);
        onClose();
      }}
      onConfirm={() => {
        setError(null);
        replaceMut.mutate();
      }}
      confirmLabel="Replace file"
      cancelLabel="Cancel"
      busy={replaceMut.isPending}
    >
      <p className="dialog-message">
        Replace &ldquo;{filename}&rdquo; with a new file? The current file is
        kept in history and can be restored.
      </p>
      <input
        ref={fileInputRef}
        type="file"
        aria-label="New file"
        onChange={(e) => {
          setError(null);
          setFile(e.target.files?.[0] ?? null);
        }}
      />
      {error && (
        <p className="field-help" role="alert">
          {error}
        </p>
      )}
    </Dialog>
  );
}
