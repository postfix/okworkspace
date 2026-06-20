import { useCallback, useState } from "react";
import { useDropzone, type FileRejection } from "react-dropzone";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { FilePlus } from "lucide-react";

import { uploadAttachment } from "../../api/client";

interface AttachmentDropzoneProps {
  pagePath: string;
  maxUploadMB: number;
  allowedTypes: string[];
}

// AttachmentDropzone is the drag-a-file-into-the-page upload target
// (react-dropzone). Client-side size/type pre-checks fail fast with a
// `.field-error` BEFORE the multipart POST — but the server is always the real
// security boundary (ATT-09). On success the page's attachment list is
// invalidated so the new card appears.
export default function AttachmentDropzone({
  pagePath,
  maxUploadMB,
  allowedTypes,
}: AttachmentDropzoneProps) {
  const queryClient = useQueryClient();
  const [clientError, setClientError] = useState<string | null>(null);

  const upload = useMutation({
    mutationFn: (file: File) => uploadAttachment(pagePath, file),
    onSuccess: () => {
      setClientError(null);
      void queryClient.invalidateQueries({ queryKey: ["attachments", pagePath] });
    },
    onError: (err: Error) => {
      setClientError(err.message);
    },
  });

  const allowedLabel =
    allowedTypes.length > 0 ? allowedTypes.join(", ") : "common file types";

  const onDrop = useCallback(
    (accepted: File[], rejections: FileRejection[]) => {
      setClientError(null);
      if (rejections.length > 0) {
        setClientError(
          `That file type isn't allowed. Accepted types: ${allowedLabel}.`,
        );
        return;
      }
      const file = accepted[0];
      if (!file) return;
      if (file.size > maxUploadMB * 1024 * 1024) {
        setClientError(
          `That file is too large. The limit is ${maxUploadMB} MB — try a smaller file.`,
        );
        return;
      }
      upload.mutate(file);
    },
    [allowedLabel, maxUploadMB, upload],
  );

  const { getRootProps, getInputProps, isDragActive } = useDropzone({
    onDrop,
    multiple: false,
  });

  let stateClass = "attachment-dropzone";
  if (isDragActive) stateClass += " attachment-dropzone-active";
  if (clientError) stateClass += " attachment-dropzone-error";

  return (
    <div>
      <div {...getRootProps({ className: stateClass })}>
        <input {...getInputProps()} />
        <FilePlus size={16} aria-hidden="true" />
        <div className="attachment-dropzone-text" aria-live="polite">
          {upload.isPending ? (
            <span className="attachment-dropzone-prompt">Uploading…</span>
          ) : isDragActive ? (
            <span className="attachment-dropzone-prompt attachment-dropzone-prompt-active">
              Drop to upload
            </span>
          ) : (
            <>
              <span className="attachment-dropzone-prompt">
                Drop a file here or click to browse
              </span>
              <span className="attachment-dropzone-hint">
                Up to {maxUploadMB} MB · {allowedLabel}
              </span>
            </>
          )}
        </div>
      </div>
      {clientError && (
        <p className="field-error attachment-dropzone-fielderror">{clientError}</p>
      )}
    </div>
  );
}
