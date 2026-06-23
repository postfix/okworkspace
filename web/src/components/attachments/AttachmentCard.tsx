import { useEffect, useState } from "react";
import {
  Download,
  FileText,
  File as FileIcon,
  Sparkles,
  Undo2,
  Trash2,
} from "lucide-react";

import {
  downloadAttachmentUrl,
  humanDate,
  humanFileSize,
  isPreviewableImage,
  subscribeExtractionStatus,
  type AttachmentMeta,
  type ExtractionStatusValue,
} from "../../api/client";
import { useAgentContext } from "../../stores/agentContext";
import { useAgentPanel } from "../../stores/agentPanel";
import ExtractionStatus from "./ExtractionStatus";
import ImagePreviewDialog from "./ImagePreviewDialog";
import ReplaceAttachmentDialog from "./ReplaceAttachmentDialog";
import RemoveAttachmentDialog from "./RemoveAttachmentDialog";
import "./AttachmentCard.css";

// extractableMediaTypes are the MIME types whose text the backend extracts (pdf /
// docx / txt). Only these show an extraction-status chip; images and other types
// show none (UI-SPEC). Mirrors the server's extractable set.
const extractableMediaTypes = new Set([
  "application/pdf",
  "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
  "text/plain",
]);

// isExtractable reports whether an attachment's stored MIME type is one the chip
// applies to. The stored mime_type may carry parameters (e.g. "text/plain;
// charset=utf-8"), so only the media type before the first ";" is compared.
function isExtractable(mimeType: string): boolean {
  return extractableMediaTypes.has(mimeType.split(";", 1)[0].trim().toLowerCase());
}

// seedExtractionStatus maps the list item's stored extraction_status to the live
// chip value so the chip renders a sensible last-known state before (and if) the
// SSE stream connects. A pending/undefined row is shown as "extracting".
function seedExtractionStatus(
  raw: AttachmentMeta["extraction_status"],
): ExtractionStatusValue {
  if (raw === "done" || raw === "empty" || raw === "failed") return raw;
  return "extracting";
}

// typeIconFor picks the lucide icon for a non-image attachment: FileText for the
// document/text family (pdf/docx/txt), a generic file icon otherwise. Images use
// a thumbnail instead and never reach this helper.
function typeIconFor(meta: AttachmentMeta) {
  const media = meta.mime_type.split(";", 1)[0].trim().toLowerCase();
  const docTypes = new Set([
    "application/pdf",
    "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
    "text/plain",
  ]);
  return docTypes.has(media) ? FileText : FileIcon;
}

// AttachmentCard renders one attachment as the full ATT-03 card: a 64×64 media
// square (an image thumbnail for png/jpg/svg, a type icon otherwise), the
// emphasised original filename, a muted `size · uploader · date` meta line, and a
// Download affordance to the byte-exact original. The opaque on-disk id is never
// shown and no version-control vocabulary appears (hidden-Git rule). The date and
// size are always human-friendly.
export default function AttachmentCard({
  meta,
  pagePath,
  canEdit = false,
}: {
  meta: AttachmentMeta;
  pagePath: string;
  canEdit?: boolean;
}) {
  const previewable = isPreviewableImage(meta.mime_type);
  const Icon = typeIconFor(meta);
  const [previewOpen, setPreviewOpen] = useState(false);
  const [replaceOpen, setReplaceOpen] = useState(false);
  const [removeOpen, setRemoveOpen] = useState(false);

  // Extraction status: only extractable types (pdf/docx/txt) show a chip. Seed
  // from the list item's stored status so a dropped/late stream still renders the
  // last-known state (no error flash, UI-SPEC), then track live over SSE. The
  // subscription is torn down on unmount / id change.
  const extractable = isExtractable(meta.mime_type);
  const [extractStatus, setExtractStatus] = useState<ExtractionStatusValue>(() =>
    seedExtractionStatus(meta.extraction_status),
  );
  useEffect(() => {
    if (!extractable) return;
    // If the stored status is already terminal, the chip is correct and a stream
    // would only emit the same terminal value once before closing — still cheap,
    // and it keeps a just-uploaded "pending" card live, so always subscribe.
    const unsubscribe = subscribeExtractionStatus(meta.id, setExtractStatus);
    return unsubscribe;
  }, [extractable, meta.id]);

  return (
    <div className="attachment-card">
      <div className="attachment-card-media">
        {previewable ? (
          <button
            type="button"
            className="attachment-card-thumb-button"
            onClick={() => setPreviewOpen(true)}
            aria-label={`Preview ${meta.original_name}`}
          >
            <img
              className="attachment-card-thumb"
              src={downloadAttachmentUrl(meta.id)}
              alt=""
            />
          </button>
        ) : (
          <span className="attachment-card-icon" aria-hidden="true">
            <Icon size={24} aria-hidden="true" />
          </span>
        )}
      </div>

      <div className="attachment-card-main">
        <span className="attachment-card-name" title={meta.original_name}>
          {meta.original_name}
        </span>
        <span className="attachment-card-meta">
          {humanFileSize(meta.size_bytes)} · {meta.uploader_name} ·{" "}
          {humanDate(meta.uploaded_at)}
        </span>
        {extractable && (
          <span className="attachment-card-extract">
            <ExtractionStatus status={extractStatus} />
          </span>
        )}
      </div>

      <div className="attachment-card-actions">
        <a
          className="btn btn-ghost attachment-card-download"
          href={downloadAttachmentUrl(meta.id)}
          download={meta.original_name}
        >
          <Download size={16} aria-hidden="true" />
          <span>Download</span>
        </a>

        {/* "Ask about this file" (AGNT-03/06) — sets the attachment as the agent
            context and opens the panel so the user sees where it landed. NOT
            editor-gated (Ask/Summarize are open to readers), and it must NOT
            trigger the download or any preview — it is its own button. */}
        <button
          type="button"
          className="btn btn-ghost attachment-card-action attachment-card-ask"
          aria-label="Ask the assistant about this file"
          onClick={(e) => {
            e.stopPropagation();
            useAgentContext.getState().setAttachment({
              id: meta.id,
              name: meta.original_name,
            });
            useAgentPanel.getState().setOpen(true);
          }}
        >
          <Sparkles size={16} aria-hidden="true" />
        </button>

        {/* Editor-only lifecycle actions (ATT-05/06/07). Readers see only
            Download + Ask. Each is a 44px icon-only ghost button (--hit-min-icon). */}
        {canEdit && (
          <>
            <button
              type="button"
              className="btn btn-ghost attachment-card-action"
              aria-label="Replace attachment"
              onClick={() => setReplaceOpen(true)}
            >
              <Undo2 size={16} aria-hidden="true" />
            </button>
            <button
              type="button"
              className="btn btn-ghost-destructive attachment-card-action"
              aria-label="Remove attachment"
              onClick={() => setRemoveOpen(true)}
            >
              <Trash2 size={16} aria-hidden="true" />
            </button>
          </>
        )}
      </div>

      {previewable && (
        <ImagePreviewDialog
          open={previewOpen}
          meta={meta}
          onClose={() => setPreviewOpen(false)}
        />
      )}

      {canEdit && (
        <>
          <ReplaceAttachmentDialog
            open={replaceOpen}
            id={meta.id}
            filename={meta.original_name}
            pagePath={pagePath}
            onClose={() => setReplaceOpen(false)}
          />
          <RemoveAttachmentDialog
            open={removeOpen}
            id={meta.id}
            filename={meta.original_name}
            pagePath={pagePath}
            onClose={() => setRemoveOpen(false)}
          />
        </>
      )}
    </div>
  );
}
