import { useRef } from "react";
import { useQuery } from "@tanstack/react-query";

import { listAttachments, type AttachmentMeta } from "../../api/client";
import AttachmentDropzone from "./AttachmentDropzone";
import AttachmentCard from "./AttachmentCard";
import "./AttachmentsSection.css";

interface AttachmentsSectionProps {
  pagePath: string;
  canEdit: boolean;
  maxUploadMB: number;
  allowedTypes: string[];
}

// AttachmentsSection mounts under the page body in PageView (read mode). It shows
// the page's attachments as cards (or an empty state) and — for editors only —
// the "Add files" affordance and the dropzone. No Git vocabulary anywhere
// (hidden-Git rule).
export default function AttachmentsSection({
  pagePath,
  canEdit,
  maxUploadMB,
  allowedTypes,
}: AttachmentsSectionProps) {
  const dropzoneRef = useRef<HTMLDivElement>(null);

  const { data: attachments } = useQuery<AttachmentMeta[]>({
    queryKey: ["attachments", pagePath],
    queryFn: () => listAttachments(pagePath),
    enabled: pagePath !== "",
  });

  const items = attachments ?? [];

  return (
    <section className="attachments-section" aria-label="Attachments">
      <header className="attachments-header">
        <h2 className="attachments-heading">Attachments</h2>
        {canEdit && (
          <button
            type="button"
            className="btn btn-primary"
            onClick={() => dropzoneRef.current?.scrollIntoView({ block: "nearest" })}
          >
            Add files
          </button>
        )}
      </header>

      {canEdit && (
        <div ref={dropzoneRef}>
          <AttachmentDropzone
            pagePath={pagePath}
            maxUploadMB={maxUploadMB}
            allowedTypes={allowedTypes}
          />
        </div>
      )}

      {items.length === 0 ? (
        <div className="attachments-empty">
          <p className="attachments-empty-heading">No attachments yet</p>
          <p className="attachments-empty-body">
            Drop a file above to attach it to this page.
          </p>
        </div>
      ) : (
        <div className="attachments-list">
          {items.map((meta) => (
            <AttachmentCard
              key={meta.id}
              meta={meta}
              pagePath={pagePath}
              canEdit={canEdit}
            />
          ))}
        </div>
      )}
    </section>
  );
}
