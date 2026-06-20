import { Download } from "lucide-react";

import { downloadAttachmentUrl, type AttachmentMeta } from "../../api/client";
import "./AttachmentCard.css";

// AttachmentCard renders one attachment. MINIMAL for slice 02-01: the original
// filename plus a Download link to the byte-exact original. Thumbnail, meta line,
// preview, and extraction status are deferred to slice 02-02/02-03.
export default function AttachmentCard({ meta }: { meta: AttachmentMeta }) {
  return (
    <div className="attachment-card">
      <span className="attachment-card-name" title={meta.original_name}>
        {meta.original_name}
      </span>
      <a
        className="btn btn-ghost attachment-card-download"
        href={downloadAttachmentUrl(meta.id)}
        download={meta.original_name}
      >
        <Download size={16} aria-hidden="true" />
        <span>Download</span>
      </a>
    </div>
  );
}
