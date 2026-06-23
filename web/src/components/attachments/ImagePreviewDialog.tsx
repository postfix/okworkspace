import Dialog from "../Dialog";
import { downloadAttachmentUrl, type AttachmentMeta } from "../../api/client";
import "./ImagePreviewDialog.css";

export interface ImagePreviewDialogProps {
  open: boolean;
  meta: AttachmentMeta;
  onClose: () => void;
}

// ImagePreviewDialog shows a full-size image preview inside the existing
// focus-trapped Dialog (Esc/backdrop close; the backdrop never mutates). The
// image is rendered via <img src> ONLY — never dangerouslySetInnerHTML and never
// inlined markup — so an uploaded SVG cannot execute script (ATT-04 stored-XSS
// guard, mirrors the MarkdownProse raw-HTML-off policy). The download endpoint
// serves images with X-Content-Type-Options: nosniff (SEC-02), so the browser
// treats an SVG as an image resource, not an HTML document. The dialog title is
// the original filename; no version-control vocabulary appears (hidden-Git rule).
export default function ImagePreviewDialog({
  open,
  meta,
  onClose,
}: ImagePreviewDialogProps) {
  return (
    <Dialog open={open} title={meta.original_name} onCancel={onClose} hideFooter>
      <div className="image-preview-body">
        <img
          className="image-preview-img"
          src={downloadAttachmentUrl(meta.id)}
          alt={meta.original_name}
        />
      </div>
    </Dialog>
  );
}
