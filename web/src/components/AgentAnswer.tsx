import { useNavigate } from "react-router-dom";
import { AlertTriangle } from "lucide-react";

import MarkdownProse from "./MarkdownProse";
import "./AgentAnswer.css";

// AgentAnswer renders a streamed agent answer. The answer Markdown goes through
// the SAME sanitized read-only surface as page content (MarkdownProse — remark-gfm
// + rehype-sanitize, rehype-raw OFF), so model output can never become stored XSS
// (T-04-21) and reads visually identical to the rest of the workspace.
//
// The region is aria-live="polite" + aria-busy while streaming so a screen reader
// hears coherent settled chunks (announced on update, never per-character). For
// workspace scope it renders the "Reasoned over:" citation links the SSE citation
// frame supplied. The error state KEEPS the partial answer + the user's prompt.
export interface AgentAnswerProps {
  // answer is the accumulated streamed Markdown (may be partial while streaming).
  answer: string;
  streaming: boolean;
  // citations are workspace-scope page paths from the SSE `event: citation` frame.
  citations?: string[];
  // error is a terminal failure message (mid-stream error frame or a pre-stream
  // structured error). The partial answer above it is kept.
  error?: string | null;
  // currentPath anchors relative `.md` links inside the answer for SPA navigation.
  currentPath: string;
}

export default function AgentAnswer({
  answer,
  streaming,
  citations,
  error,
  currentPath,
}: AgentAnswerProps) {
  const navigate = useNavigate();

  return (
    <div
      className="agent-answer"
      aria-live="polite"
      aria-busy={streaming}
    >
      {answer && (
        <div className={`agent-answer-body${streaming ? " is-streaming" : ""}`}>
          <MarkdownProse body={answer} currentPath={currentPath} />
          {streaming && (
            // A static caret marker (no pulse under reduced motion — see CSS).
            <span className="agent-answer-caret" aria-hidden="true" />
          )}
        </div>
      )}

      {!error && !streaming && citations && citations.length > 0 && (
        <p className="agent-citations">
          <span className="agent-citations-label">Reasoned over:</span>{" "}
          {citations.map((path, i) => (
            <span key={path}>
              {i > 0 && ", "}
              <a
                href={`/app/page/${path}`}
                className="prose-link"
                onClick={(e) => {
                  e.preventDefault();
                  navigate(`/app/page/${path}`);
                }}
              >
                {citationLabel(path)}
              </a>
            </span>
          ))}
        </p>
      )}

      {error && (
        <p className="agent-answer-error" role="alert">
          <AlertTriangle size={16} aria-hidden="true" />
          <span>{error}</span>
        </p>
      )}
    </div>
  );
}

// citationLabel derives a human-readable label from a page path: the filename
// without its .md extension (index → its folder), so a citation link reads as a
// title rather than a raw slug path. No Git/path internals are surfaced as such.
function citationLabel(path: string): string {
  const parts = path.split("/");
  const file = parts[parts.length - 1].replace(/\.md$/i, "");
  if (file === "index" && parts.length > 1) return parts[parts.length - 2];
  return file;
}
