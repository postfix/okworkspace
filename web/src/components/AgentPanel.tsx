import { Loader2, PanelRightClose, Sparkles, FileDiff } from "lucide-react";

import AgentAnswer from "./AgentAnswer";
import type { AgentMode, PromptBarStatus } from "./PromptBar";
import "./AgentPanel.css";

// AgentPanel is the right-side collapsible answer column (the third column of
// .appshell-body). It mirrors the .navrail chrome (a left border instead of the
// rail's right border) and is a labeled peer landmark (aria-label="Assistant").
// AppShell owns the session state (answer accumulation, streaming lifecycle) and
// the open/collapse store; this component renders the states.
export interface AgentPanelProps {
  open: boolean;
  onClose: () => void;
  mode: AgentMode;
  // scopeLabel is the human "This page" / "Whole workspace" meta for the header.
  scopeLabel: string;
  status: PromptBarStatus;
  // answer is the accumulated streamed Markdown; empty before the first prompt.
  answer: string;
  citations?: string[];
  error?: string | null;
  currentPath: string;
  // submitted is true once the user has sent at least one prompt this session
  // (drives the empty vs. loading/answer states).
  submitted: boolean;
  // canPropose gates the "Propose this as a patch" footer (editor + page scope +
  // a completed answer). Readers never see it.
  canPropose: boolean;
  onProposeFromAnswer: () => void;
}

// MODE_LABEL is the human label for the header meta line (matches PromptBar copy).
const MODE_LABEL: Record<AgentMode, string> = {
  ask: "Ask",
  summarize: "Summarize",
  rewrite: "Rewrite",
  draft: "Draft",
  propose: "Propose a patch",
};

export default function AgentPanel({
  open,
  onClose,
  mode,
  scopeLabel,
  status,
  answer,
  citations,
  error,
  currentPath,
  submitted,
  canPropose,
  onProposeFromAnswer,
}: AgentPanelProps) {
  if (!open) return null;

  const streaming = status === "streaming";
  const thinking = status === "thinking";
  const done = submitted && status === "idle";

  return (
    <aside className="agentpanel" aria-label="Assistant">
      <header className="agentpanel-header">
        <div className="agentpanel-heading">
          <h2 className="agentpanel-title">Assistant</h2>
          <span className="agentpanel-meta">
            {MODE_LABEL[mode]} · {scopeLabel}
          </span>
        </div>
        <button
          type="button"
          className="btn btn-ghost agentpanel-collapse"
          aria-label="Hide assistant"
          onClick={onClose}
        >
          <PanelRightClose size={16} aria-hidden="true" />
        </button>
      </header>

      <div className="agentpanel-body">
        {!submitted && (
          <div className="agentpanel-empty">
            <Sparkles size={24} className="agentpanel-empty-icon" aria-hidden="true" />
            <h3 className="agentpanel-empty-heading">Ask the assistant anything</h3>
            <p className="agentpanel-empty-body">
              Choose a mode and ask about this page, your selection, an
              attachment, or the whole workspace.
            </p>
          </div>
        )}

        {submitted && thinking && !answer && (
          <p className="agentpanel-loading" aria-live="polite">
            <Loader2 size={16} className="spinner" aria-hidden="true" />
            <span>Thinking…</span>
          </p>
        )}

        {submitted && (answer || streaming || error) && (
          <AgentAnswer
            answer={answer}
            streaming={streaming}
            citations={citations}
            error={error}
            currentPath={currentPath}
          />
        )}
      </div>

      {/* Editor + page + a completed answer → offer to turn it into a reviewable
          patch. Readers never see this (canPropose is false for them). */}
      {done && canPropose && !error && (
        <footer className="agentpanel-footer">
          <button
            type="button"
            className="btn btn-secondary agentpanel-propose"
            onClick={onProposeFromAnswer}
          >
            <FileDiff size={16} aria-hidden="true" />
            Propose this as a patch
          </button>
        </footer>
      )}
    </aside>
  );
}
