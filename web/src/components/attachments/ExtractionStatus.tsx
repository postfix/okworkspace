import { AlertTriangle, Check, FileText, Loader2 } from "lucide-react";

import type { ExtractionStatusValue } from "../../api/client";
import "../AutosaveStatus.css";
import "./ExtractionStatus.css";

// ExtractionStatus is the live text-extraction chip on an attachment card. It is
// modeled VERBATIM on AutosaveStatus (same `.autosave-status` flex/gap/Label-size
// structure, `aria-live="polite"`, lucide icons at size 14, `aria-hidden`) and
// deliberately carries NO Git / processing-internals vocabulary (hidden-Git rule).
//
// Four mutually-exclusive states (UI-SPEC):
//   extracting → muted spinner, "Extracting text…"
//   done       → success Check, "Text extracted"
//   empty      → warning FileText, "No text extracted" + a muted sub-note (this is
//                the legitimate scanned/image-PDF result — non-alarming amber, NOT red)
//   failed     → destructive alert, "Couldn't extract text"
//
// The chip applies only to extractable types (pdf/docx/txt); the card renders it
// only for those, so this component is never mounted for images/other types.
export default function ExtractionStatus({
  status,
}: {
  status: ExtractionStatusValue;
}) {
  if (status === "extracting") {
    return (
      <span className="autosave-status autosave-muted" aria-live="polite">
        <Loader2 size={14} aria-hidden="true" className="autosave-spinner" />
        Extracting text…
      </span>
    );
  }
  if (status === "done") {
    return (
      <span className="autosave-status autosave-saved" aria-live="polite">
        <Check size={14} aria-hidden="true" />
        Text extracted
      </span>
    );
  }
  if (status === "empty") {
    return (
      <span className="extraction-status-empty-wrap" aria-live="polite">
        <span className="autosave-status extraction-status-warning">
          <FileText size={14} aria-hidden="true" />
          No text extracted
        </span>
        <span className="extraction-status-subnote">
          This file has no readable text layer (e.g. a scanned PDF).
        </span>
      </span>
    );
  }
  return (
    <span className="autosave-status extraction-status-failed" aria-live="polite">
      <AlertTriangle size={14} aria-hidden="true" />
      Couldn't extract text
    </span>
  );
}
