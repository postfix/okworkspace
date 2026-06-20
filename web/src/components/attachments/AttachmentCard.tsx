import { useState } from "react";
import { Download, FileText, File as FileIcon } from "lucide-react";

import {
  downloadAttachmentUrl,
  humanDate,
  humanFileSize,
  isPreviewableImage,
  type AttachmentMeta,
} from "../../api/client";
import ImagePreviewDialog from "./ImagePreviewDialog";
import "./AttachmentCard.css";

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
export default function AttachmentCard({ meta }: { meta: AttachmentMeta }) {
  const previewable = isPreviewableImage(meta.mime_type);
  const Icon = typeIconFor(meta);
  const [previewOpen, setPreviewOpen] = useState(false);

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
      </div>

      <a
        className="btn btn-ghost attachment-card-download"
        href={downloadAttachmentUrl(meta.id)}
        download={meta.original_name}
      >
        <Download size={16} aria-hidden="true" />
        <span>Download</span>
      </a>

      {previewable && (
        <ImagePreviewDialog
          open={previewOpen}
          meta={meta}
          onClose={() => setPreviewOpen(false)}
        />
      )}
    </div>
  );
}
